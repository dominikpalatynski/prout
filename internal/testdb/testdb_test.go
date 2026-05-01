package testdb

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultColimaDockerHost(t *testing.T) {
	tempHome := t.TempDir()

	socketPath := filepath.Join(tempHome, ".colima", "default", "docker.sock")
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(socketPath, []byte(""), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	dockerHost, ok := defaultColimaDockerHostForHome(tempHome)
	if !ok {
		t.Fatalf("defaultColimaDockerHostForHome() ok = false, want true")
	}
	if dockerHost != "unix://"+socketPath {
		t.Fatalf("defaultColimaDockerHostForHome() = %q, want %q", dockerHost, "unix://"+socketPath)
	}
}
