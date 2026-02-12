package build

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateSymlinkNonForceKeepsExistingDirectory(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "target")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("failed to create target directory: %v", err)
	}

	linkPath := filepath.Join(tmp, "link")
	if err := os.MkdirAll(linkPath, 0o755); err != nil {
		t.Fatalf("failed to create existing directory: %v", err)
	}
	marker := filepath.Join(linkPath, "marker.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0o644); err != nil {
		t.Fatalf("failed to create marker file: %v", err)
	}

	if err := createSymlink(target, linkPath, false); err != nil {
		t.Fatalf("createSymlink(non-force) returned error: %v", err)
	}

	info, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatalf("failed to stat link path: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("expected existing directory to be preserved in non-force mode")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("expected marker file to be preserved, got: %v", err)
	}
}

func TestCreateSymlinkForceReplacesDirectory(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "target")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("failed to create target directory: %v", err)
	}

	linkPath := filepath.Join(tmp, "link")
	if err := os.MkdirAll(linkPath, 0o755); err != nil {
		t.Fatalf("failed to create existing directory: %v", err)
	}
	marker := filepath.Join(linkPath, "marker.txt")
	if err := os.WriteFile(marker, []byte("drop"), 0o644); err != nil {
		t.Fatalf("failed to create marker file: %v", err)
	}

	if err := createSymlink(target, linkPath, true); err != nil {
		t.Fatalf("createSymlink(force) returned error: %v", err)
	}

	info, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatalf("failed to stat link path: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected link path to become symlink in force mode")
	}
	realTarget, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("failed to read symlink target: %v", err)
	}
	if realTarget != target {
		t.Fatalf("symlink target mismatch: got %q want %q", realTarget, target)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("expected old directory contents to be removed in force mode")
	}
}
