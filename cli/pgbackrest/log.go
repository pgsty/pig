package pgbackrest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"pig/cli/postgres"
)

// LogDir returns the pgbackrest log directory.
// Hardcoded to Pigsty default: /pg/log/pgbackrest
func LogDir() string {
	return DefaultLogDir
}

// LogList lists pgbackrest log files in the log directory.
func LogList() error {
	logDir := LogDir()

	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		return fmt.Errorf("log directory not found: %s", logDir)
	}

	files, err := getLogFiles(logDir)
	if err != nil {
		return err
	}

	if len(files) == 0 {
		fmt.Fprintf(os.Stderr, "No log files found in %s\n", logDir)
		return nil
	}

	fmt.Fprintf(os.Stderr, "Log files in %s:\n\n", logDir)
	for _, f := range files {
		fmt.Println(f)
	}

	return nil
}

// getLogFiles returns sorted list of .log files in the directory
// Files are sorted by name in reverse order (newest first, since pgbackrest
// uses timestamp-based names like pg-meta-backup.log)
func getLogFiles(logDir string) ([]string, error) {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		// Try with sudo if permission denied
		return getLogFilesWithSudo(logDir)
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".log") {
			files = append(files, entry.Name())
		}
	}

	// Sort by name (reverse for newest first)
	sort.Sort(sort.Reverse(sort.StringSlice(files)))
	return files, nil
}

// getLogFilesWithSudo tries to list log files with sudo (for permission issues)
func getLogFilesWithSudo(logDir string) ([]string, error) {
	cmd := exec.Command("sudo", "ls", "-1", logDir)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("cannot read log directory: %w", err)
	}

	var files []string
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && strings.HasSuffix(line, ".log") {
			files = append(files, line)
		}
	}

	sort.Sort(sort.Reverse(sort.StringSlice(files)))
	return files, nil
}

// LogTail shows real-time log output using tail -f
func LogTail(n int) error {
	logDir := LogDir()

	// Find latest log file
	latestLog, err := findLatestLog(logDir)
	if err != nil {
		return err
	}

	logPath := filepath.Join(logDir, latestLog)
	fmt.Fprintf(os.Stderr, "Tailing: %s\n\n", logPath)

	// Build tail command
	nStr := "50"
	if n > 0 {
		nStr = fmt.Sprintf("%d", n)
	}

	return postgres.RunWithSudoFallback([]string{"tail", "-f", "-n", nStr, logPath})
}

// LogCat displays log file contents.
// If filename is empty, shows the latest log file.
// If n > 0, shows only the last n lines.
func LogCat(filename string, n int) error {
	logDir := LogDir()

	var logFile string
	if filename != "" {
		// Security: only use the base name to prevent path traversal
		logFile = filepath.Base(filename)
		// Verify it's a .log file
		if !strings.HasSuffix(logFile, ".log") {
			return fmt.Errorf("invalid log file: %s (must end with .log)", logFile)
		}
	} else {
		var err error
		logFile, err = findLatestLog(logDir)
		if err != nil {
			return err
		}
	}

	logPath := filepath.Join(logDir, logFile)

	// Verify the resolved path is within logDir (defense in depth)
	cleanLogDir := filepath.Clean(logDir)
	cleanLogPath := filepath.Clean(logPath)
	if !strings.HasPrefix(cleanLogPath, cleanLogDir+string(filepath.Separator)) {
		return fmt.Errorf("invalid log file path")
	}

	if n > 0 {
		return postgres.RunWithSudoFallback([]string{"tail", "-n", fmt.Sprintf("%d", n), logPath})
	}

	return postgres.RunWithSudoFallback([]string{"cat", logPath})
}

// findLatestLog finds the most recent log file by name
func findLatestLog(logDir string) (string, error) {
	files, err := getLogFiles(logDir)
	if err != nil {
		return "", err
	}

	if len(files) == 0 {
		return "", fmt.Errorf("no log files found in %s", logDir)
	}

	return files[0], nil
}
