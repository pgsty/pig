/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

Systemctl operations with proper privilege escalation.
*/
package utils

import (
	"fmt"
	"os"
	"os/exec"
)

// RunSystemctl runs systemctl command as root (via sudo if needed).
// The read-only status action runs unprivileged so it works without sudo rights.
// Returns ExitCodeError if the command exits with non-zero status.
func RunSystemctl(action, service string) error {
	cmdArgs := []string{"systemctl", action, service}
	if action == "status" {
		cmdArgs = append(cmdArgs, "--no-pager", "-l")
	}
	PrintHint(cmdArgs)

	var cmd *exec.Cmd
	if os.Geteuid() == 0 || action == "status" {
		cmd = exec.Command(cmdArgs[0], cmdArgs[1:]...)
	} else {
		cmd = exec.Command("sudo", cmdArgs...)
	}

	configureCmdIO(cmd)

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// The status action is a read-only query: systemctl reports the
			// unit state via the exit code (3 = inactive/dead, 4 = not found),
			// which is not a command failure. Preserve the exit code for
			// scripting but mark it silent so we don't dump cobra usage or an
			// alarming "command execution failed" log for a stopped service.
			return &ExitCodeError{Code: exitErr.ExitCode(), Err: err, Silent: action == "status"}
		}
		return fmt.Errorf("systemctl %s failed: %w", action, err)
	}
	return nil
}

// IsServiceActive checks if a systemd service is active.
// Returns true if active, false otherwise.
func IsServiceActive(service string) bool {
	cmd := exec.Command("systemctl", "is-active", service)
	err := cmd.Run()
	return err == nil
}
