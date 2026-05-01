package githubapp

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
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/dominikpalatynski/toolshed/internal/config"
)

type Repository struct {
	GithubRepositoryID   int64
	GithubInstallationID int64
	Owner                string
	Name                 string
	FullName             string
	HTMLURL              string
	IsPrivate            bool
}

type PullRequest struct {
	GithubPullRequestID int64
	Number              int
	HeadSHA             string
}

type Resolver interface {
	ResolveRepository(ctx context.Context, fullName string) (Repository, error)
	ResolvePullRequest(ctx context.Context, owner, name string, installationID int64, number int) (PullRequest, error)
}

type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("github api returned %d: %s", e.StatusCode, e.Message)
}

type Client struct {
	appID      int64
	baseURL    *url.URL
	httpClient *http.Client
	privateKey *rsa.PrivateKey
}

func New(cfg config.GitHubConfig) (*Client, error) {
	privateKeyPEM, err := os.ReadFile(cfg.PrivateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("read github app private key: %w", err)
	}

	privateKey, err := parsePrivateKey(privateKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("parse github app private key: %w", err)
	}

	baseURL, err := url.Parse(strings.TrimSpace(cfg.APIBaseURL))
	if err != nil {
		return nil, fmt.Errorf("parse github api base url: %w", err)
	}

	return &Client{
		appID:   cfg.AppID,
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		privateKey: privateKey,
	}, nil
}

func (c *Client) ResolveRepository(ctx context.Context, fullName string) (Repository, error) {
	owner, name, err := parseFullName(fullName)
	if err != nil {
		return Repository{}, err
	}

	appJWT, err := c.appJWT(time.Now().UTC())
	if err != nil {
		return Repository{}, fmt.Errorf("create github app jwt: %w", err)
	}

	installationID, err := c.getInstallationID(ctx, appJWT, owner, name)
	if err != nil {
		return Repository{}, err
	}

	installationToken, err := c.createInstallationToken(ctx, appJWT, installationID)
	if err != nil {
		return Repository{}, err
	}

	repo, err := c.getRepository(ctx, installationToken, owner, name)
	if err != nil {
		return Repository{}, err
	}

	repo.GithubInstallationID = installationID
	return repo, nil
}

func (c *Client) ResolvePullRequest(ctx context.Context, owner, name string, installationID int64, number int) (PullRequest, error) {
	if strings.TrimSpace(owner) == "" || strings.TrimSpace(name) == "" {
		return PullRequest{}, fmt.Errorf("repository owner and name are required")
	}
	if installationID <= 0 {
		return PullRequest{}, fmt.Errorf("github installation id must be positive")
	}
	if number <= 0 {
		return PullRequest{}, fmt.Errorf("pull request number must be positive")
	}

	appJWT, err := c.appJWT(time.Now().UTC())
	if err != nil {
		return PullRequest{}, fmt.Errorf("create github app jwt: %w", err)
	}

	installationToken, err := c.createInstallationToken(ctx, appJWT, installationID)
	if err != nil {
		return PullRequest{}, err
	}

	return c.getPullRequest(ctx, installationToken, owner, name, number)
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

func parseFullName(fullName string) (string, string, error) {
	trimmed := strings.TrimSpace(fullName)
	owner, name, ok := strings.Cut(trimmed, "/")
	if !ok || owner == "" || name == "" {
		return "", "", fmt.Errorf("repository full_name must be in owner/name form")
	}
	return owner, name, nil
}

func (c *Client) appJWT(now time.Time) (string, error) {
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
		"iss": c.appID,
	})
	if err != nil {
		return "", err
	}

	header := base64.RawURLEncoding.EncodeToString(headerJSON)
	claims := base64.RawURLEncoding.EncodeToString(claimsJSON)
	signingInput := header + "." + claims

	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, c.privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}

	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (c *Client) getInstallationID(ctx context.Context, appJWT, owner, name string) (int64, error) {
	req, err := c.newRequest(ctx, http.MethodGet, c.endpointURL(
		fmt.Sprintf("/repos/%s/%s/installation", url.PathEscape(owner), url.PathEscape(name)),
	), bytes.NewReader(nil), appJWT)
	if err != nil {
		return 0, err
	}

	var response struct {
		ID int64 `json:"id"`
	}
	if err := c.doJSON(req, &response); err != nil {
		return 0, err
	}
	if response.ID <= 0 {
		return 0, errors.New("github installation response missing id")
	}
	return response.ID, nil
}

