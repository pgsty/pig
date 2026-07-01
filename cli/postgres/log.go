/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

PostgreSQL log operations: list, tail, cat, less, grep
All operations use the default log directory: /pg/log/postgres
*/
package postgres

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
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

var openLogFileForRead = openLogFileWithSudoFallback

const maxCSVLogRecordBytes = 10 * 1024 * 1024

func openLogFileWithSudoFallback(logFile string) (io.ReadCloser, error) {
	f, err := os.Open(logFile)
	if err == nil {
		return f, nil
	}
	if !os.IsPermission(err) {
		return nil, err
	}

	var cmd *exec.Cmd
	if os.Geteuid() == 0 {
		cmd = exec.Command("cat", logFile)
	} else {
		cmd = exec.Command("sudo", "cat", logFile)
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
	if file == "" {
		return "", fmt.Errorf("invalid log file name: empty")
	}
	if file == "." || file == ".." {
		return "", fmt.Errorf("invalid log file name: %s", file)
	}
	if file != filepath.Base(file) || strings.Contains(file, string(os.PathSeparator)) || strings.Contains(file, "\\") {
		return "", fmt.Errorf("invalid log file name: only file basename is allowed")
	}

	cleanDir := filepath.Clean(logDir)
	logPath := filepath.Clean(filepath.Join(cleanDir, file))
	rel, err := filepath.Rel(cleanDir, logPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid log file path")
	}
	return logPath, nil
}

// getLatestLogFile finds the latest CSV log file in the log directory
func getLatestLogFile(logDir string) (string, error) {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		// Permission denied, try sudo ls
		if os.IsPermission(err) {
			return getLatestLogFileWithSudo(logDir)
		}
		return "", fmt.Errorf("cannot read log directory %s: %w", logDir, err)
	}

	var files []os.FileInfo
	for _, entry := range entries {
		if entry.IsDir() {
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
		return files[i].ModTime().After(files[j].ModTime())
	})

	return filepath.Join(logDir, files[0].Name()), nil
}

