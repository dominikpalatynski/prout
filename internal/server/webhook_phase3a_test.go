package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/dominikpalatynski/toolshed/internal/githubapp"
	"github.com/dominikpalatynski/toolshed/internal/jobs"
	"github.com/dominikpalatynski/toolshed/internal/operations"
	"github.com/dominikpalatynski/toolshed/internal/store"
	"github.com/dominikpalatynski/toolshed/internal/store/sqlc"
	"github.com/dominikpalatynski/toolshed/internal/testdb"
	"github.com/dominikpalatynski/toolshed/internal/triggers"
	"github.com/dominikpalatynski/toolshed/internal/webhook"
)

func TestProcessSupportedVerifiedDeliveryCreatesOperationRequestForCommentTrigger(t *testing.T) {
	pool := testdb.Start(t)
	appStore := store.New(pool)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	riverClient := newTestRiverClient(t, pool, appStore, logger)

	ctx := context.Background()
	repository := mustCreateEnabledRepository(t, ctx, appStore)
	trigger := mustCreateCommentTrigger(t, ctx, appStore, repository.ID)

	server := &Server{
		logger:         logger,
		store:          appStore,
		riverClient:    riverClient,
		githubResolver: stubGitHubResolver{pullRequest: githubapp.PullRequest{GithubPullRequestID: 12345, Number: 42, HeadSHA: "resolved-head-sha"}},
		triggerCatalog: triggers.NewCatalog(),
	}

	delivery := webhook.Delivery{
		DeliveryID:         "delivery-1",
		GithubEvent:        "issue_comment",
		EventType:          webhook.EventTypeIssueCommentCreated,
		GithubRepositoryID: &repository.GithubRepositoryID,
		Supported:          true,
		PayloadJSON:        json.RawMessage(`{"action":"created"}`),
		Event: webhook.NormalizedEvent{
			Type:               webhook.EventTypeIssueCommentCreated,
			GithubRepositoryID: repository.GithubRepositoryID,
			PRNumber:           42,
			CommentID:          99,
			CommentBody:        "/preview\nplease",
			CommentFirstLine:   "/preview",
			CommentAuthorLogin: "octocat",
		},
	}

	result, err := server.processSupportedVerifiedDelivery(ctx, delivery)
	if err != nil {
		t.Fatalf("processSupportedVerifiedDelivery() error = %v", err)
	}
	if result.ProcessingError != nil {
		t.Fatalf("processSupportedVerifiedDelivery() ProcessingError = %v, want nil", result.ProcessingError)
	}
	if result.OperationRequestCount != 1 {
		t.Fatalf("processSupportedVerifiedDelivery() OperationRequestCount = %d, want 1", result.OperationRequestCount)
	}

	pullRequest, err := appStore.Q().GetPullRequestByRepositoryIDAndPRNumber(ctx, sqlc.GetPullRequestByRepositoryIDAndPRNumberParams{
		RepositoryID: repository.ID,
		PRNumber:     42,
	})
	if err != nil {
		t.Fatalf("GetPullRequestByRepositoryIDAndPRNumber() error = %v", err)
	}
	if pullRequest.CurrentHeadCommitSha != "resolved-head-sha" {
		t.Fatalf("pull request current_head_commit_sha = %q, want %q", pullRequest.CurrentHeadCommitSha, "resolved-head-sha")
	}

	operationRequests, err := appStore.Q().ListWebhookEventOperationRequests(ctx, &result.Event.ID)
	if err != nil {
		t.Fatalf("ListWebhookEventOperationRequests() error = %v", err)
	}
	if len(operationRequests) != 1 {
		t.Fatalf("len(operationRequests) = %d, want 1", len(operationRequests))
	}

	operationRequest := operationRequests[0]
	if operationRequest.RepositoryTriggerID == nil || *operationRequest.RepositoryTriggerID != trigger.ID {
		t.Fatalf("operation request repository_trigger_id = %v, want %d", operationRequest.RepositoryTriggerID, trigger.ID)
	}
	if operationRequest.OperationType != operations.TypePreviewStart {
		t.Fatalf("operation request operation_type = %q, want %q", operationRequest.OperationType, operations.TypePreviewStart)
	}
	if operationRequest.Source != operations.SourceTrigger {
		t.Fatalf("operation request source = %q, want %q", operationRequest.Source, operations.SourceTrigger)
	}
	if operationRequest.Status != operations.StatusQueued {
		t.Fatalf("operation request status = %q, want %q", operationRequest.Status, operations.StatusQueued)
	}
	if operationRequest.TargetPrHeadCommitSha != "resolved-head-sha" {
		t.Fatalf("operation request target_pr_head_commit_sha = %q, want %q", operationRequest.TargetPrHeadCommitSha, "resolved-head-sha")
	}

	var snapshot struct {
		Target struct {
			PullRequestID         int64  `json:"pull_request_id"`
			OperationType         string `json:"operation_type"`
			TargetPRHeadCommitSHA string `json:"target_pr_head_commit_sha"`
		} `json:"target"`
	}
	if err := json.Unmarshal(operationRequest.IntentSnapshotJson, &snapshot); err != nil {
		t.Fatalf("json.Unmarshal(intent_snapshot_json) error = %v", err)
	}
	if snapshot.Target.PullRequestID != pullRequest.ID {
		t.Fatalf("snapshot target pull_request_id = %d, want %d", snapshot.Target.PullRequestID, pullRequest.ID)
	}
	if snapshot.Target.OperationType != operations.TypePreviewStart {
		t.Fatalf("snapshot target operation_type = %q, want %q", snapshot.Target.OperationType, operations.TypePreviewStart)
	}
	if snapshot.Target.TargetPRHeadCommitSHA != "resolved-head-sha" {
		t.Fatalf("snapshot target_pr_head_commit_sha = %q, want %q", snapshot.Target.TargetPRHeadCommitSHA, "resolved-head-sha")
	}

	var runtimeEnvironmentCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM runtime_environments`).Scan(&runtimeEnvironmentCount); err != nil {
		t.Fatalf("count runtime_environments error = %v", err)
	}
	if runtimeEnvironmentCount != 0 {
		t.Fatalf("runtime environment count = %d, want 0 before worker execution", runtimeEnvironmentCount)
	}

	var riverJobCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind = $1`, jobs.OperationRequestArgs{}.Kind()).Scan(&riverJobCount); err != nil {
		t.Fatalf("count river jobs error = %v", err)
	}
	if riverJobCount != 1 {
		t.Fatalf("river job count = %d, want 1", riverJobCount)
	}
}

