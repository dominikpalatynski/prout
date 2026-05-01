package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	applog "github.com/dominikpalatynski/toolshed/internal/log"
	"github.com/dominikpalatynski/toolshed/internal/store"
	"github.com/dominikpalatynski/toolshed/internal/store/sqlc"
)

type TriggerDispatchArgs struct {
	TriggerDispatchID int64 `json:"trigger_dispatch_id"`
}

func (TriggerDispatchArgs) Kind() string { return "trigger_dispatch" }

type TriggerDispatchWorker struct {
	river.WorkerDefaults[TriggerDispatchArgs]

	logger *slog.Logger
	store  *store.Store
}

func NewWorkers(appStore *store.Store, logger *slog.Logger) *river.Workers {
	workers := river.NewWorkers()
	river.AddWorker(workers, &TriggerDispatchWorker{
		logger: logger,
		store:  appStore,
	})
	return workers
}

func (w *TriggerDispatchWorker) Work(ctx context.Context, job *river.Job[TriggerDispatchArgs]) error {
	ctx = applog.WithJobID(ctx, job.ID)

	dispatchRecord, err := w.store.Q().GetTriggerDispatchByID(ctx, job.Args.TriggerDispatchID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("trigger dispatch %d not found", job.Args.TriggerDispatchID)
		}
		return fmt.Errorf("load trigger dispatch: %w", err)
	}

	ctx = applog.WithRepoID(ctx, dispatchRecord.RepositoryID)

	w.logger.InfoContext(ctx, "processing trigger dispatch",
		slog.Int64("trigger_dispatch_id", dispatchRecord.ID),
		slog.String("dispatch_type", dispatchRecord.DispatchType),
		slog.String("dispatch_status", dispatchRecord.Status),
	)

	if dispatchRecord.Status == "processed" {
		w.logger.InfoContext(ctx, "trigger dispatch already processed",
			slog.Int64("trigger_dispatch_id", dispatchRecord.ID),
		)
		return nil
	}

	if _, err := w.store.Q().MarkTriggerDispatchProcessed(ctx, dispatchRecord.ID); err != nil {
		message := err.Error()
		if _, updateErr := w.store.Q().MarkTriggerDispatchFailed(ctx, sqlc.MarkTriggerDispatchFailedParams{
			ID:        dispatchRecord.ID,
			LastError: &message,
		}); updateErr != nil {
			return errors.Join(fmt.Errorf("mark trigger dispatch processed: %w", err), fmt.Errorf("mark trigger dispatch failed: %w", updateErr))
		}
		return fmt.Errorf("mark trigger dispatch processed: %w", err)
	}

	w.logger.InfoContext(ctx, "processed trigger dispatch",
		slog.Int64("trigger_dispatch_id", dispatchRecord.ID),
		slog.String("dispatch_type", dispatchRecord.DispatchType),
	)

	return nil
}
