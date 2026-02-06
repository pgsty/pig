package cmd

import (
	"pig/cli/build"
	"pig/cli/ext"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	buildPgrxVer   string
	buildPgrxPg    string
	buildDepPg     string
	buildPkgPg     string
	buildPkgSymbol bool
	buildAllPg     string
	buildAllSymbol bool
	buildGetForce  bool
	buildSpecForce bool
	buildRustYes   bool
	buildMirror    bool
)

// buildCmd represents the top-level `build` command
var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build Postgres Extension",
	Annotations: map[string]string{
		"name":       "pig build",
		"type":       "query",
		"volatility": "stable",
		"parallel":   "safe",
		"idempotent": "true",
		"risk":       "safe",
		"confirm":    "none",
		"os_user":    "current",
		"cost":       "100",
	},
	Aliases: []string{"b"},
	GroupID: "pgext",
	Example: `pig build - Build Postgres Extension

Environment Setup:
  pig build spec                   # init build spec and directory (~ext)
  pig build repo                   # init build repo (=repo set -ru)
  pig build tool  [mini|full|...]  # init build toolset
  pig build rust  [-y]             # install Rust toolchain
  pig build pgrx  [-v <ver>]       # install & init pgrx (0.16.1)
  pig build proxy [id@host:port ]  # init build proxy (optional)

Package Building:
  pig build pkg   [ext|pkg...]     # complete pipeline: get + dep + ext
  pig build get   [ext|pkg...]     # download extension source tarball
  pig build dep   [ext|pkg...]     # install extension build dependencies
  pig build ext   [ext|pkg...]     # build extension package

Quick Start:
  pig build spec                   # setup build spec and directory
  pig build pkg citus              # build citus extension

`, PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if err := initAll(); err != nil {
			return err
		}
		applyStructuredOutputSilence(cmd)
		return ext.ReloadCatalog()
	},
}

// buildRepoCmd represents the `build repo` command
var buildRepoCmd = &cobra.Command{
	Use:   "repo",
	Short: "Initialize required repos",
	Annotations: map[string]string{
		"name":       "pig build repo",
		"type":       "action",
		"volatility": "volatile",
		"parallel":   "unsafe",
		"idempotent": "true",
		"risk":       "low",
		"confirm":    "none",
		"os_user":    "root",
		"cost":       "60000",
	},
	Aliases: []string{"r"},
	RunE: func(cmd *cobra.Command, args []string) error {
		repoRemove = true
		repoUpdate = true
		return repoSetCmd.RunE(cmd, args)
	},
}

// buildInitCmd represents the `build init` command
var buildToolCmd = &cobra.Command{
	Use:   "tool [mode]",
	Short: "Initialize build tools",
	Annotations: map[string]string{
		"name":       "pig build tool",
		"type":       "action",
		"volatility": "volatile",
		"parallel":   "unsafe",
		"idempotent": "true",
		"risk":       "low",
		"confirm":    "none",
		"os_user":    "root",
		"cost":       "60000",
	},
	Aliases: []string{"t"},
	RunE: func(cmd *cobra.Command, args []string) error {
		var mode string
		if len(args) > 0 {
			mode = args[0]
		}
		logrus.Infof("Init pg ext build env in %s mode", mode)
		return build.InstallBuildTools(mode)
	},
}

// buildProxyCmd represents the `build proxy` command
var buildProxyCmd = &cobra.Command{
	Use:   "proxy <id@remote> <local>",
	Short: "Initialize build proxy",
	Annotations: map[string]string{
		"name":       "pig build proxy",
		"type":       "action",
		"volatility": "stable",
		"parallel":   "safe",
		"idempotent": "true",
		"risk":       "low",
		"confirm":    "none",
		"os_user":    "current",
		"cost":       "5000",
	},
	Args:    cobra.MaximumNArgs(2),
	Aliases: []string{"x"},
	RunE: func(cmd *cobra.Command, args []string) error {
		var remote, local string
		if len(args) == 0 {
			return build.InstallProxy()
		} else {
			if len(args) == 2 {
				remote, local = args[0], args[1]
			} else {
				remote, local = args[0], "127.0.0.1:12345"
			}
		}
		return build.SetupProxy(remote, local)
	},
}

// buildRustCmd represents the `build rust` command
var buildRustCmd = &cobra.Command{
	Use:   "rust",
	Short: "Install Rust toolchain",
	Annotations: map[string]string{
		"name":       "pig build rust",
		"type":       "action",
		"volatility": "volatile",
		"parallel":   "unsafe",
		"idempotent": "true",
		"risk":       "low",
		"confirm":    "none",
		"os_user":    "root",
		"cost":       "60000",
	},
	Aliases: []string{"rs"},
	RunE: func(cmd *cobra.Command, args []string) error {
		return build.SetupRust(buildRustYes)
	},
}

// buildPgrxCmd represents the `build pgrx` command
var buildPgrxCmd = &cobra.Command{
	Use:   "pgrx",
	Short: "Install and initialize pgrx (requires Rust)",
	Annotations: map[string]string{
		"name":       "pig build pgrx",
		"type":       "action",
		"volatility": "volatile",
		"parallel":   "unsafe",
		"idempotent": "true",
		"risk":       "low",
		"confirm":    "none",
		"os_user":    "root",
		"cost":       "60000",
	},
	Aliases: []string{"rx"},
	RunE: func(cmd *cobra.Command, args []string) error {
		return build.SetupPgrx(buildPgrxVer, buildPgrxPg)
	},
}

