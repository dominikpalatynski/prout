-- name: UpsertRepository :one
WITH upserted_repository AS (
    INSERT INTO repositories (
        github_repository_id,
        github_installation_id,
        owner,
        name,
        full_name,
        html_url,
        is_private
    ) VALUES (
        $1, $2, $3, $4, $5, $6, $7
    )
    ON CONFLICT (github_repository_id) DO UPDATE
    SET
        github_installation_id = EXCLUDED.github_installation_id,
        owner = EXCLUDED.owner,
        name = EXCLUDED.name,
        full_name = EXCLUDED.full_name,
        html_url = EXCLUDED.html_url,
        is_private = EXCLUDED.is_private,
        updated_at = NOW()
    RETURNING
        id,
        github_repository_id,
        github_installation_id,
        owner,
        name,
        full_name,
        html_url,
        is_private,
        enabled,
        created_at,
        updated_at
), ensured_runtime_settings AS (
    INSERT INTO repository_runtime_settings (repository_id)
    SELECT upserted_repository.id
    FROM upserted_repository
    ON CONFLICT (repository_id) DO NOTHING
)
SELECT
    id,
    github_repository_id,
    github_installation_id,
    owner,
    name,
    full_name,
    html_url,
    is_private,
    enabled,
    created_at,
    updated_at
FROM upserted_repository;

-- name: ListRepositories :many
SELECT
    id,
    github_repository_id,
    github_installation_id,
    owner,
    name,
    full_name,
    html_url,
    is_private,
    enabled,
    created_at,
    updated_at
FROM repositories
ORDER BY id ASC;

-- name: GetRepositoryByID :one
SELECT
    id,
    github_repository_id,
    github_installation_id,
    owner,
    name,
    full_name,
    html_url,
    is_private,
    enabled,
    created_at,
    updated_at
FROM repositories
WHERE id = $1;

-- name: GetRepositoryByGitHubRepositoryID :one
SELECT
    id,
    github_repository_id,
    github_installation_id,
    owner,
    name,
    full_name,
    html_url,
    is_private,
    enabled,
    created_at,
    updated_at
FROM repositories
WHERE github_repository_id = $1;

-- name: SetRepositoryEnabled :one
UPDATE repositories
SET
    enabled = $2,
    updated_at = NOW()
WHERE id = $1
RETURNING
    id,
    github_repository_id,
    github_installation_id,
    owner,
    name,
    full_name,
    html_url,
    is_private,
    enabled,
    created_at,
    updated_at;
