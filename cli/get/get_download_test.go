package get

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pig/internal/config"
)

func TestIsValidVersion_NormalizesNumericPrefix(t *testing.T) {
	origSource := Source
	origRegion := Region
	origAllVersions := AllVersions
	defer func() {
		Source = origSource
		Region = origRegion
		AllVersions = origAllVersions
	}()

	Source = ViaNA
	Region = "default"
	AllVersions = nil

	info := IsValidVersion("1.2.3")
	if info == nil {
		t.Fatal("expected non-nil VersionInfo")
	}
	if info.Version != "v1.2.3" {
		t.Fatalf("Version=%q, want %q", info.Version, "v1.2.3")
	}
	if !strings.HasPrefix(info.DownloadURL, config.RepoPigstyIO) {
		t.Fatalf("DownloadURL=%q, want prefix %q", info.DownloadURL, config.RepoPigstyIO)
	}
	if !strings.Contains(info.DownloadURL, "/src/pigsty-v1.2.3.tgz") {
		t.Fatalf("DownloadURL=%q, want to contain %q", info.DownloadURL, "/src/pigsty-v1.2.3.tgz")
	}
}

func TestIsValidVersion_Invalid(t *testing.T) {
	if info := IsValidVersion("v1"); info != nil {
		t.Fatalf("expected nil for invalid version, got %+v", info)
	}
	if info := IsValidVersion("bad"); info != nil {
		t.Fatalf("expected nil for invalid version, got %+v", info)
	}
}

func TestDownloadSrcAtomicallyReplacesInvalidArchive(t *testing.T) {
	archive := testSourceArchive(t, "new")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer server.Close()
	restoreDownloadVersions(t, VersionInfo{Version: "v1.2.3", DownloadURL: server.URL})

	dir := t.TempDir()
	target := filepath.Join(dir, "pigsty-v1.2.3.tgz")
	if err := os.WriteFile(target, []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := DownloadSrc("v1.2.3", dir); err != nil {
		t.Fatalf("DownloadSrc: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, archive) {
		t.Fatal("final archive differs from validated response")
	}
	if info, err := os.Stat(target); err != nil || info.Mode().Perm() != 0644 {
		t.Fatalf("target mode = %v, %v; want 0644", info, err)
	}
	assertNoDownloadParts(t, dir)
}

func TestDownloadSrcPreservesExistingFileWhenCandidateChecksumDiffers(t *testing.T) {
	old := []byte("keep-this-existing-file")
	bad := []byte("not-a-tarball")
	checksum := md5.Sum(testSourceArchive(t, "expected"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(bad)
	}))
	defer server.Close()
	restoreDownloadVersions(t, VersionInfo{
		Version: "v1.2.3", DownloadURL: server.URL, Checksum: hex.EncodeToString(checksum[:]),
	})

	dir := t.TempDir()
	target := filepath.Join(dir, "pigsty-v1.2.3.tgz")
	if err := os.WriteFile(target, old, 0644); err != nil {
		t.Fatal(err)
	}
	if err := DownloadSrc("v1.2.3", dir); err == nil {
		t.Fatal("DownloadSrc unexpectedly accepted a checksum-mismatched candidate")
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, old) {
		t.Fatalf("existing file changed after failed download: %q", got)
	}
	assertNoDownloadParts(t, dir)
}

func TestDownloadSrcPreservesExistingFileWhenUncheckedCandidateIsInvalid(t *testing.T) {
	old := []byte("keep-partial-placeholder-until-valid-replacement")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("truncated-or-invalid-response"))
	}))
	defer server.Close()
	restoreDownloadVersions(t, VersionInfo{Version: "v1.2.3", DownloadURL: server.URL})

	dir := t.TempDir()
	target := filepath.Join(dir, "pigsty-v1.2.3.tgz")
	if err := os.WriteFile(target, old, 0644); err != nil {
		t.Fatal(err)
	}
	if err := DownloadSrc("v1.2.3", dir); err == nil || !strings.Contains(err.Error(), "archive is invalid") {
		t.Fatalf("DownloadSrc error = %v, want archive validation failure", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, old) {
		t.Fatalf("existing file changed after invalid unchecked candidate: %q", got)
	}
	assertNoDownloadParts(t, dir)
}

func TestDownloadSrcSkipsValidatedArchiveWithoutChecksum(t *testing.T) {
	archive := testSourceArchive(t, "existing")
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	restoreDownloadVersions(t, VersionInfo{Version: "v1.2.3", DownloadURL: server.URL})

	dir := t.TempDir()
	target := filepath.Join(dir, "pigsty-v1.2.3.tgz")
	if err := os.WriteFile(target, archive, 0644); err != nil {
		t.Fatal(err)
	}
	if err := DownloadSrc("v1.2.3", dir); err != nil {
		t.Fatalf("DownloadSrc: %v", err)
	}
	if requests != 0 {
		t.Fatalf("validated existing archive triggered %d requests", requests)
	}
}

func TestConcurrentDownloadsLeaveOneCompleteArchive(t *testing.T) {
	archive := testSourceArchive(t, strings.Repeat("payload", 1024))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer server.Close()
	restoreDownloadVersions(t, VersionInfo{Version: "v1.2.3", DownloadURL: server.URL})

	dir := t.TempDir()
	start := make(chan struct{})
	done := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			done <- DownloadSrc("v1.2.3", dir)
		}()
	}
	close(start)
	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Fatalf("concurrent DownloadSrc: %v", err)
		}
	}
	got, err := os.ReadFile(filepath.Join(dir, "pigsty-v1.2.3.tgz"))
	if err != nil || !bytes.Equal(got, archive) {
		t.Fatalf("final concurrent archive is incomplete: bytes=%d err=%v", len(got), err)
	}
	assertNoDownloadParts(t, dir)
}

func restoreDownloadVersions(t *testing.T, versions ...VersionInfo) {
	t.Helper()
	original := AllVersions
	AllVersions = versions
	t.Cleanup(func() { AllVersions = original })
}

func testSourceArchive(t *testing.T, content string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	body := []byte(content)
	if err := tw.WriteHeader(&tar.Header{Name: "pigsty/README.md", Mode: 0644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
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
	return buf.Bytes()
}

func assertNoDownloadParts(t *testing.T, dir string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, ".*.part-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary downloads left behind: %v", matches)
	}
}
