package workspace

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/dominikpalatynski/toolshed/internal/config"
	"go.yaml.in/yaml/v3"
)

const (
	composeUpCommandKey   = "compose_up_command"
	composeDownCommandKey = "compose_down_command"
)

type composeDocument struct {
	Services map[string]map[string]any `yaml:"services"`
	Networks map[string]map[string]any `yaml:"networks,omitempty"`
	Extras   map[string]any            `yaml:",inline"`
}

type WorkspaceHandler struct {
	cfg              *config.Config
	privateKey       *rsa.PrivateKey
	httpClient       *http.Client
	streamHTTPClient *http.Client
}

type APIError struct {
	Method     string
	URL        string
	StatusCode int
	Message    string
}

type WorkspaceLocationBuilder struct {
	FullName string
	PRNumber int
	SHA      string
}

func (e *APIError) Error() string {
	request := "github request failed"
	switch {
	case e.Method != "" && e.URL != "":
		request = fmt.Sprintf("github request %s %s failed", e.Method, e.URL)
	case e.Method != "":
		request = fmt.Sprintf("github request %s failed", e.Method)
	case e.URL != "":
		request = fmt.Sprintf("github request %s failed", e.URL)
	}

	if e.StatusCode <= 0 {
		if e.Message == "" {
			return request
		}
		return fmt.Sprintf("%s: %s", request, e.Message)
	}
	if e.Message == "" {
		return fmt.Sprintf("%s with status %d", request, e.StatusCode)
	}
	return fmt.Sprintf("%s with status %d: %s", request, e.StatusCode, e.Message)
}

func NewWorkspaceHandler(cfg *config.Config) (*WorkspaceHandler, error) {
	privateKeyData, err := os.ReadFile(cfg.GitHub.PrivateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("read github app private key: %w", err)
	}

	privateKey, err := parsePrivateKey(privateKeyData)
	if err != nil {
		return nil, fmt.Errorf("parse github app private key: %w", err)
	}

	return &WorkspaceHandler{
		cfg:        cfg,
		privateKey: privateKey,
	}, nil
}

func (w *WorkspaceHandler) HandleCreateWorkspace(location WorkspaceLocationBuilder) error {
	ctx := context.Background()
	owner := w.cfg.GitHub.Repository.Owner
	name := w.cfg.GitHub.Repository.Name
	sha := location.SHA

	slog.Info("Creating workspace from GitHub repository", "owner", owner, "repo", name, "sha", sha)
	w.SendPRComment(location, "🚀 Starting workspace setup...")
	appJWT, err := w.appJWT(time.Now().UTC())

	if err != nil {
		w.SendPRComment(location, "❌ Failed to create workspace: error generating GitHub App JWT")
		return fmt.Errorf("create github app jwt: %w", err)
	}

	installationId, err := w.getInstallationID(ctx, appJWT, owner, name)

	if err != nil {
		w.SendPRComment(location, "❌ Failed to create workspace: error getting GitHub installation ID")
		slog.Error("get github installation id", "owner", owner, "repo", name, "error", err)
		return fmt.Errorf("get github installation id: %w", err)
	}

	slog.Info("Resolved GitHub installation", "owner", owner, "repo", name, "installation_id", installationId)

	domain, err := w.prepareDomainName(location)
	if err != nil {
		w.SendPRComment(location, "❌ Failed to create workspace: error preparing workspace domain name")
		slog.Error("prepare workspace domain name failed", "error", err)
		return fmt.Errorf("prepare workspace domain name: %w", err)
	}

	body, err := w.downloadTarball(ctx, owner, name, installationId, sha)

	if err != nil {
		w.SendPRComment(location, "❌ Failed to create workspace: error downloading tarball")
		return fmt.Errorf("download tarball: %w", err)
	}
	defer body.Close()

	workspacePath, err := w.createWorkspaceFolder(location)

	if err != nil {
		w.SendPRComment(location, "❌ Failed to create workspace: error creating workspace folder")
		return err
	}

	if err := w.extractTarball(workspacePath, body); err != nil {
		w.SendPRComment(location, "❌ Failed to create workspace: error extracting tarball")
		return err
	}

	if err := w.prepareComposeFile(workspacePath, domain); err != nil {
		w.SendPRComment(location, "❌ Failed to create workspace: error preparing compose file")
		return err
	}
	w.SendPRComment(location, "🚀 Workspace files prepared, starting Docker containers...")
	if err := w.runDockerCommand(workspacePath, composeUpCommandKey); err != nil {
		w.SendPRComment(location, "❌ Failed to create workspace: error running docker command")
		return err
	}

	w.SendPRComment(location, "Workspace is up and running at http://"+domain)
	slog.Info("Workspace created successfully", "path", workspacePath)
	return nil
}

