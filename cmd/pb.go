package cmd

import (
	"fmt"
	"pig/cli/pgbackrest"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"
	"strings"

	"github.com/spf13/cobra"
)

// ============================================================================
// pig pgbackrest (pb) - Manage pgBackRest backups and PITR
// ============================================================================

// Global config
var pbConfig *pgbackrest.Config

// pbCmd represents the pgbackrest command
var pbCmd = &cobra.Command{
	Use:         "pgbackrest",
	Short:       "Manage pgBackRest backup & restore",
	Aliases:     []string{"pb"},
	GroupID:     "pigsty",
	Annotations: ancsAnn("pig pgbackrest", "query", "stable", "safe", true, "safe", "none", "current", 100),
	Long: `pig pb - Manage pgBackRest backup and point-in-time recovery.

This command wraps pgbackrest to provide simplified backup management,
low-level restore primitives, and stanza lifecycle management.
All commands are executed as the database superuser (postgres by default).

Information:
  pig pb info                      show backup info
  pig pb list                      list backups
  pig pb list repo                 list configured repositories
  pig pb list stanza               list all stanzas

Backup & Restore:
  pig pb backup                    create backup (auto: full/incr)
  pig pb restore                   restore from backup (low-level primitive)
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
  pig pb list                      # list all backups
  pig pb list repo                 # list configured repositories
  pig pb list stanza               # list all stanzas

  # Backup (must run on primary)
  pig pb backup                    # auto: full if none, else incr
  pig pb backup full               # full backup
  pig pb backup incr               # incremental backup
  pig pb backup diff               # differential backup

  # Restore (low-level primitive; use pig pitr for orchestrated recovery)
  pig pb restore -d                # restore to latest (end of WAL)
  pig pb restore -I                # restore to consistency point
  pig pb restore -t "2025-01-01 12:00:00+08"  # restore to time
  pig pb restore -t "2025-01-01"   # restore to date (00:00:00)
  pig pb restore -t "12:00:00"     # restore to time today
  pig pb restore --name savepoint  # restore to named point
  pig pb restore --lsn "0/7C82CB8" # restore to LSN

  # Stanza management
  pig pb create                    # initialize stanza
  pig pb upgrade                   # upgrade after PG major upgrade
  pig pb check                     # verify repository

  # Cleanup
  pig pb expire                    # cleanup per retention policy
  pig pb expire --set 20250101-*   # delete specific backup
  pig pb expire --plan             # preview cleanup plan (recommended)`,
}

// ============================================================================
// Initialization
// ============================================================================

func registerPgBackRestCommand() *cobra.Command {
	// Initialize config
	pbConfig = pgbackrest.DefaultConfig()
	pbCmd.PersistentPreRunE = commandModulePreRun

	registerPbFlags()
	registerPbCommands()
	return pbCmd
}

