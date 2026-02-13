package build

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestCreateSymlinkNonForceMigratesDirectoryThenLinks(t *testing.T) {
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
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected link path to become symlink in non-force mode")
	}
	realTarget, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("failed to read symlink target: %v", err)
	}
	if realTarget != target {
		t.Fatalf("symlink target mismatch: got %q want %q", realTarget, target)
	}

	migratedMarker := filepath.Join(target, "marker.txt")
	if _, err := os.Stat(migratedMarker); err != nil {
		t.Fatalf("expected marker file to be migrated to target, got: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("expected marker file to remain reachable via symlink, got: %v", err)
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

func TestCreateSymlinkNonForceConflictBackupsSourceThenLinks(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "target")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("failed to create target directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "marker.txt"), []byte("target"), 0o644); err != nil {
		t.Fatalf("failed to create target marker: %v", err)
	}

	linkPath := filepath.Join(tmp, "link")
	if err := os.MkdirAll(linkPath, 0o755); err != nil {
		t.Fatalf("failed to create source dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(linkPath, "marker.txt"), []byte("source"), 0o644); err != nil {
		t.Fatalf("failed to create source marker: %v", err)
	}

	if err := createSymlink(target, linkPath, false); err != nil {
		t.Fatalf("createSymlink(non-force) returned error: %v", err)
	}

	info, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatalf("failed to stat link path: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected link path to become symlink in non-force mode")
	}

	targetContent, err := os.ReadFile(filepath.Join(target, "marker.txt"))
	if err != nil {
		t.Fatalf("failed to read target marker: %v", err)
	}
	if string(targetContent) != "target" {
		t.Fatalf("target marker should remain unchanged, got: %q", string(targetContent))
	}

	matches, err := filepath.Glob(filepath.Join(target, ".migrated_from_link", "marker.txt*"))
	if err != nil {
		t.Fatalf("glob migration backup failed: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("expected conflicted source marker to be backed up")
	}
}

func TestMovePathCrossDeviceFallback(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src.txt")
	dst := filepath.Join(tmp, "dst.txt")

	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}

	oldRename := renamePath
	defer func() { renamePath = oldRename }()
	renamePath = func(oldPath, newPath string) error {
		return &os.LinkError{Op: "rename", Old: oldPath, New: newPath, Err: syscall.EXDEV}
	}

	if err := movePath(src, dst); err != nil {
		t.Fatalf("movePath returned error: %v", err)
	}

	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("source should be removed after fallback move")
	}
	content, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("failed to read destination file: %v", err)
	}
	if string(content) != "hello" {
		t.Fatalf("unexpected destination content: %q", string(content))
	}
}
