/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>

Systemctl operations with proper privilege escalation.
*/
package utils

import (
	"fmt"
	"os"
	"os/exec"
)

// RunSystemctl runs systemctl command as root (via sudo if needed).
// Returns ExitCodeError if the command exits with non-zero status.
func RunSystemctl(action, service string) error {
	cmdArgs := []string{"systemctl", action, service}
	PrintHint(cmdArgs)

	var cmd *exec.Cmd
	if os.Geteuid() == 0 {
		cmd = exec.Command(cmdArgs[0], cmdArgs[1:]...)
	} else {
		cmd = exec.Command("sudo", cmdArgs...)
	}

	configureCmdIO(cmd)

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return &ExitCodeError{Code: exitErr.ExitCode(), Err: err}
		}
		return fmt.Errorf("systemctl %s failed: %w", action, err)
	}
	return nil
}

// RunSystemctlQuiet runs systemctl command without printing hint.
// Returns ExitCodeError if the command exits with non-zero status.
func RunSystemctlQuiet(action, service string) error {
	cmdArgs := []string{"systemctl", action, service}

	var cmd *exec.Cmd
	if os.Geteuid() == 0 {
		cmd = exec.Command(cmdArgs[0], cmdArgs[1:]...)
	} else {
		cmd = exec.Command("sudo", cmdArgs...)
	}

	configureCmdIO(cmd)

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return &ExitCodeError{Code: exitErr.ExitCode(), Err: err}
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
