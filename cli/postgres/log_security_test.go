package postgres

import (
	"errors"
	"os"
	"path/filepath"
	"pig/internal/utils"
	"testing"
)

func TestResolveRequestedLogFileValid(t *testing.T) {
	got, err := resolveRequestedLogFile("/pg/log/postgres", "postgresql-2026-02-11.csv")
	if err != nil {
		t.Fatalf("resolveRequestedLogFile returned error: %v", err)
	}
	want := "/pg/log/postgres/postgresql-2026-02-11.csv"
	if got != want {
		t.Fatalf("resolveRequestedLogFile returned %q, want %q", got, want)
	}
}

func TestResolveRequestedLogFileValidRootDir(t *testing.T) {
	got, err := resolveRequestedLogFile("/", "passwd")
	if err != nil {
		t.Fatalf("resolveRequestedLogFile returned error: %v", err)
	}
	if got != "/passwd" {
		t.Fatalf("resolveRequestedLogFile returned %q, want %q", got, "/passwd")
	}
}

func TestResolveRequestedLogFileRejectsTraversal(t *testing.T) {
	tests := []string{
		"../../../etc/hosts",
		"/etc/hosts",
		"subdir/postgresql.csv",
		`subdir\postgresql.csv`,
		"..",
		".",
		"",
	}
	for _, input := range tests {
		if _, err := resolveRequestedLogFile("/pg/log/postgres", input); err == nil {
			t.Fatalf("expected error for input %q", input)
		}
	}
}

func TestLogCatRejectsTraversalPath(t *testing.T) {
	if err := LogCat("/pg/log/postgres", "../../../etc/hosts", 1); err == nil {
		t.Fatalf("expected LogCat to reject traversal path")
	}
}

func TestLogGrepNoMatchReturnsSilentExitCode(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "postgresql-2026-07-02.csv")
	if err := os.WriteFile(logPath, []byte("LOG,startup complete\n"), 0644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	err := LogGrep(dir, "ERROR", "", false, 0)
	var exitErr *utils.ExitCodeError
	if !errors.As(err, &exitErr) {
		t.Fatalf("LogGrep returned %T, want ExitCodeError", err)
	}
	if exitErr.Code != 1 || !exitErr.Silent {
		t.Fatalf("LogGrep no-match exit = code %d silent %v, want code 1 silent true", exitErr.Code, exitErr.Silent)
	}
}
