/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>
*/
package cmd

import (
	"os"
	"pig/cli/ext"
	"strconv"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	extPgVer       int
	extPgConfig    string
	extShowContrib bool
	extYes         bool
	extRepoDir     string
)

// extCmd represents the installation command
var extCmd = &cobra.Command{
	Use:     "ext",
	Short:   "Manage PostgreSQL Extensions (pgext)",
	Aliases: []string{"e", "ex", "pgext", "extension"},
	GroupID: "pgext",
	Long: `pig ext - Manage PostgreSQL Extensions

  Get Started: https://ext.pigsty.io/#/pig/
  pig repo add -ru             # add all repo and update cache (brute but effective)
  pig ext add pg17             # install optional postgresql 17 package
  pig ext list duck            # search extension in catalog
  pig ext scan -v 17           # scan installed extension for pg 17
  pig ext add pg_duckdb        # install certain postgresql extension
	`,
	Example: `
  pig ext list    [query]      # list & search extension      
  pig ext info    [ext...]     # get information of a specific extension
  pig ext status  [-v]         # show installed extension and pg status
  pig ext add     [ext...]     # install extension for current pg version
  pig ext rm      [ext...]     # remove extension for current pg version
  pig ext update  [ext...]     # update extension to the latest version
  pig ext import  [ext...]     # download extension to local repo
  pig ext link    [ext...]     # link postgres installation to path
  pig ext build   [ext...]     # setup building env for extension
  pig ext upgrade              # upgrade to the latest extension catalog
`, PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if err := initAll(); err != nil {
			return err
		}
		ext.ReloadCatalog()
		return nil
	},
}

var extListCmd = &cobra.Command{
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

		results := ext.Catalog.Extensions
		if len(args) == 1 {
			query := args[0]
			results = ext.SearchExtensions(query, ext.Catalog.Extensions)
			if len(results) == 0 {
				logrus.Warnf("no extensions found matching '%s'", query)
				return nil
			} else {
				logrus.Infof("found %d extensions matching '%s':", len(results), query)
			}
		}

		pgVer := extProbeVersion()
		if pgVer == 0 {
			logrus.Debugf("no active PostgreSQL found, fallback to common tabulate")
			ext.TabulteCommon(results)
		} else {
			ext.TabulteVersion(pgVer, results)
		}
		return nil

	},
}

var extInfoCmd = &cobra.Command{
	Use:     "info",
	Short:   "get extension information",
	Aliases: []string{"i"},
	RunE: func(cmd *cobra.Command, args []string) error {
		pgVer := extProbeVersion()
		logrus.Debugf("using PostgreSQL version: %d", pgVer)
		for _, name := range args {
			e, ok := ext.Catalog.ExtNameMap[name]
			if !ok {
				e, ok = ext.Catalog.ExtAliasMap[name]
				if !ok {
					logrus.Errorf("extension '%s' not found", name)
					continue
				}
			}
			e.PrintInfo()
		}
		return nil
	},
}

var extStatusCmd = &cobra.Command{
	Use:     "status",
	Short:   "show installed extension on active pg",
	Aliases: []string{"s", "st", "stat"},
	RunE: func(cmd *cobra.Command, args []string) error {
		extProbeVersion()
		ext.ExtensionStatus(extShowContrib)
		return nil
	},
}

var extScanCmd = &cobra.Command{
	Use:     "scan",
	Short:   "scan installed extensions for active pg",
	Aliases: []string{"sc"},
	RunE: func(cmd *cobra.Command, args []string) error {
		pgVer := extProbeVersion()
		ext.PostgresInstallSummary()
		if pgVer == 0 || ext.Postgres == nil {
			logrus.Debugf("no active PostgreSQL found, specify pg_config path or pg version to get more details")
			os.Exit(1)
		}
		ext.Postgres.ExtensionInstallSummary()
		return nil
	},
}

var extAddCmd = &cobra.Command{
	Use:     "add",
	Short:   "install postgres extension",
	Aliases: []string{"a", "install", "ins"},
	Example: `
Description:
  pig ext install pg_duckdb                  # install one extension
  pig ext install postgis timescaledb        # install multiple extensions
  pig ext add     pgvector pgvectorscale     # other alias: add, ins, i, a
  pig ext ins     pg_search -y               # auto confirm installation
  pig ext install pgsql                      # install the latest version of postgresql kernel
  pig ext a pg17                             # install postgresql 17 kernel packages
  pig ext ins pg16                           # install postgresql 16 kernel packages
  pig ext install pg15-core                  # install postgresql 15 core packages
  pig ext install pg14-main -y               # install pg 14 + essential extensions (vector, repack, wal2json)
  pig ext install pg13-devel --yes           # install pg 13 devel packages (auto-confirm)
  pig ext install pgsql-common               # install common utils such as patroni pgbouncer pgbackrest,...
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		pgVer := extProbeVersion()
		if err := ext.InstallExtensions(pgVer, args, extYes); err != nil {
			logrus.Errorf("failed to install extensions: %v", err)
			return nil
		}
		return nil
	},
}

var extRmCmd = &cobra.Command{
	Use:     "rm",
	Short:   "remove postgres extension",
	Aliases: []string{"r", "remove"},
	RunE: func(cmd *cobra.Command, args []string) error {
		pgVer := extProbeVersion()
		if err := ext.RemoveExtensions(pgVer, args, extYes); err != nil {
			logrus.Errorf("failed to remove extensions: %v", err)
			return nil
		}
		return nil
	},
}

var extUpdateCmd = &cobra.Command{
	Use:     "update",
	Short:   "update installed extensions for current pg version",
	Aliases: []string{"u", "upd"},
	Example: `
Description:
  pig ext update                     # update all installed extensions
  pig ext update postgis             # update specific extension
  pig ext update postgis timescaledb # update multiple extensions
  pig ext up pg_vector -y            # update with auto-confirm
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		pgVer := extProbeVersion()
		if err := ext.UpdateExtensions(pgVer, args, extYes); err != nil {
			logrus.Errorf("failed to update extensions: %v", err)
			return nil
		}
		return nil
	},
}

