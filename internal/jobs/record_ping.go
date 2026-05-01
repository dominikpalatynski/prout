package jobs

import (
	"context"
	"fmt"
	"log/slog"

	applog "github.com/dominikpalatynski/toolshed/internal/log"
	"github.com/dominikpalatynski/toolshed/internal/store"
	"github.com/riverqueue/river"
)

type RecordPingArgs struct {
	DeliveryID   string `json:"delivery_id"`
	Event        string `json:"event"`
	RepositoryID int64  `json:"repository_id"`
	PRNumber     int    `json:"pr_number"`
	SHA          string `json:"sha"`
}

func (RecordPingArgs) Kind() string { return "record_ping" }

type RecordPingWorker struct {
	river.WorkerDefaults[RecordPingArgs]

	logger *slog.Logger
	store  *store.Store
}

func NewWorkers(pingStore *store.Store, logger *slog.Logger) *river.Workers {
	workers := river.NewWorkers()
	river.AddWorker(workers, &RecordPingWorker{
		logger: logger,
		store:  pingStore,
	})
	return workers
}

func (w *RecordPingWorker) Work(ctx context.Context, job *river.Job[RecordPingArgs]) error {
	ctx = applog.WithJobID(ctx, job.ID)
	ctx = applog.WithRepoID(ctx, job.Args.RepositoryID)
	ctx = applog.WithPRNumber(ctx, job.Args.PRNumber)

	w.logger.InfoContext(ctx, "recording ping job",
		slog.String("delivery_id", job.Args.DeliveryID),
		slog.String("event", job.Args.Event),
		slog.String("sha", job.Args.SHA),
	)

	if _, err := w.store.Q().InsertPing(ctx); err != nil {
		return fmt.Errorf("insert ping: %w", err)
	}

	w.logger.InfoContext(ctx, "recorded ping job")
	return nil
}
