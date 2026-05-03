package dockercompose

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/dominikpalatynski/toolshed/internal/runtime"
)

const (
	backendName               = "docker_compose"
	renderedComposeFileName   = ".toolshed.docker-compose.rendered.yml"
	toolshedPrivateNetworkKey = "toolshed_private"
	toolshedIngressNetworkKey = "toolshed_ingress"
)

type commandRunner interface {
	Run(ctx context.Context, dir string, args []string) ([]byte, error)
}

type Backend struct {
	config Config
	runner commandRunner
}

type Config struct {
	IngressNetwork       string
	DefaultServiceCPUs   float64
	DefaultServiceMemory string
	DefaultServicePIDs   int
}

type DeploymentMetadata struct {
	RuntimeEnvironmentID    int64  `json:"runtime_environment_id"`
	RepositoryID            int64  `json:"repository_id"`
	PullRequestID           int64  `json:"pull_request_id"`
	TargetCommitSHA         string `json:"target_commit_sha"`
	ProjectName             string `json:"project_name"`
	SourceComposeFilePath   string `json:"source_compose_file_path"`
	RenderedComposeFilePath string `json:"rendered_compose_file_path"`
	RenderedComposeYAML     string `json:"rendered_compose_yaml"`
	PrivateNetworkName      string `json:"private_network_name"`
	IngressNetworkName      string `json:"ingress_network_name"`
	ExposedServiceName      string `json:"exposed_service_name"`
	ExposedServicePort      int32  `json:"exposed_service_port"`
}

type composeDocument struct {
	Services map[string]map[string]any `yaml:"services"`
	Networks map[string]map[string]any `yaml:"networks,omitempty"`
	Extras   map[string]any            `yaml:",inline"`
}

func NewBackend(config Config) *Backend {
	return &Backend{
		config: config,
		runner: execCommandRunner{},
	}
}

func NewBackendWithRunner(config Config, runner commandRunner) *Backend {
	return &Backend{
		config: config,
		runner: runner,
	}
}

func (b *Backend) Name() string {
	return backendName
}

func (b *Backend) Prepare(ctx context.Context, request runtime.PrepareRequest) (runtime.DeploymentRecord, error) {
	settings, err := validateRuntimeSettings(request.RuntimeSettings)
	if err != nil {
		return runtime.DeploymentRecord{}, runtime.PermanentError(err)
	}

	if _, err := validateComposeFilePath(request.Workspace, settings.ComposeFilePath); err != nil {
		return runtime.DeploymentRecord{}, runtime.PermanentError(err)
	}

	sourceComposeYAML, err := request.Workspace.ReadFile(settings.ComposeFilePath)
	if err != nil {
		return runtime.DeploymentRecord{}, runtime.PermanentError(fmt.Errorf("read compose file %q: %w", settings.ComposeFilePath, err))
	}

	document, err := parseComposeDocument(sourceComposeYAML)
	if err != nil {
		return runtime.DeploymentRecord{}, runtime.PermanentError(err)
	}

	renderedDocument, err := b.sanitizeComposeDocument(document, request, settings)
	if err != nil {
		return runtime.DeploymentRecord{}, runtime.PermanentError(err)
	}

	renderedComposeYAML, err := yaml.Marshal(renderedDocument)
	if err != nil {
		return runtime.DeploymentRecord{}, runtime.RetryableError(fmt.Errorf("marshal rendered compose file: %w", err))
	}

	renderedComposeFilePath, err := request.Workspace.WriteFileAdjacentTo(
		settings.ComposeFilePath,
		renderedComposeFileName,
		renderedComposeYAML,
		0o644,
	)
	if err != nil {
		return runtime.DeploymentRecord{}, runtime.RetryableError(err)
	}

	metadata := DeploymentMetadata{
		RuntimeEnvironmentID:    request.RuntimeEnvironmentID,
		RepositoryID:            request.RepositoryID,
		PullRequestID:           request.PullRequestID,
		TargetCommitSHA:         request.TargetCommitSHA,
		ProjectName:             composeProjectName(request.RuntimeEnvironmentID),
		SourceComposeFilePath:   settings.ComposeFilePath,
		RenderedComposeFilePath: renderedComposeFilePath,
		RenderedComposeYAML:     string(renderedComposeYAML),
		PrivateNetworkName:      privateNetworkName(request.RuntimeEnvironmentID),
		IngressNetworkName:      b.config.IngressNetwork,
		ExposedServiceName:      settings.ExposedServiceName,
		ExposedServicePort:      settings.ExposedServicePort,
	}

	if _, err := b.runner.Run(ctx, request.Workspace.Path(), []string{
		"compose",
		"-p", metadata.ProjectName,
		"-f", metadata.RenderedComposeFilePath,
		"config",
	}); err != nil {
		return runtime.DeploymentRecord{}, classifyComposeError("config", err, true)
	}

	frozenRuntimeSettingsJSON, err := marshalJSON(settings)
	if err != nil {
		return runtime.DeploymentRecord{}, runtime.RetryableError(fmt.Errorf("marshal frozen runtime settings: %w", err))
	}

	frozenEnvironmentVariablesJSON, err := marshalJSON(sortedEnvironmentVariables(request.EnvironmentVariables))
	if err != nil {
		return runtime.DeploymentRecord{}, runtime.RetryableError(fmt.Errorf("marshal frozen environment variables: %w", err))
	}

	metadataJSON, err := marshalJSON(metadata)
	if err != nil {
		return runtime.DeploymentRecord{}, runtime.RetryableError(fmt.Errorf("marshal deployment metadata: %w", err))
	}

	return runtime.DeploymentRecord{
		Backend:                        backendName,
		FrozenRuntimeSettingsJSON:      frozenRuntimeSettingsJSON,
		FrozenEnvironmentVariablesJSON: frozenEnvironmentVariablesJSON,
		MetadataJSON:                   metadataJSON,
	}, nil
}

