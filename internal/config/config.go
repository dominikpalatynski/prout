package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Server         ServerConfig    `yaml:"server"`
	BootstrapOwner string          `yaml:"bootstrap_owner" env:"TOOLSHED_BOOTSTRAP_OWNER"`
	GitHubApp      GitHubAppConfig `yaml:"github_app"`
	OAuth          OAuthConfig     `yaml:"oauth"`
	ACME           ACMEConfig      `yaml:"acme"`
	Defaults       DefaultsConfig  `yaml:"defaults"`
	DB             DBConfig        `yaml:"db"`
	Log            LogConfig       `yaml:"log"`
}

type ServerConfig struct {
	Bind      string `yaml:"bind" env:"TOOLSHED_BIND"`
	Domain    string `yaml:"domain" env:"TOOLSHED_DOMAIN"`
	PanelHost string `yaml:"panel_host" env:"TOOLSHED_PANEL_HOST"`
	CSRFKey   []byte `yaml:"-" env:"TOOLSHED_CSRF_KEY"`
}

type GitHubAppConfig struct {
	AppID             int64  `yaml:"app_id" env:"TOOLSHED_GH_APP_ID"`
	PrivateKeyPath    string `yaml:"private_key_path" env:"TOOLSHED_GH_PK_PATH"`
	WebhookSecretPath string `yaml:"webhook_secret_path" env:"TOOLSHED_GH_WEBHOOK_SECRET_PATH"`
}

type OAuthConfig struct {
	ClientID         string `yaml:"client_id" env:"TOOLSHED_OAUTH_CLIENT_ID"`
	ClientSecretPath string `yaml:"client_secret_path" env:"TOOLSHED_OAUTH_CLIENT_SECRET_PATH"`
}

type ACMEConfig struct {
	Email                  string `yaml:"email" env:"TOOLSHED_ACME_EMAIL"`
	CloudflareAPITokenPath string `yaml:"cloudflare_api_token_path" env:"TOOLSHED_CF_TOKEN_PATH"`
}

type DefaultsConfig struct {
	TTL                  time.Duration `yaml:"ttl"`
	MaxConcurrentPerRepo int           `yaml:"max_concurrent_per_repo"`
	CPULimit             string        `yaml:"cpu_limit"`
	MemoryLimit          string        `yaml:"memory_limit"`
	PIDsLimit            int           `yaml:"pids_limit"`
}

type DBConfig struct {
	DSN string `yaml:"dsn" env:"TOOLSHED_DB_DSN"`
}

type LogConfig struct {
	Level     string `yaml:"level" env:"TOOLSHED_LOG_LEVEL"`
	Format    string `yaml:"format" env:"TOOLSHED_LOG_FORMAT"`
	AddSource bool   `yaml:"add_source" env:"TOOLSHED_LOG_ADD_SOURCE"`
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

func (c *Config) LoadGitHubPrivateKey() ([]byte, error) {
	return os.ReadFile(c.GitHubApp.PrivateKeyPath)
}

func (c *Config) LoadGitHubWebhookSecret() ([]byte, error) {
	b, err := os.ReadFile(c.GitHubApp.WebhookSecretPath)
	if err != nil {
		return nil, err
	}
	return []byte(strings.TrimSpace(string(b))), nil
}

func (c *Config) LoadOAuthClientSecret() (string, error) {
	b, err := os.ReadFile(c.OAuth.ClientSecretPath)
	return strings.TrimSpace(string(b)), err
}

func (c *Config) LoadCloudflareToken() (string, error) {
	b, err := os.ReadFile(c.ACME.CloudflareAPITokenPath)
	return strings.TrimSpace(string(b)), err
}
