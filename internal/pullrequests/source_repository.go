package pullrequests

import "strings"

// SourceRepository identifies the GitHub repository that contains the current
// pull-request head commit targeted for preview work.
type SourceRepository struct {
	GithubRepositoryID int64  `json:"github_repository_id"`
	Owner              string `json:"owner"`
	Name               string `json:"name"`
	FullName           string `json:"full_name"`
}

func (r SourceRepository) IsComplete() bool {
	return r.GithubRepositoryID > 0 &&
		strings.TrimSpace(r.Owner) != "" &&
		strings.TrimSpace(r.Name) != "" &&
		strings.TrimSpace(r.FullName) != ""
}
