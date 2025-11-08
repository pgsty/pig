/*
Copyright © 2025 Ruohang Feng <rh@vonng.com>
*/
package cmd

import (
	"pig/cli/get"

	"github.com/spf13/cobra"
)

var updateVersion string
var updateRegion string

// updateCmd represents the installation command
var updateCmd = &cobra.Command{
	Use:          "update",
	Short:        "Upgrade pig itself",
	Aliases:      []string{"upd", "u"},
	SilenceUsage: true,
	Example: `
  
  pig update 				    # update pig to the latest version
  pig update [-v version]       # update pig to given version
  pig update -v 0.7.1 		    # update pig to version 0.6.2
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return get.UpdatePig(updateVersion, updateRegion)
	},
}

func init() {
	updateCmd.Flags().StringVarP(&updateVersion, "version", "v", "", "pigsty version to update to")
	updateCmd.Flags().StringVarP(&updateRegion, "region", "r", "", "pigsty region (default,china,...)")
}
