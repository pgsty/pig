/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

PostgreSQL log operations: list, tail, cat, less, grep
All operations use the default log directory: /pg/log/postgres
*/
package postgres

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"pig/internal/utils"
)

var csvLogFieldNames = []string{
	"log_time",
	"user_name",
	"database_name",
	"process_id",
	"connection_from",
	"session_id",
	"session_line_num",
	"command_tag",
	"session_start_time",
	"virtual_transaction_id",
	"transaction_id",
	"error_severity",
	"sql_state_code",
	"message",
	"detail",
	"hint",
	"internal_query",
	"internal_query_pos",
	"context",
	"query",
	"query_pos",
	"location",
	"application_name",
	"backend_type",
	"leader_pid",
	"query_id",
}

var (
	openLogFileForRead         = openLogFileAsDBSU
	postgresLogEUID            = os.Geteuid
	postgresLogDBSUCommand     = utils.DBSUCommand
	postgresLogDBSUPassthrough = utils.DBSUCommandPassthrough
)

const maxCSVLogRecordBytes = 10 * 1024 * 1024

func openLogFileAsDBSU(logFile, dbsu string) (io.ReadCloser, error) {
	if postgresLogEUID() != 0 {
		f, err := os.Open(logFile)
		if err == nil {
			return f, nil
		}
		if !os.IsPermission(err) {
			return nil, err
		}
	}
	cmd, err := utils.BuildDBSUCommand(dbsu, []string{"cat", "--", logFile})
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	stdout, pipeErr := cmd.StdoutPipe()
	if pipeErr != nil {
		return nil, fmt.Errorf("cannot read log file %s: %w", logFile, pipeErr)
	}
	if fallbackErr := cmd.Start(); fallbackErr != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return nil, fmt.Errorf("cannot read log file %s: %w: %s", logFile, fallbackErr, detail)
		}
		return nil, fmt.Errorf("cannot read log file %s: %w", logFile, fallbackErr)
	}
	return &commandReadCloser{ReadCloser: stdout, cmd: cmd, stderr: &stderr, source: logFile}, nil
}

type commandReadCloser struct {
	io.ReadCloser
	cmd    *exec.Cmd
	stderr *bytes.Buffer
	source string
}

func (r *commandReadCloser) Close() error {
	closeErr := r.ReadCloser.Close()
	waitErr := r.cmd.Wait()
	if waitErr != nil {
		detail := strings.TrimSpace(r.stderr.String())
		if detail != "" {
			return fmt.Errorf("cannot read log file %s: %w: %s", r.source, waitErr, detail)
		}
		return fmt.Errorf("cannot read log file %s: %w", r.source, waitErr)
	}
	return closeErr
}

func resolveRequestedLogFile(logDir string, file string) (string, error) {
	return utils.ResolveLogFilePath(logDir, file)
}

// getLatestLogFile finds the latest CSV log file in the log directory
func getLatestLogFile(logDir string, dbsuOverride ...string) (string, error) {
	dbsu := logDBSU(dbsuOverride)
	entries, err := os.ReadDir(logDir)
	if err != nil {
		if os.IsPermission(err) {
			return getLatestLogFileAsDBSU(logDir, dbsu)
		}
		return "", fmt.Errorf("cannot read log directory %s: %w", logDir, err)
	}

	var files []os.FileInfo
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		if matched, _ := filepath.Match("*.csv", entry.Name()); matched {
			if info, err := entry.Info(); err == nil {
				files = append(files, info)
			}
		}
	}

	if len(files) == 0 {
		return "", fmt.Errorf("no CSV log files found in %s", logDir)
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].ModTime().Equal(files[j].ModTime()) {
			return files[i].Name() > files[j].Name()
		}
		return files[i].ModTime().After(files[j].ModTime())
	})

	return filepath.Join(logDir, files[0].Name()), nil
}

