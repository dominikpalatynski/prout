package config

func defaults() *Config {
	return &Config{
		Server: ServerConfig{
			Bind: ":8080",
		},
		Log: LogConfig{
			Level:  "info",
			Format: "json",
		},
		GitHub: GitHubConfig{
			APIBaseURL: "https://api.github.com",
		},
	}
}
