package cmd

import (
	"pig/cli/repo"

	"github.com/spf13/cobra"
)

var (
	repoAddRegion string
	repoAddMirror bool
	repoAddUpdate bool
	repoAddRemove bool
	repoSetRegion string
	repoSetMirror bool
	repoRmUpdate  bool

	repoCacheDir string
	repoCachePkg string

	repoBootDir string
	repoBootPkg string
)

// repoCmd represents the top-level `repo` command
var repoCmd = &cobra.Command{
	Use:         "repo",
	Short:       "Manage Linux software repositories",
	Aliases:     []string{"r"},
	GroupID:     "pgext",
	Annotations: ancsAnn("pig repo", "query", "stable", "safe", true, "safe", "none", "current", 100),
	Long: `pig repo - Manage Linux APT/RPM Repositories

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
  Get Started: https://pig.pgsty.com/repo/
  pig repo add -ru                 # add all repo and update cache (brute but effective)
  pig repo add pigsty -u           # gentle version, only add pigsty repo and update cache
  pig repo add node pgdg pigsty    # essential repo to install postgres packages
  pig repo add all                 # all = node + pgdg + pigsty
  pig repo add all extra           # extra module has non-free and some 3rd repo for certain extensions
  pig repo update                  # update repo cache
  pig repo create                  # update local repository metadata
  pig repo boot                    # extract /tmp/pkg.tgz to /www/pigsty
  pig repo cache                   # cache /www/pigsty into /tmp/pkg.tgz
`,
}

var repoListCmd = &cobra.Command{
	Use:          "list [all]",
	Short:        "print available repo list",
	Aliases:      []string{"l", "ls"},
	SilenceUsage: true,
	Annotations:  ancsAnn("pig repo list", "query", "stable", "safe", true, "safe", "none", "current", 100),
	ValidArgs:    []string{"all"},
	Args:         cobra.MatchAll(cobra.MaximumNArgs(1), cobra.OnlyValidArgs),
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
	Use:          "info [repo|module...]",
	Short:        "get repo detailed information",
	Aliases:      []string{"i"},
	SilenceUsage: true,
	Annotations:  ancsAnn("pig repo info", "query", "stable", "safe", true, "safe", "none", "current", 50),
	Args:         cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		result := repo.GetRepoInfo(args)
		return handleAuxResult(result)
	},
}

var repoAddCmd = &cobra.Command{
	Use:          "add [module...]",
	Short:        "add new repository",
	Aliases:      []string{"a", "append"},
	SilenceUsage: true,
	Annotations:  ancsAnn("pig repo add", "action", "stable", "safe", true, "low", "none", "root", 2000),
	Args:         cobra.ArbitraryArgs,
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
		result := repo.ConfigureRepos(args, repo.AddOptions{
			Region: repoAddRegion,
			Remove: repoAddRemove,
			Update: repoAddUpdate,
			Mirror: repoAddMirror,
		})
		return handleAuxResult(result)
	},
}

var repoSetCmd = &cobra.Command{
	Use:          "set [module...]",
	Short:        "wipe, overwrite, and update repository",
	Aliases:      []string{"overwrite"},
	SilenceUsage: true,
	Annotations:  ancsAnn("pig repo set", "action", "stable", "restricted", true, "medium", "recommended", "root", 5000),
	Args:         cobra.ArbitraryArgs,
	Example: `
  pig repo set                      # set repo to node,pgsql,infra (= pig repo add all -ru)
  pig repo set node,pgsql           # use pgdg + pigsty repo
  pig repo set node,pgdg            # use pgdg and node repo only
  pig repo set node,pigsty          # use pgdg and node repo only
  pig repo set node,infra,docker    # enable docker-ce repo alongside base repos

  (Beware that system repo management require sudo/root privilege)
	`,
	RunE: func(cmd *cobra.Command, args []string) error {
		result := repo.ConfigureRepos(args, repo.AddOptions{
			Region: repoSetRegion,
			Remove: true,
			Update: true,
			Mirror: repoSetMirror,
		})
		return handleAuxResult(result)
	},
}

