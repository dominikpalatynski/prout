package config

import "time"

const (
	DefaultGitHubAPITimeout       = 10 * time.Second
	DefaultGitHubAPIStreamTimeout = 5 * time.Minute
)

func defaults() *Config {
	return &Config{
		Server: ServerConfig{
			Port:    ":8080",
			BaseURL: "http://localhost:8080",
		},
		Environment: EnvironmentConfig{
			Name: DevEnvironment,
		},
		Log: LogConfig{
			Level:  "info",
			Format: "json",
		},
		Auth: AuthConfig{
			SessionSecret: "change-me-session-secret",
			Username:      "login",
			Password:      "password",
			SessionTTL:    "12h",
		},
		GitHub: GitHubConfig{
			APIBaseURL:       "https://api.github.com",
			APITimeout:       DefaultGitHubAPITimeout,
			APIStreamTimeout: DefaultGitHubAPIStreamTimeout,
		},
	}
}
