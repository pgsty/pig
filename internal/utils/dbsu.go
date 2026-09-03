package utils

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"pig/internal/config"

	"github.com/sirupsen/logrus"
)

const DefaultDBSU = "postgres"

// ExitCodeError represents an error with an associated exit code.
// Use this when you want to propagate subprocess exit codes to callers.
type ExitCodeError struct {
	Code   int
	Err    error
	Silent bool
}

func (e *ExitCodeError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("command exited with code %d: %v", e.Code, e.Err)
	}
	return fmt.Sprintf("command exited with code %d", e.Code)
}

func (e *ExitCodeError) Unwrap() error {
	return e.Err
}

func IsSilentExit(err error) bool {
	var exitErr *ExitCodeError
	return errors.As(err, &exitErr) && exitErr.Silent
}

// ExitCode returns the exit code from an ExitCodeError, or 1 if not an ExitCodeError.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *ExitCodeError
	if errors.As(err, &exitErr) {
		return exitErr.Code
	}
	return 1
}

// GetDBSU returns the database superuser name
// Priority: override parameter > PIG_DBSU env > default "postgres"
func GetDBSU(override string) string {
	if override != "" {
		return override
	}
	if dbsu := os.Getenv("PIG_DBSU"); dbsu != "" {
		return dbsu
	}
	return DefaultDBSU
}

// IsDBSU checks if current user is the database superuser
func IsDBSU(dbsu string) bool {
	result := config.CurrentUser == dbsu
	logrus.Debugf("IsDBSU: currentUser=%q, dbsu=%q, result=%v", config.CurrentUser, dbsu, result)
	return result
}

// DBSUCommand executes a command as the database superuser.
// Execution strategy based on current user:
//   - If current user is DBSU: execute directly
//   - If current user is root: use "su - <dbsu> -c" (no sudo needed, works in containers)
//   - Otherwise: use "sudo -H -u <dbsu> --" (requires sudo privileges)
//
// The sudo path deliberately avoids -i and -s. Both options serialize the
// command through a shell and therefore cannot preserve arbitrary argv values.
//
// Returns ExitCodeError if the command exits with non-zero status.
// Callers can use ExitCode(err) to get the exit code if needed.
func DBSUCommand(dbsu string, args []string) error {
	return runDBSUCommand(dbsu, args, false)
}

// DBSUCommandPreserveStdout executes a command as DBSU while keeping stdout
// on stdout even in structured output mode. Use this for raw passthrough flows.
func DBSUCommandPreserveStdout(dbsu string, args []string) error {
	return runDBSUCommand(dbsu, args, true)
}

// DBSUCommandPassthrough executes a command with the child's stdin, stdout,
// and stderr attached directly to the current process. Unlike the shared
// streaming helper it adds no capture writers, preserving terminal identity,
// interactive prompts, editors, and native diagnostics.
func DBSUCommandPassthrough(dbsu string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no command specified")
	}
	cmd := buildDBSUCmd(dbsu, args)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return &ExitCodeError{Code: exitErr.ExitCode(), Err: err, Silent: true}
		}
		return fmt.Errorf("command failed: %w", err)
	}
	return nil
}

// BuildDBSUCommand creates a command that runs as the database superuser.
// Callers that need custom pipes or long-lived subprocesses can use this
// instead of DBSUCommand.
func BuildDBSUCommand(dbsu string, args []string) (*exec.Cmd, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("no command specified")
	}
	return buildDBSUCmd(dbsu, args), nil
}

func runDBSUCommand(dbsu string, args []string, preserveStdout bool) error {
	stdout, stderr := commandWriters(config.IsStructuredOutput(), preserveStdout)
	result, err := executeDBSUCommand(dbsu, args, os.Stdin, stdout, stderr)
	return dbsuCommandError(result, err)
}

func dbsuCommandError(result DBSUCommandResult, err error) error {
	if err != nil {
		outStr := strings.TrimSpace(result.CombinedOutput())
		if exitErr, ok := err.(*exec.ExitError); ok {
			if outStr != "" {
				err = fmt.Errorf("%w: %s", err, outStr)
				return &ExitCodeError{Code: exitErr.ExitCode(), Err: err, Silent: true}
			}
			return &ExitCodeError{Code: exitErr.ExitCode(), Err: err}
		}
		if outStr != "" {
			return fmt.Errorf("command failed: %w: %s", err, outStr)
		}
		return fmt.Errorf("command failed: %w", err)
	}
	return nil
}

// DBSUCommandResult keeps subprocess stdout and stderr separate. This is
// important for callers that parse stdout as JSON, YAML, or another
// machine-readable format while still retaining stderr for diagnostics.
type DBSUCommandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// CombinedOutput returns captured stdout followed by captured stderr. It is
// provided for legacy callers whose output was historically merged.
func (r DBSUCommandResult) CombinedOutput() string {
	return r.Stdout + r.Stderr
}

