package sty

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallPigsty(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "pigsty-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	tests := []struct {
		name       string
		targetDir  string
		overwrite  bool
		setup      func(string) error // setup function to prepare test environment
		wantErr    bool
		checkFiles []string // files that should exist after installation
		skipFiles  []string // files that should not be overwritten
	}{
		{
			name:      "basic installation to empty directory",
			targetDir: filepath.Join(tmpDir, "basic"),
			overwrite: false,
			checkFiles: []string{
				"pigsty.yml",
			},
		},
		{
			name:      "installation to existing directory without overwrite",
			targetDir: filepath.Join(tmpDir, "no-overwrite"),
			overwrite: false,
			setup: func(dir string) error {
				return os.MkdirAll(dir, 0755)
			},
			wantErr: true,
		},
		{
			name:      "installation to existing directory with overwrite",
			targetDir: filepath.Join(tmpDir, "with-overwrite"),
			overwrite: true,
			setup: func(dir string) error {
				if err := os.MkdirAll(dir, 0755); err != nil {
					return err
				}
				// Create some existing files
				return os.WriteFile(filepath.Join(dir, "existing.txt"), []byte("test"), 0644)
			},
			checkFiles: []string{
				"pigsty.yml",
				"existing.txt", // Should coexist with new files
			},
		},
		{
			name:      "protect existing pigsty.yml",
			targetDir: filepath.Join(tmpDir, "protect-config"),
			overwrite: true,
			setup: func(dir string) error {
				if err := os.MkdirAll(dir, 0755); err != nil {
					return err
				}
				// Create existing protected files
				return os.WriteFile(filepath.Join(dir, "pigsty.yml"), []byte("original"), 0644)
			},
			checkFiles: []string{"pigsty.yml"},
			skipFiles:  []string{"pigsty.yml"}, // Should not be overwritten
		},
		{
			name:      "protect existing pki directory",
			targetDir: filepath.Join(tmpDir, "protect-pki"),
			overwrite: true,
			setup: func(dir string) error {
				if err := os.MkdirAll(filepath.Join(dir, "files/pki"), 0755); err != nil {
					return err
				}
				return os.WriteFile(filepath.Join(dir, "files/pki/ca.key"), []byte("original"), 0644)
			},
			checkFiles: []string{"files/pki/ca.key"},
			skipFiles:  []string{"files/pki/ca.key"}, // Should not be overwritten
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test environment if needed
			if tt.setup != nil {
				if err := tt.setup(tt.targetDir); err != nil {
					t.Fatalf("Setup failed: %v", err)
				}
			}

			// Store original content of protected files
			originalContent := make(map[string][]byte)
			for _, f := range tt.skipFiles {
				path := filepath.Join(tt.targetDir, f)
				content, err := os.ReadFile(path)
				if err == nil {
					originalContent[f] = content
				}
			}

			// Run installation
			srcTarball := createTestPigstyTarball(t)
			err := InstallPigsty(srcTarball, tt.targetDir, tt.overwrite)

			// Check error expectation
			if (err != nil) != tt.wantErr {
				t.Errorf("InstallPigsty() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err != nil {
				return // Skip further checks if we expected an error
			}

			// Verify expected files exist
			targetDir := tt.targetDir
			if targetDir == "" {
				targetDir = DefaultDir
			}
			for _, f := range tt.checkFiles {
				path := filepath.Join(targetDir, f)
				if _, err := os.Stat(path); os.IsNotExist(err) {
					t.Errorf("Expected file %s does not exist", path)
				}
			}

			// Verify protected files were not modified
			for _, f := range tt.skipFiles {
				path := filepath.Join(targetDir, f)
				content, err := os.ReadFile(path)
				if err != nil {
					t.Errorf("Failed to read protected file %s: %v", path, err)
					continue
				}
				if orig, exists := originalContent[f]; exists {
					if !bytes.Equal(content, orig) {
						t.Errorf("Protected file %s was modified", path)
					}
				}
			}
		})
	}
}

