package cmd

import (
	"fmt"
	"pig/cli/repo"
	"pig/internal/config"
	"pig/internal/utils"
	"strings"

	"github.com/sirupsen/logrus"
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

	repoPkgURL string
)

// repoCmd represents the top-level `repo` command
var repoCmd = &cobra.Command{
	Use:     "repo",
	Short:   "Manage Linux Software Repo (apt/dnf)",
	Aliases: []string{"r"},
	GroupID: "pgext",
	Long: `
typical usage:

  # info
  pig repo list                    # available repo list             (info)
  pig repo info   [repo|module...] # show repo info                  (info)
  pig repo status                  # show current repo status        (info)

  # admin
  pig repo add    [repo|module...] # add repo and modules            (root)
  pig repo set    [repo|module...] # overwrite existing repo and add (root)
  pig repo rm     [repo|module...] # remove repo & modules           (root)
  pig repo update                  # update repo pkg cache           (root)
  
  # cache
  pig repo create                  # create repo on current system   (root)
  pig repo boot                    # boot repo from offline package  (root)
  pig repo cache                   # cache repo as offline package   (root)
`,
}

var repoListCmd = &cobra.Command{
	Use:          "list",
	Short:        "print available repo list",
	Aliases:      []string{"l", "ls"},
	SilenceUsage: true,
	Example: `
  pig repo list                # list available repos on current system
  pig repo list all            # list all unfiltered repo raw data
  pig repo list update         # get updated repo data to ~/pig/repo.yml (TBD)
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return repo.List()
		} else if args[0] == "all" {
			return repo.ListAll()
		} else if args[0] == "update" {
			// TODO: implement repo update
			fmt.Println("not implemented yet")
		}
		return nil
	},
}

var repoInfoCmd = &cobra.Command{
	Use:          "info",
	Short:        "get repo detailed information",
	Aliases:      []string{"i"},
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			logrus.Errorf("repo or module name is required, check available repo list:")
			repo.ListAll()
			return fmt.Errorf("repo or module name is required")
		}
		return repo.Info(args...)
	},
}

var repoAddCmd = &cobra.Command{
	Use:          "add",
	Short:        "add new repository",
	Aliases:      []string{"a", "append"},
	SilenceUsage: true,
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
	// Long: moduleNotice,

	RunE: func(cmd *cobra.Command, args []string) error {
		if config.OSType != config.DistroEL && config.OSType != config.DistroDEB {
			return fmt.Errorf("unsupported platform: %s %s", config.OSVendor, config.OSVersionFull)
		}
		if len(args) == 0 {
			args = []string{"all"}
		}
		modules := repo.ExpandModuleArgs(args)
		manager, err := repo.NewManager()
		if err != nil {
			logrus.Errorf("failed to get repo manager: %v", err)
			return fmt.Errorf("failed to get repo manager: %v", err)
		}
		if repoRemove {
			logrus.Infof("move existing repo to backup dir")
			if err := manager.BackupRepo(); err != nil {
				logrus.Error(err)
				return fmt.Errorf("failed to backup repo: %v", err)
			}
		}

		if err := manager.AddModules(modules...); err != nil {
			logrus.Error(err)
			return fmt.Errorf("failed to add repo: %v", err)
		}

		utils.PadHeader("ls -l "+manager.RepoDir, 48)
		if err := utils.ShellCommand([]string{"ls", "-l", manager.RepoDir}); err != nil {
			logrus.Errorf("failed to list repo dir: %s", manager.RepoDir)
			return fmt.Errorf("failed to list repo dir: %s", manager.RepoDir)
		}

		if repoUpdate {
			if err := utils.SudoCommand(manager.UpdateCmd); err != nil {
				logrus.Error(err)
				return fmt.Errorf("failed to update repo: %v", err)
			}
		} else {
			logrus.Infof("repo added, run: sudo %s", strings.Join(manager.UpdateCmd, " "))
		}
		return nil
	},
}

var repoSetCmd = &cobra.Command{
	Use:     "set",
	Short:   "wipe and overwrite repository",
	Aliases: []string{"overwrite"},
	Example: `
  pig repo set all                  # set repo to node,pgsql,infra  (recommended)
  pig repo set all -u               # set repo to above repo and update repo cache (or --update)
  pig repo set pigsty --update      # set repo to pigsty extension repo and update repo cache
  pig repo set pgdg   --update      # set repo to pgdg official repo and update repo cache
  pig repo set infra                # set repo to observability, grafana & prometheus stack, pg bin utils

  (Beware that system repo management require sudo/root privilege)
	`,
	RunE: func(cmd *cobra.Command, args []string) error {
		repoRemove = true
		return repoAddCmd.RunE(cmd, args)
	},
}

var repoRmCmd = &cobra.Command{
	Use:          "rm",
	Short:        "remove repository",
	Aliases:      []string{"remove"},
	SilenceUsage: true,
	Example: `
  pig repo rm                      # remove (backup) all existing repo to backup dir
  pig repo rm all --update         # remove module 'all' and update repo cache
  pig repo rm node pigsty -u       # remove module 'node' & 'pigsty' and update repo cache
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		modules := repo.ExpandModuleArgs(args)
		manager, err := repo.NewManager()
		if err != nil {
			logrus.Errorf("failed to get repo manager: %v", err)
			return fmt.Errorf("failed to get repo manager: %v", err)

		}
		if len(modules) == 0 {
			logrus.Debugf("repo remove called with no args, remove all modules & repos")
			if err := manager.BackupRepo(); err != nil {
				logrus.Error(err)
				return err
			}
			return nil
		} else {
			for _, module := range modules {
				if err := manager.RemoveRepo(module); err != nil {
					logrus.Error(err)
					return err
				}
			}
		}

		if repoUpdate {
			if err := utils.SudoCommand(manager.UpdateCmd); err != nil {
				logrus.Error(err)
				return err
			}
		}
		return nil
	},
}