func registerPbFlags() {
	// Global flags
	pbCmd.PersistentFlags().StringVarP(&pbConfig.Stanza, "stanza", "s", "", "pgBackRest stanza name (auto-detected if not specified)")
	pbCmd.PersistentFlags().StringVarP(&pbConfig.ConfigPath, "config", "c", "", "pgBackRest config file path")
	pbCmd.PersistentFlags().StringVarP(&pbConfig.Repo, "repo", "r", "", "repository number for multi-repo setups (1, 2, etc.)")
	pbCmd.PersistentFlags().StringVarP(&pbConfig.DbSU, "dbsu", "U", "", "database superuser (default: $PIG_DBSU or postgres)")

	// Info command flags
	pbInfoCmd.Flags().StringVar(&pbInfoRawOutput, "raw-output", "", "raw output format: text, json (only with --raw)")
	pbInfoCmd.Flags().StringVar(&pbInfoSet, "set", "", "show specific backup set")
	pbInfoCmd.Flags().BoolVarP(&pbInfoRaw, "raw", "R", false, "raw output mode (pass through pgbackrest output)")

	// Backup command flags
	pbBackupCmd.Flags().BoolVarP(&pbBackupForce, "force", "f", false, "skip primary role check")

	// Expire command flags
	pbExpireCmd.Flags().StringVar(&pbExpireSet, "set", "", "delete specific backup set")
	pbExpireCmd.Flags().BoolVar(&pbExpirePlan, "plan", false, "preview cleanup plan without deleting backups")
	pbExpireCmd.Flags().BoolVarP(&pbExpireYes, "yes", "y", false, "skip confirmation when --set deletes a backup set")

	// Restore command flags - targets
	pbRestoreCmd.Flags().BoolVarP(&pbRestoreDefault, "default", "d", false, "recover to end of WAL stream (latest)")
	pbRestoreCmd.Flags().BoolVarP(&pbRestoreImmediate, "immediate", "I", false, "recover to backup consistency point")
	pbRestoreCmd.Flags().StringVarP(&pbRestoreTime, "time", "t", "", "recover to specific timestamp")
	pbRestoreCmd.Flags().StringVar(&pbRestoreName, "name", "", "recover to named restore point")
	pbRestoreCmd.Flags().StringVar(&pbRestoreLSN, "lsn", "", "recover to specific LSN")
	pbRestoreCmd.Flags().StringVar(&pbRestoreXID, "xid", "", "recover to specific transaction ID")
	pbRestoreCmd.Flags().StringVarP(&pbRestoreSet, "set", "b", "", "select backup set to start recovery from")

	// Restore command flags - options
	pbRestoreCmd.Flags().StringVarP(&pbRestoreDataDir, "data", "D", "", "target data directory")
	pbRestoreCmd.Flags().BoolVarP(&pbRestoreExclusive, "exclusive", "X", false, "stop before target (exclusive)")
	pbRestoreCmd.Flags().StringVar(&pbRestoreTargetAction, "target-action", "", "action at recovery target: pause, promote, shutdown")
	pbRestoreCmd.Flags().StringVarP(&pbRestoreTargetTimeline, "target-timeline", "T", "", "recover along timeline: latest, current, N, or 0xN")
	pbRestoreCmd.Flags().BoolVarP(&pbRestoreYes, "yes", "y", false, "skip confirmation and countdown")
	pbRestoreCmd.Flags().BoolVar(&pbRestorePlan, "plan", false, "preview restore plan without executing")

	// Stanza management flags
	pbCreateCmd.Flags().BoolVar(&pbCreateNoOnline, "no-online", false, "create without PostgreSQL running")
	pbCreateCmd.Flags().BoolVarP(&pbCreateForce, "force", "f", false, "force creation")
	pbUpgradeCmd.Flags().BoolVar(&pbUpgradeNoOnline, "no-online", false, "upgrade without PostgreSQL running")
	pbDeleteCmd.Flags().BoolVarP(&pbDeleteYes, "yes", "y", false, "skip confirmation prompt")
	pbDeleteCmd.Flags().BoolVar(&pbDeletePlan, "plan", false, "preview stanza deletion plan without executing")

	// Control flags
	pbStopCmd.Flags().BoolVarP(&pbStopForce, "force", "f", false, "terminate running operations")

	// Log flags
	pbLogCmd.Flags().IntVarP(&pbLogLines, "lines", "n", 50, "number of lines to show")
	pbLogCmd.Flags().BoolVarP(&pbLogFollow, "follow", "f", false, "follow log output")
	pbLogTailCmd.Flags().IntVarP(&pbLogLines, "lines", "n", 50, "number of lines to show")
	pbLogTailCmd.Flags().BoolP("follow", "f", false, "(no-op: tail always follows)")
	pbLogCatCmd.Flags().IntVarP(&pbLogLines, "lines", "n", 50, "number of lines to show")
}

