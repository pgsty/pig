package get

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFetchLatestPigVersionTrimsWhitespace(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("1.7.0\n"))
	}))
	defer server.Close()

	version, err := fetchLatestPigVersion(server.URL)
	if err != nil {
		t.Fatalf("fetchLatestPigVersion: %v", err)
	}
	if version != "1.7.0" {
		t.Fatalf("version = %q, want %q", version, "1.7.0")
	}
}

func TestFetchLatestPigVersionRejectsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
	}))
	defer server.Close()

	_, err := fetchLatestPigVersion(server.URL)
	if err == nil || !strings.Contains(err.Error(), "502 Bad Gateway") {
		t.Fatalf("error = %v, want HTTP status failure", err)
	}
}

func TestFetchLatestPigVersionRejectsInvalidResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not-a-version\n"))
	}))
	defer server.Close()

	_, err := fetchLatestPigVersion(server.URL)
	if err == nil || !strings.Contains(err.Error(), "invalid version response") {
		t.Fatalf("error = %v, want invalid version response", err)
	}
}

func TestVerifyReleaseChecksum(t *testing.T) {
	content := []byte("package-content")
	digest := sha256.Sum256(content)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/checksums.txt" {
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprintf(w, "%x  pig.rpm\n", digest)
	}))
	t.Cleanup(server.Close)
	path := filepath.Join(t.TempDir(), "pig.rpm")
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifyReleaseChecksum([]string{server.URL + "/checksums.txt"}, "pig.rpm", path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifyReleaseChecksum([]string{server.URL + "/checksums.txt"}, "pig.rpm", path); err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("checksum mismatch error = %v", err)
	}
}

func TestVerifyReleaseChecksumRejectsMissingEntryAndHTTPError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pig.rpm")
	if err := os.WriteFile(path, []byte("package"), 0600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/missing" {
			_, _ = w.Write([]byte(strings.Repeat("0", 64) + "  other.rpm\n"))
			return
		}
		http.Error(w, "unavailable", http.StatusBadGateway)
	}))
	t.Cleanup(server.Close)
	if err := verifyReleaseChecksum([]string{server.URL + "/missing"}, "pig.rpm", path); err == nil || !strings.Contains(err.Error(), "no valid") {
		t.Fatalf("missing checksum error = %v", err)
	}
	if err := verifyReleaseChecksum([]string{server.URL + "/error"}, "pig.rpm", path); err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("HTTP checksum error = %v", err)
	}
}

func TestVerifyReleaseChecksumFallsBackWhenMirrorMetadataIsMissing(t *testing.T) {
	content := []byte("package")
	digest := sha256.Sum256(content)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/mirror" {
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprintf(w, "%x  pig.rpm\n", digest)
	}))
	t.Cleanup(server.Close)
	path := filepath.Join(t.TempDir(), "pig.rpm")
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifyReleaseChecksum([]string{server.URL + "/mirror", server.URL + "/github"}, "pig.rpm", path); err != nil {
		t.Fatalf("checksum fallback failed: %v", err)
	}
}
