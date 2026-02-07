package get

import (
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
