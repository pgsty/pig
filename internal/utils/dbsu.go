package utils

import (
	"fmt"
	"os"
	"os/exec"
	"pig/internal/config"

	"github.com/sirupsen/logrus"
)

const DefaultDBSU = "postgres"

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
	return config.CurrentUser == dbsu
}

// DBSUCommand executes a command as the database superuser
// If current user is already DBSU, execute directly
// Otherwise use sudo -inu <dbsu> -- to switch user
func DBSUCommand(dbsu string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no command specified")
	}

	var cmd *exec.Cmd
	if IsDBSU(dbsu) {
		logrus.Debugf("executing as %s: %v", dbsu, args)
		cmd = exec.Command(args[0], args[1:]...)
	} else {
		// sudo -i: simulate login shell, load user environment
		// sudo -n: non-interactive (fail if password needed)
		// sudo -u: specify target user
		sudoArgs := append([]string{"-inu", dbsu, "--"}, args...)
		logrus.Debugf("executing via sudo: sudo %v", sudoArgs)
		cmd = exec.Command("sudo", sudoArgs...)
	}

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("command failed: %w", err)
	}
	return nil
}