func registerPbCommands() {
	pbLogCmd.AddCommand(pbLogListCmd, pbLogTailCmd, pbLogCatCmd)

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

// ============================================================================
// Backup Commands
// ============================================================================

var pbBackupForce bool

var pbBackupCmd = &cobra.Command{
	Use:       "backup [type]",
	Aliases:   []string{"b"},
	Short:     "Create a backup",
	Args:      cobra.MaximumNArgs(1),
	ValidArgs: []string{"full", "diff", "incr"},
	// idempotent=false: every run creates a new backup set; retrying is safe
	// but not free (duration + repository space).
	Annotations: mergeAnn(
		ancsAnn("pig pgbackrest backup", "action", "volatile", "unsafe", false, "low", "none", "dbsu", 300000),
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
var pbExpirePlan bool
var pbExpireYes bool

var pbExpireCommandExec = pgbackrest.Expire

var pbExpireCmd = &cobra.Command{
	Use:         "expire",
	Aliases:     []string{"e"},
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
  pig pb expire --plan             # preview cleanup plan (recommended)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := &pgbackrest.ExpireOptions{
			Set:  pbExpireSet,
			Plan: pbExpirePlan,
		}
		if config.IsStructuredOutput() && pbExpirePlan {
			return handlePlanOutput(pgbackrest.BuildExpirePlan(pbConfig, opts))
		}
		if pbExpireSet != "" && !pbExpirePlan {
			if config.IsStructuredOutput() && !pbExpireYes {
				return requireStructuredConfirmation("pb",
					output.CodePbConfirmationRequired,
					"pb expire --set requires explicit confirmation",
					"expire", "pb:pgbackrest-only", "high",
					pgbackrest.ExpireCommand(pbConfig, opts, false, true),
					pgbackrest.ExpireCommand(pbConfig, opts, true, false),
				)
			}
			if err := requireTextHighRiskConfirmation(pbExpireYes,
				fmt.Sprintf("This will expire/delete pgBackRest backup set %s", pbExpireSet),
				"pb expire --set",
			); err != nil {
				return err
			}
		}
		return runLegacyStructured(legacyModulePb, "pig pgbackrest expire", args, map[string]interface{}{
			"set":  pbExpireSet,
			"plan": pbExpirePlan,
			"yes":  pbExpireYes,
		}, func() error {
			return pbExpireCommandExec(pbConfig, opts)
		})
	},
}

// ============================================================================
// Restore Command
// ============================================================================

var (
	pbRestoreDefault        bool
	pbRestoreImmediate      bool
	pbRestoreTime           string
	pbRestoreName           string
	pbRestoreLSN            string
	pbRestoreXID            string
	pbRestoreSet            string
	pbRestoreDataDir        string
	pbRestoreExclusive      bool
	pbRestoreTargetAction   string
	pbRestoreTargetTimeline string
	pbRestoreYes            bool
	pbRestorePlan           bool
)

var pbRestoreCmd = &cobra.Command{
	Use:         "restore",
	Aliases:     []string{"r"}, // B01: "rt" (=restart elsewhere) and "pitr" (=the orchestrator) removed
	Short:       "Restore from backup (low-level primitive)",
	Annotations: ancsAnn("pig pgbackrest restore", "action", "volatile", "unsafe", false, "critical", "required", "dbsu", 600000),
	Long: `Restore from backup with point-in-time recovery (PITR) support.
For orchestrated point-in-time recovery on a managed cluster, use pig pitr.

IMPORTANT: You must specify a recovery target. Running without arguments
will show this help message to prevent accidental restores.

Recovery Targets (mutually exclusive, at least one required):
  --default, -d      Recover to end of WAL stream (latest data)
  --immediate, -I    Recover to backup consistency point only
  --time, -t         Recover to specific timestamp
  --name             Recover to named restore point
  --lsn              Recover to specific LSN
  --xid              Recover to specific transaction ID

Backup Set Selection (can be combined with targets):
  --set, -b          Select backup set to start recovery from

Target Options:
  --target-action    Action when target is reached: pause, promote, shutdown
  --target-timeline  Recover along timeline: latest, current, N, or 0xN

Additional pgBackRest arguments:
  Put raw pgBackRest restore arguments after -- so Cobra stops parsing them.
  Example: pig pb restore -d -- --delta

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
  pig pb restore -d                         # restore to latest (end of WAL)
  pig pb restore -I                         # restore to consistency point
  pig pb restore -t "2025-01-01 12:00:00+08"  # restore to time
  pig pb restore -t "2025-01-01"            # restore to start of day
  pig pb restore -t "12:00:00"              # restore to time today
  pig pb restore --name my-savepoint        # restore to named point
  pig pb restore --lsn "0/7C82CB8"          # restore to LSN
  pig pb restore -b 20251225-120000F -d     # restore specific backup to latest

# Options

  pig pb restore -t "2025-01-01 12:00:00" -X  # exclusive (stop before target)
  pig pb restore -t "2025-01-01 12:00:00" --target-action=promote  # promote after reaching target
  pig pb restore -t "2025-01-01 12:00:00" -T current  # recover along current timeline
  pig pb restore -d -y                      # skip confirmation
  pig pb restore -d -D /data/pg             # restore to custom data directory
  pig pb restore -d -- --delta              # pass extra pgBackRest restore args after --`,
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
			if err := cmd.Help(); err != nil {
				return err
			}
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			return &utils.ExitCodeError{
				Code:   output.ExitCode(output.CodePbInvalidRestoreParams),
				Err:    fmt.Errorf("invalid or missing recovery target"),
				Silent: true,
			}
		}

		opts := &pgbackrest.RestoreOptions{
			Default:        pbRestoreDefault,
			Immediate:      pbRestoreImmediate,
			Time:           pbRestoreTime,
			Name:           pbRestoreName,
			LSN:            pbRestoreLSN,
			XID:            pbRestoreXID,
			Set:            pbRestoreSet,
			DataDir:        pbRestoreDataDir,
			Exclusive:      pbRestoreExclusive,
			TargetAction:   pbRestoreTargetAction,
			TargetTimeline: pbRestoreTargetTimeline,
			ExtraArgs:      append([]string(nil), args...),
			Yes:            pbRestoreYes,
		}

		if err := rejectRestoreExtraArgsBeforeDash(cmd, args, output.CodePbInvalidRestoreParams); err != nil {
			return err
		}
		if err := pgbackrest.ValidateRestoreOptions(opts); err != nil {
			return restoreInvalidParamsError(output.CodePbInvalidRestoreParams, err)
		}

		if pbRestorePlan {
			return handlePlanOutput(pgbackrest.BuildRestorePlan(pbConfig, opts))
		}

		// Structured output mode: use RestoreResult
		if config.IsStructuredOutput() {
			if !pbRestoreYes {
				return requireStructuredConfirmation("pb",
					output.CodePbConfirmationRequired,
					"pb restore requires explicit confirmation",
					"restore", "pb:pgbackrest-only", "critical",
					pgbackrest.RestoreCommand(pbConfig, opts, false, true),
					pgbackrest.RestoreCommand(pbConfig, opts, true, false),
					output.NextAction{Command: "pig pitr ... --plan", Reason: "use top-level recovery orchestration for managed PostgreSQL/Patroni clusters", Required: false},
				)
			}
			result := pgbackrest.RestoreResult(pbConfig, opts)
			return handleAuxResult(result)
		}

		// Text mode: use original Restore function
		return pgbackrest.Restore(pbConfig, opts)
	},
}

