-- name: EnsureRepositoryEventFamilies :exec
INSERT INTO repository_event_families (
    repository_id,
    event_family_key,
    enabled
)
SELECT
    sqlc.arg(repository_id),
    unnest(sqlc.arg(event_family_keys)::text[]),
    TRUE
ON CONFLICT (repository_id, event_family_key) DO NOTHING;

-- name: ListRepositoryEventFamilies :many
SELECT
    id,
    repository_id,
    event_family_key,
    enabled,
    created_at,
    updated_at
FROM repository_event_families
WHERE repository_id = $1
ORDER BY id ASC;

-- name: GetRepositoryEventFamily :one
SELECT
    id,
    repository_id,
    event_family_key,
    enabled,
    created_at,
    updated_at
FROM repository_event_families
WHERE repository_id = $1
  AND event_family_key = $2;

-- name: UpsertRepositoryEventFamily :one
INSERT INTO repository_event_families (
    repository_id,
    event_family_key,
    enabled
) VALUES (
    $1, $2, $3
)
ON CONFLICT (repository_id, event_family_key) DO UPDATE
SET
    enabled = EXCLUDED.enabled,
    updated_at = NOW()
RETURNING
    id,
    repository_id,
    event_family_key,
    enabled,
    created_at,
    updated_at;

-- name: SetRepositoryEventFamilyEnabled :one
UPDATE repository_event_families
SET
    enabled = $3,
    updated_at = NOW()
WHERE repository_id = $1
  AND event_family_key = $2
RETURNING
    id,
    repository_id,
    event_family_key,
    enabled,
    created_at,
    updated_at;
