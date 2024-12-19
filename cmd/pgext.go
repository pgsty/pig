/*
Copyright © 2024 Ruohang Feng <rh@vonng.com>
*/
package cmd

import (
	"fmt"
	"pig/cli/pgext"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	pgextDistro   string
	pgextCategory string
)

// pgextCmd represents the installation command
var pgextCmd = &cobra.Command{
	Use:     "pgext",
	Short:   "manage postgres extensions",
	Aliases: []string{"e", "ext"},
	Long: `
Description:
  pig pgext list                list all available versions     
  pig pgext repo                add extension repo according to distro
  pig pgext info     <extname>  get infomation of a specific extension
  pig pgext install  <extname>  install extension for current pg version
  pig pgext remove   <extname>  remove extension for current pg version
  pig pgext update   <extname>  update default extension list
  pig pgext reload              reload postgres to take effect
`,
}

var pgextListCmd = &cobra.Command{
	Use:     "list",
	Short:   "list & search available extensions",
	Aliases: []string{"l", "info"},
	RunE: func(cmd *cobra.Command, args []string) error {
		pgext.InitExtensionData("")
		filter := pgext.FilterExtensions(pgextDistro, pgextCategory)
		pgext.TabulateExtension(filter)
		return nil
	},
}

var pgextInfoCmd = &cobra.Command{
	Use:     "info",
	Short:   "get extension information",
	Aliases: []string{"i"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("extension name required")
		}
		pgext.InitExtensionData("")
		for _, name := range args {
			ext, ok := pgext.ExtNameMap[name]
			if !ok {
				ext, ok = pgext.ExtAliasMap[name]
				if !ok {
					logrus.Errorf("extension '%s' not found", name)
					continue
				}
			}
			ext.PrintInfo()
		}
		return nil
	},
}
var pgextAddCmd = &cobra.Command{
	Use:     "add",
	Short:   "install extension for current pg version",
	Aliases: []string{"a"},
	RunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
}

var pgextRemoveCmd = &cobra.Command{
	Use:     "rm",
	Short:   "remove extension for current pg version",
	Aliases: []string{"r", "remove"},
	RunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
}

var pgextUpdateCmd = &cobra.Command{
	Use:     "update",
	Short:   "update default extension list",
	Aliases: []string{"u"},
	RunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
}

func init() {
	pgextListCmd.Flags().StringVarP(&pgextDistro, "distro", "d", "", "filter by distribution")
	pgextListCmd.Flags().StringVarP(&pgextCategory, "category", "c", "", "filter by category")

	pgextCmd.AddCommand(pgextListCmd)
	pgextCmd.AddCommand(pgextInfoCmd)
	pgextCmd.AddCommand(pgextAddCmd)
	pgextCmd.AddCommand(pgextRemoveCmd)
	pgextCmd.AddCommand(pgextUpdateCmd)
	rootCmd.AddCommand(pgextCmd)
}
