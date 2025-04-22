/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>
*/
package cmd

import (
	"pig/cli/patroni"

	"github.com/spf13/cobra"
)

var (
	patroniConfig   string
	patroniDcsURL   string
	patroniInsecure bool
)

// TODO: validate and takeover patroni command

// patroniCmd represents the installation command
var patroniCmd = &cobra.Command{
	Use:     "patroni",
	Short:   "Manage PostgreSQL with patronictl",
	Aliases: []string{"pt"},
	GroupID: "pigsty",
	Long: `pig patroni - Manage PostgreSQL cluster with patronictl

This command is a wrapper around patronictl to provide easier PostgreSQL cluster management. 
It automatically detects the patroni configuration file and forwards the commands to patronictl.`,
	Example: `
  dsn          Generate a dsn for the provided member, defaults to a dsn...
  edit-config  Edit cluster configuration
  failover     Failover to a replica
  flush        Discard scheduled events
  history      Show the history of failovers/switchovers
  list         List the Patroni members for a given Patroni
  pause        Disable auto failover
  query        Query a Patroni PostgreSQL member
  reinit       Reinitialize cluster member
  reload       Reload cluster member configuration
  remove       Remove cluster from DCS
  restart      Restart cluster member
  resume       Resume auto failover
  show-config  Show cluster configuration
  switchover   Switchover to a replica
  topology     Prints ASCII topology for given cluster
  version      Output version of patronictl command or a running Patroni...
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Find config if not provided via flag
		config := patroniConfig
		var err error
		if config == "" {
			config, err = patroni.FindConfig()
			if err != nil {
				return err
			}
		}

		if len(args) == 0 {
			// print help
			cmd.Help()
			return nil
		}

		// Execute patronictl with the arguments
		return patroni.Execute(config, patroniDcsURL, patroniInsecure, args)
	},
}

func init() {
	patroniCmd.Flags().StringVarP(&patroniConfig, "config-file", "c", "", "Configuration file")
	patroniCmd.Flags().StringVarP(&patroniDcsURL, "dcs-url", "d", "", "The DCS connect url")
	patroniCmd.Flags().BoolVarP(&patroniInsecure, "insecure", "k", false, "Allow connections to SSL sites without certs")
}
