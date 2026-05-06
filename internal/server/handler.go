package server

import (
	"encoding/json"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/dominikpalatynski/prout/internal/config"
	"github.com/dominikpalatynski/prout/internal/event"
	"github.com/dominikpalatynski/prout/internal/github"
)

func (s *Server) readyzHandler(w http.ResponseWriter, r *http.Request) {
	slog.Info("Received readyz request")
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "ready"})
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	slog.Info("Received healthz request")
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "healthy"})
}

func (s *Server) handleGithubWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("failed to read github webhook payload", "error", err)
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	var payload event.PullRequestWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		slog.Error("failed to decode github webhook payload", "error", err)
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	if err := s.githubEventHandler.HandleGithubEvent(event.GithubWebhookRequest{
		Body:      body,
		Signature: r.Header.Get("X-Hub-Signature-256"),
		Payload:   payload,
	}); err != nil {
		slog.Error("failed to validate github webhook payload", "error", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]interface{}{"status": "download has been queued"})
}

func (s *Server) hostFromBaseURL() string {
	if s.config == nil || s.config.Server.BaseURL == "" {
		return ""
	}
	u, err := url.Parse(s.config.Server.BaseURL)
	if err != nil || u.Host == "" {
		return s.config.Server.BaseURL
	}
	return u.Host
}

func (s *Server) basePage(title string, breadcrumb []breadcrumbItem) basePage {
	return basePage{
		Title:      title,
		Host:       s.hostFromBaseURL(),
		Breadcrumb: breadcrumb,
		LogoutHref: "/auth/logout",
	}
}

func (s *Server) settingsIndexHandler(w http.ResponseWriter, r *http.Request) {
	githubItem := settingsIndexItem{
		Title:       "GitHub Setup",
		Description: "Connect a GitHub App so Prout can listen to pull requests and post status checks.",
		Href:        "/settings/github-setup",
	}
	if config.IsGithubAppConfigExists() {
		githubItem.Status = "Configured"
		githubItem.Tone = "success"
	} else {
		githubItem.Status = "Not configured"
		githubItem.Tone = "warning"
	}

	data := settingsIndexData{
		basePage: s.basePage("Prout · Settings", []breadcrumbItem{{Label: "settings"}}),
		Items: []settingsIndexItem{
			githubItem,
			{
				Title:       "Repository",
				Description: "Owner, name, wildcard preview domain and exposed service.",
				Status:      "From server.yml",
				Tone:        "neutral",
				Href:        "#",
			},
			{
				Title:       "Build settings",
				Description: "Docker compose file, exposed port, environment variables.",
				Status:      "From server.yml",
				Tone:        "neutral",
				Href:        "#",
			},
			{
				Title:       "Authentication",
				Description: "Dashboard username, password and session settings.",
				Status:      "From server.yml",
				Tone:        "neutral",
				Href:        "#",
			},
		},
	}
	s.templates.render(w, http.StatusOK, "settings_index", data)
}

func (s *Server) setupBreadcrumb() []breadcrumbItem {
	return []breadcrumbItem{
		{Label: "settings", Href: "/settings"},
		{Label: "github"},
	}
}

func (s *Server) renderConfiguredPage(w http.ResponseWriter, status int, errorField, errorMessage string) {
	appCfg, err := config.LoadGithubAppConfig()
	if err != nil {
		slog.Error("failed to load github app config", "error", err)
		s.renderSetupError(w, http.StatusInternalServerError,
			"Could not load app configuration",
			"The GitHub App config file could not be read. Check the server logs and file permissions.",
			"HTTP 500", "config_read_failed")
		return
	}
	appName := humanizeAppName(appCfg.AppSlug)
	if appName == "" {
		appName = "Prout Preview"
	}
	data := githubConfiguredData{
		basePage:     s.basePage("Prout · GitHub Setup", s.setupBreadcrumb()),
		AppName:      appName,
		AppSlug:      appCfg.AppSlug,
		AppID:        appCfg.AppID,
		ErrorField:   errorField,
		ErrorMessage: errorMessage,
	}
	s.templates.render(w, status, "github_configured", data)
}

func (s *Server) renderEmptyPage(w http.ResponseWriter, status int, errorMessage string) {
	data := githubEmptyData{
		basePage:     s.basePage("Prout · GitHub Setup", s.setupBreadcrumb()),
		ErrorMessage: errorMessage,
	}
	s.templates.render(w, status, "github_empty", data)
}

func (s *Server) githubSetupPageHandler(w http.ResponseWriter, r *http.Request) {
	if config.IsGithubAppConfigExists() {
		s.renderConfiguredPage(w, http.StatusOK, "", "")
		return
	}
	s.renderEmptyPage(w, http.StatusOK, "")
}

func humanizeAppName(slug string) string {
	if slug == "" {
		return ""
	}
	parts := strings.Split(slug, "-")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}

func (s *Server) renderSetupError(w http.ResponseWriter, status int, title, desc, httpStatus, code string) {
	data := githubErrorData{
		basePage: s.basePage("Prout · GitHub Setup", []breadcrumbItem{
			{Label: "settings", Href: "/settings"},
			{Label: "github", Href: "/settings/github-setup"},
			{Label: "error"},
		}),
		Title:       title,
		Description: desc,
		HTTPStatus:  httpStatus,
		ErrorCode:   code,
	}
	s.templates.render(w, status, "github_error", data)
}

