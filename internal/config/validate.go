package config

import (
	"errors"
	"fmt"
	"strings"
)

func (c *Config) Validate() error {
	var errs []string

	if c.Server.Bind == "" {
		errs = append(errs, "server.bind is required")
	}
	if c.DB.DSN == "" {
		errs = append(errs, "db.dsn is required (or set TOOLSHED_DB_DSN)")
	}

	switch strings.ToLower(c.Log.Level) {
	case "", "debug", "info", "warn", "error":
	default:
		errs = append(errs, fmt.Sprintf("log.level: unknown value %q", c.Log.Level))
	}
	switch strings.ToLower(c.Log.Format) {
	case "", "json", "text":
	default:
		errs = append(errs, fmt.Sprintf("log.format: unknown value %q", c.Log.Format))
	}

	if len(errs) > 0 {
		return errors.New("invalid config: " + strings.Join(errs, "; "))
	}
	return nil
}
