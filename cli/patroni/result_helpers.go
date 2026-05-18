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
	patroniLookPath          = exec.LookPath
	patroniReadFile          = os.ReadFile
	patroniStat              = os.Stat
	patroniGetClusterName    = GetClusterName
	patroniRunPatronictl     = runPatronictl
	patroniDBSUCommandOutput = utils.DBSUCommandOutput
)

var (
	errClusterConfigRead   = errors.New("patroni config read failed")
	errClusterScopeMissing = errors.New("patroni cluster scope missing")
	errClusterScopeEmpty   = errors.New("patroni cluster scope empty")
	errClusterScopeInvalid = errors.New("patroni cluster scope invalid")
)

func newClusterConfigReadError(err error) error {
	if err == nil {
		return errClusterConfigRead
	}
	return fmt.Errorf("%w: %w", errClusterConfigRead, err)
}

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
		(strings.Contains(msg, "config") && strings.Contains(msg, "not found"))
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

func clusterNameErrorResult(err error) *output.Result {
	detail := errString(err)
	switch {
	case errors.Is(err, errClusterConfigRead):
		if isPermissionDenied(err, "") {
			return output.Fail(output.CodePtPermDenied,
				fmt.Sprintf("Permission denied reading Patroni config: %s", DefaultConfigPath)).
				WithDetail(detail)
		}
		if isConfigNotFound(err, "") {
			return output.Fail(output.CodePtConfigNotFound,
				fmt.Sprintf("Patroni config not found: %s", DefaultConfigPath)).
				WithDetail(detail)
		}
		return output.Fail(output.CodePtConfigReadFailed,
			fmt.Sprintf("Cannot read Patroni config: %s", DefaultConfigPath)).
			WithDetail(detail)
	case errors.Is(err, errClusterScopeEmpty):
		return output.Fail(output.CodePtScopeMissing,
			fmt.Sprintf("Patroni cluster scope is empty in %s", DefaultConfigPath)).
			WithDetail(detail)
	case errors.Is(err, errClusterScopeMissing):
		return output.Fail(output.CodePtScopeMissing,
			fmt.Sprintf("Patroni cluster scope not found in %s", DefaultConfigPath)).
			WithDetail(detail)
	case errors.Is(err, errClusterScopeInvalid):
		return output.Fail(output.CodePtConfigResolveFailed,
			fmt.Sprintf("Patroni cluster scope is invalid in %s", DefaultConfigPath)).
			WithDetail(detail)
	default:
		return output.Fail(output.CodePtConfigResolveFailed,
			fmt.Sprintf("Failed to resolve Patroni cluster scope from %s", DefaultConfigPath)).
			WithDetail(detail)
	}
}

func validateResolvedClusterName(cluster string) error {
	trimmed := strings.TrimSpace(cluster)
	if trimmed == "" {
		return fmt.Errorf("%w in %s", errClusterScopeMissing, DefaultConfigPath)
	}
	if trimmed != cluster || strings.HasPrefix(trimmed, "-") {
		return fmt.Errorf("%w in %s: %q", errClusterScopeInvalid, DefaultConfigPath, cluster)
	}
	for _, r := range cluster {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_' || r == '.':
		default:
			return fmt.Errorf("%w in %s: %q", errClusterScopeInvalid, DefaultConfigPath, cluster)
		}
	}
	return nil
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
