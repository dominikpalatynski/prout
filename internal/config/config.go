package config

import (
	"fmt"
	"os"

	"go.yaml.in/yaml/v3"
)

type Config struct {
	Log    LogConfig    `yaml:"log"`
	Server ServerConfig `yaml:"server"`
}

type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

type ServerConfig struct {
	Port string `yaml:"port"`
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
