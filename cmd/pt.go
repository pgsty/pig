package cmd

import (
	"fmt"
	"strings"

	"pig/cli/patroni"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"

	"github.com/spf13/cobra"
)

var (
	patroniDBSU       string
	patroniPlan       bool
	patroniConfigPlan bool
	patroniLogFollow  bool
	patroniLogLines   int
	patroniLogDir     string
)

var (
	patroniConfigPGExec = patroni.ConfigPG
)

// patroniCmd represents the patroni command
var patroniCmd = &cobra.Command{
	Use:         "patroni",
	Short:       "Manage patroni cluster with patronictl",
	Aliases:     []string{"pt"},
	GroupID:     "pigsty",
	Annotations: ancsAnn("pig patroni", "query", "stable", "safe", true, "safe", "none", "current", 100),
	Long: `pig pt - Manage Patroni cluster using patronictl commands.

Cluster Operations (via patronictl):
  pig pt list [cluster]            list cluster members
  pig pt restart [member]          restart PostgreSQL (rolling restart)
  pig pt reload                    reload PostgreSQL config
  pig pt reinit <member>           reinitialize a member
  pig pt pause                     pause automatic failover
  pig pt resume                    resume automatic failover
  pig pt switchover                perform planned switchover
  pig pt failover [candidate]      perform manual failover
  pig pt config <action>           manage cluster config (edit|show|set|pg)

Service Management (via systemctl):
  pig pt status                    show comprehensive patroni status
  pig pt svc start (pig pt start)  start patroni service
  pig pt svc stop  (pig pt stop)   stop patroni service
  pig pt svc restart               restart patroni service
  pig pt svc reload                reload patroni service
  pig pt svc status                show patroni service status

Logs:
  pig pt log [-f] [-n 50]          view patroni logs
  pig pt log tail [-n 50]          follow patroni logs
  pig pt log show [-n 50]          show patroni log snapshot
  pig pt log grep <pattern>        search patroni logs
		`,
}

// ============================================================================
// Initialization
// ============================================================================

func registerPatroniCommand() *cobra.Command {
	patroniCmd.PersistentPreRunE = commandModulePreRun

	// Global flags for patroni command
	patroniCmd.PersistentFlags().StringVarP(&patroniDBSU, "dbsu", "U", "", "Database superuser (default: postgres)")

	registerPatroniFlags()
	registerPatroniSvcCommands()
	registerPatroniCommands()
	return patroniCmd
}

func registerPatroniFlags() {
	// list subcommand flags
	patroniListCmd.Flags().BoolP("watch", "W", false, "Watch mode")
	patroniListCmd.Flags().Float64P("interval", "w", 0, "Watch interval in seconds (supports decimals, e.g., 0.5)")

	// restart subcommand flags (B04: pig owns confirmation, patronictl gets --force)
	patroniRestartCmd.Flags().BoolP("yes", "y", false, "skip confirmation prompt")
	patroniRestartCmd.Flags().BoolP("pending", "p", false, "Only restart members with pending restart")
	patroniRestartCmd.Flags().StringP("role", "r", "", "Filter by role: leader, replica, any")
	patroniRestartCmd.Flags().BoolVar(&patroniPlan, "plan", false, "show execution plan without running")

	// reinit subcommand flags (B12: --wait keeps Patroni's -w shortcut)
	patroniReinitCmd.Flags().BoolP("yes", "y", false, "skip confirmation prompt")
	patroniReinitCmd.Flags().BoolP("wait", "w", false, "Wait for reinit to complete")
	patroniReinitCmd.Flags().BoolVar(&patroniPlan, "plan", false, "show execution plan without running")

	// switchover subcommand flags (B17: target flags have explicit shortcuts)
	patroniSwitchoverCmd.Flags().StringP("leader", "l", "", "Current leader name")
	patroniSwitchoverCmd.Flags().StringP("candidate", "c", "", "Candidate to promote")
	patroniSwitchoverCmd.Flags().BoolP("yes", "y", false, "skip confirmation prompt")
	patroniSwitchoverCmd.Flags().StringP("scheduled", "s", "", "Scheduled time for switchover")
	patroniSwitchoverCmd.Flags().BoolVar(&patroniPlan, "plan", false, "show execution plan without running")

	// failover subcommand flags (B17: --candidate has explicit shortcut)
	patroniFailoverCmd.Flags().StringP("candidate", "c", "", "Candidate to promote (required)")
	patroniFailoverCmd.Flags().BoolP("yes", "y", false, "skip confirmation prompt")
	patroniFailoverCmd.Flags().BoolVar(&patroniPlan, "plan", false, "show execution plan without running")

	// pause/resume subcommand flags (B12: --wait keeps Patroni's -w shortcut)
	patroniPauseCmd.Flags().BoolP("wait", "w", false, "Wait for all members to confirm")
	patroniResumeCmd.Flags().BoolP("wait", "w", false, "Wait for all members to confirm")

	// config subcommand flags
	patroniConfigCmd.Flags().BoolVar(&patroniConfigPlan, "plan", false, "preview config changes without executing")

	// log subcommand flags
	patroniLogCmd.PersistentFlags().StringVar(&patroniLogDir, "log-dir", "", "log directory (default: from /etc/patroni/patroni.yml log.dir, fallback /pg/log/patroni)")
	patroniLogCmd.Flags().BoolVarP(&patroniLogFollow, "follow", "f", false, "follow log output")
	patroniLogCmd.Flags().IntVarP(&patroniLogLines, "lines", "n", 50, "number of lines to show")
	patroniLogTailCmd.Flags().IntVarP(&patroniLogLines, "lines", "n", 50, "number of lines to show")
	patroniLogTailCmd.Flags().BoolP("follow", "f", false, "(no-op: tail always follows)")
	patroniLogCatCmd.Flags().IntVarP(&patroniLogLines, "lines", "n", 50, "number of lines to show")
	patroniLogGrepCmd.Flags().IntP("lines", "n", 0, "search only the last N lines")
}