func (w *WorkspaceHandler) HandleDeleteWorkspace(locationBuilder WorkspaceLocationBuilder) error {
	location := workspaceFolderPath(locationBuilder)

	slog.Info("Deleting workspace", "location", location)
	slog.Info("Running docker compose down to stop workspace containers", "location", location)
	if err := w.runDockerCommand(location, composeDownCommandKey); err != nil {
		return fmt.Errorf("run docker compose down: %w", err)
	}

	slog.Info("Docker compose down completed, removing workspace folder", "location", location)

	if err := os.RemoveAll(location); err != nil {
		return fmt.Errorf("remove workspace folder: %w", err)
	}

	slog.Info("Workspace deleted successfully", "location", location)
	w.SendPRComment(locationBuilder, "🚀 Workspace deleted successfully")
	return nil
}

func (w *WorkspaceHandler) downloadTarball(ctx context.Context, owner, name string, installationID int64, sha string) (io.ReadCloser, error) {
	if strings.TrimSpace(owner) == "" || strings.TrimSpace(name) == "" {
		slog.Error("repository owner and name are required")
		return nil, fmt.Errorf("repository owner and name are required")
	}
	if installationID <= 0 {
		slog.Error("github installation id must be positive")
		return nil, fmt.Errorf("github installation id must be positive")
	}
	if strings.TrimSpace(sha) == "" {
		slog.Error("pull request head sha is required")
		return nil, fmt.Errorf("pull request head sha is required")
	}

	appJWT, err := w.appJWT(time.Now().UTC())
	if err != nil {
		slog.Error("create github app jwt", "error", err)
		return nil, fmt.Errorf("create github app jwt: %w", err)
	}

	installationToken, err := w.createInstallationToken(ctx, appJWT, installationID)
	if err != nil {
		return nil, err
	}

	req, err := w.newRequest(ctx, http.MethodGet, w.endpointURL(
		fmt.Sprintf("/repos/%s/%s/tarball/%s", url.PathEscape(owner), url.PathEscape(name), url.PathEscape(sha)),
	), http.NoBody, installationToken)
	if err != nil {
		return nil, err
	}

	return w.sendStreamRequest(req)
}

func (w *WorkspaceHandler) newRequest(ctx context.Context, method, url string, body io.Reader, token string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "toolshed")
	return req, nil
}

func (w *WorkspaceHandler) endpointURL(relativePath string) string {
	cleanPath := path.Clean("/" + strings.TrimSpace(relativePath))
	return strings.TrimRight(w.cfg.GitHub.APIBaseURL, "/") + cleanPath
}

func (w *WorkspaceHandler) createInstallationToken(ctx context.Context, appJWT string, installationID int64) (string, error) {
	req, err := w.newRequest(ctx, http.MethodPost, w.endpointURL(
		fmt.Sprintf("/app/installations/%d/access_tokens", installationID),
	), bytes.NewReader([]byte("{}")), appJWT)
	if err != nil {
		return "", err
	}

	var response struct {
		Token string `json:"token"`
	}
	if err := w.sendRequest(req, &response); err != nil {
		return "", err
	}

	if strings.TrimSpace(response.Token) == "" {
		return "", errors.New("github access token response missing token")
	}
	return response.Token, nil
}

