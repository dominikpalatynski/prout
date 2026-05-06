package server

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dominikpalatynski/prout/internal/config"
	"github.com/dominikpalatynski/prout/internal/event"
)

func (s *Server) readyzHandler(w http.ResponseWriter, r *http.Request) {
	slog.Info("Received readyz request")
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "ready"})
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	slog.Info("Received healthz request")
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "healthy"})
}

func (s *Server) testDownloadHandler(w http.ResponseWriter, r *http.Request) {
	slog.Info("Received test download request")
	// s.queue.Run(func() error {
	// 	return s.workspaceHandler.HandleCreateWorkspace()
	// })
	writeJSON(w, http.StatusAccepted, map[string]interface{}{"status": "download has been queued"})
}

func (s *Server) handleGithubWebhook(w http.ResponseWriter, r *http.Request) {
	var payload event.PullRequestWebhookPayload

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		slog.Error("failed to decode github webhook payload", "error", err)
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	if err := s.githubEventHandler.HandleGithubEvent(payload); err != nil {
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

	// if !s.isValidAdminSecret(r.FormValue("admin_secret")) {
	// 	http.Error(w, "invalid admin secret", http.StatusForbidden)
	// 	return
	// }

	// if s.githubConfigExists() {
	// 	http.Error(w, "github app is already configured", http.StatusConflict)
	// 	return
	// }

	state, err := s.createSignedSetupState()
	if err != nil {
		slog.Error("failed to create github setup state", "error", err)
		http.Error(w, "failed to create setup state", http.StatusInternalServerError)
		return
	}

	baseURL := s.config.GitHub.APIBaseURL

	manifest := map[string]any{
		"name": "Prout Preview",
		"url":  baseURL,
		// "redirect_url": baseURL + "/settings/github-setup/callback",
		"redirect_url": "https://webhook.site/c770a264-ec25-4342-94cf-44a24e59ad12",
		"hook_attributes": map[string]string{
			"url": baseURL + "/webhooks/github",
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

type githubSetupStatePayload struct {
	Nonce string `json:"nonce"`
	Exp   int64  `json:"exp"`
	Kind  string `json:"kind"`
}

func (s *Server) createSignedSetupState() (string, error) {
	nonceBytes := make([]byte, 32)
	if _, err := rand.Read(nonceBytes); err != nil {
		return "", err
	}

	payload := githubSetupStatePayload{
		Nonce: base64.RawURLEncoding.EncodeToString(nonceBytes),
		Exp:   time.Now().Add(15 * time.Minute).Unix(),
		Kind:  "github_setup",
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	payloadEncoded := base64.RawURLEncoding.EncodeToString(payloadJSON)
	signature := s.signSetupPayload(payloadEncoded)

	return payloadEncoded + "." + signature, nil
}

func (s *Server) verifySignedSetupState(state string) error {
	parts := strings.SplitN(state, ".", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid state format")
	}

	payloadEncoded := parts[0]
	signature := parts[1]

	expectedSignature := s.signSetupPayload(payloadEncoded)

	if len(signature) != len(expectedSignature) {
		return fmt.Errorf("invalid state signature")
	}

	if subtle.ConstantTimeCompare([]byte(signature), []byte(expectedSignature)) != 1 {
		return fmt.Errorf("invalid state signature")
	}

	payloadJSON, err := base64.RawURLEncoding.DecodeString(payloadEncoded)
	if err != nil {
		return fmt.Errorf("invalid state payload: %w", err)
	}

	var payload githubSetupStatePayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return fmt.Errorf("invalid state payload json: %w", err)
	}

	if payload.Kind != "github_setup" {
		return fmt.Errorf("invalid state kind")
	}

	if time.Now().Unix() > payload.Exp {
		return fmt.Errorf("state expired")
	}

	return nil
}

func (s *Server) signSetupPayload(payloadEncoded string) string {
	mac := hmac.New(sha256.New, []byte(s.config.GitHub.WebhookSecret))
	_, _ = mac.Write([]byte(payloadEncoded))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
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

	if err := s.verifySignedSetupState(state); err != nil {
		slog.Error("invalid github setup state", "error", err)
		http.Error(w, "invalid or expired setup state", http.StatusBadRequest)
		return
	}

	appConfig, err := s.convertGitHubManifestCode(r.Context(), code)
	if err != nil {
		slog.Error("failed to convert github manifest code", "error", err)
		http.Error(w, "failed to create github app", http.StatusBadGateway)
		return
	}

	if err := s.saveGitHubAppConfig(appConfig); err != nil {
		slog.Error("failed to save github app config", "error", err)
		http.Error(w, "failed to save github app config", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/settings/github-setup", http.StatusSeeOther)
}

type GitHubManifestConversionResponse struct {
	ID            int64  `json:"id"`
	Slug          string `json:"slug"`
	ClientID      string `json:"client_id"`
	ClientSecret  string `json:"client_secret"`
	WebhookSecret string `json:"webhook_secret"`
	PEM           string `json:"pem"`
}

func (s *Server) convertGitHubManifestCode(ctx context.Context, code string) (*GitHubManifestConversionResponse, error) {
	endpoint := "https://api.github.com/app-manifests/" + url.PathEscape(code) + "/conversions"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "prout")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("github manifest conversion failed: status=%d body=%s", resp.StatusCode, string(body))
	}

	var out GitHubManifestConversionResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}

	if out.ID == 0 || out.PEM == "" || out.WebhookSecret == "" {
		return nil, fmt.Errorf("github manifest conversion response is missing required fields")
	}

	return &out, nil
}

type StoredGitHubAppConfig struct {
	AppID         int64     `json:"app_id"`
	AppSlug       string    `json:"app_slug"`
	ClientID      string    `json:"client_id"`
	ClientSecret  string    `json:"client_secret"`
	WebhookSecret string    `json:"webhook_secret"`
	PrivateKeyPEM string    `json:"private_key_pem"`
	CreatedAt     time.Time `json:"created_at"`
}

func (s *Server) saveGitHubAppConfig(app *GitHubManifestConversionResponse) error {
	appConfig := StoredGitHubAppConfig{
		AppID:         app.ID,
		AppSlug:       app.Slug,
		ClientID:      app.ClientID,
		ClientSecret:  app.ClientSecret,
		WebhookSecret: app.WebhookSecret,
		PrivateKeyPEM: app.PEM,
	}

	if err := config.SaveGithubAppConfig(&config.GithubAppConfig{
		AppID:         appConfig.AppID,
		AppSlug:       appConfig.AppSlug,
		ClientID:      appConfig.ClientID,
		ClientSecret:  appConfig.ClientSecret,
		WebhookSecret: appConfig.WebhookSecret,
	}, appConfig.PrivateKeyPEM); err != nil {
		return err
	}
	return nil
}