func registerPatroniSvcCommands() {
	// Build service subcommand group
	patroniSvcCmd.AddCommand(
		patroniSvcStartCmd,
		patroniSvcStopCmd,
		patroniSvcRestartCmd,
		patroniSvcReloadCmd,
		patroniSvcStatusCmd,
	)
}

func registerPatroniCommands() {
	patroniLogCmd.AddCommand(patroniLogCatCmd, patroniLogTailCmd, patroniLogGrepCmd)

	// Add all subcommands to patroni command
	patroniCmd.AddCommand(
		// Cluster operations (patronictl)
		patroniListCmd,
		patroniRestartCmd,
		patroniReloadCmd,
		patroniReinitCmd,
		patroniSwitchoverCmd,
		patroniFailoverCmd,
		patroniPauseCmd,
		patroniResumeCmd,
		patroniConfigCmd,
		patroniLogCmd,
		patroniStatusCmd,
		// Hidden top-level shortcuts for service start/stop (B03)
		patroniStartCmd,
		patroniStopCmd,
		// Service management (systemctl)
		patroniSvcCmd,
	)
}

// ============================================================================
// Cluster Operations (via patronictl)
// ============================================================================

// patroniListCmd: pig pt list [cluster] [-W] [-w interval]
var patroniListCmd = &cobra.Command{
	Use:     "list [cluster]",
	Aliases: []string{"ls"}, // B02: "l" belongs to log
	Short:   "List cluster members",
	Args:    cobra.MaximumNArgs(1),
	Long:    `List Patroni cluster members using patronictl list. Text mode uses -e -t flags; structured output uses -f json.`,
	Example: `
  pig pt list              # List cluster members
  pig pt list pg-meta      # List specific cluster
  pig pt list -o json      # Structured JSON output
  pig pt list -W           # Watch mode
  pig pt list -w 5         # Watch with 5s interval
  pig pt list pg-test -W -w 3  # Watch pg-test cluster, 3s refresh`,
	Annotations: ancsAnn("pig patroni list", "query", "stable", "safe", true, "safe", "none", "dbsu", 2000),
	RunE: func(cmd *cobra.Command, args []string) error {
		watch, _ := cmd.Flags().GetBool("watch")
		interval, _ := cmd.Flags().GetFloat64("interval")
		dbsu := utils.GetDBSU(patroniDBSU)
		cluster := ""
		if len(args) > 0 {
			cluster = args[0]
		}

		// Watch mode always uses passthrough (incompatible with structured output)
		if watch || interval > 0 {
			if config.IsStructuredOutput() {
				return handleAuxResult(
					output.Fail(output.CodePtWatchModeUnsupported, "watch mode is not supported in structured output").
						WithDetail("remove --watch/-W or --interval/-w when using -o json/-o yaml"),
				)
			}
			return patroni.List(dbsu, cluster, watch, interval)
		}

		// Structured output
		if config.IsStructuredOutput() {
			result := patroni.ListResult(dbsu, cluster)
			return handleAuxResult(result)
		}

		// Default passthrough
		return patroni.List(dbsu, cluster, false, 0)
	},
}

// patroniRestartCmd: pig pt restart [member] - restart PostgreSQL via patronictl
var patroniRestartCmd = &cobra.Command{
	Use:     "restart [member]",
	Aliases: []string{"rs"},
	Short:   "Restart PostgreSQL instance(s) via Patroni",
	Args:    cobra.MaximumNArgs(1),
	Annotations: mergeAnn(
		ancsAnn("pig patroni restart", "action", "volatile", "unsafe", false, "high", "recommended", "dbsu", 30000),
		map[string]string{
			"flags.role.choices": "leader,replica,any",
		},
	),
	Long: `Restart PostgreSQL instance(s) managed by Patroni.

This command uses patronictl restart to perform a rolling restart of
PostgreSQL instances. Unlike 'pig pt svc restart' which restarts the
Patroni daemon itself, this command restarts the PostgreSQL database
while keeping Patroni running.

Confirmation tier is conditional (D2): an explicit single member or a
--pending apply (scoped by a prior config change) executes directly; an
unscoped cluster-wide rolling restart asks for confirmation unless --yes
is given. patronictl always runs with --force; pig owns the prompt.`,
	Example: `
  pig pt restart                   # rolling restart ALL members (asks confirmation)
  pig pt restart -y                # cluster-wide restart, skip confirmation
  pig pt restart pg-test-1         # restart specific member (direct)
  pig pt restart --role=replica    # restart replicas only
  pig pt restart --pending         # apply pending restarts (direct)
  pig pt restart --plan            # show execution plan without running`,
	RunE: func(cmd *cobra.Command, args []string) error {
		member := ""
		if len(args) > 0 {
			member = args[0]
		}
		yes, _ := cmd.Flags().GetBool("yes")
		pending, _ := cmd.Flags().GetBool("pending")
		role, _ := cmd.Flags().GetString("role")

		switch role {
		case "", "leader", "replica", "any":
		default:
			return structuredParamError(output.MODULE_PT, "pig patroni restart",
				"invalid --role value",
				fmt.Sprintf("--role must be one of leader, replica, any; got %q", role),
				args, map[string]interface{}{"role": role})
		}

		// B04: patronictl never prompts; pig owns the confirmation below.
		opts := &patroni.RestartOptions{Member: member, Role: role, Force: true, Pending: pending}

		if patroniPlan {
			return handlePlanOutput(patroni.BuildRestartPlan(opts))
		}

		// D2 conditional tier: an explicit member or a --pending apply
		// (already scoped by a prior config change) executes directly;
		// an unscoped cluster-wide rolling restart requires consent.
		if member == "" && !pending {
			warning := "This will rolling-restart PostgreSQL on ALL cluster members"
			if role != "" {
				warning = fmt.Sprintf("This will rolling-restart PostgreSQL on all %s members", role)
			}
			if err := requirePtClusterConfirmation(yes, "restart", "high", warning,
				patroni.RestartCommand(opts, false, true),
				patroni.RestartCommand(opts, true, false),
				output.NextAction{Command: "pig pt restart <member>", Reason: "restart a single explicit member directly", Required: false},
			); err != nil {
				return err
			}
		}

		return runLegacyStructured(legacyModulePt, "pig patroni restart", args, map[string]interface{}{
			"member":  member,
			"yes":     yes,
			"pending": pending,
			"role":    role,
		}, func() error {
			return patroniRestartExec(utils.GetDBSU(patroniDBSU), opts)
		})
	},
}

