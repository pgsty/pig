package pgbackrest

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"pig/internal/utils"
)

// LogDir returns the pgbackrest log directory from config or Pigsty default.
func LogDir(configPath, dbsu string) string {
	logDir, _ := ResolveLogDir(configPath, dbsu, false)
	return logDir
}

// ResolveLogDir returns the pgBackRest log directory. If requireConfig is true,
// config read errors are returned instead of silently falling back to defaults.
func ResolveLogDir(configPath, dbsu string, requireConfig bool) (string, error) {
	logDir, err := getLogPathFromConfig(configPath, dbsu)
	if err != nil {
		if requireConfig {
			return "", fmt.Errorf("cannot read pgBackRest config: %w", err)
		}
		return DefaultLogDir, nil
	}
	if logDir != "" {
		return logDir, nil
	}
	return DefaultLogDir, nil
}

// GetLogPathFromConfig reads log-path from pgBackRest config.
func GetLogPathFromConfig(configPath, dbsu string) string {
	logDir, err := getLogPathFromConfig(configPath, dbsu)
	if err != nil {
		return ""
	}
	return logDir
}

func getLogPathFromConfig(configPath, dbsu string) (string, error) {
	if configPath == "" {
		configPath = DefaultConfigPath
	}
	content, err := readConfigFile(configPath, dbsu)
	if err != nil {
		return "", err
	}
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) != "log-path" {
			continue
		}
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if value != "" {
			return value, nil
		}
	}
	return "", nil
}

func logDirForCommand(configPath, dbsu string) (string, error) {
	return ResolveLogDir(configPath, dbsu, configPath != "")
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
	if !strings.HasSuffix(file, ".log") {
		return "", fmt.Errorf("invalid log file: %s (must end with .log)", file)
	}

	cleanDir := filepath.Clean(logDir)
	logPath := filepath.Clean(filepath.Join(cleanDir, file))
	rel, err := filepath.Rel(cleanDir, logPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid log file path")
	}
	return logPath, nil
}

// LogList lists pgbackrest log files in the log directory.
func LogList(configPath, dbsu string) error {
	logDir, err := logDirForCommand(configPath, dbsu)
	if err != nil {
		return err
	}

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
// Files are sorted by modification time in reverse order (newest first).
// Uses DBSU privilege escalation if needed.
func getLogFiles(logDir, dbsu string) ([]string, error) {
	if dbsu == "" {
		dbsu = utils.GetDBSU("")
	}

	// Try direct read if we have permission
	if entries, err := os.ReadDir(logDir); err == nil {
		var infos []os.FileInfo
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".log") {
				if info, err := entry.Info(); err == nil {
					infos = append(infos, info)
				}
			}
		}
		sort.Slice(infos, func(i, j int) bool {
			if infos[i].ModTime().Equal(infos[j].ModTime()) {
				return infos[i].Name() > infos[j].Name()
			}
			return infos[i].ModTime().After(infos[j].ModTime())
		})
		files := make([]string, 0, len(infos))
		for _, info := range infos {
			files = append(files, info.Name())
		}
		return files, nil
	} else if !os.IsPermission(err) {
		return nil, fmt.Errorf("cannot read log directory: %w", err)
	}

	// Permission denied - use DBSU privilege escalation
	output, err := utils.DBSUCommandStdout(dbsu, []string{"ls", "-1t", logDir})
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

// LogTail shows real-time log output using tail -f.
func LogTail(configPath, dbsu string, filename string, n int) error {
	if n <= 0 {
		return fmt.Errorf("lines must be positive")
	}
	logDir, err := logDirForCommand(configPath, dbsu)
	if err != nil {
		return err
	}
	if dbsu == "" {
		dbsu = utils.GetDBSU("")
	}

	var logPath string
	if filename != "" {
		var err error
		logPath, err = resolveRequestedLogFile(logDir, filename)
		if err != nil {
			return err
		}
	} else {
		latestLog, err := findLatestLog(logDir, dbsu)
		if err != nil {
			return err
		}
		logPath = filepath.Join(logDir, latestLog)
	}

	fmt.Fprintf(os.Stderr, "Tailing: %s\n\n", logPath)

	// Build tail command
	nStr := fmt.Sprintf("%d", n)

	args := []string{"tail", "-f", "-n", nStr, logPath}
	return utils.DBSUCommand(dbsu, args)
}

// LogCat displays log file contents.
// If filename is empty, shows the latest log file.
// If n > 0, shows only the last n lines.
func LogCat(configPath, dbsu string, filename string, n int) error {
	if n <= 0 {
		return fmt.Errorf("lines must be positive")
	}
	logDir, err := logDirForCommand(configPath, dbsu)
	if err != nil {
		return err
	}
	if dbsu == "" {
		dbsu = utils.GetDBSU("")
	}

	var logFile string
	if filename != "" {
		logPath, err := resolveRequestedLogFile(logDir, filename)
		if err != nil {
			return err
		}
		logFile = filepath.Base(logPath)
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

	args := []string{"tail", "-n", fmt.Sprintf("%d", n), logPath}
	return utils.DBSUCommand(dbsu, args)
}

// LogShowJSONL outputs pgBackRest log lines as JSONL.
func LogShowJSONL(configPath, dbsu string, filename string, n int) error {
	if n <= 0 {
		return fmt.Errorf("lines must be positive")
	}
	logDir, err := logDirForCommand(configPath, dbsu)
	if err != nil {
		return err
	}
	if dbsu == "" {
		dbsu = utils.GetDBSU("")
	}

	var logFile string
	if filename != "" {
		logPath, err := resolveRequestedLogFile(logDir, filename)
		if err != nil {
			return err
		}
		logFile = filepath.Base(logPath)
	} else {
		var err error
		logFile, err = findLatestLog(logDir, dbsu)
		if err != nil {
			return err
		}
	}
	logPath := filepath.Join(logDir, logFile)

	output, err := utils.DBSUCommandStdout(dbsu, []string{"tail", "-n", fmt.Sprintf("%d", n), logPath})
	if err != nil {
		return err
	}
	return utils.PrintLogMessagesJSONL("pgbackrest", output)
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
