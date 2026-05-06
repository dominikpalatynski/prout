package server

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dominikpalatynski/prout/internal/auth"
)

func (s *Server) authLoginHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if s.hasValidSession(r) {
			http.Redirect(w, r, safeRedirectTarget(r.URL.Query().Get("next")), http.StatusSeeOther)
			return
		}

		s.renderLoginPage(w, http.StatusOK, "", "", safeRedirectTarget(r.URL.Query().Get("next")))
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			s.renderLoginPage(w, http.StatusBadRequest, "Could not read the login form. Try again.", "", safeRedirectTarget(r.FormValue("next")))
			return
		}

		username := r.FormValue("username")
		next := safeRedirectTarget(r.FormValue("next"))
		if !s.authManager.ValidateCredentials(username, r.FormValue("password")) {
			s.renderLoginPage(w, http.StatusUnauthorized, "Invalid username or password.", username, next)
			return
		}

		cookie, err := s.authManager.NewSessionCookie(isSecureRequest(r), time.Now())
		if err != nil {
			slog.Error("failed to create auth session", "error", err)
			http.Error(w, "failed to create session", http.StatusInternalServerError)
			return
		}

		http.SetCookie(w, cookie)
		http.Redirect(w, r, next, http.StatusSeeOther)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) authLogoutHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet, http.MethodPost:
		http.SetCookie(w, auth.ExpiredSessionCookie(isSecureRequest(r)))
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) requireValidSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.hasValidSession(r) {
			next.ServeHTTP(w, r)
			return
		}

		if _, err := r.Cookie(auth.SessionCookieName); err == nil {
			http.SetCookie(w, auth.ExpiredSessionCookie(isSecureRequest(r)))
		}

		loginURL := "/auth/login?next=" + url.QueryEscape(r.URL.RequestURI())
		http.Redirect(w, r, loginURL, http.StatusSeeOther)
	})
}

func (s *Server) hasValidSession(r *http.Request) bool {
	if s.authManager == nil {
		return false
	}

	cookie, err := r.Cookie(auth.SessionCookieName)
	if err != nil {
		return false
	}

	_, err = s.authManager.ValidateSessionToken(cookie.Value, time.Now())
	return err == nil
}

func (s *Server) renderLoginPage(w http.ResponseWriter, status int, errorMessage, username, next string) {
	data := loginPageData{
		basePage: basePage{
			Title: "Prout · Sign in",
			Host:  s.hostFromBaseURL(),
		},
		Username:     username,
		Next:         next,
		ErrorMessage: errorMessage,
	}
	s.templates.render(w, status, "login", data)
}

func safeRedirectTarget(next string) string {
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		return "/settings"
	}
	return next
}

func isSecureRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}
