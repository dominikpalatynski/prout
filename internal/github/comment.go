package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type GithubCommentPayload struct {
	PRNumber int
	SHA      string
}

func (gh *GithubClient) SendPRComment(ghCommentPayload GithubCommentPayload, message string) error {
	if ghCommentPayload.PRNumber <= 0 {
		return fmt.Errorf("pull request number must be positive")
	}
	if strings.TrimSpace(message) == "" {
		return fmt.Errorf("comment message is required")
	}

	ctx := context.Background()
	owner := gh.cfg.GitHub.Repository.Owner
	name := gh.cfg.GitHub.Repository.Name

	appJWT, err := gh.AppJWT(time.Now().UTC())
	if err != nil {
		return fmt.Errorf("create github app jwt: %w", err)
	}

	installationID, err := gh.GetInstallationID(ctx, appJWT, owner, name)
	if err != nil {
		slog.Error("get github installation id", "owner", owner, "repo", name, "error", err)
		return fmt.Errorf("get github installation id: %w", err)
	}

	installationToken, err := gh.CreateInstallationToken(ctx, appJWT, installationID)
	if err != nil {
		return fmt.Errorf("create github installation token: %w", err)
	}

	payload, err := json.Marshal(map[string]string{"body": message})
	if err != nil {
		return fmt.Errorf("marshal pr comment body: %w", err)
	}

	req, err := gh.NewRequest(ctx, http.MethodPost, gh.EndpointURL(
		fmt.Sprintf("/repos/%s/%s/issues/%d/comments",
			url.PathEscape(owner), url.PathEscape(name), ghCommentPayload.PRNumber),
	), bytes.NewReader(payload), installationToken)
	if err != nil {
		return fmt.Errorf("build pr comment request: %w", err)
	}

	var response struct {
		ID int64 `json:"id"`
	}
	if err := gh.SendRequest(req, &response); err != nil {
		return fmt.Errorf("post pr comment: %w", err)
	}

	slog.Info("Posted PR comment", "owner", owner, "repo", name, "pr", ghCommentPayload.PRNumber, "comment_id", response.ID)
	return nil
}
