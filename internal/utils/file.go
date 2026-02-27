/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

File operations with privilege escalation fallback.
*/
package utils

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/sirupsen/logrus"
)

// RunWithSudoFallback runs command directly first, retries with sudo if permission denied.
// This is useful for commands like tail, cat, less that may need elevated privileges
// to read files owned by other users.
func RunWithSudoFallback(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no command specified")
	}

	// If already root, just run directly
	if os.Geteuid() == 0 {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	// Try running directly first
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		return nil
	}

	// Check if it's a permission error
	stderrStr := stderr.String()
	if strings.Contains(stderrStr, "Permission denied") ||
		strings.Contains(stderrStr, "permission denied") ||
		strings.Contains(stderrStr, "Operation not permitted") {
		// Retry with sudo
		logrus.Debugf("permission denied, retrying with sudo")
		sudoCmd := exec.Command("sudo", args...)
		sudoCmd.Stdin = os.Stdin
		sudoCmd.Stdout = os.Stdout
		sudoCmd.Stderr = os.Stderr
		return sudoCmd.Run()
	}

	// Not a permission error, print the stderr and return the error
	fmt.Fprint(os.Stderr, stderrStr)
	return err
}

// ReadFileAsDBSU reads a file using DBSU privilege escalation if needed.
// Execution strategy:
//   - If current user is DBSU: read directly
//   - Otherwise: use DBSUCommandOutput with cat
func ReadFileAsDBSU(path, dbsu string) (string, error) {
	if dbsu == "" {
		dbsu = GetDBSU("")
	}

	logrus.Debugf("ReadFileAsDBSU: path=%s, dbsu=%s, isDBSU=%v", path, dbsu, IsDBSU(dbsu))

	// If current user is DBSU, read directly
	if IsDBSU(dbsu) {
		logrus.Debugf("reading file directly as DBSU")
		content, err := os.ReadFile(path)
		if err != nil {
			logrus.Debugf("direct read failed: %v", err)
			return "", err
		}
		logrus.Debugf("direct read succeeded, content length=%d", len(content))
		return string(content), nil
	}

	// Use DBSUCommandOutput for privilege escalation
	logrus.Debugf("reading file via privilege escalation")
	return DBSUCommandOutput(dbsu, []string{"cat", path})
}
