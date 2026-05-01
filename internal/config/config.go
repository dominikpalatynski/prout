package config

import (
	"fmt"
	"os"

	"github.com/caarlos0/env/v11"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	DB       DBConfig       `yaml:"db"`
	Log      LogConfig      `yaml:"log"`
	GitHub   GitHubConfig   `yaml:"github"`
	Operator OperatorConfig `yaml:"operator"`
}

type ServerConfig struct {
	Bind string `yaml:"bind" env:"TOOLSHED_BIND"`
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
	AppID          int64  `yaml:"app_id" env:"TOOLSHED_GITHUB_APP_ID"`
	PrivateKeyPath string `yaml:"private_key_path" env:"TOOLSHED_GITHUB_PRIVATE_KEY_PATH"`
	WebhookSecret  string `yaml:"webhook_secret" env:"TOOLSHED_GITHUB_WEBHOOK_SECRET"`
	APIBaseURL     string `yaml:"api_base_url" env:"TOOLSHED_GITHUB_API_BASE_URL"`
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
