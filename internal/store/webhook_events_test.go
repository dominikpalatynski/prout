package store

import (
	"context"
	"testing"

	"github.com/dominikpalatynski/toolshed/internal/operations"
	"github.com/dominikpalatynski/toolshed/internal/store/sqlc"
	"github.com/dominikpalatynski/toolshed/internal/testdb"
)

func TestGetWebhookEventDetailHydratesOperationRequestsWithRuntimeEnvironment(t *testing.T) {
	pool := testdb.Start(t)
	appStore := New(pool)
	ctx := context.Background()

	repository, err := appStore.Q().UpsertRepository(ctx, sqlc.UpsertRepositoryParams{
		GithubRepositoryID:   501,
		GithubInstallationID: 601,
		Owner:                "acme",
		Name:                 "demo",
		FullName:             "acme/demo",
		HtmlUrl:              "https://github.com/acme/demo",
		IsPrivate:            false,
	})
	if err != nil {
		t.Fatalf("UpsertRepository() error = %v", err)
	}

	webhookEvent, err := appStore.Q().InsertWebhookEvent(ctx, sqlc.InsertWebhookEventParams{
		DeliveryID:         "delivery-1",
		GithubEvent:        "pull_request",
		EventType:          "pull_request.opened",
		GithubRepositoryID: &repository.GithubRepositoryID,
		Status:             "processed",
		PayloadJson:        []byte(`{"action":"opened"}`),
	})
	if err != nil {
		t.Fatalf("InsertWebhookEvent() error = %v", err)
	}

	webhookEvent, err = appStore.Q().MarkWebhookEventProcessed(ctx, sqlc.MarkWebhookEventProcessedParams{
		ID:           webhookEvent.ID,
		RepositoryID: &repository.ID,
	})
	if err != nil {
		t.Fatalf("MarkWebhookEventProcessed() error = %v", err)
	}

	trigger, err := appStore.Q().UpsertRepositoryTrigger(ctx, sqlc.UpsertRepositoryTriggerParams{
		RepositoryID: repository.ID,
		Type:         "pull_request_opened",
		EventFamily:  "pull_request.opened",
		IdentityKey:  "pull_request_opened",
		ConfigJson:   []byte(`{}`),
		Enabled:      true,
	})
	if err != nil {
		t.Fatalf("UpsertRepositoryTrigger() error = %v", err)
	}

	evaluation, err := appStore.Q().InsertWebhookEventTriggerEvaluation(ctx, sqlc.InsertWebhookEventTriggerEvaluationParams{
		WebhookEventID:      webhookEvent.ID,
		RepositoryTriggerID: trigger.ID,
		Matched:             true,
		Reason:              "pull_request_opened_matched",
		TriggerSnapshotJson: []byte(`{"id":1}`),
	})
	if err != nil {
		t.Fatalf("InsertWebhookEventTriggerEvaluation() error = %v", err)
	}

	pullRequest, err := appStore.Q().UpsertPullRequestAnchor(ctx, sqlc.UpsertPullRequestAnchorParams{
		RepositoryID:                    repository.ID,
		PRNumber:                        42,
		GithubPullRequestID:             int64Ptr(7001),
		CurrentHeadCommitSha:            "abc123",
		CurrentSourceGithubRepositoryID: repository.GithubRepositoryID,
		CurrentSourceOwner:              repository.Owner,
		CurrentSourceName:               repository.Name,
		CurrentSourceFullName:           repository.FullName,
	})
	if err != nil {
		t.Fatalf("UpsertPullRequestAnchor() error = %v", err)
	}

	runtimeEnvironment, err := appStore.Q().InsertRuntimeEnvironment(ctx, sqlc.InsertRuntimeEnvironmentParams{
		RepositoryID:             repository.ID,
		PullRequestID:            pullRequest.ID,
		Type:                     operations.RuntimeEnvironmentTypePreview,
		Status:                   operations.RuntimeStatusPreparing,
		TargetPrHeadCommitSha:    "abc123",
		SourceGithubRepositoryID: repository.GithubRepositoryID,
		SourceOwner:              repository.Owner,
		SourceName:               repository.Name,
		SourceFullName:           repository.FullName,
	})
	if err != nil {
		t.Fatalf("InsertRuntimeEnvironment() error = %v", err)
	}

	operationRequest, err := appStore.Q().InsertOperationRequest(ctx, sqlc.InsertOperationRequestParams{
		WebhookEventID:                  &webhookEvent.ID,
		WebhookEventTriggerEvaluationID: &evaluation.ID,
		RepositoryID:                    repository.ID,
		RepositoryTriggerID:             &trigger.ID,
		PullRequestID:                   pullRequest.ID,
		OperationType:                   operations.TypePreviewStart,
		Source:                          operations.SourceTrigger,
		Status:                          operations.StatusQueued,
		TargetPrHeadCommitSha:           "abc123",
		IntentSnapshotJson:              []byte(`{"target":{"target_pr_head_commit_sha":"abc123"}}`),
		CurrentStep:                     operations.StepSourceMaterialization,
		CurrentStepState:                operations.StepStatePending,
		CurrentStepDetailsJson:          nil,
	})
	if err != nil {
		t.Fatalf("InsertOperationRequest() error = %v", err)
	}

	if _, err := appStore.Q().MarkOperationRequestHandled(ctx, sqlc.MarkOperationRequestHandledParams{
		ID:                   operationRequest.ID,
		RuntimeEnvironmentID: &runtimeEnvironment.ID,
		Outcome:              strPtr(operations.OutcomeAlreadyPreparing),
	}); err != nil {
		t.Fatalf("MarkOperationRequestHandled() error = %v", err)
	}

	detail, err := appStore.GetWebhookEventDetail(ctx, webhookEvent.ID)
	if err != nil {
		t.Fatalf("GetWebhookEventDetail() error = %v", err)
	}
	if len(detail.OperationRequests) != 1 {
		t.Fatalf("len(detail.OperationRequests) = %d, want 1", len(detail.OperationRequests))
	}
	if detail.OperationRequests[0].RuntimeEnvironment == nil {
		t.Fatalf("detail.OperationRequests[0].RuntimeEnvironment = nil, want non-nil")
	}
	if detail.OperationRequests[0].RuntimeEnvironment.Status != operations.RuntimeStatusPreparing {
		t.Fatalf("detail.OperationRequests[0].RuntimeEnvironment.Status = %q, want %q", detail.OperationRequests[0].RuntimeEnvironment.Status, operations.RuntimeStatusPreparing)
	}
}

func int64Ptr(value int64) *int64 {
	return &value
}

func strPtr(value string) *string {
	return &value
}
