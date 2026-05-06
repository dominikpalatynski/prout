package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dominikpalatynski/prout/internal/auth"
	"github.com/dominikpalatynski/prout/internal/config"
	"github.com/go-chi/chi/v5"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()

	tmpls, err := loadTemplates()
	if err != nil {
		t.Fatalf("load templates: %v", err)
	}

	cfg := &config.Config{
		Server: config.ServerConfig{
			BaseURL: "http://localhost:8080",
		},
		Auth: config.AuthConfig{
			SessionSecret: "test-session-secret",
			Username:      "admin",
			Password:      "s3cr3t",
			SessionTTL:    "1h",
		},
	}

	authManager, err := auth.NewManager(cfg.Auth)
	if err != nil {
		t.Fatalf("create auth manager: %v", err)
	}

	return &Server{
		config:      cfg,
		authManager: authManager,
		templates:   tmpls,
	}
}

func TestProtectedRouteRedirectsToLogin(t *testing.T) {
	s := newTestServer(t)

	r := chi.NewRouter()
	s.mount(r)

	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected status %d, got %d", http.StatusSeeOther, rec.Code)
	}

	if got := rec.Header().Get("Location"); got != "/auth/login?next=%2Fsettings" {
		t.Fatalf("expected login redirect, got %q", got)
	}
}

func TestAuthLoginCreatesSession(t *testing.T) {
	s := newTestServer(t)

	r := chi.NewRouter()
	s.mount(r)

	req := httptest.NewRequest(
		http.MethodPost,
		"/auth/login",
		strings.NewReader("username=admin&password=s3cr3t&next=%2Fsettings"),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected status %d, got %d", http.StatusSeeOther, rec.Code)
	}

	if got := rec.Header().Get("Location"); got != "/settings" {
		t.Fatalf("expected settings redirect, got %q", got)
	}

	cookies := rec.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Name != auth.SessionCookieName {
		t.Fatalf("expected %s cookie to be set", auth.SessionCookieName)
	}
}

func TestProtectedRouteAcceptsValidSession(t *testing.T) {
	s := newTestServer(t)

	sessionCookie, err := s.authManager.NewSessionCookie(false, time.Now())
	if err != nil {
		t.Fatalf("create session cookie: %v", err)
	}

	r := chi.NewRouter()
	s.mount(r)

	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	req.AddCookie(sessionCookie)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}
