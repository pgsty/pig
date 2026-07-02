package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"pig/cli/patroni"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"
)

var (
	patroniDBSU       string
	patroniPlan       bool
	patroniConfigPlan bool
	patroniLogFollow  bool
	patroniLogLines   int
)

// patroniCmd represents the patroni command
var patroniCmd = &cobra.Command{
	Use:         "patroni",
	Short:       "Manage patroni cluster with patronictl",
	Aliases:     []string{"pt"},
	GroupID:     "pigsty",
	Annotations: ancsAnn("pig patroni", "query", "stable", "safe", true, "safe", "none", "current", 100),
	Long: `Manage Patroni cluster using patronictl commands.

Cluster Operations (via patronictl):
  pig pt list [cluster]            list cluster members
  pig pt restart [member]          restart PostgreSQL (rolling restart)
  pig pt reload                    reload PostgreSQL config
  pig pt reinit <member>           reinitialize a member
  pig pt pause                     pause automatic failover
  pig pt resume                    resume automatic failover
  pig pt switchover                perform planned switchover
  pig pt failover                  perform manual failover
  pig pt config <action>           manage cluster config

Service Management (via systemctl):
  pig pt status                    show comprehensive patroni status
  pig pt start                     start patroni service (shortcut)
  pig pt stop                      stop patroni service (shortcut)
  pig pt svc start                 start patroni service
  pig pt svc stop                  stop patroni service
  pig pt svc restart               restart patroni service
  pig pt svc status                show patroni service status

Logs:
  pig pt log [-f] [-n 100]         view patroni logs
  pig pt log tail [-n 100]         follow patroni logs
  pig pt log show [-n 100]         show patroni log snapshot
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

	// restart subcommand flags
	patroniRestartCmd.Flags().BoolP("force", "f", false, "Skip confirmation")
	patroniRestartCmd.Flags().BoolP("pending", "p", false, "Only restart members with pending restart")
	patroniRestartCmd.Flags().StringP("role", "r", "", "Filter by role: leader, replica, any")

	// reinit subcommand flags
	patroniReinitCmd.Flags().BoolP("force", "f", false, "Skip confirmation")
	patroniReinitCmd.Flags().BoolP("wait", "w", false, "Wait for reinit to complete")
	patroniReinitCmd.Flags().BoolVar(&patroniPlan, "plan", false, "show execution plan without running")

	// switchover subcommand flags
	patroniSwitchoverCmd.Flags().StringP("leader", "l", "", "Current leader name")
	patroniSwitchoverCmd.Flags().StringP("candidate", "c", "", "Candidate to promote")
	patroniSwitchoverCmd.Flags().BoolP("force", "f", false, "Skip confirmation")
	patroniSwitchoverCmd.Flags().StringP("scheduled", "s", "", "Scheduled time for switchover")
	patroniSwitchoverCmd.Flags().BoolVar(&patroniPlan, "plan", false, "show execution plan without running")

	// failover subcommand flags
	patroniFailoverCmd.Flags().StringP("candidate", "c", "", "Candidate to promote")
	patroniFailoverCmd.Flags().BoolP("force", "f", false, "Skip confirmation")
	patroniFailoverCmd.Flags().BoolVar(&patroniPlan, "plan", false, "show execution plan without running")

	// pause/resume subcommand flags
	patroniPauseCmd.Flags().BoolP("wait", "w", false, "Wait for all members to confirm")
	patroniResumeCmd.Flags().BoolP("wait", "w", false, "Wait for all members to confirm")

	// config subcommand flags
	patroniConfigCmd.Flags().BoolVar(&patroniConfigPlan, "plan", false, "preview config changes without executing")

	// log subcommand flags
	patroniLogCmd.Flags().BoolVarP(&patroniLogFollow, "follow", "f", false, "follow log output")
	patroniLogCmd.Flags().IntVarP(&patroniLogLines, "lines", "n", 50, "number of lines to show")
	patroniLogTailCmd.Flags().IntVarP(&patroniLogLines, "lines", "n", 50, "number of lines to show")
	patroniLogTailCmd.Flags().BoolP("follow", "f", false, "follow log output (default for tail)")
	patroniLogCatCmd.Flags().IntVarP(&patroniLogLines, "lines", "n", 50, "number of lines to show")
}

func registerPatroniSvcCommands() {
	// Build svc subcommand group
	patroniSvcCmd.AddCommand(
		patroniSvcStartCmd,
		patroniSvcStopCmd,
		patroniSvcRestartCmd,
		patroniSvcReloadCmd,
		patroniSvcStatusCmd,
	)
}

func registerPatroniCommands() {
	patroniLogCmd.AddCommand(patroniLogCatCmd, patroniLogTailCmd)

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
		// Service shortcuts (systemctl)
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
	Aliases: []string{"reboot", "rt"},
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
while keeping Patroni running.`,
	Example: `
  pig pt restart                   # restart all members (interactive)
  pig pt restart pg-test-1         # restart specific member
  pig pt restart -f                # restart without confirmation
  pig pt restart --role=replica    # restart replicas only
  pig pt restart --pending         # restart members with pending restart`,
	RunE: func(cmd *cobra.Command, args []string) error {
		member := ""
		if len(args) > 0 {
			member = args[0]
		}
		force, _ := cmd.Flags().GetBool("force")
		pending, _ := cmd.Flags().GetBool("pending")
		role, _ := cmd.Flags().GetString("role")

		opts := &patroni.RestartOptions{
			Member:  member,
			Role:    role,
			Force:   force,
			Pending: pending,
		}
		if err := requirePatroniStructuredForce(force, patroni.RestartNeedForceResult()); err != nil {
			return err
		}
		return runLegacyStructured(legacyModulePt, "pig patroni restart", args, map[string]interface{}{
			"member":  member,
			"force":   force,
			"pending": pending,
			"role":    role,
		}, func() error {
			return patroni.Restart(utils.GetDBSU(patroniDBSU), opts)
		})
	},
}

