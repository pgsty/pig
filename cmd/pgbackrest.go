/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>
*/

package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"pig/cli/postgres"

	"github.com/spf13/cobra"
)

// ============================================================================
// pig pgbackrest (pb) - Manage pgBackRest backups
// ============================================================================

var (
	pbStanza string
	pbConfig string
)

// pbCmd represents the pgbackrest command
var pbCmd = &cobra.Command{
	Use:     "pgbackrest",
	Short:   "Manage pgBackRest backup & restore",
	Aliases: []string{"pb", "pgbackup"},
	GroupID: "pigsty",
	Long: `Manage pgBackRest backup and point-in-time recovery.

This command wraps pgbackrest to provide easier backup management.
It automatically detects the configuration and forwards commands.

  pig pb info                      show backup info
  pig pb backup                    create backup
  pig pb restore                   restore from backup
  pig pb check                     verify backup integrity

Examples:
  pig pb info                      # show all backup info
  pig pb info --stanza=pg-meta     # show specific stanza
  pig pb backup --type=full        # create full backup
  pig pb backup --type=incr        # create incremental backup
  pig pb restore --target-time=... # point-in-time recovery
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check if pgbackrest exists
		pgbackrest, err := exec.LookPath("pgbackrest")
		if err != nil {
			return fmt.Errorf("pgbackrest not found in PATH (install with: pig ext add pgbackrest)")
		}

		if len(args) == 0 {
			cmd.Help()
			return nil
		}

		// Build pgbackrest command
		cmdArgs := []string{}
		if pbConfig != "" {
			cmdArgs = append(cmdArgs, "--config="+pbConfig)
		}
		if pbStanza != "" {
			cmdArgs = append(cmdArgs, "--stanza="+pbStanza)
		}
		cmdArgs = append(cmdArgs, args...)

		// Execute pgbackrest
		execCmd := exec.Command(pgbackrest, cmdArgs...)
		execCmd.Stdin = os.Stdin
		execCmd.Stdout = os.Stdout
		execCmd.Stderr = os.Stderr

		return execCmd.Run()
	},
}

// pbInfoCmd shows backup info
var pbInfoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show backup repository info",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPgBackRest("info", args)
	},
}

// pbBackupCmd creates a backup
var pbBackupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Create a backup",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPgBackRest("backup", args)
	},
}

// pbRestoreCmd restores from backup
var pbRestoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "Restore from backup",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPgBackRest("restore", args)
	},
}

// pbCheckCmd verifies backup
var pbCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Verify backup repository",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPgBackRest("check", args)
	},
}

// pbStanzaCreateCmd creates a stanza
var pbStanzaCreateCmd = &cobra.Command{
	Use:   "stanza-create",
	Short: "Create a stanza",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPgBackRest("stanza-create", args)
	},
}

// runPgBackRest runs pgbackrest with common options
func runPgBackRest(command string, extraArgs []string) error {
	pgbackrest, err := exec.LookPath("pgbackrest")
	if err != nil {
		return fmt.Errorf("pgbackrest not found in PATH (install with: pig ext add pgbackrest)")
	}

	cmdArgs := []string{}
	if pbConfig != "" {
		cmdArgs = append(cmdArgs, "--config="+pbConfig)
	}
	if pbStanza != "" {
		cmdArgs = append(cmdArgs, "--stanza="+pbStanza)
	}
	cmdArgs = append(cmdArgs, command)
	cmdArgs = append(cmdArgs, extraArgs...)

	postgres.PrintHint(append([]string{"pgbackrest"}, cmdArgs...))

	execCmd := exec.Command(pgbackrest, cmdArgs...)
	execCmd.Stdin = os.Stdin
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr

	return execCmd.Run()
}

func init() {
	// Global flags
	pbCmd.PersistentFlags().StringVar(&pbStanza, "stanza", "", "pgBackRest stanza name")
	pbCmd.PersistentFlags().StringVarP(&pbConfig, "config", "c", "", "pgBackRest config file")

	// Register subcommands
	pbCmd.AddCommand(pbInfoCmd)
	pbCmd.AddCommand(pbBackupCmd)
	pbCmd.AddCommand(pbRestoreCmd)
	pbCmd.AddCommand(pbCheckCmd)
	pbCmd.AddCommand(pbStanzaCreateCmd)
}