// buildSpecCmd represents the `build spec` command
var buildSpecCmd = &cobra.Command{
	Use:   "spec",
	Short: "Initialize building spec repo",
	Annotations: map[string]string{
		"name":       "pig build spec",
		"type":       "action",
		"volatility": "volatile",
		"parallel":   "safe",
		"idempotent": "true",
		"risk":       "low",
		"confirm":    "none",
		"os_user":    "root",
		"cost":       "60000",
	},
	Long:    "Download and sync build spec repository via tarball and incremental rsync",
	Aliases: []string{"s"},
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return build.SpecDirSetup(buildSpecForce, buildMirror)
	},
}

// buildGetCmd represents the `build get` command
var buildGetCmd = &cobra.Command{
	Use:   "get <pkg...>",
	Short: "Download source code tarball",
	Annotations: map[string]string{
		"name":       "pig build get",
		"type":       "action",
		"volatility": "volatile",
		"parallel":   "safe",
		"idempotent": "true",
		"risk":       "low",
		"confirm":    "none",
		"os_user":    "root",
		"cost":       "300000",
	},
	Aliases: []string{"g"},
	Args:    cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return build.DownloadSources(args, buildGetForce, buildMirror)
	},
}

// buildDepCmd represents the `build dep` command
var buildDepCmd = &cobra.Command{
	Use:   "dep <pkg...>",
	Short: "Install extension build dependencies",
	Annotations: map[string]string{
		"name":       "pig build dep",
		"type":       "action",
		"volatility": "volatile",
		"parallel":   "unsafe",
		"idempotent": "true",
		"risk":       "low",
		"confirm":    "none",
		"os_user":    "root",
		"cost":       "300000",
	},
	Aliases: []string{"d"},
	Args:    cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return build.InstallDepsList(args, buildDepPg)
	},
}

// buildExtCmd represents the `build ext` command
var buildExtCmd = &cobra.Command{
	Use:   "ext <pkg...>",
	Short: "Build extension package",
	Annotations: map[string]string{
		"name":       "pig build ext",
		"type":       "action",
		"volatility": "volatile",
		"parallel":   "unsafe",
		"idempotent": "true",
		"risk":       "low",
		"confirm":    "none",
		"os_user":    "root",
		"cost":       "300000",
	},
	Aliases: []string{"e"},
	Args:    cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return build.BuildExtensions(args, buildPkgPg, buildPkgSymbol)
	},
}

// buildPkgCmd represents the `build pkg` command
var buildPkgCmd = &cobra.Command{
	Use:   "pkg <pkg...>",
	Short: "Complete build pipeline: get, dep, ext",
	Annotations: map[string]string{
		"name":       "pig build pkg",
		"type":       "action",
		"volatility": "volatile",
		"parallel":   "unsafe",
		"idempotent": "true",
		"risk":       "low",
		"confirm":    "none",
		"os_user":    "root",
		"cost":       "300000",
	},
	Long:    "Execute complete build pipeline for extensions: download source, install dependencies, and build package",
	Aliases: []string{"p"},
	Args:    cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return build.BuildPackages(args, buildAllPg, buildAllSymbol)
	},
}

func init() {
	// Parse build flags
	buildPgrxCmd.PersistentFlags().StringVarP(&buildPgrxVer, "pgrx", "v", "0.16.1", "pgrx version to install")
	buildRustCmd.PersistentFlags().BoolVarP(&buildRustYes, "yes", "y", false, "enforce rust re-installation")

	// Add pgrx specific flags
	buildPgrxCmd.Flags().StringVar(&buildPgrxPg, "pg", "", "comma-separated PG versions to init (e.g. '18,17,16'), 'init' for no args, or auto-detect if empty")

	// Add spec specific flags
	buildSpecCmd.Flags().BoolVarP(&buildSpecForce, "force", "f", false, "force re-download tarball even if exists")
	buildSpecCmd.Flags().BoolVarP(&buildMirror, "mirror", "m", false, "use mirror (pigsty.cc) instead of default")

	// Add get specific flags
	buildGetCmd.Flags().BoolVarP(&buildGetForce, "force", "f", false, "force download even if file exists")
	buildGetCmd.Flags().BoolVarP(&buildMirror, "mirror", "m", false, "use mirror (pigsty.cc) instead of default")

	// Add dep specific flags
	buildDepCmd.Flags().StringVar(&buildDepPg, "pg", "", "comma-separated PG versions (e.g. '17,16'), auto-detect from extension if empty")

	// Add ext specific flags (formerly pkg)
	buildExtCmd.Flags().StringVar(&buildPkgPg, "pg", "", "comma-separated PG versions (e.g. '17,16'), auto-detect from extension if empty")
	buildExtCmd.Flags().BoolVarP(&buildPkgSymbol, "symbol", "s", false, "build with debug symbols (RPM only)")

	// Add pkg specific flags (formerly all)
	buildPkgCmd.Flags().StringVar(&buildAllPg, "pg", "", "comma-separated PG versions (e.g. '17,16'), auto-detect from extension if empty")
	buildPkgCmd.Flags().BoolVarP(&buildAllSymbol, "symbol", "s", false, "build with debug symbols (RPM only)")

	// Add subcommands
	buildCmd.AddCommand(buildSpecCmd)
	buildCmd.AddCommand(buildRepoCmd)
	buildCmd.AddCommand(buildToolCmd)
	buildCmd.AddCommand(buildRustCmd)
	buildCmd.AddCommand(buildPgrxCmd)
	buildCmd.AddCommand(buildProxyCmd)
	buildCmd.AddCommand(buildGetCmd)
	buildCmd.AddCommand(buildDepCmd)
	buildCmd.AddCommand(buildExtCmd)
	buildCmd.AddCommand(buildPkgCmd)
}
