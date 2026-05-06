package event

import (
	"errors"
	"testing"
	"time"

	"github.com/dominikpalatynski/prout/internal/config"
	gh "github.com/dominikpalatynski/prout/internal/github"
	"github.com/dominikpalatynski/prout/internal/workspace"
)

func TestHandlePreviewLabeledValidatesSenderPermissions(t *testing.T) {
	t.Parallel()

	workspaceHandler := &stubWorkspaceHandler{
		createCalls: make(chan workspace.WorkspaceLocationBuilder, 1),
	}
	signatureVerifier := &stubGitHubVerifier{}

	handler, err := NewGithubEventHandler(testConfig(), workspaceHandler, signatureVerifier)
	if err != nil {
		t.Fatalf("NewGithubEventHandler() error = %v", err)
	}
	t.Cleanup(handler.queue.Stop)

	payload := PullRequestWebhookPayload{
		Number: 42,
		Label: struct {
			Name string `json:"name"`
		}{Name: "preview"},
		Repository: struct {
			FullName string `json:"full_name"`
		}{FullName: "owner/repo"},
		Sender: struct {
			Login string `json:"login"`
		}{Login: "alice"},
	}
	payload.PullRequest.Head.SHA = "abcdef1234567890"

	if err := handler.HandlePreviewLabeled(payload); err != nil {
		t.Fatalf("HandlePreviewLabeled() error = %v", err)
	}

	if signatureVerifier.action != gh.RepositoryActionPreviewLabel {
		t.Fatalf("ValidateRepositoryActionPermission() action = %q, want %q", signatureVerifier.action, gh.RepositoryActionPreviewLabel)
	}
	if signatureVerifier.sender != "alice" {
		t.Fatalf("ValidateRepositoryActionPermission() sender = %q, want %q", signatureVerifier.sender, "alice")
	}

	select {
	case got := <-workspaceHandler.createCalls:
		if got.FullName != "owner/repo" || got.PRNumber != 42 || got.SHA != "abcdef1234567890" {
			t.Fatalf("HandleCreateWorkspace() got %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("HandleCreateWorkspace() was not called")
	}
}

func TestHandlePreviewUnlabeledReturnsPermissionError(t *testing.T) {
	t.Parallel()

	workspaceHandler := &stubWorkspaceHandler{
		deleteCalls: make(chan workspace.WorkspaceLocationBuilder, 1),
	}
	signatureVerifier := &stubGitHubVerifier{
		validateErr: errors.New("permission denied"),
	}

	handler, err := NewGithubEventHandler(testConfig(), workspaceHandler, signatureVerifier)
	if err != nil {
		t.Fatalf("NewGithubEventHandler() error = %v", err)
	}
	t.Cleanup(handler.queue.Stop)

	payload := PullRequestWebhookPayload{
		Label: struct {
			Name string `json:"name"`
		}{Name: "preview"},
		Sender: struct {
			Login string `json:"login"`
		}{Login: "bob"},
	}

	err = handler.HandlePreviewUnlabeled(payload)
	if err == nil {
		t.Fatal("HandlePreviewUnlabeled() error = nil, want permission error")
	}
	if err.Error() != "validate sender permissions: permission denied" {
		t.Fatalf("HandlePreviewUnlabeled() error = %q", err.Error())
	}

	select {
	case got := <-workspaceHandler.deleteCalls:
		t.Fatalf("HandleDeleteWorkspace() unexpectedly called with %+v", got)
	default:
	}
}

type stubWorkspaceHandler struct {
	createCalls chan workspace.WorkspaceLocationBuilder
	deleteCalls chan workspace.WorkspaceLocationBuilder
}

func (s *stubWorkspaceHandler) HandleCreateWorkspace(location workspace.WorkspaceLocationBuilder) error {
	if s.createCalls != nil {
		s.createCalls <- location
	}
	return nil
}

func (s *stubWorkspaceHandler) HandleDeleteWorkspace(location workspace.WorkspaceLocationBuilder) error {
	if s.deleteCalls != nil {
		s.deleteCalls <- location
	}
	return nil
}

type stubGitHubVerifier struct {
	action      gh.RepositoryAction
	sender      string
	validateErr error
}

func (s *stubGitHubVerifier) VerifyWebhookSignature(_ []byte, _ string) error {
	return nil
}

func (s *stubGitHubVerifier) ValidateRepositoryActionPermission(action gh.RepositoryAction, sender string) error {
	s.action = action
	s.sender = sender
	return s.validateErr
}

func testConfig() *config.Config {
	return &config.Config{
		GitHub: config.GitHubConfig{
			Repository: config.RepositoryConfig{
				Owner: "owner",
				Name:  "repo",
			},
		},
	}
}
