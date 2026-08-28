package utils

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveLogFilePath(t *testing.T) {
	tests := []struct {
		name   string
		logDir string
		file   string
		want   string
		errMsg string
	}{
		{name: "basename", logDir: "/pg/log/postgres", file: "postgresql.csv", want: "/pg/log/postgres/postgresql.csv"},
		{name: "root directory", logDir: "/", file: "postgresql.csv", want: "/postgresql.csv"},
		{name: "empty", logDir: "/pg/log", file: "", errMsg: "invalid log file name: empty"},
		{name: "dot", logDir: "/pg/log", file: ".", errMsg: "invalid log file name: ."},
		{name: "dot dot", logDir: "/pg/log", file: "..", errMsg: "invalid log file name: .."},
		{name: "relative traversal", logDir: "/pg/log", file: "../postgresql.csv", errMsg: "invalid log file name: only file basename is allowed"},
		{name: "absolute", logDir: "/pg/log", file: "/etc/passwd", errMsg: "invalid log file name: only file basename is allowed"},
		{name: "subdirectory", logDir: "/pg/log", file: "archive/postgresql.csv", errMsg: "invalid log file name: only file basename is allowed"},
		{name: "windows separator", logDir: "/pg/log", file: `archive\postgresql.csv`, errMsg: "invalid log file name: only file basename is allowed"},
		{name: "newline", logDir: "/pg/log", file: "postgresql.csv\nother", errMsg: "invalid log file name: control characters are not allowed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveLogFilePath(tt.logDir, tt.file)
			if tt.errMsg != "" {
				if err == nil {
					t.Fatalf("ResolveLogFilePath(%q, %q) = %q, want error", tt.logDir, tt.file, got)
				}
				if err.Error() != tt.errMsg {
					t.Fatalf("ResolveLogFilePath(%q, %q) error = %q, want %q", tt.logDir, tt.file, err, tt.errMsg)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveLogFilePath(%q, %q) error = %v", tt.logDir, tt.file, err)
			}
			if got != tt.want {
				t.Fatalf("ResolveLogFilePath(%q, %q) = %q, want %q", tt.logDir, tt.file, got, tt.want)
			}
		})
	}
}

func TestRequireRegularFileAsDBSURejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.log")
	if err := os.WriteFile(target, []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := RequireRegularFileAsDBSU(target, "postgres"); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.log")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := RequireRegularFileAsDBSU(link, "postgres"); err == nil {
		t.Fatal("symlink was accepted as a regular file")
	}
}
