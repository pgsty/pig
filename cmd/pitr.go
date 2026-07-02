/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>
*/

package cmd

import (
	"fmt"

	"pig/cli/pitr"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"

	"github.com/spf13/cobra"
)

// ============================================================================
// pig pitr - Orchestrated Point-In-Time Recovery
// ============================================================================

var pitrOpts *pitr.Options

var pitrCmd = &cobra.Command{
	Use:     "pitr",
	Short:   "Point-in-time recovery using pgBackRest",
	GroupID: "pigsty",
	Long: `Perform PITR with pgBackRest restore and conservative PostgreSQL stop/start handling.

For the managed default data directory, this command may:
  1. Stop Patroni only to keep the target PGDATA offline during restore
  2. Ensure PostgreSQL is stopped (with retry and fallback)
  3. Execute pgbackrest restore
  4. Start PostgreSQL
  5. Provide post-restore guidance

Patroni is left stopped after a managed-data-dir PITR. Validate the
restored database first, then resume Patroni outside this command.
This command does not rejoin Patroni, perform failover, or validate
cluster membership after restore.

Custom -D side restores require --no-restart. Restored PostgreSQL
configuration keeps the original port, so start the side restore manually
with pg_ctl -D <dir> -o "-p <free-port>" start.

Recovery Targets (at least one required):
  --default, -d      Recover to end of WAL stream (latest)
  --immediate, -I    Recover to backup consistency point
  --time, -t         Recover to specific timestamp
  --name             Recover to named restore point
  --lsn              Recover to specific LSN
  --xid              Recover to specific transaction ID

Backup and Target Options:
  --set, -b          Select backup set to start recovery from
  --target-action    Action when target is reached: pause, promote, shutdown
  --target-timeline  Recover along timeline: latest, current, N, or 0xN

Use --no-restart with --target-action=shutdown because PostgreSQL exits
after reaching the recovery target.

Additional pgBackRest arguments:
  Put raw pgBackRest restore arguments after -- so Cobra stops parsing them.
  Example: pig pitr -d -- --delta

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
  pig pitr -d --plan

  # Skip destructive confirmation (for automation)
  pig pitr -d -y

  # Side-restore to a custom data dir without touching Patroni or /pg/data
  pig pitr -d -D /tmp/pg-restore --no-restart

  # Restore the managed data dir, but leave PostgreSQL and Patroni stopped
  pig pitr -d --no-restart

  # Recover from specific backup set
  pig pitr -d -b 20241231-120000F

  # Recover along the current timeline
  pig pitr -t "2025-01-01 12:00:00" -T current

  # Exclusive recovery (stop before target)
  pig pitr -t "2025-01-01 12:00:00" -X

  # Auto-promote after reaching a manual recovery target
  pig pitr -t "2025-01-01 12:00:00" --target-action=promote

  # Pass extra pgBackRest restore args after --
  pig pitr -d -- --delta`,
	Annotations: ancsAnn("pig pitr", "action", "volatile", "unsafe", false, "critical", "required", "root", 600000),
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if err := initAll(); err != nil {
			return err
		}
		applyStructuredOutputSilence(cmd)
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		pitrOpts.ExtraArgs = append([]string(nil), args...)

		// Check if any target specified
		hasTarget := pitrOpts.Default || pitrOpts.Immediate ||
			pitrOpts.Time != "" || pitrOpts.Name != "" ||
			pitrOpts.LSN != "" || pitrOpts.XID != ""

		if !hasTarget {
			if config.IsStructuredOutput() {
				return handleAuxResult(
					output.Fail(output.CodePITRInvalidArgs, "invalid or missing recovery target").
						WithDetail("choose one of: --default, --immediate, --time, --name, --lsn, --xid"),
				)
			}
			if err := cmd.Help(); err != nil {
				return err
			}
			return &utils.ExitCodeError{
				Code: output.ExitCode(output.CodePITRInvalidArgs),
				Err:  &pitr.PITRError{Code: output.CodePITRInvalidArgs, Err: fmt.Errorf("invalid or missing recovery target")},
			}
		}
		if err := rejectRestoreExtraArgsBeforeDash(cmd, args, output.CodePITRInvalidArgs); err != nil {
			return err
		}
		if err := pitr.ValidateOptions(pitrOpts); err != nil {
			return restoreInvalidParamsError(output.CodePITRInvalidArgs, err)
		}

		// Plan mode: show plan and exit
		if pitrOpts.Plan {
			plan, err := pitr.Plan(pitrOpts)
			if err != nil {
				return err
			}
			return output.RenderPlan(plan)
		}

		// Structured output: return Result
		if config.IsStructuredOutput() {
			if !pitrOpts.Yes {
				return structuredConfirmationError(
					output.CodePITRConfirmationRequired,
					"pitr requires explicit confirmation",
					"structured output mode does not prompt interactively; rerun with --yes to execute or --plan to preview",
					output.OperationMeta{
						Module:       "pitr",
						Command:      "pitr",
						Boundary:     "pitr:managed-recovery",
						Risk:         "critical",
						Confirmation: "required",
						Executed:     false,
						DryRun:       false,
					},
					[]output.NextAction{
						{Command: "pig pitr ... --yes", Reason: "execute managed PITR after explicit confirmation", Required: true},
						{Command: "pig pitr ... --plan", Reason: "preview managed PITR prechecks and lifecycle steps", Required: false},
						{Command: "pig pb restore ... --plan", Reason: "preview the low-level pgBackRest restore primitive", Required: false},
					},
				)
			}
			preparePITRStructuredOptions(pitrOpts)
			result := pitr.ExecuteResult(pitrOpts)
			return handleAuxResult(result)
		}

		// Text output: keep existing behavior
		return pitr.Execute(pitrOpts)
	},
}

