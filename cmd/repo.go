package cmd

import (
	"fmt"
	"pig/cli/repo"
	"pig/internal/config"
	"pig/internal/output"

	"github.com/spf13/cobra"
)

var (
	repoRegion string
	repoUpdate bool
	repoRemove bool

	repoCacheDir string
	repoCachePkg string

	repoBootDir string
	repoBootPkg string
)

// repoCmd represents the top-level `repo` command
var repoCmd = &cobra.Command{
	Use:         "repo",
	Short:       "Manage Linux Software Repo (apt/dnf)",
	Aliases:     []string{"r"},
	GroupID:     "pgext",
	Annotations: ancsAnn("pig repo", "query", "stable", "safe", true, "safe", "none", "current", 100),
	Long: `pig repo - Manage Linux APT/YUM Repo

  pig repo list                    # available repo list             (info)
  pig repo info   [repo|module...] # show repo info                  (info)
  pig repo status                  # show current repo status        (info)
  pig repo add    [repo|module...] # add repo and modules            (root)
  pig repo rm     [repo|module...] # remove repo & modules           (root)
  pig repo update                  # update repo pkg cache           (root)
  pig repo create                  # create repo on current system   (root)
  pig repo boot                    # boot repo from offline package  (root)
  pig repo cache                   # cache repo as offline package   (root)
`,
	Example: `
  Get Started: https://pigsty.io/ext/pig/
  pig repo add -ru                 # add all repo and update cache (brute but effective)
  pig repo add pigsty -u           # gentle version, only add pigsty repo and update cache
  pig repo add node pgdg pigsty    # essential repo to install postgres packages
  pig repo add all                 # all = node + pgdg + pigsty
  pig repo add all extra           # extra module has non-free and some 3rd repo for certain extensions
  pig repo update                  # update repo cache
  pig repo create                  # update local repo /www/pigsty meta
  pig repo boot                    # extract /tmp/pkg.tgz to /www/pigsty
  pig repo cache                   # cache /www/pigsty into /tmp/pkg.tgz
`,
}

var repoListCmd = &cobra.Command{
	Use:          "list",
	Short:        "print available repo list",
	Aliases:      []string{"l", "ls"},
	SilenceUsage: true,
	Annotations:  ancsAnn("pig repo list", "query", "stable", "safe", true, "safe", "none", "current", 100),
	Example: `
  pig repo list                # list available repos on current system
  pig repo list all            # list all unfiltered repo raw data
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		showAll := len(args) > 0 && args[0] == "all"
		result := repo.ListRepos(showAll)
		return handleAuxResult(result)
	},
}

var repoInfoCmd = &cobra.Command{
	Use:          "info",
	Short:        "get repo detailed information",
	Aliases:      []string{"i"},
	SilenceUsage: true,
	Annotations:  ancsAnn("pig repo info", "query", "stable", "safe", true, "safe", "none", "current", 50),
	RunE: func(cmd *cobra.Command, args []string) error {
		result := repo.GetRepoInfo(args)
		return handleAuxResult(result)
	},
}

var repoAddCmd = &cobra.Command{
	Use:          "add",
	Short:        "add new repository",
	Aliases:      []string{"a", "append"},
	SilenceUsage: true,
	Annotations:  ancsAnn("pig repo add", "action", "stable", "safe", true, "low", "none", "root", 2000),
	Example: `
  pig repo add                      # = pig repo add all
  pig repo add all                  # add node,pgsql,infra repo (recommended)
  pig repo add all -u               # add above repo and update repo cache (or: --update)
  pig repo add all -r               # add all repo, remove old repos       (or: --remove)
  pig repo add pigsty --update      # add pigsty extension repo and update repo cache
  pig repo add pgdg --update        # add pgdg official repo and update repo cache
  pig repo add pgsql node --remove  # add os + postgres repo, remove old repos
  pig repo add infra                # add observability, grafana & prometheus stack, pg bin utils

  (Beware that system repo management require sudo / root privilege)

  available repo modules:
  - all      :  pgsql + node + infra (recommended)
    - pigsty :  PostgreSQL Extension Repo (default)
    - pgdg   :  PGDG the Official PostgreSQL Repo (official)
    - node   :  operating system official repo (el/debian/ubuntu)
  - pgsql    :  pigsty + pgdg (all available pg extensions) 
  # check available repo & modules with pig repo list
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if config.OSType != config.DistroEL && config.OSType != config.DistroDEB {
			result := output.Fail(output.CodeRepoUnsupportedOS,
				fmt.Sprintf("unsupported platform: %s %s", config.OSVendor, config.OSVersionFull))
			return handleAuxResult(result)
		}
		if len(args) == 0 {
			args = []string{"all"}
		}
		modules := repo.ExpandModuleArgs(args)
		result := repo.AddRepos(modules, repoRegion, repoRemove, repoUpdate)
		return handleAuxResult(result)
	},
}