type stubGitHubResolver struct {
	pullRequest githubapp.PullRequest
}

func (stubGitHubResolver) ResolveRepository(context.Context, string) (githubapp.Repository, error) {
	return githubapp.Repository{}, nil
}

func (s stubGitHubResolver) ResolvePullRequest(context.Context, string, string, int64, int) (githubapp.PullRequest, error) {
	return s.pullRequest, nil
}

func newTestRiverClient(t *testing.T, pool *pgxpool.Pool, appStore *store.Store, logger *slog.Logger) *river.Client[pgx.Tx] {
	t.Helper()

	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Logger: logger,
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: 1},
		},
		Workers: jobs.NewWorkers(appStore, logger),
	})
	if err != nil {
		t.Fatalf("river.NewClient() error = %v", err)
	}
	return client
}

func mustCreateEnabledRepository(t *testing.T, ctx context.Context, appStore *store.Store) sqlc.Repositories {
	t.Helper()

	repository, err := appStore.Q().UpsertRepository(ctx, sqlc.UpsertRepositoryParams{
		GithubRepositoryID:   101,
		GithubInstallationID: 202,
		Owner:                "acme",
		Name:                 "demo",
		FullName:             "acme/demo",
		HtmlUrl:              "https://github.com/acme/demo",
		IsPrivate:            false,
	})
	if err != nil {
		t.Fatalf("UpsertRepository() error = %v", err)
	}

	repository, err = appStore.Q().SetRepositoryEnabled(ctx, sqlc.SetRepositoryEnabledParams{
		ID:      repository.ID,
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("SetRepositoryEnabled() error = %v", err)
	}

	return repository
}

func mustCreateCommentTrigger(t *testing.T, ctx context.Context, appStore *store.Store, repositoryID int64) sqlc.RepositoryTriggers {
	t.Helper()

	trigger, err := appStore.Q().UpsertRepositoryTrigger(ctx, sqlc.UpsertRepositoryTriggerParams{
		RepositoryID: repositoryID,
		Type:         triggers.TypePullRequestCommentCommand,
		EventFamily:  webhook.EventTypeIssueCommentCreated,
		IdentityKey:  "pull_request_comment_command:exact_first_line:/preview",
		ConfigJson:   []byte(`{"matcher":"exact_first_line","command":"/preview"}`),
		Enabled:      true,
	})
	if err != nil {
		t.Fatalf("UpsertRepositoryTrigger() error = %v", err)
	}

	return trigger
}
