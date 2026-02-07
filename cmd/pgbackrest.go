/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>
*/

package cmd

import (
	"fmt"
	"strings"

	"pig/cli/pgbackrest"
	"pig/internal/config"
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
// Info Commands
// ============================================================================

var pbInfoRawOutput string
var pbInfoSet string
var pbInfoRaw bool

var pbInfoCmd = &cobra.Command{
	Use:     "info",
	Aliases: []string{"i"},
	Short:   "Show backup repository info",
	Annotations: map[string]string{
		"name":       "pig pgbackrest info",
		"type":       "query",
		"volatility": "volatile",
		"parallel":   "safe",
		"idempotent": "true",
		"risk":       "safe",
		"confirm":    "none",
		"os_user":    "dbsu",
		"cost":       "5000",
	},
	Long: `Display detailed information about the backup repository including
all backup sets, recovery window, WAL archive status, and backup list.

By default, displays a parsed and formatted view of backup information including:
  - Recovery window (earliest to latest recovery point)
  - WAL archive range
  - LSN range
  - Backup list with type, duration, size, and WAL range

Use --raw/-R for original pgbackrest output format.
Use --raw-output/-O to control raw output format (text/json).
Use -o json/yaml for structured output (agent-friendly format).`,
	Example: `
  pig pb info                      # detailed formatted output
  pig pb info -o json              # structured JSON output
  pig pb info -o yaml              # structured YAML output
  pig pb info -R                   # raw pgbackrest text output
  pig pb info --raw --raw-output json  # raw JSON output (pgbackrest native)
  pig pb info --set 20250101-*     # show specific backup set`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// raw-output only applies in --raw mode
		if !pbInfoRaw && strings.TrimSpace(pbInfoRawOutput) != "" {
			if config.IsStructuredOutput() {
				return handleAuxResult(
					output.Fail(output.CodePbInvalidInfoParams, "--raw-output can only be used with --raw"),
				)
			}
			return fmt.Errorf("--raw-output can only be used with --raw")
		}

		// Raw mode: pass through to pgbackrest directly
		if pbInfoRaw {
			rawOutput, err := resolvePbInfoRawOutput()
			if err != nil {
				return err
			}
			return pgbackrest.Info(pbConfig, &pgbackrest.InfoOptions{
				Output: rawOutput,
				Set:    pbInfoSet,
				Raw:    true,
			})
		}

		// Structured output mode: use InfoResult
		if config.IsStructuredOutput() {
			result := pgbackrest.InfoResult(pbConfig, &pgbackrest.InfoOptions{
				Set: pbInfoSet,
			})
			return handleAuxResult(result)
		}

		// Text mode: use original Info function
		return pgbackrest.Info(pbConfig, &pgbackrest.InfoOptions{
			Set: pbInfoSet,
			Raw: false,
		})
	},
}

var pbLsCmd = &cobra.Command{
	Use:     "ls [type]",
	Aliases: []string{"l", "list"},
	Short:   "List backups, repositories, or stanzas",
	Annotations: map[string]string{
		"name":       "pig pgbackrest ls",
		"type":       "query",
		"volatility": "volatile",
		"parallel":   "safe",
		"idempotent": "true",
		"risk":       "safe",
		"confirm":    "none",
		"os_user":    "dbsu",
		"cost":       "5000",
		// Parameter constraints
		"args.type.desc": "resource type to list",
		"args.type.type": "enum",
	},
	Long: `List resources in the backup repository.

Types:
  backup  - List all backup sets (default)
  repo    - List configured repositories from config file
  stanza  - List all stanzas (aliases: cluster, cls)

Examples:
  pig pb ls                        # list all backups
  pig pb ls backup                 # list all backups (explicit)
  pig pb ls repo                   # list configured repositories
  pig pb ls stanza                 # list all stanzas`,
	RunE: func(cmd *cobra.Command, args []string) error {
		listType := ""
		if len(args) > 0 {
			listType = args[0]
		}
		return runPbLegacy("pig pgbackrest ls", args, map[string]interface{}{
			"type": listType,
		}, func() error {
			return pgbackrest.Ls(pbConfig, &pgbackrest.LsOptions{
				Type: listType,
			})
		})
	},
}

// ============================================================================
// Backup Commands
// ============================================================================

var pbBackupForce bool

