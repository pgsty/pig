package repo

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"pig/internal/config"
	"reflect"
	"strings"
	"testing"
)

func TestELLocalRepoContentEnablesModuleHotfixesOnEL8AndEL9(t *testing.T) {
	originalVersion := config.OSVersion
	t.Cleanup(func() { config.OSVersion = originalVersion })

	for _, tc := range []struct {
		version string
		want    bool
	}{
		{version: "7", want: false},
		{version: "8", want: true},
		{version: "9", want: true},
		{version: "10", want: false},
	} {
		t.Run("el"+tc.version, func(t *testing.T) {
			config.OSVersion = tc.version
			content := elLocalRepoContent("/www/pigsty")
			if got := strings.Contains(content, "module_hotfixes=1"); got != tc.want {
				t.Fatalf("EL%s local repo module_hotfixes=%t, want %t:\n%s", tc.version, got, tc.want, content)
			}
			for _, required := range []string{"[pigsty-local]", "baseurl=file:///www/pigsty", "enabled=1", "gpgcheck=0"} {
				if !strings.Contains(content, required) {
					t.Fatalf("EL%s local repo missing %q:\n%s", tc.version, required, content)
				}
			}
		})
	}
}

func TestBootWithResultReusesCommittedRepoAsRoot(t *testing.T) {
	origEUID, origAddLocal := repoBootEUID, repoBootAddLocal
	t.Cleanup(func() {
		repoBootEUID, repoBootAddLocal = origEUID, origAddLocal
	})
	repoBootEUID = func() int { return 0 }
	t.Setenv("PIG_NO_SUDO", "1")

	target := t.TempDir()
	repoDir := filepath.Join(target, "pigsty")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "repo_complete"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	pkg := filepath.Join(t.TempDir(), "already-removed.tgz")
	called := false
	repoBootAddLocal = func(got string) error {
		called = true
		if got != repoDir {
			t.Fatalf("local repo path = %q", got)
		}
		return nil
	}

	result := BootWithResult(pkg, target)
	if !result.Success || !called || !strings.Contains(result.Message, "Reused existing repo") {
		t.Fatalf("unexpected reuse result: %+v", result)
	}
	if !strings.Contains(result.Detail, "offline package was not inspected") {
		t.Fatalf("reuse detail = %q", result.Detail)
	}
	data, ok := result.Data.(*RepoBootData)
	if !ok || !data.Reused || len(data.ExtractedFiles) != 0 {
		t.Fatalf("reuse reported extracted files: %#v", result.Data)
	}
}

func TestRegularFileAtRejectsSymlinkMarker(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real-marker")
	link := filepath.Join(dir, "repo_complete")
	if err := os.WriteFile(target, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if regularFileAt(link) {
		t.Fatal("repo_complete symlink was accepted as a committed repository marker")
	}
}

func TestRegularFileAtAcceptsEmptyMarker(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repo_complete")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if !regularFileAt(path) {
		t.Fatal("empty regular repo_complete marker was rejected")
	}
}

func TestEnsureRepoTargetLayoutCreatesCanonicalSymlink(t *testing.T) {
	root := t.TempDir()
	repoHome := filepath.Join(root, "www")
	repoData := filepath.Join(root, "data", "nginx")

	if err := ensureRepoTargetLayout(repoHome, repoHome, repoData); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(repoHome)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("repository home mode = %v, want symlink", info.Mode())
	}
	link, err := os.Readlink(repoHome)
	if err != nil || link != repoData {
		t.Fatalf("repository home link = %q, %v; want %q", link, err, repoData)
	}
	if dataInfo, err := os.Stat(repoData); err != nil || !dataInfo.IsDir() {
		t.Fatalf("repository data directory = %+v, %v", dataInfo, err)
	}
}

func TestEnsureRepoTargetLayoutPreservesExistingDirectoryAndSymlink(t *testing.T) {
	root := t.TempDir()
	repoData := filepath.Join(root, "data", "nginx")
	if err := os.MkdirAll(repoData, 0o755); err != nil {
		t.Fatal(err)
	}

	directory := filepath.Join(root, "existing-directory")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ensureRepoTargetLayout(directory, directory, repoData); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(directory); err != nil || !info.IsDir() {
		t.Fatalf("existing directory was not preserved: %+v, %v", info, err)
	}

	linkTarget := filepath.Join(root, "alternate-data")
	if err := os.MkdirAll(linkTarget, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "existing-link")
	if err := os.Symlink(linkTarget, link); err != nil {
		t.Fatal(err)
	}
	if err := ensureRepoTargetLayout(link, link, repoData); err != nil {
		t.Fatal(err)
	}
	if got, err := os.Readlink(link); err != nil || got != linkTarget {
		t.Fatalf("existing repository link = %q, %v; want %q", got, err, linkTarget)
	}
}

