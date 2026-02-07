/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>
*/

package cmd

import (
	"pig/cli/pgbackrest"
	"pig/internal/output"

	"github.com/spf13/cobra"
)

// ============================================================================
// pig pgbackrest (pb) - Manage pgBackRest backups and PITR
// ============================================================================

// Global config
var pbConfig *pgbackrest.Config

func runPbLegacy(command string, args []string, params map[string]interface{}, fn func() error) error {
	return runLegacyStructured(output.MODULE_PB, command, args, params, fn)
}

// pbCmd represents the pgbackrest command
var pbCmd = &cobra.Command{
	Use:     "pgbackrest",
	Short:   "Manage pgBackRest backup & restore",
	Aliases: []string{"pb"},
	GroupID: "pigsty",
	Annotations: map[string]string{
		"name":       "pig pgbackrest",
		"type":       "query",
		"volatility": "stable",
		"parallel":   "safe",
		"idempotent": "true",
		"risk":       "safe",
		"confirm":    "none",
		"os_user":    "current",
		"cost":       "100",
	},
	Long: `Manage pgBackRest backup and point-in-time recovery.

This command wraps pgbackrest to provide simplified backup management,
PITR (point-in-time recovery), and stanza lifecycle management.
All commands are executed as the database superuser (postgres by default).

Information:
  pig pb info                      show backup info
  pig pb ls                        list backups
  pig pb ls repo                   list configured repositories
  pig pb ls stanza                 list all stanzas

Backup & Restore:
  pig pb backup                    create backup (auto: full/incr)
  pig pb backup full               create full backup
  pig pb restore                   restore from backup (PITR)
  pig pb restore -t "..."          restore to specific time
  pig pb expire                    cleanup expired backups

Stanza Management:
  pig pb create                    create stanza (first-time setup)
  pig pb upgrade                   upgrade stanza (after PG upgrade)
  pig pb delete                    delete stanza (DANGEROUS!)

Control:
  pig pb check                     verify backup integrity
  pig pb start                     enable pgBackRest operations
  pig pb stop                      disable pgBackRest operations
  pig pb log                       view pgBackRest logs
`,
	Example: `
  # Information
  pig pb info                      # show all backup info
  pig pb info -o json              # JSON format output
  pig pb ls                        # list all backups
  pig pb ls repo                   # list configured repositories
  pig pb ls stanza                 # list all stanzas

  # Backup (must run on primary)
  pig pb backup                    # auto: full if none, else incr
  pig pb backup full               # full backup
  pig pb backup incr               # incremental backup

  # Restore / PITR
  pig pb restore                   # restore to latest (default)
  pig pb restore -I                # restore to consistency point
  pig pb restore -t "2025-01-01 12:00:00+08"  # restore to time
  pig pb restore -t "2025-01-01"   # restore to date (00:00:00)
  pig pb restore -t "12:00:00"     # restore to time today
  pig pb restore -n savepoint      # restore to named point
  pig pb restore -l "0/7C82CB8"    # restore to LSN

  # Stanza management
  pig pb create                    # initialize stanza
  pig pb upgrade                   # upgrade after PG major upgrade
  pig pb check                     # verify repository

  # Cleanup
  pig pb expire                    # cleanup per retention policy
  pig pb expire --set 20250101-*   # delete specific backup
  pig pb expire --dry-run          # dry-run mode`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if err := initAll(); err != nil {
			return err
		}
		applyStructuredOutputSilence(cmd)
		return nil
	},
}

// ============================================================================
// Initialization
// ============================================================================

func init() {
	// Initialize config
	pbConfig = pgbackrest.DefaultConfig()

	registerPbFlags()
	registerPbCommands()
}

