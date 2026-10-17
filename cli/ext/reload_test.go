package ext

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pig/internal/config"
	"pig/internal/utils"
)

func TestExtensionCatalogURLsMirrorUsesPigstyCCOnly(t *testing.T) {
	urls := extensionCatalogURLs(true)
	if len(urls) != 1 {
		t.Fatalf("mirror urls length = %d, want 1", len(urls))
	}
	want := config.RepoPigstyCC + "/ext/data/extension.csv"
	if urls[0] != want {
		t.Fatalf("mirror url = %q, want %q", urls[0], want)
	}
}

func TestExtensionCatalogURLsDefaultKeepsFallbackOrder(t *testing.T) {
	urls := extensionCatalogURLs(false)
	if len(urls) != 2 {
		t.Fatalf("default urls length = %d, want 2", len(urls))
	}
	if urls[0] != config.RepoPigstyIO+"/ext/data/extension.csv" {
		t.Fatalf("default first url = %q", urls[0])
	}
	if urls[1] != config.RepoPigstyCC+"/ext/data/extension.csv" {
		t.Fatalf("default fallback url = %q", urls[1])
	}
}

func TestMirrorAttemptDetailHandlesSingleMirrorURL(t *testing.T) {
	detail := utils.MirrorAttemptDetail([]string{config.RepoPigstyCC + "/ext/data/extension.csv"}, errors.New("boom"))
	if !strings.Contains(detail, config.RepoPigstyCC+"/ext/data/extension.csv") {
		t.Fatalf("detail missing mirror url: %q", detail)
	}
	if !strings.Contains(detail, "boom") {
		t.Fatalf("detail missing error: %q", detail)
	}
}

type catalogRoundTripFunc func(*http.Request) (*http.Response, error)

func (f catalogRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestReloadCatalogCountsEveryDownloadedRecord(t *testing.T) {
	ec := loadEmbeddedCatalogForTest(t)
	want := len(ec.Extensions)
	client := utils.DefaultClient()
	origTransport, origConfigDir := client.Transport, config.ConfigDir
	config.ConfigDir = t.TempDir()
	client.Transport = catalogRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		var body io.ReadCloser = http.NoBody
		if req.Method == http.MethodGet {
			body = io.NopCloser(bytes.NewReader(embedExtensionData))
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       body,
			Request:    req,
		}, nil
	})
	t.Cleanup(func() { client.Transport, config.ConfigDir = origTransport, origConfigDir })

	result := ReloadCatalogResult(true)
	data, ok := result.Data.(*ReloadResultData)
	if !result.Success || !ok || data.ExtensionCount != want {
		t.Fatalf("reload result = %#v, want %d catalog records", result, want)
	}
	if !strings.Contains(result.Message, fmt.Sprintf("%d extensions", want)) {
		t.Fatalf("reload message = %q, want %d catalog records", result.Message, want)
	}
	saved, err := os.ReadFile(filepath.Join(config.ConfigDir, "extension.csv"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(saved, embedExtensionData) {
		t.Fatal("reload must preserve the downloaded catalog")
	}
}
