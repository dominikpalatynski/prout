package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

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
}

func NewWorkers(appStore *store.Store, logger *slog.Logger) *river.Workers {
	workers := river.NewWorkers()
	river.AddWorker(workers, &OperationRequestWorker{
		logger: logger,
		store:  appStore,
	})
	return workers
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
		message := err.Error()
		if _, updateErr := w.store.Q().MarkOperationRequestFailed(ctx, sqlc.MarkOperationRequestFailedParams{
			ID:        operationRequest.ID,
			Outcome:   strPtr(operations.OutcomeOperationFailed),
			LastError: &message,
		}); updateErr != nil {
			return errors.Join(fmt.Errorf("handle operation request: %w", err), fmt.Errorf("mark operation request failed: %w", updateErr))
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
	default:
		return fmt.Errorf("unsupported operation type %q", operationRequest.OperationType)
	}
}

func (w *OperationRequestWorker) handlePreviewStart(ctx context.Context, operationRequest sqlc.OperationRequests) error {
	return w.store.Tx(ctx, func(q *sqlc.Queries, _ pgx.Tx) error {
		runtimeEnvironmentType, err := operations.RuntimeEnvironmentTypeForOperation(operationRequest.OperationType)
		if err != nil {
			return err
		}

		runtimeEnvironment, outcome, err := ensureRuntimeEnvironment(ctx, q, ensureRuntimeEnvironmentParams{
			RepositoryID:           operationRequest.RepositoryID,
			PullRequestID:          operationRequest.PullRequestID,
			RuntimeEnvironmentType: runtimeEnvironmentType,
			TargetPRHeadCommitSHA:  operationRequest.TargetPrHeadCommitSha,
		})
		if err != nil {
			return err
		}

		if _, err := q.MarkOperationRequestHandled(ctx, sqlc.MarkOperationRequestHandledParams{
			ID:                   operationRequest.ID,
			RuntimeEnvironmentID: &runtimeEnvironment.ID,
			Outcome:              &outcome,
		}); err != nil {
			return fmt.Errorf("mark operation request handled: %w", err)
		}

		return nil
	})
}

type ensureRuntimeEnvironmentParams struct {
	RepositoryID           int64
	PullRequestID          int64
	RuntimeEnvironmentType string
	TargetPRHeadCommitSHA  string
}

func ensureRuntimeEnvironment(ctx context.Context, q *sqlc.Queries, params ensureRuntimeEnvironmentParams) (sqlc.RuntimeEnvironments, string, error) {
	runtimeEnvironment, err := q.GetLatestRuntimeEnvironmentByTarget(ctx, sqlc.GetLatestRuntimeEnvironmentByTargetParams{
		RepositoryID:          params.RepositoryID,
		PullRequestID:         params.PullRequestID,
		Type:                  params.RuntimeEnvironmentType,
		TargetPrHeadCommitSha: params.TargetPRHeadCommitSHA,
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return sqlc.RuntimeEnvironments{}, "", fmt.Errorf("lookup runtime environment: %w", err)
	}

	if err == nil {
		switch runtimeEnvironment.Status {
		case operations.RuntimeStatusPreparing:
			return runtimeEnvironment, operations.OutcomeAlreadyPreparing, nil
		case operations.RuntimeStatusPrepared:
			return runtimeEnvironment, operations.OutcomeAlreadyPrepared, nil
		}
	}

	runtimeEnvironment, err = q.InsertRuntimeEnvironment(ctx, sqlc.InsertRuntimeEnvironmentParams{
		RepositoryID:          params.RepositoryID,
		PullRequestID:         params.PullRequestID,
		Type:                  params.RuntimeEnvironmentType,
		Status:                operations.RuntimeStatusPreparing,
		TargetPrHeadCommitSha: params.TargetPRHeadCommitSHA,
	})
	if err != nil {
		return sqlc.RuntimeEnvironments{}, "", fmt.Errorf("insert runtime environment: %w", err)
	}

	return runtimeEnvironment, operations.OutcomeNewAttemptCreated, nil
}

func strPtr(value string) *string {
	return &value
}