func preparePITRStructuredOptions(opts *pitr.Options) {
	if opts != nil {
		opts.Quiet = true
	}
}

func init() {
	pitrOpts = &pitr.Options{}

	// Recovery targets (mutually exclusive)
	pitrCmd.Flags().BoolVarP(&pitrOpts.Default, "default", "d", false, "recover to end of WAL stream (latest)")
	pitrCmd.Flags().BoolVarP(&pitrOpts.Immediate, "immediate", "I", false, "recover to backup consistency point")
	pitrCmd.Flags().StringVarP(&pitrOpts.Time, "time", "t", "", "recover to specific timestamp")
	pitrCmd.Flags().StringVar(&pitrOpts.Name, "name", "", "recover to named restore point")
	pitrCmd.Flags().StringVar(&pitrOpts.LSN, "lsn", "", "recover to specific LSN")
	pitrCmd.Flags().StringVar(&pitrOpts.XID, "xid", "", "recover to specific transaction ID")

	// Backup selection
	pitrCmd.Flags().StringVarP(&pitrOpts.Set, "set", "b", "", "select backup set to start recovery from")

	// PITR control
	pitrCmd.Flags().BoolVar(&pitrOpts.NoRestart, "no-restart", false, "don't restart PostgreSQL after restore")
	pitrCmd.Flags().BoolVar(&pitrOpts.Plan, "plan", false, "show execution plan without running")
	pitrCmd.Flags().BoolVarP(&pitrOpts.Yes, "yes", "y", false, "skip destructive confirmation prompt")
	pitrCmd.Flags().IntVar(&pitrOpts.Timeout, "timeout", 120, "PostgreSQL start/recovery timeout in seconds")

	// Common flags (inherited from pgbackrest)
	pitrCmd.Flags().StringVarP(&pitrOpts.Stanza, "stanza", "s", "", "pgBackRest stanza name")
	pitrCmd.Flags().StringVarP(&pitrOpts.ConfigPath, "config", "c", "", "pgBackRest config file path")
	pitrCmd.Flags().StringVarP(&pitrOpts.Repo, "repo", "r", "", "repository number")
	pitrCmd.Flags().StringVarP(&pitrOpts.DbSU, "dbsu", "U", "", "database superuser (default: postgres)")
	pitrCmd.Flags().StringVarP(&pitrOpts.DataDir, "data", "D", "", "target data directory")
	pitrCmd.Flags().BoolVarP(&pitrOpts.Exclusive, "exclusive", "X", false, "stop before target (exclusive)")
	pitrCmd.Flags().StringVar(&pitrOpts.TargetAction, "target-action", "", "action at recovery target: pause, promote, shutdown")
	pitrCmd.Flags().StringVarP(&pitrOpts.TargetTimeline, "target-timeline", "T", "", "recover along timeline: latest, current, N, or 0xN")
	pitrCmd.Flags().BoolVar(&pitrOpts.ForceStop, "force-stop", false, "allow immediate shutdown and kill fallback if fast stop fails")
}
