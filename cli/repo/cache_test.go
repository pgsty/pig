package repo

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
)

func TestPreparePackageOutputRejectsRootPath(t *testing.T) {
	if _, err := preparePackageOutput(string(os.PathSeparator)); err == nil {
		t.Fatalf("preparePackageOutput('/') should fail")
	}
}

func TestPreparePackageOutputRejectsDirectory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pig-cache-dir-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if _, err := preparePackageOutput(tmpDir); err == nil {
		t.Fatalf("preparePackageOutput(directory) should fail")
	}
}

func TestPreparePackageOutputRejectsSymlink(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "existing.tgz")
	if err := os.WriteFile(target, []byte("previous-good-package"), 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(tmpDir, "pkg.tgz")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	if _, err := preparePackageOutput(link); err == nil {
		t.Fatal("preparePackageOutput(symlink) should fail")
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("package symlink should remain: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("package path is no longer a symlink: mode=%v", info.Mode())
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "previous-good-package" {
		t.Fatalf("symlink target changed: %q", got)
	}
}

func TestPreparePackageOutputPreservesExistingFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pig-cache-file-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	target := filepath.Join(tmpDir, "pkg.tgz")
	if err := os.WriteFile(target, []byte("old"), 0644); err != nil {
		t.Fatalf("failed to create old package file: %v", err)
	}

	mode, err := preparePackageOutput(target)
	if err != nil {
		t.Fatalf("preparePackageOutput returned error: %v", err)
	}
	if mode != 0644 {
		t.Fatalf("existing package mode = %04o, want 0644", mode)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("target file should be preserved: %v", err)
	}
	if string(got) != "old" {
		t.Fatalf("target file changed: got %q", got)
	}
}

func TestCacheRejectsEscapingRepoAndPreservesPackage(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "base")
	if err := os.MkdirAll(base, 0755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "pkg.tgz")
	if err := os.WriteFile(target, []byte("previous-good-package"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := Cache(base, target, []string{"../outside"}); err == nil {
		t.Fatal("Cache should reject a repository path outside the base directory")
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "previous-good-package" {
		t.Fatalf("failed cache replaced previous package: %q", got)
	}
}

func TestCacheRejectsRepoPathThroughSymlinkOutsideBase(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "base")
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(filepath.Join(outside, "repo"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(base, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(base, "link")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	target := filepath.Join(root, "pkg.tgz")
	if err := os.WriteFile(target, []byte("previous-good-package"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := Cache(base, target, []string{"link/repo"}); err == nil {
		t.Fatal("Cache should reject a repository path resolving outside the base directory")
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "previous-good-package" {
		t.Fatalf("rejected cache changed existing target: %q", got)
	}
}

func TestCacheRejectsPackageInsideInputTree(t *testing.T) {
	base := filepath.Join(t.TempDir(), "www")
	repoDir := filepath.Join(base, "pigsty")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(repoDir, "pkg.tgz")
	if err := os.WriteFile(target, []byte("previous"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := Cache(base, target, []string{"pigsty"}); err == nil {
		t.Fatal("Cache should reject an output package inside its input tree")
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "previous" {
		t.Fatalf("rejected cache changed existing target: %q", got)
	}
}

func TestCacheRejectsPackageThroughSymlinkIntoInputTree(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "www")
	repoDir := filepath.Join(base, "pigsty")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "repo-link")
	if err := os.Symlink(repoDir, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	if err := Cache(base, filepath.Join(link, "pkg.tgz"), []string{"pigsty"}); err == nil {
		t.Fatal("Cache should reject an output path resolving into its input tree")
	}
}

func TestPackageBoundaryResolvesSymlinkBeforeMissingSuffix(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "www")
	repoDir := filepath.Join(base, "pigsty")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "repo-link")
	if err := os.Symlink(repoDir, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	pkgPath := filepath.Join(link, "missing", "pkg.tgz")

	// Test the boundary directly: if this regresses, invoking Cache could place
	// its temporary archive inside the tree being walked.
	if err := ensurePackageOutsideRepos(base, pkgPath, []string{"pigsty"}); err == nil {
		t.Fatal("package path through a symlink and missing suffix should be rejected")
	}
	if _, err := os.Stat(filepath.Join(repoDir, "missing")); !os.IsNotExist(err) {
		t.Fatalf("boundary validation should not create the missing directory: %v", err)
	}
}

func TestCacheAtomicallyReplacesPackageWithValidArchive(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "www")
	repoDir := filepath.Join(base, "pigsty")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "hello.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "repo_complete"), []byte("ok"), 0644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "pkg.tgz")
	if err := os.WriteFile(target, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := Cache(base, target, []string{"pigsty"}); err != nil {
		t.Fatalf("Cache returned error: %v", err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("replacement mode = %04o, want preserved 0600", info.Mode().Perm())
	}
	f, err := os.Open(target)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("replacement is not a valid gzip stream: %v", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("replacement is not a valid tar archive: %v", err)
		}
		if header.Name == filepath.Join("pigsty", "hello.txt") {
			content, err := io.ReadAll(tr)
			if err != nil {
				t.Fatal(err)
			}
			if string(content) != "hello" {
				t.Fatalf("archived content = %q", content)
			}
			return
		}
	}
	t.Fatal("hello.txt not found in archive")
}

func TestCacheRejectsAllMissingReposAndPreservesPackage(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "www")
	if err := os.MkdirAll(base, 0755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "pkg.tgz")
	if err := os.WriteFile(target, []byte("previous-good-package"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := Cache(base, target, []string{"missing"}); err == nil {
		t.Fatal("Cache should fail when every requested repository is missing")
	}
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "previous-good-package" {
		t.Fatalf("failed cache changed existing package: %q", content)
	}
}

func TestCacheRejectsSymlinkEntries(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "www")
	repoDir := filepath.Join(base, "pigsty")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "repo_complete"), []byte("ok"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("repo_complete", filepath.Join(repoDir, "latest")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	if err := Cache(base, filepath.Join(root, "pkg.tgz"), []string{"pigsty"}); err == nil || !strings.Contains(err.Error(), "symbolic links") {
		t.Fatalf("Cache symlink error = %v", err)
	}
}

func TestCacheWithResultReportsOnlyIncludedRepos(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "www")
	repoDir := filepath.Join(base, "pigsty")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "repo_complete"), []byte("ok"), 0644); err != nil {
		t.Fatal(err)
	}

	result := CacheWithResult(base, filepath.Join(root, "pkg.tgz"), []string{"pigsty", "missing"})
	if !result.Success {
		t.Fatalf("CacheWithResult failed: %+v", result)
	}
	data, ok := result.Data.(*RepoCacheData)
	if !ok || len(data.IncludedRepos) != 1 || data.IncludedRepos[0] != "pigsty" {
		t.Fatalf("included repos = %#v", result.Data)
	}
}

func TestResolveCacheReposNormalizesAndDeduplicatesNames(t *testing.T) {
	base := t.TempDir()
	repoDir := filepath.Join(base, "pigsty")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "repo_complete"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := resolveCacheRepos(base, []string{"pigsty", "pigsty/"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"pigsty"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("included repos = %v, want %v", got, want)
	}
}

func TestResolveCacheReposRejectsOverlappingRoots(t *testing.T) {
	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "pigsty", "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "pigsty", "repo_complete"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveCacheRepos(base, []string{"pigsty", "pigsty/nested"}); err == nil || !strings.Contains(err.Error(), "overlapping") {
		t.Fatalf("overlapping roots error = %v", err)
	}
}

func TestCacheRejectsFIFOEntries(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "www")
	repoDir := filepath.Join(base, "pigsty")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "repo_complete"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(repoDir, "queue"), 0o600); err != nil {
		t.Skipf("fifo unavailable: %v", err)
	}
	if err := Cache(base, filepath.Join(root, "pkg.tgz"), []string{"pigsty"}); err == nil || !strings.Contains(err.Error(), "unsupported file type") {
		t.Fatalf("fifo cache error = %v", err)
	}
}

func TestCacheResultAfterCommitToleratesMetadataFailure(t *testing.T) {
	data := &RepoCacheData{
		SourceDir:     "/www",
		TargetPkg:     "/tmp/pkg.tgz",
		IncludedRepos: []string{"pigsty"},
	}
	inspectErr := errors.New("permission denied")

	result := cacheResultAfterCommit(data, inspectErr)
	if !result.Success || result.Code != 0 {
		t.Fatalf("post-commit metadata failure result = success %v, code %d; want success", result.Success, result.Code)
	}
	if result.Data != data {
		t.Fatalf("result data = %T %v, want original cache data", result.Data, result.Data)
	}
	if !strings.Contains(result.Detail, "package created but metadata inspection failed") || !strings.Contains(result.Detail, inspectErr.Error()) {
		t.Fatalf("result detail = %q, want metadata warning", result.Detail)
	}
	if strings.Contains(result.Message, "0.00 GiB") {
		t.Fatalf("result message reports an unknown size as zero: %q", result.Message)
	}
	if result.Message != "Created offline package /tmp/pkg.tgz" {
		t.Fatalf("result message = %q, want path without unknown size", result.Message)
	}
}

func TestCacheResultAfterCommitIncludesKnownMetadata(t *testing.T) {
	data := &RepoCacheData{
		TargetPkg:     "/tmp/pkg.tgz",
		IncludedRepos: []string{"pigsty"},
		PackageSize:   2 * 1024 * 1024 * 1024,
		PackageMD5:    "checksum",
	}

	result := cacheResultAfterCommit(data, nil)
	if !result.Success || result.Detail != "" {
		t.Fatalf("metadata success result = success %v, detail %q", result.Success, result.Detail)
	}
	if result.Message != "Created offline package /tmp/pkg.tgz (2.00 GiB)" {
		t.Fatalf("result message = %q", result.Message)
	}
}