var pbBackupCmd = &cobra.Command{
	Use:     "backup [type]",
	Aliases: []string{"bk", "b"},
	Short:   "Create a backup",
	Annotations: map[string]string{
		"name":       "pig pgbackrest backup",
		"type":       "action",
		"volatility": "volatile",
		"parallel":   "unsafe",
		"idempotent": "true",
		"risk":       "low",
		"confirm":    "none",
		"os_user":    "dbsu",
		"cost":       "300000",
		// Parameter constraints
		"args.type.desc": "backup type (auto-detected if omitted)",
		"args.type.type": "enum",
	},
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
	Use:     "expire",
	Aliases: []string{"ex", "e"},
	Short:   "Cleanup expired backups",
	Annotations: map[string]string{
		"name":       "pig pgbackrest expire",
		"type":       "action",
		"volatility": "volatile",
		"parallel":   "restricted",
		"idempotent": "true",
		"risk":       "medium",
		"confirm":    "recommended",
		"os_user":    "dbsu",
		"cost":       "30000",
	},
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
		return runPbLegacy("pig pgbackrest expire", args, map[string]interface{}{
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
	Use:     "restore",
	Aliases: []string{"rt", "r", "pitr"},
	Short:   "Restore from backup (PITR)",
	Annotations: map[string]string{
		"name":       "pig pgbackrest restore",
		"type":       "action",
		"volatility": "volatile",
		"parallel":   "unsafe",
		"idempotent": "false",
		"risk":       "critical",
		"confirm":    "required",
		"os_user":    "dbsu",
		"cost":       "600000",
	},
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

// ============================================================================
// Stanza Management Commands
// ============================================================================

var pbCreateNoOnline bool
var pbCreateForce bool

var pbCreateCmd = &cobra.Command{
	Use:     "create",
	Aliases: []string{"cr"},
	Short:   "Create stanza (stanza-create)",
	Annotations: map[string]string{
		"name":       "pig pgbackrest create",
		"type":       "action",
		"volatility": "stable",
		"parallel":   "unsafe",
		"idempotent": "true",
		"risk":       "low",
		"confirm":    "none",
		"os_user":    "dbsu",
		"cost":       "5000",
	},
	Long: `Initialize a new stanza. Must be run before the first backup.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := &pgbackrest.CreateOptions{
			NoOnline: pbCreateNoOnline,
			Force:    pbCreateForce,
		}

		if config.IsStructuredOutput() {
			result := pgbackrest.CreateResult(pbConfig, opts)
			return handleAuxResult(result)
		}

		return pgbackrest.Create(pbConfig, opts)
	},
}

var pbUpgradeNoOnline bool

var pbUpgradeCmd = &cobra.Command{
	Use:     "upgrade",
	Aliases: []string{"up"},
	Short:   "Upgrade stanza (stanza-upgrade)",
	Annotations: map[string]string{
		"name":       "pig pgbackrest upgrade",
		"type":       "action",
		"volatility": "stable",
		"parallel":   "unsafe",
		"idempotent": "true",
		"risk":       "low",
		"confirm":    "none",
		"os_user":    "dbsu",
		"cost":       "5000",
	},
	Long: `Update stanza after PostgreSQL major version upgrade.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := &pgbackrest.UpgradeOptions{
			NoOnline: pbUpgradeNoOnline,
		}

		if config.IsStructuredOutput() {
			result := pgbackrest.UpgradeResult(pbConfig, opts)
			return handleAuxResult(result)
		}

		return pgbackrest.Upgrade(pbConfig, opts)
	},
}

var pbDeleteForce bool
var pbDeleteYes bool

var pbDeleteCmd = &cobra.Command{
	Use:     "delete",
	Aliases: []string{"del", "rm"},
	Short:   "Delete stanza (stanza-delete)",
	Annotations: map[string]string{
		"name":       "pig pgbackrest delete",
		"type":       "action",
		"volatility": "volatile",
		"parallel":   "unsafe",
		"idempotent": "false",
		"risk":       "critical",
		"confirm":    "required",
		"os_user":    "dbsu",
		"cost":       "5000",
	},
	Long: `Delete a stanza and all its backups.

WARNING: This is a DESTRUCTIVE and IRREVERSIBLE operation!
All backups for the stanza will be permanently deleted.

	Requires --force flag to confirm the operation.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := &pgbackrest.DeleteOptions{
			Force: pbDeleteForce,
			Yes:   pbDeleteYes,
		}

		if config.IsStructuredOutput() {
			// Structured mode implicitly skips confirmation (equivalent to --yes)
			opts.Yes = true
			result := pgbackrest.DeleteResult(pbConfig, opts)
			return handleAuxResult(result)
		}

		return pgbackrest.Delete(pbConfig, opts)
	},
}

// ============================================================================
// Control Commands
// ============================================================================

var pbCheckCmd = &cobra.Command{
	Use:     "check",
	Aliases: []string{"ck"},
	Short:   "Verify backup repository",
	Annotations: map[string]string{
		"name":       "pig pgbackrest check",
		"type":       "query",
		"volatility": "volatile",
		"parallel":   "safe",
		"idempotent": "true",
		"risk":       "safe",
		"confirm":    "none",
		"os_user":    "dbsu",
		"cost":       "10000",
	},
	Long: `Verify the backup repository integrity and configuration.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPbLegacy("pig pgbackrest check", args, nil, func() error {
			return pgbackrest.Check(pbConfig)
		})
	},
}

var pbStartCmd = &cobra.Command{
	Use:     "start",
	Aliases: []string{"on"},
	Short:   "Enable pgBackRest operations",
	Annotations: map[string]string{
		"name":       "pig pgbackrest start",
		"type":       "action",
		"volatility": "volatile",
		"parallel":   "restricted",
		"idempotent": "true",
		"risk":       "low",
		"confirm":    "none",
		"os_user":    "dbsu",
		"cost":       "1000",
	},
	Long: `Allow pgBackRest to perform operations on the stanza.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPbLegacy("pig pgbackrest start", args, nil, func() error {
			return pgbackrest.Start(pbConfig)
		})
	},
}

var pbStopForce bool

var pbStopCmd = &cobra.Command{
	Use:     "stop",
	Aliases: []string{"off"},
	Short:   "Disable pgBackRest operations",
	Annotations: map[string]string{
		"name":       "pig pgbackrest stop",
		"type":       "action",
		"volatility": "volatile",
		"parallel":   "restricted",
		"idempotent": "true",
		"risk":       "medium",
		"confirm":    "recommended",
		"os_user":    "dbsu",
		"cost":       "1000",
	},
	Long: `Prevent pgBackRest from performing operations on the stanza (for maintenance).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPbLegacy("pig pgbackrest stop", args, map[string]interface{}{
			"force": pbStopForce,
		}, func() error {
			return pgbackrest.Stop(pbConfig, &pgbackrest.StopOptions{
				Force: pbStopForce,
			})
		})
	},
}

