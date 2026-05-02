package workspaces

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type FilesystemManager struct {
	root string
}

func NewFilesystemManager(root string) *FilesystemManager {
	return &FilesystemManager{root: root}
}

func (m *FilesystemManager) WorkspaceExists(locator string) (bool, error) {
	workspacePath, err := m.workspacePath(locator)
	if err != nil {
		return false, err
	}

	info, err := os.Stat(workspacePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("stat workspace: %w", err)
	}

	return info.IsDir(), nil
}

func (m *FilesystemManager) CreateStaging(locator string) (string, error) {
	if _, err := m.workspacePath(locator); err != nil {
		return "", err
	}
	if err := os.MkdirAll(m.root, 0o755); err != nil {
		return "", fmt.Errorf("create workspace root: %w", err)
	}

	stagingRoot := filepath.Join(m.root, ".staging")
	if err := os.MkdirAll(stagingRoot, 0o755); err != nil {
		return "", fmt.Errorf("create staging root: %w", err)
	}

	prefix := strings.NewReplacer("/", "_", "\\", "_").Replace(locator) + "-"
	path, err := os.MkdirTemp(stagingRoot, prefix)
	if err != nil {
		return "", fmt.Errorf("create staging directory: %w", err)
	}

	return path, nil
}

func (m *FilesystemManager) ExtractTarball(stagingPath string, body io.Reader) error {
	gzipReader, err := gzip.NewReader(body)
	if err != nil {
		return fmt.Errorf("create gzip reader: %w", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("read tar header: %w", err)
		}

		relativePath, skip, err := stripTarballWrapper(header.Name)
		if err != nil {
			return err
		}
		if skip {
			continue
		}

		targetPath, err := secureJoin(stagingPath, relativePath)
		if err != nil {
			return err
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return fmt.Errorf("create directory %q: %w", relativePath, err)
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return fmt.Errorf("create file parent %q: %w", relativePath, err)
			}

			file, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, header.FileInfo().Mode().Perm())
			if err != nil {
				return fmt.Errorf("open file %q: %w", relativePath, err)
			}

			if _, err := io.Copy(file, tarReader); err != nil {
				_ = file.Close()
				return fmt.Errorf("write file %q: %w", relativePath, err)
			}
			if err := file.Close(); err != nil {
				return fmt.Errorf("close file %q: %w", relativePath, err)
			}
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return fmt.Errorf("create symlink parent %q: %w", relativePath, err)
			}
			if err := os.Symlink(header.Linkname, targetPath); err != nil {
				return fmt.Errorf("create symlink %q: %w", relativePath, err)
			}
		default:
			// Ignore unsupported entry types in phase 3B.
		}
	}
}

func (m *FilesystemManager) PromoteStaging(stagingPath, locator string) error {
	finalPath, err := m.workspacePath(locator)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(finalPath), 0o755); err != nil {
		return fmt.Errorf("create workspace parent: %w", err)
	}
	if _, err := os.Stat(finalPath); err == nil {
		return fmt.Errorf("workspace %q already exists", locator)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat workspace before promote: %w", err)
	}
	if err := os.Rename(stagingPath, finalPath); err != nil {
		return fmt.Errorf("promote staging workspace: %w", err)
	}
	return nil
}

func (m *FilesystemManager) CleanupStaging(stagingPath string) error {
	if stagingPath == "" {
		return nil
	}
	if err := os.RemoveAll(stagingPath); err != nil {
		return fmt.Errorf("cleanup staging workspace: %w", err)
	}
	return nil
}

func (m *FilesystemManager) CleanupWorkspace(locator string) error {
	workspacePath, err := m.workspacePath(locator)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(workspacePath); err != nil {
		return fmt.Errorf("cleanup workspace: %w", err)
	}
	return nil
}

func (m *FilesystemManager) workspacePath(locator string) (string, error) {
	return secureJoin(m.root, locator)
}

func stripTarballWrapper(name string) (string, bool, error) {
	clean := path.Clean(strings.TrimSpace(name))
	if clean == "." || clean == "/" {
		return "", true, nil
	}
	if strings.HasPrefix(clean, "/") || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", false, fmt.Errorf("tarball entry %q escapes workspace root", name)
	}

	parts := strings.Split(clean, "/")
	if len(parts) == 1 {
		return "", true, nil
	}

	relative := path.Join(parts[1:]...)
	if relative == "." || relative == "" {
		return "", true, nil
	}
	if relative == ".." || strings.HasPrefix(relative, "../") {
		return "", false, fmt.Errorf("tarball entry %q escapes workspace root", name)
	}
	return relative, false, nil
}

func secureJoin(root, relative string) (string, error) {
	trimmed := strings.TrimSpace(relative)
	if trimmed == "" {
		return "", fmt.Errorf("workspace locator is required")
	}

	clean := filepath.Clean(trimmed)
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid workspace locator %q", relative)
	}

	fullPath := filepath.Join(root, clean)
	relToRoot, err := filepath.Rel(root, fullPath)
	if err != nil {
		return "", fmt.Errorf("resolve workspace locator: %w", err)
	}
	if relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("workspace locator %q escapes workspace root", relative)
	}

	return fullPath, nil
}
