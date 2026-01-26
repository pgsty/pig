/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>

PostgreSQL log operations: list, tail, cat, less, grep
All operations use the default log directory: /pg/log/postgres
*/
package postgres

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

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

	fmt.Printf("%s%-40s %10s  %s%s\n", ColorBold, "NAME", "SIZE", "MODIFIED", ColorReset)
	for _, f := range files {
		fmt.Printf("%-40s %10s  %s\n", f.Name(), FormatSize(f.Size()), f.ModTime().Format("2006-01-02 15:04:05"))
	}
	fmt.Printf("\n%sTotal: %d files, %s%s\n", ColorCyan, len(files), FormatSize(totalSize), ColorReset)
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
	var logFile string
	if file != "" {
		logFile = filepath.Join(logDir, file)
	} else {
		var err error
		logFile, err = getLatestLogFile(logDir)
		if err != nil {
			return err
		}
	}

	if lines <= 0 {
		lines = 50
	}

	cmdArgs := []string{"tail", "-n", strconv.Itoa(lines), "-f", logFile}
	PrintHint(cmdArgs)
	return RunWithSudoFallback(cmdArgs)
}

// LogCat outputs log file content
func LogCat(logDir, file string, lines int) error {
	var logFile string
	if file != "" {
		logFile = filepath.Join(logDir, file)
	} else {
		var err error
		logFile, err = getLatestLogFile(logDir)
		if err != nil {
			return err
		}
	}

	if lines <= 0 {
		lines = 100
	}

	cmdArgs := []string{"tail", "-n", strconv.Itoa(lines), logFile}
	PrintHint(cmdArgs)
	return RunWithSudoFallback(cmdArgs)
}

// LogLess opens the latest log file in less
func LogLess(logDir, file string) error {
	var logFile string
	if file != "" {
		logFile = filepath.Join(logDir, file)
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
		logFile = filepath.Join(logDir, file)
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
