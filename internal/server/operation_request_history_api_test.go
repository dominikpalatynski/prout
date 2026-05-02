package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/dominikpalatynski/toolshed/internal/automation"
	"github.com/dominikpalatynski/toolshed/internal/config"
	"github.com/dominikpalatynski/toolshed/internal/operations"
	"github.com/dominikpalatynski/toolshed/internal/store"
	"github.com/dominikpalatynski/toolshed/internal/store/sqlc"
	"github.com/dominikpalatynski/toolshed/internal/testdb"
)

func TestListOperationRequestHistoryReturnsRequestScopedEntries(t *testing.T) {
	pool := testdb.Start(t)
	appStore := store.New(pool)
	ctx := context.Background()

	operationRequest := mustInsertOperationRequestForHistoryAPI(t, ctx, appStore, 501, "abc123")
	if _, err := appStore.Q().InsertOperationRequestHistoryEntry(ctx, sqlc.InsertOperationRequestHistoryEntryParams{
		OperationRequestID: operationRequest.ID,
		Kind:               operations.HistoryKindRequestStarted,
		Message:            "Started handling operation request.",
		DetailsJson:        []byte(`{"attempt":1}`),
	}); err != nil {
		t.Fatalf("InsertOperationRequestHistoryEntry(started) error = %v", err)
	}
	step := operations.StepSourceMaterialization
	if _, err := appStore.Q().InsertOperationRequestHistoryEntry(ctx, sqlc.InsertOperationRequestHistoryEntryParams{
		OperationRequestID: operationRequest.ID,
		Kind:               operations.HistoryKindStepEntered,
		Step:               &step,
		Message:            "Entered step source materialization.",
		DetailsJson:        []byte(`{"runtime_environment_id":42}`),
	}); err != nil {
		t.Fatalf("InsertOperationRequestHistoryEntry(step) error = %v", err)
	}

	server := newOperationRequestHistoryTestServer(appStore)
	router := chi.NewRouter()
	server.mount(router)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/operation-requests/%d/history", operationRequest.ID), http.NoBody)
	req.Header.Set("Authorization", "Bearer test-token")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ServeHTTP() status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body struct {
		Entries []operationRequestHistoryEntryResponse `json:"operation_request_history_entries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(body.Entries) != 2 {
		t.Fatalf("len(body.Entries) = %d, want 2", len(body.Entries))
	}
	if body.Entries[0].Kind != operations.HistoryKindRequestStarted {
		t.Fatalf("body.Entries[0].Kind = %q, want %q", body.Entries[0].Kind, operations.HistoryKindRequestStarted)
	}
	if body.Entries[1].Step == nil || *body.Entries[1].Step != operations.StepSourceMaterialization {
		t.Fatalf("body.Entries[1].Step = %v, want %q", body.Entries[1].Step, operations.StepSourceMaterialization)
	}
	if string(body.Entries[1].DetailsJSON) != `{"runtime_environment_id":42}` {
		t.Fatalf("body.Entries[1].DetailsJSON = %s, want runtime environment details", body.Entries[1].DetailsJSON)
	}
}

func TestStreamOperationRequestHistoryDeliversLiveEntries(t *testing.T) {
	pool := testdb.Start(t)
	appStore := store.New(pool)
	ctx := context.Background()

	operationRequest := mustInsertOperationRequestForHistoryAPI(t, ctx, appStore, 601, "def456")
	server := newOperationRequestHistoryTestServer(appStore)
	router := chi.NewRouter()
	server.mount(router)

	httpServer := httptest.NewServer(router)
	defer httpServer.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/api/operation-requests/%d/history/live", httpServer.URL, operationRequest.ID), http.NoBody)
	if err != nil {
		t.Fatalf("http.NewRequestWithContext() error = %v", err)
	}
	req.Header.Set("Authorization", "Bearer test-token")

	resp, err := httpServer.Client().Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want %d, body = %s", resp.StatusCode, http.StatusOK, body)
	}

	entry, err := appStore.Q().InsertOperationRequestHistoryEntry(ctx, sqlc.InsertOperationRequestHistoryEntryParams{
		OperationRequestID: operationRequest.ID,
		Kind:               operations.HistoryKindRequestStarted,
		Message:            "Started handling operation request.",
		DetailsJson:        []byte(`{"attempt":1}`),
	})
	if err != nil {
		t.Fatalf("InsertOperationRequestHistoryEntry() error = %v", err)
	}
	server.operationRequestHistoryHub.PublishOperationRequestHistoryEntry(entry)

	reader := bufio.NewReader(resp.Body)
	var dataLine string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("ReadString() error = %v", err)
		}
		if strings.HasPrefix(line, "data: ") {
			dataLine = strings.TrimSpace(strings.TrimPrefix(line, "data: "))
			break
		}
	}

	var payload operationRequestHistoryEntryResponse
	if err := json.Unmarshal([]byte(dataLine), &payload); err != nil {
		t.Fatalf("json.Unmarshal(stream payload) error = %v", err)
	}
	if payload.OperationRequestID != operationRequest.ID {
		t.Fatalf("payload.OperationRequestID = %d, want %d", payload.OperationRequestID, operationRequest.ID)
	}
	if payload.Kind != operations.HistoryKindRequestStarted {
		t.Fatalf("payload.Kind = %q, want %q", payload.Kind, operations.HistoryKindRequestStarted)
	}
}

func newOperationRequestHistoryTestServer(appStore *store.Store) *Server {
	return &Server{
		cfg: &config.Config{
			Operator: config.OperatorConfig{
				BearerToken: "test-token",
			},
		},
		logger:                     slog.New(slog.NewTextHandler(io.Discard, nil)),
		store:                      appStore,
		automationRegistry:         automation.NewRegistry(),
		operationRequestHistoryHub: newOperationRequestHistoryHub(),
	}
}

func mustInsertOperationRequestForHistoryAPI(
	t *testing.T,
	ctx context.Context,
	appStore *store.Store,
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
	repository := store.RepositoryFromUpsertRow(repositoryRow)

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
