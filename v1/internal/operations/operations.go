package operations

import (
	"encoding/json"
	"fmt"

	"github.com/dominikpalatynski/toolshed/internal/pullrequests"
	"github.com/dominikpalatynski/toolshed/internal/webhook"
)

const (
	TypePreviewStart              = "preview-start"
	TypePreviewRestart            = "preview-restart"
	TypePreviewDelete             = "preview-delete"
	TypePreviewCleanupSuperseded  = "preview-cleanup-superseded"
	RuntimeEnvironmentTypePreview = "preview"

	SourceTrigger = "trigger"
	SourceSystem  = "system"

	StatusQueued  = "queued"
	StatusHandled = "handled"
	StatusFailed  = "failed"

	RuntimeStatusPreparing  = "preparing"
	RuntimeStatusPrepared   = "prepared"
	RuntimeStatusFailed     = "failed"
	RuntimeStatusSuperseded = "superseded"
	RuntimeStatusDeleted    = "deleted"

	OutcomeNewAttemptCreated = "new_attempt_created"
	OutcomeAlreadyPreparing  = "already_preparing"
	OutcomeAlreadyPrepared   = "already_prepared"
	OutcomeAlreadyDeleted    = "already_deleted"
	OutcomeAttemptSuperseded = "attempt_superseded"
	OutcomeCleanupCompleted  = "cleanup_completed"
	OutcomeOperationFailed   = "operation_failed"
)

type Intent struct {
	OperationType string
}

type TriggerIntentSnapshotInput struct {
	RepositoryID           int64
	PullRequestID          int64
	PRNumber               int64
	GithubPullRequestID    *int64
	PRSourceRepository     pullrequests.SourceRepository
	DeliveryID             string
	Event                  webhook.NormalizedEvent
	TriggerID              int64
	TriggerType            string
	OperationType          string
	RuntimeEnvironmentType string
	TargetPRHeadCommitSHA  string
	TargetRuntime          *TargetRuntimeSnapshot
}

type PreviewStartSnapshotInput = TriggerIntentSnapshotInput

type TargetRuntimeSnapshot struct {
	ID               int64  `json:"id"`
	Status           string `json:"status"`
	WorkspaceLocator string `json:"workspace_locator,omitempty"`
}

type PreviewStartSnapshot struct {
	Source  string `json:"source"`
	Trigger struct {
		ID   int64  `json:"id"`
		Type string `json:"type"`
	} `json:"trigger"`
	Target struct {
		RepositoryID           int64                         `json:"repository_id"`
		PullRequestID          int64                         `json:"pull_request_id"`
		PRNumber               int64                         `json:"pr_number"`
		GithubPullRequestID    *int64                        `json:"github_pull_request_id,omitempty"`
		PullRequestSourceRepo  pullrequests.SourceRepository `json:"pull_request_source_repository"`
		OperationType          string                        `json:"operation_type"`
		RuntimeEnvironmentType string                        `json:"runtime_environment_type"`
		TargetPRHeadCommitSHA  string                        `json:"target_pr_head_commit_sha"`
	} `json:"target"`
	Delivery struct {
		ID    string                  `json:"id"`
		Event webhook.NormalizedEvent `json:"event"`
	} `json:"delivery"`
}

type PreviewDeleteSnapshot struct {
	Source  string `json:"source"`
	Trigger struct {
		ID   int64  `json:"id"`
		Type string `json:"type"`
	} `json:"trigger"`
	Target struct {
		RepositoryID           int64                         `json:"repository_id"`
		PullRequestID          int64                         `json:"pull_request_id"`
		PRNumber               int64                         `json:"pr_number"`
		GithubPullRequestID    *int64                        `json:"github_pull_request_id,omitempty"`
		PullRequestSourceRepo  pullrequests.SourceRepository `json:"pull_request_source_repository"`
		OperationType          string                        `json:"operation_type"`
		RuntimeEnvironmentType string                        `json:"runtime_environment_type"`
		TargetPRHeadCommitSHA  string                        `json:"target_pr_head_commit_sha"`
		RuntimeEnvironment     *TargetRuntimeSnapshot        `json:"runtime_environment,omitempty"`
	} `json:"target"`
	Delivery struct {
		ID    string                  `json:"id"`
		Event webhook.NormalizedEvent `json:"event"`
	} `json:"delivery"`
}

