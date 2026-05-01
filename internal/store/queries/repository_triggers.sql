-- name: UpsertRepositoryTrigger :one
INSERT INTO repository_triggers (
    repository_id,
    type,
    event_family,
    identity_key,
    config_json,
    enabled
) VALUES (
    $1, $2, $3, $4, $5, $6
)
ON CONFLICT (repository_id, identity_key) DO UPDATE
SET
    type = EXCLUDED.type,
    event_family = EXCLUDED.event_family,
    config_json = EXCLUDED.config_json,
    enabled = EXCLUDED.enabled,
    updated_at = NOW()
RETURNING
    id,
    repository_id,
    type,
    event_family,
    identity_key,
    config_json,
    enabled,
    created_at,
    updated_at;

-- name: ListRepositoryTriggers :many
SELECT
    id,
    repository_id,
    type,
    event_family,
    identity_key,
    config_json,
    enabled,
    created_at,
    updated_at
FROM repository_triggers
WHERE repository_id = $1
ORDER BY id ASC;

-- name: ListEnabledRepositoryTriggers :many
SELECT
    id,
    repository_id,
    type,
    event_family,
    identity_key,
    config_json,
    enabled,
    created_at,
    updated_at
FROM repository_triggers
WHERE repository_id = $1
  AND enabled = TRUE
ORDER BY id ASC;

-- name: GetRepositoryTriggerByID :one
SELECT
    id,
    repository_id,
    type,
    event_family,
    identity_key,
    config_json,
    enabled,
    created_at,
    updated_at
FROM repository_triggers
WHERE repository_id = $1
  AND id = $2;

-- name: SetRepositoryTriggerEnabled :one
UPDATE repository_triggers
SET
    enabled = $3,
    updated_at = NOW()
WHERE repository_id = $1
  AND id = $2
RETURNING
    id,
    repository_id,
    type,
    event_family,
    identity_key,
    config_json,
    enabled,
    created_at,
    updated_at;
