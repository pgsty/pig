package utils

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
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

// Command runs a command with current user
func Command(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no command to run")
	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

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

// ShellOutput runs a command and returns the output
func ShellOutput(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

// SudoCommand runs a command with sudo if the current user is not root
// TODO: FineGrained control of which commands can be run with sudo
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

// QuietSudoCommand runs a command with sudo if the current user is not root
func QuietSudoCommand(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no command to run")
	}
	if config.CurrentUser != "root" {
		// insert sudo as first cmd arg
		args = append([]string{"sudo"}, args...)
	}

	// now split command and args again
	cmd := exec.Command(args[0], args[1:]...)
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

// Mkdir creates a directory, if permission denied, try to create with sudo
func Mkdir(path string) error {
	logrus.Debugf("mkdir -p %s", path)
	// check if dir exists, if exists, return nil
	if fi, err := os.Stat(path); err == nil && fi.IsDir() {
		return nil
	}
	err := os.MkdirAll(path, 0755)
	if err == nil {
		return nil
	}
	// otherwise, try sudo
	return SudoCommand([]string{"mkdir", "-p", path})
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

// SudoRunShellScript will run a shell script with sudo
func SudoRunShellScript(script string) error {
	// generate tmp file name with timestamp
	tmpFile := fmt.Sprintf("script-%s.sh", time.Now().Format("20240101120000"))
	scriptPath := filepath.Join(os.TempDir(), tmpFile)
	logrus.Debugf("create tmp script: %s", scriptPath)

	if err := os.WriteFile(scriptPath, []byte(script), 0644); err != nil {
		return fmt.Errorf("failed to create tmp script %s: %s", scriptPath, err)
	}

	err := SudoCommand([]string{"bash", scriptPath})
	if err != nil {
		return fmt.Errorf("failed to run script: %v", err)
	}
	return nil
}

func DownloadFile(srcURL, dstPath string) error {
	// Check remote file size first
	resp, err := http.Head(srcURL)
	if err != nil {
		return fmt.Errorf("failed to head url: %v", err)
	}
	remoteSize := resp.ContentLength

	// Check if local file exists and has the same size
	if fi, err := os.Stat(dstPath); err == nil {
		localSize := fi.Size()
		if localSize == remoteSize {
			logrus.Debugf("skip downloading %s: local file exists with same size", dstPath)
			return nil
		}
	}

	// Download the file
	resp, err = http.Get(srcURL)
	if err != nil {
		return fmt.Errorf("download failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad http status code: %d", resp.StatusCode)
	}

	// Create the file with a temporary name first
	tmpPath := dstPath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to create tmp file: %v", err)
	}

	// Copy the content
	written, err := io.Copy(out, resp.Body)
	out.Close() // Close before rename
	if err != nil {
		os.Remove(tmpPath) // Clean up on error
		return fmt.Errorf("failed to save file: %v", err)
	}

	// Verify downloaded size
	if written != remoteSize {
		os.Remove(tmpPath)
		return fmt.Errorf("size mismatch after download: got %d, expected %d", written, remoteSize)
	}

	// Rename temporary file to final destination
	if err := os.Rename(tmpPath, dstPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename temporary file: %v", err)
	}

	logrus.Debugf("download %s to %s, (%d bytes)", srcURL, dstPath, written)
	return nil
}