// requirePtClusterConfirmation is the T2 gate for Patroni cluster operations
// in both output modes (B04: pig owns confirmation, patronictl always runs
// with --force). Structured mode is fail-closed without --yes; text mode asks
// a one-line confirmation. executeCmd/planCmd come from the patroni command
// renderers so refusals always carry replayable commands.
func requirePtClusterConfirmation(yes bool, action, risk, warning, executeCmd, planCmd string, extra ...output.NextAction) error {
	if config.IsStructuredOutput() {
		if yes {
			return nil
		}
		return requireStructuredConfirmation("pt",
			output.CodePtConfirmationRequired,
			action+" requires --yes (-y) flag in structured output mode",
			action, "pt:patroni-cluster", risk,
			executeCmd, planCmd, extra...)
	}
	return requireTextHighRiskConfirmation(yes, warning, action)
}

func requirePtSwitchPreflight(action string, state *patroni.SwitchPreflight) error {
	if state == nil || !state.Paused {
		return nil
	}
	cluster := valueOrUnknown(state.Cluster)
	detail := fmt.Sprintf("cluster %s is paused; run 'pig pt resume' before %s", cluster, action)
	if !config.IsStructuredOutput() {
		return fmt.Errorf("%s", detail)
	}
	return handleAuxResult(
		output.Fail(output.CodePtClusterPaused, "Patroni cluster is paused").
			WithDetail(detail).
			WithData(state).
			WithNextActions(
				output.NextAction{Command: "pig pt resume", Reason: "resume Patroni cluster management before leader transfer", Required: true},
				output.NextAction{Command: "pig pt list", Reason: "verify cluster pause and role state", Required: false},
			),
	)
}

func buildSwitchoverWarning(state *patroni.SwitchPreflight, opts *patroni.SwitchoverOptions) string {
	cluster := switchClusterName(state)
	leader := switchLeaderName(state, opts.Leader)
	candidates := switchCandidateSummary(state)
	if opts.Candidate != "" {
		return fmt.Sprintf("Cluster %s leadership will transfer from %s to %s (planned switchover).\nObserved candidates: %s",
			cluster, leader, opts.Candidate, candidates)
	}
	return fmt.Sprintf("Cluster %s leadership will transfer from %s to the most eligible replica selected by Patroni (planned switchover).\nObserved candidates: %s\nTo choose explicitly, rerun with: pig pt switchover -c <instance>",
		cluster, leader, candidates)
}

func buildFailoverWarning(state *patroni.SwitchPreflight, opts *patroni.FailoverOptions) string {
	cluster := switchClusterName(state)
	leader := switchLeaderName(state, "")
	candidates := switchCandidateSummary(state)
	return fmt.Sprintf("Cluster %s leadership will be forced from current leader %s to %s (failover, data loss possible).\nObserved candidates: %s",
		cluster, leader, opts.Candidate, candidates)
}

func switchClusterName(state *patroni.SwitchPreflight) string {
	if state == nil {
		return "<unknown>"
	}
	return valueOrUnknown(state.Cluster)
}

func switchLeaderName(state *patroni.SwitchPreflight, explicit string) string {
	if explicit != "" {
		return explicit
	}
	if state == nil {
		return "<unknown>"
	}
	return valueOrUnknown(state.Leader)
}

func switchCandidateSummary(state *patroni.SwitchPreflight) string {
	if state == nil || len(state.Candidates) == 0 {
		return "<none>"
	}
	return strings.Join(state.Candidates, ", ")
}

func valueOrUnknown(value string) string {
	if strings.TrimSpace(value) == "" {
		return "<unknown>"
	}
	return value
}

