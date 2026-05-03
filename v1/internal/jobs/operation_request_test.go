package jobs

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/dominikpalatynski/toolshed/internal/automation"
	"github.com/dominikpalatynski/toolshed/internal/config"
	"github.com/dominikpalatynski/toolshed/internal/operations"
	"github.com/dominikpalatynski/toolshed/internal/pullrequests"
	runtimebackend "github.com/dominikpalatynski/toolshed/internal/runtime"
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

	historyEntries, err := appStore.ListOperationRequestHistoryEntries(ctx, operationRequest.ID)
	if err != nil {
		t.Fatalf("ListOperationRequestHistoryEntries() error = %v", err)
	}
	if got, want := historyKinds(historyEntries), []string{
		operations.HistoryKindRequestStarted,
		operations.HistoryKindStepEntered,
		operations.HistoryKindRequestRetried,
		operations.HistoryKindStepCompleted,
		operations.HistoryKindStepEntered,
		operations.HistoryKindStepCompleted,
		operations.HistoryKindStepEntered,
		operations.HistoryKindStepCompleted,
		operations.HistoryKindRequestCompleted,
	}; !equalStrings(got, want) {
		t.Fatalf("history kinds = %v, want %v", got, want)
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

	cleanupHistory, err := appStore.ListOperationRequestHistoryEntries(ctx, cleanupRequest.ID)
	if err != nil {
		t.Fatalf("ListOperationRequestHistoryEntries(cleanup) error = %v", err)
	}
	if got, want := historyKinds(cleanupHistory), []string{
		operations.HistoryKindRequestStarted,
		operations.HistoryKindStepEntered,
		operations.HistoryKindStepCompleted,
		operations.HistoryKindStepEntered,
		operations.HistoryKindStepCompleted,
		operations.HistoryKindRequestCompleted,
	}; !equalStrings(got, want) {
		t.Fatalf("cleanup history kinds = %v, want %v", got, want)
	}
}

func TestOperationRequestWorkerPreviewDeleteCleansFrozenActiveRuntimeEnvironment(t *testing.T) {
	pool := testdb.Start(t)
	appStore := store.New(pool)
	worker, workspaceManager := newTestWorker(t, appStore, []downloadResult{
		{tarball: mustBuildTarball(t, map[string]string{"README.md": "first\n"})},
	})

	ctx := context.Background()
	repository, pullRequest, sourceRepository := mustCreateRepositoryAndPullRequest(t, ctx, appStore, "abc123")

	startRequest := mustInsertPreviewStartOperationRequest(t, ctx, appStore, repository.ID, pullRequest.ID, sourceRepository, "abc123")
	if err := worker.Work(ctx, testJob(startRequest.ID, 1, 1)); err != nil {
		t.Fatalf("start Work() error = %v", err)
	}

	startHandled, err := appStore.Q().GetOperationRequestByID(ctx, startRequest.ID)
	if err != nil {
		t.Fatalf("GetOperationRequestByID(start) error = %v", err)
	}
	targetRuntimeEnvironment, err := appStore.Q().GetRuntimeEnvironmentByID(ctx, *startHandled.RuntimeEnvironmentID)
	if err != nil {
		t.Fatalf("GetRuntimeEnvironmentByID(target) error = %v", err)
	}

	deleteRequest := mustInsertPreviewDeleteOperationRequest(
		t,
		ctx,
		appStore,
		repository.ID,
		pullRequest.ID,
		sourceRepository,
		"abc123",
		&targetRuntimeEnvironment,
	)
	if err := worker.Work(ctx, testJob(deleteRequest.ID, 1, 1)); err != nil {
		t.Fatalf("delete Work() error = %v", err)
	}

	deleteHandled, err := appStore.Q().GetOperationRequestByID(ctx, deleteRequest.ID)
	if err != nil {
		t.Fatalf("GetOperationRequestByID(delete) error = %v", err)
	}
	if deleteHandled.Outcome == nil || *deleteHandled.Outcome != operations.OutcomeCleanupCompleted {
		t.Fatalf("delete outcome = %v, want %q", deleteHandled.Outcome, operations.OutcomeCleanupCompleted)
	}

	deletedRuntimeEnvironment, err := appStore.Q().GetRuntimeEnvironmentByID(ctx, targetRuntimeEnvironment.ID)
	if err != nil {
		t.Fatalf("GetRuntimeEnvironmentByID(deleted) error = %v", err)
	}
	if deletedRuntimeEnvironment.Status != operations.RuntimeStatusDeleted {
		t.Fatalf("deleted runtime environment status = %q, want %q", deletedRuntimeEnvironment.Status, operations.RuntimeStatusDeleted)
	}

	workspaceExists, err := workspaceManager.WorkspaceExists(stringPtrValue(targetRuntimeEnvironment.WorkspaceLocator))
	if err != nil {
		t.Fatalf("WorkspaceExists(cleaned) error = %v", err)
	}
	if workspaceExists {
		t.Fatalf("workspaceExists(cleaned) = true, want false")
	}
}

