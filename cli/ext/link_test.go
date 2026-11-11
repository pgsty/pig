package ext

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePGHomePolarDBAliases(t *testing.T) {
	for _, alias := range []string{"polar", "polardb", "polardb-17"} {
		t.Run(alias, func(t *testing.T) {
			got, err := resolvePGHome(alias)
			if err != nil {
				t.Fatalf("resolvePGHome(%q) returned error: %v", alias, err)
			}
			if got != "/usr/polar-17" {
				t.Fatalf("resolvePGHome(%q) = %q, want %q", alias, got, "/usr/polar-17")
			}
		})
	}
}

func TestValidatePGHomeRejectsTrailingSlashSystemPathAndShellMetachars(t *testing.T) {
	for _, sysPath := range []string{"/", "/usr/", "/usr/local/"} {
		resolved, err := resolvePGHome(sysPath)
		if err != nil {
			t.Fatalf("resolvePGHome(%q) unexpected error: %v", sysPath, err)
		}
		if err := validatePGHome(resolved); err == nil {
			t.Fatalf("validatePGHome(%q -> %q) succeeded, want rejection", sysPath, resolved)
		}
	}

	unsafeDir := filepath.Join(t.TempDir(), "pg$HOME")
	if err := os.MkdirAll(filepath.Join(unsafeDir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := validatePGHome(unsafeDir); err == nil {
		t.Fatalf("validatePGHome(%q) succeeded, want rejection for shell metacharacters", unsafeDir)
	}
}
