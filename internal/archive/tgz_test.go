package archive

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	t.Helper()
	filename := filepath.Join(t.TempDir(), "pkg.tgz")
	f, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(hdr); err != nil {
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

func TestExtractTarGzRejectsTraversalAndLinks(t *testing.T) {
	for _, name := range []string{"../escape", "/absolute", "other/file"} {
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