func registerPbFlags() {
	// Global flags
	pbCmd.PersistentFlags().StringVarP(&pbConfig.Stanza, "stanza", "s", "", "pgBackRest stanza name (auto-detected if not specified)")
	pbCmd.PersistentFlags().StringVarP(&pbConfig.ConfigPath, "config", "c", "", "pgBackRest config file path")
	pbCmd.PersistentFlags().StringVarP(&pbConfig.Repo, "repo", "r", "", "repository number for multi-repo setups (1, 2, etc.)")
	pbCmd.PersistentFlags().StringVarP(&pbConfig.DbSU, "dbsu", "U", "", "database superuser (default: $PIG_DBSU or postgres)")

	// Info command flags
	pbInfoCmd.Flags().StringVarP(&pbInfoRawOutput, "raw-output", "O", "", "raw output format: text, json (only with --raw)")
	pbInfoCmd.Flags().StringVar(&pbInfoSet, "set", "", "show specific backup set")
	pbInfoCmd.Flags().BoolVarP(&pbInfoRaw, "raw", "R", false, "raw output mode (pass through pgbackrest output)")

	// Backup command flags
	pbBackupCmd.Flags().BoolVarP(&pbBackupForce, "force", "f", false, "skip primary role check")

	// Expire command flags
	pbExpireCmd.Flags().StringVar(&pbExpireSet, "set", "", "delete specific backup set")
	pbExpireCmd.Flags().BoolVar(&pbExpireDryRun, "dry-run", false, "dry-run mode: show only")

	// Restore command flags - targets
	pbRestoreCmd.Flags().BoolVarP(&pbRestoreDefault, "default", "d", false, "recover to end of WAL stream (latest)")
	pbRestoreCmd.Flags().BoolVarP(&pbRestoreImmediate, "immediate", "I", false, "recover to backup consistency point")
	pbRestoreCmd.Flags().StringVarP(&pbRestoreTime, "time", "t", "", "recover to specific timestamp")
	pbRestoreCmd.Flags().StringVarP(&pbRestoreName, "name", "n", "", "recover to named restore point")
	pbRestoreCmd.Flags().StringVarP(&pbRestoreLSN, "lsn", "l", "", "recover to specific LSN")
	pbRestoreCmd.Flags().StringVarP(&pbRestoreXID, "xid", "x", "", "recover to specific transaction ID")
	pbRestoreCmd.Flags().StringVarP(&pbRestoreSet, "set", "b", "", "recover from specific backup set")

	// Restore command flags - options
	pbRestoreCmd.Flags().StringVarP(&pbRestoreDataDir, "data", "D", "", "target data directory")
	pbRestoreCmd.Flags().BoolVarP(&pbRestoreExclusive, "exclusive", "X", false, "stop before target (exclusive)")
	pbRestoreCmd.Flags().BoolVarP(&pbRestorePromote, "promote", "P", false, "auto-promote after recovery")
	pbRestoreCmd.Flags().BoolVarP(&pbRestoreYes, "yes", "y", false, "skip confirmation and countdown")

	// Stanza management flags
	pbCreateCmd.Flags().BoolVar(&pbCreateNoOnline, "no-online", false, "create without PostgreSQL running")
	pbCreateCmd.Flags().BoolVarP(&pbCreateForce, "force", "f", false, "force creation")
	pbUpgradeCmd.Flags().BoolVar(&pbUpgradeNoOnline, "no-online", false, "upgrade without PostgreSQL running")
	pbDeleteCmd.Flags().BoolVarP(&pbDeleteForce, "force", "f", false, "confirm deletion (required)")
	pbDeleteCmd.Flags().BoolVarP(&pbDeleteYes, "yes", "y", false, "skip countdown confirmation")

	// Control flags
	pbStopCmd.Flags().BoolVarP(&pbStopForce, "force", "f", false, "terminate running operations")

	// Log flags
	pbLogCmd.Flags().IntVarP(&pbLogLines, "lines", "n", 50, "number of lines to show")
}

func registerPbCommands() {
	// Register all subcommands
	pbCmd.AddCommand(
		// Information
		pbInfoCmd,
		pbLsCmd,

		// Backup & Restore
		pbBackupCmd,
		pbRestoreCmd,
		pbExpireCmd,

		// Stanza management
		pbCreateCmd,
		pbUpgradeCmd,
		pbDeleteCmd,

		// Control
		pbCheckCmd,
		pbStartCmd,
		pbStopCmd,

		// Logs
		pbLogCmd,
	)
}