// CaptureDBSUCommand executes a command as DBSU and captures stdout and stderr
// independently. The returned error is the underlying command execution error;
// ExitCode is zero on success and non-zero on failure.
func CaptureDBSUCommand(dbsu string, args []string) (DBSUCommandResult, error) {
	return executeDBSUCommand(dbsu, args, nil, nil, nil)
}

// executeDBSUCommand is the single execution primitive used by both streaming
// and captured DBSU commands. Separate buffers avoid concurrent stdout/stderr
// writes to the same bytes.Buffer when os/exec copies both pipes in parallel.
func executeDBSUCommand(dbsu string, args []string, stdin io.Reader, stdout, stderr io.Writer) (DBSUCommandResult, error) {
	if len(args) == 0 {
		return DBSUCommandResult{ExitCode: 1}, fmt.Errorf("no command specified")
	}

	cmd := buildDBSUCmd(dbsu, args)
	var stdoutBuffer bytes.Buffer
	var stderrBuffer bytes.Buffer
	cmd.Stdin = stdin
	cmd.Stdout = captureWriter(stdout, &stdoutBuffer)
	cmd.Stderr = captureWriter(stderr, &stderrBuffer)

	err := cmd.Run()
	result := DBSUCommandResult{
		Stdout: stdoutBuffer.String(),
		Stderr: stderrBuffer.String(),
	}
	if err == nil {
		return result, nil
	}
	result.ExitCode = 1
	if exitErr, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exitErr.ExitCode()
	}
	return result, err
}

func captureWriter(stream io.Writer, capture io.Writer) io.Writer {
	if stream == nil {
		return capture
	}
	return io.MultiWriter(stream, capture)
}

func commandWriters(structuredOutput bool, preserveStdout bool) (io.Writer, io.Writer) {
	if structuredOutput && !preserveStdout {
		return os.Stderr, os.Stderr
	}
	return os.Stdout, os.Stderr
}

// DBSUCommandOutput executes a command as the database superuser and returns
// combined stdout and stderr. Prefer DBSUCommandStdout for machine-readable
// output; this helper is intended for diagnostic command output.
func DBSUCommandOutput(dbsu string, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("no command specified")
	}

	// Deliberately give os/exec the same comparable writer for both streams.
	// It then connects stdout and stderr to one child pipe, preserving their
	// cross-stream order without the data race caused by two MultiWriters that
	// shared an unprotected buffer in the streaming path.
	cmd := buildDBSUCmd(dbsu, args)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		return output.String(), fmt.Errorf("command failed: %w", err)
	}
	return output.String(), nil
}

// DBSUCommandStdout executes a command as the database superuser and returns
// stdout only. Stderr is reserved for error details and is not mixed into data.
func DBSUCommandStdout(dbsu string, args []string) (string, error) {
	result, err := CaptureDBSUCommand(dbsu, args)
	if err != nil {
		errText := strings.TrimSpace(result.Stderr)
		if errText != "" {
			return result.Stdout, fmt.Errorf("command failed: %w: %s", err, errText)
		}
		return result.Stdout, fmt.Errorf("command failed: %w", err)
	}
	return result.Stdout, nil
}

// buildDBSUCmd creates an exec.Cmd for running a command as DBSU.
func buildDBSUCmd(dbsu string, args []string) *exec.Cmd {
	if IsDBSU(dbsu) {
		logrus.Debugf("executing as %s: %s", dbsu, args[0])
		return exec.Command(args[0], args[1:]...)
	}

	if config.CurrentUser == "root" {
		cmdStr := ShellQuoteArgs(args)
		logrus.Debugf("executing via su as %s: %s", dbsu, args[0])
		return exec.Command("su", "-", dbsu, "-c", cmdStr)
	}

	sudoArgs := []string{"-H", "-u", dbsu, "--"}
	if os.Getenv("PIG_NON_INTERACTIVE") != "" {
		sudoArgs = []string{"-n", "-H", "-u", dbsu, "--"}
	}
	sudoArgs = append(sudoArgs, args...)
	logrus.Debugf("executing via sudo as %s: %s", dbsu, args[0])
	return exec.Command("sudo", sudoArgs...)
}

// ShellQuoteArgs joins args into a shell-safe command string.
// Each argument is properly quoted to handle spaces and special characters.
func ShellQuoteArgs(args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		// Wrap in single quotes and escape existing single quotes
		if arg == "" || strings.ContainsAny(arg, " \t\n'\"\\$`!*?[]{}()<>|&;#~") {
			quoted[i] = "'" + strings.ReplaceAll(arg, "'", "'\"'\"'") + "'"
		} else {
			quoted[i] = arg
		}
	}
	return strings.Join(quoted, " ")
}