var repoSetCmd = &cobra.Command{
	Use:          "set",
	Short:        "wipe, overwrite, and update repository",
	Aliases:      []string{"overwrite"},
	SilenceUsage: true,
	Annotations:  ancsAnn("pig repo set", "action", "stable", "restricted", true, "medium", "recommended", "root", 5000),
	Example: `
  pig repo set                      # set repo to node,pgsql,infra (= pig repo add all -ru)
  pig repo set node,pgsql           # use pgdg + pigsty repo
  pig repo set node,pgdg            # use pgdg and node repo only
  pig repo set node,pigsty          # use pgdg and node repo only
  pig repo set node,infra,mssql     # use wiltondb/babelfish repo

  (Beware that system repo management require sudo/root privilege)
	`,
	RunE: func(cmd *cobra.Command, args []string) error {
		repoRemove = true
		repoUpdate = true
		return repoAddCmd.RunE(cmd, args)
	},
}

var repoRmCmd = &cobra.Command{
	Use:          "rm",
	Short:        "remove repository",
	Aliases:      []string{"remove"},
	SilenceUsage: true,
	Annotations:  ancsAnn("pig repo rm", "action", "stable", "restricted", true, "medium", "recommended", "root", 2000),
	Example: `
  pig repo rm                      # remove (backup) all existing repo to backup dir
  pig repo rm all --update         # remove module 'all' and update repo cache
  pig repo rm node pigsty -u       # remove module 'node' & 'pigsty' and update repo cache
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if config.OSType != config.DistroEL && config.OSType != config.DistroDEB {
			result := output.Fail(output.CodeRepoUnsupportedOS,
				fmt.Sprintf("unsupported platform: %s %s", config.OSVendor, config.OSVersionFull))
			return handleAuxResult(result)
		}
		modules := repo.ExpandModuleArgs(args)
		result := repo.RmRepos(modules, repoUpdate)
		return handleAuxResult(result)
	},
}

var repoUpdateCmd = &cobra.Command{
	Use:          "update",
	Short:        "update repo cache",
	Aliases:      []string{"u"},
	SilenceUsage: true,
	Annotations:  ancsAnn("pig repo update", "action", "volatile", "safe", true, "safe", "none", "root", 10000),
	Example: `
  pig repo update                  # yum makecache or apt update
  `,
	RunE: func(cmd *cobra.Command, args []string) error {
		if config.OSType != config.DistroEL && config.OSType != config.DistroDEB {
			result := output.Fail(output.CodeRepoUnsupportedOS,
				fmt.Sprintf("unsupported platform: %s %s", config.OSVendor, config.OSVersionFull))
			return handleAuxResult(result)
		}
		result := repo.UpdateCache()
		return handleAuxResult(result)
	},
}

var repoStatusCmd = &cobra.Command{
	Use:          "status",
	Short:        "show current repo status",
	Aliases:      []string{"s", "st"},
	SilenceUsage: true,
	Annotations:  ancsAnn("pig repo status", "query", "volatile", "safe", true, "safe", "none", "current", 500),
	RunE: func(cmd *cobra.Command, args []string) error {
		result := repo.GetRepoStatus()
		return handleAuxResult(result)
	},
}

var repoBootCmd = &cobra.Command{
	Use:          "boot",
	Short:        "bootstrap repo from offline package",
	Aliases:      []string{"b", "bt"},
	SilenceUsage: true,
	Annotations:  ancsAnn("pig repo boot", "action", "stable", "restricted", true, "medium", "recommended", "root", 10000),
	Example: `
  pig repo boot                    # boot repo from /tmp/pkg.tgz to /www
  pig repo boot -p /tmp/pkg.tgz    # boot repo from given package path
  pig repo boot -d /srv            # boot repo to another directory /srv
  `,
	RunE: func(cmd *cobra.Command, args []string) error {
		result := repo.BootWithResult(repoBootPkg, repoBootDir)
		return handleAuxResult(result)
	},
}

var repoCacheCmd = &cobra.Command{
	Use:          "cache",
	Short:        "create offline package from local repo",
	Aliases:      []string{"c"},
	SilenceUsage: true,
	Annotations:  ancsAnn("pig repo cache", "action", "stable", "restricted", true, "low", "none", "root", 30000),
	Example: `
  pig repo cache                    # create /tmp/pkg.tgz offline package from /www/pigsty
  pig repo cache -f                 # force overwrite existing package
  pig repo cache -d /srv            # overwrite default content dir /www to /srv
  pig repo cache pigsty mssql       # create the tarball with both pigsty & mssql repo
  pig repo c -f                     # the simplest use case to make offline package

  (Beware that system repo management require sudo/root privilege)
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		repos := []string{"pigsty"}
		if len(args) > 0 {
			repos = args
		}
		result := repo.CacheWithResult(repoCacheDir, repoCachePkg, repos)
		return handleAuxResult(result)
	},
}

