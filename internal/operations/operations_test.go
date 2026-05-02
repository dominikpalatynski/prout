package operations_test

import (
	"encoding/json"
	"testing"

	"github.com/dominikpalatynski/toolshed/internal/automation"
	"github.com/dominikpalatynski/toolshed/internal/operations"
	"github.com/dominikpalatynski/toolshed/internal/pullrequests"
	"github.com/dominikpalatynski/toolshed/internal/webhook"
)

func TestBuildPreviewStartSnapshotFreezesTargetCommit(t *testing.T) {
	t.Parallel()

	githubPullRequestID := int64(9988)
	snapshotJSON, err := operations.BuildPreviewStartSnapshot(operations.PreviewStartSnapshotInput{
		RepositoryID:        7,
		PullRequestID:       13,
		PRNumber:            42,
		GithubPullRequestID: &githubPullRequestID,
		PRSourceRepository: pullrequests.SourceRepository{
			GithubRepositoryID: 101,
			Owner:              "acme",
			Name:               "demo",
			FullName:           "acme/demo",
		},
		DeliveryID: "delivery-1",
		Event: webhook.NormalizedEvent{
			Type:      webhook.EventTypePullRequestOpened,
			PRNumber:  42,
			PRHeadSHA: "abc123",
			PRSourceRepository: pullrequests.SourceRepository{
				GithubRepositoryID: 101,
				Owner:              "acme",
				Name:               "demo",
				FullName:           "acme/demo",
			},
		},
		TriggerID:              5,
		TriggerType:            automation.TriggerTypePreviewOnPullRequestOpened,
		OperationType:          operations.TypePreviewStart,
		RuntimeEnvironmentType: operations.RuntimeEnvironmentTypePreview,
		TargetPRHeadCommitSHA:  "abc123",
	})
	if err != nil {
		t.Fatalf("BuildPreviewStartSnapshot() error = %v", err)
	}

	var snapshot struct {
		Trigger struct {
			Type string `json:"type"`
		} `json:"trigger"`
		Target struct {
			OperationType          string `json:"operation_type"`
			RuntimeEnvironmentType string `json:"runtime_environment_type"`
			TargetPRHeadCommitSHA  string `json:"target_pr_head_commit_sha"`
			PRNumber               int64  `json:"pr_number"`
			PullRequestSourceRepo  struct {
				FullName string `json:"full_name"`
			} `json:"pull_request_source_repository"`
		} `json:"target"`
		Delivery struct {
			ID    string                  `json:"id"`
			Event webhook.NormalizedEvent `json:"event"`
		} `json:"delivery"`
	}
	if err := json.Unmarshal(snapshotJSON, &snapshot); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if snapshot.Trigger.Type != automation.TriggerTypePreviewOnPullRequestOpened {
		t.Fatalf(
			"snapshot trigger type = %q, want %q",
			snapshot.Trigger.Type,
			automation.TriggerTypePreviewOnPullRequestOpened,
		)
	}
	if snapshot.Target.OperationType != operations.TypePreviewStart {
		t.Fatalf("snapshot target operation_type = %q, want %q", snapshot.Target.OperationType, operations.TypePreviewStart)
	}
	if snapshot.Target.RuntimeEnvironmentType != operations.RuntimeEnvironmentTypePreview {
		t.Fatalf(
			"snapshot target runtime_environment_type = %q, want %q",
			snapshot.Target.RuntimeEnvironmentType,
			operations.RuntimeEnvironmentTypePreview,
		)
	}
	if snapshot.Target.TargetPRHeadCommitSHA != "abc123" {
		t.Fatalf("snapshot target_pr_head_commit_sha = %q, want %q", snapshot.Target.TargetPRHeadCommitSHA, "abc123")
	}
	if snapshot.Target.PullRequestSourceRepo.FullName != "acme/demo" {
		t.Fatalf(
			"snapshot target pull_request_source_repository.full_name = %q, want %q",
			snapshot.Target.PullRequestSourceRepo.FullName,
			"acme/demo",
		)
	}
	if snapshot.Delivery.ID != "delivery-1" {
		t.Fatalf("snapshot delivery id = %q, want %q", snapshot.Delivery.ID, "delivery-1")
	}
}
