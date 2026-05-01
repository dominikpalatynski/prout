-- name: InsertOperationRequest :one
INSERT INTO operation_requests (
    webhook_event_id,
    webhook_event_trigger_evaluation_id,
    repository_id,
    repository_trigger_id,
    pull_request_id,
    operation_type,
    source,
    status,
    target_pr_head_commit_sha,
    intent_snapshot_json
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
)
RETURNING
    id,
    webhook_event_id,
    webhook_event_trigger_evaluation_id,
    repository_id,
    repository_trigger_id,
    pull_request_id,
    runtime_environment_id,
    target_runtime_environment_id,
    operation_type,
    source,
    status,
    target_pr_head_commit_sha,
    intent_snapshot_json,
    outcome,
    last_error,
    handled_at,
    created_at;

-- name: GetOperationRequestByID :one
SELECT
    id,
    webhook_event_id,
    webhook_event_trigger_evaluation_id,
    repository_id,
    repository_trigger_id,
    pull_request_id,
    runtime_environment_id,
    target_runtime_environment_id,
    operation_type,
    source,
    status,
    target_pr_head_commit_sha,
    intent_snapshot_json,
    outcome,
    last_error,
    handled_at,
    created_at
FROM operation_requests
WHERE id = $1;

-- name: MarkOperationRequestHandled :one
UPDATE operation_requests
SET
    runtime_environment_id = $2,
    status = 'handled',
    outcome = $3,
    last_error = NULL,
    handled_at = NOW()
WHERE id = $1
RETURNING
    id,
    webhook_event_id,
    webhook_event_trigger_evaluation_id,
    repository_id,
    repository_trigger_id,
    pull_request_id,
    runtime_environment_id,
    target_runtime_environment_id,
    operation_type,
    source,
    status,
    target_pr_head_commit_sha,
    intent_snapshot_json,
    outcome,
    last_error,
    handled_at,
    created_at;

-- name: MarkOperationRequestFailed :one
UPDATE operation_requests
SET
    status = 'failed',
    outcome = $2,
    last_error = $3,
    handled_at = NOW()
WHERE id = $1
RETURNING
    id,
    webhook_event_id,
    webhook_event_trigger_evaluation_id,
    repository_id,
    repository_trigger_id,
    pull_request_id,
    runtime_environment_id,
    target_runtime_environment_id,
    operation_type,
    source,
    status,
    target_pr_head_commit_sha,
    intent_snapshot_json,
    outcome,
    last_error,
    handled_at,
    created_at;

-- name: ListWebhookEventOperationRequests :many
SELECT
    id,
    webhook_event_id,
    webhook_event_trigger_evaluation_id,
    repository_id,
    repository_trigger_id,
    pull_request_id,
    runtime_environment_id,
    target_runtime_environment_id,
    operation_type,
    source,
    status,
    target_pr_head_commit_sha,
    intent_snapshot_json,
    outcome,
    last_error,
    handled_at,
    created_at
FROM operation_requests
WHERE operation_requests.webhook_event_id = $1
ORDER BY operation_requests.id ASC;
