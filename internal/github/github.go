package github

import (
	"fmt"
	"log/slog"

	"github.com/dominikpalatynski/toolshed/internal/config"
	"github.com/dominikpalatynski/toolshed/internal/queue"
	"github.com/dominikpalatynski/toolshed/internal/workspace"
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

type GithubEventHandler struct {
	cfg              *config.Config
	workspaceHandler *workspace.WorkspaceHandler
	queue            *queue.Queue
}

func NewGithubEventHandler(cfg *config.Config) (*GithubEventHandler, error) {

	workspaceHandler, err := workspace.NewWorkspaceHandler(cfg)
	if err != nil {
		return nil, fmt.Errorf("create workspace handler: %w", err)
	}

	q := queue.New(100)
	q.Start(3)

	return &GithubEventHandler{
		cfg:              cfg,
		workspaceHandler: workspaceHandler,
		queue:            q,
	}, nil
}

func (h *GithubEventHandler) HandleGithubEvent(payload PullRequestWebhookPayload) error {
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

	h.queue.Run(func() error {
		return h.workspaceHandler.HandleDeleteWorkspace(workspace.WorkspaceLocationBuilder{
			FullName: payload.Repository.FullName,
			PRNumber: payload.Number,
			SHA:      payload.PullRequest.Head.SHA,
		})
	})

	return nil
}
