/*
Copyright © 2025 Ruohang Feng <rh@vonng.com>
*/
package cmd

import (
	"pig/cli/get"
	"strings"

	"github.com/spf13/cobra"
)

var updateVersion string
var updateRegion string

// updateCmd represents the installation command
var updateCmd = &cobra.Command{
	Use:          "update",
	Short:        "Upgrade pig itself",
	Annotations:  ancsAnn("pig update", "action", "volatile", "unsafe", true, "medium", "recommended", "root", 30000),
	Aliases:      []string{"upd", "u"},
	SilenceUsage: true,
	Example: `
  
  pig update 				    # update pig to the latest version
  pig update [-v version]       # update pig to given version
  pig update -v 1.1.1 		    # update pig to version 1.1.1
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModuleSty, "pig update", args, map[string]interface{}{
			"version": updateVersion,
			"region":  updateRegion,
		}, func() error {
			pigVersion := updateVersion
			if strings.HasPrefix(updateVersion, "v") {
				pigVersion = strings.TrimLeft(updateVersion, "v")
			} // remove the vx.y.z 'v' prefix
			return get.UpdatePig(pigVersion, updateRegion)
		})
	},
}

func init() {
	updateCmd.Flags().StringVarP(&updateVersion, "version", "v", "", "pigsty version to update to")
	updateCmd.Flags().StringVarP(&updateRegion, "region", "r", "", "pigsty region (default,china,...)")
}
