package store

import (
	"context"
	"testing"

	"github.com/dominikpalatynski/toolshed/internal/operations"
	"github.com/dominikpalatynski/toolshed/internal/store/sqlc"
	"github.com/dominikpalatynski/toolshed/internal/testdb"
)

func TestUpsertRepositoryCreatesDefaultRuntimeSettingsRecord(t *testing.T) {
	pool := testdb.Start(t)
	appStore := New(pool)
	ctx := context.Background()

	repositoryRow, err := appStore.Q().UpsertRepository(ctx, sqlc.UpsertRepositoryParams{
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
	repository := RepositoryFromUpsertRow(repositoryRow)

	settings, err := appStore.Q().GetRepositoryRuntimeSettingsByRepositoryID(ctx, repository.ID)
	if err != nil {
		t.Fatalf("GetRepositoryRuntimeSettingsByRepositoryID() error = %v", err)
	}
	if settings.ComposeFilePath != nil {
		t.Fatalf("ComposeFilePath = %v, want nil by default", settings.ComposeFilePath)
	}
	if settings.ExposedServiceName != nil {
		t.Fatalf("ExposedServiceName = %v, want nil by default", settings.ExposedServiceName)
	}
	if settings.ExposedServicePort != nil {
		t.Fatalf("ExposedServicePort = %v, want nil by default", settings.ExposedServicePort)
	}
}

func TestReplaceRepositoryEnvironmentVariablesReplacesAndSortsValues(t *testing.T) {
	pool := testdb.Start(t)
	appStore := New(pool)
	ctx := context.Background()

	repositoryRow, err := appStore.Q().UpsertRepository(ctx, sqlc.UpsertRepositoryParams{
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
	repository := RepositoryFromUpsertRow(repositoryRow)

	firstSet, err := appStore.ReplaceRepositoryEnvironmentVariables(ctx, repository.ID, []RepositoryEnvironmentVariableInput{
		{Name: "BETA", Value: "2"},
		{Name: "ALPHA", Value: "1"},
	})
	if err != nil {
		t.Fatalf("ReplaceRepositoryEnvironmentVariables(first) error = %v", err)
	}
	if len(firstSet) != 2 {
		t.Fatalf("len(firstSet) = %d, want 2", len(firstSet))
	}
	if firstSet[0].Name != "ALPHA" || firstSet[1].Name != "BETA" {
		t.Fatalf("firstSet order = [%q %q], want [ALPHA BETA]", firstSet[0].Name, firstSet[1].Name)
	}

	secondSet, err := appStore.ReplaceRepositoryEnvironmentVariables(ctx, repository.ID, []RepositoryEnvironmentVariableInput{
		{Name: "GAMMA", Value: "3"},
	})
	if err != nil {
		t.Fatalf("ReplaceRepositoryEnvironmentVariables(second) error = %v", err)
	}
	if len(secondSet) != 1 || secondSet[0].Name != "GAMMA" {
		t.Fatalf("secondSet = %#v, want one GAMMA record", secondSet)
	}

	stored, err := appStore.Q().ListRepositoryEnvironmentVariablesByRepositoryID(ctx, repository.ID)
	if err != nil {
		t.Fatalf("ListRepositoryEnvironmentVariablesByRepositoryID() error = %v", err)
	}
	if len(stored) != 1 || stored[0].Name != "GAMMA" || stored[0].Value != "3" {
		t.Fatalf("stored variables = %#v, want only GAMMA=3", stored)
	}
}

func TestUpsertRuntimeEnvironmentDeploymentStoresFrozenMetadata(t *testing.T) {
	pool := testdb.Start(t)
	appStore := New(pool)
	ctx := context.Background()

	repositoryRow, err := appStore.Q().UpsertRepository(ctx, sqlc.UpsertRepositoryParams{
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
	repository := RepositoryFromUpsertRow(repositoryRow)
	pullRequest := mustCreatePullRequest(t, ctx, appStore, repository, 42, "abc123")

	runtimeEnvironment, err := appStore.Q().InsertRuntimeEnvironment(ctx, sqlc.InsertRuntimeEnvironmentParams{
		RepositoryID:             repository.ID,
		PullRequestID:            pullRequest.ID,
		Type:                     operations.RuntimeEnvironmentTypePreview,
		Status:                   operations.RuntimeStatusPreparing,
		TargetPrHeadCommitSha:    "abc123",
		SourceGithubRepositoryID: repository.GithubRepositoryID,
		SourceOwner:              repository.Owner,
		SourceName:               repository.Name,
		SourceFullName:           repository.FullName,
	})
	if err != nil {
		t.Fatalf("InsertRuntimeEnvironment() error = %v", err)
	}

	_, err = appStore.Q().UpsertRuntimeEnvironmentDeployment(ctx, sqlc.UpsertRuntimeEnvironmentDeploymentParams{
		RuntimeEnvironmentID:           runtimeEnvironment.ID,
		Backend:                        "docker_compose",
		FrozenRuntimeSettingsJson:      []byte(`{"compose_file_path":"compose.yml"}`),
		FrozenEnvironmentVariablesJson: []byte(`[{"name":"ALPHA","value":"1"}]`),
		DeploymentMetadataJson:         []byte(`{"rendered_compose_file_path":".toolshed.docker-compose.rendered.yml"}`),
	})
	if err != nil {
		t.Fatalf("UpsertRuntimeEnvironmentDeployment() error = %v", err)
	}

	record, err := appStore.Q().GetRuntimeEnvironmentDeploymentByRuntimeEnvironmentID(ctx, runtimeEnvironment.ID)
	if err != nil {
		t.Fatalf("GetRuntimeEnvironmentDeploymentByRuntimeEnvironmentID() error = %v", err)
	}
	if record.Backend != "docker_compose" {
		t.Fatalf("Backend = %q, want %q", record.Backend, "docker_compose")
	}
	if string(record.FrozenRuntimeSettingsJson) != `{"compose_file_path": "compose.yml"}` {
		t.Fatalf("FrozenRuntimeSettingsJson = %s, want compose.yml snapshot", record.FrozenRuntimeSettingsJson)
	}
}