func (b *Backend) PreparedArtifactsExist(
	_ context.Context,
	request runtime.PreparedArtifactsRequest,
) (bool, error) {
	metadata, err := decodeMetadata(request.Deployment)
	if err != nil {
		return false, err
	}

	return request.Workspace.FileExists(metadata.RenderedComposeFilePath)
}

func (b *Backend) Deploy(ctx context.Context, request runtime.DeployRequest) error {
	metadata, err := decodeMetadata(request.Deployment)
	if err != nil {
		return runtime.PermanentError(err)
	}

	if err := ensureRenderedComposeArtifact(request.Workspace, metadata); err != nil {
		return runtime.RetryableError(err)
	}

	if _, err := b.runner.Run(ctx, request.Workspace.Path(), []string{
		"compose",
		"-p", metadata.ProjectName,
		"-f", metadata.RenderedComposeFilePath,
		"up",
		"-d",
		"--remove-orphans",
	}); err != nil {
		return classifyComposeError("up", err, false)
	}

	return nil
}

func (b *Backend) Teardown(ctx context.Context, request runtime.TeardownRequest) error {
	metadata, err := decodeMetadata(request.Deployment)
	if err != nil {
		return runtime.PermanentError(err)
	}

	if err := ensureRenderedComposeArtifact(request.Workspace, metadata); err != nil {
		return runtime.RetryableError(err)
	}

	if _, err := b.runner.Run(ctx, request.Workspace.Path(), []string{
		"compose",
		"-p", metadata.ProjectName,
		"-f", metadata.RenderedComposeFilePath,
		"down",
		"--remove-orphans",
		"--volumes",
	}); err != nil {
		return classifyComposeError("down", err, false)
	}

	return nil
}

func validateRuntimeSettings(settings runtime.RuntimeSettings) (runtime.RuntimeSettings, error) {
	normalized := runtime.RuntimeSettings{
		ComposeFilePath:    strings.TrimSpace(settings.ComposeFilePath),
		ExposedServiceName: strings.TrimSpace(settings.ExposedServiceName),
		ExposedServicePort: settings.ExposedServicePort,
	}

	switch {
	case normalized.ComposeFilePath == "":
		return runtime.RuntimeSettings{}, fmt.Errorf("repository runtime settings are incomplete: compose_file_path is required")
	case normalized.ExposedServiceName == "":
		return runtime.RuntimeSettings{}, fmt.Errorf("repository runtime settings are incomplete: exposed_service_name is required")
	case normalized.ExposedServicePort <= 0:
		return runtime.RuntimeSettings{}, fmt.Errorf("repository runtime settings are incomplete: exposed_service_port must be greater than 0")
	default:
		return normalized, nil
	}
}

