package dockercompose

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	runtimebackend "github.com/dominikpalatynski/toolshed/internal/runtime"
	"github.com/dominikpalatynski/toolshed/internal/workspaces"
)

func TestPrepareRejectsAbsoluteComposePath(t *testing.T) {
	t.Parallel()

	workspace := mustCreateWorkspace(t, map[string]string{
		"compose.yml": "services:\n  app:\n    image: nginx:latest\n",
	})
	backend := NewBackendWithRunner(testBackendConfig(), &fakeRunner{})

	_, err := backend.Prepare(context.Background(), runtimebackend.PrepareRequest{
		RuntimeEnvironmentID: 7,
		RepositoryID:         11,
		PullRequestID:        13,
		TargetCommitSHA:      "abc123",
		Workspace:            workspace,
		RuntimeSettings: runtimebackend.RuntimeSettings{
			ComposeFilePath:    "/compose.yml",
			ExposedServiceName: "app",
			ExposedServicePort: 8080,
		},
	})
	if !runtimebackend.IsPermanentError(err) {
		t.Fatalf("Prepare() permanent = false, want true (err=%v)", err)
	}
}

func TestPrepareStripsHostPublishedPortsAndContainerNames(t *testing.T) {
	t.Parallel()

	workspace := mustCreateWorkspace(t, map[string]string{
		"compose.yml": "services:\n  app:\n    image: nginx:latest\n    ports:\n      - \"8080:80\"\n    container_name: custom-app\n  redis:\n    image: redis:7-alpine\n    container_name: custom-redis\n",
	})
	runner := &fakeRunner{}
	backend := NewBackendWithRunner(testBackendConfig(), runner)

	_, err := backend.Prepare(context.Background(), runtimebackend.PrepareRequest{
		RuntimeEnvironmentID: 7,
		RepositoryID:         11,
		PullRequestID:        13,
		TargetCommitSHA:      "abc123",
		Workspace:            workspace,
		RuntimeSettings: runtimebackend.RuntimeSettings{
			ComposeFilePath:    "compose.yml",
			ExposedServiceName: "app",
			ExposedServicePort: 8080,
		},
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("len(runner.calls) = %d, want 1", len(runner.calls))
	}
	if got := runner.calls[0].args; len(got) != 6 || got[5] != "config" {
		t.Fatalf("runner calls = %#v, want compose config validation", runner.calls)
	}

	renderedPath, err := workspace.ResolvePath(".toolshed.docker-compose.rendered.yml")
	if err != nil {
		t.Fatalf("ResolvePath() error = %v", err)
	}
	renderedCompose, err := os.ReadFile(renderedPath)
	if err != nil {
		t.Fatalf("os.ReadFile(rendered compose) error = %v", err)
	}
	if contains(string(renderedCompose), "ports:") {
		t.Fatalf("rendered compose still contains ports: %s", renderedCompose)
	}
	if contains(string(renderedCompose), "container_name:") {
		t.Fatalf("rendered compose still contains container_name: %s", renderedCompose)
	}
}

func TestPrepareWritesRenderedArtifactAndRunsComposeConfig(t *testing.T) {
	t.Parallel()

	workspace := mustCreateWorkspace(t, map[string]string{
		"compose.yml": "services:\n  app:\n    image: nginx:latest\n  worker:\n    image: busybox:latest\n",
	})
	runner := &fakeRunner{}
	backend := NewBackendWithRunner(testBackendConfig(), runner)

	deployment, err := backend.Prepare(context.Background(), runtimebackend.PrepareRequest{
		RuntimeEnvironmentID: 7,
		RepositoryID:         11,
		PullRequestID:        13,
		TargetCommitSHA:      "abc123",
		Workspace:            workspace,
		RuntimeSettings: runtimebackend.RuntimeSettings{
			ComposeFilePath:    "compose.yml",
			ExposedServiceName: "app",
			ExposedServicePort: 8080,
		},
		EnvironmentVariables: []runtimebackend.EnvironmentVariable{
			{Name: "ALPHA", Value: "1"},
		},
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}

	if deployment.Backend != backendName {
		t.Fatalf("deployment.Backend = %q, want %q", deployment.Backend, backendName)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("len(runner.calls) = %d, want 1", len(runner.calls))
	}
	if got := runner.calls[0].args; len(got) != 6 || got[0] != "compose" || got[5] != "config" {
		t.Fatalf("runner args = %#v, want docker compose config invocation", got)
	}

	renderedPath, err := workspace.ResolvePath(".toolshed.docker-compose.rendered.yml")
	if err != nil {
		t.Fatalf("ResolvePath() error = %v", err)
	}
	renderedCompose, err := os.ReadFile(renderedPath)
	if err != nil {
		t.Fatalf("os.ReadFile(rendered compose) error = %v", err)
	}
	if !contains(string(renderedCompose), "toolshed_private") {
		t.Fatalf("rendered compose missing toolshed_private network: %s", renderedCompose)
	}
	if !contains(string(renderedCompose), "mem_limit: 512m") {
		t.Fatalf("rendered compose missing mem_limit injection: %s", renderedCompose)
	}

	exists, err := backend.PreparedArtifactsExist(context.Background(), runtimebackend.PreparedArtifactsRequest{
		Workspace:  workspace,
		Deployment: deployment,
	})
	if err != nil {
		t.Fatalf("PreparedArtifactsExist() error = %v", err)
	}
	if !exists {
		t.Fatalf("PreparedArtifactsExist() = false, want true")
	}
}

func TestPrepareWritesRenderedComposeNextToSourceComposeFile(t *testing.T) {
	t.Parallel()

	workspace := mustCreateWorkspace(t, map[string]string{
		"deploy/compose.yml": "services:\n  app:\n    build:\n      context: .\n    volumes:\n      - .:/app\n",
	})
	runner := &fakeRunner{}
	backend := NewBackendWithRunner(testBackendConfig(), runner)

	_, err := backend.Prepare(context.Background(), runtimebackend.PrepareRequest{
		RuntimeEnvironmentID: 7,
		RepositoryID:         11,
		PullRequestID:        13,
		TargetCommitSHA:      "abc123",
		Workspace:            workspace,
		RuntimeSettings: runtimebackend.RuntimeSettings{
			ComposeFilePath:    "deploy/compose.yml",
			ExposedServiceName: "app",
			ExposedServicePort: 8080,
		},
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("len(runner.calls) = %d, want 1", len(runner.calls))
	}

	wantRenderedPath := "deploy/.toolshed.docker-compose.rendered.yml"
	if got := runner.calls[0].args[4]; got != wantRenderedPath {
		t.Fatalf("runner rendered compose path = %q, want %q", got, wantRenderedPath)
	}

	renderedPath, err := workspace.ResolvePath(wantRenderedPath)
	if err != nil {
		t.Fatalf("ResolvePath() error = %v", err)
	}
	renderedCompose, err := os.ReadFile(renderedPath)
	if err != nil {
		t.Fatalf("os.ReadFile(rendered compose) error = %v", err)
	}
	if !contains(string(renderedCompose), "context: .") {
		t.Fatalf("rendered compose changed relative build context unexpectedly: %s", renderedCompose)
	}
	if !contains(string(renderedCompose), "- .:/app") {
		t.Fatalf("rendered compose changed relative bind mount unexpectedly: %s", renderedCompose)
	}
}

func TestPrepareTreatsComposeConfigFailureAsPermanent(t *testing.T) {
	t.Parallel()

	workspace := mustCreateWorkspace(t, map[string]string{
		"compose.yml": "services:\n  app:\n    image: nginx:latest\n",
	})
	runner := &fakeRunner{
		results: []fakeRunResult{
			{err: errors.New("exit status 1: services.app depends on undefined service")},
		},
	}
	backend := NewBackendWithRunner(testBackendConfig(), runner)

	_, err := backend.Prepare(context.Background(), runtimebackend.PrepareRequest{
		RuntimeEnvironmentID: 7,
		RepositoryID:         11,
		PullRequestID:        13,
		TargetCommitSHA:      "abc123",
		Workspace:            workspace,
		RuntimeSettings: runtimebackend.RuntimeSettings{
			ComposeFilePath:    "compose.yml",
			ExposedServiceName: "app",
			ExposedServicePort: 8080,
		},
	})
	if !runtimebackend.IsPermanentError(err) {
		t.Fatalf("Prepare() permanent = false, want true (err=%v)", err)
	}
}

func TestClassifyComposeErrorTreatsDeadlineExceededAsRetryable(t *testing.T) {
	t.Parallel()

	err := classifyComposeError("up", context.DeadlineExceeded, false)
	if runtimebackend.IsPermanentError(err) {
		t.Fatalf("classifyComposeError() permanent = true, want false (err=%v)", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("classifyComposeError() error = %v, want wrapped context deadline exceeded", err)
	}
	if !strings.Contains(err.Error(), "exceeded the operation request timeout") {
		t.Fatalf("classifyComposeError() error = %q, want timeout summary", err.Error())
	}
}

func TestClassifyComposeErrorTreatsMissingExternalNetworkAsPermanent(t *testing.T) {
	t.Parallel()

	err := classifyComposeError("up", errors.New("exit status 1: network toolshed-traefik declared as external, but could not be found"), false)
	if !runtimebackend.IsPermanentError(err) {
		t.Fatalf("classifyComposeError() permanent = false, want true (err=%v)", err)
	}
	if !strings.Contains(err.Error(), "configured external ingress network") {
		t.Fatalf("classifyComposeError() error = %q, want ingress network summary", err.Error())
	}
}

func TestContainsInfrastructureComposeErrorRecognizesMissingBuildx(t *testing.T) {
	t.Parallel()

	if !containsInfrastructureComposeError("Docker Compose requires buildx plugin to be installed") {
		t.Fatalf("containsInfrastructureComposeError() = false, want true for missing buildx plugin")
	}
}

type fakeRunner struct {
	results []fakeRunResult
	calls   []fakeRunCall
}

type fakeRunResult struct {
	output []byte
	err    error
}

type fakeRunCall struct {
	dir  string
	args []string
}

func (r *fakeRunner) Run(_ context.Context, dir string, args []string) ([]byte, error) {
	r.calls = append(r.calls, fakeRunCall{
		dir:  dir,
		args: append([]string(nil), args...),
	})
	if len(r.results) == 0 {
		return nil, nil
	}

	result := r.results[0]
	r.results = r.results[1:]
	return result.output, result.err
}

func testBackendConfig() Config {
	return Config{
		IngressNetwork:       "toolshed-traefik",
		DefaultServiceCPUs:   1,
		DefaultServiceMemory: "512m",
		DefaultServicePIDs:   256,
	}
}

func mustCreateWorkspace(t *testing.T, files map[string]string) runtimebackend.Workspace {
	t.Helper()

	root := t.TempDir()
	workspaceRoot := filepath.Join(root, "runtime-environments", "7")
	if err := os.MkdirAll(workspaceRoot, 0o755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	for name, contents := range files {
		targetPath := filepath.Join(workspaceRoot, name)
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			t.Fatalf("os.MkdirAll(parent) error = %v", err)
		}
		if err := os.WriteFile(targetPath, []byte(contents), 0o644); err != nil {
			t.Fatalf("os.WriteFile(%q) error = %v", name, err)
		}
	}

	manager := workspaces.NewFilesystemManager(root)
	workspace, err := manager.OpenWorkspace("runtime-environments/7")
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	return workspace
}

func contains(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}