// ============================================================================
// Log Commands
// ============================================================================

var pbLogLines int

var pbLogCmd = &cobra.Command{
	Use:     "log [list|tail|cat]",
	Aliases: []string{"l", "lg"},
	Short:   "View pgBackRest logs",
	Annotations: map[string]string{
		"name":       "pig pgbackrest log",
		"type":       "query",
		"volatility": "volatile",
		"parallel":   "safe",
		"idempotent": "true",
		"risk":       "safe",
		"confirm":    "none",
		"os_user":    "dbsu",
		"cost":       "500",
	},
	Long: `View pgBackRest log files from /pg/log/pgbackrest/.

Subcommands:
  list  - List available log files (default)
  tail  - Follow latest log file in real-time
  cat   - Display log file contents`,
	Example: `
  pig pb log                       # list log files
  pig pb log list                  # list log files
  pig pb log tail                  # follow latest log
  pig pb log cat                   # show latest log content`,
	RunE: func(cmd *cobra.Command, args []string) error {
		subCmd := "list"
		if len(args) > 0 {
			subCmd = args[0]
		}

		dbsu := pbConfig.DbSU
		if config.IsStructuredOutput() && (subCmd == "tail" || subCmd == "follow" || subCmd == "f") {
			return structuredParamError(
				output.MODULE_PB,
				"pig pgbackrest log",
				"streaming log tail is not supported in structured output",
				"use 'pig pb log cat' in structured mode to get a log snapshot",
				args,
				map[string]interface{}{"subcommand": subCmd},
			)
		}

		return runPbLegacy("pig pgbackrest log", args, map[string]interface{}{
			"subcommand": subCmd,
			"lines":      pbLogLines,
		}, func() error {
			switch subCmd {
			case "list", "ls":
				return pgbackrest.LogList(dbsu)
			case "tail", "follow", "f":
				return pgbackrest.LogTail(dbsu, pbLogLines)
			case "cat", "show":
				filename := ""
				if len(args) > 1 {
					filename = args[1]
				}
				return pgbackrest.LogCat(dbsu, filename, pbLogLines)
			default:
				return pgbackrest.LogList(dbsu)
			}
		})
	},
}

func resolvePbInfoRawOutput() (string, error) {
	if out := strings.ToLower(strings.TrimSpace(pbInfoRawOutput)); out != "" {
		switch out {
		case "text", "json":
			return out, nil
		default:
			return "", fmt.Errorf("invalid --raw-output value %q, must be text or json", pbInfoRawOutput)
		}
	}

	if !config.IsStructuredOutput() {
		return "", nil
	}
	switch config.OutputFormat {
	case config.OUTPUT_JSON, config.OUTPUT_JSON_PRETTY:
		return "json", nil
	case config.OUTPUT_YAML:
		return "", fmt.Errorf("raw mode does not support YAML output, use JSON or text")
	default:
		return "", nil
	}
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
