package utils

import (
	"bytes"
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
	Code int
	Err  error
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

// ExitCode returns the exit code from an ExitCodeError, or 1 if not an ExitCodeError.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	if exitErr, ok := err.(*ExitCodeError); ok {
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
//   - Otherwise: use "sudo -inu <dbsu> --" (requires sudo privileges)
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

func runDBSUCommand(dbsu string, args []string, preserveStdout bool) error {
	if len(args) == 0 {
		return fmt.Errorf("no command specified")
	}

	cmd := buildDBSUCmd(dbsu, args)
	var out bytes.Buffer
	var stdout io.Writer
	var stderr io.Writer
	cmd.Stdin = os.Stdin
	stdout, stderr = commandWriters(config.IsStructuredOutput(), preserveStdout)
	cmd.Stdout = io.MultiWriter(stdout, &out)
	cmd.Stderr = io.MultiWriter(stderr, &out)

	if err := cmd.Run(); err != nil {
		outStr := strings.TrimSpace(out.String())
		if exitErr, ok := err.(*exec.ExitError); ok {
			if outStr != "" {
				err = fmt.Errorf("%w: %s", err, outStr)
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

func commandWriters(structuredOutput bool, preserveStdout bool) (io.Writer, io.Writer) {
	if structuredOutput && !preserveStdout {
		return os.Stderr, os.Stderr
	}
	return os.Stdout, os.Stderr
}

// DBSUCommandOutput executes a command as the database superuser and captures output.
// Uses the same execution strategy as DBSUCommand.
func DBSUCommandOutput(dbsu string, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("no command specified")
	}

	cmd := buildDBSUCmd(dbsu, args)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("command failed: %w", err)
	}
	return out.String(), nil
}

// buildDBSUCmd creates an exec.Cmd for running a command as DBSU.
func buildDBSUCmd(dbsu string, args []string) *exec.Cmd {
	if IsDBSU(dbsu) {
		logrus.Debugf("executing as %s: %v", dbsu, args)
		return exec.Command(args[0], args[1:]...)
	}

	if config.CurrentUser == "root" {
		cmdStr := ShellQuoteArgs(args)
		logrus.Debugf("executing via su: su - %s -c %q", dbsu, cmdStr)
		return exec.Command("su", "-", dbsu, "-c", cmdStr)
	}

	sudoArgs := []string{"-inu", dbsu, "--"}
	if os.Getenv("PIG_NON_INTERACTIVE") != "" {
		sudoArgs = []string{"-n", "-inu", dbsu, "--"}
	}
	sudoArgs = append(sudoArgs, args...)
	logrus.Debugf("executing via sudo: sudo %v", sudoArgs)
	return exec.Command("sudo", sudoArgs...)
}

// ShellQuoteArgs joins args into a shell-safe command string.
// Each argument is properly quoted to handle spaces and special characters.
func ShellQuoteArgs(args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		// Wrap in single quotes and escape existing single quotes
		if strings.ContainsAny(arg, " \t\n'\"\\$`!*?[]{}()<>|&;#~") {
			quoted[i] = "'" + strings.ReplaceAll(arg, "'", "'\"'\"'") + "'"
		} else {
			quoted[i] = arg
		}
	}
	return strings.Join(quoted, " ")
}
