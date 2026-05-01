package triggers

import (
	"encoding/json"
	"testing"

	"github.com/dominikpalatynski/toolshed/internal/store/sqlc"
	"github.com/dominikpalatynski/toolshed/internal/webhook"
)

func TestValidateAndNormalizePullRequestLabel(t *testing.T) {
	t.Parallel()

	catalog := NewCatalog()
	trigger, err := catalog.ValidateAndNormalize(TypePullRequestLabel, json.RawMessage(`{"label":"preview"}`))
	if err != nil {
		t.Fatalf("ValidateAndNormalize() error = %v", err)
	}

	if trigger.EventFamily != webhook.EventTypePullRequestLabeled {
		t.Fatalf("ValidateAndNormalize() EventFamily = %q, want %q", trigger.EventFamily, webhook.EventTypePullRequestLabeled)
	}
	if trigger.IdentityKey != "pull_request_label:preview" {
		t.Fatalf("ValidateAndNormalize() IdentityKey = %q, want %q", trigger.IdentityKey, "pull_request_label:preview")
	}
}

func TestEvaluatePullRequestLabelMatch(t *testing.T) {
	t.Parallel()

	catalog := NewCatalog()
	evaluation, err := catalog.Evaluate(sqlc.RepositoryTriggers{
		ID:           7,
		RepositoryID: 3,
		Type:         TypePullRequestLabel,
		EventFamily:  webhook.EventTypePullRequestLabeled,
		IdentityKey:  "pull_request_label:preview",
		ConfigJson:   []byte(`{"label":"preview"}`),
		Enabled:      true,
	}, "delivery-1", webhook.NormalizedEvent{
		Type:               webhook.EventTypePullRequestLabeled,
		GithubRepositoryID: 123456,
		PRNumber:           42,
		PRHeadSHA:          "abc123",
		Label:              "preview",
	})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	if !evaluation.Matched {
		t.Fatalf("Evaluate() Matched = false, want true")
	}
	if evaluation.OperationIntent == nil {
		t.Fatalf("Evaluate() OperationIntent = nil, want non-nil")
	}
}

func TestEvaluateCommentCommandMismatch(t *testing.T) {
	t.Parallel()

	catalog := NewCatalog()
	evaluation, err := catalog.Evaluate(sqlc.RepositoryTriggers{
		ID:           8,
		RepositoryID: 3,
		Type:         TypePullRequestCommentCommand,
		EventFamily:  webhook.EventTypeIssueCommentCreated,
		IdentityKey:  "pull_request_comment_command:exact_first_line:/deploy",
		ConfigJson:   []byte(`{"matcher":"exact_first_line","command":"/deploy"}`),
		Enabled:      true,
	}, "delivery-2", webhook.NormalizedEvent{
		Type:               webhook.EventTypeIssueCommentCreated,
		GithubRepositoryID: 123456,
		PRNumber:           42,
		CommentBody:        "/deploy now",
		CommentFirstLine:   "/deploy now",
	})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	if evaluation.Matched {
		t.Fatalf("Evaluate() Matched = true, want false")
	}
	if evaluation.Reason != "comment_command_mismatch" {
		t.Fatalf("Evaluate() Reason = %q, want %q", evaluation.Reason, "comment_command_mismatch")
	}
}
