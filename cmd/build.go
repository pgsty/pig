package cmd

import (
	"fmt"
	"pig/cli/build"

	"github.com/sirupsen/logrus"

	"github.com/spf13/cobra"
)

var (
	buildPgrxVer string
)

// buildCmd represents the top-level `build` command
var buildCmd = &cobra.Command{
	Use:     "build",
	Short:   "Build Postgres Extension",
	Aliases: []string{"b"},
	Example: `pig build - Build Postgres Extension

  pig build repo                   # init build repo (=repo set -ru)
  pig build tool  [mini|full|...]  # init build toolset
  pig build proxy [user@host:port] # init build proxy (optional)
  pig build rust  [-v <pgrx_ver>]  # init rustc & pgrx (0.12.9)
  pig build spec                   # init build spec repo
  pig build code  [all|prefix]     # download extension source code
  pig build deps  [extname...]     # init build dependencies   (TBD)
  pig build deps  [extname...]     # init build dependencies   (TBD)
  pig build new   [extname...]     # new extension boilerplate (TBD)
  pig build ext   [extname...] -v  # build extensions          (TBD)
  pig build list  [extname...]     # list built extensions     (TBD)
  pig build pub   [extname...]     # publish extensions        (TBD)
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
			return fmt.Errorf("proxy cred 'id@host:port' is required as the first argument")
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
		return build.SetupRust(buildPgrxVer)
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

// buildCodeCmd represents the `build code` command
var buildCodeCmd = &cobra.Command{
	Use:     "code",
	Short:   "Download extension source code tarball",
	Aliases: []string{"c"},
	RunE: func(cmd *cobra.Command, args []string) error {
		return build.DownloadSourceTarball(args)
	},
}

func init() {
	rootCmd.AddCommand(buildCmd)
	// Add subcommands
	buildCmd.AddCommand(buildRepoCmd)
	buildCmd.AddCommand(buildToolCmd)
	buildCmd.AddCommand(buildProxyCmd)
	buildCmd.AddCommand(buildRustCmd)
	buildCmd.AddCommand(buildSpecCmd)
	buildCmd.AddCommand(buildCodeCmd)
}
