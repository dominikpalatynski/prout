package workspaces

import "testing"

func TestFilesystemWorkspaceResolvePathRejectsEscapingRelativePath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	manager := NewFilesystemManager(root)

	stagingPath, err := manager.CreateStaging("runtime-environments/42")
	if err != nil {
		t.Fatalf("CreateStaging() error = %v", err)
	}
	if err := manager.PromoteStaging(stagingPath, "runtime-environments/42"); err != nil {
		t.Fatalf("PromoteStaging() error = %v", err)
	}

	workspace, err := manager.OpenWorkspace("runtime-environments/42")
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}

	if _, err := workspace.ResolvePath("../outside"); err == nil {
		t.Fatalf("ResolvePath() error = nil, want non-nil for escaping path")
	}
}

func TestFilesystemWorkspaceWriteFileCreatesParentDirectories(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	manager := NewFilesystemManager(root)

	stagingPath, err := manager.CreateStaging("runtime-environments/42")
	if err != nil {
		t.Fatalf("CreateStaging() error = %v", err)
	}
	if err := manager.PromoteStaging(stagingPath, "runtime-environments/42"); err != nil {
		t.Fatalf("PromoteStaging() error = %v", err)
	}

	workspace, err := manager.OpenWorkspace("runtime-environments/42")
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}

	if err := workspace.WriteFile(".toolshed/runtime/config.yml", []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	contents, err := workspace.ReadFile(".toolshed/runtime/config.yml")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(contents) != "hello\n" {
		t.Fatalf("ReadFile() = %q, want %q", string(contents), "hello\n")
	}
}

func TestFilesystemWorkspaceWriteFileAdjacentToPlacesSiblingNextToSourceFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	manager := NewFilesystemManager(root)

	stagingPath, err := manager.CreateStaging("runtime-environments/42")
	if err != nil {
		t.Fatalf("CreateStaging() error = %v", err)
	}
	if err := manager.PromoteStaging(stagingPath, "runtime-environments/42"); err != nil {
		t.Fatalf("PromoteStaging() error = %v", err)
	}

	workspace, err := manager.OpenWorkspace("runtime-environments/42")
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}

	relativePath, err := workspace.WriteFileAdjacentTo(
		"deploy/compose.yml",
		".toolshed.docker-compose.rendered.yml",
		[]byte("rendered\n"),
		0o644,
	)
	if err != nil {
		t.Fatalf("WriteFileAdjacentTo() error = %v", err)
	}
	if relativePath != "deploy/.toolshed.docker-compose.rendered.yml" {
		t.Fatalf("WriteFileAdjacentTo() path = %q, want %q", relativePath, "deploy/.toolshed.docker-compose.rendered.yml")
	}

	contents, err := workspace.ReadFile(relativePath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(contents) != "rendered\n" {
		t.Fatalf("ReadFile() = %q, want %q", string(contents), "rendered\n")
	}
}