// ============================================================================
// Control Commands
// ============================================================================

var pbCheckCmd = &cobra.Command{
	Use:         "check",
	Aliases:     []string{"ck"},
	Short:       "Verify backup repository",
	Annotations: ancsAnn("pig pgbackrest check", "query", "volatile", "safe", true, "safe", "none", "dbsu", 10000),
	Long:        `Verify the backup repository integrity and configuration.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModulePb, "pig pgbackrest check", args, nil, func() error {
			return pgbackrest.Check(pbConfig)
		})
	},
}

var pbStartCmd = &cobra.Command{
	Use:         "start",
	Aliases:     []string{"up"},
	Short:       "Enable pgBackRest operations",
	Annotations: ancsAnn("pig pgbackrest start", "action", "volatile", "restricted", true, "low", "none", "dbsu", 1000),
	Long:        `Allow pgBackRest to perform operations on the stanza.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModulePb, "pig pgbackrest start", args, nil, func() error {
			return pgbackrest.Start(pbConfig)
		})
	},
}

var pbStopForce bool

var pbStopCmd = &cobra.Command{
	Use:         "stop",
	Aliases:     []string{"dw"},
	Short:       "Disable pgBackRest operations",
	Annotations: ancsAnn("pig pgbackrest stop", "action", "volatile", "restricted", true, "medium", "recommended", "dbsu", 1000),
	Long:        `Prevent pgBackRest from performing operations on the stanza (for maintenance).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModulePb, "pig pgbackrest stop", args, map[string]interface{}{
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

var (
	pbLogLines  int
	pbLogFollow bool
)

var pbLogCmd = &cobra.Command{
	Use:         "log",
	Aliases:     []string{"l"},
	Short:       "View pgBackRest logs",
	Annotations: ancsAnn("pig pgbackrest log", "query", "volatile", "safe", true, "safe", "none", "dbsu", 500),
	Long: `View pgBackRest log files from /pg/log/pgbackrest/.

Default action shows the latest log snapshot. Use -f/--follow or the tail
subcommand for real-time output.`,
	Example: `
	  pig pb log                       # show latest log lines
	  pig pb log -f                    # follow latest log
	  pig pb log list                  # list log files
	  pig pb log tail                  # follow latest log
	  pig pb log show                  # show latest log content`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateLogLines(pbLogLines); err != nil {
			return err
		}
		dbsu := pbConfig.DbSU
		if pbLogFollow {
			if config.IsStructuredOutput() {
				return structuredParamError(
					output.MODULE_PB,
					"pig pgbackrest log",
					"streaming log tail is not supported in structured output",
					"use 'pig pb log show -n N -o json' in structured mode to get a log snapshot",
					args,
					map[string]interface{}{"follow": pbLogFollow, "lines": pbLogLines},
				)
			}
			return pgbackrest.LogTail(pbConfig.ConfigPath, dbsu, "", pbLogLines)
		}

		if err := rejectUnsupportedLogOutputFormat("pig pb log"); err != nil {
			return err
		}
		if isJSONLogOutput() {
			return pgbackrest.LogShowJSONL(pbConfig.ConfigPath, dbsu, "", pbLogLines)
		}
		return runLegacyStructured(legacyModulePb, "pig pgbackrest log", args, map[string]interface{}{
			"follow": pbLogFollow,
			"lines":  pbLogLines,
		}, func() error {
			return pgbackrest.LogCat(pbConfig.ConfigPath, dbsu, "", pbLogLines)
		})
	},
}