var repoUpdateCmd = &cobra.Command{
	Use:          "update",
	Short:        "update repo cache",
	Aliases:      []string{"u"},
	SilenceUsage: true,
	Example: `
  pig repo update                  # yum makecache or apt update
  `,
	RunE: func(cmd *cobra.Command, args []string) error {
		manager, err := repo.NewManager()
		if err != nil {
			logrus.Errorf("failed to get repo manager: %v", err)
			return fmt.Errorf("failed to get repo manager: %v", err)

		}
		if err := utils.SudoCommand(manager.UpdateCmd); err != nil {
			logrus.Error(err)
			return fmt.Errorf("failed to update repo: %v", err)
		}
		return nil
	},
}

var repoStatusCmd = &cobra.Command{
	Use:          "status",
	Short:        "show current repo status",
	Aliases:      []string{"s", "st"},
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return repo.Status()
	},
}

var repoBootCmd = &cobra.Command{
	Use:          "boot",
	Short:        "bootstrap repo from offline package",
	Aliases:      []string{"b", "bt"},
	SilenceUsage: true,
	Example: `
  pig repo boot                    # boot repo from /tmp/pkg.tgz to /www
  pig repo boot -p /tmp/pkg.tgz    # boot repo from given package path
  pig repo boot -d /srv            # boot repo to another directory /srv
  `,
	RunE: func(cmd *cobra.Command, args []string) error {
		return repo.Boot(repoBootPkg, repoBootDir)
	},
}

var repoCacheCmd = &cobra.Command{
	Use:          "cache",
	Short:        "create offline package from local repo",
	Aliases:      []string{"c"},
	SilenceUsage: true,
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
		return repo.Cache(repoCacheDir, repoCachePkg, repos)
	},
}

var repoCreateCmd = &cobra.Command{
	Use:          "create",
	Short:        "create local YUM/APT repository",
	Aliases:      []string{"cr"},
	SilenceUsage: true,
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
		return repo.CreateRepos(repos...)
	},
}

var repoFetchCmd = &cobra.Command{
	Use:          "fetch",
	Short:        "fetch offline package from Pigsty Github",
	Aliases:      []string{"f"},
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		logrus.Errorf("not implemented yet")
		return nil
	},
}

func init() {
	repoAddCmd.Flags().StringVar(&repoRegion, "region", "", "region code")
	repoAddCmd.Flags().BoolVarP(&repoUpdate, "update", "u", false, "run apt update or dnf makecache")
	repoAddCmd.Flags().BoolVarP(&repoRemove, "remove", "r", false, "remove existing repo")

	repoSetCmd.Flags().StringVar(&repoRegion, "region", "", "region code")
	repoSetCmd.Flags().BoolVarP(&repoUpdate, "update", "u", false, "run apt update or dnf makecache")

	repoRmCmd.Flags().BoolVarP(&repoUpdate, "update", "u", false, "run apt update or dnf makecache")

	// boot command flags
	repoBootCmd.Flags().StringVarP(&repoBootDir, "dir", "d", "/www/", "target repo path")
	repoBootCmd.Flags().StringVarP(&repoBootPkg, "path", "p", "/tmp/pkg.tgz", "offline package path")

	// cache command flags
	repoCacheCmd.Flags().StringVarP(&repoCacheDir, "dir", "d", "/www/", "source repo path")
	repoCacheCmd.Flags().StringVarP(&repoCachePkg, "path", "p", "/tmp/pkg.tgz", "offline package path")

	repoFetchCmd.Flags().StringVarP(&repoPkgURL, "url", "u", "", "package URL (default: latest from Pigsty Github)")

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
	repoCmd.AddCommand(repoFetchCmd)
}
