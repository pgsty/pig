/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>
*/
package cmd

import (
	"pig/cli/patroni"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"
	"strings"

	"github.com/spf13/cobra"
)

var (
	patroniDBSU string
	patroniPlan bool
)

func runPatroniLegacy(command string, args []string, params map[string]interface{}, fn func() error) error {
	return runLegacyStructured(output.MODULE_PT, command, args, params, fn)
}

// patroniCmd represents the patroni command
var patroniCmd = &cobra.Command{
	Use:     "patroni",
	Short:   "Manage patroni cluster with patronictl",
	Aliases: []string{"pt"},
	GroupID: "pigsty",
	Annotations: map[string]string{
		"name":       "pig patroni",
		"type":       "query",
		"volatility": "stable",
		"parallel":   "safe",
		"idempotent": "true",
		"risk":       "safe",
		"confirm":    "none",
		"os_user":    "current",
		"cost":       "100",
	},
	Long: `Manage Patroni cluster using patronictl commands.

Cluster Operations (via patronictl):
  pig pt list                      list cluster members
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
`,
}

// ============================================================================
// Cluster Operations (via patronictl)
// ============================================================================

// patroniListCmd: pig pt list [-W] [-w interval]
var patroniListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls", "l"},
	Short:   "List cluster members",
	Long:    `List Patroni cluster members using patronictl list with -e -t flags.`,
	Example: `
  pig pt list              # List cluster members
  pig pt list -o json      # Structured JSON output
  pig pt list -W           # Watch mode
  pig pt list -w 5         # Watch with 5s interval
  pig pt list -w 0.5       # Watch with 0.5s interval`,
	Annotations: map[string]string{
		"name":       "pig patroni list",
		"type":       "query",
		"volatility": "stable",
		"parallel":   "safe",
		"idempotent": "true",
		"risk":       "safe",
		"confirm":    "none",
		"os_user":    "dbsu",
		"cost":       "2000",
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		watch, _ := cmd.Flags().GetBool("watch")
		interval, _ := cmd.Flags().GetFloat64("interval")
		dbsu := utils.GetDBSU(patroniDBSU)

		// Watch mode always uses passthrough (incompatible with structured output)
		if watch || interval > 0 {
			if config.IsStructuredOutput() {
				return structuredParamError(
					output.MODULE_PT,
					"pig patroni list",
					"watch mode is not supported in structured output",
					"remove --watch/-W or --interval/-w when using -o json/-o yaml",
					args,
					map[string]interface{}{"watch": watch, "interval": interval},
				)
			}
			return patroni.List(dbsu, watch, interval)
		}

		// Structured output
		if config.IsStructuredOutput() {
			result := patroni.ListResult(dbsu)
			return handleAuxResult(result)
		}

		// Default passthrough
		return patroni.List(dbsu, false, 0)
	},
}

// patroniRestartCmd: pig pt restart [member] - restart PostgreSQL via patronictl
var patroniRestartCmd = &cobra.Command{
	Use:     "restart [member]",
	Aliases: []string{"reboot", "rt"},
	Short:   "Restart PostgreSQL instance(s) via Patroni",
	Annotations: map[string]string{
		"name":       "pig patroni restart",
		"type":       "action",
		"volatility": "volatile",
		"parallel":   "unsafe",
		"idempotent": "false",
		"risk":       "high",
		"confirm":    "recommended",
		"os_user":    "dbsu",
		"cost":       "30000",
		// Parameter constraints
		"flags.role.choices": "leader,replica,any",
	},
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
		return runPatroniLegacy("pig patroni restart", args, map[string]interface{}{
			"member":  member,
			"force":   force,
			"pending": pending,
			"role":    role,
		}, func() error {
			return patroni.Restart(utils.GetDBSU(patroniDBSU), opts)
		})
	},
}

