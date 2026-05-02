package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadUsesDefaultTimeoutsWhenOmitted(t *testing.T) {
	t.Setenv("TOOLSHED_GITHUB_API_TIMEOUT", "")
	t.Setenv("TOOLSHED_JOBS_OPERATION_REQUEST_TIMEOUT", "")

	cfg := mustLoadConfig(t, `
server:
  bind: ":8080"
storage:
  backend: filesystem
  filesystem:
    workspace_root: /tmp/toolshed/workspaces
db:
  dsn: "postgres://toolshed:toolshed@localhost:5433/toolshed?sslmode=disable"
log:
  level: info
  format: json
github:
  app_id: 123456
  private_key_path: /tmp/github-app.pem
  webhook_secret: replace-me
  api_base_url: https://api.github.com
operator:
  bearer_token: replace-me
`)

	if cfg.GitHub.APITimeout != DefaultGitHubAPITimeout {
		t.Fatalf("cfg.GitHub.APITimeout = %s, want %s", cfg.GitHub.APITimeout, DefaultGitHubAPITimeout)
	}
	if cfg.Jobs.OperationRequestTimeout != DefaultOperationRequestJobTimeout {
		t.Fatalf("cfg.Jobs.OperationRequestTimeout = %s, want %s", cfg.Jobs.OperationRequestTimeout, DefaultOperationRequestJobTimeout)
	}
}

func TestLoadOverridesConfiguredTimeouts(t *testing.T) {
	t.Setenv("TOOLSHED_GITHUB_API_TIMEOUT", "")
	t.Setenv("TOOLSHED_JOBS_OPERATION_REQUEST_TIMEOUT", "")

	cfg := mustLoadConfig(t, `
server:
  bind: ":8080"
storage:
  backend: filesystem
  filesystem:
    workspace_root: /tmp/toolshed/workspaces
jobs:
  operation_request_timeout: 15m
db:
  dsn: "postgres://toolshed:toolshed@localhost:5433/toolshed?sslmode=disable"
log:
  level: info
  format: json
github:
  app_id: 123456
  private_key_path: /tmp/github-app.pem
  webhook_secret: replace-me
  api_base_url: https://api.github.com
  api_timeout: 30s
operator:
  bearer_token: replace-me
`)

	if cfg.GitHub.APITimeout != 30*time.Second {
		t.Fatalf("cfg.GitHub.APITimeout = %s, want %s", cfg.GitHub.APITimeout, 30*time.Second)
	}
	if cfg.Jobs.OperationRequestTimeout != 15*time.Minute {
		t.Fatalf("cfg.Jobs.OperationRequestTimeout = %s, want %s", cfg.Jobs.OperationRequestTimeout, 15*time.Minute)
	}
}

func mustLoadConfig(t *testing.T, contents string) *Config {
	t.Helper()

	path := filepath.Join(t.TempDir(), "server.yml")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	return cfg
}
