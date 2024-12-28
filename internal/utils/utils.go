package utils

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"pig/internal/config"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

var (

	// TrySudo is a flag to try to run a command with sudo
	TrySudo = false
)

// ShellCommand runs a command without sudo
func ShellCommand(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no command to run")
	}
	if TrySudo {
		return SudoCommand(args)
	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// SudoCommand runs a command with sudo if the current user is not root
func SudoCommand(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no command to run")
	}
	if config.CurrentUser != "root" {
		// insert sudo as first cmd arg
		args = append([]string{"sudo"}, args...)
	}

	// now split command and args again
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// PutFile writes content to a file at the specified path with proper permissions.
// It performs the following steps:
// 1. Checks if file exists and has identical content (to avoid unnecessary writes)
// 2. Ensures parent directory exists
// 3. Attempts direct write with standard permissions
// 4. Falls back to sudo if permission denied
func PutFile(filePath string, content []byte) error {
	logrus.Debugf("put file %q", filePath)

	// Skip write if file exists with identical content
	if data, err := os.ReadFile(filePath); err == nil && bytes.Equal(data, content) {
		return nil
	}

	// Ensure parent directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create parent directories %q: %w", dir, err)
	}

	// Attempt direct write first
	if err := os.WriteFile(filePath, content, 0644); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrPermission) {
		return fmt.Errorf("failed to write file %q: %w", filePath, err)
	}

	// Fall back to sudo mv approach for permission issues
	tmpFileName := filepath.Join(os.TempDir(), fmt.Sprintf("%s.%d", filepath.Base(filePath), time.Now().UnixNano()))
	tmpFile, err := os.Create(tmpFileName)
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	defer func(name string) {
		err := os.Remove(name)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				logrus.Debugf("failed to remove temporary file %q: %v", name, err)
			}
		}
	}(tmpFile.Name()) // Clean up temp file regardless of outcome

	// Write content to temporary file
	if err := os.WriteFile(tmpFile.Name(), content, 0644); err != nil {
		return fmt.Errorf("failed to write content to temporary file %q: %w", tmpFile.Name(), err)
	}

	// Use sudo to move temp file to target location
	if err := SudoCommand([]string{"mv", tmpFile.Name(), filePath}); err != nil {
		return fmt.Errorf("failed to move file with sudo %q -> %q: %w", tmpFile.Name(), filePath, err)
	}

	return nil
}

// DelFile removes a file, if permission denied, try to remove with sudo
func DelFile(filePath string) error {
	logrus.Debugf("remove file %q", filePath)
	err := os.Remove(filePath)
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return nil // file not exists, do nothing, success
	}

	// if not permission error, return original error
	if !errors.Is(err, os.ErrPermission) {
		return fmt.Errorf("failed to remove %q: %w", filePath, err)
	}

	// if permission denied, try to remove with sudo
	if serr := SudoCommand([]string{"rm", "-f", filePath}); serr != nil {
		return fmt.Errorf("failed to sudo rm %q: %w", filePath, serr)
	}
	return nil
}

// PadKV pads a key-value pair with spaces to the right
func PadKV(key string, value string) {
	fmt.Printf("%-16s : %s\n", key, value)
}

// PadRight pads a string with spaces to the right to a given length
func PadHeader(str string, length int) string {
	var buf strings.Builder
	buf.WriteByte('#')
	buf.WriteByte(' ')
	buf.WriteByte('[')
	buf.WriteString(str)
	buf.WriteByte(']')
	if buf.Len() >= length {
		return buf.String()
	}
	// pad with '='
	pad := length - buf.Len() - 1
	buf.WriteByte(' ')
	for i := 0; i < pad; i++ {
		buf.WriteByte('=')
	}
	return buf.String()
}
