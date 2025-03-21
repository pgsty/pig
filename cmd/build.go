package cmd

import (
	"pig/cli/build"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	buildPgrxVer string
	buildRustYes bool
)

// buildCmd represents the top-level `build` command
var buildCmd = &cobra.Command{
	Use:     "build",
	Short:   "Build Postgres Extension",
	Aliases: []string{"b"},
	Example: `pig build - Build Postgres Extension

  pig build repo                   # init build repo (=repo set -ru)
  pig build tool  [mini|full|...]  # init build toolset
  pig build proxy [id@host:port ]  # init build proxy (optional)
  pig build rust  [-v <pgrx_ver>]  # init rustc & pgrx (0.13.1)
  pig build spec                   # init build spec repo
  pig build get   [all|std|..]     # get ext code tarball with prefixes
  pig build ext   [extname...]     # build extension
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
		mode := "mini"
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
	Short:   "Initialize rust and pgrx environment",
	Aliases: []string{"rs"},
	RunE: func(cmd *cobra.Command, args []string) error {
		return build.SetupRust(buildPgrxVer, buildRustYes)
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

// buildExtCmd represents the `build ext` command
var buildExtCmd = &cobra.Command{
	Use:     "ext",
	Short:   "Build extension",
	Aliases: []string{"e"},
	RunE: func(cmd *cobra.Command, args []string) error {
		return build.BuildExtension(args)
	},
}

func init() {
	rootCmd.AddCommand(buildCmd)

	// Parse build flags
	buildCmd.PersistentFlags().StringVarP(&buildPgrxVer, "pgrx", "v", "0.13.1", "pgrx version to install")
	buildCmd.PersistentFlags().BoolVarP(&buildRustYes, "yes", "y", false, "enforce rust re-installation")

	// Add subcommands
	buildCmd.AddCommand(buildRepoCmd)
	buildCmd.AddCommand(buildToolCmd)
	buildCmd.AddCommand(buildProxyCmd)
	buildCmd.AddCommand(buildRustCmd)
	buildCmd.AddCommand(buildSpecCmd)
	buildCmd.AddCommand(buildGetCmd)
	buildCmd.AddCommand(buildExtCmd)
}
