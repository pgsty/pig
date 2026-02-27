/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>
*/
package cmd

import (
	"github.com/spf13/cobra"
)

var (
	patroniDBSU string
	patroniPlan bool
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
