package server

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/dominikpalatynski/toolshed/internal/store"
	"github.com/dominikpalatynski/toolshed/internal/store/sqlc"
)

func TestWebhookEventDetailResponsesOmitHeavyFieldsByDefault(t *testing.T) {
	t.Parallel()

	evaluationResponse := webhookEventTriggerEvaluationResponseFromModel(sqlc.WebhookEventTriggerEvaluations{
		ID:                  1,
		WebhookEventID:      2,
		RepositoryTriggerID: 3,
		Matched:             true,
		Reason:              "label_matched",
		TriggerSnapshotJson: []byte(`{"type":"pull_request_label"}`),
		CreatedAt:           validTimestamp(),
	}, false)
	if len(evaluationResponse.TriggerSnapshotJSON) != 0 {
		t.Fatalf("TriggerSnapshotJSON = %s, want omitted by default", evaluationResponse.TriggerSnapshotJSON)
	}

	detail := store.WebhookEventOperationRequestDetail{
		OperationRequest: sqlc.OperationRequests{
			ID:                    4,
			RepositoryID:          1,
			PullRequestID:         2,
			OperationType:         "preview-start",
			Source:                "trigger",
			Status:                "handled",
			TargetPrHeadCommitSha: "abc123",
			IntentSnapshotJson:    []byte(`{"target":{"target_pr_head_commit_sha":"abc123"}}`),
			CreatedAt:             validTimestamp(),
		},
	}
	operationRequestResponse := operationRequestResponseFromDetail(detail, false)
	if len(operationRequestResponse.IntentSnapshotJSON) != 0 {
		t.Fatalf("IntentSnapshotJSON = %s, want omitted by default", operationRequestResponse.IntentSnapshotJSON)
	}

	eventResponse := webhookEventResponseFromModel(sqlc.WebhookEvents{
		ID:          5,
		DeliveryID:  "delivery-1",
		GithubEvent: "pull_request",
		EventType:   "pull_request.labeled",
		Status:      "processed",
		PayloadJson: []byte(`{"action":"labeled"}`),
		CreatedAt:   validTimestamp(),
	}, false)
	if len(eventResponse.PayloadJSON) != 0 {
		t.Fatalf("PayloadJSON = %s, want omitted by default", eventResponse.PayloadJSON)
	}
}

func TestWebhookEventDetailResponsesIncludeHeavyFieldsWhenRequested(t *testing.T) {
	t.Parallel()

	evaluationResponse := webhookEventTriggerEvaluationResponseFromModel(sqlc.WebhookEventTriggerEvaluations{
		TriggerSnapshotJson: []byte(`{"type":"pull_request_label"}`),
		CreatedAt:           validTimestamp(),
	}, true)
	if got := string(evaluationResponse.TriggerSnapshotJSON); got != `{"type":"pull_request_label"}` {
		t.Fatalf("TriggerSnapshotJSON = %q, want included", got)
	}

	detail := store.WebhookEventOperationRequestDetail{
		OperationRequest: sqlc.OperationRequests{
			TargetPrHeadCommitSha: "abc123",
			IntentSnapshotJson:    []byte(`{"target":{"target_pr_head_commit_sha":"abc123"}}`),
			CreatedAt:             validTimestamp(),
		},
	}
	operationRequestResponse := operationRequestResponseFromDetail(detail, true)
	if got := string(operationRequestResponse.IntentSnapshotJSON); got != `{"target":{"target_pr_head_commit_sha":"abc123"}}` {
		t.Fatalf("IntentSnapshotJSON = %q, want included", got)
	}

	eventResponse := webhookEventResponseFromModel(sqlc.WebhookEvents{
		PayloadJson: []byte(`{"action":"labeled"}`),
		CreatedAt:   validTimestamp(),
	}, true)
	if got := string(eventResponse.PayloadJSON); got != `{"action":"labeled"}` {
		t.Fatalf("PayloadJSON = %q, want included", got)
	}
}

