package archive

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func makeTarGz(t *testing.T, names ...string) string {
	t.Helper()
	filename := filepath.Join(t.TempDir(), "pkg.tgz")
	f, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for _, name := range names {
		body := []byte("ok")
		if strings.HasSuffix(name, "/") {
			body = nil
		}
		hdr := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}
		if body == nil {
			hdr.Typeflag, hdr.Mode = tar.TypeDir, 0o755
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(body); err != nil {
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
	return filename
}

func makeTarGzHeader(t *testing.T, hdr *tar.Header, body []byte) string {
	return makeTarGzEntries(t, tarGzEntry{header: hdr, body: body})
}

type tarGzEntry struct {
	header *tar.Header
	body   []byte
}

func makeTarGzEntries(t *testing.T, entries ...tarGzEntry) string {
	t.Helper()
	filename := filepath.Join(t.TempDir(), "pkg.tgz")
	f, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for _, entry := range entries {
		if err := tw.WriteHeader(entry.header); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(entry.body); err != nil {
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
	return filename
}

func TestExtractTarGz(t *testing.T) {
	pkg := makeTarGz(t, "pigsty/", "pigsty/repo_complete", "pigsty/repodata/", "pigsty/repodata/repomd.xml")
	dst := t.TempDir()
	if err := ExtractTarGz(pkg, dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "pigsty", "repo_complete")); err != nil {
		t.Fatal(err)
	}
}

func TestExtractTarGzAllowsMultipleSafeRoots(t *testing.T) {
	pkg := makeTarGz(t, "pigsty/", "pigsty/repo_complete", "docker/", "docker/metadata")
	dst := t.TempDir()
	if err := ExtractTarGz(pkg, dst); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"pigsty/repo_complete", "docker/metadata"} {
		if _, err := os.Stat(filepath.Join(dst, path)); err != nil {
			t.Fatalf("missing extracted %s: %v", path, err)
		}
	}
}

func TestExtractTarGzStillRejectsTraversalAndLinks(t *testing.T) {
	if err := ExtractTarGz(makeTarGz(t, "../escape"), t.TempDir()); err == nil {
		t.Fatal("multi-root extraction accepted traversal")
	}
	hdr := &tar.Header{Name: "pigsty/link", Linkname: "pigsty/target", Mode: 0o777, Typeflag: tar.TypeSymlink}
	if err := ExtractTarGz(makeTarGzHeader(t, hdr, nil), t.TempDir()); err == nil {
		t.Fatal("multi-root extraction accepted symlink")
	}
}

func TestExtractTarGzRoot(t *testing.T) {
	pkg := makeTarGz(t, "rpmbuild/", "rpmbuild/SPECS/", "rpmbuild/SPECS/example.spec")
	dst := t.TempDir()
	if err := ExtractTarGzRoot(pkg, dst, "rpmbuild"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "rpmbuild", "SPECS", "example.spec")); err != nil {
		t.Fatal(err)
	}
}

func TestExtractTarGzRootPreservesModificationTimes(t *testing.T) {
	fileTime := time.Date(2026, 7, 14, 3, 2, 1, 0, time.UTC)
	directoryTime := fileTime.Add(-time.Hour)
	directory := &tar.Header{Name: "rpmbuild/SPECS/", Mode: 0o755, Typeflag: tar.TypeDir, ModTime: directoryTime}
	file := &tar.Header{
		Name: "rpmbuild/SPECS/example.spec", Mode: 0o644, Size: 2,
		Typeflag: tar.TypeReg, ModTime: fileTime,
	}
	dst := t.TempDir()
	archive := makeTarGzEntries(t, tarGzEntry{header: directory}, tarGzEntry{header: file, body: []byte("ok")})
	if err := ExtractTarGzRoot(archive, dst, "rpmbuild"); err != nil {
		t.Fatal(err)
	}
	directoryInfo, err := os.Stat(filepath.Join(dst, "rpmbuild", "SPECS"))
	if err != nil {
		t.Fatal(err)
	}
	fileInfo, err := os.Stat(filepath.Join(dst, "rpmbuild", "SPECS", "example.spec"))
	if err != nil {
		t.Fatal(err)
	}
	if !directoryInfo.ModTime().Equal(directoryTime) || !fileInfo.ModTime().Equal(fileTime) {
		t.Fatalf("modification times dir=%s/%s file=%s/%s", directoryInfo.ModTime(), directoryTime, fileInfo.ModTime(), fileTime)
	}
}

func TestExtractTarGzRootRejectsOtherRoots(t *testing.T) {
	pkg := makeTarGz(t, "other/", "other/example.spec")
	if err := ExtractTarGzRoot(pkg, t.TempDir(), "rpmbuild"); err == nil {
		t.Fatal("archive with an unexpected root was accepted")
	}
}

func TestExtractTarGzRootRejectsInvalidRequiredRoot(t *testing.T) {
	for _, root := range []string{"", ".", "..", "nested/root", " rpmbuild"} {
		if err := ExtractTarGzRoot("unused", t.TempDir(), root); err == nil {
			t.Fatalf("invalid required root %q was accepted", root)
		}
	}
}

func TestExtractTarGzRootSkipsPAXGlobalHeader(t *testing.T) {
	body := []byte("ok")
	global := &tar.Header{
		Name: "pax_global_header", Typeflag: tar.TypeXGlobalHeader,
		PAXRecords: map[string]string{"comment": "metadata only"},
	}
	file := &tar.Header{Name: "rpmbuild/SPECS/example.spec", Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}
	archive := makeTarGzEntries(t, tarGzEntry{header: global}, tarGzEntry{header: file, body: body})
	dst := t.TempDir()
	if err := ExtractTarGzRoot(archive, dst, "rpmbuild"); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(dst, "rpmbuild", "SPECS", "example.spec")); err != nil || string(got) != "ok" {
		t.Fatalf("extracted file = %q, err=%v", got, err)
	}
}