func BuildPreviewStartSnapshot(input TriggerIntentSnapshotInput) ([]byte, error) {
	snapshot := PreviewStartSnapshot{
		Source: SourceTrigger,
	}
	snapshot.Trigger.ID = input.TriggerID
	snapshot.Trigger.Type = input.TriggerType
	snapshot.Target.RepositoryID = input.RepositoryID
	snapshot.Target.PullRequestID = input.PullRequestID
	snapshot.Target.PRNumber = input.PRNumber
	snapshot.Target.GithubPullRequestID = input.GithubPullRequestID
	snapshot.Target.PullRequestSourceRepo = input.PRSourceRepository
	snapshot.Target.OperationType = input.OperationType
	snapshot.Target.RuntimeEnvironmentType = input.RuntimeEnvironmentType
	snapshot.Target.TargetPRHeadCommitSHA = input.TargetPRHeadCommitSHA
	snapshot.Delivery.ID = input.DeliveryID
	snapshot.Delivery.Event = input.Event
	return json.Marshal(snapshot)
}

func BuildPreviewDeleteSnapshot(input TriggerIntentSnapshotInput) ([]byte, error) {
	snapshot := PreviewDeleteSnapshot{
		Source: SourceTrigger,
	}
	snapshot.Trigger.ID = input.TriggerID
	snapshot.Trigger.Type = input.TriggerType
	snapshot.Target.RepositoryID = input.RepositoryID
	snapshot.Target.PullRequestID = input.PullRequestID
	snapshot.Target.PRNumber = input.PRNumber
	snapshot.Target.GithubPullRequestID = input.GithubPullRequestID
	snapshot.Target.PullRequestSourceRepo = input.PRSourceRepository
	snapshot.Target.OperationType = input.OperationType
	snapshot.Target.RuntimeEnvironmentType = input.RuntimeEnvironmentType
	snapshot.Target.TargetPRHeadCommitSHA = input.TargetPRHeadCommitSHA
	snapshot.Target.RuntimeEnvironment = input.TargetRuntime
	snapshot.Delivery.ID = input.DeliveryID
	snapshot.Delivery.Event = input.Event
	return json.Marshal(snapshot)
}

func ParsePreviewStartSnapshot(snapshotJSON []byte) (PreviewStartSnapshot, error) {
	var snapshot PreviewStartSnapshot
	if err := json.Unmarshal(snapshotJSON, &snapshot); err != nil {
		return PreviewStartSnapshot{}, fmt.Errorf("decode preview-start snapshot: %w", err)
	}
	return snapshot, nil
}

func BuildCleanupSupersededSnapshot(
	repositoryID, pullRequestID, runtimeEnvironmentID int64,
	targetHeadSHA, workspaceLocator string,
) ([]byte, error) {
	return json.Marshal(struct {
		Source string `json:"source"`
		System struct {
			Reason string `json:"reason"`
		} `json:"system"`
		Target struct {
			RepositoryID           int64  `json:"repository_id"`
			PullRequestID          int64  `json:"pull_request_id"`
			RuntimeEnvironmentID   int64  `json:"runtime_environment_id"`
			OperationType          string `json:"operation_type"`
			RuntimeEnvironmentType string `json:"runtime_environment_type"`
			TargetPRHeadCommitSHA  string `json:"target_pr_head_commit_sha"`
			WorkspaceLocator       string `json:"workspace_locator"`
		} `json:"target"`
	}{
		Source: SourceSystem,
		System: struct {
			Reason string `json:"reason"`
		}{
			Reason: "superseded_runtime_environment_cleanup",
		},
		Target: struct {
			RepositoryID           int64  `json:"repository_id"`
			PullRequestID          int64  `json:"pull_request_id"`
			RuntimeEnvironmentID   int64  `json:"runtime_environment_id"`
			OperationType          string `json:"operation_type"`
			RuntimeEnvironmentType string `json:"runtime_environment_type"`
			TargetPRHeadCommitSHA  string `json:"target_pr_head_commit_sha"`
			WorkspaceLocator       string `json:"workspace_locator"`
		}{
			RepositoryID:           repositoryID,
			PullRequestID:          pullRequestID,
			RuntimeEnvironmentID:   runtimeEnvironmentID,
			OperationType:          TypePreviewCleanupSuperseded,
			RuntimeEnvironmentType: RuntimeEnvironmentTypePreview,
			TargetPRHeadCommitSHA:  targetHeadSHA,
			WorkspaceLocator:       workspaceLocator,
		},
	})
}

func RuntimeEnvironmentStatusIsActive(status string) bool {
	switch status {
	case RuntimeStatusPreparing, RuntimeStatusPrepared:
		return true
	default:
		return false
	}
}
