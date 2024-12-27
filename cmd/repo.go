package cmd

import (
	"fmt"
	"os"
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
  pig repo list                  # available repo list             (info)
  pig repo info [repo...]        # show repo info                  (info)
  pig repo status                # show current repo status        (info)

  # admin
  pig repo add  [repo|module...] # add repo and modules            (root)
  pig repo set  [repo|module...] # overwrite existing repo and add (root)
  pig repo rm   [repo|module...] # remove repo & modules           (root)
  pig repo update                # update repo pkg cache           (root)
  
  # cache
  pig repo create                # create repo on current system   (root) TBD 
  pig repo setup [-p]            # setup repo from offline package (root) TBD
  pig repo cache                 # cache repo as offline package   (root) TBD
  pig repo fetch                 # get pre-made offline package    (root) TBD PRO

`,
}

var repoListCmd = &cobra.Command{
	Use:     "list",
	Short:   "print available repo list",
	Aliases: []string{"l", "ls"},
	Example: `
  pig repo list                # list available repos on current system
  pig repo list all            # list all unfiltered repo raw data
  pig repo list update         # get updated repo data to ~/pig/repo.yml (TBD)
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return repo.ListRepo()
		} else if args[0] == "all" {
			repo.ListRepoData()
		} else if args[0] == "update" {
			// TODO: implement repo update
			fmt.Println("not implemented yet")
		}
		return nil
	},
}

var repoAddCmd = &cobra.Command{
	Use:     "add",
	Short:   "add new repository",
	Aliases: []string{"a", "append"},
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
		cmd.SilenceUsage = true
		if len(args) == 0 {
			args = []string{"all"}
		}
		var repoDir string
		var updateCmd []string
		switch config.OSType {
		case config.DistroEL:
			repoDir, updateCmd = "/etc/yum.repos.d/", []string{"yum", "makecache"}
		case config.DistroDEB:
			repoDir, updateCmd = "/etc/apt/sources.list.d/", []string{"apt-get", "update"}
		default:
			logrus.Errorf("unsupported OS type: %s", config.OSType)
			return fmt.Errorf("unsupported OS type: %s", config.OSType)
			// os.Exit(1)
		}
		manager, err := repo.NewRepoManager()
		if err != nil {
			logrus.Errorf("failed to get repo manager: %v", err)
			return fmt.Errorf("failed to get repo manager: %v", err)
			// os.Exit(1)
		}
		if repoRemove {
			logrus.Infof("move existing repo to backup dir")
			if err := manager.BackupRepo(); err != nil {
				logrus.Error(err)
				return fmt.Errorf("failed to backup repo: %v", err)
				// os.Exit(1)
			}
		}

		if err := manager.AddModules(args...); err != nil {
			logrus.Error(err)
			return fmt.Errorf("failed to add repo: %v", err)
			// os.Exit(1)
		}

		fmt.Printf("======== ls %s\n", repoDir)
		if err := utils.ShellCommand([]string{"ls", "-l", repoDir}); err != nil {
			logrus.Errorf("failed to list repo dir: %s", repoDir)
			return fmt.Errorf("failed to list repo dir: %s", repoDir)
			// os.Exit(1)
		}

		if repoUpdate {
			if err := utils.SudoCommand(updateCmd); err != nil {
				logrus.Error(err)
				return fmt.Errorf("failed to update repo: %v", err)
				// os.Exit(1)
			}
		} else {
			logrus.Infof("repo added, run: sudo %s", strings.Join(updateCmd, " "))
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
	Run: func(cmd *cobra.Command, args []string) {
		repoRemove = true
		repoAddCmd.Run(cmd, args)
	},
}

var repoRmCmd = &cobra.Command{
	Use:     "rm",
	Short:   "remove repository",
	Aliases: []string{"remove"},
	Run: func(cmd *cobra.Command, args []string) {
		// if len(args) == 0 {
		// 	err := repo.BackupRepo()
		// 	if err != nil {
		// 		logrus.Error(err)
		// 		os.Exit(1)
		// 	}
		// 	return
		// }
		// // err := repo.RemoveRepo(args...)
		// if err != nil {
		// 	logrus.Error(err)
		// 	os.Exit(1)
		// }

		// if repoUpdate {
		// 	var updateCmd []string
		// 	if config.OSType == config.DistroEL {
		// 		updateCmd = []string{"yum", "makecache"}
		// 	} else if config.OSType == config.DistroDEB {
		// 		updateCmd = []string{"apt-get", "update"}
		// 	} else {
		// 		logrus.Errorf("unsupported OS type: %s", config.OSType)
		// 		os.Exit(1)
		// 	}

		// 	err = utils.SudoCommand(updateCmd)
		// 	if err != nil {
		// 		logrus.Error(err)
		// 		os.Exit(1)
		// 	}
		// }
	},
}

var repoUpdateCmd = &cobra.Command{
	Use:     "update",
	Short:   "update repo cache",
	Aliases: []string{"u"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if config.OSType == config.DistroEL {
			err := utils.SudoCommand([]string{"yum", "makecache"})
			if err != nil {
				logrus.Error(err)
				os.Exit(1)
			}
		} else if config.OSType == config.DistroDEB {
			err := utils.SudoCommand([]string{"apt", "update"})
			if err != nil {
				logrus.Error(err)
				os.Exit(1)
			}
		} else {
			logrus.Errorf("unsupported OS type: %s", config.OSType)
			os.Exit(1)
		}
		return nil
	},
}

var repoStatusCmd = &cobra.Command{
	Use:     "status",
	Short:   "show current repo status",
	Aliases: []string{"s", "st"},
	RunE: func(cmd *cobra.Command, args []string) error {
		return repo.RepoStatus()
	},
}

func init() {
	repoAddCmd.Flags().StringVar(&repoRegion, "region", "", "region code")
	repoAddCmd.Flags().BoolVarP(&repoUpdate, "update", "u", false, "run apt update or dnf makecache")
	repoAddCmd.Flags().BoolVarP(&repoRemove, "remove", "r", false, "remove exisitng repo")
	repoSetCmd.Flags().StringVar(&repoRegion, "region", "", "region code")
	repoSetCmd.Flags().BoolVarP(&repoUpdate, "update", "u", false, "run apt update or dnf makecache")
	repoRmCmd.Flags().BoolVarP(&repoUpdate, "update", "u", false, "run apt update or dnf makecache")

	repoCmd.AddCommand(repoAddCmd)
	repoCmd.AddCommand(repoSetCmd)
	repoCmd.AddCommand(repoRmCmd)
	repoCmd.AddCommand(repoListCmd)
	repoCmd.AddCommand(repoUpdateCmd)
	repoCmd.AddCommand(repoStatusCmd)
}