func requirePatroniStructuredForce(force bool, result *output.Result) error {
	if !config.IsStructuredOutput() || force {
		return nil
	}
	return handleAuxResult(result)
}

// splitConfigKVPairs partitions config args into key=value pairs and invalid tokens.
func splitConfigKVPairs(args []string) (pairs []string, invalid []string) {
	for _, arg := range args {
		if strings.Contains(arg, "=") {
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
	Aliases:     []string{"rl", "hup"},
	Short:       "Reload PostgreSQL configuration via Patroni",
	Args:        cobra.NoArgs,
	Annotations: ancsAnn("pig patroni reload", "action", "volatile", "restricted", true, "low", "none", "dbsu", 5000),
	Long: `Reload PostgreSQL configuration for all cluster members.

This triggers a configuration reload (similar to pg_reload_conf()) on all
PostgreSQL instances managed by Patroni.`,
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
  pig pt reinit pg-test-2          # reinit member pg-test-2
  pig pt reinit pg-test-2 -f       # reinit without confirmation
  pig pt reinit pg-test-2 -w       # wait for completion`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		force, _ := cmd.Flags().GetBool("force")
		wait, _ := cmd.Flags().GetBool("wait")

		opts := &patroni.ReinitOptions{
			Member: args[0],
			Force:  force,
			Wait:   wait,
		}
		if patroniPlan {
			return handlePlanOutput(patroni.BuildReinitPlan(opts))
		}
		if err := requirePatroniStructuredForce(force, patroni.ReinitNeedForceResult()); err != nil {
			return err
		}
		return runLegacyStructured(legacyModulePt, "pig patroni reinit", args, map[string]interface{}{
			"member": args[0],
			"force":  force,
			"wait":   wait,
		}, func() error {
			return patroni.Reinit(utils.GetDBSU(patroniDBSU), opts)
		})
	},
}

// patroniSwitchoverCmd: pig pt switchover
var patroniSwitchoverCmd = &cobra.Command{
	Use:     "switchover",
	Aliases: []string{"sw"},
	Short:   "Perform planned switchover",
	Args:    cobra.NoArgs,
	Long: `Perform a planned switchover to transfer leadership to another member.

A switchover is a planned operation that gracefully transfers leadership
from the current leader to a specified candidate (or auto-selected replica).
The old leader becomes a replica after switchover.`,
	Example: `
  pig pt switchover                          # interactive switchover
  pig pt switchover --candidate pg-test-2    # switchover to specific member
  pig pt switchover -f                       # switchover without confirmation
  pig pt switchover --scheduled "2024-01-01T12:00:00"  # scheduled switchover`,
	Annotations: ancsAnn("pig patroni switchover", "action", "volatile", "unsafe", false, "high", "required", "dbsu", 300000),
	RunE: func(cmd *cobra.Command, args []string) error {
		leader, _ := cmd.Flags().GetString("leader")
		candidate, _ := cmd.Flags().GetString("candidate")
		force, _ := cmd.Flags().GetBool("force")
		scheduled, _ := cmd.Flags().GetString("scheduled")

		opts := &patroni.SwitchoverOptions{
			Leader:    leader,
			Candidate: candidate,
			Force:     force,
			Scheduled: scheduled,
		}

		// Plan mode (highest priority)
		if patroniPlan {
			plan := patroni.BuildSwitchoverPlan(opts)
			return output.RenderPlan(plan)
		}

		// Structured output mode
		if config.IsStructuredOutput() {
			result := patroni.SwitchoverResult(utils.GetDBSU(patroniDBSU), opts)
			return handleAuxResult(result)
		}

		// Default passthrough
		return patroni.Switchover(utils.GetDBSU(patroniDBSU), opts)
	},
}

// patroniFailoverCmd: pig pt failover
var patroniFailoverCmd = &cobra.Command{
	Use:     "failover",
	Aliases: []string{"fo"},
	Short:   "Perform manual failover",
	Args:    cobra.NoArgs,
	Long: `Perform a manual failover when the leader is unavailable.

Unlike switchover, failover is used when the current leader is unhealthy
or unavailable. This may result in data loss if there are unreplicated
transactions.

WARNING: Use switchover for planned maintenance. Only use failover when
the leader is truly unavailable.`,
	Example: `
  pig pt failover                          # interactive failover
  pig pt failover --candidate pg-test-2    # failover to specific member
  pig pt failover -f                       # failover without confirmation
  pig pt failover -f -o json               # structured JSON output
  pig pt failover --plan                   # show execution plan`,
	Annotations: ancsAnn("pig patroni failover", "action", "volatile", "unsafe", false, "critical", "required", "dbsu", 300000),
	RunE: func(cmd *cobra.Command, args []string) error {
		candidate, _ := cmd.Flags().GetString("candidate")
		force, _ := cmd.Flags().GetBool("force")

		opts := &patroni.FailoverOptions{
			Candidate: candidate,
			Force:     force,
		}

		// Plan mode (highest priority)
		if patroniPlan {
			plan := patroni.BuildFailoverPlan(opts)
			return output.RenderPlan(plan)
		}

		// Structured output mode
		if config.IsStructuredOutput() {
			result := patroni.FailoverResult(utils.GetDBSU(patroniDBSU), opts)
			return handleAuxResult(result)
		}

		// Default passthrough
		return patroni.Failover(utils.GetDBSU(patroniDBSU), opts)
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
  pig pt pause --wait       # Wait for all members to confirm`,
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
  pig pt resume --wait       # Wait for all members to confirm`,
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
	Aliases: []string{"cfg", "c"},
	Short:   "Show or edit cluster config",
	Long: `Manage Patroni cluster configuration.

Actions:
  edit              Interactive config editor
  show              Display current configuration
  set  key=value    Set Patroni config (ttl, loop_wait, etc.)
  pg   key=value    Set PostgreSQL config (max_connections, etc.)`,
	Example: `
  pig pt config edit                      # Interactive edit
  pig pt config show                      # Show current config
  pig pt config show -o json              # Show config as structured JSON
  pig pt config set ttl=60                # Set Patroni config
  pig pt config set ttl=60 loop_wait=15   # Set multiple values
  pig pt config pg max_connections=200    # Set PostgreSQL config
  pig pt config pg shared_buffers=4GB work_mem=256MB`,
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
			// No args: structured output defaults to show, text mode shows help
			if config.IsStructuredOutput() {
				result := patroni.ConfigShowResult(dbsu)
				return handleAuxResult(result)
			}
			return cmd.Help()
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
			return runLegacyStructured(legacyModulePt, "pig patroni config pg", args, map[string]interface{}{
				"action": action,
				"pairs":  filteredKV,
			}, func() error {
				return patroni.ConfigPG(dbsu, filteredKV)
			})
		default:
			if config.IsStructuredOutput() {
				return handleAuxResult(
					output.Fail(output.CodePtInvalidConfigAction, "invalid config action").
						WithDetail("unknown action: " + action + " (valid: show, edit, set, pg)"),
				)
			}
			return cmd.Help()
		}
	},
}

