package workspace

import (
	"testing"

	"github.com/dominikpalatynski/toolshed/internal/config"
)

func TestPrepareDomainName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		wildcardDomain string
		want           string
	}{
		{
			name:           "base wildcard suffix",
			wildcardDomain: ".app.localhost",
			want:           "owner-repo-123-abcdef1.app.localhost",
		},
		{
			name:           "legacy wildcard placeholder",
			wildcardDomain: "*.app.localhost",
			want:           "owner-repo-123-abcdef1.app.localhost",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler := &WorkspaceHandler{
				cfg: &config.Config{
					GitHub: config.GitHubConfig{
						Repository: config.RepositoryConfig{
							WildcardDomain: tt.wildcardDomain,
						},
					},
				},
			}

			got, err := handler.prepareDomainName(WorkspaceLocationBuilder{
				FullName: "owner/repo",
				PRNumber: 123,
				SHA:      "abcdef1234567890",
			})
			if err != nil {
				t.Fatalf("prepareDomainName returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("prepareDomainName = %q, want %q", got, tt.want)
			}
		})
	}
}
