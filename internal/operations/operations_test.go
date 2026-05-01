package operations

import (
	"encoding/json"
	"testing"

	"github.com/dominikpalatynski/toolshed/internal/webhook"
)

func TestTypeForTriggerMapsCurrentTriggerTypesToPreviewStart(t *testing.T) {
	t.Parallel()

	triggerTypes := []string{
		"pull_request_opened",
		"pull_request_label",
		"pull_request_comment_command",
	}

	for _, triggerType := range triggerTypes {
		triggerType := triggerType
		t.Run(triggerType, func(t *testing.T) {
			t.Parallel()

			operationType, err := TypeForTrigger(triggerType)
			if err != nil {
				t.Fatalf("TypeForTrigger() error = %v", err)
			}
			if operationType != TypePreviewStart {
				t.Fatalf("TypeForTrigger() = %q, want %q", operationType, TypePreviewStart)
			}
		})
	}
}

func TestBuildTriggerSnapshotFreezesTargetCommit(t *testing.T) {
	t.Parallel()

	githubPullRequestID := int64(9988)
	snapshotJSON, err := BuildTriggerSnapshot(TriggerSnapshotInput{
		RepositoryID:           7,
		PullRequestID:          13,
		PRNumber:               42,
		GithubPullRequestID:    &githubPullRequestID,
		DeliveryID:             "delivery-1",
		Event:                  webhook.NormalizedEvent{Type: webhook.EventTypePullRequestOpened, PRNumber: 42, PRHeadSHA: "abc123"},
		TriggerID:              5,
		TriggerType:            "pull_request_opened",
		TriggerIdentityKey:     "pull_request_opened",
		OperationType:          TypePreviewStart,
		RuntimeEnvironmentType: RuntimeEnvironmentTypePreview,
		TargetPRHeadCommitSHA:  "abc123",
	})
	if err != nil {
		t.Fatalf("BuildTriggerSnapshot() error = %v", err)
	}

	var snapshot struct {
		Target struct {
			OperationType          string `json:"operation_type"`
			RuntimeEnvironmentType string `json:"runtime_environment_type"`
			TargetPRHeadCommitSHA  string `json:"target_pr_head_commit_sha"`
			PRNumber               int64  `json:"pr_number"`
		} `json:"target"`
		Delivery struct {
			ID    string                  `json:"id"`
			Event webhook.NormalizedEvent `json:"event"`
		} `json:"delivery"`
	}
	if err := json.Unmarshal(snapshotJSON, &snapshot); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if snapshot.Target.OperationType != TypePreviewStart {
		t.Fatalf("snapshot target operation_type = %q, want %q", snapshot.Target.OperationType, TypePreviewStart)
	}
	if snapshot.Target.RuntimeEnvironmentType != RuntimeEnvironmentTypePreview {
		t.Fatalf("snapshot target runtime_environment_type = %q, want %q", snapshot.Target.RuntimeEnvironmentType, RuntimeEnvironmentTypePreview)
	}
	if snapshot.Target.TargetPRHeadCommitSHA != "abc123" {
		t.Fatalf("snapshot target_pr_head_commit_sha = %q, want %q", snapshot.Target.TargetPRHeadCommitSHA, "abc123")
	}
	if snapshot.Delivery.ID != "delivery-1" {
		t.Fatalf("snapshot delivery id = %q, want %q", snapshot.Delivery.ID, "delivery-1")
	}
}
