package workspace

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

func (w *WorkspaceHandler) SendPRComment(location WorkspaceLocationBuilder, message string) error {
	if location.PRNumber <= 0 {
		return fmt.Errorf("pull request number must be positive")
	}
	if strings.TrimSpace(message) == "" {
		return fmt.Errorf("comment message is required")
	}

	ctx := context.Background()
	owner := w.cfg.GitHub.Repository.Owner
	name := w.cfg.GitHub.Repository.Name

	appJWT, err := w.appJWT(time.Now().UTC())
	if err != nil {
		return fmt.Errorf("create github app jwt: %w", err)
	}

	installationID, err := w.getInstallationID(ctx, appJWT, owner, name)
	if err != nil {
		slog.Error("get github installation id", "owner", owner, "repo", name, "error", err)
		return fmt.Errorf("get github installation id: %w", err)
	}

	installationToken, err := w.createInstallationToken(ctx, appJWT, installationID)
	if err != nil {
		return fmt.Errorf("create github installation token: %w", err)
	}

	payload, err := json.Marshal(map[string]string{"body": message})
	if err != nil {
		return fmt.Errorf("marshal pr comment body: %w", err)
	}

	req, err := w.newRequest(ctx, http.MethodPost, w.endpointURL(
		fmt.Sprintf("/repos/%s/%s/issues/%d/comments",
			url.PathEscape(owner), url.PathEscape(name), location.PRNumber),
	), bytes.NewReader(payload), installationToken)
	if err != nil {
		return fmt.Errorf("build pr comment request: %w", err)
	}

	var response struct {
		ID int64 `json:"id"`
	}
	if err := w.sendRequest(req, &response); err != nil {
		return fmt.Errorf("post pr comment: %w", err)
	}

	slog.Info("Posted PR comment", "owner", owner, "repo", name, "pr", location.PRNumber, "comment_id", response.ID)
	return nil
}
