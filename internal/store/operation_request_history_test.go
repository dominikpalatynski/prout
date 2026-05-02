package store

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/dominikpalatynski/toolshed/internal/operations"
	"github.com/dominikpalatynski/toolshed/internal/store/sqlc"
	"github.com/dominikpalatynski/toolshed/internal/testdb"
)

func TestListOperationRequestHistoryEntriesReturnsStableChronologicalEntriesForOneRequest(t *testing.T) {
	pool := testdb.Start(t)
	appStore := New(pool)
	ctx := context.Background()

	firstRequest := mustInsertOperationRequestForHistory(t, ctx, appStore, 501, "abc123")
	secondRequest := mustInsertOperationRequestForHistory(t, ctx, appStore, 502, "def456")

	firstStarted, err := appStore.Q().InsertOperationRequestHistoryEntry(ctx, sqlc.InsertOperationRequestHistoryEntryParams{
		OperationRequestID: firstRequest.ID,
		Kind:               operations.HistoryKindRequestStarted,
		Message:            "Started handling operation request.",
		DetailsJson:        []byte(`{"attempt":1}`),
	})
	if err != nil {
		t.Fatalf("InsertOperationRequestHistoryEntry(firstStarted) error = %v", err)
	}

	if _, err := appStore.Q().InsertOperationRequestHistoryEntry(ctx, sqlc.InsertOperationRequestHistoryEntryParams{
		OperationRequestID: secondRequest.ID,
		Kind:               operations.HistoryKindRequestStarted,
		Message:            "Started handling operation request.",
		DetailsJson:        []byte(`{"attempt":1}`),
	}); err != nil {
		t.Fatalf("InsertOperationRequestHistoryEntry(secondStarted) error = %v", err)
	}

	firstStep := operations.StepSourceMaterialization
	firstEntered, err := appStore.Q().InsertOperationRequestHistoryEntry(ctx, sqlc.InsertOperationRequestHistoryEntryParams{
		OperationRequestID: firstRequest.ID,
		Kind:               operations.HistoryKindStepEntered,
		Step:               &firstStep,
		Message:            "Entered step source materialization.",
		DetailsJson:        []byte(`{"runtime_environment_id":42}`),
	})
	if err != nil {
		t.Fatalf("InsertOperationRequestHistoryEntry(firstEntered) error = %v", err)
	}

	entries, err := appStore.ListOperationRequestHistoryEntries(ctx, firstRequest.ID)
	if err != nil {
		t.Fatalf("ListOperationRequestHistoryEntries() error = %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}
	if entries[0].ID != firstStarted.ID || entries[1].ID != firstEntered.ID {
		t.Fatalf("entry ids = [%d %d], want [%d %d]", entries[0].ID, entries[1].ID, firstStarted.ID, firstEntered.ID)
	}
	if entries[0].Kind != operations.HistoryKindRequestStarted {
		t.Fatalf("entries[0].Kind = %q, want %q", entries[0].Kind, operations.HistoryKindRequestStarted)
	}
	if entries[1].Step == nil || *entries[1].Step != operations.StepSourceMaterialization {
		t.Fatalf("entries[1].Step = %v, want %q", entries[1].Step, operations.StepSourceMaterialization)
	}

	var details map[string]any
	if err := json.Unmarshal(entries[1].DetailsJson, &details); err != nil {
		t.Fatalf("json.Unmarshal(entries[1].DetailsJson) error = %v", err)
	}
	if got := details["runtime_environment_id"]; got != float64(42) {
		t.Fatalf("runtime_environment_id = %v, want 42", got)
	}
}

func mustInsertOperationRequestForHistory(
	t *testing.T,
	ctx context.Context,
	appStore *Store,
	githubRepositoryID int64,
	headSHA string,
) sqlc.OperationRequests {
	t.Helper()

	repositoryRow, err := appStore.Q().UpsertRepository(ctx, sqlc.UpsertRepositoryParams{
		GithubRepositoryID:   githubRepositoryID,
		GithubInstallationID: githubRepositoryID + 1000,
		Owner:                "acme",
		Name:                 fmt.Sprintf("demo-%d", githubRepositoryID),
		FullName:             fmt.Sprintf("acme/demo-%d", githubRepositoryID),
		HtmlUrl:              fmt.Sprintf("https://github.com/acme/demo-%d", githubRepositoryID),
		IsPrivate:            false,
	})
	if err != nil {
		t.Fatalf("UpsertRepository() error = %v", err)
	}
	repository := RepositoryFromUpsertRow(repositoryRow)

	pullRequest, err := appStore.Q().UpsertPullRequestAnchor(ctx, sqlc.UpsertPullRequestAnchorParams{
		RepositoryID:                    repository.ID,
		PRNumber:                        githubRepositoryID,
		GithubPullRequestID:             &githubRepositoryID,
		CurrentHeadCommitSha:            headSHA,
		CurrentSourceGithubRepositoryID: repository.GithubRepositoryID,
		CurrentSourceOwner:              repository.Owner,
		CurrentSourceName:               repository.Name,
		CurrentSourceFullName:           repository.FullName,
	})
	if err != nil {
		t.Fatalf("UpsertPullRequestAnchor() error = %v", err)
	}

	initialStep, err := operations.InitialStepForOperation(operations.TypePreviewStart)
	if err != nil {
		t.Fatalf("InitialStepForOperation() error = %v", err)
	}

	operationRequest, err := appStore.Q().InsertOperationRequest(ctx, sqlc.InsertOperationRequestParams{
		RepositoryID:           repository.ID,
		PullRequestID:          pullRequest.ID,
		OperationType:          operations.TypePreviewStart,
		Source:                 operations.SourceTrigger,
		Status:                 operations.StatusQueued,
		TargetPrHeadCommitSha:  headSHA,
		IntentSnapshotJson:     []byte(`{"target":{"target_pr_head_commit_sha":"` + headSHA + `"}}`),
		CurrentStep:            initialStep.Name,
		CurrentStepState:       initialStep.State,
		CurrentStepDetailsJson: nil,
	})
	if err != nil {
		t.Fatalf("InsertOperationRequest() error = %v", err)
	}

	return operationRequest
}
