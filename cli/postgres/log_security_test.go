package postgres

import (
	"errors"
	"os"
	"path/filepath"
	"pig/internal/utils"
	"strings"
	"testing"
	"time"
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

func TestLogCatRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(target, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "latest.csv")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := LogCat(dir, "latest.csv", 1); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("LogCat symlink error = %v", err)
	}
}

func TestLatestLogSkipsSymlink(t *testing.T) {
	dir := t.TempDir()
	realLog := filepath.Join(dir, "postgresql-real.csv")
	if err := os.WriteFile(realLog, []byte("ok"), 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "postgresql-latest.csv")
	if err := os.Symlink(realLog, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := os.Chtimes(link, time.Now().Add(time.Hour), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, err := getLatestLogFile(dir)
	if err != nil || got != realLog {
		t.Fatalf("latest log = %q, err=%v; want %q", got, err, realLog)
	}
}

func TestRootLogCommandDropsToDBSU(t *testing.T) {
	origEUID := postgresLogEUID
	origRun := postgresLogDBSUCommand
	origPassthrough := postgresLogDBSUPassthrough
	t.Cleanup(func() {
		postgresLogEUID = origEUID
		postgresLogDBSUCommand = origRun
		postgresLogDBSUPassthrough = origPassthrough
	})
	postgresLogEUID = func() int { return 0 }
	called := false
	postgresLogDBSUCommand = func(dbsu string, args []string) error {
		called = dbsu == "postgres" && len(args) > 0 && args[0] == "tail"
		return nil
	}
	if err := runPostgresLogCommand("postgres", []string{"tail", "/pg/log/postgres/log.csv"}, false); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("root log command did not execute as DBSU")
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
