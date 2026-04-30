package config

import "time"

func defaults() *Config {
	return &Config{
		Server: ServerConfig{
			Bind: ":8080",
		},
		Defaults: DefaultsConfig{
			TTL:                  72 * time.Hour,
			MaxConcurrentPerRepo: 2,
			CPULimit:             "2",
			MemoryLimit:          "2g",
			PIDsLimit:            512,
		},
		Log: LogConfig{
			Level:  "info",
			Format: "json",
		},
	}
}
