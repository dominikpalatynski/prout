-- name: InsertRuntimeEnvironment :one
INSERT INTO runtime_environments (
    repository_id,
    pull_request_id,
    type,
    status,
    target_pr_head_commit_sha,
    source_github_repository_id,
    source_owner,
    source_name,
    source_full_name
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
)
RETURNING
    id,
    repository_id,
    pull_request_id,
    type,
    status,
    target_pr_head_commit_sha,
    created_at,
    updated_at,
    source_github_repository_id,
    source_owner,
    source_name,
    source_full_name,
    workspace_locator;

-- name: GetLatestRuntimeEnvironmentByTarget :one
SELECT
    id,
    repository_id,
    pull_request_id,
    type,
    status,
    target_pr_head_commit_sha,
    created_at,
    updated_at,
    source_github_repository_id,
    source_owner,
    source_name,
    source_full_name,
    workspace_locator
FROM runtime_environments
WHERE repository_id = $1
  AND pull_request_id = $2
  AND type = $3
  AND target_pr_head_commit_sha = $4
ORDER BY id DESC
LIMIT 1;

-- name: ListActiveRuntimeEnvironmentsByPullRequestAndType :many
SELECT
    id,
    repository_id,
    pull_request_id,
    type,
    status,
    target_pr_head_commit_sha,
    created_at,
    updated_at,
    source_github_repository_id,
    source_owner,
    source_name,
    source_full_name,
    workspace_locator
FROM runtime_environments
WHERE pull_request_id = $1
  AND type = $2
  AND status IN ('preparing', 'prepared')
ORDER BY id DESC;

-- name: GetRuntimeEnvironmentByID :one
SELECT
    id,
    repository_id,
    pull_request_id,
    type,
    status,
    target_pr_head_commit_sha,
    created_at,
    updated_at,
    source_github_repository_id,
    source_owner,
    source_name,
    source_full_name,
    workspace_locator
FROM runtime_environments
WHERE id = $1;

-- name: UpdateRuntimeEnvironmentStatus :one
UPDATE runtime_environments
SET
    status = $2,
    updated_at = NOW()
WHERE id = $1
RETURNING
    id,
    repository_id,
    pull_request_id,
    type,
    status,
    target_pr_head_commit_sha,
    created_at,
    updated_at,
    source_github_repository_id,
    source_owner,
    source_name,
    source_full_name,
    workspace_locator;
