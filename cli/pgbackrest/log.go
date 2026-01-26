package pgbackrest

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"pig/internal/config"
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

	var output string
	var err error

	// If current user is DBSU, read directly
	if utils.IsDBSU(dbsu) {
		entries, dirErr := os.ReadDir(logDir)
		if dirErr != nil {
			return nil, fmt.Errorf("cannot read log directory: %w", dirErr)
		}
		var files []string
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".log") {
				files = append(files, entry.Name())
			}
		}
		sort.Sort(sort.Reverse(sort.StringSlice(files)))
		return files, nil
	}

	// If current user is root, use su
	if config.CurrentUser == "root" {
		output, err = runAsDBSU(dbsu, []string{"ls", "-1", logDir})
	} else {
		// Try direct read first
		entries, dirErr := os.ReadDir(logDir)
		if dirErr == nil {
			var files []string
			for _, entry := range entries {
				if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".log") {
					files = append(files, entry.Name())
				}
			}
			sort.Sort(sort.Reverse(sort.StringSlice(files)))
			return files, nil
		}
		// Permission denied - try sudo as DBSU
		if os.IsPermission(dirErr) {
			output, err = runAsDBSUSudo(dbsu, []string{"ls", "-1", logDir})
		} else {
			return nil, fmt.Errorf("cannot read log directory: %w", dirErr)
		}
	}

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

// runAsDBSU runs a command as DBSU using su (when current user is root)
func runAsDBSU(dbsu string, args []string) (string, error) {
	cmdStr := utils.ShellQuoteArgs(args)
	cmd := exec.Command("su", "-", dbsu, "-c", cmdStr)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("su failed: %w: %s", err, stderr.String())
	}
	return out.String(), nil
}

// runAsDBSUSudo runs a command as DBSU using sudo (when current user is neither DBSU nor root)
func runAsDBSUSudo(dbsu string, args []string) (string, error) {
	sudoArgs := append([]string{"-inu", dbsu, "--"}, args...)
	cmd := exec.Command("sudo", sudoArgs...)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("sudo failed: %w: %s", err, stderr.String())
	}
	return out.String(), nil
}
