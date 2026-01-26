/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>
*/

package cmd

import (
	"pig/cli/pitr"

	"github.com/spf13/cobra"
)

// ============================================================================
// pig pitr - Orchestrated Point-In-Time Recovery
// ============================================================================

var pitrOpts *pitr.Options

var pitrCmd = &cobra.Command{
	Use:     "pitr",
	Short:   "Point-in-time recovery with cluster orchestration",
	GroupID: "pigsty",
	Long: `Perform PITR with automatic Patroni/PostgreSQL lifecycle management.

This command orchestrates a complete PITR workflow:
  1. Stop Patroni service (if running)
  2. Ensure PostgreSQL is stopped (with retry and fallback)
  3. Execute pgbackrest restore
  4. Start PostgreSQL
  5. Provide post-restore guidance

Recovery Targets (at least one required):
  --default, -d      Recover to end of WAL stream (latest)
  --immediate, -I    Recover to backup consistency point
  --time, -t         Recover to specific timestamp
  --name, -n         Recover to named restore point
  --lsn, -l          Recover to specific LSN
  --xid, -x          Recover to specific transaction ID

Time Format:
  - Full: "2025-01-01 12:00:00+08"
  - Date only: "2025-01-01" (defaults to 00:00:00)
  - Time only: "12:00:00" (defaults to today)

The command uses the same execution privilege strategy as other pig commands:
  - If running as DBSU (postgres): execute directly
  - If running as root: use "su - postgres -c"
  - Otherwise: use "sudo -inu postgres --"
`,
	Example: `
  # Recover to latest (most common)
  pig pitr -d

  # Recover to specific time
  pig pitr -t "2025-01-01 12:00:00+08"

  # Recover to date (00:00:00 of that day)
  pig pitr -t "2025-01-01"

  # Recover to backup consistency point
  pig pitr -I

  # Show execution plan without running
  pig pitr -d --dry-run

  # Skip confirmation (for automation)
  pig pitr -d -y

  # Skip Patroni management (standalone PostgreSQL)
  pig pitr -d --skip-patroni

  # Don't auto-start PostgreSQL after restore
  pig pitr -d --no-restart

  # Recover from specific backup set
  pig pitr -d -b 20241231-120000F

  # Exclusive recovery (stop before target)
  pig pitr -t "2025-01-01 12:00:00" -X

  # Auto-promote after recovery
  pig pitr -d -P`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return initAll()
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check if any target specified
		hasTarget := pitrOpts.Default || pitrOpts.Immediate ||
			pitrOpts.Time != "" || pitrOpts.Name != "" ||
			pitrOpts.LSN != "" || pitrOpts.XID != ""

		if !hasTarget {
			return cmd.Help()
		}

		return pitr.Execute(pitrOpts)
	},
}

func init() {
	pitrOpts = &pitr.Options{}

	// Recovery targets (mutually exclusive)
	pitrCmd.Flags().BoolVarP(&pitrOpts.Default, "default", "d", false, "recover to end of WAL stream (latest)")
	pitrCmd.Flags().BoolVarP(&pitrOpts.Immediate, "immediate", "I", false, "recover to backup consistency point")
	pitrCmd.Flags().StringVarP(&pitrOpts.Time, "time", "t", "", "recover to specific timestamp")
	pitrCmd.Flags().StringVarP(&pitrOpts.Name, "name", "n", "", "recover to named restore point")
	pitrCmd.Flags().StringVarP(&pitrOpts.LSN, "lsn", "l", "", "recover to specific LSN")
	pitrCmd.Flags().StringVarP(&pitrOpts.XID, "xid", "x", "", "recover to specific transaction ID")

	// Backup selection
	pitrCmd.Flags().StringVarP(&pitrOpts.Set, "set", "b", "", "recover from specific backup set")

	// PITR control
	pitrCmd.Flags().BoolVarP(&pitrOpts.SkipPatroni, "skip-patroni", "S", false, "skip Patroni stop operation")
	pitrCmd.Flags().BoolVarP(&pitrOpts.NoRestart, "no-restart", "N", false, "don't restart PostgreSQL after restore")
	pitrCmd.Flags().BoolVar(&pitrOpts.DryRun, "dry-run", false, "show execution plan without running")
	pitrCmd.Flags().BoolVarP(&pitrOpts.Yes, "yes", "y", false, "skip confirmation countdown")

	// Common flags (inherited from pgbackrest)
	pitrCmd.Flags().StringVarP(&pitrOpts.Stanza, "stanza", "s", "", "pgBackRest stanza name")
	pitrCmd.Flags().StringVarP(&pitrOpts.ConfigPath, "config", "c", "", "pgBackRest config file path")
	pitrCmd.Flags().StringVarP(&pitrOpts.Repo, "repo", "r", "", "repository number")
	pitrCmd.Flags().StringVarP(&pitrOpts.DbSU, "dbsu", "U", "", "database superuser (default: postgres)")
	pitrCmd.Flags().StringVarP(&pitrOpts.DataDir, "data", "D", "", "target data directory")
	pitrCmd.Flags().BoolVarP(&pitrOpts.Exclusive, "exclusive", "X", false, "stop before target (exclusive)")
	pitrCmd.Flags().BoolVarP(&pitrOpts.Promote, "promote", "P", false, "auto-promote after recovery")
}