var extImportCmd = &cobra.Command{
	Use:          "import [ext...]",
	Short:        "import extension packages to local repo",
	Aliases:      []string{"get"},
	SilenceUsage: true,
	Example: `
  pig ext import postgis                # import postgis extension packages
  pig ext import timescaledb pg_cron    # import multiple extensions
  pig ext import pg16                   # import postgresql 16 packages
  pig ext import pgsql-common           # import common utilities
  pig ext import -d /www/pigsty postgis # import to specific path
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		pgVer := extProbeVersion()
		if err := ext.ImportExtensions(pgVer, args, extRepoDir); err != nil {
			logrus.Errorf("failed to import extensions: %v", err)
			return nil
		}
		return nil
	},
}

var extLinkCmd = &cobra.Command{
	Use:          "link <-v pgver|-p pgpath>",
	Short:        "link postgres to active PATH",
	Aliases:      []string{"ln"},
	SilenceUsage: true,
	Example: `
  pig ext link 16                      # link pgdg postgresql 16 to /usr/pgsql
  pig ext link /usr/pgsql-16           # link specific pg to /usr/pgsql
  pig ext link /u01/polardb_pg         # link polardb pg to /usr/pgsql
  pig ext link /u01/polardb_pg         # link polardb pg to /usr/pgsql
  pig ext link /u01/polardb_pg         # link polardb pg to /usr/pgsql
  pig ext link /u01/polardb_pg         # link polardb pg to /usr/pgsql
  pig ext link null|none|nil|nop|no    # unlink current postgres install
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return ext.LinkPostgres(args...)
	},
}

var extBuildCmd = &cobra.Command{
	Use:          "build [ext...]",
	Short:        "setup building env for extension",
	Aliases:      []string{"b"},
	SilenceUsage: true,
	Example: `
  pig ext build                        # prepare building environment
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return ext.BuildEnv()
	},
}

var extUpgradeCmd = &cobra.Command{
	Use:          "upgrade",
	Short:        "upgrade extension catalog to the latest version",
	SilenceUsage: true,
	Aliases:      []string{"ug"},
	RunE: func(cmd *cobra.Command, args []string) error {
		return ext.UpgradeCatalog()
	},
}

// extProbeVersion returns the PostgreSQL version to use
func extProbeVersion() int {
	// check args
	if extPgVer != 0 && extPgConfig != "" {
		logrus.Errorf("both pg version and pg_config path are specified, please specify only one")
		os.Exit(1)
	}

	// detect postgres installation, but don't fail if not found
	err := ext.DetectPostgres()
	if err != nil {
		logrus.Debugf("failed to detect PostgreSQL: %v", err)
	}

	// if pg version is specified, try if we can find the actual installation
	if extPgVer != 0 {
		_, err := ext.GetPostgres(strconv.Itoa(extPgVer))
		if err != nil {
			logrus.Debugf("PostgreSQL installation %d not found: %v , but it's ok", extPgVer, err)
			// if version is explicitly given, we can fallback without any installation
		}
		return extPgVer
	}

	// if pg_config is specified, we must find the actual installation, to get the major version
	if extPgConfig != "" {
		_, err := ext.GetPostgres(extPgConfig)
		if err != nil {
			logrus.Errorf("failed to get PostgreSQL by pg_config path %s: %v", extPgConfig, err)
			os.Exit(3)
		} else {
			return ext.Postgres.MajorVersion
		}
	}

	// if none given, we can fall back to active installation, or if we can't infer the version, we can fallback to no version tabulate
	if ext.Active != nil {
		logrus.Debugf("fallback to active PostgreSQL: %d", ext.Active.MajorVersion)
		ext.Postgres = ext.Active
		return ext.Active.MajorVersion
	} else {
		logrus.Debugf("no active PostgreSQL found, but it's ok")
		return 0
	}
}

func init() {
	extCmd.PersistentFlags().IntVarP(&extPgVer, "version", "v", 0, "specify a postgres by major version")
	extCmd.PersistentFlags().StringVarP(&extPgConfig, "path", "p", "", "specify a postgres by pg_config path")

	extStatusCmd.Flags().BoolVarP(&extShowContrib, "contrib", "c", false, "show contrib extensions too")
	extAddCmd.Flags().BoolVarP(&extYes, "yes", "y", false, "auto confirm install")
	extRmCmd.Flags().BoolVarP(&extYes, "yes", "y", false, "auto confirm removal")
	extUpdateCmd.Flags().BoolVarP(&extYes, "yes", "y", false, "auto confirm update")

	extImportCmd.Flags().StringVarP(&extRepoDir, "repo", "d", "/www/pigsty", "specify repo dir")
	extBuildCmd.Flags().BoolVarP(&extYes, "yes", "y", false, "auto confirm installation")

	extCmd.AddCommand(extAddCmd)
	extCmd.AddCommand(extRmCmd)
	extCmd.AddCommand(extListCmd)
	extCmd.AddCommand(extInfoCmd)
	extCmd.AddCommand(extScanCmd)
	extCmd.AddCommand(extUpdateCmd)
	extCmd.AddCommand(extStatusCmd)
	extCmd.AddCommand(extImportCmd)
	extCmd.AddCommand(extLinkCmd)
	extCmd.AddCommand(extBuildCmd)
	extCmd.AddCommand(extUpgradeCmd)
}