// Execution seams for cluster operations, stubbed in tests so RunE-level
// gating can be verified without invoking patronictl/sudo.
var (
	patroniRestartExec     = patroni.Restart
	patroniReinitExec      = patroni.Reinit
	patroniSwitchoverExec  = patroni.Switchover
	patroniFailoverExec    = patroni.Failover
	patroniSwitchPreflight = patroni.LoadSwitchPreflight
)

// splitConfigKVPairs partitions config args into key=value pairs (non-empty
// key required) and invalid tokens.
func splitConfigKVPairs(args []string) (pairs []string, invalid []string) {
	for _, arg := range args {
		if key, _, ok := strings.Cut(arg, "="); ok && key != "" {
			pairs = append(pairs, arg)
		} else {
			invalid = append(invalid, arg)
		}
	}
	return pairs, invalid
}

// patroniReloadCmd: pig pt reload - reload PostgreSQL config via patronictl
var patroniReloadCmd = &cobra.Command{
	Use:         "reload",
	Aliases:     []string{"rl"},
	Short:       "Reload PostgreSQL configuration via Patroni",
	Args:        cobra.NoArgs,
	Annotations: ancsAnn("pig patroni reload", "action", "volatile", "restricted", true, "low", "none", "dbsu", 5000),
	Long: `Reload PostgreSQL configuration for all cluster members.

This triggers a configuration reload (similar to pg_reload_conf()) on all
PostgreSQL instances managed by Patroni. The cluster scope is resolved from
/etc/patroni/patroni.yml and passed to patronictl internally.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModulePt, "pig patroni reload", args, nil, func() error {
			return patroni.Reload(utils.GetDBSU(patroniDBSU))
		})
	},
}

// patroniReinitCmd: pig pt reinit <member>
var patroniReinitCmd = &cobra.Command{
	Use:         "reinit <member>",
	Aliases:     []string{"ri"},
	Short:       "Reinitialize a cluster member",
	Annotations: ancsAnn("pig patroni reinit", "action", "volatile", "unsafe", false, "critical", "required", "dbsu", 300000),
	Long: `Reinitialize a cluster member by rebuilding it from the leader.

WARNING: This will DELETE all data on the target member and rebuild it
from scratch using pg_basebackup from the current leader.`,
	Example: `
  pig pt reinit pg-test-2          # reinit member pg-test-2 (asks confirmation)
  pig pt reinit pg-test-2 -y       # reinit without confirmation
  pig pt reinit pg-test-2 -w       # wait for completion`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		yes, _ := cmd.Flags().GetBool("yes")
		wait, _ := cmd.Flags().GetBool("wait")

		// B04: patronictl never prompts; pig owns the confirmation below.
		opts := &patroni.ReinitOptions{Member: args[0], Force: true, Wait: wait}

		if patroniPlan {
			return handlePlanOutput(patroni.BuildReinitPlan(opts))
		}
		if err := requirePtClusterConfirmation(yes, "reinit", "critical",
			fmt.Sprintf("This will WIPE and rebuild member %s from a replica copy", args[0]),
			patroni.ReinitCommand(opts, false, true),
			patroni.ReinitCommand(opts, true, false),
		); err != nil {
			return err
		}
		return runLegacyStructured(legacyModulePt, "pig patroni reinit", args, map[string]interface{}{
			"member": args[0],
			"yes":    yes,
			"wait":   wait,
		}, func() error {
			return patroniReinitExec(utils.GetDBSU(patroniDBSU), opts)
		})
	},
}

// patroniSwitchoverCmd: pig pt switchover
var patroniSwitchoverCmd = &cobra.Command{
	Use:     "switchover",
	Aliases: []string{"so"},
	Short:   "Perform planned switchover",
	Args:    cobra.NoArgs,
	Long: `Perform a planned switchover to transfer leadership to another member.

A switchover is a planned operation that gracefully transfers leadership
from the current leader to a specified candidate (or auto-selected replica).
The old leader becomes a replica after switchover.`,
	Example: `
  pig pt switchover                          # planned switchover (asks confirmation)
  pig pt switchover --candidate pg-test-2    # switchover to specific member
  pig pt switchover -l pg-test-1 -c pg-test-2 # short target flags
  pig pt switchover -y                       # switchover without confirmation
  pig pt switchover -s "2024-01-01T12:00:00" # scheduled switchover`,
	Annotations: ancsAnn("pig patroni switchover", "action", "volatile", "unsafe", false, "high", "required", "dbsu", 300000),
	RunE: func(cmd *cobra.Command, args []string) error {
		leader, _ := cmd.Flags().GetString("leader")
		candidate, _ := cmd.Flags().GetString("candidate")
		yes, _ := cmd.Flags().GetBool("yes")
		scheduled, _ := cmd.Flags().GetString("scheduled")

		// B04: patronictl never prompts; pig owns the confirmation below.
		opts := &patroni.SwitchoverOptions{
			Leader:    leader,
			Candidate: candidate,
			Force:     true,
			Scheduled: scheduled,
		}

		if patroniPlan {
			return handlePlanOutput(patroni.BuildSwitchoverPlan(opts))
		}

		dbsu := utils.GetDBSU(patroniDBSU)
		state, preflightResult := patroniSwitchPreflight(dbsu)
		if preflightResult != nil {
			return handleAuxResult(preflightResult)
		}
		if err := requirePtSwitchPreflight("switchover", state); err != nil {
			return err
		}

		if err := requirePtClusterConfirmation(yes, "switchover", "high",
			buildSwitchoverWarning(state, opts),
			patroni.SwitchoverCommand(opts, false, true),
			patroni.SwitchoverCommand(opts, true, false),
		); err != nil {
			return err
		}

		if config.IsStructuredOutput() {
			return handleAuxResult(patroni.SwitchoverResult(dbsu, opts))
		}
		return patroniSwitchoverExec(dbsu, opts)
	},
}