// patroniLogCmd: pig pt log
var patroniLogCmd = &cobra.Command{
	Use:         "log",
	Aliases:     []string{"l", "lg"},
	Short:       "View patroni logs",
	Annotations: ancsAnn("pig patroni log", "query", "volatile", "safe", true, "safe", "none", "dbsu", 500),
	Long:        `View patroni service logs using journalctl.`,
	Example: `
	  pig pt log          # View recent logs
	  pig pt log -f       # Follow logs
	  pig pt log tail     # Follow logs
	  pig pt log show     # View recent logs
	  pig pt log -n 100   # Show last 100 lines`,
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
			return patroni.LogJSONL(patroniLogLines)
		}
		return runLegacyStructured(legacyModulePt, "pig patroni log", args, map[string]interface{}{
			"follow": patroniLogFollow,
			"lines":  patroniLogLines,
		}, func() error {
			return patroni.Log(patroniLogFollow, patroniLogLines)
		})
	},
}

var patroniLogCatCmd = &cobra.Command{
	Use:         "show",
	Aliases:     []string{"cat", "c"},
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
			return patroni.LogJSONL(patroniLogLines)
		}
		return runLegacyStructured(legacyModulePt, "pig patroni log show", args, map[string]interface{}{
			"lines": patroniLogLines,
		}, func() error {
			return patroni.Log(false, patroniLogLines)
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
		return patroni.Log(true, patroniLogLines)
	},
}

