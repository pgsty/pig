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

// configureCmdIO sets stdin/stdout/stderr for external commands.
// In structured output mode, redirect stdout to stderr to keep stdout clean.
func configureCmdIO(cmd *exec.Cmd) {
	cmd.Stdin = os.Stdin
	if config.IsStructuredOutput() {
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		return
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
}

// Command runs a command with current user
func Command(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no command specified")
	}
	logrus.Debugf("executing command: %v", args)
	cmd := exec.Command(args[0], args[1:]...)
	configureCmdIO(cmd)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("command failed: %w", err)
	}
	return nil
}

// ShellCommand runs a command without sudo
func ShellCommand(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no command specified")
	}
	if TrySudo {
		return SudoCommand(args)
	}
	logrus.Debugf("executing shell command: %v", args)
	cmd := exec.Command(args[0], args[1:]...)
	configureCmdIO(cmd)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("shell command failed: %w", err)
	}
	return nil
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
		return fmt.Errorf("no command specified")
	}

	// Check environment variable to force no sudo (useful in Docker)
	if nosudoEnv := os.Getenv("PIG_NO_SUDO"); nosudoEnv == "1" || nosudoEnv == "true" {
		logrus.Debugf("PIG_NO_SUDO set, executing without sudo: %v", args)
		return Command(args)
	}

	if isRoot := os.Geteuid() == 0 || config.CurrentUser == "root"; !isRoot {
		// Check if sudo exists before trying to use it
		if _, err := exec.LookPath("sudo"); err != nil {
			// sudo not found - common in Docker containers
			// Try to execute directly as we might be root anyway
			logrus.Debugf("sudo not found, attempting direct execution: %v", args)
			logrus.Warnf("sudo command not available, trying to execute directly")
		} else {
			// sudo exists, prepend it to the command
			args = append([]string{"sudo"}, args...)
			logrus.Debugf("executing sudo command: %v", args)
		}
	} else {
		logrus.Debugf("executing as root: %v", args)
	}

	// Execute the command
	cmd := exec.Command(args[0], args[1:]...)
	configureCmdIO(cmd)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("command failed: %w", err)
	}
	return nil
}

// QuietSudoCommand runs a command with sudo if the current user is not root
func QuietSudoCommand(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no command specified")
	}

	// Check environment variable to force no sudo (useful in Docker)
	if nosudoEnv := os.Getenv("PIG_NO_SUDO"); nosudoEnv == "1" || nosudoEnv == "true" {
		logrus.Debugf("PIG_NO_SUDO set, executing quietly without sudo: %v", args)
		cmd := exec.Command(args[0], args[1:]...)
		return cmd.Run()
	}

	if isRoot := os.Geteuid() == 0 || config.CurrentUser == "root"; !isRoot {
		// Check if sudo exists before trying to use it
		if _, err := exec.LookPath("sudo"); err != nil {
			// sudo not found - common in Docker containers
			logrus.Debugf("sudo not found, attempting quiet direct execution: %v", args)
		} else {
			// sudo exists, prepend it to the command
			args = append([]string{"sudo"}, args...)
			logrus.Debugf("executing quiet sudo command: %v", args)
		}
	} else {
		logrus.Debugf("executing quietly as root: %v", args)
	}

	// Execute the command quietly
	cmd := exec.Command(args[0], args[1:]...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("quiet command failed: %w", err)
	}
	return nil
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

func DownloadFile(srcURL, dstPath string) error {
	// Best-effort remote size probe via HEAD.
	// HEAD is an optimization: it should not make downloads fail, and we must close the response
	// body to avoid leaking connections/HTTP2 streams.
	remoteSize := int64(-1)
	resp, err := http.Head(srcURL)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err == nil {
		if resp != nil && resp.StatusCode >= 200 && resp.StatusCode < 300 && resp.ContentLength > 0 {
			remoteSize = resp.ContentLength
		}
	} else {
		logrus.Debugf("HEAD failed for %s: %v", srcURL, err)
	}

	// If local file exists and we know the remote size, skip download when sizes match.
	if remoteSize > 0 {
		if fi, err := os.Stat(dstPath); err == nil {
			localSize := fi.Size()
			if localSize == remoteSize {
				logrus.Debugf("file already exists with same size: %s", dstPath)
				return nil
			}
		}
	}

	// Download the file
	logrus.Debugf("downloading: %s -> %s", srcURL, dstPath)
	resp, err = http.Get(srcURL)
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		return fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad http status: %d", resp.StatusCode)
	}

	// If HEAD did not provide a usable size, fallback to Content-Length from GET response.
	if remoteSize <= 0 && resp.ContentLength > 0 {
		remoteSize = resp.ContentLength
	}

	// Create the file with a temporary name first
	tmpPath := dstPath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}

	// Copy the content
	written, err := io.Copy(out, resp.Body)
	out.Close() // Close before rename
	if err != nil {
		os.Remove(tmpPath) // Clean up on error
		return fmt.Errorf("failed to write file: %w", err)
	}

	// Verify downloaded size
	if remoteSize > 0 && written != remoteSize {
		os.Remove(tmpPath)
		return fmt.Errorf("size mismatch: got %d bytes, expected %d bytes", written, remoteSize)
	}

	// Rename temporary file to final destination
	if err := os.Rename(tmpPath, dstPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename temporary file: %w", err)
	}

	logrus.Debugf("downloaded: %s (%d bytes)", dstPath, written)
	return nil
}
