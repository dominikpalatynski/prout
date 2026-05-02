package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/dominikpalatynski/toolshed/internal/config"
	"github.com/dominikpalatynski/toolshed/internal/githubapp"
	applog "github.com/dominikpalatynski/toolshed/internal/log"
	"github.com/dominikpalatynski/toolshed/internal/operations"
	"github.com/dominikpalatynski/toolshed/internal/store"
	"github.com/dominikpalatynski/toolshed/internal/store/sqlc"
)

type OperationRequestArgs struct {
	OperationRequestID int64 `json:"operation_request_id"`
}

func (OperationRequestArgs) Kind() string { return "operation_request" }

type OperationRequestWorker struct {
	river.WorkerDefaults[OperationRequestArgs]

	logger *slog.Logger
	store  *store.Store

	githubDownloader githubapp.TarballDownloader
	workspaceManager workspaceManager
	jobEnqueuer      operationRequestEnqueuer
	jobTimeout       time.Duration
}

type operationRequestEnqueuer interface {
	InsertTx(context.Context, pgx.Tx, river.JobArgs, *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

func (w *OperationRequestWorker) SetJobEnqueuer(enqueuer operationRequestEnqueuer) {
	w.jobEnqueuer = enqueuer
}

func (w *OperationRequestWorker) SetJobTimeout(timeout time.Duration) {
	w.jobTimeout = timeout
}

func NewWorkers(
	appStore *store.Store,
	logger *slog.Logger,
	githubDownloader githubapp.TarballDownloader,
	workspaceManager workspaceManager,
) (*river.Workers, *OperationRequestWorker) {
	workers := river.NewWorkers()
	worker := &OperationRequestWorker{
		logger:           logger,
		store:            appStore,
		githubDownloader: githubDownloader,
		workspaceManager: workspaceManager,
	}
	river.AddWorker(workers, worker)
	return workers, worker
}

func (w *OperationRequestWorker) Timeout(*river.Job[OperationRequestArgs]) time.Duration {
	if w.jobTimeout > 0 {
		return w.jobTimeout
	}
	return config.DefaultOperationRequestJobTimeout
}

func (w *OperationRequestWorker) Work(ctx context.Context, job *river.Job[OperationRequestArgs]) error {
	ctx = applog.WithJobID(ctx, job.ID)

	operationRequest, err := w.store.Q().GetOperationRequestByID(ctx, job.Args.OperationRequestID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("operation request %d not found", job.Args.OperationRequestID)
		}
		return fmt.Errorf("load operation request: %w", err)
	}

	ctx = applog.WithRepoID(ctx, operationRequest.RepositoryID)

	w.logger.InfoContext(ctx, "processing operation request",
		slog.Int64("operation_request_id", operationRequest.ID),
		slog.String("operation_type", operationRequest.OperationType),
		slog.String("operation_request_status", operationRequest.Status),
	)

	if operationRequest.Status == operations.StatusHandled {
		w.logger.InfoContext(ctx, "operation request already handled",
			slog.Int64("operation_request_id", operationRequest.ID),
		)
		return nil
	}

	if err := w.handleOperationRequest(ctx, operationRequest); err != nil {
		if job.JobRow.Attempt >= job.JobRow.MaxAttempts {
			if updateErr := w.finalizeOperationRequestFailure(ctx, operationRequest.ID, err); updateErr != nil {
				return errors.Join(fmt.Errorf("handle operation request: %w", err), fmt.Errorf("mark operation request failed: %w", updateErr))
			}
		}
		return fmt.Errorf("handle operation request: %w", err)
	}

	w.logger.InfoContext(ctx, "handled operation request",
		slog.Int64("operation_request_id", operationRequest.ID),
		slog.String("operation_type", operationRequest.OperationType),
	)

	return nil
}

func (w *OperationRequestWorker) handleOperationRequest(ctx context.Context, operationRequest sqlc.OperationRequests) error {
	switch operationRequest.OperationType {
	case operations.TypePreviewStart:
		return w.handlePreviewStart(ctx, operationRequest)
	case operations.TypePreviewCleanupSuperseded:
		return w.handlePreviewCleanupSuperseded(ctx, operationRequest)
	default:
		return fmt.Errorf("unsupported operation type %q", operationRequest.OperationType)
	}
}

func strPtr(value string) *string {
	return &value
}