var pbLogListCmd = &cobra.Command{
	Use:         "list",
	Aliases:     []string{"ls"},
	Short:       "List pgBackRest log files",
	Annotations: ancsAnn("pig pgbackrest log list", "query", "volatile", "safe", true, "safe", "none", "dbsu", 500),
	Args:        cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		dbsu := pbConfig.DbSU
		return runLegacyStructured(legacyModulePb, "pig pgbackrest log list", args, nil, func() error {
			return pgbackrest.LogList(pbConfig.ConfigPath, dbsu)
		})
	},
}

var pbLogTailCmd = &cobra.Command{
	Use:         "tail",
	Aliases:     []string{"t", "f", "follow"},
	Short:       "Tail latest pgBackRest log file",
	Annotations: ancsAnn("pig pgbackrest log tail", "query", "volatile", "safe", true, "safe", "none", "dbsu", 0),
	Args:        cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateLogLines(pbLogLines); err != nil {
			return err
		}
		if config.IsStructuredOutput() {
			return structuredParamError(
				output.MODULE_PB,
				"pig pgbackrest log tail",
				"streaming log tail is not supported in structured output",
				"use 'pig pb log show -n N -o json' in structured mode to get a log snapshot",
				args,
				map[string]interface{}{"lines": pbLogLines},
			)
		}
		return pgbackrest.LogTail(pbConfig.ConfigPath, pbConfig.DbSU, "", pbLogLines)
	},
}

var pbLogCatCmd = &cobra.Command{
	Use:         "show",
	Aliases:     []string{"cat", "c"},
	Short:       "Show latest pgBackRest log content",
	Annotations: ancsAnn("pig pgbackrest log show", "query", "volatile", "safe", true, "safe", "none", "dbsu", 500),
	Args:        cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateLogLines(pbLogLines); err != nil {
			return err
		}
		if err := rejectUnsupportedLogOutputFormat("pig pb log show"); err != nil {
			return err
		}
		if isJSONLogOutput() {
			return pgbackrest.LogShowJSONL(pbConfig.ConfigPath, pbConfig.DbSU, "", pbLogLines)
		}
		return runLegacyStructured(legacyModulePb, "pig pgbackrest log show", args, map[string]interface{}{
			"lines": pbLogLines,
		}, func() error {
			return pgbackrest.LogCat(pbConfig.ConfigPath, pbConfig.DbSU, "", pbLogLines)
		})
	},
}

// ============================================================================
// Info Commands
// ============================================================================

var pbInfoRawOutput string
var pbInfoSet string
var pbInfoRaw bool