// patroniStatusCmd: pig pt status - comprehensive status check
var patroniStatusCmd = &cobra.Command{
	Use:     "status",
	Aliases: []string{"st", "stat"},
	Short:   "Show comprehensive patroni status",
	Long: `Show comprehensive Patroni status including:
  1. Patroni service status (systemctl status patroni)
  2. Patroni processes (ps aux | grep patroni)
  3. Patroni cluster status (patronictl list)`,
	Example: `
  pig pt status          # Show comprehensive status
  pig pt status -o json  # Structured JSON output
  pig pt st              # Same as above (shortcut)`,
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
// Service Shortcuts (via systemctl) - pig pt start/stop
// ============================================================================

// patroniStartCmd: pig pt start - shortcut for pig pt svc start
var patroniStartCmd = &cobra.Command{
	Use:         "start",
	Aliases:     []string{"boot", "up"},
	Short:       "Start patroni service (shortcut for 'svc start')",
	Annotations: ancsAnn("pig patroni start", "action", "volatile", "unsafe", true, "medium", "none", "root", 10000),
	Long:        `Start the Patroni daemon service using systemctl. This is a shortcut for 'pig pt svc start'.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModulePt, "pig patroni start", args, nil, func() error {
			return patroni.Systemctl("start")
		})
	},
}

// patroniStopCmd: pig pt stop - shortcut for pig pt svc stop
var patroniStopCmd = &cobra.Command{
	Use:         "stop",
	Aliases:     []string{"halt", "dn", "down"},
	Short:       "Stop patroni service (shortcut for 'svc stop')",
	Annotations: ancsAnn("pig patroni stop", "action", "volatile", "unsafe", true, "high", "recommended", "root", 10000),
	Long:        `Stop the Patroni daemon service using systemctl. This is a shortcut for 'pig pt svc stop'.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModulePt, "pig patroni stop", args, nil, func() error {
			return patroni.Systemctl("stop")
		})
	},
}

// ============================================================================
// Service Management (via systemctl) - pig pt svc
// ============================================================================

var patroniSvcCmd = &cobra.Command{
	Use:         "service",
	Aliases:     []string{"svc", "s"},
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
	Aliases:     []string{"boot", "up"},
	Short:       "Start patroni service",
	Annotations: ancsAnn("pig patroni service start", "action", "volatile", "unsafe", true, "medium", "none", "root", 10000),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModulePt, "pig patroni service start", args, nil, func() error {
			return patroni.Systemctl("start")
		})
	},
}

var patroniSvcStopCmd = &cobra.Command{
	Use:         "stop",
	Aliases:     []string{"halt", "dn", "down"},
	Short:       "Stop patroni service",
	Annotations: ancsAnn("pig patroni service stop", "action", "volatile", "unsafe", true, "high", "recommended", "root", 10000),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModulePt, "pig patroni service stop", args, nil, func() error {
			return patroni.Systemctl("stop")
		})
	},
}

var patroniSvcRestartCmd = &cobra.Command{
	Use:         "restart",
	Aliases:     []string{"reboot", "rt"},
	Short:       "Restart patroni service",
	Annotations: ancsAnn("pig patroni service restart", "action", "volatile", "unsafe", false, "high", "recommended", "root", 30000),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModulePt, "pig patroni service restart", args, nil, func() error {
			return patroni.Systemctl("restart")
		})
	},
}

var patroniSvcReloadCmd = &cobra.Command{
	Use:         "reload",
	Aliases:     []string{"rl", "hup"},
	Short:       "Reload patroni service",
	Annotations: ancsAnn("pig patroni service reload", "action", "volatile", "restricted", true, "low", "none", "root", 1000),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModulePt, "pig patroni service reload", args, nil, func() error {
			return patroni.Systemctl("reload")
		})
	},
}

var patroniSvcStatusCmd = &cobra.Command{
	Use:         "status",
	Aliases:     []string{"st", "stat"},
	Short:       "Show patroni service status",
	Annotations: ancsAnn("pig patroni service status", "query", "volatile", "safe", true, "safe", "none", "root", 500),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModulePt, "pig patroni service status", args, nil, func() error {
			return patroni.Systemctl("status")
		})
	},
}