func validateComposeFilePath(workspace runtime.Workspace, composeFilePath string) (string, error) {
	if strings.HasPrefix(composeFilePath, "/") {
		return "", fmt.Errorf("compose file path %q must be relative to the workspace root", composeFilePath)
	}

	resolvedPath, err := workspace.ResolvePath(composeFilePath)
	if err != nil {
		return "", fmt.Errorf("compose file path %q is invalid: %w", composeFilePath, err)
	}

	exists, err := workspace.FileExists(composeFilePath)
	if err != nil {
		return "", fmt.Errorf("check compose file %q: %w", composeFilePath, err)
	}
	if !exists {
		return "", fmt.Errorf("compose file %q does not exist in the workspace", composeFilePath)
	}

	return resolvedPath, nil
}

func parseComposeDocument(contents []byte) (composeDocument, error) {
	var document composeDocument
	if err := yaml.Unmarshal(contents, &document); err != nil {
		return composeDocument{}, fmt.Errorf("parse compose file: %w", err)
	}
	if len(document.Services) == 0 {
		return composeDocument{}, fmt.Errorf("compose file must define at least one service")
	}
	return document, nil
}

func (b *Backend) sanitizeComposeDocument(
	document composeDocument,
	request runtime.PrepareRequest,
	settings runtime.RuntimeSettings,
) (composeDocument, error) {
	if err := rejectReservedNetworkKeys(document.Networks); err != nil {
		return composeDocument{}, err
	}
	if err := rejectExternalNetworks(document.Networks); err != nil {
		return composeDocument{}, err
	}
	if _, exists := document.Services[settings.ExposedServiceName]; !exists {
		return composeDocument{}, fmt.Errorf("exposed service %q was not found in the compose file", settings.ExposedServiceName)
	}

	if document.Networks == nil {
		document.Networks = make(map[string]map[string]any)
	}
	document.Networks[toolshedPrivateNetworkKey] = map[string]any{
		"name": privateNetworkName(request.RuntimeEnvironmentID),
	}
	document.Networks[toolshedIngressNetworkKey] = map[string]any{
		"external": true,
		"name":     b.config.IngressNetwork,
	}

	for serviceName, service := range document.Services {
		stripToolshedManagedServiceFields(service)
		if err := rejectUnsafeServiceFields(serviceName, service); err != nil {
			return composeDocument{}, err
		}

		mergeServiceEnvironment(service, request.EnvironmentVariables)
		mergeServiceLabels(service, request, settings)
		mergeServiceNetworks(service, serviceName == settings.ExposedServiceName)
		injectServiceLimits(service, b.config)
	}

	return document, nil
}

func rejectReservedNetworkKeys(networks map[string]map[string]any) error {
	if networks == nil {
		return nil
	}

	if _, exists := networks[toolshedPrivateNetworkKey]; exists {
		return fmt.Errorf("compose network %q is reserved for Toolshed", toolshedPrivateNetworkKey)
	}
	if _, exists := networks[toolshedIngressNetworkKey]; exists {
		return fmt.Errorf("compose network %q is reserved for Toolshed", toolshedIngressNetworkKey)
	}
	return nil
}

func rejectExternalNetworks(networks map[string]map[string]any) error {
	for networkName, network := range networks {
		if isTruthy(network["external"]) {
			return fmt.Errorf("compose network %q cannot be external", networkName)
		}
	}
	return nil
}

func stripToolshedManagedServiceFields(service map[string]any) {
	delete(service, "ports")
	delete(service, "container_name")
}

func rejectUnsafeServiceFields(serviceName string, service map[string]any) error {
	if hasNonEmptyValue(service["network_mode"]) {
		return fmt.Errorf("service %q cannot declare network_mode", serviceName)
	}
	if isTruthy(service["privileged"]) {
		return fmt.Errorf("service %q cannot run as privileged", serviceName)
	}
	return nil
}

func mergeServiceEnvironment(service map[string]any, variables []runtime.EnvironmentVariable) {
	if len(variables) == 0 {
		return
	}

	additions := make(map[string]string, len(variables))
	for _, variable := range variables {
		additions[variable.Name] = variable.Value
	}

	switch existing := service["environment"].(type) {
	case nil:
		service["environment"] = additions
	case map[string]any:
		for name, value := range additions {
			existing[name] = value
		}
	case []any:
		service["environment"] = appendEnvList(existing, additions)
	case []string:
		list := make([]any, 0, len(existing))
		for _, item := range existing {
			list = append(list, item)
		}
		service["environment"] = appendEnvList(list, additions)
	default:
		service["environment"] = additions
	}
}

