package config

import "testing"

func TestIsValidAdminSecret(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			AdminSecret: "expected-secret",
		},
	}

	if !cfg.IsValidAdminSecret("expected-secret") {
		t.Fatal("expected matching admin secret to be valid")
	}

	if cfg.IsValidAdminSecret("wrong-secret") {
		t.Fatal("expected non-matching admin secret to be invalid")
	}
}
