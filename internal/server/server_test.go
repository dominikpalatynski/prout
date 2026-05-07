package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dominikpalatynski/prout/internal/auth"
	"github.com/dominikpalatynski/prout/internal/config"
	"github.com/dominikpalatynski/prout/internal/workspace"
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

	workspaceHandler, err := workspace.NewWorkspaceHandler(cfg, nil)
	if err != nil {
		t.Fatalf("create workspace handler: %v", err)
	}

	return &Server{
		config:           cfg,
		authManager:      authManager,
		workspaceHandler: workspaceHandler,
		templates:        tmpls,
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

func TestWorkspacesPageRenders(t *testing.T) {
	s := newTestServer(t)

	sessionCookie, err := s.authManager.NewSessionCookie(false, time.Now())
	if err != nil {
		t.Fatalf("create session cookie: %v", err)
	}

	r := chi.NewRouter()
	s.mount(r)

	req := httptest.NewRequest(http.MethodGet, "/workspaces", nil)
	req.AddCookie(sessionCookie)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Workspaces") {
		t.Fatalf("expected workspaces page heading in body, got %q", body)
	}
	if !strings.Contains(body, "No workspaces were found") {
		t.Fatalf("expected empty workspaces message, got %q", body)
	}
}

func TestWorkspacesPageShowsSuccessNotice(t *testing.T) {
	s := newTestServer(t)

	sessionCookie, err := s.authManager.NewSessionCookie(false, time.Now())
	if err != nil {
		t.Fatalf("create session cookie: %v", err)
	}

	r := chi.NewRouter()
	s.mount(r)

	req := httptest.NewRequest(http.MethodGet, "/workspaces?success=workspace_deleted", nil)
	req.AddCookie(sessionCookie)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	if !strings.Contains(rec.Body.String(), "Workspace deleted successfully.") {
		t.Fatalf("expected success message in body, got %q", rec.Body.String())
	}
}

func TestDeleteWorkspaceRejectsInvalidRequest(t *testing.T) {
	s := newTestServer(t)

	sessionCookie, err := s.authManager.NewSessionCookie(false, time.Now())
	if err != nil {
		t.Fatalf("create session cookie: %v", err)
	}

	r := chi.NewRouter()
	s.mount(r)

	req := httptest.NewRequest(
		http.MethodPost,
		"/workspaces/delete",
		strings.NewReader("full_name=owner%2Frepo&pr_number=bad&sha=abcdef"),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(sessionCookie)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected status %d, got %d", http.StatusSeeOther, rec.Code)
	}

	if got := rec.Header().Get("Location"); got != "/workspaces?error=invalid_delete_request" {
		t.Fatalf("expected delete error redirect, got %q", got)
	}
}
