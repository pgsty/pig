package get

import (
	"net/http"
	"net/http/httptest"
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
