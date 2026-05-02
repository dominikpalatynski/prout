package webhook

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/dominikpalatynski/toolshed/internal/pullrequests"
)

const (
	EventTypePullRequestOpened   = "pull_request.opened"
	EventTypePullRequestLabeled  = "pull_request.labeled"
	EventTypeIssueCommentCreated = "issue_comment.created"
)

type Delivery struct {
	DeliveryID         string
	GithubEvent        string
	GithubAction       string
	EventType          string
	GithubRepositoryID *int64
	PayloadJSON        json.RawMessage
	Payload            GitHubPayload
}

type GitHubPayload struct {
	Action      string                   `json:"action"`
	Number      int                      `json:"number"`
	Repository  GitHubRepositoryPayload  `json:"repository"`
	PullRequest GitHubPullRequestPayload `json:"pull_request"`
	Label       GitHubLabelPayload       `json:"label"`
	Issue       GitHubIssuePayload       `json:"issue"`
	Comment     GitHubCommentPayload     `json:"comment"`
}

type GitHubRepositoryPayload struct {
	ID       int64             `json:"id"`
	Name     string            `json:"name"`
	FullName string            `json:"full_name"`
	Owner    GitHubUserPayload `json:"owner"`
}

type GitHubUserPayload struct {
	Login string `json:"login"`
}

type GitHubPullRequestPayload struct {
	ID     int64                    `json:"id"`
	Number int                      `json:"number"`
	Head   GitHubPullRequestHeadRef `json:"head"`
}

type GitHubPullRequestHeadRef struct {
	SHA  string                  `json:"sha"`
	Repo GitHubRepositoryPayload `json:"repo"`
}

type GitHubLabelPayload struct {
	Name string `json:"name"`
}

type GitHubIssuePayload struct {
	Number      int                           `json:"number"`
	PullRequest *GitHubPullRequestLinkPayload `json:"pull_request"`
}

type GitHubPullRequestLinkPayload struct {
	URL string `json:"url"`
}

type GitHubCommentPayload struct {
	ID   int64             `json:"id"`
	Body string            `json:"body"`
	User GitHubUserPayload `json:"user"`
}

type NormalizedEvent struct {
	Type                string                        `json:"type"`
	GithubRepositoryID  int64                         `json:"github_repository_id"`
	PRNumber            int                           `json:"pr_number"`
	GithubPullRequestID int64                         `json:"github_pull_request_id,omitempty"`
	PRHeadSHA           string                        `json:"pr_head_sha,omitempty"`
	PRSourceRepository  pullrequests.SourceRepository `json:"pr_source_repository,omitempty"`
	Label               string                        `json:"label,omitempty"`
	CommentID           int64                         `json:"comment_id,omitempty"`
	CommentBody         string                        `json:"comment_body,omitempty"`
	CommentFirstLine    string                        `json:"comment_first_line,omitempty"`
	CommentAuthorLogin  string                        `json:"comment_author_login,omitempty"`
}

func VerifySignature(secret, signatureHeader string, body []byte) error {
	signatureHeader = strings.TrimSpace(signatureHeader)
	if signatureHeader == "" {
		return errors.New("missing X-Hub-Signature-256 header")
	}

	const prefix = "sha256="
	if !strings.HasPrefix(signatureHeader, prefix) {
		return errors.New("X-Hub-Signature-256 must use sha256")
	}

	provided, err := hex.DecodeString(strings.TrimPrefix(signatureHeader, prefix))
	if err != nil {
		return fmt.Errorf("decode X-Hub-Signature-256: %w", err)
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := mac.Sum(nil)

	if !hmac.Equal(provided, expected) {
		return errors.New("github webhook signature mismatch")
	}

	return nil
}

func ParseDelivery(eventHeader, deliveryHeader string, body []byte) (Delivery, error) {
	eventHeader = strings.TrimSpace(eventHeader)
	deliveryHeader = strings.TrimSpace(deliveryHeader)

	if eventHeader == "" {
		return Delivery{}, errors.New("missing X-GitHub-Event header")
	}
	if deliveryHeader == "" {
		return Delivery{}, errors.New("missing X-GitHub-Delivery header")
	}
	if !json.Valid(body) {
		return Delivery{}, errors.New("webhook payload must be valid JSON")
	}

	var payload GitHubPayload
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(&payload); err != nil {
		return Delivery{}, fmt.Errorf("decode webhook payload: %w", err)
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err == nil {
		return Delivery{}, errors.New("webhook payload must contain exactly one JSON object")
	} else if !errors.Is(err, io.EOF) {
		return Delivery{}, errors.New("webhook payload must contain exactly one JSON object")
	}

	delivery := Delivery{
		DeliveryID:   deliveryHeader,
		GithubEvent:  eventHeader,
		GithubAction: strings.TrimSpace(payload.Action),
		EventType:    classifyEventType(eventHeader, payload.Action),
		PayloadJSON:  append(json.RawMessage(nil), body...),
		Payload:      payload,
	}
	if payload.Repository.ID > 0 {
		delivery.GithubRepositoryID = int64Ptr(payload.Repository.ID)
	}

	return delivery, nil
}

func classifyEventType(eventHeader, action string) string {
	eventType := strings.TrimSpace(eventHeader)
	action = strings.TrimSpace(action)
	if action == "" {
		return eventType
	}
	return eventType + "." + action
}

func int64Ptr(v int64) *int64 {
	return &v
}
