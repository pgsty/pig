/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>
*/
package cmd

import (
	"fmt"
	"pig/cli/ext"
	"pig/internal/output"

	"github.com/spf13/cobra"
)

var (
	extPgVer       int
	extPgConfig    string
	extShowContrib bool
	extYes         bool
	extRepoDir     string
	extAddPlan     bool
	extRmPlan      bool
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
	Annotations: ancsAnn("pig ext", "query", "stable", "safe", true, "safe", "none", "current", 100),
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if err := initAll(); err != nil {
			return err
		}
		applyStructuredOutputSilence(cmd)
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
	Annotations: ancsAnn("pig ext list", "query", "stable", "safe", true, "safe", "none", "current", 100),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 1 {
			return handleAuxResult(output.Fail(output.CodeExtensionInvalidArgs, "too many arguments, only one search query allowed"))
		}

		pgVer, err := extProbeVersion()
		if err != nil {
			return handleExtProbeError(err)
		}
		query := ""
		if len(args) == 1 {
			query = args[0]
		}

		result := ext.ListExtensions(query, pgVer)
		return handleAuxResult(result)
	},
}

var extInfoCmd = &cobra.Command{
	Use:         "info [ext...]",
	Short:       "get extension information",
	Aliases:     []string{"i"},
	Annotations: ancsAnn("pig ext info", "query", "stable", "safe", true, "safe", "none", "current", 50),
	RunE: func(cmd *cobra.Command, args []string) error {
		result := ext.GetExtensionInfo(args)
		return handleAuxResult(result)
	},
}

var extStatusCmd = &cobra.Command{
	Use:         "status",
	Short:       "show installed extension on active pg",
	Aliases:     []string{"s", "st", "stat"},
	Annotations: ancsAnn("pig ext status", "query", "volatile", "safe", true, "safe", "none", "current", 200),
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := extProbeVersion(); err != nil {
			return handleExtProbeError(err)
		}
		result := ext.GetExtStatus(extShowContrib)
		return handleAuxResult(result)
	},
}

var extScanCmd = &cobra.Command{
	Use:         "scan",
	Short:       "scan installed extensions for active pg",
	Aliases:     []string{"sc"},
	Annotations: ancsAnn("pig ext scan", "query", "volatile", "safe", true, "safe", "none", "current", 500),
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := extProbeVersion(); err != nil {
			return handleExtProbeError(err)
		}
		result := ext.ScanExtensionsResult()
		return handleAuxResult(result)
	},
}

var extAddCmd = &cobra.Command{
	Use:         "add",
	Short:       "install postgres extension",
	Aliases:     []string{"a", "install", "ins"},
	Annotations: ancsAnn("pig ext add", "action", "stable", "restricted", true, "low", "none", "root", 10000),
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
  pig ext add postgis --plan                 # preview install plan without executing
  pig ext add postgis -o json --plan         # plan output in JSON format
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		pgVer, err := extProbeVersion()
		if err != nil {
			return handleExtProbeError(err)
		}

		// Plan mode: show plan without executing
		if extAddPlan {
			plan := ext.BuildAddPlan(pgVer, args, extYes)
			return handlePlanOutput(plan)
		}

		result := ext.AddExtensions(pgVer, args, extYes)
		return handleAuxResult(result)
	},
}

var extRmCmd = &cobra.Command{
	Use:         "rm",
	Short:       "remove postgres extension",
	Aliases:     []string{"r", "remove"},
	Annotations: ancsAnn("pig ext rm", "action", "stable", "restricted", false, "medium", "recommended", "root", 10000),
	RunE: func(cmd *cobra.Command, args []string) error {
		pgVer, err := extProbeVersion()
		if err != nil {
			return handleExtProbeError(err)
		}

		// Plan mode: show plan without executing
		if extRmPlan {
			plan := ext.BuildRmPlan(pgVer, args, extYes)
			return handlePlanOutput(plan)
		}

		result := ext.RmExtensions(pgVer, args, extYes)
		return handleAuxResult(result)
	},
}

