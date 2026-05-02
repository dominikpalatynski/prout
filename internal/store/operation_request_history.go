package store

import (
	"context"

	"github.com/dominikpalatynski/toolshed/internal/store/sqlc"
)

func (s *Store) ListOperationRequestHistoryEntries(
	ctx context.Context,
	operationRequestID int64,
) ([]sqlc.OperationRequestHistoryEntries, error) {
	return s.queries.ListOperationRequestHistoryEntriesByOperationRequestID(ctx, operationRequestID)
}
