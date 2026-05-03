package config

import (
	"fmt"
	"os"
	"time"

	"github.com/caarlos0/env/v11"
	"gopkg.in/yaml.v3"
)

const (
	DefaultGitHubAPITimeout           = 10 * time.Second
	DefaultOperationRequestJobTimeout = 30 * time.Minute
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Storage  StorageConfig  `yaml:"storage"`
	Jobs     JobsConfig     `yaml:"jobs"`
	Runtime  RuntimeConfig  `yaml:"runtime"`
	DB       DBConfig       `yaml:"db"`
	Log      LogConfig      `yaml:"log"`
	GitHub   GitHubConfig   `yaml:"github"`
	Operator OperatorConfig `yaml:"operator"`
}

type ServerConfig struct {
	Bind string `yaml:"bind" env:"TOOLSHED_BIND"`
}

type StorageConfig struct {
	Backend    string                  `yaml:"backend" env:"TOOLSHED_STORAGE_BACKEND"`
	Filesystem FilesystemStorageConfig `yaml:"filesystem"`
}

type FilesystemStorageConfig struct {
	WorkspaceRoot string `yaml:"workspace_root" env:"TOOLSHED_STORAGE_FILESYSTEM_WORKSPACE_ROOT"`
}

type JobsConfig struct {
	OperationRequestTimeout time.Duration `yaml:"operation_request_timeout" env:"TOOLSHED_JOBS_OPERATION_REQUEST_TIMEOUT"`
}

type RuntimeConfig struct {
	IngressNetwork string                     `yaml:"ingress_network" env:"TOOLSHED_RUNTIME_INGRESS_NETWORK"`
	DockerCompose  DockerComposeRuntimeConfig `yaml:"docker_compose"`
}

type DockerComposeRuntimeConfig struct {
	DefaultServiceCPUs   float64 `yaml:"default_service_cpus" env:"TOOLSHED_RUNTIME_DOCKER_COMPOSE_DEFAULT_SERVICE_CPUS"`
	DefaultServiceMemory string  `yaml:"default_service_memory" env:"TOOLSHED_RUNTIME_DOCKER_COMPOSE_DEFAULT_SERVICE_MEMORY"`
	DefaultServicePIDs   int     `yaml:"default_service_pids" env:"TOOLSHED_RUNTIME_DOCKER_COMPOSE_DEFAULT_SERVICE_PIDS"`
}

type DBConfig struct {
	DSN string `yaml:"dsn" env:"TOOLSHED_DB_DSN"`
}

type LogConfig struct {
	Level     string `yaml:"level" env:"TOOLSHED_LOG_LEVEL"`
	Format    string `yaml:"format" env:"TOOLSHED_LOG_FORMAT"`
	AddSource bool   `yaml:"add_source" env:"TOOLSHED_LOG_ADD_SOURCE"`
}

type GitHubConfig struct {
	AppID          int64         `yaml:"app_id" env:"TOOLSHED_GITHUB_APP_ID"`
	PrivateKeyPath string        `yaml:"private_key_path" env:"TOOLSHED_GITHUB_PRIVATE_KEY_PATH"`
	WebhookSecret  string        `yaml:"webhook_secret" env:"TOOLSHED_GITHUB_WEBHOOK_SECRET"`
	APIBaseURL     string        `yaml:"api_base_url" env:"TOOLSHED_GITHUB_API_BASE_URL"`
	APITimeout     time.Duration `yaml:"api_timeout" env:"TOOLSHED_GITHUB_API_TIMEOUT"`
}

type OperatorConfig struct {
	BearerToken string `yaml:"bearer_token" env:"TOOLSHED_OPERATOR_BEARER_TOKEN"`
}

func Load(path string) (*Config, error) {
	c := defaults()
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read config: %w", err)
		}
		if err := yaml.Unmarshal(b, c); err != nil {
			return nil, fmt.Errorf("parse yaml: %w", err)
		}
	}
	if err := env.Parse(c); err != nil {
		return nil, fmt.Errorf("parse env: %w", err)
	}
	return c, c.Validate()
}
