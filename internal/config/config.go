package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.yaml.in/yaml/v3"
)

const (
	ProdEnvironment     = "production"
	DevEnvironment      = "development"
	privateKeyFileMode  = 0600
	privateKeyFilePath  = "app_config/github_app_private_key.pem"
	githubAppConfigPath = "app_config/github_app_config.yaml"
)

type Config struct {
	Log         LogConfig         `yaml:"log"`
	Server      ServerConfig      `yaml:"server"`
	GitHub      GitHubConfig      `yaml:"github"`
	Environment EnvironmentConfig `yaml:"environment"`
}

type GithubAppConfig struct {
	AppID         int64  `yaml:"app_id"`
	AppSlug       string `yaml:"app_slug"`
	ClientID      string `yaml:"client_id"`
	ClientSecret  string `yaml:"client_secret"`
	WebhookSecret string `yaml:"webhook_secret"`
}

type EnvironmentConfig struct {
	Name string `yaml:"name"`
}

type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

type ServerConfig struct {
	Port        string `yaml:"port"`
	BaseURL     string `yaml:"base_url"`
	AdminSecret string `yaml:"admin_secret"`
}

type GitHubConfig struct {
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

func (c *Config) IsValidAdminSecret(secret string) bool {
	return c.Server.AdminSecret == secret
}

func LoadGithubAppConfig() (*GithubAppConfig, error) {
	c := &GithubAppConfig{}
	b, err := os.ReadFile(githubAppConfigPath)
	if err != nil {
		return nil, fmt.Errorf("read github app config: %w", err)
	}
	if err := yaml.Unmarshal(b, c); err != nil {
		return nil, fmt.Errorf("parse github app config yaml: %w", err)
	}
	return c, nil
}

func SaveGithubAppConfig(c *GithubAppConfig, privateKey string) error {
	b, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal github app config: %w", err)
	}
	if err := ensureFileExists(githubAppConfigPath, privateKeyFileMode); err != nil {
		return fmt.Errorf("prepare github app config file: %w", err)
	}
	if err := os.WriteFile(githubAppConfigPath, b, privateKeyFileMode); err != nil {
		return fmt.Errorf("write github app config: %w", err)
	}

	if privateKey != "" {
		if err := ensureFileExists(privateKeyFilePath, privateKeyFileMode); err != nil {
			return fmt.Errorf("prepare private key file: %w", err)
		}

		if err := os.WriteFile(privateKeyFilePath, []byte(privateKey), privateKeyFileMode); err != nil {
			return fmt.Errorf("write private key: %w", err)
		}
	}

	return nil
}

func ResetGithubAppConfig() error {
	if err := os.Remove(githubAppConfigPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove github app config: %w", err)
	}

	if err := os.Remove(privateKeyFilePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove private key file: %w", err)
	}

	return nil
}

func ensureFileExists(path string, mode os.FileMode) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("create parent directory: %w", err)
		}

		file, err := os.OpenFile(path, os.O_CREATE, mode)
		if err != nil {
			return fmt.Errorf("create file: %w", err)
		}
		if err := file.Close(); err != nil {
			return fmt.Errorf("close file: %w", err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("stat file: %w", err)
	}

	return nil
}

func LoadPrivateKey() (string, error) {
	b, err := os.ReadFile(privateKeyFilePath)
	if err != nil {
		return "", fmt.Errorf("read private key: %w", err)
	}
	return string(b), nil
}

func IsGithubAppConfigExists() bool {
	_, err := os.Stat(githubAppConfigPath)
	_, err2 := os.Stat(privateKeyFilePath)
	return err == nil && err2 == nil
}