var pbInfoCmd = &cobra.Command{
	Use:         "info",
	Aliases:     []string{"i"},
	Short:       "Show backup repository info",
	Annotations: ancsAnn("pig pgbackrest info", "query", "volatile", "safe", true, "safe", "none", "dbsu", 5000),
	Long: `Display detailed information about the backup repository including
all backup sets, recovery window, WAL archive status, and backup list.

By default, displays a parsed and formatted view of backup information including:
  - Recovery window (earliest to latest recovery point)
  - WAL archive range
  - LSN range
  - Backup list with type, duration, size, and WAL range

Use --raw/-R for original pgbackrest output format.
Use --raw-output to control raw output format (text/json).
Use -o json/yaml for structured output (Result wrapper with pgbackrest native JSON in data).`,
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
				if config.IsStructuredOutput() {
					return handleAuxResult(
						output.Fail(output.CodePbInvalidInfoParams, "invalid pb info raw parameters").
							WithDetail(err.Error()),
					)
				}
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
	Use:       "list [type]",
	Aliases:   []string{"ls"},
	Short:     "List backups, repositories, or stanzas",
	Args:      cobra.MaximumNArgs(1),
	ValidArgs: []string{"backup", "repo", "stanza"},
	Annotations: mergeAnn(
		ancsAnn("pig pgbackrest list", "query", "volatile", "safe", true, "safe", "none", "dbsu", 5000),
		map[string]string{
			"args.type.desc": "resource type to list",
			"args.type.type": "enum",
		},
	),
	Long: `List resources in the backup repository.

Types:
  backup  - List all backup sets (default)
  repo    - List configured repositories from config file
  stanza  - List all stanzas

Examples:
  pig pb list                      # list all backups
  pig pb list backup               # list all backups (explicit)
  pig pb list repo                 # list configured repositories
  pig pb list stanza               # list all stanzas`,
	RunE: func(cmd *cobra.Command, args []string) error {
		listType := ""
		if len(args) > 0 {
			listType = args[0]
		}
		opts := &pgbackrest.LsOptions{Type: listType}
		if config.IsStructuredOutput() {
			return handleAuxResult(pgbackrest.LsResult(pbConfig, opts))
		}
		return pgbackrest.Ls(pbConfig, opts)
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
// Stanza Management Commands
// ============================================================================

var pbCreateNoOnline bool
var pbCreateForce bool

var pbCreateCmd = &cobra.Command{
	Use:         "create",
	Aliases:     []string{"c"},
	Short:       "Create stanza (stanza-create)",
	Annotations: ancsAnn("pig pgbackrest create", "action", "stable", "unsafe", true, "low", "none", "dbsu", 5000),
	Long:        `Initialize a new stanza. Must be run before the first backup.`,
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
	Use:         "upgrade",
	Aliases:     []string{"u"},
	Short:       "Upgrade stanza (stanza-upgrade)",
	Annotations: ancsAnn("pig pgbackrest upgrade", "action", "stable", "unsafe", true, "low", "none", "dbsu", 5000),
	Long:        `Update stanza after PostgreSQL major version upgrade.`,
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

var pbDeleteYes bool
var pbDeletePlan bool

var pbDeleteCmd = &cobra.Command{
	Use:         "delete",
	Aliases:     []string{"d"},
	Short:       "Delete stanza (stanza-delete)",
	Annotations: ancsAnn("pig pgbackrest delete", "action", "volatile", "unsafe", false, "critical", "required", "dbsu", 5000),
	Long: `Delete a stanza and all its backups.

WARNING: This is a DESTRUCTIVE and IRREVERSIBLE operation!
All backups for the stanza will be permanently deleted.

Prompts for interactive confirmation unless --yes is provided.
When the config file defines multiple stanzas, --stanza must be given
explicitly: auto-detection never selects a deletion target.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := &pgbackrest.DeleteOptions{
			Yes: pbDeleteYes,
		}

		if pbDeletePlan {
			return handlePlanOutput(pgbackrest.BuildDeletePlan(pbConfig, opts))
		}

		if config.IsStructuredOutput() {
			// Ambiguity is checked before the confirmation gate so the gate
			// never suggests a delete command pinned to an auto-detected
			// stanza the user did not name.
			if stanzas, err := pgbackrest.RequireExplicitStanza(pbConfig); err != nil {
				return handleAuxResult(pgbackrest.AmbiguousStanzaResult(pbConfig, stanzas, err))
			}
			if !pbDeleteYes {
				return requireStructuredConfirmation("pb",
					output.CodePbConfirmationRequired,
					"pb delete requires explicit confirmation",
					"delete", "pb:pgbackrest-only", "critical",
					pgbackrest.DeleteCommand(pbConfig, false, true),
					pgbackrest.DeleteCommand(pbConfig, true, false),
					output.NextAction{Command: "pig pb info", Reason: "inspect backup inventory before deletion", Required: false},
				)
			}
			result := pgbackrest.DeleteResult(pbConfig, opts)
			return handleAuxResult(result)
		}

		return pgbackrest.Delete(pbConfig, opts)
	},
}
