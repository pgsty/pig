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
