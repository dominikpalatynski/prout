package config

import (
	"fmt"
	"os"
	"time"

	"go.yaml.in/yaml/v3"
)

const (
	ProdEnvironment = "production"
	DevEnvironment  = "development"
)

type Config struct {
	Log         LogConfig         `yaml:"log"`
	Server      ServerConfig      `yaml:"server"`
	GitHub      GitHubConfig      `yaml:"github"`
	Environment EnvironmentConfig `yaml:"environment"`
}

type EnvironmentConfig struct {
	Name string `yaml:"name"`
}

type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

type ServerConfig struct {
	Port string `yaml:"port"`
}

type GitHubConfig struct {
	AppID            int64            `yaml:"app_id"`
	PrivateKeyPath   string           `yaml:"private_key_path"`
	WebhookSecret    string           `yaml:"webhook_secret"`
	APIBaseURL       string           `yaml:"api_base_url"`
	APITimeout       time.Duration    `yaml:"api_timeout"`
	APIStreamTimeout time.Duration    `yaml:"api_stream_timeout"`
	Repository       RepositoryConfig `yaml:"repository"`
}

type RepositoryConfig struct {
	Owner          string              `yaml:"owner"`
	Name           string              `yaml:"name"`
	BuildSettings  BuildSettingsConfig `yaml:"build_settings"`
	WildcardDomain string              `yaml:"wildcard_domain"`
}

type BuildSettingsConfig struct {
	DockerComposeFilePath string            `yaml:"docker_compose_file_path"`
	ExposedPort           int               `yaml:"exposed_port"`
	ExposedServiceName    string            `yaml:"exposed_service_name"`
	EnvironmentVariables  map[string]string `yaml:"environment_variables"`
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
	return c, nil
}
