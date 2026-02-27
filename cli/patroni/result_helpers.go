/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

Shared helpers for patroni structured outputs.
*/
package patroni

import "strings"

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
		(strings.Contains(msg, "cannot") && strings.Contains(msg, "config") && strings.Contains(msg, "file")) ||
		(strings.Contains(msg, "could not") && strings.Contains(msg, "config"))
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

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
