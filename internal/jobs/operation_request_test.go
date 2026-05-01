package jobs

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/dominikpalatynski/toolshed/internal/operations"
	"github.com/dominikpalatynski/toolshed/internal/store"
	"github.com/dominikpalatynski/toolshed/internal/store/sqlc"
	"github.com/dominikpalatynski/toolshed/internal/testdb"
)

func TestOperationRequestWorkerPreviewStartCreateAndReuse(t *testing.T) {
	pool := testdb.Start(t)
	appStore := store.New(pool)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	worker := &OperationRequestWorker{logger: logger, store: appStore}

	ctx := context.Background()
	repository, pullRequest := mustCreateRepositoryAndPullRequest(t, ctx, appStore)

	firstRequest := mustInsertOperationRequest(t, ctx, appStore, repository.ID, pullRequest.ID, "abc123")
	if err := worker.Work(ctx, testJob(firstRequest.ID)); err != nil {
		t.Fatalf("first Work() error = %v", err)
	}

	firstHandled, err := appStore.Q().GetOperationRequestByID(ctx, firstRequest.ID)
	if err != nil {
		t.Fatalf("GetOperationRequestByID(first) error = %v", err)
	}
	if firstHandled.Outcome == nil || *firstHandled.Outcome != operations.OutcomeNewAttemptCreated {
		t.Fatalf("first outcome = %v, want %q", firstHandled.Outcome, operations.OutcomeNewAttemptCreated)
	}
	if firstHandled.RuntimeEnvironmentID == nil {
		t.Fatalf("first runtime_environment_id = nil, want non-nil")
	}

	runtimeEnvironment, err := appStore.Q().GetRuntimeEnvironmentByID(ctx, *firstHandled.RuntimeEnvironmentID)
	if err != nil {
		t.Fatalf("GetRuntimeEnvironmentByID() error = %v", err)
	}
	if runtimeEnvironment.Status != operations.RuntimeStatusPreparing {
		t.Fatalf("runtime environment status = %q, want %q", runtimeEnvironment.Status, operations.RuntimeStatusPreparing)
	}

	secondRequest := mustInsertOperationRequest(t, ctx, appStore, repository.ID, pullRequest.ID, "abc123")
	if err := worker.Work(ctx, testJob(secondRequest.ID)); err != nil {
		t.Fatalf("second Work() error = %v", err)
	}

	secondHandled, err := appStore.Q().GetOperationRequestByID(ctx, secondRequest.ID)
	if err != nil {
		t.Fatalf("GetOperationRequestByID(second) error = %v", err)
	}
	if secondHandled.Outcome == nil || *secondHandled.Outcome != operations.OutcomeAlreadyPreparing {
		t.Fatalf("second outcome = %v, want %q", secondHandled.Outcome, operations.OutcomeAlreadyPreparing)
	}
	if secondHandled.RuntimeEnvironmentID == nil || *secondHandled.RuntimeEnvironmentID != runtimeEnvironment.ID {
		t.Fatalf("second runtime_environment_id = %v, want %d", secondHandled.RuntimeEnvironmentID, runtimeEnvironment.ID)
	}

	if _, err := appStore.Q().UpdateRuntimeEnvironmentStatus(ctx, sqlc.UpdateRuntimeEnvironmentStatusParams{
		ID:     runtimeEnvironment.ID,
		Status: operations.RuntimeStatusPrepared,
	}); err != nil {
		t.Fatalf("UpdateRuntimeEnvironmentStatus() error = %v", err)
	}

	thirdRequest := mustInsertOperationRequest(t, ctx, appStore, repository.ID, pullRequest.ID, "abc123")
	if err := worker.Work(ctx, testJob(thirdRequest.ID)); err != nil {
		t.Fatalf("third Work() error = %v", err)
	}

	thirdHandled, err := appStore.Q().GetOperationRequestByID(ctx, thirdRequest.ID)
	if err != nil {
		t.Fatalf("GetOperationRequestByID(third) error = %v", err)
	}
	if thirdHandled.Outcome == nil || *thirdHandled.Outcome != operations.OutcomeAlreadyPrepared {
		t.Fatalf("third outcome = %v, want %q", thirdHandled.Outcome, operations.OutcomeAlreadyPrepared)
	}
	if thirdHandled.RuntimeEnvironmentID == nil || *thirdHandled.RuntimeEnvironmentID != runtimeEnvironment.ID {
		t.Fatalf("third runtime_environment_id = %v, want %d", thirdHandled.RuntimeEnvironmentID, runtimeEnvironment.ID)
	}
}

