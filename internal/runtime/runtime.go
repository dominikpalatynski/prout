// Package runtime defines the escape-hatch interface that decouples Toolshed's
// preview-env orchestration from the underlying compute backend (docker compose,
// k8s, etc.). See ADR-005.
package runtime

import "context"

// DeployParams carries everything a runtime needs to spin up a preview env for
// a single PR. Concrete fields will grow as the preview package evolves.
type DeployParams struct {
	RepositoryID int64
	PRNumber     int
	SHA          string
	WorkspaceDir string
	EnvVars      map[string]string
}

// Runtime is the contract every backend implementation must satisfy.
// Default implementation lives under internal/runtime/dockercompose.
type Runtime interface {
	Deploy(ctx context.Context, p DeployParams) error
	Teardown(ctx context.Context, prID int) error
}
