package store

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dominikpalatynski/toolshed/internal/store/sqlc"
)

// Store wraps the connection pool and the sqlc-generated query set.
// Single-query reads call s.Q().GetXByY(ctx, ...) directly.
// Composite operations (cross-table or state+job) live as methods on *Store.
type Store struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{
		pool:    pool,
		queries: sqlc.New(pool),
	}
}

func (s *Store) Pool() *pgxpool.Pool { return s.pool }

func (s *Store) Q() *sqlc.Queries { return s.queries }

// Tx executes fn in a transaction with automatic rollback on error.
// For future state+job coupling, use this instead of beginning pool
// transactions directly so queries stay on the same sqlc boundary.
func (s *Store) Tx(ctx context.Context, fn func(*sqlc.Queries, pgx.Tx) error) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.queries.WithTx(tx), tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