// getLatestLogFileAsDBSU uses the database OS user and rejects link entries.
func getLatestLogFileAsDBSU(logDir, dbsu string) (string, error) {
	output, err := utils.DBSUCommandStdout(dbsu, []string{"ls", "-1t", "--", logDir})
	if err != nil {
		return "", fmt.Errorf("cannot read log directory %s: %w", logDir, err)
	}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		if !strings.HasSuffix(line, ".csv") {
			continue
		}
		path, pathErr := resolveRequestedLogFile(logDir, line)
		if pathErr == nil && ensureRegularLogFile(path, dbsu) == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("no CSV log files found in %s", logDir)
}

// LogList lists all log files in the log directory
func LogList(logDir string, dbsuOverride ...string) error {
	dbsu := logDBSU(dbsuOverride)
	entries, err := os.ReadDir(logDir)
	if err != nil {
		if os.IsPermission(err) {
			return logListAsDBSU(logDir, dbsu)
		}
		return fmt.Errorf("cannot read log directory %s: %w", logDir, err)
	}

	var files []os.FileInfo
	var totalSize int64
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		if info, err := entry.Info(); err == nil {
			files = append(files, info)
			totalSize += info.Size()
		}
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].ModTime().After(files[j].ModTime())
	})

	fmt.Printf("%s%-40s %10s  %s%s\n", utils.ColorBold, "NAME", "SIZE", "MODIFIED", utils.ColorReset)
	for _, f := range files {
		fmt.Printf("%-40s %10s  %s\n", f.Name(), FormatSize(f.Size()), f.ModTime().Format("2006-01-02 15:04:05"))
	}
	fmt.Printf("\n%sTotal: %d files, %s%s\n", utils.ColorCyan, len(files), FormatSize(totalSize), utils.ColorReset)
	return nil
}

func logListAsDBSU(logDir, dbsu string) error {
	args := []string{
		"sh", "-c",
		`for path in "$1"/*; do test -f "$path" && test ! -L "$path" && printf '%s\n' "${path##*/}"; done; true`,
		"sh", logDir,
	}
	PrintHint(args)
	return utils.DBSUCommand(dbsu, args)
}

// LogTail tails the latest log file (follow mode)
func LogTail(logDir, file string, lines int, dbsuOverride ...string) error {
	dbsu := logDBSU(dbsuOverride)
	if lines <= 0 {
		return fmt.Errorf("lines must be positive")
	}
	var logFile string
	if file != "" {
		var err error
		logFile, err = resolveRequestedLogFile(logDir, file)
		if err != nil {
			return err
		}
	} else {
		var err error
		logFile, err = getLatestLogFile(logDir, dbsu)
		if err != nil {
			return err
		}
	}
	if err := ensureRegularLogFile(logFile, dbsu); err != nil {
		return err
	}

	cmdArgs := []string{"tail", "-n", strconv.Itoa(lines), "-f", logFile}
	PrintHint(cmdArgs)
	return runPostgresLogCommand(dbsu, cmdArgs, true)
}

// LogCat outputs log file content
func LogCat(logDir, file string, lines int, dbsuOverride ...string) error {
	dbsu := logDBSU(dbsuOverride)
	if lines <= 0 {
		return fmt.Errorf("lines must be positive")
	}
	var logFile string
	if file != "" {
		var err error
		logFile, err = resolveRequestedLogFile(logDir, file)
		if err != nil {
			return err
		}
	} else {
		var err error
		logFile, err = getLatestLogFile(logDir, dbsu)
		if err != nil {
			return err
		}
	}
	if err := ensureRegularLogFile(logFile, dbsu); err != nil {
		return err
	}

	cmdArgs := []string{"tail", "-n", strconv.Itoa(lines), logFile}
	PrintHint(cmdArgs)
	return runPostgresLogCommand(dbsu, cmdArgs, false)
}

// LogShowJSONL outputs the latest PostgreSQL CSV log records as JSONL.
func LogShowJSONL(logDir, file string, lines int, dbsuOverride ...string) error {
	dbsu := logDBSU(dbsuOverride)
	logFile, err := resolveLogSelection(logDir, file, dbsu)
	if err != nil {
		return err
	}
	return writeCSVLogJSONL(os.Stdout, logFile, lines, dbsu)
}

