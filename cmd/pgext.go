/*
Copyright © 2024 Ruohang Feng <rh@vonng.com>
*/
package cmd

import (
	"fmt"
	"os"
	"pig/cli/pgext"
	"pig/internal/config"
	"sort"
	"text/tabwriter"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	pgextPgVer       int
	pgextPgConfig    string
	pgextShowContrib bool
	pgextYes         bool
)

// pgextCmd represents the installation command
var pgextCmd = &cobra.Command{
	Use:     "pgext",
	Short:   "Manage PostgreSQL Extensions",
	Aliases: []string{"e", "ex", "ext"},
	Example: `
Description:
  pig ext list                list & search extension      
  pig ext info    [ext...]    get infomation of a specific extension
  pig ext install [ext...]    install extension for current pg version
  pig ext remove  [ext...]    remove extension for current pg version
  pig ext update  [ext...]    update default extension list
  pig ext status              show installed extension and pg status
`,
}

var pgextListCmd = &cobra.Command{
	Use:     "list [query]",
	Short:   "list & search available extensions",
	Aliases: []string{"l", "ls", "find"},
	Example: `
  pig ext list                # list all extensions
  pig ext list postgis        # search extensions by name/description
  pig ext ls olap             # list extension of olap category
  pig ext ls gis -v 16        # list gis category for pg 16
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 1 {
			logrus.Errorf("too many arguments, only one search query allowed")
			os.Exit(1)
		}
		if err := pgext.InitExtension(nil); err != nil {
			logrus.Errorf("failed to initialize extension data: %v", err)
			os.Exit(2)
		}

		err := pgext.InitPostgres(pgextPgConfig, pgextPgVer)
		if err != nil {
			logrus.Warnf("failed to detect active postgres: %v", err)
			if pgextPgVer != 0 {
				logrus.Infof("use given pg version: %d", pgextPgVer)
			} else {
				pgextPgVer = pgext.DefaultPgVer
				logrus.Infof("fail to detect active postgres, use default pg version: %d", pgext.DefaultPgVer)
			}
		} else {
			logrus.Debugf("found active postgres, version: %d", pgext.Postgres.MajorVersion)
		}

		// If search query provided
		results := pgext.Extensions
		if len(args) == 1 {
			query := args[0]
			results = pgext.SearchExtensions(query, pgext.Extensions)
			if len(results) == 0 {
				logrus.Warnf("no extensions found matching '%s'", query)
				return nil
			} else {
				logrus.Infof("found %d extensions matching '%s':", len(results), query)
			}
		}
		pgext.TabulateSearchResult(pgextPgVer, results)
		return nil
	},
}

var pgextInfoCmd = &cobra.Command{
	Use:     "info",
	Short:   "get extension information",
	Aliases: []string{"show"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 1 {
			logrus.Errorf("too many arguments, only one search query allowed")
			os.Exit(1)
		}
		if err := pgext.InitExtension(nil); err != nil {
			logrus.Errorf("failed to initialize extension data: %v", err)
			os.Exit(2)
		}

		err := pgext.InitPostgres(pgextPgConfig, pgextPgVer)
		if err != nil {
			logrus.Warnf("failed to detect active postgres: %v", err)
			if pgextPgVer != 0 {
				logrus.Infof("use given pg version: %d", pgextPgVer)
			} else {
				logrus.Infof("fail to detect active postgres, use default pg version: %d", pgext.DefaultPgVer)
			}
		} else {
			logrus.Debugf("found active postgres, version: %d", pgext.Postgres.MajorVersion)
		}

		if len(args) == 0 {
			return fmt.Errorf("extension name required")
		}
		pgext.InitExtension(nil)
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
  pig ext ins     pg_search -y               # auto confirm installation
  pig ext install pgsql                      # install the latest version of postgresql kernel
  pig ext a pg17                             # install postgresql 17 kernel packages
  pig ext i pg16                             # install postgresql 16 kernel packages
  pig ext install pg15-core                  # install postgresql 15 core packages
  pig ext install pg14-main -y               # install pg 14 + essential extensions (vector, repack, wal2json)
  pig ext install pg13-devel --yes           # install pg 13 devel packages (auto-confirm)
  pig ext install pgsql-common               # install common utils such as patroni pgbouncer pgbackrest,...
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var pgVer int
		err := pgext.InitPostgres(pgextPgConfig, pgextPgVer)
		if err != nil {
			if pgextPgVer != 0 {
				pgVer = pgextPgVer
				logrus.Infof("fail to detect active postgres: %v, use given pg version: %d", err, pgVer)
			} else {
				logrus.Infof("fail to detect active postgres, %v, use default pg version: %d", err, pgext.DefaultPgVer)
				pgVer = pgext.DefaultPgVer
			}
		} else {
			logrus.Debugf("found active postgres, version: %d", pgext.Postgres.MajorVersion)
			pgVer = pgext.Postgres.MajorVersion
		}

		pgext.InitExtension(nil)
		pgext.InitPackageMap(config.OSType)
		if err = pgext.InstallExtensions(pgVer, args, pgextYes); err != nil {
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
		var pgVer int
		err := pgext.InitPostgres(pgextPgConfig, pgextPgVer)
		if err != nil {
			logrus.Warnf("failed to detect active postgres: %v", err)
			if pgextPgVer != 0 {
				pgVer = pgextPgVer
				logrus.Infof("use given pg version: %d", pgVer)
			} else {
				logrus.Errorf("fail to detect active postgres, please specify it explicitly")
				os.Exit(1)
			}
		} else {
			logrus.Debugf("found active postgres, version: %d", pgext.Postgres.MajorVersion)
			pgVer = pgext.Postgres.MajorVersion
		}

		pgext.InitExtension(nil)
		pgext.InitPackageMap(config.OSCode)
		if err = pgext.RemoveExtensions(pgVer, args, pgextYes); err != nil {
			logrus.Errorf("failed to remove extensions: %v", err)
			return nil
		}
		return nil
	},
}

var pgextUpdateCmd = &cobra.Command{
	Use:     "update",
	Short:   "update installed extensions for current pg version",
	Aliases: []string{"u", "up", "upgrade"},
	Example: `
Description:
  pig ext update                     # update all installed extensions
  pig ext update postgis             # update specific extension
  pig ext update postgis timescaledb # update multiple extensions
  pig ext up pg_vector -y            # update with auto-confirm
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var pgVer int
		err := pgext.InitPostgres(pgextPgConfig, pgextPgVer)
		if err != nil {
			if pgextPgVer != 0 {
				pgVer = pgextPgVer
				logrus.Infof("fail to detect active postgres: %v, use given pg version: %d", err, pgVer)
			} else {
				logrus.Errorf("fail to detect active postgres, please specify it explicitly")
				os.Exit(1)
			}
		} else {
			logrus.Debugf("found active postgres, version: %d", pgext.Postgres.MajorVersion)
			pgVer = pgext.Postgres.MajorVersion
		}

		pgext.InitExtension(nil)
		pgext.InitPackageMap(config.OSType)
		if err = pgext.UpdateExtensions(pgVer, args, pgextYes); err != nil {
			logrus.Errorf("failed to update extensions: %v", err)
			return nil
		}
		return nil
	},
}