func TestEnsureRepoTargetLayoutRejectsInvalidCanonicalPath(t *testing.T) {
	root := t.TempDir()
	repoHome := filepath.Join(root, "www")
	if err := os.WriteFile(repoHome, []byte("occupied"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureRepoTargetLayout(repoHome, repoHome, filepath.Join(root, "data", "nginx")); err == nil {
		t.Fatal("regular file was accepted as the canonical repository home")
	}
}

func TestBootWithResultStreamsArchiveWithGo(t *testing.T) {
	origEUID, origAddLocal := repoBootEUID, repoBootAddLocal
	t.Cleanup(func() {
		repoBootEUID, repoBootAddLocal = origEUID, origAddLocal
	})
	repoBootEUID = func() int { return 0 }
	repoBootAddLocal = func(string) error { return nil }
	t.Setenv("PATH", "") // prove extraction does not discover tar, gzip, or mkdir

	pkg := filepath.Join(t.TempDir(), "pkg.tgz")
	f, err := os.Create(pkg)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for name, body := range map[string]string{
		"pigsty/repo_complete": "ok",
		"pigsty/Packages":      "metadata",
	} {
		content := []byte(body)
		if err := tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	target := t.TempDir()
	result := BootWithResult(pkg, target)
	content, readErr := os.ReadFile(filepath.Join(target, "pigsty", "Packages"))
	if !result.Success || readErr != nil || string(content) != "metadata" {
		t.Fatalf("unexpected Go extraction result: %+v content=%q err=%v", result, content, readErr)
	}
}

func TestCacheBootRoundTripWithMultipleRepos(t *testing.T) {
	origEUID, origAddLocal := repoBootEUID, repoBootAddLocal
	t.Cleanup(func() {
		repoBootEUID, repoBootAddLocal = origEUID, origAddLocal
	})
	repoBootEUID = func() int { return 0 }
	repoBootAddLocal = func(string) error { return nil }

	root := t.TempDir()
	source := filepath.Join(root, "source")
	for name, files := range map[string]map[string]string{
		"pigsty": {"repo_complete": "ok", "Packages": "packages"},
		"docker": {"metadata": "docker"},
	} {
		for file, body := range files {
			path := filepath.Join(source, name, file)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	pkg := filepath.Join(root, "pkg.tgz")
	if err := cacheForTest(source, pkg, []string{"pigsty", "docker"}); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "target")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	result := BootWithResult(pkg, target)
	if !result.Success {
		t.Fatalf("round-trip boot failed: %+v", result)
	}
	data, ok := result.Data.(*RepoBootData)
	if !ok || data.Reused || !reflect.DeepEqual(data.ExtractedFiles, []string{"docker", "pigsty"}) {
		t.Fatalf("round-trip extraction data = %#v", result.Data)
	}
	for path, want := range map[string]string{
		"pigsty/Packages": "packages",
		"docker/metadata": "docker",
	} {
		content, err := os.ReadFile(filepath.Join(target, path))
		if err != nil || string(content) != want {
			t.Fatalf("round-trip %s = %q, err=%v", path, content, err)
		}
	}
}

func TestRepoCommitOrderMovesPigstyLast(t *testing.T) {
	stage := t.TempDir()
	for _, name := range []string{"pigsty", "zeta", "docker"} {
		if err := os.Mkdir(filepath.Join(stage, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(stage)
	if err != nil {
		t.Fatal(err)
	}
	ordered := repoCommitOrder(entries)
	got := make([]string, 0, len(ordered))
	for _, entry := range ordered {
		got = append(got, entry.Name())
	}
	want := []string{"docker", "zeta", "pigsty"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commit order = %v, want %v", got, want)
	}
}

func TestExtractOfflineWithResultDoesNotWriteLocalDefinition(t *testing.T) {
	origEUID, origAddLocal := repoBootEUID, repoBootAddLocal
	t.Cleanup(func() {
		repoBootEUID, repoBootAddLocal = origEUID, origAddLocal
	})
	repoBootEUID = func() int { return 0 }
	repoBootAddLocal = func(string) error {
		t.Fatal("extract-only workflow wrote local repository definition")
		return nil
	}

	pkg := filepath.Join(t.TempDir(), "pkg.tgz")
	f, err := os.Create(pkg)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	body := []byte("ok")
	if err := tw.WriteHeader(&tar.Header{Name: "pigsty/repo_complete", Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(body))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	target := t.TempDir()
	result := ExtractOfflineWithResult(pkg, target)
	data := result.Data.(*RepoBootData)
	if !result.Success || data.LocalRepoAdded || data.LocalRepoPath != "" {
		t.Fatalf("extract-only result = %+v data=%+v", result, data)
	}
}

func TestBootWithResultRejectsNonRootInsteadOfInvokingTar(t *testing.T) {
	origEUID := repoBootEUID
	t.Cleanup(func() { repoBootEUID = origEUID })
	repoBootEUID = func() int { return 501 }
	pkg := filepath.Join(t.TempDir(), "pkg.tgz")
	if err := os.WriteFile(pkg, []byte("not-read-before-root-check"), 0o600); err != nil {
		t.Fatal(err)
	}
	result := BootWithResult(pkg, t.TempDir())
	if result.Success || !strings.Contains(result.Message, "requires root") {
		t.Fatalf("unexpected non-root result: %+v", result)
	}
}