func (w *WorkspaceHandler) sendRequest(req *http.Request, v any) error {
	httpClient := w.httpClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: w.cfg.GitHub.APITimeout,
		}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		slog.Error("GitHub API request failed", "method", req.Method, "url", req.URL.Redacted(), "error", err)
		return fmt.Errorf("github request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if readErr != nil {
			return fmt.Errorf("read github error response: %w", readErr)
		}
		return logAndWrapGitHubAPIError(req, resp.StatusCode, body)
	}

	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("decode github response: %w", err)
	}
	return nil
}

func (w *WorkspaceHandler) appJWT(now time.Time) (string, error) {
	headerJSON, err := json.Marshal(map[string]string{
		"alg": "RS256",
		"typ": "JWT",
	})
	if err != nil {
		return "", err
	}

	claimsJSON, err := json.Marshal(map[string]any{
		"iat": now.Add(-60 * time.Second).Unix(),
		"exp": now.Add(9 * time.Minute).Unix(),
		"iss": w.cfg.GitHub.AppID,
	})
	if err != nil {
		return "", err
	}

	header := base64.RawURLEncoding.EncodeToString(headerJSON)
	claims := base64.RawURLEncoding.EncodeToString(claimsJSON)
	signingInput := header + "." + claims

	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, w.privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}

	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (w *WorkspaceHandler) sendStreamRequest(req *http.Request) (io.ReadCloser, error) {
	httpClient := w.streamHTTPClient
	if httpClient == nil {
		httpClient = w.httpClient
	}
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: w.cfg.GitHub.APIStreamTimeout,
		}
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		slog.Error("GitHub API request failed", "method", req.Method, "url", req.URL.Redacted(), "error", err)
		return nil, fmt.Errorf("github request failed: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if readErr != nil {
			return nil, fmt.Errorf("read github error response: %w", readErr)
		}
		return nil, logAndWrapGitHubAPIError(req, resp.StatusCode, body)
	}

	return resp.Body, nil
}

func (w *WorkspaceHandler) createWorkspaceFolder(locationBuilder WorkspaceLocationBuilder) (string, error) {
	workspacePath := workspaceFolderPath(locationBuilder)
	if err := os.MkdirAll(workspacePath, 0o755); err != nil {
		return "", fmt.Errorf("create workspace folder: %w", err)
	}

	return workspacePath, nil
}

func workspaceFolderPath(locationBuilder WorkspaceLocationBuilder) string {
	return filepath.Join(
		".",
		"toolshed",
		"workspaces",
		fmt.Sprintf("%s-%d-%s", locationBuilder.FullName, locationBuilder.PRNumber, locationBuilder.SHA),
	)
}

func (w *WorkspaceHandler) extractTarball(stagingPath string, body io.Reader) error {
	gzipReader, err := gzip.NewReader(body)
	if err != nil {
		return fmt.Errorf("create gzip reader: %w", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("read tar header: %w", err)
		}

		relativePath, skip, err := stripTarballWrapper(header.Name)
		if err != nil {
			return err
		}
		if skip {
			continue
		}

		targetPath, err := secureJoin(stagingPath, relativePath)
		if err != nil {
			return err
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return fmt.Errorf("create directory %q: %w", relativePath, err)
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return fmt.Errorf("create file parent %q: %w", relativePath, err)
			}

			file, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, header.FileInfo().Mode().Perm())
			if err != nil {
				return fmt.Errorf("open file %q: %w", relativePath, err)
			}

			if _, err := io.Copy(file, tarReader); err != nil {
				_ = file.Close()
				return fmt.Errorf("write file %q: %w", relativePath, err)
			}
			if err := file.Close(); err != nil {
				return fmt.Errorf("close file %q: %w", relativePath, err)
			}
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return fmt.Errorf("create symlink parent %q: %w", relativePath, err)
			}
			if err := os.Symlink(header.Linkname, targetPath); err != nil {
				return fmt.Errorf("create symlink %q: %w", relativePath, err)
			}
		default:
			// Ignore unsupported entry types in phase 3B.
		}
	}
}

