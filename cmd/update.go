/*
Copyright © 2026 Ruohang Feng <rh@vonng.com>
*/
package cmd

import (
	"pig/cli/get"
	"strings"

	"github.com/spf13/cobra"
)

var updateVersion string
var updateRegion string
var updateMirror bool

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
  pig update -v 1.5.1 		    # update pig to version 1.5.1
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		effectiveRegion := updateRegion
		if updateMirror {
			effectiveRegion = "china"
		}
		return runLegacyStructured(legacyModuleSty, "pig update", args, map[string]interface{}{
			"version": updateVersion,
			"region":  effectiveRegion,
			"mirror":  updateMirror,
		}, func() error {
			pigVersion := updateVersion
			if strings.HasPrefix(updateVersion, "v") {
				pigVersion = strings.TrimLeft(updateVersion, "v")
			} // remove the vx.y.z 'v' prefix
			return updateExec(pigVersion, effectiveRegion)
		})
	},
}

var updateExec = get.UpdatePig

func init() {
	updateCmd.Flags().StringVarP(&updateVersion, "version", "v", "", "pigsty version to update to")
	updateCmd.Flags().StringVarP(&updateRegion, "region", "r", "", "pigsty region (default,china,...)")
	updateCmd.Flags().BoolVarP(&updateMirror, "mirror", "m", false, "prefer mirror (pigsty.cc) as primary source")
}
