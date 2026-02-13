package postgres

import "testing"

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