func stripTarballWrapper(name string) (string, bool, error) {
	clean := path.Clean(strings.TrimSpace(name))
	if clean == "." || clean == "/" {
		return "", true, nil
	}
	if strings.HasPrefix(clean, "/") || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", false, fmt.Errorf("tarball entry %q escapes workspace root", name)
	}

	parts := strings.Split(clean, "/")
	if len(parts) == 1 {
		return "", true, nil
	}

	relative := path.Join(parts[1:]...)
	if relative == "." || relative == "" {
		return "", true, nil
	}
	if relative == ".." || strings.HasPrefix(relative, "../") {
		return "", false, fmt.Errorf("tarball entry %q escapes workspace root", name)
	}

	return relative, false, nil
}

func secureJoin(root, relative string) (string, error) {
	trimmed := strings.TrimSpace(relative)
	if trimmed == "" {
		return "", fmt.Errorf("workspace path is required")
	}

	clean := filepath.Clean(trimmed)
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid workspace path %q", relative)
	}

	fullPath := filepath.Join(root, clean)
	relToRoot, err := filepath.Rel(root, fullPath)
	if err != nil {
		return "", fmt.Errorf("resolve workspace path: %w", err)
	}
	if relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("workspace path %q escapes workspace root", relative)
	}

	return fullPath, nil
}

func (w *WorkspaceHandler) getInstallationID(ctx context.Context, appJWT, owner, name string) (int64, error) {
	req, err := w.newRequest(ctx, http.MethodGet, w.endpointURL(
		fmt.Sprintf("/repos/%s/%s/installation", url.PathEscape(owner), url.PathEscape(name)),
	), bytes.NewReader(nil), appJWT)
	if err != nil {
		slog.Error("create github installation request failed", "error", err)
		return 0, err
	}

	var response struct {
		ID int64 `json:"id"`
	}
	if err := w.sendRequest(req, &response); err != nil {
		slog.Error("get github installation id failed", "error", err)
		return 0, err
	}
	if response.ID <= 0 {
		slog.Error("github installation response missing id")
		return 0, errors.New("github installation response missing id")
	}
	return response.ID, nil
}

func logAndWrapGitHubAPIError(req *http.Request, statusCode int, body []byte) error {
	message := strings.TrimSpace(string(body))
	if message == "" {
		message = http.StatusText(statusCode)
	}

	slog.Error("GitHub API returned non-success status", "method", req.Method, "url", req.URL.Redacted(), "status_code", statusCode, "response_body", message)

	return &APIError{
		Method:     req.Method,
		URL:        req.URL.Redacted(),
		StatusCode: statusCode,
		Message:    message,
	}
}

func parsePrivateKey(privateKeyPEM []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(privateKeyPEM)
	if block == nil {
		return nil, errors.New("missing PEM block")
	}

	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}

	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("private key is not RSA")
	}

	return rsaKey, nil
}

func (w *WorkspaceHandler) runDockerCommand(workspacePath, composeCommandKey string) error {

	var cmd *exec.Cmd
	switch composeCommandKey {
	case composeUpCommandKey:
		cmd = exec.Command("docker", "compose", "-f", "compose.yml", "up", "-d")
	case composeDownCommandKey:
		cmd = exec.Command("docker", "compose", "-f", "compose.yml", "down")
	default:
		return fmt.Errorf("unknown compose command key: %s", composeCommandKey)
	}

	cmd.Dir = workspacePath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run docker compose in %s: %w", workspacePath, err)
	}
	return nil
}

