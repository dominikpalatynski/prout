-- name: InsertOperationRequestHistoryEntry :one
INSERT INTO operation_request_history_entries (
    operation_request_id,
    kind,
    step,
    message,
    details_json
) VALUES (
    $1, $2, $3, $4, $5
)
RETURNING
    id,
    operation_request_id,
    kind,
    step,
    message,
    details_json,
    created_at;

-- name: ListOperationRequestHistoryEntriesByOperationRequestID :many
SELECT
    id,
    operation_request_id,
    kind,
    step,
    message,
    details_json,
    created_at
FROM operation_request_history_entries
WHERE operation_request_id = $1
ORDER BY id ASC;