func mergeServiceLabels(
	service map[string]any,
	request runtime.PrepareRequest,
	settings runtime.RuntimeSettings,
) {
	additions := map[string]string{
		"toolshed.runtime_environment_id": strconv.FormatInt(request.RuntimeEnvironmentID, 10),
		"toolshed.repository_id":          strconv.FormatInt(request.RepositoryID, 10),
		"toolshed.pull_request_id":        strconv.FormatInt(request.PullRequestID, 10),
		"toolshed.target_commit_sha":      request.TargetCommitSHA,
		"toolshed.exposed_service_name":   settings.ExposedServiceName,
		"toolshed.exposed_service_port":   strconv.FormatInt(int64(settings.ExposedServicePort), 10),
	}

	switch existing := service["labels"].(type) {
	case nil:
		service["labels"] = additions
	case map[string]any:
		for name, value := range additions {
			existing[name] = value
		}
	case []any:
		service["labels"] = appendEnvList(existing, additions)
	case []string:
		list := make([]any, 0, len(existing))
		for _, item := range existing {
			list = append(list, item)
		}
		service["labels"] = appendEnvList(list, additions)
	default:
		service["labels"] = additions
	}
}

func mergeServiceNetworks(service map[string]any, exposedService bool) {
	requiredNetworks := []string{toolshedPrivateNetworkKey}
	if exposedService {
		requiredNetworks = append(requiredNetworks, toolshedIngressNetworkKey)
	}

	switch existing := service["networks"].(type) {
	case nil:
		networks := make([]string, 0, len(requiredNetworks))
		networks = append(networks, requiredNetworks...)
		service["networks"] = networks
	case []any:
		service["networks"] = appendMissingListEntries(existing, requiredNetworks)
	case []string:
		list := make([]any, 0, len(existing))
		for _, item := range existing {
			list = append(list, item)
		}
		service["networks"] = appendMissingListEntries(list, requiredNetworks)
	case map[string]any:
		for _, networkName := range requiredNetworks {
			if _, exists := existing[networkName]; !exists {
				existing[networkName] = map[string]any{}
			}
		}
	default:
		networks := make([]string, 0, len(requiredNetworks))
		networks = append(networks, requiredNetworks...)
		service["networks"] = networks
	}
}

func injectServiceLimits(service map[string]any, config Config) {
	service["cpus"] = strconv.FormatFloat(config.DefaultServiceCPUs, 'f', -1, 64)
	service["mem_limit"] = config.DefaultServiceMemory
	service["pids_limit"] = config.DefaultServicePIDs
}

func appendEnvList(existing []any, additions map[string]string) []any {
	filtered := make([]any, 0, len(existing)+len(additions))
	for _, item := range existing {
		key, ok := envLikeEntryKey(item)
		if ok {
			if _, replaced := additions[key]; replaced {
				continue
			}
		}
		filtered = append(filtered, item)
	}

	for _, name := range sortedMapKeys(additions) {
		filtered = append(filtered, name+"="+additions[name])
	}

	return filtered
}

func appendMissingListEntries(existing []any, required []string) []any {
	seen := make(map[string]struct{}, len(existing))
	for _, item := range existing {
		if name, ok := item.(string); ok {
			seen[name] = struct{}{}
		}
	}

	filtered := make([]any, 0, len(existing)+len(required))
	filtered = append(filtered, existing...)
	for _, name := range required {
		if _, exists := seen[name]; exists {
			continue
		}
		filtered = append(filtered, name)
	}

	return filtered
}

func hasNonEmptyValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(typed) != ""
	case []any:
		return len(typed) > 0
	case []string:
		return len(typed) > 0
	case map[string]any:
		return len(typed) > 0
	default:
		return true
	}
}

