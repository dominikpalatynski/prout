package event

import (
	"fmt"
	"log/slog"

	"github.com/dominikpalatynski/prout/internal/config"
	"github.com/dominikpalatynski/prout/internal/github"
	"github.com/dominikpalatynski/prout/internal/queue"
	"github.com/dominikpalatynski/prout/internal/workspace"
)

type PullRequestWebhookPayload struct {
	Action string `json:"action"`
	Number int    `json:"number"`

	PullRequest struct {
		Head struct {
			SHA string `json:"sha"`
			Ref string `json:"ref"`
		} `json:"head"`
	} `json:"pull_request"`

	Label struct {
		Name string `json:"name"`
	} `json:"label"`

	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`

	Sender struct {
		Login string `json:"login"`
	} `json:"sender"`

	Installation struct {
		ID int64 `json:"id"`
	} `json:"installation"`
}

type GithubWebhookRequest struct {
	Body      []byte
	Signature string
	Payload   PullRequestWebhookPayload
}

type githubWebhookSignatureVerifier interface {
	VerifyWebhookSignature(body []byte, signatureHeader string) error
	ValidateRepositoryActionPermission(action github.RepositoryAction, sender string) error
}

type workspaceHandler interface {
	HandleCreateWorkspace(location workspace.WorkspaceLocationBuilder) error
	HandleDeleteWorkspace(location workspace.WorkspaceLocationBuilder) error
}

type GithubEventHandler struct {
	cfg               *config.Config
	workspaceHandler  workspaceHandler
	signatureVerifier githubWebhookSignatureVerifier
	queue             *queue.Queue
}

func NewGithubEventHandler(cfg *config.Config, workspaceHandler workspaceHandler, signatureVerifier githubWebhookSignatureVerifier) (*GithubEventHandler, error) {
	if workspaceHandler == nil {
		return nil, fmt.Errorf("workspace handler is required")
	}
	if signatureVerifier == nil {
		return nil, fmt.Errorf("github webhook signature verifier is required")
	}

	q := queue.New(100)
	q.Start(3)

	return &GithubEventHandler{
		cfg:               cfg,
		workspaceHandler:  workspaceHandler,
		signatureVerifier: signatureVerifier,
		queue:             q,
	}, nil
}

func (h *GithubEventHandler) HandleGithubEvent(webhook GithubWebhookRequest) error {
	if err := h.signatureVerifier.VerifyWebhookSignature(webhook.Body, webhook.Signature); err != nil {
		slog.Error("GitHub webhook signature verification failed", "error", err)
		return fmt.Errorf("github webhook signature verification failed: %w", err)
	}

	payload := webhook.Payload

	if fmt.Sprintf("%s/%s", h.cfg.GitHub.Repository.Owner, h.cfg.GitHub.Repository.Name) != payload.Repository.FullName {
		return fmt.Errorf("unexpected repository: %s", payload.Repository.FullName)
	}

	if payload.Action == "labeled" {
		return h.HandlePreviewLabeled(payload)
	}

	if payload.Action == "unlabeled" {
		return h.HandlePreviewUnlabeled(payload)
	}

	slog.Error("Received unsupported GitHub event", "action", payload.Action)
	return fmt.Errorf("unexpected action: %s", payload.Action)
}

func (h *GithubEventHandler) HandlePreviewLabeled(payload PullRequestWebhookPayload) error {
	if payload.Label.Name != "preview" {
		return fmt.Errorf("unexpected label: %s", payload.Label.Name)
	}
	if err := h.signatureVerifier.ValidateRepositoryActionPermission(github.RepositoryActionPreviewLabel, payload.Sender.Login); err != nil {
		return fmt.Errorf("validate sender permissions: %w", err)
	}

	h.queue.Run(func() error {
		return h.workspaceHandler.HandleCreateWorkspace(workspace.WorkspaceLocationBuilder{
			FullName: payload.Repository.FullName,
			PRNumber: payload.Number,
			SHA:      payload.PullRequest.Head.SHA,
		})
	})

	return nil
}

func (h *GithubEventHandler) HandlePreviewUnlabeled(payload PullRequestWebhookPayload) error {
	if payload.Label.Name != "preview" {
		return fmt.Errorf("unexpected label: %s", payload.Label.Name)
	}
	if err := h.signatureVerifier.ValidateRepositoryActionPermission(github.RepositoryActionPreviewUnlabel, payload.Sender.Login); err != nil {
		return fmt.Errorf("validate sender permissions: %w", err)
	}

	h.queue.Run(func() error {
		return h.workspaceHandler.HandleDeleteWorkspace(workspace.WorkspaceLocationBuilder{
			FullName: payload.Repository.FullName,
			PRNumber: payload.Number,
			SHA:      payload.PullRequest.Head.SHA,
		})
	})

	return nil
}