func resolveLogSelection(logDir, file, dbsu string) (string, error) {
	var path string
	var err error
	if file != "" {
		path, err = resolveRequestedLogFile(logDir, file)
	} else {
		path, err = getLatestLogFile(logDir, dbsu)
	}
	if err != nil {
		return "", err
	}
	if err := ensureRegularLogFile(path, dbsu); err != nil {
		return "", err
	}
	return path, nil
}

func writeCSVLogJSONL(w io.Writer, logFile string, lines int, dbsuOverride ...string) error {
	if lines <= 0 {
		return fmt.Errorf("lines must be positive")
	}

	f, err := openLogFileForRead(logFile, logDBSU(dbsuOverride))
	if err != nil {
		return err
	}

	entries, err := readCSVLogEntries(f, lines)
	if err != nil {
		return closeLogReader(f, err)
	}

	for _, entry := range entries {
		row := entry.toMap()
		data, err := json.Marshal(row)
		if err != nil {
			return closeLogReader(f, err)
		}
		if _, err := fmt.Fprintln(w, string(data)); err != nil {
			return closeLogReader(f, err)
		}
	}
	return closeLogReader(f, nil)
}

type csvLogEntry struct {
	record    []string
	malformed *malformedCSVLogEntry
}

type malformedCSVLogEntry struct {
	raw      string
	parseErr string
}

func (e csvLogEntry) toMap() interface{} {
	if e.malformed != nil {
		return map[string]interface{}{
			"component":   "postgres",
			"malformed":   true,
			"raw":         e.malformed.raw,
			"parse_error": e.malformed.parseErr,
		}
	}
	return csvLogRecordToMap(e.record)
}

func readCSVLogEntries(r io.Reader, lines int) ([]csvLogEntry, error) {
	reader := bufio.NewReader(r)
	var entries []csvLogEntry
	pending := ""
	for {
		raw, readErr := reader.ReadString('\n')
		if len(raw) > 0 {
			raw = strings.TrimRight(raw, "\r\n")
			if raw != "" || pending != "" {
				pending = appendCSVPhysicalLine(pending, raw)
				entry, complete, err := consumeCSVLogRecord(pending, readErr == io.EOF)
				if err != nil {
					return nil, err
				}
				if complete {
					entries = appendRecentCSVLogEntry(entries, entry, lines)
					pending = ""
				}
			}
		}
		if readErr == io.EOF {
			if pending != "" {
				entry, _, err := consumeCSVLogRecord(pending, true)
				if err != nil {
					return nil, err
				}
				entries = appendRecentCSVLogEntry(entries, entry, lines)
			}
			break
		}
		if readErr != nil {
			return nil, readErr
		}
	}
	return entries, nil
}

func appendCSVPhysicalLine(pending, line string) string {
	if pending == "" {
		return line
	}
	return pending + "\n" + line
}

func consumeCSVLogRecord(raw string, eof bool) (csvLogEntry, bool, error) {
	entry, err := parseCSVLogEntry(raw)
	if err == nil {
		return entry, true, nil
	}
	if strings.Contains(err.Error(), "exceeds") {
		return csvLogEntry{}, false, err
	}
	if isContinuableCSVParseError(err) && !eof {
		return csvLogEntry{}, false, nil
	}
	return csvLogEntry{malformed: &malformedCSVLogEntry{raw: raw, parseErr: err.Error()}}, true, nil
}

func appendRecentCSVLogEntry(entries []csvLogEntry, entry csvLogEntry, limit int) []csvLogEntry {
	entries = append(entries, entry)
	if len(entries) <= limit {
		return entries
	}
	copy(entries, entries[1:])
	return entries[:limit]
}

func isContinuableCSVParseError(err error) bool {
	var parseErr *csv.ParseError
	return errors.As(err, &parseErr) && parseErr.Err == csv.ErrQuote
}

