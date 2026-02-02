/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>
*/
package cmd

import (
	"fmt"
	"os"
	"pig/cli/ext"
	"pig/internal/config"
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

  Get Started: https://pgext.cloud/pig/
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
  pig ext reload               # reload the latest extension catalog data
`,
	Annotations: map[string]string{
		"name":       "pig ext",
		"type":       "group",
		"volatility": "stable",
		"parallel":   "safe",
		"idempotent": "true",
		"risk":       "safe",
		"confirm":    "none",
		"os_user":    "current",
		"cost":       "100",
	},
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if err := initAll(); err != nil {
			return err
		}
		return ext.ReloadCatalog()
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
	Annotations: map[string]string{
		"name":       "pig ext list",
		"type":       "query",
		"volatility": "stable",
		"parallel":   "safe",
		"idempotent": "true",
		"risk":       "safe",
		"confirm":    "none",
		"os_user":    "current",
		"cost":       "100",
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 1 {
			logrus.Errorf("too many arguments, only one search query allowed")
			os.Exit(1)
		}

		pgVer := extProbeVersion()
		query := ""
		if len(args) == 1 {
			query = args[0]
		}

		// Structured output mode (YAML/JSON)
		format := config.OutputFormat
		if format == config.OUTPUT_YAML || format == config.OUTPUT_JSON || format == config.OUTPUT_JSON_PRETTY {
			result := ext.ListExtensions(query, pgVer)
			bytes, err := result.Render(format)
			if err != nil {
				return err
			}
			fmt.Println(string(bytes))
			return nil
		}

		// Text mode: preserve existing behavior
		results := ext.Catalog.Extensions
		if query != "" {
			results = ext.SearchExtensions(query, ext.Catalog.Extensions)
			if len(results) == 0 {
				logrus.Warnf("no extensions found matching '%s'", query)
				return nil
			} else {
				logrus.Infof("found %d extensions matching '%s':", len(results), query)
			}
		}

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
	Use:     "info [ext...]",
	Short:   "get extension information",
	Aliases: []string{"i"},
	Annotations: map[string]string{
		"name":       "pig ext info",
		"type":       "query",
		"volatility": "stable",
		"parallel":   "safe",
		"idempotent": "true",
		"risk":       "safe",
		"confirm":    "none",
		"os_user":    "current",
		"cost":       "50",
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Structured output mode (YAML/JSON)
		format := config.OutputFormat
		if format == config.OUTPUT_YAML || format == config.OUTPUT_JSON || format == config.OUTPUT_JSON_PRETTY {
			result := ext.GetExtensionInfo(args)
			bytes, err := result.Render(format)
			if err != nil {
				return err
			}
			fmt.Println(string(bytes))
			return nil
		}

		// Text mode: preserve existing behavior
		pgVer := extProbeVersion()
		logrus.Debugf("using PostgreSQL version: %d", pgVer)
		for _, name := range args {
			e, ok := ext.Catalog.ExtNameMap[name]
			if !ok {
				e, ok = ext.Catalog.ExtPkgMap[name]
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
	Annotations: map[string]string{
		"name":       "pig ext status",
		"type":       "query",
		"volatility": "volatile",
		"parallel":   "safe",
		"idempotent": "true",
		"risk":       "safe",
		"confirm":    "none",
		"os_user":    "current",
		"cost":       "200",
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		extProbeVersion()

		// Structured output mode (YAML/JSON)
		format := config.OutputFormat
		if format == config.OUTPUT_YAML || format == config.OUTPUT_JSON || format == config.OUTPUT_JSON_PRETTY {
			result := ext.GetExtStatus(extShowContrib)
			bytes, err := result.Render(format)
			if err != nil {
				return err
			}
			fmt.Println(string(bytes))
			return nil
		}

		// Text mode: preserve existing behavior
		ext.ExtensionStatus(extShowContrib)
		return nil
	},
}

var extScanCmd = &cobra.Command{
	Use:     "scan",
	Short:   "scan installed extensions for active pg",
	Aliases: []string{"sc"},
	Annotations: map[string]string{
		"name":       "pig ext scan",
		"type":       "query",
		"volatility": "volatile",
		"parallel":   "safe",
		"idempotent": "true",
		"risk":       "safe",
		"confirm":    "none",
		"os_user":    "current",
		"cost":       "500",
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		pgVer := extProbeVersion()

		// Structured output mode (YAML/JSON)
		format := config.OutputFormat
		if format == config.OUTPUT_YAML || format == config.OUTPUT_JSON || format == config.OUTPUT_JSON_PRETTY {
			result := ext.ScanExtensionsResult()
			bytes, err := result.Render(format)
			if err != nil {
				return err
			}
			fmt.Println(string(bytes))
			return nil
		}

		// Text mode: preserve existing behavior
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
	Annotations: map[string]string{
		"name":       "pig ext add",
		"type":       "action",
		"volatility": "stable",
		"parallel":   "restricted",
		"idempotent": "true",
		"risk":       "low",
		"confirm":    "none",
		"os_user":    "root",
		"cost":       "10000",
	},
	Example: `
Description:
  pig ext add     pg_duckdb                  # install one extension
  pig ext add     postgis timescaledb        # install multiple extensions
  pig ext add     pgvector pgvectorscale     # other alias: add, ins, i, a
  pig ext ins     pg_search -y               # auto confirm installation
  pig ext install pgsql                      # install the latest version of postgresql kernel
  pig ext a pg17                             # install postgresql 17 kernel packages
  pig ext ins pg16                           # install postgresql 16 kernel packages
  pig ext install pg15-core                  # install postgresql 15 core packages
  pig ext install pg14-main -y               # install pg 14 + essential extensions (vector, repack, wal2json)
  pig ext install pg13-devel --yes           # install pg 13 devel packages (auto-confirm)
  pig ext install pg12-mini                  # install postgresql 12 minimal packages
  pig ext install pgsql-common               # install common utils such as patroni pgbouncer pgbackrest,...
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		pgVer := extProbeVersion()

		// Structured output mode (YAML/JSON)
		format := config.OutputFormat
		if format == config.OUTPUT_YAML || format == config.OUTPUT_JSON || format == config.OUTPUT_JSON_PRETTY {
			result := ext.AddExtensions(pgVer, args, extYes)
			bytes, err := result.Render(format)
			if err != nil {
				return err
			}
			fmt.Println(string(bytes))
			// If operation failed, return error for exit code
			if !result.Success {
				return fmt.Errorf("%s", result.Message)
			}
			return nil
		}

		// Text mode: preserve existing behavior
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
	Annotations: map[string]string{
		"name":       "pig ext rm",
		"type":       "action",
		"volatility": "stable",
		"parallel":   "restricted",
		"idempotent": "true",
		"risk":       "medium",
		"confirm":    "recommended",
		"os_user":    "root",
		"cost":       "10000",
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		pgVer := extProbeVersion()

		// Structured output mode (YAML/JSON)
		format := config.OutputFormat
		if format == config.OUTPUT_YAML || format == config.OUTPUT_JSON || format == config.OUTPUT_JSON_PRETTY {
			result := ext.RmExtensions(pgVer, args, extYes)
			bytes, err := result.Render(format)
			if err != nil {
				return err
			}
			fmt.Println(string(bytes))
			// If operation failed, return error for exit code
			if !result.Success {
				return fmt.Errorf("%s", result.Message)
			}
			return nil
		}

		// Text mode: preserve existing behavior
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
	Annotations: map[string]string{
		"name":       "pig ext update",
		"type":       "action",
		"volatility": "stable",
		"parallel":   "restricted",
		"idempotent": "true",
		"risk":       "low",
		"confirm":    "none",
		"os_user":    "root",
		"cost":       "10000",
	},
	Example: `
Description:
  pig ext update                     # update all installed extensions
  pig ext update postgis             # update specific extension
  pig ext update postgis timescaledb # update multiple extensions
  pig ext up pg_vector -y            # update with auto-confirm
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		pgVer := extProbeVersion()

		// Structured output mode (YAML/JSON)
		format := config.OutputFormat
		if format == config.OUTPUT_YAML || format == config.OUTPUT_JSON || format == config.OUTPUT_JSON_PRETTY {
			result := ext.UpgradeExtensions(pgVer, args, extYes)
			bytes, err := result.Render(format)
			if err != nil {
				return err
			}
			fmt.Println(string(bytes))
			// If operation failed, return error for exit code
			if !result.Success {
				return fmt.Errorf("%s", result.Message)
			}
			return nil
		}

		// Text mode: preserve existing behavior
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
	Annotations: map[string]string{
		"name":       "pig ext import",
		"type":       "action",
		"volatility": "stable",
		"parallel":   "restricted",
		"idempotent": "true",
		"risk":       "low",
		"confirm":    "none",
		"os_user":    "root",
		"cost":       "30000",
	},
	Example: `
  pig ext import postgis                # import postgis extension packages
  pig ext import timescaledb pg_cron    # import multiple extensions
  pig ext import pg16                   # import postgresql 16 packages
  pig ext import pgsql-common           # import common utilities
  pig ext import -d /www/pigsty postgis # import to specific path
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		pgVer := extProbeVersion()

		// Structured output mode (YAML/JSON)
		format := config.OutputFormat
		if format == config.OUTPUT_YAML || format == config.OUTPUT_JSON || format == config.OUTPUT_JSON_PRETTY {
			result := ext.ImportExtensionsResult(pgVer, args, extRepoDir)
			bytes, err := result.Render(format)
			if err != nil {
				return err
			}
			fmt.Println(string(bytes))
			// If operation failed, return error for exit code
			if !result.Success {
				return fmt.Errorf("%s", result.Message)
			}
			return nil
		}

		// Text mode: preserve existing behavior
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
	Annotations: map[string]string{
		"name":       "pig ext link",
		"type":       "action",
		"volatility": "stable",
		"parallel":   "unsafe",
		"idempotent": "true",
		"risk":       "medium",
		"confirm":    "none",
		"os_user":    "root",
		"cost":       "100",
	},
	Example: `
  pig ext link 18                      # link pgdg postgresql 18 to /usr/pgsql
  pig ext link pg17                    # link postgresql 17 to /usr/pgsql (pg prefix stripped)
  pig ext link /usr/pgsql-16           # link specific pg to /usr/pgsql
  pig ext link /u01/polardb_pg         # link polardb pg to /usr/pgsql
  pig ext link null|none|nil|nop|no    # unlink current postgres install
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Structured output mode (YAML/JSON)
		format := config.OutputFormat
		if format == config.OUTPUT_YAML || format == config.OUTPUT_JSON || format == config.OUTPUT_JSON_PRETTY {
			result := ext.LinkPostgresResult(args...)
			bytes, err := result.Render(format)
			if err != nil {
				return err
			}
			fmt.Println(string(bytes))
			// If operation failed, return error for exit code
			if !result.Success {
				return fmt.Errorf("%s", result.Message)
			}
			return nil
		}

		// Text mode: preserve existing behavior
		return ext.LinkPostgres(args...)
	},
}

var extReloadCmd = &cobra.Command{
	Use:          "reload",
	Short:        "reload extension catalog to the latest version",
	SilenceUsage: true,
	Aliases:      []string{"rl"},
	Annotations: map[string]string{
		"name":       "pig ext reload",
		"type":       "action",
		"volatility": "volatile",
		"parallel":   "safe",
		"idempotent": "true",
		"risk":       "safe",
		"confirm":    "none",
		"os_user":    "current",
		"cost":       "5000",
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Structured output mode (YAML/JSON)
		format := config.OutputFormat
		if format == config.OUTPUT_YAML || format == config.OUTPUT_JSON || format == config.OUTPUT_JSON_PRETTY {
			result := ext.ReloadCatalogResult()
			bytes, err := result.Render(format)
			if err != nil {
				return err
			}
			fmt.Println(string(bytes))
			// If operation failed, return error for exit code
			if !result.Success {
				return fmt.Errorf("%s", result.Message)
			}
			return nil
		}

		// Text mode: preserve existing behavior
		return ext.ReloadExtensionCatalog()
	},
}

var extAvailCmd = &cobra.Command{
	Use:     "avail [ext...]",
	Short:   "show extension availability matrix",
	Aliases: []string{"av", "m", "matrix"},
	Example: `
  pig ext avail                     # show all packages availability on current OS
  pig ext avail timescaledb         # show availability matrix for timescaledb
  pig ext avail postgis pg_duckdb   # show matrix for multiple extensions
  pig ext av pgvector               # show availability for pgvector
  pig ext matrix citus              # alias for avail command
`,
	Annotations: map[string]string{
		"name":       "pig ext avail",
		"type":       "query",
		"volatility": "stable",
		"parallel":   "safe",
		"idempotent": "true",
		"risk":       "safe",
		"confirm":    "none",
		"os_user":    "current",
		"cost":       "100",
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Structured output mode (YAML/JSON)
		format := config.OutputFormat
		if format == config.OUTPUT_YAML || format == config.OUTPUT_JSON || format == config.OUTPUT_JSON_PRETTY {
			result := ext.GetExtensionAvailability(args)
			bytes, err := result.Render(format)
			if err != nil {
				return err
			}
			fmt.Println(string(bytes))
			return nil
		}

		// Text mode: preserve existing behavior
		if len(args) == 0 {
			// No arguments: show global package availability matrix for current OS
			ext.PrintGlobalAvailability()
			return nil
		}
		for _, name := range args {
			e, ok := ext.Catalog.ExtNameMap[name]
			if !ok {
				e, ok = ext.Catalog.ExtPkgMap[name]
				if !ok {
					logrus.Errorf("extension '%s' not found", name)
					continue
				}
			}
			ext.PrintAvailability(e)
		}
		return nil
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
	extCmd.PersistentFlags().BoolVar(&ext.ShowPkg, "pkg", false, "show Pkg instead of Name, only list lead extensions")

	extStatusCmd.Flags().BoolVarP(&extShowContrib, "contrib", "c", false, "show contrib extensions too")
	extAddCmd.Flags().BoolVarP(&extYes, "yes", "y", false, "auto confirm install")
	extRmCmd.Flags().BoolVarP(&extYes, "yes", "y", false, "auto confirm removal")
	extUpdateCmd.Flags().BoolVarP(&extYes, "yes", "y", false, "auto confirm update")
	extImportCmd.Flags().StringVarP(&extRepoDir, "repo", "d", "/www/pigsty", "specify repo dir")

	extCmd.AddCommand(extAddCmd)
	extCmd.AddCommand(extRmCmd)
	extCmd.AddCommand(extListCmd)
	extCmd.AddCommand(extInfoCmd)
	extCmd.AddCommand(extScanCmd)
	extCmd.AddCommand(extUpdateCmd)
	extCmd.AddCommand(extStatusCmd)
	extCmd.AddCommand(extImportCmd)
	extCmd.AddCommand(extLinkCmd)
	extCmd.AddCommand(extReloadCmd)
	extCmd.AddCommand(extAvailCmd)
}
