package store

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	// Re-enable after the first `task generate:sqlc` run; see scaffold_tech.md §14.3.
	// "github.com/dominikpalatynski/toolshed/internal/store/sqlc"
)

// Store wraps the connection pool and the sqlc-generated query set.
// Single-query reads call s.Q().GetXByY(ctx, ...) directly.
// Composite operations (cross-table or state+job) live as methods on *Store.
type Store struct {
	pool *pgxpool.Pool
	// queries *sqlc.Queries
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// func (s *Store) Q() *sqlc.Queries { return s.queries }

// Tx executes fn in a transaction with automatic rollback on error.
// For ADR-004 coupling (state insert + River job insert in one transaction), use this —
// never s.pool.BeginTx directly.
func (s *Store) Tx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
