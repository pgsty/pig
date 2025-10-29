package cmd

import (
	"pig/cli/build"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	buildPgrxVer   string
	buildPgrxPg    string
	buildDepPg     string
	buildPkgPg     string
	buildPkgSymbol bool
	buildRustYes   bool
)

// buildCmd represents the top-level `build` command
var buildCmd = &cobra.Command{
	Use:     "build",
	Short:   "Build Postgres Extension",
	Aliases: []string{"b"},
	GroupID: "pgext",
	Example: `pig build - Build Postgres Extension

  pig build repo                   # init build repo (=repo set -ru)
  pig build tool  [mini|full|...]  # init build toolset
  pig build proxy [id@host:port ]  # init build proxy (optional)
  pig build rust  [-y]             # install Rust toolchain
  pig build pgrx  [-v <ver>]       # install & init pgrx (0.16.1)
  pig build spec                   # init build spec repo
  pig build get   [ext|pkg|..]     # get extension source code tarball
  pig build dep   [ext|pkg...]     # install extension build deps
  pig build pkg   [ext|pkg]        # build extension package
  pig build all   [ext|pkg...]     # all = get + dep + pkg
`,
}

// buildRepoCmd represents the `build repo` command
var buildRepoCmd = &cobra.Command{
	Use:     "repo",
	Short:   "Initialize required repos",
	Aliases: []string{"r"},
	RunE: func(cmd *cobra.Command, args []string) error {
		repoRemove = true
		repoUpdate = true
		return repoAddCmd.RunE(cmd, args)
	},
}

// buildInitCmd represents the `build init` command
var buildToolCmd = &cobra.Command{
	Use:     "tool [mode]",
	Short:   "Initialize build tools",
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
	Use:     "proxy <id@remote> <local>",
	Short:   "Initialize build proxy",
	Args:    cobra.MaximumNArgs(2),
	Aliases: []string{"p"},
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
	Use:     "rust",
	Short:   "Install Rust toolchain",
	Aliases: []string{"rs"},
	RunE: func(cmd *cobra.Command, args []string) error {
		return build.SetupRust(buildRustYes)
	},
}

// buildPgrxCmd represents the `build pgrx` command
var buildPgrxCmd = &cobra.Command{
	Use:     "pgrx",
	Short:   "Install and initialize pgrx (requires Rust)",
	Aliases: []string{"px"},
	RunE: func(cmd *cobra.Command, args []string) error {
		return build.SetupPgrx(buildPgrxVer, buildPgrxPg)
	},
}

// buildSpecCmd represents the `build spec` command
var buildSpecCmd = &cobra.Command{
	Use:     "spec",
	Short:   "Initialize building spec repo",
	Aliases: []string{"s"},
	RunE: func(cmd *cobra.Command, args []string) error {
		return build.GetSpecRepo()
	},
}

// buildGetCmd represents the `build get` command
var buildGetCmd = &cobra.Command{
	Use:     "get",
	Short:   "Download source code tarball",
	Aliases: []string{"get"},
	RunE: func(cmd *cobra.Command, args []string) error {
		return build.DownloadCodeTarball(args)
	},
}

// buildDepCmd represents the `build dep` command
var buildDepCmd = &cobra.Command{
	Use:     "dep",
	Short:   "Install extension build dependencies",
	Aliases: []string{"d"},
	RunE: func(cmd *cobra.Command, args []string) error {
		return build.InstallExtensionDeps(args, buildDepPg)
	},
}

// buildExtCmd represents the `build ext` command
var buildExtCmd = &cobra.Command{
	Use:     "ext",
	Short:   "Build extension",
	Aliases: []string{"e"},
	RunE: func(cmd *cobra.Command, args []string) error {
		return build.BuildExtension(args)
	},
}

// buildPkgCmd represents the `build pkg` command
var buildPkgCmd = &cobra.Command{
	Use:     "pkg <extname>",
	Short:   "Build package for extension",
	Aliases: []string{"p"},
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return build.BuildPackage(args[0], buildPkgPg, buildPkgSymbol)
	},
}

func init() {
	// Parse build flags
	buildPgrxCmd.PersistentFlags().StringVarP(&buildPgrxVer, "pgrx", "v", "0.16.1", "pgrx version to install")
	buildRustCmd.PersistentFlags().BoolVarP(&buildRustYes, "yes", "y", false, "enforce rust re-installation")

	// Add pgrx specific flags
	buildPgrxCmd.Flags().StringVar(&buildPgrxPg, "pg", "", "comma-separated PG versions to init (e.g. '18,17,16'), 'init' for no args, or auto-detect if empty")

	// Add dep specific flags
	buildDepCmd.Flags().StringVar(&buildDepPg, "pg", "", "comma-separated PG versions (e.g. '17,16'), auto-detect from extension if empty")

	// Add pkg specific flags
	buildPkgCmd.Flags().StringVar(&buildPkgPg, "pg", "", "comma-separated PG versions (e.g. '17,16'), auto-detect from extension if empty")
	buildPkgCmd.Flags().BoolVarP(&buildPkgSymbol, "symbol", "s", false, "build with debug symbols (RPM only)")

	// Add subcommands
	buildCmd.AddCommand(buildRepoCmd)
	buildCmd.AddCommand(buildToolCmd)
	buildCmd.AddCommand(buildProxyCmd)
	buildCmd.AddCommand(buildRustCmd)
	buildCmd.AddCommand(buildPgrxCmd)
	buildCmd.AddCommand(buildSpecCmd)
	buildCmd.AddCommand(buildGetCmd)
	buildCmd.AddCommand(buildDepCmd)
	buildCmd.AddCommand(buildExtCmd)
	buildCmd.AddCommand(buildPkgCmd)
}
