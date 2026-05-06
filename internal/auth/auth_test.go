package auth

import (
	"testing"
	"time"

	"github.com/dominikpalatynski/prout/internal/config"
)

func newTestManager(t *testing.T) *Manager {
	t.Helper()

	manager, err := NewManager(config.AuthConfig{
		SessionSecret: "test-session-secret",
		Username:      "admin",
		Password:      "s3cr3t",
		SessionTTL:    "1h",
	})
	if err != nil {
		t.Fatalf("create auth manager: %v", err)
	}
	return manager
}

func TestValidateCredentials(t *testing.T) {
	manager := newTestManager(t)

	if !manager.ValidateCredentials("admin", "s3cr3t") {
		t.Fatal("expected matching credentials to validate")
	}

	if manager.ValidateCredentials("admin", "wrong") {
		t.Fatal("expected wrong password to fail validation")
	}
}

func TestSessionTokenValidation(t *testing.T) {
	manager := newTestManager(t)
	now := time.Now().UTC()

	token, _, err := manager.NewSessionToken(now)
	if err != nil {
		t.Fatalf("create session token: %v", err)
	}

	session, err := manager.ValidateSessionToken(token, now.Add(10*time.Minute))
	if err != nil {
		t.Fatalf("validate session token: %v", err)
	}

	if session.Username != "admin" {
		t.Fatalf("expected username admin, got %q", session.Username)
	}
}

func TestSessionTokenExpires(t *testing.T) {
	manager := newTestManager(t)
	now := time.Now().UTC()

	token, expiresAt, err := manager.NewSessionToken(now)
	if err != nil {
		t.Fatalf("create session token: %v", err)
	}

	if _, err := manager.ValidateSessionToken(token, expiresAt.Add(time.Second)); err == nil {
		t.Fatal("expected expired session token to fail validation")
	}
}
