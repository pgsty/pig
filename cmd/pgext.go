/*
Copyright © 2024 Ruohang Feng <rh@vonng.com>
*/
package cmd

import (
	"fmt"
	"os"
	"pig/cli/pgext"
	"pig/cli/pgsql"
	"sort"
	"text/tabwriter"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	pgextDistro   string
	pgextCategory string
	pgextPgVer    int
	pgextPgConfig string
)

// pgextCmd represents the installation command
var pgextCmd = &cobra.Command{
	Use:     "pgext",
	Short:   "Manage PostgreSQL Extensions",
	Aliases: []string{"e", "ex", "ext"},
	Example: `
Description:
  pig ext list                list all available versions     
  pig ext repo                add extension repo according to distro
  pig ext info    [ext...]    get infomation of a specific extension
  pig ext install [ext...]    install extension for current pg version
  pig ext remove  [ext...]    remove extension for current pg version
  pig ext update  [ext...]    update default extension list
  pig ext reload              reload postgres to take effect
`,
}

var pgextListCmd = &cobra.Command{
	Use:     "list",
	Short:   "list & search available extensions",
	Aliases: []string{"l", "ls"},
	RunE: func(cmd *cobra.Command, args []string) error {
		pgext.InitExtensionData(nil)
		filter := pgext.FilterExtensions(pgextDistro, pgextCategory)
		pgext.TabulateExtension(filter)
		return nil
	},
}

var pgextInfoCmd = &cobra.Command{
	Use:     "info",
	Short:   "get extension information",
	Aliases: []string{"show"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("extension name required")
		}
		pgext.InitExtensionData(nil)
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

var pgextInstallCmd = &cobra.Command{
	Use:     "install",
	Short:   "install extension for current pg version",
	Aliases: []string{"i", "ins", "add", "a"},
	Example: `
Description:
  pig ext install pg_duckdb                  # install one extension
  pig ext install postgis timescaledb        # install multiple extensions
  pig ext add     pgvector pgvectorscale     # other alias: add, ins, i, a
	`,
	RunE: func(cmd *cobra.Command, args []string) error {
		pg, err := pgsql.GetPostgres(pgextPgConfig, pgextPgVer)
		if err != nil {
			logrus.Errorf("failed to find installed PostgreSQL: %v", err)
			return nil
		}
		if err = pgext.InstallExtensions(args, pg); err != nil {
			logrus.Errorf("failed to install extensions: %v", err)
			return nil
		}
		return nil
	},
}

var pgextRemoveCmd = &cobra.Command{
	Use:     "remove",
	Short:   "remove extension for current pg version",
	Aliases: []string{"r", "rm"},
	RunE: func(cmd *cobra.Command, args []string) error {
		pg, err := pgsql.GetPostgres(pgextPgConfig, pgextPgVer)
		if err != nil {
			logrus.Errorf("failed to find installed PostgreSQL: %v", err)
			return nil
		}
		if err = pgext.RemoveExtensions(args, pg); err != nil {
			logrus.Errorf("failed to remove extensions: %v", err)
			return nil
		}
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

var pgextStatusCmd = &cobra.Command{
	Use:     "status",
	Short:   "show installed extension on active pg",
	Aliases: []string{"s", "st", "stat"},
	RunE: func(cmd *cobra.Command, args []string) error {
		pg, err := pgsql.GetPostgres(pgextPgConfig, pgextPgVer)
		if err != nil {
			logrus.Errorf("failed to get PostgreSQL installation: %v", err)
			return nil
		}

		fmt.Printf("PostgreSQL     :  %s\n", pg.Version)
		fmt.Printf("Binary Path    :  %s\n", pg.BinPath)
		fmt.Printf("Lib Path       :  %s\n", pg.LibPath)
		fmt.Printf("Share Path     :  %s\n", pg.SharePath)
		fmt.Printf("Include Path   :  %s\n", pg.IncludePath)
		fmt.Printf("Extensions     :  %d\n", len(pg.Extensions))
		fmt.Printf("\nInstalled Extensions (%d) :\n\n", len(pg.Extensions))

		// Sort extensions by name for consistent output
		extensions := pg.Extensions
		sort.Slice(extensions, func(i, j int) bool {
			return extensions[i].Name < extensions[j].Name
		})

		// Print extension details in a tabulated format
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "Name\tVersion\tDescription")
		fmt.Fprintln(w, "----\t-------\t---------------------")
		for _, ext := range extensions {
			extDescHead := ext.Description
			if len(extDescHead) > 64 {
				extDescHead = extDescHead[:64] + "..."
			}
			fmt.Fprintf(w, "%s\t%s\t%s\n",
				ext.Name,
				ext.Version,
				extDescHead,
			)
		}
		w.Flush()
		return nil
	},
}

func init() {
	pgextCmd.PersistentFlags().IntVarP(&pgextPgVer, "version", "v", 0, "specify a postgres by major version")
	pgextCmd.PersistentFlags().StringVarP(&pgextPgConfig, "path", "p", "", "specify a postgres by pg_config path")

	pgextListCmd.Flags().StringVarP(&pgextDistro, "distro", "d", "", "filter by distribution")
	pgextListCmd.Flags().StringVarP(&pgextCategory, "category", "c", "", "filter by category")

	pgextCmd.AddCommand(pgextInstallCmd)
	pgextCmd.AddCommand(pgextRemoveCmd)
	pgextCmd.AddCommand(pgextListCmd)
	pgextCmd.AddCommand(pgextInfoCmd)
	pgextCmd.AddCommand(pgextUpdateCmd)
	pgextCmd.AddCommand(pgextStatusCmd)
	rootCmd.AddCommand(pgextCmd)
}
