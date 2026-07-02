package patroni

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pig/internal/config"
)

func TestStatusTextModeDoesNotUseSudoWhenAlreadyDBSU(t *testing.T) {
	tmp := t.TempDir()
	sudoMarker := filepath.Join(tmp, "sudo-called")
	systemctlArgs := filepath.Join(tmp, "systemctl-args")

	writeTestCommand(t, tmp, "sudo", "printf '%s\\n' \"$@\" > "+shellQuote(sudoMarker)+"\nexit 0\n")
	writeTestCommand(t, tmp, "systemctl", "printf '%s\\n' \"$*\" > "+shellQuote(systemctlArgs)+"\nexit 0\n")
	writeTestCommand(t, tmp, "patronictl", "printf 'cluster ok\\n'\nexit 0\n")

	t.Setenv("PATH", tmp)
	origUser := config.CurrentUser
	origFormat := config.OutputFormat
	t.Cleanup(func() {
		config.CurrentUser = origUser
		config.OutputFormat = origFormat
	})
	config.CurrentUser = "postgres"
	config.OutputFormat = config.OUTPUT_TEXT

	if err := Status("postgres"); err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if _, err := os.Stat(sudoMarker); err == nil {
		t.Fatalf("Status should not call sudo when current user is already DBSU")
	} else if !os.IsNotExist(err) {
		t.Fatalf("checking sudo marker: %v", err)
	}

	raw, err := os.ReadFile(systemctlArgs)
	if err != nil {
		t.Fatalf("systemctl was not called directly: %v", err)
	}
	if got := strings.TrimSpace(string(raw)); got != "status patroni --no-pager -l" {
		t.Fatalf("systemctl args = %q, want status patroni --no-pager -l", got)
	}
}

func writeTestCommand(t *testing.T, dir, name, body string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0755); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