// patroniFailoverCmd: pig pt failover
var patroniFailoverCmd = &cobra.Command{
	Use:     "failover [candidate]",
	Aliases: []string{"fo"},
	Short:   "Perform manual failover",
	Args:    cobra.MaximumNArgs(1),
	Long: `Perform a manual failover when the leader is unavailable.

Unlike switchover, failover is used when the current leader is unhealthy
or unavailable. This may result in data loss if there are unreplicated
transactions. Patroni performs failover only to an explicit candidate,
so a candidate is required. Use --candidate/-c <member> or the positional
form: pig pt failover <member>.

WARNING: Use switchover for planned maintenance. Only use failover when
the leader is truly unavailable.`,
	Example: `
  pig pt failover --candidate pg-test-2         # failover to member (asks confirmation)
  pig pt failover -c pg-test-2                  # failover to member (short form)
  pig pt failover pg-test-2                     # failover to member (positional candidate)
  pig pt failover --candidate pg-test-2 -y      # failover without confirmation
  pig pt failover --candidate pg-test-2 -o json # structured JSON output
  pig pt failover --candidate pg-test-2 --plan  # show execution plan`,
	Annotations: ancsAnn("pig patroni failover", "action", "volatile", "unsafe", false, "critical", "required", "dbsu", 300000),
	RunE: func(cmd *cobra.Command, args []string) error {
		candidate, _ := cmd.Flags().GetString("candidate")
		yes, _ := cmd.Flags().GetBool("yes")
		if len(args) > 0 {
			if candidate != "" && candidate != args[0] {
				return structuredParamError(output.MODULE_PT, "pig patroni failover",
					"conflicting failover candidates",
					fmt.Sprintf("positional candidate %q conflicts with --candidate %q", args[0], candidate),
					args, map[string]interface{}{"candidate": candidate, "positional_candidate": args[0]})
			}
			candidate = args[0]
		}

		// Patroni's REST API only performs failover to an explicit candidate;
		// fail fast instead of leaving the rejection to patronictl.
		if candidate == "" {
			return structuredParamError(output.MODULE_PT, "pig patroni failover",
				"failover requires --candidate",
				"specify the member to promote with --candidate <member> or 'pig pt failover <member>', or use 'pig pt switchover' for planned leader transfer",
				args, nil)
		}

		// B04: patronictl never prompts; pig owns the confirmation below.
		opts := &patroni.FailoverOptions{Candidate: candidate, Force: true}

		if patroniPlan {
			return handlePlanOutput(patroni.BuildFailoverPlan(opts))
		}

		dbsu := utils.GetDBSU(patroniDBSU)
		state, preflightResult := patroniSwitchPreflight(dbsu)
		if preflightResult != nil {
			return handleAuxResult(preflightResult)
		}
		if err := requirePtSwitchPreflight("failover", state); err != nil {
			return err
		}

		if err := requirePtClusterConfirmation(yes, "failover", "critical",
			buildFailoverWarning(state, opts),
			patroni.FailoverCommand(opts, false, true),
			patroni.FailoverCommand(opts, true, false),
		); err != nil {
			return err
		}

		if config.IsStructuredOutput() {
			return handleAuxResult(patroni.FailoverResult(dbsu, opts))
		}
		return patroniFailoverExec(dbsu, opts)
	},
}

// patroniPauseCmd: pig pt pause
var patroniPauseCmd = &cobra.Command{
	Use:         "pause",
	Aliases:     []string{"p"},
	Short:       "Pause automatic failover",
	Args:        cobra.NoArgs,
	Annotations: ancsAnn("pig patroni pause", "action", "volatile", "restricted", true, "medium", "recommended", "dbsu", 5000),
	Long:        `Pause automatic failover for the Patroni cluster.`,
	Example: `
  pig pt pause              # Pause automatic failover
  pig pt pause -w           # Wait for all members to confirm`,
	RunE: func(cmd *cobra.Command, args []string) error {
		wait, _ := cmd.Flags().GetBool("wait")
		return runLegacyStructured(legacyModulePt, "pig patroni pause", args, map[string]interface{}{
			"wait": wait,
		}, func() error {
			return patroni.Pause(utils.GetDBSU(patroniDBSU), wait)
		})
	},
}

// patroniResumeCmd: pig pt resume
var patroniResumeCmd = &cobra.Command{
	Use:         "resume",
	Aliases:     []string{"r"},
	Short:       "Resume automatic failover",
	Args:        cobra.NoArgs,
	Annotations: ancsAnn("pig patroni resume", "action", "volatile", "restricted", true, "low", "none", "dbsu", 5000),
	Long:        `Resume automatic failover for the Patroni cluster.`,
	Example: `
  pig pt resume              # Resume automatic failover
  pig pt resume -w           # Wait for all members to confirm`,
	RunE: func(cmd *cobra.Command, args []string) error {
		wait, _ := cmd.Flags().GetBool("wait")
		return runLegacyStructured(legacyModulePt, "pig patroni resume", args, map[string]interface{}{
			"wait": wait,
		}, func() error {
			return patroni.Resume(utils.GetDBSU(patroniDBSU), wait)
		})
	},
}

