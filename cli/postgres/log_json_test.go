package postgres

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
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

func TestGetLatestLogFileTiesByName(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "postgresql-aa.csv")
	second := filepath.Join(dir, "postgresql-zz.csv")
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, []byte("row\n"), 0644); err != nil {
			t.Fatalf("write log %s: %v", path, err)
		}
	}
	sameTime := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	for _, path := range []string{first, second} {
		if err := os.Chtimes(path, sameTime, sameTime); err != nil {
			t.Fatalf("chtimes log %s: %v", path, err)
		}
	}

	got, err := getLatestLogFile(dir)
	if err != nil {
		t.Fatalf("getLatestLogFile returned error: %v", err)
	}
	if got != second {
		t.Fatalf("getLatestLogFile = %q, want deterministic name tie-break %q", got, second)
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

func TestWriteCSVLogJSONLDegradesMalformedRows(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "postgresql-2026-07-02.csv")
	rawLine := `2026-07-02 12:00:00.000 CST,alice,appdb,12345,"unterminated`
	if err := os.WriteFile(path, []byte(rawLine+"\n"), 0644); err != nil {
		t.Fatalf("write malformed csv log: %v", err)
	}

	var out bytes.Buffer
	if err := writeCSVLogJSONL(&out, path, 1); err != nil {
		t.Fatalf("writeCSVLogJSONL should degrade malformed row instead of failing: %v", err)
	}

	var row map[string]interface{}
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &row); err != nil {
		t.Fatalf("invalid json row: %v, row=%q", err, out.String())
	}
	if malformed, _ := row["malformed"].(bool); !malformed {
		t.Fatalf("expected malformed=true row, got %v", row)
	}
	if raw, _ := row["raw"].(string); !strings.Contains(raw, "unterminated") {
		t.Fatalf("malformed row should preserve raw content, got %v", row)
	}
	if parseErr, _ := row["parse_error"].(string); parseErr == "" {
		t.Fatalf("malformed row should include parse_error, got %v", row)
	}
}

func TestWriteCSVLogJSONLPreservesMultilineCSVRecord(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "postgresql-2026-07-02.csv")
	writeCSVLogRows(t, path, [][]string{minimalCSVLogRow("first line\nsecond line")})

	var out bytes.Buffer
	if err := writeCSVLogJSONL(&out, path, 1); err != nil {
		t.Fatalf("writeCSVLogJSONL returned error for multiline csv record: %v", err)
	}

	lines := bytes.Split(bytes.TrimSpace(out.Bytes()), []byte("\n"))
	if len(lines) != 1 {
		t.Fatalf("expected one JSONL row for one multiline CSV record, got %d: %q", len(lines), out.String())
	}
	var row map[string]interface{}
	if err := json.Unmarshal(lines[0], &row); err != nil {
		t.Fatalf("invalid json row: %v, row=%q", err, lines[0])
	}
	if row["message"] != "first line\nsecond line" {
		t.Fatalf("message = %v, want multiline message", row["message"])
	}
	if malformed, _ := row["malformed"].(bool); malformed {
		t.Fatalf("valid multiline CSV record should not be marked malformed: %v", row)
	}
}

func TestOpenLogFileFallbackStreamsWithoutBufferingCommandOutput(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permission fallback requires non-root unix user")
	}

	dir := t.TempDir()
	logPath := filepath.Join(dir, "postgresql-2026-06-30.csv")
	if err := os.WriteFile(logPath, []byte("direct file should be unreadable\n"), 0600); err != nil {
		t.Fatalf("write log file: %v", err)
	}
	if err := os.Chmod(logPath, 0000); err != nil {
		t.Fatalf("chmod log file: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(logPath, 0600) })

	fakeBin := filepath.Join(dir, "bin")
	if err := os.Mkdir(fakeBin, 0755); err != nil {
		t.Fatalf("mkdir fake bin: %v", err)
	}
	fakeSudo := filepath.Join(fakeBin, "sudo")
	if err := os.WriteFile(fakeSudo, []byte("#!/bin/sh\nprintf 'streamed fallback row\\n'\nsleep 0.3\n"), 0755); err != nil {
		t.Fatalf("write fake sudo: %v", err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	start := time.Now()
	rc, err := openLogFileWithSudoFallback(logPath)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("openLogFileWithSudoFallback returned error: %v", err)
	}
	defer rc.Close()

	if elapsed > 100*time.Millisecond {
		t.Fatalf("fallback open took %s; expected streaming reader to return before command exits", elapsed)
	}
}

func TestWriteCSVLogJSONLRejectsOversizedCSVRecord(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "postgresql-2026-06-30.csv")
	writeCSVLogRows(t, path, [][]string{minimalCSVLogRow(strings.Repeat("x", 10*1024*1024+1))})

	var out bytes.Buffer
	err := writeCSVLogJSONL(&out, path, 1)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected oversized CSV record error, got %v", err)
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