func (w *WorkspaceHandler) prepareComposeFile(workspacePath, domain string) error {

	composePath := filepath.Join(workspacePath, w.cfg.GitHub.Repository.BuildSettings.DockerComposeFilePath)
	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		slog.Info("No docker compose file found in repository, skipping preparation")
		return nil
	}

	exposedServiceName := strings.TrimSpace(w.cfg.GitHub.Repository.BuildSettings.ExposedServiceName)
	if exposedServiceName == "" {
		return fmt.Errorf("exposed_service_name is required")
	}
	exposedPort := w.cfg.GitHub.Repository.BuildSettings.ExposedPort
	if exposedPort <= 0 {
		return fmt.Errorf("exposed_port must be greater than 0")
	}

	content, err := os.ReadFile(composePath)
	if err != nil {
		return fmt.Errorf("read docker compose file: %w", err)
	}

	var document composeDocument
	if err := yaml.Unmarshal(content, &document); err != nil {
		return fmt.Errorf("parse docker compose file: %w", err)
	}
	if len(document.Services) == 0 {
		return fmt.Errorf("docker compose file must define at least one service")
	}

	service, exists := document.Services[exposedServiceName]
	if !exists {
		return fmt.Errorf("exposed service %q not found in compose file", exposedServiceName)
	}

	if document.Networks == nil {
		document.Networks = make(map[string]map[string]any)
	}
	document.Networks["proxy"] = map[string]any{"external": true}

	delete(service, "ports")
	attachProxyNetwork(service)
	mergeTraefikLabels(service, exposedServiceName, domain, w.cfg.Environment.Name, exposedPort)
	loadEnvironmentVariables(service, w.cfg.GitHub.Repository.BuildSettings.EnvironmentVariables)

	output, err := yaml.Marshal(document)
	if err != nil {
		return fmt.Errorf("marshal docker compose file: %w", err)
	}

	if err := os.WriteFile(composePath, output, 0o644); err != nil {
		return fmt.Errorf("write docker compose file: %w", err)
	}

	slog.Info("Prepared docker compose file with Traefik wiring", "path", composePath, "service", exposedServiceName, "port", exposedPort)
	return nil
}

func attachProxyNetwork(service map[string]any) {
	const proxyNetwork = "proxy"

	switch existing := service["networks"].(type) {
	case nil:
		service["networks"] = []any{proxyNetwork}
	case []any:
		for _, item := range existing {
			if name, ok := item.(string); ok && name == proxyNetwork {
				return
			}
		}
		service["networks"] = append(existing, proxyNetwork)
	case []string:
		for _, name := range existing {
			if name == proxyNetwork {
				return
			}
		}
		list := make([]any, 0, len(existing)+1)
		for _, name := range existing {
			list = append(list, name)
		}
		service["networks"] = append(list, proxyNetwork)
	case map[string]any:
		if _, ok := existing[proxyNetwork]; !ok {
			existing[proxyNetwork] = map[string]any{}
		}
	default:
		service["networks"] = []any{proxyNetwork}
	}
}

func loadEnvironmentVariables(service map[string]any, envVars map[string]string) {
	if len(envVars) == 0 {
		return
	}
	switch existing := service["environment"].(type) {
	case nil:
		list := make([]any, 0, len(envVars))
		for key, value := range envVars {
			list = append(list, fmt.Sprintf("%s=%s", key, value))
		}
		service["environment"] = list
	case []any:
		existingMap := make(map[string]struct{}, len(existing))
		for _, item := range existing {
			if s, ok := item.(string); ok {
				key, _, _ := strings.Cut(s, "=")
				existingMap[key] = struct{}{}
			}
		}
		for key, value := range envVars {
			if _, exists := existingMap[key]; exists {
				continue
			}
			existing = append(existing, fmt.Sprintf("%s=%s", key, value))
			existingMap[key] = struct{}{}
		}
		service["environment"] = existing
	case []string:
		existingMap := make(map[string]struct{}, len(existing))
		for _, item := range existing {
			key, _, _ := strings.Cut(item, "=")
			existingMap[key] = struct{}{}
		}
		list := make([]any, 0, len(existing)+len(envVars))
		for _, item := range existing {
			list = append(list, item)
		}
		for key, value := range envVars {
			if _, exists := existingMap[key]; exists {
				continue
			}
			list = append(list, fmt.Sprintf("%s=%s", key, value))
			existingMap[key] = struct{}{}
		}
		service["environment"] = list
	case map[string]any:
		for key, value := range envVars {
			if _, exists := existing[key]; exists {
				continue
			}
			existing[key] = value
		}
	default:
		list := make([]any, 0, len(envVars))
		for key, value := range envVars {
			list = append(list, fmt.Sprintf("%s=%s", key, value))
		}
		service["environment"] = list
	}
}