// patroniConfigCmd: pig pt config <action> [key=value ...]
var patroniConfigCmd = &cobra.Command{
	Use:     "config <action> [key=value ...]",
	Aliases: []string{"c"},
	Short:   "Show or edit cluster config",
	Long: `Manage Patroni cluster configuration.

Actions:
  edit              Interactive config editor
  show              Display current configuration
  set  key=value    Set Patroni config (ttl, loop_wait, etc.)
  pg   key=value    Set PostgreSQL config (max_connections, shared_buffers, etc.)

PostgreSQL parameters known to use postmaster context in PG14-19 are treated as
restart-required. After changing those parameters, inspect the cluster with
"pig pt list" and apply them with "pig pt restart --pending".
`,
	Example: `
  pig pt config edit                                                        # Interactive edit
  pig pt config show                                                        # Show current config
  pig pt config show -o json                                                # Show config as JSON
  pig pt config set ttl=60                                                  # Set Patroni config
  pig pt config set ttl=60 loop_wait=15                                     # Set multiple values
  pig pt config pg max_connections=200                                      # Restart-required PG config
  pig pt config pg shared_buffers=4GB work_mem=256MB                        # Mixed PG parameters
  pig pt config pg log_min_duration_statement=250ms                         # Logging parameter, no restart
  pig pt config pg shared_preload_libraries='timescaledb,pg_stat_statements' # Restart-required preload`,
	Annotations: mergeAnn(
		ancsAnn("pig patroni config", "action", "volatile", "restricted", false, "medium", "recommended", "dbsu", 3000),
		map[string]string{
			"args.action.desc": "config action to perform",
			"args.action.type": "enum",
		},
	),
	RunE: func(cmd *cobra.Command, args []string) error {
		dbsu := utils.GetDBSU(patroniDBSU)

		if len(args) == 0 {
			args = []string{"show"} // default action in both output modes
		}

		action := args[0]
		kvPairs := args[1:]

		// Reject non key=value args instead of silently dropping them:
		// partially applied cluster config with exit 0 is worse than failing.
		filteredKV, invalidKV := splitConfigKVPairs(kvPairs)
		if (action == "set" || action == "pg") && len(invalidKV) > 0 {
			return structuredParamError(
				output.MODULE_PT,
				"pig patroni config "+action,
				"invalid config arguments",
				fmt.Sprintf("expected key=value pairs, got: %s", strings.Join(invalidKV, ", ")),
				args,
				map[string]interface{}{"action": action, "invalid": invalidKV},
			)
		}

		switch action {
		case "show":
			if config.IsStructuredOutput() {
				result := patroni.ConfigShowResult(dbsu)
				return handleAuxResult(result)
			}
			return patroni.ConfigShow(dbsu)
		case "edit":
			if config.IsStructuredOutput() {
				return structuredParamError(
					output.MODULE_PT,
					"pig patroni config",
					"interactive config edit is not supported in structured output",
					"use 'pig pt config show -o json' for read-only structured output",
					args,
					map[string]interface{}{"action": action},
				)
			}
			return patroni.ConfigEdit(dbsu)
		case "set":
			if patroniConfigPlan {
				if len(filteredKV) == 0 {
					return structuredParamError(
						output.MODULE_PT,
						"pig patroni config set",
						"invalid config plan",
						"no key=value pairs provided; usage: pig pt config set key=value --plan",
						args,
						map[string]interface{}{"action": action, "pairs": filteredKV, "plan": patroniConfigPlan},
					)
				}
				return handlePlanOutput(patroni.BuildConfigPlan(action, filteredKV))
			}
			return runLegacyStructured(legacyModulePt, "pig patroni config set", args, map[string]interface{}{
				"action": action,
				"pairs":  filteredKV,
			}, func() error {
				return patroni.ConfigSet(dbsu, filteredKV)
			})
		case "pg":
			if patroniConfigPlan {
				if len(filteredKV) == 0 {
					return structuredParamError(
						output.MODULE_PT,
						"pig patroni config pg",
						"invalid config plan",
						"no key=value pairs provided; usage: pig pt config pg key=value --plan",
						args,
						map[string]interface{}{"action": action, "pairs": filteredKV, "plan": patroniConfigPlan},
					)
				}
				return handlePlanOutput(patroni.BuildConfigPlan(action, filteredKV))
			}
			return runLegacyStructuredWithNextActions(legacyModulePt, "pig patroni config pg", args, map[string]interface{}{
				"action": action,
				"pairs":  filteredKV,
			}, patroni.ConfigPGNextActions(filteredKV), func() error {
				return runPatroniConfigPG(dbsu, filteredKV)
			})
		default:
			if config.IsStructuredOutput() {
				return handleAuxResult(
					output.Fail(output.CodePtInvalidConfigAction, "invalid config action").
						WithDetail("unknown action: " + action + " (valid: show, edit, set, pg)"),
				)
			}
			if err := cmd.Help(); err != nil {
				return err
			}
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			return &utils.ExitCodeError{
				Code:   output.ExitCode(output.CodePtInvalidConfigAction),
				Err:    fmt.Errorf("invalid config action: %s", action),
				Silent: true,
			}
		}
	},
}

func runPatroniConfigPG(dbsu string, kvPairs []string) error {
	if err := patroniConfigPGExec(dbsu, kvPairs); err != nil {
		return err
	}
	printPatroniConfigPGHints(kvPairs)
	return nil
}

