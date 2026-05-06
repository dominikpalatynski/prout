package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dominikpalatynski/prout/internal/config"
	"github.com/go-chi/chi/v5"
)

func TestGithubSetupResetRoute(t *testing.T) {
	tmpls, err := loadTemplates()
	if err != nil {
		t.Fatalf("load templates: %v", err)
	}

	s := &Server{
		config: &config.Config{
			Server: config.ServerConfig{
				AdminSecret: "expected-secret",
			},
		},
		templates: tmpls,
	}

	r := chi.NewRouter()
	s.mount(r)

	req := httptest.NewRequest(
		http.MethodPost,
		"/settings/github-setup/reset",
		strings.NewReader("admin_secret=wrong-secret&confirm=RESET"),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status %d for mounted reset handler, got %d", http.StatusForbidden, rec.Code)
	}
}
