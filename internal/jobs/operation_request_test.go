package jobs

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/dominikpalatynski/toolshed/internal/config"
	"github.com/dominikpalatynski/toolshed/internal/operations"
	"github.com/dominikpalatynski/toolshed/internal/pullrequests"
	"github.com/dominikpalatynski/toolshed/internal/store"
	"github.com/dominikpalatynski/toolshed/internal/store/sqlc"
	"github.com/dominikpalatynski/toolshed/internal/testdb"
	"github.com/dominikpalatynski/toolshed/internal/webhook"
	"github.com/dominikpalatynski/toolshed/internal/workspaces"
)

func TestOperationRequestWorkerTimeout(t *testing.T) {
	worker := &OperationRequestWorker{}

	if got := worker.Timeout(testJob(1, 1, 1)); got != config.DefaultOperationRequestJobTimeout {
		t.Fatalf("Timeout() = %s, want %s", got, config.DefaultOperationRequestJobTimeout)
	}
	if config.DefaultOperationRequestJobTimeout <= time.Minute {
		t.Fatalf("DefaultOperationRequestJobTimeout = %s, want > %s", config.DefaultOperationRequestJobTimeout, time.Minute)
	}

	customTimeout := 3 * time.Minute
	worker.SetJobTimeout(customTimeout)
	if got := worker.Timeout(testJob(1, 1, 1)); got != customTimeout {
		t.Fatalf("Timeout() after SetJobTimeout() = %s, want %s", got, customTimeout)
	}
}

