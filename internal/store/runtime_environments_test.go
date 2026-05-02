package store

import (
	"context"
	"testing"

	"github.com/dominikpalatynski/toolshed/internal/operations"
	"github.com/dominikpalatynski/toolshed/internal/store/sqlc"
	"github.com/dominikpalatynski/toolshed/internal/testdb"
)

func TestListRuntimeEnvironmentsFiltersAndOrdersNewestFirst(t *testing.T) {
	pool := testdb.Start(t)
	appStore := New(pool)
	ctx := context.Background()

	repository, err := appStore.Q().UpsertRepository(ctx, sqlc.UpsertRepositoryParams{
		GithubRepositoryID:   1001,
		GithubInstallationID: 2001,
		Owner:                "acme",
		Name:                 "demo",
		FullName:             "acme/demo",
		HtmlUrl:              "https://github.com/acme/demo",
		IsPrivate:            false,
	})
	if err != nil {
		t.Fatalf("UpsertRepository() error = %v", err)
	}

	firstPullRequest := mustCreatePullRequest(t, ctx, appStore, repository, 41, "aaa111")
	secondPullRequest := mustCreatePullRequest(t, ctx, appStore, repository, 42, "bbb222")

	if _, err := appStore.Q().InsertRuntimeEnvironment(ctx, sqlc.InsertRuntimeEnvironmentParams{
		RepositoryID:             repository.ID,
		PullRequestID:            firstPullRequest.ID,
		Type:                     operations.RuntimeEnvironmentTypePreview,
		Status:                   operations.RuntimeStatusPrepared,
		TargetPrHeadCommitSha:    "aaa111",
		SourceGithubRepositoryID: repository.GithubRepositoryID,
		SourceOwner:              repository.Owner,
		SourceName:               repository.Name,
		SourceFullName:           repository.FullName,
	}); err != nil {
		t.Fatalf("InsertRuntimeEnvironment(first) error = %v", err)
	}

	olderSecond, err := appStore.Q().InsertRuntimeEnvironment(ctx, sqlc.InsertRuntimeEnvironmentParams{
		RepositoryID:             repository.ID,
		PullRequestID:            secondPullRequest.ID,
		Type:                     operations.RuntimeEnvironmentTypePreview,
		Status:                   operations.RuntimeStatusSuperseded,
		TargetPrHeadCommitSha:    "bbb111",
		SourceGithubRepositoryID: repository.GithubRepositoryID,
		SourceOwner:              repository.Owner,
		SourceName:               repository.Name,
		SourceFullName:           repository.FullName,
	})
	if err != nil {
		t.Fatalf("InsertRuntimeEnvironment(older second) error = %v", err)
	}

	newerSecond, err := appStore.Q().InsertRuntimeEnvironment(ctx, sqlc.InsertRuntimeEnvironmentParams{
		RepositoryID:             repository.ID,
		PullRequestID:            secondPullRequest.ID,
		Type:                     operations.RuntimeEnvironmentTypePreview,
		Status:                   operations.RuntimeStatusPrepared,
		TargetPrHeadCommitSha:    "bbb222",
		SourceGithubRepositoryID: repository.GithubRepositoryID,
		SourceOwner:              repository.Owner,
		SourceName:               repository.Name,
		SourceFullName:           repository.FullName,
	})
	if err != nil {
		t.Fatalf("InsertRuntimeEnvironment(newer second) error = %v", err)
	}

	allItems, err := appStore.ListRuntimeEnvironments(ctx, ListRuntimeEnvironmentsParams{})
	if err != nil {
		t.Fatalf("ListRuntimeEnvironments(all) error = %v", err)
	}
	if len(allItems) != 3 {
		t.Fatalf("len(allItems) = %d, want 3", len(allItems))
	}
	if allItems[0].RuntimeEnvironment.ID != newerSecond.ID || allItems[1].RuntimeEnvironment.ID != olderSecond.ID {
		t.Fatalf("unexpected ordering: got [%d %d ...], want newest-first [%d %d ...]", allItems[0].RuntimeEnvironment.ID, allItems[1].RuntimeEnvironment.ID, newerSecond.ID, olderSecond.ID)
	}

	filteredItems, err := appStore.ListRuntimeEnvironments(ctx, ListRuntimeEnvironmentsParams{
		RepositoryID: &repository.ID,
		PRNumber:     int64Ptr(42),
	})
	if err != nil {
		t.Fatalf("ListRuntimeEnvironments(filtered) error = %v", err)
	}
	if len(filteredItems) != 2 {
		t.Fatalf("len(filteredItems) = %d, want 2", len(filteredItems))
	}
	for _, item := range filteredItems {
		if item.PullRequest.PRNumber != 42 {
			t.Fatalf("filtered pull request number = %d, want 42", item.PullRequest.PRNumber)
		}
		if item.PullRequest.CurrentSourceFullName != repository.FullName {
			t.Fatalf("filtered current_source_full_name = %q, want %q", item.PullRequest.CurrentSourceFullName, repository.FullName)
		}
	}
}

func mustCreatePullRequest(
	t *testing.T,
	ctx context.Context,
	appStore *Store,
	repository sqlc.Repositories,
	prNumber int64,
	headSHA string,
) sqlc.PullRequests {
	t.Helper()

	pullRequest, err := appStore.Q().UpsertPullRequestAnchor(ctx, sqlc.UpsertPullRequestAnchorParams{
		RepositoryID:                    repository.ID,
		PRNumber:                        prNumber,
		GithubPullRequestID:             int64Ptr(7000 + prNumber),
		CurrentHeadCommitSha:            headSHA,
		CurrentSourceGithubRepositoryID: repository.GithubRepositoryID,
		CurrentSourceOwner:              repository.Owner,
		CurrentSourceName:               repository.Name,
		CurrentSourceFullName:           repository.FullName,
	})
	if err != nil {
		t.Fatalf("UpsertPullRequestAnchor() error = %v", err)
	}
	return pullRequest
}
