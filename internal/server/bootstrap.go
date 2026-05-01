package server

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/dominikpalatynski/toolshed/internal/config"
	"github.com/dominikpalatynski/toolshed/internal/jobs"
	"github.com/dominikpalatynski/toolshed/internal/store"
	"github.com/dominikpalatynski/toolshed/migrations"
)

func bootstrap(ctx context.Context, cfg *config.Config, logger *slog.Logger) (*pgxpool.Pool, *store.Store, *river.Client[pgx.Tx], error) {
	pool, err := store.NewPool(ctx, cfg.DB.DSN)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("open postgres pool: %w", err)
	}

	closePoolOnError := func(err error) (*pgxpool.Pool, *store.Store, *river.Client[pgx.Tx], error) {
		pool.Close()
		return nil, nil, nil, err
	}

	if err := pool.Ping(ctx); err != nil {
		return closePoolOnError(fmt.Errorf("ping postgres: %w", err))
	}

	sqlDB := stdlib.OpenDBFromPool(pool)
	defer sqlDB.Close()

	current, target, err := migrations.Versions(ctx, sqlDB)
	if err != nil {
		return closePoolOnError(fmt.Errorf("check schema version: %w", err))
	}

	hasPending, err := migrations.HasPending(ctx, sqlDB)
	if err != nil {
		return closePoolOnError(fmt.Errorf("check pending migrations: %w", err))
	}
	if hasPending {
		return closePoolOnError(fmt.Errorf("database schema is out of date (current=%d target=%d): run `toolshed migrate up` or `task migrate:up` before starting the server", current, target))
	}
	if current > target {
		logger.WarnContext(ctx, "database schema is ahead of this binary",
			slog.Int64("current", current),
			slog.Int64("target", target),
		)
	}

	pingStore := store.New(pool)
	workers := jobs.NewWorkers(pingStore, logger)

	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Logger: logger,
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: 1},
		},
		Workers: workers,
	})
	if err != nil {
		return closePoolOnError(fmt.Errorf("init river client: %w", err))
	}

	return pool, pingStore, riverClient, nil
}
