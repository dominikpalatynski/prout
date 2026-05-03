package config

func defaults() *Config {
	return &Config{
		Server: ServerConfig{
			Port: ":8080",
		},
		Log: LogConfig{
			Level:  "info",
			Format: "json",
		},
	}
}
