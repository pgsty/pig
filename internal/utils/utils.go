package utils

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"pig/internal/config"
	"strings"

	"github.com/sirupsen/logrus"
)

var sudoCommandSummaryOut io.Writer = os.Stderr

// configureCmdIO sets stdin/stdout/stderr for external commands.
// In structured output mode, redirect stdout to stderr to keep stdout clean.
func configureCmdIO(cmd *exec.Cmd) {
	if os.Getenv("PIG_NON_INTERACTIVE") == "" && !config.IsStructuredOutput() {
		cmd.Stdin = os.Stdin
	}
	if config.IsStructuredOutput() {
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		return
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
}

func prependSudoArgs(args []string) []string {
	if os.Getenv("PIG_NON_INTERACTIVE") != "" || config.IsStructuredOutput() {
		return append([]string{"sudo", "-n"}, args...)
	}
	return append([]string{"sudo"}, args...)
}

// Command runs a command with current user
func Command(args []string) error {
	return CommandInDir(args, "")
}

// CommandInDir runs a command with current user in the given working directory.
func CommandInDir(args []string, dir string) error {
	if len(args) == 0 {
		return fmt.Errorf("no command specified")
	}
	logrus.Debugf("executing command: %v", args)
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	configureCmdIO(cmd)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("command failed: %w", err)
	}
	return nil
}

// CommandWithEnv runs a command with additional environment variables.
func CommandWithEnv(args []string, env []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no command specified")
	}
	logrus.Debugf("executing command: %v", args)
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Env = append(os.Environ(), env...)
	configureCmdIO(cmd)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("command failed: %w", err)
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
			args = prependSudoArgs(args)
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
			args = prependSudoArgs(args)
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

// SudoCommandSummary rolls subprocess output through one terminal line and
// includes only a short tail when the command fails. Structured output remains
// silent so stdout and stderr stay machine-readable.
func SudoCommandSummary(args []string, tailLines int) error {
	if len(args) == 0 {
		return fmt.Errorf("no command specified")
	}
	args = append([]string(nil), args...)
	if nosudo := os.Getenv("PIG_NO_SUDO"); nosudo != "1" && nosudo != "true" {
		if isRoot := os.Geteuid() == 0 || config.CurrentUser == "root"; !isRoot {
			if _, err := exec.LookPath("sudo"); err == nil {
				args = prependSudoArgs(args)
			}
		}
	}

	logrus.Debugf("executing summarized command: %v", args)
	tail := NewTailBuffer(64 * 1024)
	var commandOutput io.Writer = tail
	var progress *rollingLineWriter
	if !config.IsStructuredOutput() && sudoCommandSummaryOut != nil {
		progress = newRollingLineWriter(sudoCommandSummaryOut, tail)
		commandOutput = progress
		progress.render([]byte(strings.Join(args, " ")))
	}
	cmd := exec.Command(args[0], args[1:]...)
	if os.Getenv("PIG_NON_INTERACTIVE") == "" && !config.IsStructuredOutput() {
		cmd.Stdin = os.Stdin
	}
	cmd.Stdout = commandOutput
	cmd.Stderr = commandOutput
	err := cmd.Run()
	if progress != nil {
		progress.Finish()
	}
	if err != nil {
		detail := commandOutputTail(tail.String(), tailLines)
		if detail != "" {
			return fmt.Errorf("command failed: %w: %s", err, detail)
		}
		return fmt.Errorf("command failed: %w", err)
	}
	return nil
}

func commandOutputTail(value string, lines int) string {
	if lines <= 0 {
		return ""
	}
	value = strings.ReplaceAll(value, "\r", "\n")
	raw := strings.Split(value, "\n")
	parts := make([]string, 0, len(raw))
	for _, line := range raw {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		const lineLimit = 320
		if runes := []rune(line); len(runes) > lineLimit {
			line = string(runes[:lineLimit-3]) + "..."
		}
		parts = append(parts, line)
	}
	if len(parts) > lines {
		parts = parts[len(parts)-lines:]
	}
	return strings.Join(parts, " | ")
}

// PutFile writes content to a file at the specified path with proper permissions.
// It performs the following steps:
// 1. Checks if file exists and has identical content (to avoid unnecessary writes)
// 2. Ensures parent directory exists
// 3. Atomically replaces the file while preserving an existing permission mode
// 4. Falls back to a privileged install if the directory is not writable
func PutFile(filePath string, content []byte) error {
	mode, err := existingFileMode(filePath, 0644)
	if err != nil {
		return err
	}
	return PutFileMode(filePath, content, mode)
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

// PadHeader formats a section header padded with '=' to the given length
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
	mode, err := existingFileMode(dstPath, 0644)
	if err != nil {
		return err
	}

	logrus.Debugf("downloading: %s -> %s", srcURL, dstPath)
	resp, err := DefaultClient().Get(srcURL)
	if err != nil {
		return fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad http status: %d", resp.StatusCode)
	}

	var written int64
	if err := AtomicWrite(dstPath, mode, func(w io.Writer) error {
		var copyErr error
		written, copyErr = io.Copy(w, resp.Body)
		if copyErr != nil {
			return fmt.Errorf("failed to copy download: %w", copyErr)
		}
		if resp.ContentLength >= 0 && written != resp.ContentLength {
			return fmt.Errorf("size mismatch: got %d bytes, expected %d bytes", written, resp.ContentLength)
		}
		return nil
	}); err != nil {
		return err
	}

	logrus.Debugf("downloaded: %s (%d bytes)", dstPath, written)
	return nil
}
