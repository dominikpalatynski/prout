package store

import (
	"context"
	"testing"

	"github.com/dominikpalatynski/toolshed/internal/automation"
	"github.com/dominikpalatynski/toolshed/internal/store/sqlc"
	"github.com/dominikpalatynski/toolshed/internal/testdb"
)

func TestEnsureRepositoryEventFamiliesSeedsDefaultsWithoutOverwritingState(t *testing.T) {
	pool := testdb.Start(t)
	appStore := New(pool)
	ctx := context.Background()

	repositoryRow, err := appStore.Q().UpsertRepository(ctx, sqlc.UpsertRepositoryParams{
		GithubRepositoryID:   701,
		GithubInstallationID: 801,
		Owner:                "acme",
		Name:                 "demo",
		FullName:             "acme/demo",
		HtmlUrl:              "https://github.com/acme/demo",
		IsPrivate:            false,
	})
	if err != nil {
		t.Fatalf("UpsertRepository() error = %v", err)
	}
	repository := RepositoryFromUpsertRow(repositoryRow)

	eventFamilyKeys := automation.NewRegistry().SupportedEventFamilyKeys()
	if err := appStore.Q().EnsureRepositoryEventFamilies(ctx, sqlc.EnsureRepositoryEventFamiliesParams{
		RepositoryID:    repository.ID,
		EventFamilyKeys: eventFamilyKeys,
	}); err != nil {
		t.Fatalf("EnsureRepositoryEventFamilies() error = %v", err)
	}

	records, err := appStore.Q().ListRepositoryEventFamilies(ctx, repository.ID)
	if err != nil {
		t.Fatalf("ListRepositoryEventFamilies() error = %v", err)
	}
	if len(records) != len(eventFamilyKeys) {
		t.Fatalf("len(ListRepositoryEventFamilies()) = %d, want %d", len(records), len(eventFamilyKeys))
	}
	for _, record := range records {
		if !record.Enabled {
			t.Fatalf("repository event family %q enabled = false, want true", record.EventFamilyKey)
		}
	}

	if _, err := appStore.Q().SetRepositoryEventFamilyEnabled(ctx, sqlc.SetRepositoryEventFamilyEnabledParams{
		RepositoryID:   repository.ID,
		EventFamilyKey: automation.EventFamilyPullRequestCommentCreated,
		Enabled:        false,
	}); err != nil {
		t.Fatalf("SetRepositoryEventFamilyEnabled() error = %v", err)
	}

	if err := appStore.Q().EnsureRepositoryEventFamilies(ctx, sqlc.EnsureRepositoryEventFamiliesParams{
		RepositoryID:    repository.ID,
		EventFamilyKeys: eventFamilyKeys,
	}); err != nil {
		t.Fatalf("EnsureRepositoryEventFamilies() second call error = %v", err)
	}

	commentEventFamily, err := appStore.Q().GetRepositoryEventFamily(ctx, sqlc.GetRepositoryEventFamilyParams{
		RepositoryID:   repository.ID,
		EventFamilyKey: automation.EventFamilyPullRequestCommentCreated,
	})
	if err != nil {
		t.Fatalf("GetRepositoryEventFamily() error = %v", err)
	}
	if commentEventFamily.Enabled {
		t.Fatalf("comment event family enabled = true, want false after ensure rerun")
	}
}