var repoRmCmd = &cobra.Command{
	Use:          "rm [module...]",
	Short:        "remove repository",
	Aliases:      []string{"remove"},
	SilenceUsage: true,
	Annotations:  ancsAnn("pig repo rm", "action", "stable", "restricted", true, "medium", "recommended", "root", 2000),
	Args:         cobra.ArbitraryArgs,
	Example: `
  pig repo rm                      # remove (backup) all existing repo to backup dir
  pig repo rm all --update         # remove module 'all' and update repo cache
  pig repo rm node pigsty -u       # remove module 'node' & 'pigsty' and update repo cache
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		modules := repo.ExpandModuleArgs(args)
		result := repo.RmRepos(modules, repoRmUpdate)
		return handleAuxResult(result)
	},
}

var repoUpdateCmd = &cobra.Command{
	Use:          "update",
	Short:        "update repo cache",
	Aliases:      []string{"u"},
	SilenceUsage: true,
	Annotations:  ancsAnn("pig repo update", "action", "volatile", "safe", true, "safe", "none", "root", 10000),
	Args:         cobra.NoArgs,
	Example: `
  pig repo update                  # dnf makecache (yum on EL7) or apt-get update
  `,
	RunE: func(cmd *cobra.Command, args []string) error {
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
	Args:         cobra.NoArgs,
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
	Args:         cobra.NoArgs,
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
	Use:          "cache [repo...]",
	Short:        "create offline package from local repo",
	Aliases:      []string{"c"},
	SilenceUsage: true,
	Annotations:  ancsAnn("pig repo cache", "action", "stable", "restricted", true, "low", "none", "root", 30000),
	Args:         cobra.ArbitraryArgs,
	Example: `
  pig repo cache                    # create /tmp/pkg.tgz offline package from /www/pigsty
  pig repo cache -d /srv            # overwrite default content dir /www to /srv
  pig repo cache pigsty docker      # create the tarball with both pigsty & docker repo
  pig repo c                        # the simplest use case to make offline package

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
	Use:          "create [path...]",
	Short:        "create local RPM/DEB repository",
	Aliases:      []string{"cr"},
	SilenceUsage: true,
	Annotations:  ancsAnn("pig repo create", "action", "stable", "restricted", true, "low", "none", "root", 5000),
	Args:         cobra.ArbitraryArgs,
	Example: `
  pig repo create                    # current dir on macOS, /www/pigsty on Linux
  pig repo create /www/pigsty /www/b # create repo on multiple locations

  (Linux defaults to /www/pigsty and requires sudo/root.
   macOS defaults to the current directory and runs as the current user.)
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		result := repo.CreateReposWithResult(args)
		return handleAuxResult(result)
	},
}

var repoReloadCmd = &cobra.Command{
	Use:          "reload",
	Short:        "reload repo catalog to the latest version",
	SilenceUsage: true,
	Aliases:      []string{"r", "re"},
	Annotations:  ancsAnn("pig repo reload", "action", "volatile", "safe", true, "safe", "none", "current", 3000),
	Args:         cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		result := repo.ReloadRepoCatalogWithResult()
		return handleAuxResult(result)
	},
}

func init() {
	repoAddCmd.Flags().StringVar(&repoAddRegion, "region", "", "region code")
	repoAddCmd.Flags().BoolVarP(&repoAddUpdate, "update", "u", false, "run apt update or dnf makecache")
	repoAddCmd.Flags().BoolVarP(&repoAddRemove, "remove", "r", false, "remove existing repo")
	repoAddCmd.Flags().BoolVarP(&repoAddMirror, "mirror", "m", false, "prefer China mirrors")
	repoSetCmd.Flags().StringVar(&repoSetRegion, "region", "", "region code")
	repoSetCmd.Flags().BoolVarP(&repoSetMirror, "mirror", "m", false, "prefer China mirrors")
	repoRmCmd.Flags().BoolVarP(&repoRmUpdate, "update", "u", false, "run apt update or dnf makecache")

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
