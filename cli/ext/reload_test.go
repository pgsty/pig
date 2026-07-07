package ext

import (
	"errors"
	"strings"
	"testing"

	"pig/internal/config"
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

func TestReloadAttemptDetailHandlesSingleMirrorURL(t *testing.T) {
	detail := reloadAttemptDetail([]string{config.RepoPigstyCC + "/ext/data/extension.csv"}, errors.New("boom"))
	if !strings.Contains(detail, config.RepoPigstyCC+"/ext/data/extension.csv") {
		t.Fatalf("detail missing mirror url: %q", detail)
	}
	if !strings.Contains(detail, "boom") {
		t.Fatalf("detail missing error: %q", detail)
	}
}