func (c *Client) createInstallationToken(ctx context.Context, appJWT string, installationID int64) (string, error) {
	req, err := c.newRequest(ctx, http.MethodPost, c.endpointURL(
		fmt.Sprintf("/app/installations/%d/access_tokens", installationID),
	), bytes.NewReader([]byte("{}")), appJWT)
	if err != nil {
		return "", err
	}

	var response struct {
		Token string `json:"token"`
	}
	if err := c.doJSON(req, &response); err != nil {
		return "", err
	}
	if strings.TrimSpace(response.Token) == "" {
		return "", errors.New("github access token response missing token")
	}
	return response.Token, nil
}

func (c *Client) getRepository(ctx context.Context, installationToken, owner, name string) (Repository, error) {
	req, err := c.newRequest(ctx, http.MethodGet, c.endpointURL(
		fmt.Sprintf("/repos/%s/%s", url.PathEscape(owner), url.PathEscape(name)),
	), bytes.NewReader(nil), installationToken)
	if err != nil {
		return Repository{}, err
	}

	var response struct {
		ID       int64  `json:"id"`
		Name     string `json:"name"`
		FullName string `json:"full_name"`
		HTMLURL  string `json:"html_url"`
		Private  bool   `json:"private"`
		Owner    struct {
			Login string `json:"login"`
		} `json:"owner"`
	}
	if err := c.doJSON(req, &response); err != nil {
		return Repository{}, err
	}
	if response.ID <= 0 {
		return Repository{}, errors.New("github repository response missing id")
	}

	fullName := strings.TrimSpace(response.FullName)
	if fullName == "" {
		fullName = strings.TrimSpace(response.Owner.Login) + "/" + strings.TrimSpace(response.Name)
	}

	return Repository{
		GithubRepositoryID: response.ID,
		Owner:              strings.TrimSpace(response.Owner.Login),
		Name:               strings.TrimSpace(response.Name),
		FullName:           fullName,
		HTMLURL:            strings.TrimSpace(response.HTMLURL),
		IsPrivate:          response.Private,
	}, nil
}

func (c *Client) getPullRequest(ctx context.Context, installationToken, owner, name string, number int) (PullRequest, error) {
	req, err := c.newRequest(ctx, http.MethodGet, c.endpointURL(
		fmt.Sprintf("/repos/%s/%s/pulls/%d", url.PathEscape(owner), url.PathEscape(name), number),
	), bytes.NewReader(nil), installationToken)
	if err != nil {
		return PullRequest{}, err
	}

	var response struct {
		ID     int64 `json:"id"`
		Number int   `json:"number"`
		Head   struct {
			SHA string `json:"sha"`
		} `json:"head"`
	}
	if err := c.doJSON(req, &response); err != nil {
		return PullRequest{}, err
	}
	if response.ID <= 0 {
		return PullRequest{}, errors.New("github pull request response missing id")
	}
	if response.Number <= 0 {
		return PullRequest{}, errors.New("github pull request response missing number")
	}
	if strings.TrimSpace(response.Head.SHA) == "" {
		return PullRequest{}, errors.New("github pull request response missing head.sha")
	}

	return PullRequest{
		GithubPullRequestID: response.ID,
		Number:              response.Number,
		HeadSHA:             strings.TrimSpace(response.Head.SHA),
	}, nil
}

func (c *Client) newRequest(ctx context.Context, method, url string, body io.Reader, token string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "toolshed")
	return req, nil
}

func (c *Client) doJSON(req *http.Request, dst any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("github request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if readErr != nil {
			return fmt.Errorf("read github error response: %w", readErr)
		}
		return &APIError{
			StatusCode: resp.StatusCode,
			Message:    strings.TrimSpace(string(body)),
		}
	}

	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return fmt.Errorf("decode github response: %w", err)
	}
	return nil
}

func (c *Client) endpointURL(relativePath string) string {
	u := *c.baseURL
	u.Path = path.Join(c.baseURL.Path, relativePath)
	return u.String()
}