func printPatroniConfigPGHints(kvPairs []string) {
	if config.IsStructuredOutput() {
		return
	}
	analysis := patroni.AnalyzePGConfigPairs(kvPairs)
	if analysis.RequiresRestart {
		utils.PrintWarn("PostgreSQL parameter change requires PostgreSQL restart: %s", strings.Join(analysis.RestartParams, ", "))
		utils.PrintInfo("Next actions:")
		utils.PrintHint([]string{"pig", "pt", "list"})
		utils.PrintHint([]string{"pig", "pt", "restart", "--pending"})
		return
	}
	utils.PrintInfo("Next action:")
	utils.PrintHint([]string{"pig", "pt", "list"})
}

// patroniLogCmd: pig pt log
var patroniLogCmd = &cobra.Command{
	Use:         "log",
	Aliases:     []string{"l"},
	Short:       "View patroni logs",
	Annotations: ancsAnn("pig patroni log", "query", "volatile", "safe", true, "safe", "none", "dbsu", 500),
	Long:        `View and search Patroni log files from the resolved Patroni log directory.`,
	Example: `
	  pig pt log             # View recent logs
	  pig pt log -f          # Follow logs
	  pig pt log tail        # Follow logs
	  pig pt log show        # View recent logs
	  pig pt log grep ERROR  # Search logs
	  pig pt log -n 100      # Show last 100 lines`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateLogLines(patroniLogLines); err != nil {
			return err
		}
		if config.IsStructuredOutput() && patroniLogFollow {
			return structuredParamError(
				output.MODULE_PT,
				"pig patroni log",
				"log follow mode is not supported in structured output",
				"use 'pig pt log show -n N -o json' without --follow for structured snapshot",
				args,
				map[string]interface{}{"follow": patroniLogFollow, "lines": patroniLogLines},
			)
		}
		if err := rejectUnsupportedLogOutputFormat("pig pt log"); err != nil {
			return err
		}
		if isJSONLogOutput() {
			return patroni.LogShowJSONL(patroniLogDir, patroniDBSU, patroniLogLines)
		}
		logDir := patroni.LogDir(patroniLogDir, patroniDBSU)
		return runLegacyStructured(legacyModulePt, "pig patroni log", args, map[string]interface{}{
			"log_dir": logDir,
			"follow":  patroniLogFollow,
			"lines":   patroniLogLines,
		}, func() error {
			if patroniLogFollow {
				return patroni.LogTail(logDir, patroniDBSU, patroniLogLines)
			}
			return patroni.LogCat(logDir, patroniDBSU, patroniLogLines)
		})
	},
}

var patroniLogCatCmd = &cobra.Command{
	Use:         "show",
	Aliases:     []string{"cat", "c", "s"},
	Short:       "Output recent patroni logs",
	Annotations: ancsAnn("pig patroni log show", "query", "volatile", "safe", true, "safe", "none", "dbsu", 500),
	Args:        cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateLogLines(patroniLogLines); err != nil {
			return err
		}
		if err := rejectUnsupportedLogOutputFormat("pig pt log show"); err != nil {
			return err
		}
		if isJSONLogOutput() {
			return patroni.LogShowJSONL(patroniLogDir, patroniDBSU, patroniLogLines)
		}
		logDir := patroni.LogDir(patroniLogDir, patroniDBSU)
		return runLegacyStructured(legacyModulePt, "pig patroni log show", args, map[string]interface{}{
			"log_dir": logDir,
			"lines":   patroniLogLines,
		}, func() error {
			return patroni.LogCat(logDir, patroniDBSU, patroniLogLines)
		})
	},
}

var patroniLogTailCmd = &cobra.Command{
	Use:         "tail",
	Aliases:     []string{"t", "f", "follow"},
	Short:       "Tail patroni logs",
	Annotations: ancsAnn("pig patroni log tail", "query", "volatile", "safe", true, "safe", "none", "dbsu", 0),
	Args:        cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateLogLines(patroniLogLines); err != nil {
			return err
		}
		if config.IsStructuredOutput() {
			return structuredParamError(
				output.MODULE_PT,
				"pig patroni log tail",
				"log follow mode is not supported in structured output",
				"use 'pig pt log show -n N -o json' for structured snapshot",
				args,
				map[string]interface{}{"lines": patroniLogLines},
			)
		}
		return patroni.LogTail(patroniLogDir, patroniDBSU, patroniLogLines)
	},
}

var patroniLogGrepCmd = &cobra.Command{
	Use:         "grep <pattern>",
	Aliases:     []string{"g", "search"},
	Short:       "Search patroni log files",
	Annotations: ancsAnn("pig patroni log grep", "query", "volatile", "safe", true, "safe", "none", "dbsu", 5000),
	Args: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceErrors = false
		cmd.SilenceUsage = false
		return cobra.ExactArgs(1)(cmd, args)
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		lines := 0
		if cmd.Flags().Changed("lines") {
			var err error
			lines, err = cmd.Flags().GetInt("lines")
			if err != nil {
				return err
			}
			if err := validateLogLines(lines); err != nil {
				return err
			}
		}
		logDir := patroni.LogDir(patroniLogDir, patroniDBSU)
		if config.IsStructuredOutput() {
			return structuredParamError(
				output.MODULE_PT,
				"pig patroni log grep",
				"log grep is not supported in structured output",
				"use VictoriaLogs for structured log filtering",
				args,
				map[string]interface{}{
					"log_dir": logDir,
					"pattern": args[0],
					"lines":   lines,
				},
			)
		}
		return runLegacyStructured(legacyModulePt, "pig patroni log grep", args, map[string]interface{}{
			"log_dir": logDir,
			"pattern": args[0],
			"lines":   lines,
		}, func() error {
			err := patroni.LogGrep(logDir, patroniDBSU, args[0], lines)
			if utils.IsSilentExit(err) {
				cmd.SilenceErrors = true
				cmd.SilenceUsage = true
			}
			return err
		})
	},
}