var repoCreateCmd = &cobra.Command{
	Use:          "create",
	Short:        "create local YUM/APT repository",
	Aliases:      []string{"cr"},
	SilenceUsage: true,
	Annotations:  ancsAnn("pig repo create", "action", "stable", "restricted", true, "low", "none", "root", 5000),
	Example: `
  pig repo create                    # create repo on /www/pigsty
  pig repo create /www/mssql /www/b  # create repo on multiple locations

  (Beware that system repo management require sudo/root privilege)
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		repos := []string{"/www/pigsty"}
		if len(args) > 0 {
			repos = args
		}
		result := repo.CreateReposWithResult(repos)
		return handleAuxResult(result)
	},
}

var repoReloadCmd = &cobra.Command{
	Use:          "reload",
	Short:        "reload repo catalog to the latest version",
	SilenceUsage: true,
	Aliases:      []string{"r", "re"},
	Annotations:  ancsAnn("pig repo reload", "action", "volatile", "safe", true, "safe", "none", "current", 3000),
	RunE: func(cmd *cobra.Command, args []string) error {
		result := repo.ReloadRepoCatalogWithResult()
		return handleAuxResult(result)
	},
}

func init() {
	repoAddCmd.Flags().StringVar(&repoRegion, "region", "", "region code")
	repoAddCmd.Flags().BoolVarP(&repoUpdate, "update", "u", false, "run apt update or dnf makecache")
	repoAddCmd.Flags().BoolVarP(&repoRemove, "remove", "r", false, "remove existing repo")
	repoSetCmd.Flags().StringVar(&repoRegion, "region", "", "region code")
	repoRmCmd.Flags().BoolVarP(&repoUpdate, "update", "u", false, "run apt update or dnf makecache")

	// boot command flags
	repoBootCmd.Flags().StringVarP(&repoBootDir, "dir", "d", "/www/", "target repo path")
	repoBootCmd.Flags().StringVarP(&repoBootPkg, "path", "p", "/tmp/pkg.tgz", "offline package path")

	// cache command flags
	repoCacheCmd.Flags().StringVarP(&repoCacheDir, "dir", "d", "/www/", "source repo path")
	repoCacheCmd.Flags().StringVarP(&repoCachePkg, "path", "p", "/tmp/pkg.tgz", "offline package path")

	repoCmd.AddCommand(repoAddCmd)
	repoCmd.AddCommand(repoSetCmd)
	repoCmd.AddCommand(repoRmCmd)
	repoCmd.AddCommand(repoListCmd)
	repoCmd.AddCommand(repoUpdateCmd)
	repoCmd.AddCommand(repoStatusCmd)
	repoCmd.AddCommand(repoInfoCmd)
	repoCmd.AddCommand(repoBootCmd)
	repoCmd.AddCommand(repoCacheCmd)
	repoCmd.AddCommand(repoCreateCmd)
	repoCmd.AddCommand(repoReloadCmd)
}
