package store

import "github.com/dominikpalatynski/toolshed/internal/store/sqlc"

func RepositoryFromUpsertRow(row sqlc.UpsertRepositoryRow) sqlc.Repositories {
	return sqlc.Repositories{
		ID:                   row.ID,
		GithubRepositoryID:   row.GithubRepositoryID,
		GithubInstallationID: row.GithubInstallationID,
		Owner:                row.Owner,
		Name:                 row.Name,
		FullName:             row.FullName,
		HtmlUrl:              row.HtmlUrl,
		IsPrivate:            row.IsPrivate,
		Enabled:              row.Enabled,
		CreatedAt:            row.CreatedAt,
		UpdatedAt:            row.UpdatedAt,
	}
}
