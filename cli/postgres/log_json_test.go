package postgres

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLogShowJSONLParsesCSVLog(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "postgresql-2026-06-30.csv")
	writeCSVLogRows(t, path, [][]string{
		{
			"2026-06-30 10:00:00.000 CST", "alice", "appdb", "12345", "127.0.0.1:54321",
			"session-1", "1", "SELECT", "2026-06-30 09:59:59 CST", "3/14", "99",
			"ERROR", "23505", "duplicate key value violates unique constraint", "Key (id)=(1) already exists.",
			"", "", "", "", "insert into t values (1)", "12", "nbtinsert.c:666", "psql",
			"client backend", "", "0",
		},
	})

	var out bytes.Buffer
	withStdout(t, &out, func() {
		if err := LogShowJSONL(dir, "", 1); err != nil {
			t.Fatalf("LogShowJSONL returned error: %v", err)
		}
	})

	lines := bytes.Split(bytes.TrimSpace(out.Bytes()), []byte("\n"))
	if len(lines) != 1 {
		t.Fatalf("expected one JSONL row, got %d: %q", len(lines), out.String())
	}

	var row map[string]interface{}
	if err := json.Unmarshal(lines[0], &row); err != nil {
		t.Fatalf("invalid json row: %v, row=%q", err, string(lines[0]))
	}
	if row["message"] != "duplicate key value violates unique constraint" {
		t.Fatalf("message = %v", row["message"])
	}
	if row["error_severity"] != "ERROR" {
		t.Fatalf("error_severity = %v", row["error_severity"])
	}
	if row["query"] != "insert into t values (1)" {
		t.Fatalf("query = %v", row["query"])
	}
	if _, ok := row["captured_output"]; ok {
		t.Fatalf("JSONL log output should not contain command wrapper fields: %v", row)
	}
}

func TestLogShowJSONLReadsThroughFallbackReader(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "postgresql-2026-06-30.csv")
	writeCSVLogRows(t, path, [][]string{
		{
			"2026-06-30 10:00:00.000 CST", "alice", "appdb", "12345", "127.0.0.1:54321",
			"session-1", "1", "SELECT", "2026-06-30 09:59:59 CST", "3/14", "99",
			"ERROR", "23505", "direct file should not be used", "", "", "", "", "", "",
			"select 1", "", "", "psql", "client backend", "", "0",
		},
	})

	origOpen := openLogFileForRead
	defer func() { openLogFileForRead = origOpen }()
	openLogFileForRead = func(logFile string) (io.ReadCloser, error) {
		if logFile != path {
			t.Fatalf("openLogFileForRead got %q, want %q", logFile, path)
		}
		return io.NopCloser(bytes.NewBufferString(
			`2026-06-30 10:00:01.000 CST,bob,appdb,12345,127.0.0.1:54321,session-2,2,SELECT,2026-06-30 09:59:59 CST,3/14,99,LOG,00000,"fallback reader was used",,,,,select 2,,,psql,client backend,,0` + "\n",
		)), nil
	}

	var out bytes.Buffer
	withStdout(t, &out, func() {
		if err := LogShowJSONL(dir, "", 1); err != nil {
			t.Fatalf("LogShowJSONL returned error: %v", err)
		}
	})

	var row map[string]interface{}
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &row); err != nil {
		t.Fatalf("invalid json row: %v, row=%q", err, out.String())
	}
	if row["message"] != "fallback reader was used" {
		t.Fatalf("message = %v, want fallback reader output", row["message"])
	}
}

func TestWriteCSVLogJSONLRejectsNonPositiveLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "postgresql-2026-06-30.csv")
	writeCSVLogRows(t, path, [][]string{minimalCSVLogRow("message")})

	var out bytes.Buffer
	err := writeCSVLogJSONL(&out, path, 0)
	if err == nil || !strings.Contains(err.Error(), "lines must be positive") {
		t.Fatalf("expected positive line count error, got %v", err)
	}
}

func minimalCSVLogRow(message string) []string {
	return []string{
		"2026-06-30 10:00:00.000 CST", "alice", "appdb", "12345", "127.0.0.1:54321",
		"session-1", "1", "SELECT", "2026-06-30 09:59:59 CST", "3/14", "99",
		"LOG", "00000", message, "", "", "", "", "", "select 1", "", "", "psql",
		"client backend", "", "0",
	}
}

func writeCSVLogRows(t *testing.T, path string, rows [][]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create csv log: %v", err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	for _, row := range rows {
		if err := w.Write(row); err != nil {
			t.Fatalf("write csv row: %v", err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		t.Fatalf("flush csv log: %v", err)
	}
}

func withStdout(t *testing.T, w io.Writer, fn func()) {
	t.Helper()
	old := os.Stdout
	r, pipeW, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = pipeW
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(w, r)
		close(done)
	}()

	fn()

	_ = pipeW.Close()
	os.Stdout = old
	<-done
	_ = r.Close()
}