func isTruthy(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

func envLikeEntryKey(value any) (string, bool) {
	text, ok := value.(string)
	if !ok {
		return "", false
	}

	if before, _, found := strings.Cut(text, "="); found {
		return before, true
	}

	return strings.TrimSpace(text), true
}

func sortedEnvironmentVariables(
	variables []runtime.EnvironmentVariable,
) []runtime.EnvironmentVariable {
	sorted := append([]runtime.EnvironmentVariable(nil), variables...)
	slices.SortFunc(sorted, func(left, right runtime.EnvironmentVariable) int {
		return strings.Compare(left.Name, right.Name)
	})
	return sorted
}

func sortedMapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func decodeMetadata(deployment runtime.DeploymentRecord) (DeploymentMetadata, error) {
	if deployment.Backend != backendName {
		return DeploymentMetadata{}, fmt.Errorf("deployment backend %q does not match %q", deployment.Backend, backendName)
	}

	var metadata DeploymentMetadata
	if err := json.Unmarshal(deployment.MetadataJSON, &metadata); err != nil {
		return DeploymentMetadata{}, fmt.Errorf("decode deployment metadata: %w", err)
	}
	if metadata.RenderedComposeFilePath == "" {
		return DeploymentMetadata{}, fmt.Errorf("deployment metadata is missing rendered_compose_file_path")
	}
	if metadata.RenderedComposeYAML == "" {
		return DeploymentMetadata{}, fmt.Errorf("deployment metadata is missing rendered_compose_yaml")
	}
	if metadata.ProjectName == "" {
		return DeploymentMetadata{}, fmt.Errorf("deployment metadata is missing project_name")
	}
	return metadata, nil
}

func ensureRenderedComposeArtifact(
	workspace runtime.Workspace,
	metadata DeploymentMetadata,
) error {
	exists, err := workspace.FileExists(metadata.RenderedComposeFilePath)
	if err != nil {
		return fmt.Errorf("check rendered compose artifact: %w", err)
	}
	if exists {
		return nil
	}

	if err := workspace.WriteFile(
		metadata.RenderedComposeFilePath,
		[]byte(metadata.RenderedComposeYAML),
		0o644,
	); err != nil {
		return err
	}

	return nil
}

func classifyComposeError(action string, err error, defaultPermanent bool) error {
	var classified error
	message := err.Error()

	switch {
	case errors.Is(err, exec.ErrNotFound):
		classified = runtime.RetryableError(fmt.Errorf("docker is not installed or not in PATH: %w", err))
	case errors.Is(err, context.DeadlineExceeded):
		classified = runtime.RetryableError(fmt.Errorf("docker compose %s exceeded the operation request timeout: %w", action, err))
	case errors.Is(err, context.Canceled):
		classified = runtime.RetryableError(fmt.Errorf("docker compose %s was canceled before completion: %w", action, err))
	case containsMissingExternalNetworkError(message):
		classified = runtime.PermanentError(fmt.Errorf("docker compose %s could not find the configured external ingress network; create it or update runtime.ingress_network: %w", action, err))
	case containsInfrastructureComposeError(message):
		classified = runtime.RetryableError(fmt.Errorf("docker compose %s failed due to runtime infrastructure: %w", action, err))
	case defaultPermanent:
		classified = runtime.PermanentError(fmt.Errorf("docker compose %s rejected the deployment input: %w", action, err))
	default:
		classified = runtime.RetryableError(fmt.Errorf("docker compose %s failed: %w", action, err))
	}

	return classified
}

func containsMissingExternalNetworkError(message string) bool {
	lower := strings.ToLower(message)

	return strings.Contains(lower, "declared as external, but could not be found") ||
		strings.Contains(lower, "no such network")
}

func containsInfrastructureComposeError(message string) bool {
	lower := strings.ToLower(message)

	return strings.Contains(lower, "not a docker command") ||
		strings.Contains(lower, "docker daemon") ||
		strings.Contains(lower, "permission denied") ||
		strings.Contains(lower, "requires buildx plugin") ||
		strings.Contains(lower, "cannot connect") ||
		strings.Contains(lower, "connection refused") ||
		strings.Contains(lower, "dial unix")
}

func composeProjectName(runtimeEnvironmentID int64) string {
	return fmt.Sprintf("toolshed-runtime-%d", runtimeEnvironmentID)
}

func privateNetworkName(runtimeEnvironmentID int64) string {
	return fmt.Sprintf("toolshed-runtime-%d-private", runtimeEnvironmentID)
}

func marshalJSON(value any) ([]byte, error) {
	return json.Marshal(value)
}

type execCommandRunner struct{}

func (execCommandRunner) Run(ctx context.Context, dir string, args []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			if trimmed := strings.TrimSpace(string(output)); trimmed != "" {
				return output, fmt.Errorf("%w: %s", ctxErr, trimmed)
			}
			return output, ctxErr
		}
		return output, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return output, nil
}