var pgextStatusCmd = &cobra.Command{
	Use:     "status",
	Short:   "show installed extension on active pg",
	Aliases: []string{"s", "st", "stat"},
	RunE: func(cmd *cobra.Command, args []string) error {
		err := pgext.InitPostgres(pgextPgConfig, pgextPgVer)
		if err != nil {
			logrus.Errorf("failed to get PostgreSQL installation: %v", err)
			return nil
		}
		pgext.InitExtension(nil)
		pgext.ExtensionStatus(pgextShowContrib)
		return nil
	},
}

var pgextScanCmd = &cobra.Command{
	Use:     "scan",
	Short:   "scan installed extensions for active pg",
	Aliases: []string{"sc"},
	RunE: func(cmd *cobra.Command, args []string) error {
		err := pgext.InitPostgres(pgextPgConfig, pgextPgVer)
		if err != nil {
			logrus.Errorf("failed to get PostgreSQL installation: %v", err)
			return nil
		}
		pgext.Postgres.PrintSummary()

		// Sort extensions by name for consistent output
		extensions := pgext.Postgres.Extensions
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

		// Tabulate unmatched shared libraries
		fmt.Println("\nUnmatched Shared Libraries:")
		w = tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		for _, lib := range pgext.Postgres.UnmatchedLibs {
			fmt.Fprintf(w, "%s\t%d\t%s\t%s\n", lib.Name, lib.Size, lib.Mtime, lib.Path)
		}
		w.Flush()

		return nil
	},
}

func init() {
	pgextCmd.PersistentFlags().IntVarP(&pgextPgVer, "version", "v", 0, "specify a postgres by major version")
	pgextCmd.PersistentFlags().StringVarP(&pgextPgConfig, "path", "p", "", "specify a postgres by pg_config path")
	pgextStatusCmd.Flags().BoolVarP(&pgextShowContrib, "contrib", "c", false, "show contrib extensions too")

	pgextInstallCmd.Flags().BoolVarP(&pgextYes, "yes", "y", false, "auto confirm installation")
	pgextRemoveCmd.Flags().BoolVarP(&pgextYes, "yes", "y", false, "auto confirm removal")

	pgextCmd.AddCommand(pgextInstallCmd)
	pgextCmd.AddCommand(pgextRemoveCmd)
	pgextCmd.AddCommand(pgextListCmd)
	pgextCmd.AddCommand(pgextInfoCmd)
	pgextCmd.AddCommand(pgextUpdateCmd)
	pgextCmd.AddCommand(pgextStatusCmd)
	pgextCmd.AddCommand(pgextScanCmd)
	rootCmd.AddCommand(pgextCmd)
}
