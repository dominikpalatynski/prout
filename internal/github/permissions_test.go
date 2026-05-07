package github

import (
	"testing"

	"github.com/dominikpalatynski/prout/internal/config"
)

func TestValidateRepositoryPermission(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		action  RepositoryAction
		role    RepositoryRole
		wantErr string
	}{
		{
			name:   "write can create preview",
			action: RepositoryActionPreviewLabel,
			role:   RepositoryRoleWrite,
		},
		{
			name:   "maintain can delete preview",
			action: RepositoryActionPreviewUnlabel,
			role:   RepositoryRoleMaintain,
		},
		{
			name:    "triage cannot create preview",
			action:  RepositoryActionPreviewLabel,
			role:    RepositoryRoleTriage,
			wantErr: `repository role "triage" is insufficient for action "preview_label": requires "write" or higher`,
		},
		{
			name:    "unknown action is rejected",
			action:  RepositoryAction("unknown"),
			role:    RepositoryRoleAdmin,
			wantErr: `unsupported repository action "unknown"`,
		},
		{
			name:    "unknown role is rejected",
			action:  RepositoryActionPreviewLabel,
			role:    RepositoryRole("owner"),
			wantErr: `unknown repository role "owner"`,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateRepositoryPermission(&config.Config{}, tt.action, tt.role)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateRepositoryPermission() error = %v", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("ValidateRepositoryPermission() error = nil, want %q", tt.wantErr)
			}
			if err.Error() != tt.wantErr {
				t.Fatalf("ValidateRepositoryPermission() error = %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}
