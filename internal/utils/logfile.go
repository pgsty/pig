/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

Safe lexical resolution for user-selected log files.
*/
package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResolveLogFilePath resolves a basename inside logDir without allowing an
// absolute path, a subdirectory, or lexical traversal. It does not inspect the
// filesystem or follow symlinks; callers retain command-specific suffix and
// file-type checks.
func ResolveLogFilePath(logDir, file string) (string, error) {
	if file == "" {
		return "", fmt.Errorf("invalid log file name: empty")
	}
	if file == "." || file == ".." {
		return "", fmt.Errorf("invalid log file name: %s", file)
	}
	if strings.ContainsAny(file, "\x00\r\n") {
		return "", fmt.Errorf("invalid log file name: control characters are not allowed")
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

// RequireRegularFileAsDBSU rejects links and non-regular files. When the caller
// cannot inspect the path directly, the same check is performed as the database
// OS user without following symlinks.
func RequireRegularFileAsDBSU(path, dbsu string) error {
	info, err := os.Lstat(path)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("path is not a regular file: %s", path)
		}
		return nil
	}
	if !os.IsPermission(err) {
		return fmt.Errorf("inspect file %s: %w", path, err)
	}
	output, dbsuErr := DBSUCommandStdout(dbsu, []string{
		"sh", "-c", `test ! -L "$1" && test -f "$1" && printf '%s\n' "$1"`, "sh", path,
	})
	if dbsuErr != nil {
		return fmt.Errorf("inspect file %s as %s: %w", path, dbsu, dbsuErr)
	}
	if strings.TrimSpace(output) != path {
		return fmt.Errorf("path is not a regular file: %s", path)
	}
	return nil
}
