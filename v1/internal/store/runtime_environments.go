package store

import (
	"context"

	"github.com/dominikpalatynski/toolshed/internal/store/sqlc"
)

type ListRuntimeEnvironmentsParams struct {
	RepositoryID *int64
	PRNumber     *int64
}

type RuntimeEnvironmentListItem struct {
	RuntimeEnvironment sqlc.RuntimeEnvironments
	PullRequest        sqlc.PullRequests
}

func (s *Store) ListRuntimeEnvironments(ctx context.Context, params ListRuntimeEnvironmentsParams) ([]RuntimeEnvironmentListItem, error) {
	const query = `
SELECT
    runtime_environments.id,
    runtime_environments.repository_id,
    runtime_environments.pull_request_id,
    runtime_environments.type,
    runtime_environments.status,
    runtime_environments.target_pr_head_commit_sha,
    runtime_environments.source_github_repository_id,
    runtime_environments.source_owner,
    runtime_environments.source_name,
    runtime_environments.source_full_name,
    runtime_environments.workspace_locator,
    runtime_environments.created_at,
    runtime_environments.updated_at,
    pull_requests.id,
    pull_requests.repository_id,
    pull_requests.pr_number,
    pull_requests.github_pull_request_id,
    pull_requests.current_head_commit_sha,
    pull_requests.current_source_github_repository_id,
    pull_requests.current_source_owner,
    pull_requests.current_source_name,
    pull_requests.current_source_full_name,
    pull_requests.created_at,
    pull_requests.updated_at
FROM runtime_environments
JOIN pull_requests
  ON pull_requests.id = runtime_environments.pull_request_id
WHERE ($1::bigint IS NULL OR runtime_environments.repository_id = $1)
  AND ($2::bigint IS NULL OR pull_requests.pr_number = $2)
ORDER BY runtime_environments.id DESC
`

	rows, err := s.pool.Query(ctx, query, params.RepositoryID, params.PRNumber)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []RuntimeEnvironmentListItem
	for rows.Next() {
		var item RuntimeEnvironmentListItem
		if err := rows.Scan(
			&item.RuntimeEnvironment.ID,
			&item.RuntimeEnvironment.RepositoryID,
			&item.RuntimeEnvironment.PullRequestID,
			&item.RuntimeEnvironment.Type,
			&item.RuntimeEnvironment.Status,
			&item.RuntimeEnvironment.TargetPrHeadCommitSha,
			&item.RuntimeEnvironment.SourceGithubRepositoryID,
			&item.RuntimeEnvironment.SourceOwner,
			&item.RuntimeEnvironment.SourceName,
			&item.RuntimeEnvironment.SourceFullName,
			&item.RuntimeEnvironment.WorkspaceLocator,
			&item.RuntimeEnvironment.CreatedAt,
			&item.RuntimeEnvironment.UpdatedAt,
			&item.PullRequest.ID,
			&item.PullRequest.RepositoryID,
			&item.PullRequest.PRNumber,
			&item.PullRequest.GithubPullRequestID,
			&item.PullRequest.CurrentHeadCommitSha,
			&item.PullRequest.CurrentSourceGithubRepositoryID,
			&item.PullRequest.CurrentSourceOwner,
			&item.PullRequest.CurrentSourceName,
			&item.PullRequest.CurrentSourceFullName,
			&item.PullRequest.CreatedAt,
			&item.PullRequest.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return items, nil
}
