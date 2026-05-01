-- name: UpsertPullRequestAnchor :one
INSERT INTO pull_requests (
    repository_id,
    pr_number,
    github_pull_request_id,
    current_head_commit_sha
) VALUES (
    $1, $2, $3, $4
)
ON CONFLICT (repository_id, pr_number) DO UPDATE
SET
    github_pull_request_id = COALESCE(EXCLUDED.github_pull_request_id, pull_requests.github_pull_request_id),
    current_head_commit_sha = EXCLUDED.current_head_commit_sha,
    updated_at = NOW()
RETURNING
    id,
    repository_id,
    pr_number,
    github_pull_request_id,
    current_head_commit_sha,
    created_at,
    updated_at;

-- name: GetPullRequestByRepositoryIDAndPRNumber :one
SELECT
    id,
    repository_id,
    pr_number,
    github_pull_request_id,
    current_head_commit_sha,
    created_at,
    updated_at
FROM pull_requests
WHERE repository_id = $1
  AND pr_number = $2;
