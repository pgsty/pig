/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>
*/
package cmd

import (
	"pig/cli/patroni"

	"github.com/spf13/cobra"
)

var (
	patroniConfigFile string
	patroniDcsURL     string
	patroniInsecure   bool
)

// patroniCmd represents the patroni command
var patroniCmd = &cobra.Command{
	Use:     "patroni",
	Short:   "Manage patroni service and cluster",
	Aliases: []string{"pt"},
	GroupID: "pigsty",
	Long:    `Manage Patroni service and PostgreSQL HA cluster.`,
}

// patroniListCmd: pig pt list [cluster] [-W] [-w interval]
var patroniListCmd = &cobra.Command{
	Use:   "list [cluster]",
	Short: "List cluster members",
	Long:  `List Patroni cluster members using patronictl list with -e -t flags.`,
	Example: `
  pig pt list              # List default cluster
  pig pt list pg-meta      # List specific cluster
  pig pt list -W           # Watch mode
  pig pt list -w 5         # Watch with 5s interval`,
	RunE: func(cmd *cobra.Command, args []string) error {
		watch, _ := cmd.Flags().GetBool("watch")
		interval, _ := cmd.Flags().GetString("interval")
		cluster := ""
		if len(args) > 0 {
			cluster = args[0]
		}
		return patroni.List(patroniConfigFile, patroniDcsURL, patroniInsecure, cluster, watch, interval)
	},
}

// patroniConfigCmd: pig pt config key=value ...
var patroniConfigCmd = &cobra.Command{
	Use:   "config [key=value ...]",
	Short: "Show or edit cluster config",
	Long:  `Show cluster configuration, or edit it with key=value pairs.`,
	Example: `
  pig pt config                           # Show current config
  pig pt config ttl=60                    # Set single value
  pig pt config ttl=60 loop_wait=15       # Set multiple values
  pig pt config -I ttl=60                 # Interactive mode (confirm before apply)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		interactive, _ := cmd.Flags().GetBool("interactive")
		return patroni.Config(patroniConfigFile, patroniDcsURL, patroniInsecure, args, interactive)
	},
}

// patroniReloadCmd: pig pt reload
var patroniReloadCmd = &cobra.Command{
	Use:   "reload",
	Short: "Reload patroni service",
	RunE: func(cmd *cobra.Command, args []string) error {
		return patroni.Systemctl("reload")
	},
}

// patroniRestartCmd: pig pt restart
var patroniRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart patroni service",
	RunE: func(cmd *cobra.Command, args []string) error {
		return patroni.Systemctl("restart")
	},
}

// patroniStartCmd: pig pt start
var patroniStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start patroni service",
	RunE: func(cmd *cobra.Command, args []string) error {
		return patroni.Systemctl("start")
	},
}

// patroniStopCmd: pig pt stop
var patroniStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop patroni service",
	RunE: func(cmd *cobra.Command, args []string) error {
		return patroni.Systemctl("stop")
	},
}

// patroniStatusCmd: pig pt status
var patroniStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show patroni service status",
	RunE: func(cmd *cobra.Command, args []string) error {
		return patroni.Systemctl("status")
	},
}

// patroniLogCmd: pig pt log
var patroniLogCmd = &cobra.Command{
	Use:   "log",
	Short: "View patroni logs",
	Long:  `View patroni service logs using journalctl.`,
	Example: `
  pig pt log          # View recent logs
  pig pt log -f       # Follow logs
  pig pt log -n 100   # Show last 100 lines`,
	RunE: func(cmd *cobra.Command, args []string) error {
		follow, _ := cmd.Flags().GetBool("follow")
		lines, _ := cmd.Flags().GetString("lines")
		return patroni.Log(follow, lines)
	},
}

func init() {
	// Global flags for patroni command
	patroniCmd.PersistentFlags().StringVarP(&patroniConfigFile, "config-file", "c", "", "Patroni configuration file")
	patroniCmd.PersistentFlags().StringVarP(&patroniDcsURL, "dcs-url", "d", "", "DCS connect url")
	patroniCmd.PersistentFlags().BoolVarP(&patroniInsecure, "insecure", "k", false, "Allow insecure SSL connections")

	// list subcommand flags
	patroniListCmd.Flags().BoolP("watch", "W", false, "Watch mode")
	patroniListCmd.Flags().StringP("interval", "w", "", "Watch interval in seconds")

	// config subcommand flags
	patroniConfigCmd.Flags().BoolP("interactive", "I", false, "Interactive mode (confirm before apply)")

	// log subcommand flags
	patroniLogCmd.Flags().BoolP("follow", "f", false, "Follow log output")
	patroniLogCmd.Flags().StringP("lines", "n", "50", "Number of lines to show")

	// Add subcommands
	patroniCmd.AddCommand(
		patroniListCmd,
		patroniConfigCmd,
		patroniReloadCmd,
		patroniRestartCmd,
		patroniStartCmd,
		patroniStopCmd,
		patroniStatusCmd,
		patroniLogCmd,
	)
}
