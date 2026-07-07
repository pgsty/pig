package sty

import (
	"strings"
	"testing"

	"pig/cli/get"
	"pig/internal/config"
)

func TestUsePigstyMirrorSourceRewritesDownloadURLs(t *testing.T) {
	origSource := get.Source
	origRegion := get.Region
	origAllVersions := get.AllVersions
	defer func() {
		get.Source = origSource
		get.Region = origRegion
		get.AllVersions = origAllVersions
	}()

	get.Source = get.ViaIO
	get.Region = "default"
	get.AllVersions = []get.VersionInfo{
		{
			Version:     "v1.2.3",
			Checksum:    "abc",
			DownloadURL: config.RepoPigstyIO + "/src/pigsty-v1.2.3.tgz",
		},
	}

	usePigstyMirrorSource()

	if get.Source != get.ViaCC {
		t.Fatalf("Source = %q, want %q", get.Source, get.ViaCC)
	}
	if get.Region != "china" {
		t.Fatalf("Region = %q, want china", get.Region)
	}
	if got := get.AllVersions[0].DownloadURL; !strings.HasPrefix(got, config.RepoPigstyCC) {
		t.Fatalf("DownloadURL = %q, want prefix %q", got, config.RepoPigstyCC)
	}
	if got := get.AllVersions[0].Checksum; got != "abc" {
		t.Fatalf("Checksum = %q, want abc", got)
	}
}
