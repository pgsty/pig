package pgbackrest

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"pig/internal/utils"
)

// LogDir returns the pgbackrest log directory.
// Hardcoded to Pigsty default: /pg/log/pgbackrest
func LogDir() string {
	return DefaultLogDir
}

// LogList lists pgbackrest log files in the log directory.
func LogList(dbsu string) error {
	logDir := LogDir()

	files, err := getLogFiles(logDir, dbsu)
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
// Uses DBSU privilege escalation if needed.
func getLogFiles(logDir, dbsu string) ([]string, error) {
	if dbsu == "" {
		dbsu = utils.GetDBSU("")
	}

	// Try direct read if we have permission
	if entries, err := os.ReadDir(logDir); err == nil {
		var files []string
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".log") {
				files = append(files, entry.Name())
			}
		}
		sort.Sort(sort.Reverse(sort.StringSlice(files)))
		return files, nil
	} else if !os.IsPermission(err) {
		return nil, fmt.Errorf("cannot read log directory: %w", err)
	}

	// Permission denied - use DBSU privilege escalation
	output, err := utils.DBSUCommandOutput(dbsu, []string{"ls", "-1", logDir})
	if err != nil {
		return nil, fmt.Errorf("cannot read log directory: %w", err)
	}

	var files []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && strings.HasSuffix(line, ".log") {
			files = append(files, line)
		}
	}

	sort.Sort(sort.Reverse(sort.StringSlice(files)))
	return files, nil
}

// LogTail shows real-time log output using tail -f
func LogTail(dbsu string, n int) error {
	logDir := LogDir()
	if dbsu == "" {
		dbsu = utils.GetDBSU("")
	}

	// Find latest log file
	latestLog, err := findLatestLog(logDir, dbsu)
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

	args := []string{"tail", "-f", "-n", nStr, logPath}
	return utils.DBSUCommand(dbsu, args)
}

// LogCat displays log file contents.
// If filename is empty, shows the latest log file.
// If n > 0, shows only the last n lines.
func LogCat(dbsu string, filename string, n int) error {
	logDir := LogDir()
	if dbsu == "" {
		dbsu = utils.GetDBSU("")
	}

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
		logFile, err = findLatestLog(logDir, dbsu)
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

	var args []string
	if n > 0 {
		args = []string{"tail", "-n", fmt.Sprintf("%d", n), logPath}
	} else {
		args = []string{"cat", logPath}
	}

	return utils.DBSUCommand(dbsu, args)
}

// findLatestLog finds the most recent log file by name
func findLatestLog(logDir, dbsu string) (string, error) {
	files, err := getLogFiles(logDir, dbsu)
	if err != nil {
		return "", err
	}

	if len(files) == 0 {
		return "", fmt.Errorf("no log files found in %s", logDir)
	}

	return files[0], nil
}

