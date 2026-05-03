-- name: UpsertPullRequestAnchor :one
INSERT INTO pull_requests (
    repository_id,
    pr_number,
    github_pull_request_id,
    current_head_commit_sha,
    current_source_github_repository_id,
    current_source_owner,
    current_source_name,
    current_source_full_name
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
)
ON CONFLICT (repository_id, pr_number) DO UPDATE
SET
    github_pull_request_id = COALESCE(EXCLUDED.github_pull_request_id, pull_requests.github_pull_request_id),
    current_head_commit_sha = EXCLUDED.current_head_commit_sha,
    current_source_github_repository_id = EXCLUDED.current_source_github_repository_id,
    current_source_owner = EXCLUDED.current_source_owner,
    current_source_name = EXCLUDED.current_source_name,
    current_source_full_name = EXCLUDED.current_source_full_name,
    updated_at = NOW()
RETURNING
    id,
    repository_id,
    pr_number,
    github_pull_request_id,
    current_head_commit_sha,
    created_at,
    updated_at,
    current_source_github_repository_id,
    current_source_owner,
    current_source_name,
    current_source_full_name;

-- name: GetPullRequestByRepositoryIDAndPRNumber :one
SELECT
    id,
    repository_id,
    pr_number,
    github_pull_request_id,
    current_head_commit_sha,
    created_at,
    updated_at,
    current_source_github_repository_id,
    current_source_owner,
    current_source_name,
    current_source_full_name
FROM pull_requests
WHERE repository_id = $1
  AND pr_number = $2;
