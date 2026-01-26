/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>

PostgreSQL log operations: list, tail, cat, less, grep
All operations use the default log directory: /pg/log/postgres
*/
package postgres

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
)

// getLatestLogFile finds the latest CSV log file in /pg/log/postgres
func getLatestLogFile() (string, error) {
	entries, err := os.ReadDir(DefaultLogDir)
	if err != nil {
		return "", fmt.Errorf("cannot read log directory %s: %w", DefaultLogDir, err)
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
		return "", fmt.Errorf("no CSV log files found in %s", DefaultLogDir)
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].ModTime().After(files[j].ModTime())
	})

	return filepath.Join(DefaultLogDir, files[0].Name()), nil
}

// LogList lists all log files in /pg/log/postgres
func LogList() error {
	entries, err := os.ReadDir(DefaultLogDir)
	if err != nil {
		return fmt.Errorf("cannot read log directory %s: %w", DefaultLogDir, err)
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

// LogTail tails the latest log file (follow mode)
func LogTail(file string, lines int) error {
	var logFile string
	if file != "" {
		logFile = filepath.Join(DefaultLogDir, file)
	} else {
		var err error
		logFile, err = getLatestLogFile()
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
func LogCat(file string, lines int) error {
	var logFile string
	if file != "" {
		logFile = filepath.Join(DefaultLogDir, file)
	} else {
		var err error
		logFile, err = getLatestLogFile()
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
func LogLess(file string) error {
	var logFile string
	if file != "" {
		logFile = filepath.Join(DefaultLogDir, file)
	} else {
		var err error
		logFile, err = getLatestLogFile()
		if err != nil {
			return err
		}
	}

	cmdArgs := []string{"less", "+G", logFile}
	PrintHint(cmdArgs)
	return RunWithSudoFallback(cmdArgs)
}

// LogGrep searches log files
func LogGrep(pattern, file string, ignoreCase bool, context int) error {
	var logFile string
	if file != "" {
		logFile = filepath.Join(DefaultLogDir, file)
	} else {
		var err error
		logFile, err = getLatestLogFile()
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