var extUpdateCmd = &cobra.Command{
	Use:         "update",
	Short:       "update installed extensions for current pg version",
	Aliases:     []string{"u", "upd"},
	Annotations: ancsAnn("pig ext update", "action", "stable", "restricted", true, "low", "none", "root", 10000),
	Example: `
Description:
  pig ext update                     # no-op (safety), requires explicit targets
  pig ext update postgis             # update specific extension
  pig ext update postgis timescaledb # update multiple extensions
  pig ext up pg_vector -y            # update with auto-confirm
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Safety: no arguments means no-op (avoid "update everything" surprises).
		// Do not force PG probe in this case.
		if len(args) == 0 {
			result := ext.UpgradeExtensions(extPgVer, args, extYes)
			return handleAuxResult(result)
		}

		pgVer, err := extProbeVersion()
		if err != nil {
			return handleExtProbeError(err)
		}
		result := ext.UpgradeExtensions(pgVer, args, extYes)
		return handleAuxResult(result)
	},
}

var extImportCmd = &cobra.Command{
	Use:          "import [ext...]",
	Short:        "import extension packages to local repo",
	Aliases:      []string{"get"},
	SilenceUsage: true,
	Annotations:  ancsAnn("pig ext import", "action", "stable", "restricted", true, "low", "none", "root", 30000),
	Example: `
  pig ext import postgis                # import postgis extension packages
  pig ext import timescaledb pg_cron    # import multiple extensions
  pig ext import pg16                   # import postgresql 16 packages
  pig ext import pgsql-common           # import common utilities
  pig ext import -d /www/pigsty postgis # import to specific path
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		pgVer, err := extProbeVersion()
		if err != nil {
			return handleExtProbeError(err)
		}
		result := ext.ImportExtensionsResult(pgVer, args, extRepoDir)
		return handleAuxResult(result)
	},
}

var extLinkCmd = &cobra.Command{
	Use:          "link <-v pgver|-p pgpath>",
	Short:        "link postgres to active PATH",
	Aliases:      []string{"ln"},
	SilenceUsage: true,
	Annotations:  ancsAnn("pig ext link", "action", "stable", "unsafe", true, "medium", "none", "root", 100),
	Example: `
  pig ext link 18                      # link pgdg postgresql 18 to /usr/pgsql
  pig ext link pg17                    # link postgresql 17 to /usr/pgsql (pg prefix stripped)
  pig ext link /usr/pgsql-16           # link specific pg to /usr/pgsql
  pig ext link /u01/polardb_pg         # link polardb pg to /usr/pgsql
  pig ext link null|none|nil|nop|no    # unlink current postgres install
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		result := ext.LinkPostgresResult(args...)
		return handleAuxResult(result)
	},
}

var extReloadCmd = &cobra.Command{
	Use:          "reload",
	Short:        "reload extension catalog to the latest version",
	SilenceUsage: true,
	Aliases:      []string{"rl"},
	Annotations:  ancsAnn("pig ext reload", "action", "volatile", "safe", true, "safe", "none", "current", 5000),
	RunE: func(cmd *cobra.Command, args []string) error {
		result := ext.ReloadCatalogResult()
		return handleAuxResult(result)
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
	Annotations: ancsAnn("pig ext avail", "query", "stable", "safe", true, "safe", "none", "current", 100),
	RunE: func(cmd *cobra.Command, args []string) error {
		result := ext.GetExtensionAvailability(args)
		return handleAuxResult(result)
	},
}

type extProbeError struct {
	Code int
	Err  error
}

func (e *extProbeError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return "failed to probe PostgreSQL"
}

func (e *extProbeError) Unwrap() error {
	return e.Err
}

func extProbeErrorCode(err error) int {
	if err == nil {
		return 0
	}
	if pe, ok := err.(*extProbeError); ok {
		return pe.Code
	}
	return output.CodeExtensionInvalidArgs
}

// extProbeVersion returns the PostgreSQL version to use
func extProbeVersion() (int, error) {
	return probePostgresMajorVersion(pgMajorProbeOptions{
		Version:        extPgVer,
		PGConfig:       extPgConfig,
		DefaultVersion: 0,
		BothSetError: func() error {
			return &extProbeError{
				Code: output.CodeExtensionInvalidArgs,
				Err:  fmt.Errorf("both pg version and pg_config path are specified, please specify only one"),
			}
		},
		PGConfigError: func(err error) error {
			return &extProbeError{
				Code: output.CodeExtensionPgConfigError,
				Err:  fmt.Errorf("failed to get PostgreSQL by pg_config path %s: %v", extPgConfig, err),
			}
		},
	})
}

func init() {
	extCmd.PersistentFlags().IntVarP(&extPgVer, "version", "v", 0, "specify a postgres by major version")
	extCmd.PersistentFlags().StringVarP(&extPgConfig, "path", "p", "", "specify a postgres by pg_config path")
	extCmd.PersistentFlags().BoolVar(&ext.ShowPkg, "pkg", false, "show Pkg instead of Name, only list lead extensions")

	extStatusCmd.Flags().BoolVarP(&extShowContrib, "contrib", "c", false, "show contrib extensions too")
	extAddCmd.Flags().BoolVarP(&extYes, "yes", "y", false, "auto confirm install")
	extAddCmd.Flags().BoolVar(&extAddPlan, "plan", false, "preview install plan without executing")
	extRmCmd.Flags().BoolVarP(&extYes, "yes", "y", false, "auto confirm removal")
	extRmCmd.Flags().BoolVar(&extRmPlan, "plan", false, "preview remove plan without executing")
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

func handleExtProbeError(err error) error {
	if err == nil {
		return nil
	}
	code := extProbeErrorCode(err)
	return handleAuxResult(output.Fail(code, err.Error()))
}