// patroniReloadCmd: pig pt reload - reload PostgreSQL config via patronictl
var patroniReloadCmd = &cobra.Command{
	Use:     "reload",
	Aliases: []string{"rl", "hup"},
	Short:   "Reload PostgreSQL configuration via Patroni",
	Annotations: map[string]string{
		"name":       "pig patroni reload",
		"type":       "action",
		"volatility": "volatile",
		"parallel":   "restricted",
		"idempotent": "true",
		"risk":       "low",
		"confirm":    "none",
		"os_user":    "dbsu",
		"cost":       "5000",
	},
	Long: `Reload PostgreSQL configuration for all cluster members.

This triggers a configuration reload (similar to pg_reload_conf()) on all
PostgreSQL instances managed by Patroni.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPatroniLegacy("pig patroni reload", args, nil, func() error {
			return patroni.Reload(utils.GetDBSU(patroniDBSU))
		})
	},
}

// patroniReinitCmd: pig pt reinit <member>
var patroniReinitCmd = &cobra.Command{
	Use:     "reinit <member>",
	Aliases: []string{"ri"},
	Short:   "Reinitialize a cluster member",
	Annotations: map[string]string{
		"name":       "pig patroni reinit",
		"type":       "action",
		"volatility": "volatile",
		"parallel":   "unsafe",
		"idempotent": "false",
		"risk":       "critical",
		"confirm":    "required",
		"os_user":    "dbsu",
		"cost":       "300000",
	},
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
		return runPatroniLegacy("pig patroni reinit", args, map[string]interface{}{
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
	Long: `Perform a planned switchover to transfer leadership to another member.

A switchover is a planned operation that gracefully transfers leadership
from the current leader to a specified candidate (or auto-selected replica).
The old leader becomes a replica after switchover.`,
	Example: `
  pig pt switchover                          # interactive switchover
  pig pt switchover --candidate pg-test-2    # switchover to specific member
  pig pt switchover -f                       # switchover without confirmation
  pig pt switchover --scheduled "2024-01-01T12:00:00"  # scheduled switchover`,
	Annotations: map[string]string{
		"name":       "pig patroni switchover",
		"type":       "action",
		"volatility": "volatile",
		"parallel":   "unsafe",
		"idempotent": "false",
		"risk":       "high",
		"confirm":    "required",
		"os_user":    "dbsu",
		"cost":       "300000",
	},
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
	Annotations: map[string]string{
		"name":       "pig patroni failover",
		"type":       "action",
		"volatility": "volatile",
		"parallel":   "unsafe",
		"idempotent": "false",
		"risk":       "critical",
		"confirm":    "required",
		"os_user":    "dbsu",
		"cost":       "300000",
	},
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
	Use:     "pause",
	Aliases: []string{"p"},
	Short:   "Pause automatic failover",
	Annotations: map[string]string{
		"name":       "pig patroni pause",
		"type":       "action",
		"volatility": "volatile",
		"parallel":   "restricted",
		"idempotent": "true",
		"risk":       "medium",
		"confirm":    "recommended",
		"os_user":    "dbsu",
		"cost":       "5000",
	},
	Long: `Pause automatic failover for the Patroni cluster.`,
	Example: `
  pig pt pause              # Pause automatic failover
  pig pt pause --wait       # Wait for all members to confirm`,
	RunE: func(cmd *cobra.Command, args []string) error {
		wait, _ := cmd.Flags().GetBool("wait")
		return runPatroniLegacy("pig patroni pause", args, map[string]interface{}{
			"wait": wait,
		}, func() error {
			return patroni.Pause(utils.GetDBSU(patroniDBSU), wait)
		})
	},
}

