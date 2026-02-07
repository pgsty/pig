package cmd

import (
	"pig/cli/pgbackrest"
	"pig/internal/config"
	"pig/internal/output"

	"github.com/spf13/cobra"
)

// ============================================================================
// Backup Commands
// ============================================================================

var pbBackupForce bool

var pbBackupCmd = &cobra.Command{
	Use:     "backup [type]",
	Aliases: []string{"bk", "b"},
	Short:   "Create a backup",
	Annotations: mergeAnn(
		ancsAnn("pig pgbackrest backup", "action", "volatile", "unsafe", true, "low", "none", "dbsu", 300000),
		map[string]string{
			"args.type.desc": "backup type (auto-detected if omitted)",
			"args.type.type": "enum",
		},
	),
	Long: `Create a physical backup. Backup can only run on the primary instance.

Types:
  (empty) - Auto: pgBackRest determines type (full if none, else incr)
  full    - Full backup
  diff    - Differential backup (changes since last full)
  incr    - Incremental backup (changes since last backup)

The command automatically verifies the current instance is primary before
executing. Use --force to skip this check.`,
	Example: `
  pig pb backup                    # auto-detect type
  pig pb backup full               # full backup
  pig pb backup diff               # differential backup
  pig pb backup incr               # incremental backup
  pig pb backup -o json            # structured output mode`,
	RunE: func(cmd *cobra.Command, args []string) error {
		backupType := ""
		if len(args) > 0 {
			backupType = args[0]
		}
		opts := &pgbackrest.BackupOptions{
			Type:  backupType,
			Force: pbBackupForce,
		}

		// Structured output mode: use BackupResult
		if config.IsStructuredOutput() {
			result := pgbackrest.BackupResult(pbConfig, opts)
			return handleAuxResult(result)
		}

		// Text mode: use original Backup function
		return pgbackrest.Backup(pbConfig, opts)
	},
}

var pbExpireSet string
var pbExpireDryRun bool

var pbExpireCmd = &cobra.Command{
	Use:         "expire",
	Aliases:     []string{"ex", "e"},
	Short:       "Cleanup expired backups",
	Annotations: ancsAnn("pig pgbackrest expire", "action", "volatile", "restricted", true, "medium", "recommended", "dbsu", 30000),
	Long: `Clean up expired backups and WAL archives according to retention policy.

The retention policy is configured in pgbackrest.conf:
  repo1-retention-full     - Number of full backups to keep
  repo1-retention-diff     - Number of diff backups to keep
  repo1-retention-archive  - WAL archive retention policy`,
	Example: `
  pig pb expire                    # cleanup per policy
  pig pb expire --set 20250101-*   # delete specific backup
  pig pb expire --dry-run          # dry-run (show only)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModulePb, "pig pgbackrest expire", args, map[string]interface{}{
			"set":     pbExpireSet,
			"dry_run": pbExpireDryRun,
		}, func() error {
			return pgbackrest.Expire(pbConfig, &pgbackrest.ExpireOptions{
				Set:    pbExpireSet,
				DryRun: pbExpireDryRun,
			})
		})
	},
}

// ============================================================================
// Restore Command
// ============================================================================

var (
	pbRestoreDefault   bool
	pbRestoreImmediate bool
	pbRestoreTime      string
	pbRestoreName      string
	pbRestoreLSN       string
	pbRestoreXID       string
	pbRestoreSet       string
	pbRestoreDataDir   string
	pbRestoreExclusive bool
	pbRestorePromote   bool
	pbRestoreYes       bool
)

var pbRestoreCmd = &cobra.Command{
	Use:         "restore",
	Aliases:     []string{"rt", "r", "pitr"},
	Short:       "Restore from backup (PITR)",
	Annotations: ancsAnn("pig pgbackrest restore", "action", "volatile", "unsafe", false, "critical", "required", "dbsu", 600000),
	Long: `Restore from backup with point-in-time recovery (PITR) support.

IMPORTANT: You must specify a recovery target. Running without arguments
will show this help message to prevent accidental restores.

Recovery Targets (mutually exclusive, at least one required):
  --default, -d      Recover to end of WAL stream (latest data)
  --immediate, -I    Recover to backup consistency point only
  --time, -t         Recover to specific timestamp
  --name, -n         Recover to named restore point
  --lsn, -l          Recover to specific LSN
  --xid, -x          Recover to specific transaction ID

Backup Set Selection (can be combined with targets):
  --set, -b          Recover from specific backup set

Time Format:
  - Full: "2025-01-01 12:00:00+08"
  - Date only: "2025-01-01" (defaults to 00:00:00 in current timezone)
  - Time only: "12:00:00" (defaults to today in current timezone)

The restore process:
  1. Validates parameters and environment
  2. Verifies PostgreSQL is stopped
  3. Shows restore plan and waits for confirmation
  4. Executes pgbackrest restore
  5. Provides post-restore guidance

IMPORTANT: PostgreSQL must be stopped before restore.`,
	Example: `
  pig pb restore -d                # restore to latest (end of WAL)
  pig pb restore -I                # restore to consistency point
  pig pb restore -t "2025-01-01 12:00:00+08"   # restore to time
  pig pb restore -t "2025-01-01"   # restore to start of day
  pig pb restore -t "12:00:00"     # restore to time today
  pig pb restore -n my-savepoint   # restore to named point
  pig pb restore -l "0/7C82CB8"    # restore to LSN
  pig pb restore -b 20251225-120000F -d        # restore specific backup to latest

  # Options
  pig pb restore -d -X             # exclusive (stop before target)
  pig pb restore -d -P             # auto-promote after recovery
  pig pb restore -d -y             # skip confirmation
  pig pb restore -d -D /data/pg    # restore to custom data directory`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check if any recovery target is specified
		hasTarget := pbRestoreDefault || pbRestoreImmediate ||
			pbRestoreTime != "" || pbRestoreName != "" ||
			pbRestoreLSN != "" || pbRestoreXID != ""

		// If no target specified, structured mode returns machine-readable error.
		if !hasTarget {
			if config.IsStructuredOutput() {
				return handleAuxResult(
					output.Fail(output.CodePbInvalidRestoreParams, "invalid restore parameters").
						WithDetail("no recovery target specified, choose one of: --default, --immediate, --time, --name, --lsn, --xid"),
				)
			}
			return cmd.Help()
		}

		opts := &pgbackrest.RestoreOptions{
			Default:   pbRestoreDefault,
			Immediate: pbRestoreImmediate,
			Time:      pbRestoreTime,
			Name:      pbRestoreName,
			LSN:       pbRestoreLSN,
			XID:       pbRestoreXID,
			Set:       pbRestoreSet,
			DataDir:   pbRestoreDataDir,
			Exclusive: pbRestoreExclusive,
			Promote:   pbRestorePromote,
			Yes:       pbRestoreYes,
		}

		// Structured output mode: use RestoreResult
		if config.IsStructuredOutput() {
			// Structured mode implicitly skips confirmation (equivalent to --yes)
			opts.Yes = true
			result := pgbackrest.RestoreResult(pbConfig, opts)
			return handleAuxResult(result)
		}

		// Text mode: use original Restore function
		return pgbackrest.Restore(pbConfig, opts)
	},
}