func TestExtractPigstyRejectsPathTraversal(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pigsty-traversal-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dst := filepath.Join(tmpDir, "install")
	if err := os.MkdirAll(dst, 0755); err != nil {
		t.Fatalf("Failed to create install dir: %v", err)
	}

	payload := createCustomTarball(t, "pigsty/../../escape.txt", []byte("pwned"))
	err = extractPigsty(payload, dst)
	if err == nil {
		t.Fatalf("extractPigsty() expected path traversal error, got nil")
	}

	outsidePath := filepath.Join(tmpDir, "escape.txt")
	if _, statErr := os.Stat(outsidePath); !os.IsNotExist(statErr) {
		t.Fatalf("path traversal file should not be created outside destination: %s", outsidePath)
	}
}

func TestResolveArchiveTargetPathAcceptsDoubleSlash(t *testing.T) {
	dst := t.TempDir()
	rel, target, err := resolveArchiveTargetPath(dst, "pigsty//files/readme.txt")
	if err != nil {
		t.Fatalf("resolveArchiveTargetPath returned error: %v", err)
	}
	if rel != filepath.Clean("files/readme.txt") {
		t.Fatalf("unexpected rel path: %s", rel)
	}
	if filepath.Dir(target) != filepath.Join(dst, "files") {
		t.Fatalf("unexpected target path: %s", target)
	}
}

func TestExtractPigstyRejectsExistingSymlinkPathComponent(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pigsty-existing-link-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dst := filepath.Join(tmpDir, "install")
	outside := filepath.Join(tmpDir, "outside")
	if err := os.MkdirAll(dst, 0755); err != nil {
		t.Fatalf("Failed to create install dir: %v", err)
	}
	if err := os.MkdirAll(outside, 0755); err != nil {
		t.Fatalf("Failed to create outside dir: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(dst, "link")); err != nil {
		t.Fatalf("Failed to create symlink in install dir: %v", err)
	}

	payload := createCustomTarball(t, "pigsty/link/escape.txt", []byte("pwned"))
	err = extractPigsty(payload, dst)
	if err == nil {
		t.Fatalf("extractPigsty() expected symlink path component error, got nil")
	}

	if _, statErr := os.Stat(filepath.Join(outside, "escape.txt")); !os.IsNotExist(statErr) {
		t.Fatalf("file should not be created outside destination via symlink path")
	}
}

func TestExtractPigstyRejectsExistingSymlinkTargetFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pigsty-existing-target-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dst := filepath.Join(tmpDir, "install")
	outside := filepath.Join(tmpDir, "outside")
	if err := os.MkdirAll(dst, 0755); err != nil {
		t.Fatalf("Failed to create install dir: %v", err)
	}
	if err := os.MkdirAll(outside, 0755); err != nil {
		t.Fatalf("Failed to create outside dir: %v", err)
	}
	outsideFile := filepath.Join(outside, "victim.txt")
	if err := os.WriteFile(outsideFile, []byte("old"), 0644); err != nil {
		t.Fatalf("Failed to create outside file: %v", err)
	}
	if err := os.Symlink(outsideFile, filepath.Join(dst, "victim.txt")); err != nil {
		t.Fatalf("Failed to create target symlink: %v", err)
	}

	payload := createCustomTarball(t, "pigsty/victim.txt", []byte("new-content"))
	err = extractPigsty(payload, dst)
	if err == nil {
		t.Fatalf("extractPigsty() expected symlink target file error, got nil")
	}

	content, readErr := os.ReadFile(outsideFile)
	if readErr != nil {
		t.Fatalf("Failed to read outside file: %v", readErr)
	}
	if string(content) != "old" {
		t.Fatalf("outside file should remain unchanged, got: %s", string(content))
	}
}

func TestExtractPigstyAllowsSafeRelativeSymlinkTarget(t *testing.T) {
	dst := t.TempDir()
	payload := createSymlinkTarball(t,
		"pigsty/templates/olap.yml",
		"../roles/pgsql/templates/olap.yml",
		map[string][]byte{
			"pigsty/roles/pgsql/templates/olap.yml": []byte("shared-template"),
		},
	)

	if err := extractPigsty(payload, dst); err != nil {
		t.Fatalf("extractPigsty() unexpected error: %v", err)
	}

	linkPath := filepath.Join(dst, "templates", "olap.yml")
	linkInfo, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatalf("failed to stat symlink: %v", err)
	}
	if linkInfo.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected symlink at %s", linkPath)
	}
	linkTarget, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("failed to read symlink target: %v", err)
	}
	if linkTarget != "../roles/pgsql/templates/olap.yml" {
		t.Fatalf("unexpected symlink target: %s", linkTarget)
	}
}

