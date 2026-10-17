//go:build linux

package ext

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Opt in inside a disposable Debian/Ubuntu image with populated APT indexes.
// Native apt-cache resolves dependencies and apt-get validates candidates with
// --print-uris. This checks resolution without downloading or installing packages.
func TestDownloadDEBWithSystemAPT(t *testing.T) {
	if os.Getenv("PIG_TEST_APT_DEPENDENCIES") != "1" {
		t.Skip("requires PIG_TEST_APT_DEPENDENCIES=1 and system APT indexes")
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "apt-get"), []byte("#!/bin/sh\nexec /usr/bin/apt-get --print-uris \"$@\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PIG_NO_SUDO", "1")
	got, err := DownloadDEB([]string{"postgresql-common"}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) < 2 {
		t.Fatalf("dependencies were not resolved: %v", got)
	}
	for _, candidate := range got {
		if strings.ContainsAny(candidate, "<>") {
			t.Fatalf("virtual dependency passed to apt-get: %q", candidate)
		}
	}
	t.Logf("system APT accepted %d package candidates without virtual names", len(got))
}