// getLatestLogFileWithSudo uses sudo/direct ls to find the latest CSV log file
func getLatestLogFileWithSudo(logDir string) (string, error) {
	// ls -t sorts by modification time (newest first), filter *.csv
	var cmd *exec.Cmd
	if os.Geteuid() == 0 {
		cmd = exec.Command("ls", "-t", logDir)
	} else {
		cmd = exec.Command("sudo", "ls", "-t", logDir)
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("cannot read log directory %s: %w", logDir, err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	for _, line := range lines {
		if strings.HasSuffix(line, ".csv") {
			return filepath.Join(logDir, line), nil
		}
	}
	return "", fmt.Errorf("no CSV log files found in %s", logDir)
}

// LogList lists all log files in the log directory
func LogList(logDir string) error {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		// Permission denied, try sudo ls
		if os.IsPermission(err) {
			return logListWithSudo(logDir)
		}
		return fmt.Errorf("cannot read log directory %s: %w", logDir, err)
	}

	var files []os.FileInfo
	var totalSize int64
	for _, entry := range entries {
		if entry.IsDir() {
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

// logListWithSudo uses sudo/direct ls to list log files when permission denied
func logListWithSudo(logDir string) error {
	cmdArgs := []string{"ls", "-lhtr", logDir}
	var cmd *exec.Cmd
	if os.Geteuid() == 0 {
		PrintHint(cmdArgs)
		cmd = exec.Command(cmdArgs[0], cmdArgs[1:]...)
	} else {
		PrintHint(append([]string{"sudo"}, cmdArgs...))
		cmd = exec.Command("sudo", cmdArgs...)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// LogTail tails the latest log file (follow mode)
func LogTail(logDir, file string, lines int) error {
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
		logFile, err = getLatestLogFile(logDir)
		if err != nil {
			return err
		}
	}

	cmdArgs := []string{"tail", "-n", strconv.Itoa(lines), "-f", logFile}
	PrintHint(cmdArgs)
	return RunWithSudoFallback(cmdArgs)
}

// LogCat outputs log file content
func LogCat(logDir, file string, lines int) error {
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
		logFile, err = getLatestLogFile(logDir)
		if err != nil {
			return err
		}
	}

	cmdArgs := []string{"tail", "-n", strconv.Itoa(lines), logFile}
	PrintHint(cmdArgs)
	return RunWithSudoFallback(cmdArgs)
}

// LogShowJSONL outputs the latest PostgreSQL CSV log records as JSONL.
func LogShowJSONL(logDir, file string, lines int) error {
	logFile, err := resolveLogSelection(logDir, file)
	if err != nil {
		return err
	}
	return writeCSVLogJSONL(os.Stdout, logFile, lines)
}

func resolveLogSelection(logDir, file string) (string, error) {
	if file != "" {
		return resolveRequestedLogFile(logDir, file)
	}
	return getLatestLogFile(logDir)
}

func writeCSVLogJSONL(w io.Writer, logFile string, lines int) error {
	if lines <= 0 {
		return fmt.Errorf("lines must be positive")
	}

	f, err := openLogFileForRead(logFile)
	if err != nil {
		return err
	}

	reader := csv.NewReader(newCSVRecordLimitReader(f, maxCSVLogRecordBytes))
	reader.FieldsPerRecord = -1

	var records [][]string
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return closeLogReader(f, fmt.Errorf("parse csv log %s: %w", logFile, err))
		}
		records = append(records, record)
		if len(records) > lines {
			copy(records, records[1:])
			records = records[:lines]
		}
	}

	for _, record := range records {
		row := csvLogRecordToMap(record)
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

func closeLogReader(r io.Closer, err error) error {
	closeErr := r.Close()
	if err != nil {
		return err
	}
	return closeErr
}

type csvRecordLimitReader struct {
	r            io.Reader
	max          int
	current      int
	inQuotes     bool
	quotePending bool
	atFieldStart bool
	err          error
}

func newCSVRecordLimitReader(r io.Reader, max int) io.Reader {
	return &csvRecordLimitReader{r: r, max: max, atFieldStart: true}
}

func (r *csvRecordLimitReader) Read(p []byte) (int, error) {
	if r.err != nil {
		return 0, r.err
	}
	n, err := r.r.Read(p)
	for i := 0; i < n; i++ {
		r.current++
		if r.max > 0 && r.current > r.max {
			r.err = fmt.Errorf("csv log record exceeds %d bytes", r.max)
			if i == 0 {
				return 0, r.err
			}
			return i, nil
		}
		r.observeCSVByte(p[i])
	}
	return n, err
}

func (r *csvRecordLimitReader) observeCSVByte(b byte) {
	for {
		if r.quotePending {
			r.quotePending = false
			if b == '"' {
				r.atFieldStart = false
				return
			}
			r.inQuotes = false
			continue
		}
		if r.inQuotes {
			if b == '"' {
				r.quotePending = true
			}
			return
		}
		switch b {
		case '"':
			if r.atFieldStart {
				r.inQuotes = true
			}
			r.atFieldStart = false
		case ',':
			r.atFieldStart = true
		case '\n', '\r':
			r.current = 0
			r.atFieldStart = true
		default:
			r.atFieldStart = false
		}
		return
	}
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
func LogLess(logDir, file string) error {
	var logFile string
	if file != "" {
		var err error
		logFile, err = resolveRequestedLogFile(logDir, file)
		if err != nil {
			return err
		}
	} else {
		var err error
		logFile, err = getLatestLogFile(logDir)
		if err != nil {
			return err
		}
	}

	cmdArgs := []string{"less", "+G", logFile}
	PrintHint(cmdArgs)
	return RunWithSudoFallback(cmdArgs)
}

// LogGrep searches log files
func LogGrep(logDir, pattern, file string, ignoreCase bool, context int) error {
	var logFile string
	if file != "" {
		var err error
		logFile, err = resolveRequestedLogFile(logDir, file)
		if err != nil {
			return err
		}
	} else {
		var err error
		logFile, err = getLatestLogFile(logDir)
		if err != nil {
			return err
		}
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
	return RunWithSudoFallback(cmdArgs)
}