func mergeTraefikLabels(service map[string]any, serviceName, domain, environment string, port int) {
	additions := generateTraefikLabels(serviceName, domain, environment, port)

	switch existing := service["labels"].(type) {
	case nil:
		list := make([]any, 0, len(additions))
		for _, label := range additions {
			list = append(list, label)
		}
		service["labels"] = list
	case []any:
		service["labels"] = appendLabelList(existing, additions)
	case []string:
		list := make([]any, 0, len(existing)+len(additions))
		for _, item := range existing {
			list = append(list, item)
		}
		service["labels"] = appendLabelList(list, additions)
	case map[string]any:
		for _, label := range additions {
			key, value, _ := strings.Cut(label, "=")
			existing[key] = value
		}
	default:
		list := make([]any, 0, len(additions))
		for _, label := range additions {
			list = append(list, label)
		}
		service["labels"] = list
	}
}

func generateTraefikLabels(serviceName, domain, environment string, port int) []string {
	var webEntrypoint string
	if environment == config.ProdEnvironment {
		webEntrypoint = "websecure"
	} else {
		webEntrypoint = "web"
	}

	additions := []string{
		"traefik.enable=true",
		fmt.Sprintf("traefik.http.routers.%s.rule=Host(`%s`)", serviceName, domain),
		fmt.Sprintf("traefik.http.routers.%s.entrypoints=%s", serviceName, webEntrypoint),
		fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port=%d", serviceName, port),
	}

	if environment == config.ProdEnvironment {
		additions = append(additions, "traefik.http.routers.my-app.tls=true")
	}
	return additions
}

func appendLabelList(existing []any, additions []string) []any {
	seen := make(map[string]struct{}, len(existing))
	for _, item := range existing {
		if s, ok := item.(string); ok {
			key, _, _ := strings.Cut(s, "=")
			seen[key] = struct{}{}
		}
	}
	for _, label := range additions {
		key, _, _ := strings.Cut(label, "=")
		if _, exists := seen[key]; exists {
			continue
		}
		existing = append(existing, label)
		seen[key] = struct{}{}
	}
	return existing
}

func (w *WorkspaceHandler) prepareDomainName(locationBuilder WorkspaceLocationBuilder) (string, error) {
	domain := strings.TrimSpace(w.cfg.GitHub.Repository.WildcardDomain)
	if domain == "" {
		return "", fmt.Errorf("invalid wildcard domain %q: empty value", w.cfg.GitHub.Repository.WildcardDomain)
	}

	// Support both ".app.localhost" and legacy "*.app.localhost" config values.
	domain = strings.TrimPrefix(domain, "*.")
	domain = strings.TrimPrefix(domain, "*")
	if !strings.HasPrefix(domain, ".") {
		domain = "." + domain
	}

	sanitizedFullName := strings.ReplaceAll(locationBuilder.FullName, "/", "-")
	sanitizedPRNumber := fmt.Sprintf("%d", locationBuilder.PRNumber)
	sanitizedSHA := locationBuilder.SHA
	if len(sanitizedSHA) > 7 {
		sanitizedSHA = sanitizedSHA[:7]
	}

	return fmt.Sprintf("%s-%s-%s%s", sanitizedFullName, sanitizedPRNumber, sanitizedSHA, domain), nil
}
