package github

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/dominikpalatynski/prout/internal/config"
)

type GithubClient struct {
	cfg        *config.Config
	privateKey *rsa.PrivateKey
}

func NewGithubClient(cfg *config.Config) (*GithubClient, error) {

	privateKeyData, err := os.ReadFile(cfg.GitHub.PrivateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("read github app private key: %w", err)
	}

	privateKey, err := parsePrivateKey(privateKeyData)
	if err != nil {
		return nil, fmt.Errorf("parse github app private key: %w", err)
	}
	return &GithubClient{
		cfg:        cfg,
		privateKey: privateKey,
	}, nil
}

func (gh *GithubClient) SecureJoin(root, relative string) (string, error) {
	trimmed := strings.TrimSpace(relative)
	if trimmed == "" {
		return "", fmt.Errorf("workspace path is required")
	}

	clean := filepath.Clean(trimmed)
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid workspace path %q", relative)
	}

	fullPath := filepath.Join(root, clean)
	relToRoot, err := filepath.Rel(root, fullPath)
	if err != nil {
		return "", fmt.Errorf("resolve workspace path: %w", err)
	}
	if relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("workspace path %q escapes workspace root", relative)
	}

	return fullPath, nil
}

func (gh *GithubClient) GetInstallationID(ctx context.Context, appJWT, owner, name string) (int64, error) {
	req, err := gh.NewRequest(ctx, http.MethodGet, gh.EndpointURL(
		fmt.Sprintf("/repos/%s/%s/installation", url.PathEscape(owner), url.PathEscape(name)),
	), bytes.NewReader(nil), appJWT)
	if err != nil {
		slog.Error("create github installation request failed", "error", err)
		return 0, err
	}

	var response struct {
		ID int64 `json:"id"`
	}
	if err := gh.SendRequest(req, &response); err != nil {
		slog.Error("get github installation id failed", "error", err)
		return 0, err
	}
	if response.ID <= 0 {
		slog.Error("github installation response missing id")
		return 0, errors.New("github installation response missing id")
	}
	return response.ID, nil
}

func logAndWrapGitHubAPIError(req *http.Request, statusCode int, body []byte) error {
	message := strings.TrimSpace(string(body))
	if message == "" {
		message = http.StatusText(statusCode)
	}

	slog.Error("GitHub API returned non-success status", "method", req.Method, "url", req.URL.Redacted(), "status_code", statusCode, "response_body", message)

	return &APIError{
		Method:     req.Method,
		URL:        req.URL.Redacted(),
		StatusCode: statusCode,
		Message:    message,
	}
}

func parsePrivateKey(privateKeyPEM []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(privateKeyPEM)
	if block == nil {
		return nil, errors.New("missing PEM block")
	}

	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}

	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("private key is not RSA")
	}

	return rsaKey, nil
}

func (gh *GithubClient) DownloadTarball(ctx context.Context, owner, name string, installationID int64, sha string) (io.ReadCloser, error) {
	if strings.TrimSpace(owner) == "" || strings.TrimSpace(name) == "" {
		slog.Error("repository owner and name are required")
		return nil, fmt.Errorf("repository owner and name are required")
	}
	if installationID <= 0 {
		slog.Error("github installation id must be positive")
		return nil, fmt.Errorf("github installation id must be positive")
	}
	if strings.TrimSpace(sha) == "" {
		slog.Error("pull request head sha is required")
		return nil, fmt.Errorf("pull request head sha is required")
	}

	appJWT, err := gh.AppJWT(time.Now().UTC())
	if err != nil {
		slog.Error("create github app jwt", "error", err)
		return nil, fmt.Errorf("create github app jwt: %w", err)
	}

	installationToken, err := gh.CreateInstallationToken(ctx, appJWT, installationID)
	if err != nil {
		return nil, err
	}

	req, err := gh.NewRequest(ctx, http.MethodGet, gh.EndpointURL(
		fmt.Sprintf("/repos/%s/%s/tarball/%s", url.PathEscape(owner), url.PathEscape(name), url.PathEscape(sha)),
	), http.NoBody, installationToken)
	if err != nil {
		return nil, err
	}

	return gh.sendStreamRequest(req)
}

func (gh *GithubClient) NewRequest(ctx context.Context, method, url string, body io.Reader, token string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "prout")
	return req, nil
}

func (gh *GithubClient) EndpointURL(relativePath string) string {
	cleanPath := path.Clean("/" + strings.TrimSpace(relativePath))
	return strings.TrimRight(gh.cfg.GitHub.APIBaseURL, "/") + cleanPath
}

func (gh *GithubClient) CreateInstallationToken(ctx context.Context, appJWT string, installationID int64) (string, error) {
	req, err := gh.NewRequest(ctx, http.MethodPost, gh.EndpointURL(
		fmt.Sprintf("/app/installations/%d/access_tokens", installationID),
	), bytes.NewReader([]byte("{}")), appJWT)
	if err != nil {
		return "", err
	}

	var response struct {
		Token string `json:"token"`
	}
	if err := gh.SendRequest(req, &response); err != nil {
		return "", err
	}

	if strings.TrimSpace(response.Token) == "" {
		return "", errors.New("github access token response missing token")
	}
	return response.Token, nil
}

func (gh *GithubClient) SendRequest(req *http.Request, v any) error {
	httpClient := &http.Client{
		Timeout: gh.cfg.GitHub.APITimeout,
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		slog.Error("GitHub API request failed", "method", req.Method, "url", req.URL.Redacted(), "error", err)
		return fmt.Errorf("github request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if readErr != nil {
			return fmt.Errorf("read github error response: %w", readErr)
		}
		return logAndWrapGitHubAPIError(req, resp.StatusCode, body)
	}

	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("decode github response: %w", err)
	}
	return nil
}

func (gh *GithubClient) AppJWT(now time.Time) (string, error) {
	headerJSON, err := json.Marshal(map[string]string{
		"alg": "RS256",
		"typ": "JWT",
	})
	if err != nil {
		return "", err
	}

	claimsJSON, err := json.Marshal(map[string]any{
		"iat": now.Add(-60 * time.Second).Unix(),
		"exp": now.Add(9 * time.Minute).Unix(),
		"iss": gh.cfg.GitHub.AppID,
	})
	if err != nil {
		return "", err
	}

	header := base64.RawURLEncoding.EncodeToString(headerJSON)
	claims := base64.RawURLEncoding.EncodeToString(claimsJSON)
	signingInput := header + "." + claims

	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, gh.privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}

	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (gh *GithubClient) sendStreamRequest(req *http.Request) (io.ReadCloser, error) {
	httpClient := &http.Client{
		Timeout: gh.cfg.GitHub.APIStreamTimeout,
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		slog.Error("GitHub API request failed", "method", req.Method, "url", req.URL.Redacted(), "error", err)
		return nil, fmt.Errorf("github request failed: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if readErr != nil {
			return nil, fmt.Errorf("read github error response: %w", readErr)
		}
		return nil, logAndWrapGitHubAPIError(req, resp.StatusCode, body)
	}

	return resp.Body, nil
}