func TestOperationRequestWorkerPreviewStartCreatesNewAttemptAfterFailure(t *testing.T) {
	pool := testdb.Start(t)
	appStore := store.New(pool)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	worker := &OperationRequestWorker{logger: logger, store: appStore}

	ctx := context.Background()
	repository, pullRequest := mustCreateRepositoryAndPullRequest(t, ctx, appStore)

	firstRequest := mustInsertOperationRequest(t, ctx, appStore, repository.ID, pullRequest.ID, "abc123")
	if err := worker.Work(ctx, testJob(firstRequest.ID)); err != nil {
		t.Fatalf("first Work() error = %v", err)
	}

	firstHandled, err := appStore.Q().GetOperationRequestByID(ctx, firstRequest.ID)
	if err != nil {
		t.Fatalf("GetOperationRequestByID(first) error = %v", err)
	}
	if _, err := appStore.Q().UpdateRuntimeEnvironmentStatus(ctx, sqlc.UpdateRuntimeEnvironmentStatusParams{
		ID:     *firstHandled.RuntimeEnvironmentID,
		Status: operations.RuntimeStatusFailed,
	}); err != nil {
		t.Fatalf("UpdateRuntimeEnvironmentStatus() error = %v", err)
	}

	secondRequest := mustInsertOperationRequest(t, ctx, appStore, repository.ID, pullRequest.ID, "abc123")
	if err := worker.Work(ctx, testJob(secondRequest.ID)); err != nil {
		t.Fatalf("second Work() error = %v", err)
	}

	secondHandled, err := appStore.Q().GetOperationRequestByID(ctx, secondRequest.ID)
	if err != nil {
		t.Fatalf("GetOperationRequestByID(second) error = %v", err)
	}
	if secondHandled.Outcome == nil || *secondHandled.Outcome != operations.OutcomeNewAttemptCreated {
		t.Fatalf("second outcome = %v, want %q", secondHandled.Outcome, operations.OutcomeNewAttemptCreated)
	}
	if *secondHandled.RuntimeEnvironmentID == *firstHandled.RuntimeEnvironmentID {
		t.Fatalf("second runtime_environment_id = %d, want a new runtime environment id", *secondHandled.RuntimeEnvironmentID)
	}
}

func mustCreateRepositoryAndPullRequest(t *testing.T, ctx context.Context, appStore *store.Store) (sqlc.Repositories, sqlc.PullRequests) {
	t.Helper()

	repository, err := appStore.Q().UpsertRepository(ctx, sqlc.UpsertRepositoryParams{
		GithubRepositoryID:   301,
		GithubInstallationID: 401,
		Owner:                "acme",
		Name:                 "demo",
		FullName:             "acme/demo",
		HtmlUrl:              "https://github.com/acme/demo",
		IsPrivate:            false,
	})
	if err != nil {
		t.Fatalf("UpsertRepository() error = %v", err)
	}

	pullRequest, err := appStore.Q().UpsertPullRequestAnchor(ctx, sqlc.UpsertPullRequestAnchorParams{
		RepositoryID:         repository.ID,
		PRNumber:             42,
		GithubPullRequestID:  int64Ptr(9001),
		CurrentHeadCommitSha: "abc123",
	})
	if err != nil {
		t.Fatalf("UpsertPullRequestAnchor() error = %v", err)
	}

	return repository, pullRequest
}

func mustInsertOperationRequest(t *testing.T, ctx context.Context, appStore *store.Store, repositoryID, pullRequestID int64, targetHeadSHA string) sqlc.OperationRequests {
	t.Helper()

	operationRequest, err := appStore.Q().InsertOperationRequest(ctx, sqlc.InsertOperationRequestParams{
		RepositoryID:          repositoryID,
		PullRequestID:         pullRequestID,
		OperationType:         operations.TypePreviewStart,
		Source:                operations.SourceTrigger,
		Status:                operations.StatusQueued,
		TargetPrHeadCommitSha: targetHeadSHA,
		IntentSnapshotJson:    []byte(`{"target":{"target_pr_head_commit_sha":"abc123"}}`),
	})
	if err != nil {
		t.Fatalf("InsertOperationRequest() error = %v", err)
	}

	return operationRequest
}

func testJob(operationRequestID int64) *river.Job[OperationRequestArgs] {
	return &river.Job[OperationRequestArgs]{
		JobRow: &rivertype.JobRow{
			ID: 1,
		},
		Args: OperationRequestArgs{OperationRequestID: operationRequestID},
	}
}

func int64Ptr(value int64) *int64 {
	return &value
}
