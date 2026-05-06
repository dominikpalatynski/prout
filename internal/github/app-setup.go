package github

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dominikpalatynski/prout/internal/config"
)

type GitHubManifestConversionResponse struct {
	ID            int64  `json:"id"`
	Slug          string `json:"slug"`
	ClientID      string `json:"client_id"`
	ClientSecret  string `json:"client_secret"`
	WebhookSecret string `json:"webhook_secret"`
	PEM           string `json:"pem"`
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

type githubSetupStatePayload struct {
	Nonce string `json:"nonce"`
	Exp   int64  `json:"exp"`
	Kind  string `json:"kind"`
}

func ConvertGitHubManifestCode(ctx context.Context, code string) (*GitHubManifestConversionResponse, error) {
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

func SaveGitHubAppConfig(app *GitHubManifestConversionResponse) error {
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

func SignSetupPayload(payloadEncoded string) string {

	githubAppConfig, err := config.LoadGithubAppConfig()
	if err != nil {
		slog.Error("failed to load github app config for signing setup state", "error", err)
		return ""
	}

	mac := hmac.New(sha256.New, []byte(githubAppConfig.WebhookSecret))
	_, _ = mac.Write([]byte(payloadEncoded))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func VerifySignedSetupState(state string) error {
	parts := strings.SplitN(state, ".", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid state format")
	}

	payloadEncoded := parts[0]
	signature := parts[1]

	expectedSignature := SignSetupPayload(payloadEncoded)

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

func CreateSignedSetupState() (string, error) {
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
	signature := SignSetupPayload(payloadEncoded)

	return payloadEncoded + "." + signature, nil
}