// patroniStatusCmd: pig pt status - comprehensive status check
var patroniStatusCmd = &cobra.Command{
	Use:     "status",
	Aliases: []string{"st"},
	Short:   "Show comprehensive patroni status",
	Long: `Show comprehensive Patroni status including:
  1. Patroni service status (systemctl status patroni)
  2. Patroni processes (ps aux | grep patroni)
  3. Patroni cluster status (patronictl list)`,
	Example: `
  pig pt status          # Show comprehensive status
  pig pt status -o json  # Structured JSON output`,
	Annotations: ancsAnn("pig patroni status", "query", "stable", "safe", true, "safe", "none", "dbsu", 3000),
	RunE: func(cmd *cobra.Command, args []string) error {
		dbsu := utils.GetDBSU(patroniDBSU)

		// Structured output
		if config.IsStructuredOutput() {
			result := patroni.StatusResult(dbsu)
			return handleAuxResult(result)
		}

		// Default passthrough
		return patroni.Status(dbsu)
	},
}

// ============================================================================
// Service Shortcuts (B03) - hidden top-level aliases
// ============================================================================

// patroniStartCmd: hidden shortcut for 'pig pt svc start'.
var patroniStartCmd = &cobra.Command{
	Use:          "start",
	Aliases:      []string{"up"},
	Hidden:       true,
	SilenceUsage: true,
	Short:        "Start patroni service",
	Long:         `Hidden shortcut for 'pig pt svc start'. Starts the Patroni daemon service through systemctl.`,
	Example: `
  pig pt start       # Shortcut for pig pt svc start`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPatroniSvcAction("start", args)
	},
}

// patroniStopCmd: hidden shortcut for 'pig pt svc stop'.
var patroniStopCmd = &cobra.Command{
	Use:          "stop",
	Aliases:      []string{"dn"},
	Hidden:       true,
	SilenceUsage: true,
	Short:        "Stop patroni service",
	Long:         `Hidden shortcut for 'pig pt svc stop'. Stops the Patroni daemon service through systemctl.`,
	Example: `
  pig pt stop        # Shortcut for pig pt svc stop`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPatroniSvcAction("stop", args)
	},
}

// ============================================================================
// Service Management (via systemctl) - pig pt service (alias: pig pt svc)
// ============================================================================

var patroniSvcCmd = &cobra.Command{
	Use:         "service",
	Aliases:     []string{"svc"},
	Short:       "Manage patroni daemon service",
	Annotations: ancsAnn("pig patroni service", "query", "stable", "safe", true, "safe", "none", "root", 100),
	Long: `Manage the Patroni daemon service using systemctl.

These commands control the Patroni process itself, not the PostgreSQL
instances it manages. For PostgreSQL operations, use:
  - pig pt restart   (restart PostgreSQL via patronictl)
  - pig pt reload    (reload PostgreSQL config)`,
}

var patroniSvcStartCmd = &cobra.Command{
	Use:         "start",
	Aliases:     []string{"up"},
	Short:       "Start patroni service",
	Annotations: ancsAnn("pig patroni service start", "action", "volatile", "unsafe", true, "medium", "none", "root", 10000),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPatroniSvcAction("start", args)
	},
}

var patroniSvcStopCmd = &cobra.Command{
	Use:         "stop",
	Aliases:     []string{"dn"},
	Short:       "Stop patroni service",
	Annotations: ancsAnn("pig patroni service stop", "action", "volatile", "unsafe", true, "high", "recommended", "root", 10000),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPatroniSvcAction("stop", args)
	},
}

var patroniSvcRestartCmd = &cobra.Command{
	Use:         "restart",
	Aliases:     []string{"rs"},
	Short:       "Restart patroni service",
	Annotations: ancsAnn("pig patroni service restart", "action", "volatile", "unsafe", false, "high", "recommended", "root", 30000),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPatroniSvcAction("restart", args)
	},
}

var patroniSvcReloadCmd = &cobra.Command{
	Use:         "reload",
	Aliases:     []string{"rl"},
	Short:       "Reload patroni service",
	Annotations: ancsAnn("pig patroni service reload", "action", "volatile", "restricted", true, "low", "none", "root", 1000),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPatroniSvcAction("reload", args)
	},
}

var patroniSvcStatusCmd = &cobra.Command{
	Use:         "status",
	Aliases:     []string{"st"},
	Short:       "Show patroni service status",
	Annotations: ancsAnn("pig patroni service status", "query", "volatile", "safe", true, "safe", "none", "root", 500),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPatroniSvcAction("status", args)
	},
}

func runPatroniSvcAction(action string, args []string) error {
	return runLegacyStructured(legacyModulePt, "pig patroni service "+action, args, nil, func() error {
		return patroni.Systemctl(action)
	})
}
