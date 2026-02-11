package repo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPreparePackageOutputRejectsRootPath(t *testing.T) {
	if err := preparePackageOutput(string(os.PathSeparator)); err == nil {
		t.Fatalf("preparePackageOutput('/') should fail")
	}
}

func TestPreparePackageOutputRejectsDirectory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pig-cache-dir-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := preparePackageOutput(tmpDir); err == nil {
		t.Fatalf("preparePackageOutput(directory) should fail")
	}
}

func TestPreparePackageOutputRemovesExistingFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pig-cache-file-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	target := filepath.Join(tmpDir, "pkg.tgz")
	if err := os.WriteFile(target, []byte("old"), 0644); err != nil {
		t.Fatalf("failed to create old package file: %v", err)
	}

	if err := preparePackageOutput(target); err != nil {
		t.Fatalf("preparePackageOutput returned error: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("target file should be removed, stat err=%v", err)
	}
}
