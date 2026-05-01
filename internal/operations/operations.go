package operations

import (
	"encoding/json"
	"fmt"

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

	OutcomeNewAttemptCreated = "new_attempt_created"
	OutcomeAlreadyPreparing  = "already_preparing"
	OutcomeAlreadyPrepared   = "already_prepared"
	OutcomeOperationFailed   = "operation_failed"
)

type Intent struct {
	OperationType string
}

type TriggerSnapshotInput struct {
	RepositoryID           int64
	PullRequestID          int64
	PRNumber               int64
	GithubPullRequestID    *int64
	DeliveryID             string
	Event                  webhook.NormalizedEvent
	TriggerID              int64
	TriggerType            string
	TriggerIdentityKey     string
	OperationType          string
	RuntimeEnvironmentType string
	TargetPRHeadCommitSHA  string
}

func NewIntent(triggerType string) (Intent, error) {
	operationType, err := TypeForTrigger(triggerType)
	if err != nil {
		return Intent{}, err
	}

	return Intent{OperationType: operationType}, nil
}

func TypeForTrigger(triggerType string) (string, error) {
	switch triggerType {
	case "pull_request_opened", "pull_request_label", "pull_request_comment_command":
		return TypePreviewStart, nil
	default:
		return "", fmt.Errorf("unknown trigger type %q", triggerType)
	}
}

func RuntimeEnvironmentTypeForOperation(operationType string) (string, error) {
	switch operationType {
	case TypePreviewStart, TypePreviewRestart, TypePreviewDelete, TypePreviewCleanupSuperseded:
		return RuntimeEnvironmentTypePreview, nil
	default:
		return "", fmt.Errorf("unknown operation type %q", operationType)
	}
}

func BuildTriggerSnapshot(input TriggerSnapshotInput) ([]byte, error) {
	return json.Marshal(struct {
		Source  string `json:"source"`
		Trigger struct {
			ID          int64  `json:"id"`
			Type        string `json:"type"`
			IdentityKey string `json:"identity_key"`
		} `json:"trigger"`
		Target struct {
			RepositoryID           int64  `json:"repository_id"`
			PullRequestID          int64  `json:"pull_request_id"`
			PRNumber               int64  `json:"pr_number"`
			GithubPullRequestID    *int64 `json:"github_pull_request_id,omitempty"`
			OperationType          string `json:"operation_type"`
			RuntimeEnvironmentType string `json:"runtime_environment_type"`
			TargetPRHeadCommitSHA  string `json:"target_pr_head_commit_sha"`
		} `json:"target"`
		Delivery struct {
			ID    string                  `json:"id"`
			Event webhook.NormalizedEvent `json:"event"`
		} `json:"delivery"`
	}{
		Source: SourceTrigger,
		Trigger: struct {
			ID          int64  `json:"id"`
			Type        string `json:"type"`
			IdentityKey string `json:"identity_key"`
		}{
			ID:          input.TriggerID,
			Type:        input.TriggerType,
			IdentityKey: input.TriggerIdentityKey,
		},
		Target: struct {
			RepositoryID           int64  `json:"repository_id"`
			PullRequestID          int64  `json:"pull_request_id"`
			PRNumber               int64  `json:"pr_number"`
			GithubPullRequestID    *int64 `json:"github_pull_request_id,omitempty"`
			OperationType          string `json:"operation_type"`
			RuntimeEnvironmentType string `json:"runtime_environment_type"`
			TargetPRHeadCommitSHA  string `json:"target_pr_head_commit_sha"`
		}{
			RepositoryID:           input.RepositoryID,
			PullRequestID:          input.PullRequestID,
			PRNumber:               input.PRNumber,
			GithubPullRequestID:    input.GithubPullRequestID,
			OperationType:          input.OperationType,
			RuntimeEnvironmentType: input.RuntimeEnvironmentType,
			TargetPRHeadCommitSHA:  input.TargetPRHeadCommitSHA,
		},
		Delivery: struct {
			ID    string                  `json:"id"`
			Event webhook.NormalizedEvent `json:"event"`
		}{
			ID:    input.DeliveryID,
			Event: input.Event,
		},
	})
}