func parseCSVLogEntry(raw string) (csvLogEntry, error) {
	if len(raw) > maxCSVLogRecordBytes {
		return csvLogEntry{}, fmt.Errorf("csv log record exceeds %d bytes", maxCSVLogRecordBytes)
	}
	reader := csv.NewReader(strings.NewReader(raw))
	reader.FieldsPerRecord = -1
	record, err := reader.Read()
	if err != nil {
		return csvLogEntry{}, err
	}
	return csvLogEntry{record: record}, nil
}

func closeLogReader(r io.Closer, err error) error {
	closeErr := r.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func csvLogRecordToMap(record []string) map[string]string {
	row := map[string]string{"component": "postgres"}
	for i, value := range record {
		if i < len(csvLogFieldNames) {
			row[csvLogFieldNames[i]] = value
			continue
		}
		row[fmt.Sprintf("field_%d", i+1)] = value
	}
	return row
}

// LogLess opens the latest log file in less
func LogLess(logDir, file string, dbsuOverride ...string) error {
	dbsu := logDBSU(dbsuOverride)
	var logFile string
	if file != "" {
		var err error
		logFile, err = resolveRequestedLogFile(logDir, file)
		if err != nil {
			return err
		}
	} else {
		var err error
		logFile, err = getLatestLogFile(logDir, dbsu)
		if err != nil {
			return err
		}
	}
	if err := ensureRegularLogFile(logFile, dbsu); err != nil {
		return err
	}

	cmdArgs := []string{"less", "+G", logFile}
	PrintHint(cmdArgs)
	return runPostgresLogCommand(dbsu, cmdArgs, true)
}

// LogGrep searches log files
func LogGrep(logDir, pattern, file string, ignoreCase bool, context int, dbsuOverride ...string) error {
	dbsu := logDBSU(dbsuOverride)
	var logFile string
	if file != "" {
		var err error
		logFile, err = resolveRequestedLogFile(logDir, file)
		if err != nil {
			return err
		}
	} else {
		var err error
		logFile, err = getLatestLogFile(logDir, dbsu)
		if err != nil {
			return err
		}
	}
	if err := ensureRegularLogFile(logFile, dbsu); err != nil {
		return err
	}

	cmdArgs := []string{"grep", "--color=auto"}
	if ignoreCase {
		cmdArgs = append(cmdArgs, "-i")
	}
	if context > 0 {
		cmdArgs = append(cmdArgs, "-C", strconv.Itoa(context))
	}
	cmdArgs = append(cmdArgs, pattern, logFile)
	PrintHint(cmdArgs)
	err := runPostgresLogCommand(dbsu, cmdArgs, false)
	if isGrepNoMatch(err) {
		return &utils.ExitCodeError{Code: 1, Err: err, Silent: true}
	}
	return err
}

func logDBSU(overrides []string) string {
	if len(overrides) > 0 && strings.TrimSpace(overrides[0]) != "" {
		return utils.GetDBSU(overrides[0])
	}
	return utils.GetDBSU("")
}

func ensureRegularLogFile(path, dbsu string) error {
	if err := utils.RequireRegularFileAsDBSU(path, dbsu); err != nil {
		return fmt.Errorf("invalid log file: %w", err)
	}
	return nil
}

func runPostgresLogCommand(dbsu string, args []string, passthrough bool) error {
	runAsDBSU := func() error {
		if passthrough {
			return postgresLogDBSUPassthrough(dbsu, args)
		}
		return postgresLogDBSUCommand(dbsu, args)
	}
	if postgresLogEUID() == 0 {
		return runAsDBSU()
	}

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err == nil {
		return nil
	} else if !isPermissionDiagnostic(stderr.String()) {
		if stderr.Len() > 0 {
			fmt.Fprint(os.Stderr, stderr.String())
		}
		return err
	}
	return runAsDBSU()
}

func isPermissionDiagnostic(value string) bool {
	value = strings.ToLower(value)
	return strings.Contains(value, "permission denied") || strings.Contains(value, "operation not permitted")
}

func isGrepNoMatch(err error) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr) && exitErr.ExitCode() == 1
}