func TestOperationRequestWorkerPreviewDeleteWithoutFrozenTargetCompletesAsAlreadyDeleted(t *testing.T) {
	pool := testdb.Start(t)
	appStore := store.New(pool)
	worker, _ := newTestWorker(t, appStore, nil)

	ctx := context.Background()
	repository, pullRequest, sourceRepository := mustCreateRepositoryAndPullRequest(t, ctx, appStore, "abc123")

	deleteRequest := mustInsertPreviewDeleteOperationRequest(
		t,
		ctx,
		appStore,
		repository.ID,
		pullRequest.ID,
		sourceRepository,
		"abc123",
		nil,
	)
	if err := worker.Work(ctx, testJob(deleteRequest.ID, 1, 1)); err != nil {
		t.Fatalf("delete Work() error = %v", err)
	}

	deleteHandled, err := appStore.Q().GetOperationRequestByID(ctx, deleteRequest.ID)
	if err != nil {
		t.Fatalf("GetOperationRequestByID(delete) error = %v", err)
	}
	if deleteHandled.Outcome == nil || *deleteHandled.Outcome != operations.OutcomeAlreadyDeleted {
		t.Fatalf("delete outcome = %v, want %q", deleteHandled.Outcome, operations.OutcomeAlreadyDeleted)
	}
	if deleteHandled.Status != operations.StatusHandled {
		t.Fatalf("delete status = %q, want %q", deleteHandled.Status, operations.StatusHandled)
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

func TestOperationRequestWorkerPreviewStartRuntimeDeploymentRetryReusesFrozenInput(t *testing.T) {
	pool := testdb.Start(t)
	appStore := store.New(pool)
	worker, _ := newTestWorker(t, appStore, []downloadResult{
		{tarball: mustBuildTarball(t, map[string]string{"README.md": "first\n"})},
	})

	backend := worker.runtimeBackend.(*stubRuntimeBackend)
	backend.deployErrors = []error{runtimebackend.RetryableError(io.ErrUnexpectedEOF)}

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
	if afterFirstAttempt.CurrentStep != operations.StepRuntimeDeployment {
		t.Fatalf("current_step after first attempt = %q, want %q", afterFirstAttempt.CurrentStep, operations.StepRuntimeDeployment)
	}
	if afterFirstAttempt.CurrentStepState != operations.StepStateInProgress {
		t.Fatalf("current_step_state after first attempt = %q, want %q", afterFirstAttempt.CurrentStepState, operations.StepStateInProgress)
	}

	if err := worker.Work(ctx, testJob(operationRequest.ID, 2, 2)); err != nil {
		t.Fatalf("second Work() error = %v", err)
	}

	handled, err := appStore.Q().GetOperationRequestByID(ctx, operationRequest.ID)
	if err != nil {
		t.Fatalf("GetOperationRequestByID(handled) error = %v", err)
	}
	if handled.Outcome == nil || *handled.Outcome != operations.OutcomeNewAttemptCreated {
		t.Fatalf("outcome after retry = %v, want %q", handled.Outcome, operations.OutcomeNewAttemptCreated)
	}

	if backend.prepareCalls != 1 {
		t.Fatalf("prepareCalls = %d, want 1", backend.prepareCalls)
	}
	if backend.deployCalls != 2 {
		t.Fatalf("deployCalls = %d, want 2", backend.deployCalls)
	}
	if backend.teardownCalls != 1 {
		t.Fatalf("teardownCalls = %d, want 1", backend.teardownCalls)
	}
}

func TestOperationRequestWorkerPreviewStartPermanentPreparationFailureFinalizesImmediately(t *testing.T) {
	pool := testdb.Start(t)
	appStore := store.New(pool)
	worker, _ := newTestWorker(t, appStore, []downloadResult{
		{tarball: mustBuildTarball(t, map[string]string{"README.md": "first\n"})},
	})

	backend := worker.runtimeBackend.(*stubRuntimeBackend)
	backend.prepareErrors = []error{runtimebackend.PermanentError(errors.New("invalid compose configuration"))}

	ctx := context.Background()
	repository, pullRequest, sourceRepository := mustCreateRepositoryAndPullRequest(t, ctx, appStore, "abc123")

	operationRequest := mustInsertPreviewStartOperationRequest(t, ctx, appStore, repository.ID, pullRequest.ID, sourceRepository, "abc123")
	if err := worker.Work(ctx, testJob(operationRequest.ID, 1, 3)); err != nil {
		t.Fatalf("Work() error = %v, want nil for immediate permanent finalization", err)
	}

	handled, err := appStore.Q().GetOperationRequestByID(ctx, operationRequest.ID)
	if err != nil {
		t.Fatalf("GetOperationRequestByID(handled) error = %v", err)
	}
	if handled.Status != operations.StatusFailed {
		t.Fatalf("status = %q, want %q", handled.Status, operations.StatusFailed)
	}
	if handled.CurrentStep != operations.StepComposePreparation {
		t.Fatalf("current_step = %q, want %q", handled.CurrentStep, operations.StepComposePreparation)
	}
	if handled.CurrentStepState != operations.StepStateFailed {
		t.Fatalf("current_step_state = %q, want %q", handled.CurrentStepState, operations.StepStateFailed)
	}

	runtimeEnvironment, err := appStore.Q().GetRuntimeEnvironmentByID(ctx, *handled.RuntimeEnvironmentID)
	if err != nil {
		t.Fatalf("GetRuntimeEnvironmentByID() error = %v", err)
	}
	if runtimeEnvironment.Status != operations.RuntimeStatusFailed {
		t.Fatalf("runtime environment status = %q, want %q", runtimeEnvironment.Status, operations.RuntimeStatusFailed)
	}
}

func TestOperationRequestWorkerPermanentFailureSanitizesHistoryAndEmitsDebugLogs(t *testing.T) {
	pool := testdb.Start(t)
	appStore := store.New(pool)
	worker, _ := newTestWorker(t, appStore, []downloadResult{
		{tarball: mustBuildTarball(t, map[string]string{"README.md": "first\n"})},
	})

	var logOutput bytes.Buffer
	worker.logger = slog.New(slog.NewJSONHandler(&logOutput, &slog.HandlerOptions{Level: slog.LevelDebug}))

	backend := worker.runtimeBackend.(*stubRuntimeBackend)
	backend.prepareErrors = []error{runtimebackend.PermanentError(errors.New("invalid compose configuration SECRET=super-secret-value"))}

	ctx := context.Background()
	repository, pullRequest, sourceRepository := mustCreateRepositoryAndPullRequest(t, ctx, appStore, "abc123")

	operationRequest := mustInsertPreviewStartOperationRequest(t, ctx, appStore, repository.ID, pullRequest.ID, sourceRepository, "abc123")
	if err := worker.Work(ctx, testJob(operationRequest.ID, 1, 3)); err != nil {
		t.Fatalf("Work() error = %v, want nil for immediate permanent finalization", err)
	}

	historyEntries, err := appStore.ListOperationRequestHistoryEntries(ctx, operationRequest.ID)
	if err != nil {
		t.Fatalf("ListOperationRequestHistoryEntries() error = %v", err)
	}
	if got, want := historyKinds(historyEntries), []string{
		operations.HistoryKindRequestStarted,
		operations.HistoryKindStepEntered,
		operations.HistoryKindStepCompleted,
		operations.HistoryKindStepEntered,
		operations.HistoryKindStepFailed,
		operations.HistoryKindRequestFailed,
	}; !equalStrings(got, want) {
		t.Fatalf("history kinds = %v, want %v", got, want)
	}

	for _, entry := range historyEntries {
		if strings.Contains(string(entry.DetailsJson), "super-secret-value") {
			t.Fatalf("history details leaked raw secret: %s", entry.DetailsJson)
		}
	}

	failureDetails := decodeHistoryDetails(t, historyEntries[len(historyEntries)-1].DetailsJson)
	if got := failureDetails["error_summary"]; got != "invalid compose configuration SECRET=[REDACTED]" {
		t.Fatalf("error_summary = %v, want redacted summary", got)
	}

	logs := logOutput.String()
	if !strings.Contains(logs, `"history_kind":"request_failed"`) {
		t.Fatalf("logs missing request_failed history entry: %s", logs)
	}
	if !strings.Contains(logs, `"step":"compose_preparation"`) {
		t.Fatalf("logs missing step context: %s", logs)
	}
	if strings.Contains(logs, "super-secret-value") {
		t.Fatalf("logs leaked raw secret: %s", logs)
	}
}

func TestOperationRequestWorkerPreviewStartRepairsMissingPreparedDeploymentArtifact(t *testing.T) {
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

	deploymentRecord, err := appStore.Q().GetRuntimeEnvironmentDeploymentByRuntimeEnvironmentID(ctx, *firstHandled.RuntimeEnvironmentID)
	if err != nil {
		t.Fatalf("GetRuntimeEnvironmentDeploymentByRuntimeEnvironmentID() error = %v", err)
	}
	metadata, err := decodeStubDeploymentMetadata(deploymentRecordFromModel(deploymentRecord))
	if err != nil {
		t.Fatalf("decodeStubDeploymentMetadata() error = %v", err)
	}

	workspace, err := workspaceManager.OpenWorkspace("runtime-environments/" + fmt.Sprint(*firstHandled.RuntimeEnvironmentID))
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	artifactPath, err := workspace.ResolvePath(metadata.ArtifactPath)
	if err != nil {
		t.Fatalf("ResolvePath() error = %v", err)
	}
	if err := os.Remove(artifactPath); err != nil {
		t.Fatalf("os.Remove() error = %v", err)
	}

	secondRequest := mustInsertPreviewStartOperationRequest(t, ctx, appStore, repository.ID, pullRequest.ID, sourceRepository, "abc123")
	if err := worker.Work(ctx, testJob(secondRequest.ID, 1, 1)); err != nil {
		t.Fatalf("second Work() error = %v", err)
	}
	secondHandled, err := appStore.Q().GetOperationRequestByID(ctx, secondRequest.ID)
	if err != nil {
		t.Fatalf("GetOperationRequestByID(second) error = %v", err)
	}
	if secondHandled.RuntimeEnvironmentID == nil || *secondHandled.RuntimeEnvironmentID == *firstHandled.RuntimeEnvironmentID {
		t.Fatalf("second runtime_environment_id = %v, want a new runtime environment id", secondHandled.RuntimeEnvironmentID)
	}

	if len(enqueuer.operationRequestIDs) != 1 {
		t.Fatalf("len(enqueued cleanup requests) = %d, want 1", len(enqueuer.operationRequestIDs))
	}
}

func TestOperationRequestWorkerCleanupTeardownRunsBeforeWorkspaceCleanup(t *testing.T) {
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

	if len(enqueuer.operationRequestIDs) != 1 {
		t.Fatalf("len(enqueued cleanup requests) = %d, want 1", len(enqueuer.operationRequestIDs))
	}

	cleanupRequest, err := appStore.Q().GetOperationRequestByID(ctx, enqueuer.operationRequestIDs[0])
	if err != nil {
		t.Fatalf("GetOperationRequestByID(cleanup) error = %v", err)
	}
	if cleanupRequest.CurrentStep != operations.StepRuntimeTeardown {
		t.Fatalf("cleanup current_step = %q, want %q", cleanupRequest.CurrentStep, operations.StepRuntimeTeardown)
	}

	backend := worker.runtimeBackend.(*stubRuntimeBackend)
	if err := worker.Work(ctx, testJob(cleanupRequest.ID, 1, 1)); err != nil {
		t.Fatalf("cleanup Work() error = %v", err)
	}

	if backend.teardownCalls != 1 {
		t.Fatalf("teardownCalls = %d, want 1", backend.teardownCalls)
	}

	firstRuntimeEnvironment, err := appStore.Q().GetRuntimeEnvironmentByID(ctx, *firstHandled.RuntimeEnvironmentID)
	if err != nil {
		t.Fatalf("GetRuntimeEnvironmentByID(first) error = %v", err)
	}
	workspaceExists, err := workspaceManager.WorkspaceExists(stringPtrValue(firstRuntimeEnvironment.WorkspaceLocator))
	if err != nil {
		t.Fatalf("WorkspaceExists(cleaned) error = %v", err)
	}
	if workspaceExists {
		t.Fatalf("workspaceExists(cleaned) = true, want false")
	}
}

func mustCreateRepositoryAndPullRequest(
	t *testing.T,
	ctx context.Context,
	appStore *store.Store,
	headSHA string,
) (sqlc.Repositories, sqlc.PullRequests, pullrequests.SourceRepository) {
	t.Helper()

	repositoryRow, err := appStore.Q().UpsertRepository(ctx, sqlc.UpsertRepositoryParams{
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
	repository := store.RepositoryFromUpsertRow(repositoryRow)
	if _, err := appStore.Q().UpsertRepositoryRuntimeSettings(ctx, sqlc.UpsertRepositoryRuntimeSettingsParams{
		RepositoryID:       repository.ID,
		ComposeFilePath:    strPtr("compose.yml"),
		ExposedServiceName: strPtr("app"),
		ExposedServicePort: int32Ptr(8080),
	}); err != nil {
		t.Fatalf("UpsertRepositoryRuntimeSettings() error = %v", err)
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
	intentSnapshot, err := operations.BuildPreviewStartSnapshot(operations.PreviewStartSnapshotInput{
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
		TriggerType:            automation.TriggerTypePreviewOnPullRequestOpened,
		OperationType:          operations.TypePreviewStart,
		RuntimeEnvironmentType: operations.RuntimeEnvironmentTypePreview,
		TargetPRHeadCommitSHA:  targetHeadSHA,
	})
	if err != nil {
		t.Fatalf("BuildPreviewStartSnapshot() error = %v", err)
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

func mustInsertPreviewDeleteOperationRequest(
	t *testing.T,
	ctx context.Context,
	appStore *store.Store,
	repositoryID, pullRequestID int64,
	sourceRepository pullrequests.SourceRepository,
	targetHeadSHA string,
	targetRuntimeEnvironment *sqlc.RuntimeEnvironments,
) sqlc.OperationRequests {
	t.Helper()

	initialStep, err := operations.InitialStepForOperation(operations.TypePreviewDelete)
	if err != nil {
		t.Fatalf("InitialStepForOperation() error = %v", err)
	}

	githubPullRequestID := int64(9001)
	input := operations.TriggerIntentSnapshotInput{
		RepositoryID:        repositoryID,
		PullRequestID:       pullRequestID,
		PRNumber:            42,
		GithubPullRequestID: &githubPullRequestID,
		PRSourceRepository:  sourceRepository,
		DeliveryID:          "delivery-delete-1",
		Event: webhook.NormalizedEvent{
			Type:                webhook.EventTypePullRequestUnlabeled,
			GithubRepositoryID:  sourceRepository.GithubRepositoryID,
			PRNumber:            42,
			GithubPullRequestID: githubPullRequestID,
			PRHeadSHA:           targetHeadSHA,
			Label:               "preview",
			PRSourceRepository:  sourceRepository,
		},
		TriggerID:              6,
		TriggerType:            automation.TriggerTypePreviewDeleteOnLabelPreviewRemoved,
		OperationType:          operations.TypePreviewDelete,
		RuntimeEnvironmentType: operations.RuntimeEnvironmentTypePreview,
		TargetPRHeadCommitSHA:  targetHeadSHA,
	}
	var targetRuntimeEnvironmentID *int64
	if targetRuntimeEnvironment != nil {
		targetRuntimeEnvironmentID = int64Ptr(targetRuntimeEnvironment.ID)
		input.TargetRuntime = &operations.TargetRuntimeSnapshot{
			ID:               targetRuntimeEnvironment.ID,
			Status:           targetRuntimeEnvironment.Status,
			WorkspaceLocator: stringPtrValue(targetRuntimeEnvironment.WorkspaceLocator),
		}
		input.TargetPRHeadCommitSHA = targetRuntimeEnvironment.TargetPrHeadCommitSha
	}

	intentSnapshot, err := operations.BuildPreviewDeleteSnapshot(input)
	if err != nil {
		t.Fatalf("BuildPreviewDeleteSnapshot() error = %v", err)
	}

	operationRequest, err := appStore.Q().InsertOperationRequest(ctx, sqlc.InsertOperationRequestParams{
		RepositoryID:               repositoryID,
		PullRequestID:              pullRequestID,
		TargetRuntimeEnvironmentID: targetRuntimeEnvironmentID,
		OperationType:              operations.TypePreviewDelete,
		Source:                     operations.SourceTrigger,
		Status:                     operations.StatusQueued,
		TargetPrHeadCommitSha:      input.TargetPRHeadCommitSHA,
		IntentSnapshotJson:         intentSnapshot,
		CurrentStep:                initialStep.Name,
		CurrentStepState:           initialStep.State,
		CurrentStepDetailsJson:     nil,
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
	runtimeBackend := &stubRuntimeBackend{}
	worker := &OperationRequestWorker{
		logger:             logger,
		store:              appStore,
		automationRegistry: automation.NewRegistry(),
		githubDownloader:   &stubTarballDownloader{results: downloadResults},
		workspaceManager:   workspaceManager,
		runtimeBackend:     runtimeBackend,
		jobEnqueuer:        enqueuer,
		operationHandlers: map[string]operationHandler{
			automation.HandlerPreviewStart:             nil,
			automation.HandlerPreviewDelete:            nil,
			automation.HandlerPreviewCleanupSuperseded: nil,
		},
	}
	worker.operationHandlers[automation.HandlerPreviewStart] = worker.handlePreviewStart
	worker.operationHandlers[automation.HandlerPreviewDelete] = worker.handlePreviewDelete
	worker.operationHandlers[automation.HandlerPreviewCleanupSuperseded] = worker.handlePreviewCleanupSuperseded
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

func int32Ptr(value int32) *int32 {
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

type stubRuntimeBackend struct {
	prepareErrors  []error
	deployErrors   []error
	teardownErrors []error
	prepareCalls   int
	deployCalls    int
	teardownCalls  int
}

type stubDeploymentMetadata struct {
	ArtifactPath string `json:"artifact_path"`
}

func (s *stubRuntimeBackend) Name() string {
	return "stub"
}

func (s *stubRuntimeBackend) Prepare(
	_ context.Context,
	request runtimebackend.PrepareRequest,
) (runtimebackend.DeploymentRecord, error) {
	if err := s.nextPrepareError(); err != nil {
		return runtimebackend.DeploymentRecord{}, err
	}

	metadata := stubDeploymentMetadata{
		ArtifactPath: ".toolshed/runtime/rendered-compose.yml",
	}
	if err := request.Workspace.WriteFile(metadata.ArtifactPath, []byte("rendered\n"), 0o644); err != nil {
		return runtimebackend.DeploymentRecord{}, err
	}

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return runtimebackend.DeploymentRecord{}, err
	}

	return runtimebackend.DeploymentRecord{
		Backend:                        s.Name(),
		FrozenRuntimeSettingsJSON:      []byte(`{"compose_file_path":"compose.yml"}`),
		FrozenEnvironmentVariablesJSON: []byte(`[]`),
		MetadataJSON:                   metadataJSON,
	}, nil
}

func (s *stubRuntimeBackend) PreparedArtifactsExist(
	_ context.Context,
	request runtimebackend.PreparedArtifactsRequest,
) (bool, error) {
	metadata, err := decodeStubDeploymentMetadata(request.Deployment)
	if err != nil {
		return false, err
	}
	return request.Workspace.FileExists(metadata.ArtifactPath)
}

func (s *stubRuntimeBackend) Deploy(
	_ context.Context,
	_ runtimebackend.DeployRequest,
) error {
	return s.nextDeployError()
}

func (s *stubRuntimeBackend) Teardown(
	_ context.Context,
	_ runtimebackend.TeardownRequest,
) error {
	return s.nextTeardownError()
}

func (s *stubRuntimeBackend) nextPrepareError() error {
	s.prepareCalls++
	if len(s.prepareErrors) == 0 {
		return nil
	}

	err := s.prepareErrors[0]
	s.prepareErrors = s.prepareErrors[1:]
	return err
}

func (s *stubRuntimeBackend) nextDeployError() error {
	s.deployCalls++
	if len(s.deployErrors) == 0 {
		return nil
	}

	err := s.deployErrors[0]
	s.deployErrors = s.deployErrors[1:]
	return err
}

func (s *stubRuntimeBackend) nextTeardownError() error {
	s.teardownCalls++
	if len(s.teardownErrors) == 0 {
		return nil
	}

	err := s.teardownErrors[0]
	s.teardownErrors = s.teardownErrors[1:]
	return err
}

func decodeStubDeploymentMetadata(
	deployment runtimebackend.DeploymentRecord,
) (stubDeploymentMetadata, error) {
	var metadata stubDeploymentMetadata
	if err := json.Unmarshal(deployment.MetadataJSON, &metadata); err != nil {
		return stubDeploymentMetadata{}, err
	}
	return metadata, nil
}

func historyKinds(entries []sqlc.OperationRequestHistoryEntries) []string {
	kinds := make([]string, 0, len(entries))
	for _, entry := range entries {
		kinds = append(kinds, entry.Kind)
	}
	return kinds
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx] != right[idx] {
			return false
		}
	}
	return true
}

func decodeHistoryDetails(t *testing.T, payload []byte) map[string]any {
	t.Helper()

	if len(payload) == 0 {
		return nil
	}

	var details map[string]any
	if err := json.Unmarshal(payload, &details); err != nil {
		t.Fatalf("json.Unmarshal(history details) error = %v", err)
	}
	return details
}
