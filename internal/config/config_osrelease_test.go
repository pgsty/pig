package config

import (
	"strings"
	"testing"
)

func TestParseOSRelease(t *testing.T) {
	content := `
# comment
ID="ubuntu"
VERSION_ID="22.04"
VERSION_CODENAME=jammy
`

	info := parseOSRelease(strings.NewReader(content))
	if info.ID != "ubuntu" {
		t.Fatalf("ID=%q, want %q", info.ID, "ubuntu")
	}
	if info.VersionID != "22.04" {
		t.Fatalf("VersionID=%q, want %q", info.VersionID, "22.04")
	}
	if info.VersionCodename != "jammy" {
		t.Fatalf("VersionCodename=%q, want %q", info.VersionCodename, "jammy")
	}
}

func TestParseOSRelease_EmptyAndUnknownKeys(t *testing.T) {
	content := `
SOMETHING=else
ID=debian

# another comment
VERSION_ID=12
`

	info := parseOSRelease(strings.NewReader(content))
	if info.ID != "debian" {
		t.Fatalf("ID=%q, want %q", info.ID, "debian")
	}
	if info.VersionID != "12" {
		t.Fatalf("VersionID=%q, want %q", info.VersionID, "12")
	}
	if info.VersionCodename != "" {
		t.Fatalf("VersionCodename=%q, want empty", info.VersionCodename)
	}
}
