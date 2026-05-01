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
)

const (
	EventTypePullRequestOpened   = "pull_request.opened"
	EventTypePullRequestLabeled  = "pull_request.labeled"
	EventTypeIssueCommentCreated = "issue_comment.created"
)

type Delivery struct {
	DeliveryID         string
	GithubEvent        string
	EventType          string
	GithubRepositoryID *int64
	PayloadJSON        json.RawMessage
	Supported          bool
	Event              NormalizedEvent
}

type NormalizedEvent struct {
	Type               string `json:"type"`
	GithubRepositoryID int64  `json:"github_repository_id"`
	PRNumber           int    `json:"pr_number"`
	PRHeadSHA          string `json:"pr_head_sha,omitempty"`
	Label              string `json:"label,omitempty"`
	CommentID          int64  `json:"comment_id,omitempty"`
	CommentBody        string `json:"comment_body,omitempty"`
	CommentFirstLine   string `json:"comment_first_line,omitempty"`
	CommentAuthorLogin string `json:"comment_author_login,omitempty"`
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

	var payload githubPayload
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
		DeliveryID:  deliveryHeader,
		GithubEvent: eventHeader,
		EventType:   classifyEventType(eventHeader, payload.Action),
		PayloadJSON: append(json.RawMessage(nil), body...),
	}
	if payload.Repository.ID > 0 {
		delivery.GithubRepositoryID = int64Ptr(payload.Repository.ID)
	}

	switch delivery.EventType {
	case EventTypePullRequestOpened:
		event, err := normalizePullRequestEvent(payload, delivery.EventType, false)
		if err != nil {
			return delivery, err
		}
		delivery.Supported = true
		delivery.Event = event
	case EventTypePullRequestLabeled:
		event, err := normalizePullRequestEvent(payload, delivery.EventType, true)
		if err != nil {
			return delivery, err
		}
		delivery.Supported = true
		delivery.Event = event
	case EventTypeIssueCommentCreated:
		if payload.Issue.PullRequest == nil {
			return delivery, nil
		}
		event, err := normalizeIssueCommentEvent(payload)
		if err != nil {
			return delivery, err
		}
		delivery.Supported = true
		delivery.Event = event
	}

	return delivery, nil
}

type githubPayload struct {
	Action     string `json:"action"`
	Number     int    `json:"number"`
	Repository struct {
		ID int64 `json:"id"`
	} `json:"repository"`
	PullRequest struct {
		Number int `json:"number"`
		Head   struct {
			SHA string `json:"sha"`
		} `json:"head"`
	} `json:"pull_request"`
	Label struct {
		Name string `json:"name"`
	} `json:"label"`
	Issue struct {
		Number      int `json:"number"`
		PullRequest *struct {
			URL string `json:"url"`
		} `json:"pull_request"`
	} `json:"issue"`
	Comment struct {
		ID   int64  `json:"id"`
		Body string `json:"body"`
		User struct {
			Login string `json:"login"`
		} `json:"user"`
	} `json:"comment"`
}

func classifyEventType(eventHeader, action string) string {
	eventType := strings.TrimSpace(eventHeader)
	action = strings.TrimSpace(action)
	if action == "" {
		return eventType
	}
	return eventType + "." + action
}

func normalizePullRequestEvent(payload githubPayload, eventType string, requireLabel bool) (NormalizedEvent, error) {
	if payload.Repository.ID <= 0 {
		return NormalizedEvent{}, errors.New("webhook payload missing repository.id")
	}

	prNumber := payload.Number
	if prNumber == 0 {
		prNumber = payload.PullRequest.Number
	}
	if prNumber <= 0 {
		return NormalizedEvent{}, errors.New("webhook payload missing pull request number")
	}

	headSHA := strings.TrimSpace(payload.PullRequest.Head.SHA)
	if headSHA == "" {
		return NormalizedEvent{}, errors.New("webhook payload missing pull_request.head.sha")
	}

	event := NormalizedEvent{
		Type:               eventType,
		GithubRepositoryID: payload.Repository.ID,
		PRNumber:           prNumber,
		PRHeadSHA:          headSHA,
	}

	if requireLabel {
		label := strings.TrimSpace(payload.Label.Name)
		if label == "" {
			return NormalizedEvent{}, errors.New("webhook payload missing label.name")
		}
		event.Label = label
	}

	return event, nil
}

func normalizeIssueCommentEvent(payload githubPayload) (NormalizedEvent, error) {
	if payload.Repository.ID <= 0 {
		return NormalizedEvent{}, errors.New("webhook payload missing repository.id")
	}
	if payload.Issue.Number <= 0 {
		return NormalizedEvent{}, errors.New("webhook payload missing issue.number")
	}

	body := payload.Comment.Body
	return NormalizedEvent{
		Type:               EventTypeIssueCommentCreated,
		GithubRepositoryID: payload.Repository.ID,
		PRNumber:           payload.Issue.Number,
		CommentID:          payload.Comment.ID,
		CommentBody:        body,
		CommentFirstLine:   firstLine(body),
		CommentAuthorLogin: strings.TrimSpace(payload.Comment.User.Login),
	}, nil
}

func firstLine(body string) string {
	line, _, _ := strings.Cut(body, "\n")
	return strings.TrimSuffix(line, "\r")
}

func int64Ptr(v int64) *int64 {
	return &v
}