func (s *Server) githubSetupStartHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		s.renderSetupError(w, http.StatusBadRequest,
			"Invalid form submission",
			"Could not parse the submitted form. Try again.",
			"HTTP 400", "invalid_form")
		return
	}

	if config.IsGithubAppConfigExists() {
		http.Redirect(w, r, "/settings/github-setup", http.StatusSeeOther)
		return
	}

	state, err := github.CreateSignedSetupState()
	if err != nil {
		slog.Error("failed to create github setup state", "error", err)
		s.renderSetupError(w, http.StatusInternalServerError,
			"Failed to create setup state",
			"Could not generate a signed state token. Check the server logs.",
			"HTTP 500", "state_creation_failed")
		return
	}

	githubBaseURL := s.config.GitHub.APIBaseURL
	appBaseURL := s.config.Server.BaseURL

	manifest := map[string]any{
		"name":         "Prout Preview",
		"url":          githubBaseURL,
		"redirect_url": appBaseURL + "/settings/github-setup/callback",
		"hook_attributes": map[string]string{
			"url": appBaseURL + "/webhooks/github",
		},
		"public": false,
		"default_events": []string{
			"pull_request",
			"push",
		},
		"default_permissions": map[string]string{
			"metadata":      "read",
			"contents":      "read",
			"pull_requests": "write",
			"checks":        "write",
			"statuses":      "write",
			"issues":        "write",
		},
	}

	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		slog.Error("failed to marshal github app manifest", "error", err)
		s.renderSetupError(w, http.StatusInternalServerError,
			"Failed to build GitHub manifest",
			"Could not serialize the app manifest payload. Check the server logs.",
			"HTTP 500", "manifest_marshal_failed")
		return
	}

	githubURL := "https://github.com/settings/apps/new?state=" + url.QueryEscape(state)

	data := githubRedirectData{
		basePage: s.basePage("Prout · Redirecting", []breadcrumbItem{
			{Label: "settings", Href: "/settings"},
			{Label: "github", Href: "/settings/github-setup"},
			{Label: "redirect"},
		}),
		GitHubURL:    template.URL(githubURL),
		ManifestJSON: string(manifestJSON),
	}
	s.templates.render(w, http.StatusOK, "github_redirect", data)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Error("Failed to write JSON response", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) githubSetupCallbackHandler(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		s.renderSetupError(w, http.StatusBadRequest,
			"Missing setup parameters",
			"GitHub redirected back without a code or state token. Restart the setup flow from the beginning.",
			"HTTP 400", "missing_code_or_state")
		return
	}

	if config.IsGithubAppConfigExists() {
		http.Redirect(w, r, "/settings/github-setup", http.StatusSeeOther)
		return
	}

	if err := github.VerifySignedSetupState(state); err != nil {
		slog.Error("invalid github setup state", "error", err)
		s.renderSetupError(w, http.StatusBadRequest,
			"Invalid or expired setup state",
			"The state token returned by GitHub didn't match. This usually means the setup link was opened more than once, the page was reloaded mid-flow, or the token expired (15 min).",
			"HTTP 400", "setup_state_invalid")
		return
	}

	appConfig, err := github.ConvertGitHubManifestCode(r.Context(), code)
	if err != nil {
		slog.Error("failed to convert github manifest code", "error", err)
		s.renderSetupError(w, http.StatusBadGateway,
			"Failed to create GitHub App",
			"GitHub rejected the manifest exchange. The setup code may have already been used, or GitHub is temporarily unavailable. Restart the setup flow to try again.",
			"HTTP 502", "manifest_conversion_failed")
		return
	}

	if err := github.SaveGitHubAppConfig(appConfig); err != nil {
		slog.Error("failed to save github app config", "error", err)
		s.renderSetupError(w, http.StatusInternalServerError,
			"Failed to save GitHub App config",
			"The app was created on GitHub but the credentials could not be persisted locally. Check the server logs and file permissions on app_config/.",
			"HTTP 500", "config_save_failed")
		return
	}

	http.Redirect(w, r, "/settings/github-setup", http.StatusSeeOther)
}

func (s *Server) githubSetupResetHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		s.renderSetupError(w, http.StatusBadRequest,
			"Invalid form submission",
			"Could not parse the submitted form. Try again.",
			"HTTP 400", "invalid_form")
		return
	}

	if r.FormValue("confirm") != "RESET" {
		s.renderSetupError(w, http.StatusBadRequest,
			"Reset confirmation missing",
			"You must type RESET (case-sensitive) in the confirmation field to reset the GitHub integration.",
			"HTTP 400", "invalid_reset_confirm")
		return
	}

	if !config.IsGithubAppConfigExists() {
		http.Redirect(w, r, "/settings/github-setup", http.StatusSeeOther)
		return
	}

	if err := config.ResetGithubAppConfig(); err != nil {
		slog.Error("failed to reset github config", "error", err)
		s.renderSetupError(w, http.StatusInternalServerError,
			"Failed to reset GitHub integration",
			"The config could not be removed. Check file permissions on app_config/.",
			"HTTP 500", "config_reset_failed")
		return
	}

	http.Redirect(w, r, "/settings/github-setup", http.StatusSeeOther)
}
