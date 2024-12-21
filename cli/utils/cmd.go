package utils

import (
	"fmt"
	"os"
	"os/exec"
	"pig/internal/config"
)

var (

	// TrySudo is a flag to try to run a command with sudo
	TrySudo = false
)

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

// SudoCommand runs a command with sudo if the current user is not root
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
