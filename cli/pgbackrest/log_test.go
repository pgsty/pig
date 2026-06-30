package pgbackrest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResolveRequestedLogFileRejectsTraversal(t *testing.T) {
	tests := []string{
		"../pg-meta-backup.log",
		"/pg/log/pgbackrest/pg-meta-backup.log",
		"subdir/pg-meta-backup.log",
		`subdir\pg-meta-backup.log`,
		"..",
		".",
		"",
	}

	for _, input := range tests {
		if _, err := resolveRequestedLogFile("/pg/log/pgbackrest", input); err == nil {
			t.Fatalf("expected error for input %q", input)
		}
	}
}

func TestResolveRequestedLogFileRequiresLogSuffix(t *testing.T) {
	_, err := resolveRequestedLogFile("/pg/log/pgbackrest", "pg-meta-backup.txt")
	if err == nil || !strings.Contains(err.Error(), "must end with .log") {
		t.Fatalf("expected .log suffix error, got %v", err)
	}
}

func TestResolveRequestedLogFileValid(t *testing.T) {
	got, err := resolveRequestedLogFile("/pg/log/pgbackrest", "pg-meta-backup.log")
	if err != nil {
		t.Fatalf("resolveRequestedLogFile returned error: %v", err)
	}
	want := "/pg/log/pgbackrest/pg-meta-backup.log"
	if got != want {
		t.Fatalf("resolveRequestedLogFile returned %q, want %q", got, want)
	}
}

func TestLogDirUsesConfigLogPath(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "pgbackrest.conf")
	if err := os.WriteFile(configPath, []byte("[global]\nlog-path = /custom/pgbackrest/log\n"), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if got := LogDir(configPath, ""); got != "/custom/pgbackrest/log" {
		t.Fatalf("LogDir() = %q, want custom log-path", got)
	}
}

func TestLogDirFallsBackToDefaultWhenConfigMissing(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "missing.conf")

	if got := LogDir(configPath, ""); got != DefaultLogDir {
		t.Fatalf("LogDir() = %q, want default %q", got, DefaultLogDir)
	}
}

func TestFindLatestLogUsesModificationTime(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "zz-old.log")
	newPath := filepath.Join(dir, "aa-new.log")
	if err := os.WriteFile(oldPath, []byte("old\n"), 0644); err != nil {
		t.Fatalf("write old log: %v", err)
	}
	if err := os.WriteFile(newPath, []byte("new\n"), 0644); err != nil {
		t.Fatalf("write new log: %v", err)
	}
	oldTime := time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC)
	newTime := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatalf("chtimes old log: %v", err)
	}
	if err := os.Chtimes(newPath, newTime, newTime); err != nil {
		t.Fatalf("chtimes new log: %v", err)
	}

	got, err := findLatestLog(dir, "")
	if err != nil {
		t.Fatalf("findLatestLog returned error: %v", err)
	}
	if got != "aa-new.log" {
		t.Fatalf("findLatestLog = %q, want newest by mtime", got)
	}
}

func TestResolveLogDirReportsExplicitConfigReadError(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "missing.conf")

	_, err := ResolveLogDir(configPath, "", true)
	if err == nil || !strings.Contains(err.Error(), "cannot read pgBackRest config") {
		t.Fatalf("expected explicit config read error, got %v", err)
	}
}