func TestOperationRequestWorkerPreviewStartMaterializesWorkspaceAndReusesPreparedAttempt(t *testing.T) {
	pool := testdb.Start(t)
	appStore := store.New(pool)
	worker, workspaceManager := newTestWorker(t, appStore, []downloadResult{{tarball: mustBuildTarball(t, map[string]string{
		"README.md":       "hello\n",
		"nested/file.txt": "world\n",
		"compose.yml":     "services: {}\n",
	})}})

	ctx := context.Background()
	repository, pullRequest, sourceRepository := mustCreateRepositoryAndPullRequest(t, ctx, appStore, "abc123")

	firstRequest := mustInsertPreviewStartOperationRequest(t, ctx, appStore, repository.ID, pullRequest.ID, sourceRepository, "abc123")
	if err := worker.Work(ctx, testJob(firstRequest.ID, 1, 1)); err != nil {
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
	if firstHandled.CurrentStepState != operations.StepStateCompleted {
		t.Fatalf("first current_step_state = %q, want %q", firstHandled.CurrentStepState, operations.StepStateCompleted)
	}

	runtimeEnvironment, err := appStore.Q().GetRuntimeEnvironmentByID(ctx, *firstHandled.RuntimeEnvironmentID)
	if err != nil {
		t.Fatalf("GetRuntimeEnvironmentByID() error = %v", err)
	}
	if runtimeEnvironment.Status != operations.RuntimeStatusPrepared {
		t.Fatalf("runtime environment status = %q, want %q", runtimeEnvironment.Status, operations.RuntimeStatusPrepared)
	}

	workspaceExists, err := workspaceManager.WorkspaceExists(stringPtrValue(runtimeEnvironment.WorkspaceLocator))
	if err != nil {
		t.Fatalf("WorkspaceExists() error = %v", err)
	}
	if !workspaceExists {
		t.Fatalf("workspaceExists = false, want true")
	}

	secondRequest := mustInsertPreviewStartOperationRequest(t, ctx, appStore, repository.ID, pullRequest.ID, sourceRepository, "abc123")
	if err := worker.Work(ctx, testJob(secondRequest.ID, 1, 1)); err != nil {
		t.Fatalf("second Work() error = %v", err)
	}

	secondHandled, err := appStore.Q().GetOperationRequestByID(ctx, secondRequest.ID)
	if err != nil {
		t.Fatalf("GetOperationRequestByID(second) error = %v", err)
	}
	if secondHandled.Outcome == nil || *secondHandled.Outcome != operations.OutcomeAlreadyPrepared {
		t.Fatalf("second outcome = %v, want %q", secondHandled.Outcome, operations.OutcomeAlreadyPrepared)
	}
	if secondHandled.RuntimeEnvironmentID == nil || *secondHandled.RuntimeEnvironmentID != runtimeEnvironment.ID {
		t.Fatalf("second runtime_environment_id = %v, want %d", secondHandled.RuntimeEnvironmentID, runtimeEnvironment.ID)
	}
}

func TestOperationRequestWorkerPreviewStartRetryReusesSameAttempt(t *testing.T) {
	pool := testdb.Start(t)
	appStore := store.New(pool)
	worker, workspaceManager := newTestWorker(t, appStore, []downloadResult{
		{err: io.ErrUnexpectedEOF},
		{tarball: mustBuildTarball(t, map[string]string{"README.md": "hello\n"})},
	})

	ctx := context.Background()
	repository, pullRequest, sourceRepository := mustCreateRepositoryAndPullRequest(t, ctx, appStore, "abc123")

	operationRequest := mustInsertPreviewStartOperationRequest(t, ctx, appStore, repository.ID, pullRequest.ID, sourceRepository, "abc123")
	if err := worker.Work(ctx, testJob(operationRequest.ID, 1, 2)); err == nil {
		t.Fatalf("first Work() error = nil, want non-nil")
	}

	afterFirstAttempt, err := appStore.Q().GetOperationRequestByID(ctx, operationRequest.ID)
	if err != nil {
		t.Fatalf("GetOperationRequestByID(after first attempt) error = %v", err)
	}
	if afterFirstAttempt.Status != operations.StatusQueued {
		t.Fatalf("status after first attempt = %q, want %q", afterFirstAttempt.Status, operations.StatusQueued)
	}
	if afterFirstAttempt.RuntimeEnvironmentID == nil {
		t.Fatalf("runtime_environment_id after first attempt = nil, want non-nil")
	}
	if afterFirstAttempt.CurrentStepState != operations.StepStateInProgress {
		t.Fatalf("current_step_state after first attempt = %q, want %q", afterFirstAttempt.CurrentStepState, operations.StepStateInProgress)
	}

	firstRuntimeEnvironmentID := *afterFirstAttempt.RuntimeEnvironmentID
	firstRuntimeEnvironment, err := appStore.Q().GetRuntimeEnvironmentByID(ctx, firstRuntimeEnvironmentID)
	if err != nil {
		t.Fatalf("GetRuntimeEnvironmentByID(after first attempt) error = %v", err)
	}
	if firstRuntimeEnvironment.Status != operations.RuntimeStatusPreparing {
		t.Fatalf("runtime environment status after first attempt = %q, want %q", firstRuntimeEnvironment.Status, operations.RuntimeStatusPreparing)
	}

	if err := worker.Work(ctx, testJob(operationRequest.ID, 2, 2)); err != nil {
		t.Fatalf("second Work() error = %v", err)
	}

	handled, err := appStore.Q().GetOperationRequestByID(ctx, operationRequest.ID)
	if err != nil {
		t.Fatalf("GetOperationRequestByID(handled) error = %v", err)
	}
	if handled.RuntimeEnvironmentID == nil || *handled.RuntimeEnvironmentID != firstRuntimeEnvironmentID {
		t.Fatalf("runtime_environment_id after retry = %v, want %d", handled.RuntimeEnvironmentID, firstRuntimeEnvironmentID)
	}
	if handled.Outcome == nil || *handled.Outcome != operations.OutcomeNewAttemptCreated {
		t.Fatalf("outcome after retry = %v, want %q", handled.Outcome, operations.OutcomeNewAttemptCreated)
	}

	runtimeEnvironment, err := appStore.Q().GetRuntimeEnvironmentByID(ctx, firstRuntimeEnvironmentID)
	if err != nil {
		t.Fatalf("GetRuntimeEnvironmentByID(after retry) error = %v", err)
	}
	if runtimeEnvironment.Status != operations.RuntimeStatusPrepared {
		t.Fatalf("runtime environment status after retry = %q, want %q", runtimeEnvironment.Status, operations.RuntimeStatusPrepared)
	}

	workspaceExists, err := workspaceManager.WorkspaceExists(stringPtrValue(runtimeEnvironment.WorkspaceLocator))
	if err != nil {
		t.Fatalf("WorkspaceExists() error = %v", err)
	}
	if !workspaceExists {
		t.Fatalf("workspaceExists after retry = false, want true")
	}
}

func TestOperationRequestWorkerPreviewStartCreatesNewAttemptAfterFailure(t *testing.T) {
	pool := testdb.Start(t)
	appStore := store.New(pool)
	worker, workspaceManager := newTestWorker(t, appStore, []downloadResult{{err: io.ErrUnexpectedEOF}})

	ctx := context.Background()
	repository, pullRequest, sourceRepository := mustCreateRepositoryAndPullRequest(t, ctx, appStore, "abc123")

	firstRequest := mustInsertPreviewStartOperationRequest(t, ctx, appStore, repository.ID, pullRequest.ID, sourceRepository, "abc123")
	if err := worker.Work(ctx, testJob(firstRequest.ID, 1, 1)); err == nil {
		t.Fatalf("first Work() error = nil, want non-nil")
	}

	firstHandled, err := appStore.Q().GetOperationRequestByID(ctx, firstRequest.ID)
	if err != nil {
		t.Fatalf("GetOperationRequestByID(first) error = %v", err)
	}
	if firstHandled.Status != operations.StatusFailed {
		t.Fatalf("first status = %q, want %q", firstHandled.Status, operations.StatusFailed)
	}
	if firstHandled.RuntimeEnvironmentID == nil {
		t.Fatalf("first runtime_environment_id = nil, want non-nil")
	}

	firstRuntimeEnvironment, err := appStore.Q().GetRuntimeEnvironmentByID(ctx, *firstHandled.RuntimeEnvironmentID)
	if err != nil {
		t.Fatalf("GetRuntimeEnvironmentByID(first) error = %v", err)
	}
	if firstRuntimeEnvironment.Status != operations.RuntimeStatusFailed {
		t.Fatalf("first runtime environment status = %q, want %q", firstRuntimeEnvironment.Status, operations.RuntimeStatusFailed)
	}

	worker.githubDownloader = &stubTarballDownloader{results: []downloadResult{{tarball: mustBuildTarball(t, map[string]string{"README.md": "fixed\n"})}}}
	secondRequest := mustInsertPreviewStartOperationRequest(t, ctx, appStore, repository.ID, pullRequest.ID, sourceRepository, "abc123")
	if err := worker.Work(ctx, testJob(secondRequest.ID, 1, 1)); err != nil {
		t.Fatalf("second Work() error = %v", err)
	}

	secondHandled, err := appStore.Q().GetOperationRequestByID(ctx, secondRequest.ID)
	if err != nil {
		t.Fatalf("GetOperationRequestByID(second) error = %v", err)
	}
	if secondHandled.Outcome == nil || *secondHandled.Outcome != operations.OutcomeNewAttemptCreated {
		t.Fatalf("second outcome = %v, want %q", secondHandled.Outcome, operations.OutcomeNewAttemptCreated)
	}
	if secondHandled.RuntimeEnvironmentID == nil || *secondHandled.RuntimeEnvironmentID == *firstHandled.RuntimeEnvironmentID {
		t.Fatalf("second runtime_environment_id = %v, want a new runtime environment id", secondHandled.RuntimeEnvironmentID)
	}

	secondRuntimeEnvironment, err := appStore.Q().GetRuntimeEnvironmentByID(ctx, *secondHandled.RuntimeEnvironmentID)
	if err != nil {
		t.Fatalf("GetRuntimeEnvironmentByID(second) error = %v", err)
	}
	if secondRuntimeEnvironment.Status != operations.RuntimeStatusPrepared {
		t.Fatalf("second runtime environment status = %q, want %q", secondRuntimeEnvironment.Status, operations.RuntimeStatusPrepared)
	}

	workspaceExists, err := workspaceManager.WorkspaceExists(stringPtrValue(secondRuntimeEnvironment.WorkspaceLocator))
	if err != nil {
		t.Fatalf("WorkspaceExists(second) error = %v", err)
	}
	if !workspaceExists {
		t.Fatalf("second workspaceExists = false, want true")
	}
}

func TestOperationRequestWorkerPreviewStartSupersedesOlderAttemptAndCreatesCleanupRequest(t *testing.T) {
	pool := testdb.Start(t)
	appStore := store.New(pool)
	enqueuer := &stubOperationRequestEnqueuer{}
	worker, workspaceManager := newTestWorkerWithEnqueuer(t, appStore, []downloadResult{
		{tarball: mustBuildTarball(t, map[string]string{"README.md": "first\n"})},
		{tarball: mustBuildTarball(t, map[string]string{"README.md": "second\n"})},
	}, enqueuer)

	ctx := context.Background()
	repository, pullRequest, sourceRepository := mustCreateRepositoryAndPullRequest(t, ctx, appStore, "abc123")

	firstRequest := mustInsertPreviewStartOperationRequest(t, ctx, appStore, repository.ID, pullRequest.ID, sourceRepository, "abc123")
	if err := worker.Work(ctx, testJob(firstRequest.ID, 1, 1)); err != nil {
		t.Fatalf("first Work() error = %v", err)
	}
	firstHandled, err := appStore.Q().GetOperationRequestByID(ctx, firstRequest.ID)
	if err != nil {
		t.Fatalf("GetOperationRequestByID(first) error = %v", err)
	}

	secondRequest := mustInsertPreviewStartOperationRequest(t, ctx, appStore, repository.ID, pullRequest.ID, sourceRepository, "def456")
	if err := worker.Work(ctx, testJob(secondRequest.ID, 1, 1)); err != nil {
		t.Fatalf("second Work() error = %v", err)
	}
	secondHandled, err := appStore.Q().GetOperationRequestByID(ctx, secondRequest.ID)
	if err != nil {
		t.Fatalf("GetOperationRequestByID(second) error = %v", err)
	}

	firstRuntimeEnvironment, err := appStore.Q().GetRuntimeEnvironmentByID(ctx, *firstHandled.RuntimeEnvironmentID)
	if err != nil {
		t.Fatalf("GetRuntimeEnvironmentByID(first) error = %v", err)
	}
	if firstRuntimeEnvironment.Status != operations.RuntimeStatusSuperseded {
		t.Fatalf("first runtime environment status = %q, want %q", firstRuntimeEnvironment.Status, operations.RuntimeStatusSuperseded)
	}

	secondRuntimeEnvironment, err := appStore.Q().GetRuntimeEnvironmentByID(ctx, *secondHandled.RuntimeEnvironmentID)
	if err != nil {
		t.Fatalf("GetRuntimeEnvironmentByID(second) error = %v", err)
	}
	if secondRuntimeEnvironment.Status != operations.RuntimeStatusPrepared {
		t.Fatalf("second runtime environment status = %q, want %q", secondRuntimeEnvironment.Status, operations.RuntimeStatusPrepared)
	}

	if len(enqueuer.operationRequestIDs) != 1 {
		t.Fatalf("len(enqueued cleanup requests) = %d, want 1", len(enqueuer.operationRequestIDs))
	}

	cleanupRequest, err := appStore.Q().GetOperationRequestByID(ctx, enqueuer.operationRequestIDs[0])
	if err != nil {
		t.Fatalf("GetOperationRequestByID(cleanup) error = %v", err)
	}
	if cleanupRequest.OperationType != operations.TypePreviewCleanupSuperseded {
		t.Fatalf("cleanup operation type = %q, want %q", cleanupRequest.OperationType, operations.TypePreviewCleanupSuperseded)
	}
	if cleanupRequest.TargetRuntimeEnvironmentID == nil || *cleanupRequest.TargetRuntimeEnvironmentID != firstRuntimeEnvironment.ID {
		t.Fatalf("cleanup target_runtime_environment_id = %v, want %d", cleanupRequest.TargetRuntimeEnvironmentID, firstRuntimeEnvironment.ID)
	}

	if err := worker.Work(ctx, testJob(cleanupRequest.ID, 1, 1)); err != nil {
		t.Fatalf("cleanup Work() error = %v", err)
	}

	cleanupHandled, err := appStore.Q().GetOperationRequestByID(ctx, cleanupRequest.ID)
	if err != nil {
		t.Fatalf("GetOperationRequestByID(cleanup handled) error = %v", err)
	}
	if cleanupHandled.Outcome == nil || *cleanupHandled.Outcome != operations.OutcomeCleanupCompleted {
		t.Fatalf("cleanup outcome = %v, want %q", cleanupHandled.Outcome, operations.OutcomeCleanupCompleted)
	}

	workspaceExists, err := workspaceManager.WorkspaceExists(stringPtrValue(firstRuntimeEnvironment.WorkspaceLocator))
	if err != nil {
		t.Fatalf("WorkspaceExists(cleaned) error = %v", err)
	}
	if workspaceExists {
		t.Fatalf("workspaceExists(cleaned) = true, want false")
	}
}

func TestOperationRequestWorkerPreviewStartRepairsMissingPreparedWorkspace(t *testing.T) {
	pool := testdb.Start(t)
	appStore := store.New(pool)
	enqueuer := &stubOperationRequestEnqueuer{}
	worker, workspaceManager := newTestWorkerWithEnqueuer(t, appStore, []downloadResult{
		{tarball: mustBuildTarball(t, map[string]string{"README.md": "first\n"})},
		{tarball: mustBuildTarball(t, map[string]string{"README.md": "second\n"})},
	}, enqueuer)

	ctx := context.Background()
	repository, pullRequest, sourceRepository := mustCreateRepositoryAndPullRequest(t, ctx, appStore, "abc123")

	firstRequest := mustInsertPreviewStartOperationRequest(t, ctx, appStore, repository.ID, pullRequest.ID, sourceRepository, "abc123")
	if err := worker.Work(ctx, testJob(firstRequest.ID, 1, 1)); err != nil {
		t.Fatalf("first Work() error = %v", err)
	}
	firstHandled, err := appStore.Q().GetOperationRequestByID(ctx, firstRequest.ID)
	if err != nil {
		t.Fatalf("GetOperationRequestByID(first) error = %v", err)
	}

	firstRuntimeEnvironment, err := appStore.Q().GetRuntimeEnvironmentByID(ctx, *firstHandled.RuntimeEnvironmentID)
	if err != nil {
		t.Fatalf("GetRuntimeEnvironmentByID(first) error = %v", err)
	}
	if err := workspaceManager.CleanupWorkspace(stringPtrValue(firstRuntimeEnvironment.WorkspaceLocator)); err != nil {
		t.Fatalf("CleanupWorkspace() error = %v", err)
	}

	secondRequest := mustInsertPreviewStartOperationRequest(t, ctx, appStore, repository.ID, pullRequest.ID, sourceRepository, "abc123")
	if err := worker.Work(ctx, testJob(secondRequest.ID, 1, 1)); err != nil {
		t.Fatalf("second Work() error = %v", err)
	}
	secondHandled, err := appStore.Q().GetOperationRequestByID(ctx, secondRequest.ID)
	if err != nil {
		t.Fatalf("GetOperationRequestByID(second) error = %v", err)
	}
	if secondHandled.RuntimeEnvironmentID == nil || *secondHandled.RuntimeEnvironmentID == firstRuntimeEnvironment.ID {
		t.Fatalf("second runtime_environment_id = %v, want a new runtime environment id", secondHandled.RuntimeEnvironmentID)
	}

	repairedFirstRuntimeEnvironment, err := appStore.Q().GetRuntimeEnvironmentByID(ctx, firstRuntimeEnvironment.ID)
	if err != nil {
		t.Fatalf("GetRuntimeEnvironmentByID(repaired first) error = %v", err)
	}
	if repairedFirstRuntimeEnvironment.Status != operations.RuntimeStatusSuperseded {
		t.Fatalf("repaired first runtime environment status = %q, want %q", repairedFirstRuntimeEnvironment.Status, operations.RuntimeStatusSuperseded)
	}

	if len(enqueuer.operationRequestIDs) != 1 {
		t.Fatalf("len(enqueued cleanup requests) = %d, want 1", len(enqueuer.operationRequestIDs))
	}
}

func mustCreateRepositoryAndPullRequest(
	t *testing.T,
	ctx context.Context,
	appStore *store.Store,
	headSHA string,
) (sqlc.Repositories, sqlc.PullRequests, pullrequests.SourceRepository) {
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

	sourceRepository := pullrequests.SourceRepository{
		GithubRepositoryID: 301,
		Owner:              "acme",
		Name:               "demo",
		FullName:           "acme/demo",
	}
	pullRequest, err := appStore.Q().UpsertPullRequestAnchor(ctx, sqlc.UpsertPullRequestAnchorParams{
		RepositoryID:                    repository.ID,
		PRNumber:                        42,
		GithubPullRequestID:             int64Ptr(9001),
		CurrentHeadCommitSha:            headSHA,
		CurrentSourceGithubRepositoryID: sourceRepository.GithubRepositoryID,
		CurrentSourceOwner:              sourceRepository.Owner,
		CurrentSourceName:               sourceRepository.Name,
		CurrentSourceFullName:           sourceRepository.FullName,
	})
	if err != nil {
		t.Fatalf("UpsertPullRequestAnchor() error = %v", err)
	}

	return repository, pullRequest, sourceRepository
}

func mustInsertPreviewStartOperationRequest(
	t *testing.T,
	ctx context.Context,
	appStore *store.Store,
	repositoryID, pullRequestID int64,
	sourceRepository pullrequests.SourceRepository,
	targetHeadSHA string,
) sqlc.OperationRequests {
	t.Helper()

	initialStep, err := operations.InitialStepForOperation(operations.TypePreviewStart)
	if err != nil {
		t.Fatalf("InitialStepForOperation() error = %v", err)
	}

	githubPullRequestID := int64(9001)
	intentSnapshot, err := operations.BuildTriggerSnapshot(operations.TriggerSnapshotInput{
		RepositoryID:        repositoryID,
		PullRequestID:       pullRequestID,
		PRNumber:            42,
		GithubPullRequestID: &githubPullRequestID,
		PRSourceRepository:  sourceRepository,
		DeliveryID:          "delivery-1",
		Event: webhook.NormalizedEvent{
			Type:                webhook.EventTypePullRequestOpened,
			GithubRepositoryID:  sourceRepository.GithubRepositoryID,
			PRNumber:            42,
			GithubPullRequestID: githubPullRequestID,
			PRHeadSHA:           targetHeadSHA,
			PRSourceRepository:  sourceRepository,
		},
		TriggerID:              5,
		TriggerType:            "pull_request_opened",
		TriggerIdentityKey:     "pull_request_opened",
		OperationType:          operations.TypePreviewStart,
		RuntimeEnvironmentType: operations.RuntimeEnvironmentTypePreview,
		TargetPRHeadCommitSHA:  targetHeadSHA,
	})
	if err != nil {
		t.Fatalf("BuildTriggerSnapshot() error = %v", err)
	}

	operationRequest, err := appStore.Q().InsertOperationRequest(ctx, sqlc.InsertOperationRequestParams{
		RepositoryID:           repositoryID,
		PullRequestID:          pullRequestID,
		OperationType:          operations.TypePreviewStart,
		Source:                 operations.SourceTrigger,
		Status:                 operations.StatusQueued,
		TargetPrHeadCommitSha:  targetHeadSHA,
		IntentSnapshotJson:     intentSnapshot,
		CurrentStep:            initialStep.Name,
		CurrentStepState:       initialStep.State,
		CurrentStepDetailsJson: nil,
	})
	if err != nil {
		t.Fatalf("InsertOperationRequest() error = %v", err)
	}

	return operationRequest
}

func newTestWorker(t *testing.T, appStore *store.Store, downloadResults []downloadResult) (*OperationRequestWorker, *workspaces.FilesystemManager) {
	t.Helper()
	return newTestWorkerWithEnqueuer(t, appStore, downloadResults, nil)
}

func newTestWorkerWithEnqueuer(
	t *testing.T,
	appStore *store.Store,
	downloadResults []downloadResult,
	enqueuer operationRequestEnqueuer,
) (*OperationRequestWorker, *workspaces.FilesystemManager) {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	workspaceRoot := t.TempDir()
	workspaceManager := workspaces.NewFilesystemManager(workspaceRoot)
	worker := &OperationRequestWorker{
		logger:           logger,
		store:            appStore,
		githubDownloader: &stubTarballDownloader{results: downloadResults},
		workspaceManager: workspaceManager,
		jobEnqueuer:      enqueuer,
	}
	return worker, workspaceManager
}

func testJob(operationRequestID int64, attempt, maxAttempts int) *river.Job[OperationRequestArgs] {
	return &river.Job[OperationRequestArgs]{
		JobRow: &rivertype.JobRow{
			ID:          operationRequestID,
			Attempt:     attempt,
			MaxAttempts: maxAttempts,
		},
		Args: OperationRequestArgs{OperationRequestID: operationRequestID},
	}
}

type downloadResult struct {
	tarball []byte
	err     error
}

type stubTarballDownloader struct {
	results []downloadResult
	calls   int
}

func (s *stubTarballDownloader) DownloadTarball(context.Context, string, string, int64, string) (io.ReadCloser, error) {
	if s.calls >= len(s.results) {
		return nil, io.EOF
	}
	result := s.results[s.calls]
	s.calls++
	if result.err != nil {
		return nil, result.err
	}
	return io.NopCloser(bytes.NewReader(result.tarball)), nil
}

type stubOperationRequestEnqueuer struct {
	operationRequestIDs []int64
}

func (s *stubOperationRequestEnqueuer) InsertTx(_ context.Context, tx pgx.Tx, args river.JobArgs, _ *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	operationRequestArgs, ok := args.(OperationRequestArgs)
	if !ok {
		return nil, io.ErrUnexpectedEOF
	}
	if tx == nil {
		return nil, io.ErrUnexpectedEOF
	}
	s.operationRequestIDs = append(s.operationRequestIDs, operationRequestArgs.OperationRequestID)
	return &rivertype.JobInsertResult{}, nil
}

func mustBuildTarball(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)

	for name, contents := range files {
		header := &tar.Header{
			Name: "acme-demo-123/" + name,
			Mode: 0o644,
			Size: int64(len(contents)),
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatalf("WriteHeader(%q) error = %v", name, err)
		}
		if _, err := tarWriter.Write([]byte(contents)); err != nil {
			t.Fatalf("Write(%q) error = %v", name, err)
		}
	}

	if err := tarWriter.Close(); err != nil {
		t.Fatalf("tarWriter.Close() error = %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("gzipWriter.Close() error = %v", err)
	}

	return buffer.Bytes()
}

func int64Ptr(value int64) *int64 {
	return &value
}

func TestFilesystemWorkspaceContent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	workspaceManager := workspaces.NewFilesystemManager(root)
	stagingPath, err := workspaceManager.CreateStaging("runtime-environments/42")
	if err != nil {
		t.Fatalf("CreateStaging() error = %v", err)
	}

	if err := workspaceManager.ExtractTarball(stagingPath, bytes.NewReader(mustBuildTarball(t, map[string]string{
		"README.md":       "hello\n",
		"nested/file.txt": "world\n",
	}))); err != nil {
		t.Fatalf("ExtractTarball() error = %v", err)
	}
	if err := workspaceManager.PromoteStaging(stagingPath, "runtime-environments/42"); err != nil {
		t.Fatalf("PromoteStaging() error = %v", err)
	}

	readme, err := os.ReadFile(filepath.Join(root, "runtime-environments", "42", "README.md"))
	if err != nil {
		t.Fatalf("ReadFile(README.md) error = %v", err)
	}
	if string(readme) != "hello\n" {
		t.Fatalf("README.md = %q, want %q", string(readme), "hello\n")
	}

	nested, err := os.ReadFile(filepath.Join(root, "runtime-environments", "42", "nested", "file.txt"))
	if err != nil {
		t.Fatalf("ReadFile(nested/file.txt) error = %v", err)
	}
	if string(nested) != "world\n" {
		t.Fatalf("nested/file.txt = %q, want %q", string(nested), "world\n")
	}

	if _, err := os.Stat(filepath.Join(root, "runtime-environments", "42", "acme-demo-123")); !os.IsNotExist(err) {
		t.Fatalf("tarball wrapper directory should not exist, stat error = %v", err)
	}
}