func TestRuntimeEnvironmentResponsesIncludeSourceRepositoryAndWorkspaceLocator(t *testing.T) {
	t.Parallel()

	response := runtimeEnvironmentResponseFromModel(sqlc.RuntimeEnvironments{
		ID:                       11,
		RepositoryID:             7,
		PullRequestID:            9,
		Type:                     "preview",
		Status:                   "prepared",
		TargetPrHeadCommitSha:    "abc123",
		SourceGithubRepositoryID: 501,
		SourceOwner:              "acme",
		SourceName:               "demo",
		SourceFullName:           "acme/demo",
		WorkspaceLocator:         strPtr("runtime-environments/11"),
		CreatedAt:                validTimestamp(),
		UpdatedAt:                validTimestamp(),
	})

	if response.SourceRepository.FullName != "acme/demo" {
		t.Fatalf("SourceRepository.FullName = %q, want %q", response.SourceRepository.FullName, "acme/demo")
	}
	if response.WorkspaceLocator != "runtime-environments/11" {
		t.Fatalf("WorkspaceLocator = %q, want %q", response.WorkspaceLocator, "runtime-environments/11")
	}
}

func TestPullRequestSummaryResponseIncludesCurrentSourceRepository(t *testing.T) {
	t.Parallel()

	response := pullRequestSummaryResponseFromModel(sqlc.PullRequests{
		ID:                              9,
		RepositoryID:                    7,
		PRNumber:                        42,
		GithubPullRequestID:             int64Ptr(12345),
		CurrentHeadCommitSha:            "abc123",
		CurrentSourceGithubRepositoryID: 501,
		CurrentSourceOwner:              "acme",
		CurrentSourceName:               "demo",
		CurrentSourceFullName:           "acme/demo",
		CreatedAt:                       validTimestamp(),
		UpdatedAt:                       validTimestamp(),
	})

	if response.CurrentSourceRepository.FullName != "acme/demo" {
		t.Fatalf("CurrentSourceRepository.FullName = %q, want %q", response.CurrentSourceRepository.FullName, "acme/demo")
	}
}

func validTimestamp() pgtype.Timestamptz {
	return pgtype.Timestamptz{
		Time:  time.Date(2026, time.May, 1, 13, 31, 0, 0, time.UTC),
		Valid: true,
	}
}

func int64Ptr(value int64) *int64 {
	return &value
}

func strPtr(value string) *string {
	return &value
}

func TestWebhookEventDetailResponsesStillMarshalCleanlyWithoutHeavyFields(t *testing.T) {
	t.Parallel()

	response := map[string]any{
		"webhook_event": webhookEventResponseFromModel(sqlc.WebhookEvents{
			ID:          1,
			DeliveryID:  "delivery-1",
			GithubEvent: "pull_request",
			EventType:   "pull_request.labeled",
			Status:      "processed",
			PayloadJson: []byte(`{"action":"labeled"}`),
			CreatedAt:   validTimestamp(),
		}, false),
		"evaluations": []webhookEventTriggerEvaluationResponse{
			webhookEventTriggerEvaluationResponseFromModel(sqlc.WebhookEventTriggerEvaluations{
				ID:                  2,
				WebhookEventID:      1,
				RepositoryTriggerID: 3,
				Matched:             true,
				Reason:              "label_matched",
				TriggerSnapshotJson: []byte(`{"type":"pull_request_label"}`),
				CreatedAt:           validTimestamp(),
			}, false),
		},
		"operation_requests": []operationRequestResponse{
			operationRequestResponseFromDetail(store.WebhookEventOperationRequestDetail{
				OperationRequest: sqlc.OperationRequests{
					ID:                    4,
					RepositoryID:          1,
					PullRequestID:         1,
					OperationType:         "preview-start",
					Source:                "trigger",
					Status:                "handled",
					TargetPrHeadCommitSha: "abc123",
					IntentSnapshotJson:    []byte(`{"target":{"target_pr_head_commit_sha":"abc123"}}`),
					CreatedAt:             validTimestamp(),
				},
			}, false),
		},
	}

	payload, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if string(payload) == "" {
		t.Fatalf("json.Marshal() produced empty payload")
	}
	if strings.Contains(string(payload), "payload_json") {
		t.Fatalf("payload_json should be omitted by default, got %s", payload)
	}
	if strings.Contains(string(payload), "trigger_snapshot_json") {
		t.Fatalf("trigger_snapshot_json should be omitted by default, got %s", payload)
	}
	if strings.Contains(string(payload), "intent_snapshot_json") {
		t.Fatalf("intent_snapshot_json should be omitted by default, got %s", payload)
	}
}