// patroniResumeCmd: pig pt resume
var patroniResumeCmd = &cobra.Command{
	Use:     "resume",
	Aliases: []string{"r"},
	Short:   "Resume automatic failover",
	Annotations: map[string]string{
		"name":       "pig patroni resume",
		"type":       "action",
		"volatility": "volatile",
		"parallel":   "restricted",
		"idempotent": "true",
		"risk":       "low",
		"confirm":    "none",
		"os_user":    "dbsu",
		"cost":       "5000",
	},
	Long: `Resume automatic failover for the Patroni cluster.`,
	Example: `
  pig pt resume              # Resume automatic failover
  pig pt resume --wait       # Wait for all members to confirm`,
	RunE: func(cmd *cobra.Command, args []string) error {
		wait, _ := cmd.Flags().GetBool("wait")
		return runPatroniLegacy("pig patroni resume", args, map[string]interface{}{
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
	Annotations: map[string]string{
		"name":       "pig patroni config",
		"type":       "action",
		"volatility": "volatile",
		"parallel":   "restricted",
		"idempotent": "false",
		"risk":       "medium",
		"confirm":    "recommended",
		"os_user":    "dbsu",
		"cost":       "3000",
		// Parameter constraints
		"args.action.desc": "config action to perform",
		"args.action.type": "enum",
	},
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

		// Filter out non key=value args (should all be k=v after action)
		var filteredKV []string
		for _, arg := range kvPairs {
			if strings.Contains(arg, "=") {
				filteredKV = append(filteredKV, arg)
			}
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
			return runPatroniLegacy("pig patroni config set", args, map[string]interface{}{
				"action": action,
				"pairs":  filteredKV,
			}, func() error {
				return patroni.ConfigSet(dbsu, filteredKV)
			})
		case "pg":
			return runPatroniLegacy("pig patroni config pg", args, map[string]interface{}{
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
	Use:     "log",
	Aliases: []string{"l", "lg"},
	Short:   "View patroni logs",
	Annotations: map[string]string{
		"name":       "pig patroni log",
		"type":       "query",
		"volatility": "volatile",
		"parallel":   "safe",
		"idempotent": "true",
		"risk":       "safe",
		"confirm":    "none",
		"os_user":    "dbsu",
		"cost":       "500",
	},
	Long: `View patroni service logs using journalctl.`,
	Example: `
  pig pt log          # View recent logs
  pig pt log -f       # Follow logs
  pig pt log -n 100   # Show last 100 lines`,
	RunE: func(cmd *cobra.Command, args []string) error {
		follow, _ := cmd.Flags().GetBool("follow")
		lines, _ := cmd.Flags().GetString("lines")
		if config.IsStructuredOutput() && follow {
			return structuredParamError(
				output.MODULE_PT,
				"pig patroni log",
				"log follow mode is not supported in structured output",
				"use 'pig pt log -n N -o json' without --follow for structured snapshot",
				args,
				map[string]interface{}{"follow": follow, "lines": lines},
			)
		}
		return runPatroniLegacy("pig patroni log", args, map[string]interface{}{
			"follow": follow,
			"lines":  lines,
		}, func() error {
			return patroni.Log(follow, lines)
		})
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
	Annotations: map[string]string{
		"name":       "pig patroni status",
		"type":       "query",
		"volatility": "stable",
		"parallel":   "safe",
		"idempotent": "true",
		"risk":       "safe",
		"confirm":    "none",
		"os_user":    "dbsu",
		"cost":       "3000",
	},
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
	Use:     "start",
	Aliases: []string{"boot", "up"},
	Short:   "Start patroni service (shortcut for 'svc start')",
	Annotations: map[string]string{
		"name":       "pig patroni start",
		"type":       "action",
		"volatility": "volatile",
		"parallel":   "unsafe",
		"idempotent": "true",
		"risk":       "medium",
		"confirm":    "none",
		"os_user":    "root",
		"cost":       "10000",
	},
	Long: `Start the Patroni daemon service using systemctl. This is a shortcut for 'pig pt svc start'.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPatroniLegacy("pig patroni start", args, nil, func() error {
			return patroni.Systemctl("start")
		})
	},
}

// patroniStopCmd: pig pt stop - shortcut for pig pt svc stop
var patroniStopCmd = &cobra.Command{
	Use:     "stop",
	Aliases: []string{"halt", "dn", "down"},
	Short:   "Stop patroni service (shortcut for 'svc stop')",
	Annotations: map[string]string{
		"name":       "pig patroni stop",
		"type":       "action",
		"volatility": "volatile",
		"parallel":   "unsafe",
		"idempotent": "true",
		"risk":       "high",
		"confirm":    "recommended",
		"os_user":    "root",
		"cost":       "10000",
	},
	Long: `Stop the Patroni daemon service using systemctl. This is a shortcut for 'pig pt svc stop'.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPatroniLegacy("pig patroni stop", args, nil, func() error {
			return patroni.Systemctl("stop")
		})
	},
}

// ============================================================================
// Service Management (via systemctl) - pig pt svc
// ============================================================================

var patroniSvcCmd = &cobra.Command{
	Use:     "service",
	Aliases: []string{"svc", "s"},
	Short:   "Manage patroni daemon service",
	Annotations: map[string]string{
		"name":       "pig patroni service",
		"type":       "query",
		"volatility": "stable",
		"parallel":   "safe",
		"idempotent": "true",
		"risk":       "safe",
		"confirm":    "none",
		"os_user":    "root",
		"cost":       "100",
	},
	Long: `Manage the Patroni daemon service using systemctl.

These commands control the Patroni process itself, not the PostgreSQL
instances it manages. For PostgreSQL operations, use:
  - pig pt restart   (restart PostgreSQL via patronictl)
  - pig pt reload    (reload PostgreSQL config)`,
}

var patroniSvcStartCmd = &cobra.Command{
	Use:     "start",
	Aliases: []string{"boot", "up"},
	Short:   "Start patroni service",
	Annotations: map[string]string{
		"name":       "pig patroni service start",
		"type":       "action",
		"volatility": "volatile",
		"parallel":   "unsafe",
		"idempotent": "true",
		"risk":       "medium",
		"confirm":    "none",
		"os_user":    "root",
		"cost":       "10000",
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPatroniLegacy("pig patroni service start", args, nil, func() error {
			return patroni.Systemctl("start")
		})
	},
}

var patroniSvcStopCmd = &cobra.Command{
	Use:     "stop",
	Aliases: []string{"halt", "dn", "down"},
	Short:   "Stop patroni service",
	Annotations: map[string]string{
		"name":       "pig patroni service stop",
		"type":       "action",
		"volatility": "volatile",
		"parallel":   "unsafe",
		"idempotent": "true",
		"risk":       "high",
		"confirm":    "recommended",
		"os_user":    "root",
		"cost":       "10000",
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPatroniLegacy("pig patroni service stop", args, nil, func() error {
			return patroni.Systemctl("stop")
		})
	},
}

var patroniSvcRestartCmd = &cobra.Command{
	Use:     "restart",
	Aliases: []string{"reboot", "rt"},
	Short:   "Restart patroni service",
	Annotations: map[string]string{
		"name":       "pig patroni service restart",
		"type":       "action",
		"volatility": "volatile",
		"parallel":   "unsafe",
		"idempotent": "false",
		"risk":       "high",
		"confirm":    "recommended",
		"os_user":    "root",
		"cost":       "30000",
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPatroniLegacy("pig patroni service restart", args, nil, func() error {
			return patroni.Systemctl("restart")
		})
	},
}

var patroniSvcReloadCmd = &cobra.Command{
	Use:     "reload",
	Aliases: []string{"rl", "hup"},
	Short:   "Reload patroni service",
	Annotations: map[string]string{
		"name":       "pig patroni service reload",
		"type":       "action",
		"volatility": "volatile",
		"parallel":   "restricted",
		"idempotent": "true",
		"risk":       "low",
		"confirm":    "none",
		"os_user":    "root",
		"cost":       "1000",
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPatroniLegacy("pig patroni service reload", args, nil, func() error {
			return patroni.Systemctl("reload")
		})
	},
}

var patroniSvcStatusCmd = &cobra.Command{
	Use:     "status",
	Aliases: []string{"st", "stat"},
	Short:   "Show patroni service status",
	Annotations: map[string]string{
		"name":       "pig patroni service status",
		"type":       "query",
		"volatility": "volatile",
		"parallel":   "safe",
		"idempotent": "true",
		"risk":       "safe",
		"confirm":    "none",
		"os_user":    "root",
		"cost":       "500",
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPatroniLegacy("pig patroni service status", args, nil, func() error {
			return patroni.Systemctl("status")
		})
	},
}

// ============================================================================
// Initialization
// ============================================================================

func init() {
	// Global flags for patroni command
	patroniCmd.PersistentFlags().StringVarP(&patroniDBSU, "dbsu", "U", "", "Database superuser (default: postgres)")

	registerPatroniFlags()
	registerPatroniSvcCommands()
	registerPatroniCommands()
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

	// log subcommand flags
	patroniLogCmd.Flags().BoolP("follow", "f", false, "Follow log output")
	patroniLogCmd.Flags().StringP("lines", "n", "50", "Number of lines to show")
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