func TestExtractPigstyRejectsEscapingRelativeSymlinkTarget(t *testing.T) {
	dst := t.TempDir()
	payload := createSymlinkTarball(t, "pigsty/templates/bad", "../../../etc/passwd", nil)

	err := extractPigsty(payload, dst)
	if err == nil {
		t.Fatalf("extractPigsty() expected symlink escape error, got nil")
	}
}

func TestExtractPigstyRejectsAbsoluteSymlinkTarget(t *testing.T) {
	dst := t.TempDir()
	payload := createSymlinkTarball(t, "pigsty/templates/bad", "/etc/passwd", nil)

	err := extractPigsty(payload, dst)
	if err == nil {
		t.Fatalf("extractPigsty() expected absolute symlink target error, got nil")
	}
}

func createTestPigstyTarball(t *testing.T) []byte {
	t.Helper()

	buf := new(bytes.Buffer)
	gw := gzip.NewWriter(buf)
	tw := tar.NewWriter(gw)

	addDir := func(name string) {
		hdr := &tar.Header{
			Name:     name,
			Mode:     0755,
			Typeflag: tar.TypeDir,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("failed to write dir header %s: %v", name, err)
		}
	}

	addFile := func(name string, data []byte, mode int64) {
		hdr := &tar.Header{
			Name:     name,
			Mode:     mode,
			Size:     int64(len(data)),
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("failed to write file header %s: %v", name, err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatalf("failed to write file body %s: %v", name, err)
		}
	}

	addDir("pigsty")
	addDir("pigsty/files")
	addDir("pigsty/files/pki")
	addFile("pigsty/pigsty.yml", []byte("all:\n  children:\n    pg-meta:\n"), 0644)
	addFile("pigsty/files/pki/ca.key", []byte("new-key"), 0600)

	if err := tw.Close(); err != nil {
		t.Fatalf("failed to close tar writer: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("failed to close gzip writer: %v", err)
	}

	return buf.Bytes()
}

func createCustomTarball(t *testing.T, filePath string, content []byte) []byte {
	t.Helper()

	buf := new(bytes.Buffer)
	gw := gzip.NewWriter(buf)
	tw := tar.NewWriter(gw)

	root := &tar.Header{
		Name:     "pigsty",
		Mode:     0755,
		Typeflag: tar.TypeDir,
	}
	if err := tw.WriteHeader(root); err != nil {
		t.Fatalf("failed to write root dir header: %v", err)
	}

	hdr := &tar.Header{
		Name:     filePath,
		Mode:     0644,
		Size:     int64(len(content)),
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("failed to write custom file header %s: %v", filePath, err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("failed to write custom file content %s: %v", filePath, err)
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("failed to close tar writer: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("failed to close gzip writer: %v", err)
	}
	return buf.Bytes()
}

func createSymlinkTarball(t *testing.T, linkPath, linkTarget string, files map[string][]byte) []byte {
	t.Helper()

	buf := new(bytes.Buffer)
	gw := gzip.NewWriter(buf)
	tw := tar.NewWriter(gw)

	root := &tar.Header{
		Name:     "pigsty",
		Mode:     0755,
		Typeflag: tar.TypeDir,
	}
	if err := tw.WriteHeader(root); err != nil {
		t.Fatalf("failed to write root dir header: %v", err)
	}

	for filePath, content := range files {
		hdr := &tar.Header{
			Name:     filePath,
			Mode:     0644,
			Size:     int64(len(content)),
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("failed to write file header %s: %v", filePath, err)
		}
		if _, err := tw.Write(content); err != nil {
			t.Fatalf("failed to write file content %s: %v", filePath, err)
		}
	}

	linkHeader := &tar.Header{
		Name:     linkPath,
		Mode:     0777,
		Typeflag: tar.TypeSymlink,
		Linkname: linkTarget,
	}
	if err := tw.WriteHeader(linkHeader); err != nil {
		t.Fatalf("failed to write symlink header %s -> %s: %v", linkPath, linkTarget, err)
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("failed to close tar writer: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("failed to close gzip writer: %v", err)
	}
	return buf.Bytes()
}
