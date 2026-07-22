/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

Shared helpers for patroni commands and structured outputs.
*/
package patroni

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"pig/internal/output"
	"pig/internal/utils"
)

var (
	patroniLookPath               = exec.LookPath
	patroniReadFile               = os.ReadFile
	patroniDBSUCommand            = utils.DBSUCommand
	patroniDBSUCommandStdout      = utils.DBSUCommandStdout
	patroniPassthroughDBSUCommand = utils.DBSUCommandPassthrough
)

type codedPatroniError struct {
	code int
	err  error
}

func (e *codedPatroniError) Error() string { return e.err.Error() }
func (e *codedPatroniError) Unwrap() error {
	return &utils.ExitCodeError{Code: output.ExitCode(e.code), Err: e.err}
}
func (e *codedPatroniError) StatusCode() int { return e.code }

func patroniConfigError(err error) error {
	if err == nil {
		return nil
	}
	var coded interface{ StatusCode() int }
	if errors.As(err, &coded) {
		return err
	}
	code := output.CodePtConfigReadFailed
	switch {
	case isPermissionDenied(err, ""):
		code = output.CodePtPermDenied
	case isConfigNotFound(err, ""):
		code = output.CodePtConfigNotFound
	case errors.Is(err, errClusterScopeMissing), errors.Is(err, errClusterScopeEmpty):
		code = output.CodePtScopeMissing
	case errors.Is(err, errClusterScopeInvalid):
		code = output.CodePtConfigResolveFailed
	}
	return &codedPatroniError{code: code, err: err}
}

var (
	errClusterScopeMissing = errors.New("patroni cluster scope missing")
	errClusterScopeEmpty   = errors.New("patroni cluster scope empty")
	errClusterScopeInvalid = errors.New("patroni cluster scope invalid")
)

// normalizeRole converts Patroni role strings to a stable snake_case form.
// Examples: "Leader" -> "leader", "Standby Leader" -> "standby_leader".
func normalizeRole(role string) string {
	role = strings.TrimSpace(strings.ToLower(role))
	if role == "" {
		return role
	}
	parts := strings.Fields(role)
	return strings.Join(parts, "_")
}

// isPermissionDenied checks if an error/output indicates a permission issue.
func isPermissionDenied(err error, output string) bool {
	if err == nil && output == "" {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(output + " " + errString(err)))
	if msg == "" {
		return false
	}
	return strings.Contains(msg, "permission denied") ||
		strings.Contains(msg, "not in the sudoers") ||
		strings.Contains(msg, "a password is required") ||
		strings.Contains(msg, "no tty present") ||
		strings.Contains(msg, "authentication failure") ||
		strings.Contains(msg, "authentication is required")
}

// isConfigNotFound checks if an error/output indicates Patroni config is missing.
func isConfigNotFound(err error, output string) bool {
	msg := strings.ToLower(strings.TrimSpace(output + " " + errString(err)))
	if msg == "" {
		return false
	}
	return (strings.Contains(msg, "patroni.yml") && strings.Contains(msg, "no such file")) ||
		(strings.Contains(msg, "config") && strings.Contains(msg, "not found")) ||
		(strings.Contains(msg, "provided config file") && strings.Contains(msg, "not existing or no read rights"))
}

// commandErrorDetail combines command output and error into a single detail string.
func commandErrorDetail(output string, err error) string {
	outMsg := strings.TrimSpace(output)
	errMsg := strings.TrimSpace(errString(err))
	if outMsg == "" {
		return errMsg
	}
	if errMsg == "" {
		return outMsg
	}
	if strings.Contains(errMsg, outMsg) {
		return errMsg
	}
	return errMsg + ": " + outMsg
}

func validateResolvedClusterName(cluster string) error {
	trimmed := strings.TrimSpace(cluster)
	if trimmed == "" {
		return fmt.Errorf("%w in %s", errClusterScopeMissing, configFile())
	}
	if trimmed != cluster || strings.HasPrefix(trimmed, "-") {
		return fmt.Errorf("%w in %s: %q", errClusterScopeInvalid, configFile(), cluster)
	}
	return nil
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
