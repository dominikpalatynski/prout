package config

import (
	"errors"
	"fmt"
	"strings"
)

func (c *Config) Validate() error {
	var errs []string

	if c.Server.Bind == "" {
		errs = append(errs, "server.bind is required")
	}
	switch strings.ToLower(c.Storage.Backend) {
	case "", "filesystem":
	default:
		errs = append(errs, fmt.Sprintf("storage.backend: unknown value %q", c.Storage.Backend))
	}
	if strings.EqualFold(c.Storage.Backend, "filesystem") && strings.TrimSpace(c.Storage.Filesystem.WorkspaceRoot) == "" {
		errs = append(errs, "storage.filesystem.workspace_root is required (or set TOOLSHED_STORAGE_FILESYSTEM_WORKSPACE_ROOT)")
	}
	if c.Jobs.OperationRequestTimeout <= 0 {
		errs = append(errs, "jobs.operation_request_timeout must be greater than 0")
	}
	if c.DB.DSN == "" {
		errs = append(errs, "db.dsn is required (or set TOOLSHED_DB_DSN)")
	}
	if c.GitHub.AppID <= 0 {
		errs = append(errs, "github.app_id is required (or set TOOLSHED_GITHUB_APP_ID)")
	}
	if c.GitHub.PrivateKeyPath == "" {
		errs = append(errs, "github.private_key_path is required (or set TOOLSHED_GITHUB_PRIVATE_KEY_PATH)")
	}
	if c.GitHub.WebhookSecret == "" {
		errs = append(errs, "github.webhook_secret is required (or set TOOLSHED_GITHUB_WEBHOOK_SECRET)")
	}
	if c.GitHub.APIBaseURL == "" {
		errs = append(errs, "github.api_base_url is required")
	}
	if c.GitHub.APITimeout <= 0 {
		errs = append(errs, "github.api_timeout must be greater than 0")
	}
	if c.Operator.BearerToken == "" {
		errs = append(errs, "operator.bearer_token is required (or set TOOLSHED_OPERATOR_BEARER_TOKEN)")
	}

	switch strings.ToLower(c.Log.Level) {
	case "", "debug", "info", "warn", "error":
	default:
		errs = append(errs, fmt.Sprintf("log.level: unknown value %q", c.Log.Level))
	}
	switch strings.ToLower(c.Log.Format) {
	case "", "json", "text":
	default:
		errs = append(errs, fmt.Sprintf("log.format: unknown value %q", c.Log.Format))
	}

	if len(errs) > 0 {
		return errors.New("invalid config: " + strings.Join(errs, "; "))
	}
	return nil
}
