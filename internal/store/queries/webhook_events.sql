-- name: InsertWebhookEvent :one
INSERT INTO webhook_events (
    delivery_id,
    github_event,
    event_type,
    github_repository_id,
    status,
    payload_json
) VALUES (
    $1, $2, $3, $4, $5, $6
)
ON CONFLICT (delivery_id) DO NOTHING
RETURNING
    id,
    delivery_id,
    github_event,
    event_type,
    repository_id,
    github_repository_id,
    status,
    ignored_reason,
    failure_message,
    payload_json,
    created_at;

-- name: MarkWebhookEventIgnored :one
UPDATE webhook_events
SET
    repository_id = $2,
    status = 'ignored',
    ignored_reason = $3,
    failure_message = NULL
WHERE id = $1
RETURNING
    id,
    delivery_id,
    github_event,
    event_type,
    repository_id,
    github_repository_id,
    status,
    ignored_reason,
    failure_message,
    payload_json,
    created_at;

-- name: MarkWebhookEventProcessed :one
UPDATE webhook_events
SET
    repository_id = $2,
    status = 'processed',
    ignored_reason = NULL,
    failure_message = NULL
WHERE id = $1
RETURNING
    id,
    delivery_id,
    github_event,
    event_type,
    repository_id,
    github_repository_id,
    status,
    ignored_reason,
    failure_message,
    payload_json,
    created_at;

-- name: MarkWebhookEventFailed :one
UPDATE webhook_events
SET
    repository_id = $2,
    status = 'failed',
    ignored_reason = NULL,
    failure_message = $3
WHERE id = $1
RETURNING
    id,
    delivery_id,
    github_event,
    event_type,
    repository_id,
    github_repository_id,
    status,
    ignored_reason,
    failure_message,
    payload_json,
    created_at;

-- name: GetWebhookEventByID :one
SELECT
    id,
    delivery_id,
    github_event,
    event_type,
    repository_id,
    github_repository_id,
    status,
    ignored_reason,
    failure_message,
    payload_json,
    created_at
FROM webhook_events
WHERE id = $1;

-- name: ListWebhookEventTriggerEvaluationsByWebhookEventID :many
SELECT
    id,
    webhook_event_id,
    repository_trigger_id,
    matched,
    reason,
    trigger_snapshot_json,
    created_at
FROM webhook_event_trigger_evaluations
WHERE webhook_event_id = $1
ORDER BY id ASC;

-- name: InsertWebhookEventTriggerEvaluation :one
INSERT INTO webhook_event_trigger_evaluations (
    webhook_event_id,
    repository_trigger_id,
    matched,
    reason,
    trigger_snapshot_json
) VALUES (
    $1, $2, $3, $4, $5
)
RETURNING
    id,
    webhook_event_id,
    repository_trigger_id,
    matched,
    reason,
    trigger_snapshot_json,
    created_at;

-- name: InsertTriggerDispatch :one
INSERT INTO trigger_dispatches (
    webhook_event_id,
    webhook_event_trigger_evaluation_id,
    repository_id,
    repository_trigger_id,
    dispatch_type,
    status,
    dispatch_payload_json
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
)
RETURNING
    id,
    webhook_event_id,
    webhook_event_trigger_evaluation_id,
    repository_id,
    repository_trigger_id,
    dispatch_type,
    status,
    dispatch_payload_json,
    last_error,
    processed_at,
    created_at;

-- name: ListTriggerDispatchesByWebhookEventID :many
SELECT
    id,
    webhook_event_id,
    webhook_event_trigger_evaluation_id,
    repository_id,
    repository_trigger_id,
    dispatch_type,
    status,
    dispatch_payload_json,
    last_error,
    processed_at,
    created_at
FROM trigger_dispatches
WHERE webhook_event_id = $1
ORDER BY id ASC;

-- name: GetTriggerDispatchByID :one
SELECT
    id,
    webhook_event_id,
    webhook_event_trigger_evaluation_id,
    repository_id,
    repository_trigger_id,
    dispatch_type,
    status,
    dispatch_payload_json,
    last_error,
    processed_at,
    created_at
FROM trigger_dispatches
WHERE id = $1;

-- name: MarkTriggerDispatchProcessed :one
UPDATE trigger_dispatches
SET
    status = 'processed',
    last_error = NULL,
    processed_at = NOW()
WHERE id = $1
RETURNING
    id,
    webhook_event_id,
    webhook_event_trigger_evaluation_id,
    repository_id,
    repository_trigger_id,
    dispatch_type,
    status,
    dispatch_payload_json,
    last_error,
    processed_at,
    created_at;

-- name: MarkTriggerDispatchFailed :one
UPDATE trigger_dispatches
SET
    status = 'failed',
    last_error = $2
WHERE id = $1
RETURNING
    id,
    webhook_event_id,
    webhook_event_trigger_evaluation_id,
    repository_id,
    repository_trigger_id,
    dispatch_type,
    status,
    dispatch_payload_json,
    last_error,
    processed_at,
    created_at;