func TestExtractTarGzRootRejectsExistingSymlinkParent(t *testing.T) {
	dst := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dst, "rpmbuild"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dst, "rpmbuild", "SPECS")); err != nil {
		t.Fatal(err)
	}
	archive := makeTarGz(t, "rpmbuild/SPECS/escape.spec")
	if err := ExtractTarGzRoot(archive, dst, "rpmbuild"); err == nil {
		t.Fatal("archive extracted through an existing symlink parent")
	}
	if _, err := os.Stat(filepath.Join(outside, "escape.spec")); !os.IsNotExist(err) {
		t.Fatalf("outside path was modified: %v", err)
	}
}

func TestExtractTarGzRootRejectsExistingSymlinkTarget(t *testing.T) {
	dst := t.TempDir()
	outside := filepath.Join(t.TempDir(), "victim.spec")
	if err := os.WriteFile(outside, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	targetDir := filepath.Join(dst, "rpmbuild", "SPECS")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(targetDir, "example.spec")); err != nil {
		t.Fatal(err)
	}
	archive := makeTarGz(t, "rpmbuild/SPECS/example.spec")
	if err := ExtractTarGzRoot(archive, dst, "rpmbuild"); err == nil {
		t.Fatal("archive replaced an existing symlink target")
	}
	if got, err := os.ReadFile(outside); err != nil || string(got) != "old" {
		t.Fatalf("outside file = %q, err=%v", got, err)
	}
}

func TestExtractTarGzRootRejectsDirectoryFileConflict(t *testing.T) {
	dst := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dst, "rpmbuild"), 0o755); err != nil {
		t.Fatal(err)
	}
	conflict := filepath.Join(dst, "rpmbuild", "SPECS")
	if err := os.WriteFile(conflict, []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}
	header := &tar.Header{Name: "rpmbuild/SPECS/", Mode: 0o755, Typeflag: tar.TypeDir}
	if err := ExtractTarGzRoot(makeTarGzHeader(t, header, nil), dst, "rpmbuild"); err == nil {
		t.Fatal("archive directory replaced an existing regular file")
	}
}

func TestExtractTarGzRootRejectsLinkEntries(t *testing.T) {
	for name, kind := range map[string]byte{"symlink": tar.TypeSymlink, "hardlink": tar.TypeLink} {
		t.Run(name, func(t *testing.T) {
			header := &tar.Header{Name: "rpmbuild/link", Linkname: "rpmbuild/target", Mode: 0o777, Typeflag: kind}
			if err := ExtractTarGzRoot(makeTarGzHeader(t, header, nil), t.TempDir(), "rpmbuild"); err == nil {
				t.Fatalf("%s entry was accepted", name)
			}
		})
	}
}

func TestExtractTarGzRejectsTraversalAndLinks(t *testing.T) {
	for _, name := range []string{"../escape", "/absolute"} {
		t.Run(name, func(t *testing.T) {
			err := ExtractTarGz(makeTarGz(t, name), t.TempDir())
			if err == nil {
				t.Fatalf("unsafe path %q was accepted", name)
			}
		})
	}
}

func TestExtractTarGzRejectsLinkEntries(t *testing.T) {
	for name, kind := range map[string]byte{"symlink": tar.TypeSymlink, "hardlink": tar.TypeLink} {
		t.Run(name, func(t *testing.T) {
			hdr := &tar.Header{Name: "pigsty/link", Linkname: "pigsty/target", Mode: 0o777, Typeflag: kind}
			if err := ExtractTarGz(makeTarGzHeader(t, hdr, nil), t.TempDir()); err == nil {
				t.Fatalf("%s entry was accepted", name)
			}
		})
	}
}

func TestExtractTarGzRejectsTruncatedStream(t *testing.T) {
	pkg := makeTarGz(t, "pigsty/", "pigsty/repo_complete")
	content, err := os.ReadFile(pkg)
	if err != nil {
		t.Fatal(err)
	}
	if len(content) < 16 {
		t.Fatal("test archive is unexpectedly short")
	}
	truncated := filepath.Join(t.TempDir(), "truncated.tgz")
	if err := os.WriteFile(truncated, content[:len(content)-8], 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ExtractTarGz(truncated, t.TempDir()); err == nil {
		t.Fatal("truncated gzip stream was accepted")
	}
}
