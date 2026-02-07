package cmd

import (
	"pig/cli/patroni"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"

	"github.com/spf13/cobra"
)

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
	Annotations: ancsAnn("pig patroni list", "query", "stable", "safe", true, "safe", "none", "dbsu", 2000),
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

// patroniReloadCmd: pig pt reload - reload PostgreSQL config via patronictl
var patroniReloadCmd = &cobra.Command{
	Use:         "reload",
	Aliases:     []string{"rl", "hup"},
	Short:       "Reload PostgreSQL configuration via Patroni",
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
