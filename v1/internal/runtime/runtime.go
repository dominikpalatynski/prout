// Package runtime defines the orchestration-facing contract for preview
// backends such as Docker Compose. The worker owns repository config, durable
// state, and step progression; the backend owns preparation, deployment, and
// teardown for one frozen deployment input.
package runtime

import (
	"context"
	"os"
)

type Workspace interface {
	Locator() string
	Path() string
	ResolvePath(relativePath string) (string, error)
	FileExists(relativePath string) (bool, error)
	ReadFile(relativePath string) ([]byte, error)
	WriteFile(relativePath string, contents []byte, mode os.FileMode) error
	WriteFileAdjacentTo(relativePath string, siblingName string, contents []byte, mode os.FileMode) (string, error)
}

type RuntimeSettings struct {
	ComposeFilePath    string
	ExposedServiceName string
	ExposedServicePort int32
}

type EnvironmentVariable struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type PrepareRequest struct {
	RuntimeEnvironmentID int64
	RepositoryID         int64
	PullRequestID        int64
	TargetCommitSHA      string
	Workspace            Workspace
	RuntimeSettings      RuntimeSettings
	EnvironmentVariables []EnvironmentVariable
}

type DeploymentRecord struct {
	Backend                        string
	FrozenRuntimeSettingsJSON      []byte
	FrozenEnvironmentVariablesJSON []byte
	MetadataJSON                   []byte
}

type PreparedArtifactsRequest struct {
	Workspace  Workspace
	Deployment DeploymentRecord
}

type DeployRequest struct {
	Workspace  Workspace
	Deployment DeploymentRecord
}

type TeardownRequest struct {
	Workspace  Workspace
	Deployment DeploymentRecord
}

type Backend interface {
	Name() string
	Prepare(ctx context.Context, request PrepareRequest) (DeploymentRecord, error)
	PreparedArtifactsExist(ctx context.Context, request PreparedArtifactsRequest) (bool, error)
	Deploy(ctx context.Context, request DeployRequest) error
	Teardown(ctx context.Context, request TeardownRequest) error
}
