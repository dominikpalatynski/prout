package server

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/url"

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

func (s *Server) githubSetupPageHandler(w http.ResponseWriter, r *http.Request) {
	s.githubClient.LoadGithubSetupPage(w, r)
}

func (s *Server) githubSetupStartHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	if !s.config.IsValidAdminSecret(r.FormValue("admin_secret")) {
		http.Error(w, "invalid admin secret", http.StatusForbidden)
		return
	}

	if config.IsGithubAppConfigExists() {
		http.Error(w, "github app is already configured", http.StatusConflict)
		return
	}

	state, err := github.CreateSignedSetupState()
	if err != nil {
		slog.Error("failed to create github setup state", "error", err)
		http.Error(w, "failed to create setup state", http.StatusInternalServerError)
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
		http.Error(w, "failed to build github manifest", http.StatusInternalServerError)
		return
	}

	githubURL := "https://github.com/settings/apps/new?state=" + url.QueryEscape(state)

	page := fmt.Sprintf(`
<!doctype html>
<html>
  <head>
    <title>Redirecting to GitHub</title>
  </head>
  <body>
    <p>Redirecting to GitHub...</p>

    <form id="github-form" action="%s" method="post">
      <input type="hidden" name="manifest" value="%s" />
    </form>

    <script>
      document.getElementById("github-form").submit();
    </script>
  </body>
</html>
`,
		html.EscapeString(githubURL),
		html.EscapeString(string(manifestJSON)),
	)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(page))
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
		http.Error(w, "missing code or state", http.StatusBadRequest)
		return
	}

	if config.IsGithubAppConfigExists() {
		http.Redirect(w, r, "/settings/github-setup", http.StatusSeeOther)
		return
	}

	if err := github.VerifySignedSetupState(state); err != nil {
		slog.Error("invalid github setup state", "error", err)
		http.Error(w, "invalid or expired setup state", http.StatusBadRequest)
		return
	}

	appConfig, err := github.ConvertGitHubManifestCode(r.Context(), code)
	if err != nil {
		slog.Error("failed to convert github manifest code", "error", err)
		http.Error(w, "failed to create github app", http.StatusBadGateway)
		return
	}

	if err := github.SaveGitHubAppConfig(appConfig); err != nil {
		slog.Error("failed to save github app config", "error", err)
		http.Error(w, "failed to save github app config", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/settings/github-setup", http.StatusSeeOther)
}
