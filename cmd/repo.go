package cmd

import (
	"fmt"
	"os"
	"pig/cli/repo"
	"pig/internal/config"

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
	Short:   "Manage OS Software Repositories",
	Aliases: []string{"r"},
	Example: `
  typical usage: (Beware that manage repo require sudo/root privilege)
  
  sudo pig repo add all             # add all necessary repo
  sudo pig repo update              # update yum/apt repo cache
  sudo pig ext install xxx          # install postgres extensions and ...

  pig repo list                     # list current repos
  pig repo update                   # update repo cache (apt update or dnf makecache, same as '-u|--update' option) 
  pig repo add -u                   # add all necessary repo and update repo cache
  pig repo set -u                   # overwrite repo and update repo cache
  pig repo set all -u               # same as above, but remove(backup) old repos first (same as '-r|--remove' option)
  pig repo add all -u               # same as 'pig repo add', also update repo cache 
  pig repo add pigsty pgdg          # add pigsty extension repo + pgdg offical repo
  pig repo add pgsql node           # add os + postgres repo
  pig repo add infra                # add observability, grafana & prometheus stack, pg bin utils
  pig repo rm                       # remove old repos (move existing repos to ${repodir}/backup)
  pig repo rm pigsty                # remove pigsty repo
  pig repo rm pgsql infra           # remove two repo module: pgsql & infra
	`,
}

var repoAddCmd = &cobra.Command{
	Use:     "add",
	Short:   "add pigsty yum/apt repository",
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
  - pgsql    :  pigsty + pgdg (all available pg extensions)
    - pigsty :  PostgreSQL Extension Repo (default)
    - pgdg   :  PGDG the Official PostgreSQL Repo (official)
  - node     :  operating system official repo (el/debian/ubuntu)
  - infra    :  observability, grafana & prometheus stack, pg bin utils
  - extra    :  extra postgres modules, non-free, citus, timescaledb upstream
  - mssql    :  babelfish, microsoft mssql server compaible postgres
  - ivory    :  ivorysql, the oracle compatible postgres kernel fork (el)
  - kube     :  kubernetes, kubectl, kubeadm, kubelet
  - docker   :  docker-ce, docker-compose
  - ...
`,
	// Long: moduleNotice,

	Run: func(cmd *cobra.Command, args []string) {
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
			os.Exit(1)
		}

		if repoRemove {
			logrus.Infof("move existing repo to backup dir")
			if err := repo.BackupRepo(); err != nil {
				logrus.Error(err)
				os.Exit(1)
			}
		}

		if err := repo.AddRepo(repoRegion, args...); err != nil {
			logrus.Error(err)
			os.Exit(1)
		}

		fmt.Printf("======== ls %s\n", repoDir)
		repo.RunCmd([]string{"ls", repoDir})

		if repoUpdate {
			if err := repo.RunCmd(updateCmd); err != nil {
				logrus.Error(err)
				os.Exit(1)
			}
		} else {
			logrus.Infof("repo added, consider run: sudo %s", updateCmd)
		}
	},
}

var repoSetCmd = &cobra.Command{
	Use:     "set",
	Short:   "set pigsty software repository",
	Aliases: []string{"s", "overwrite"},
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
	Short:   "remove pigsty yum/apt repository",
	Aliases: []string{"r", "remove"},
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			err := repo.BackupRepo()
			if err != nil {
				logrus.Error(err)
				os.Exit(1)
			}
			return
		}
		err := repo.RemoveRepo(args...)
		if err != nil {
			logrus.Error(err)
			os.Exit(1)
		}

		if repoUpdate {
			var updateCmd []string
			if config.OSType == config.DistroEL {
				updateCmd = []string{"yum", "makecache"}
			} else if config.OSType == config.DistroDEB {
				updateCmd = []string{"apt-get", "update"}
			} else {
				logrus.Errorf("unsupported OS type: %s", config.OSType)
				os.Exit(1)
			}

			err = repo.RunCmd(updateCmd)
			if err != nil {
				logrus.Error(err)
				os.Exit(1)
			}
		}
	},
}

var repoListCmd = &cobra.Command{
	Use:     "list",
	Short:   "list pigsty yum/apt repository",
	Aliases: []string{"l", "ls"},
	Run: func(cmd *cobra.Command, args []string) {
		if config.OSType == config.DistroEL {
			if err := repo.ListPigstyRpmRepo(); err != nil {
				logrus.Error(err)
				os.Exit(1)
			}
		} else if config.OSType == config.DistroDEB {
			if err := repo.ListPigstyDebRepo(); err != nil {
				logrus.Error(err)
				os.Exit(1)
			}
		} else {
			logrus.Errorf("unsupported OS type: %s", config.OSType)
			os.Exit(1)
		}
	},
}

var repoCacheCmd = &cobra.Command{
	Use:     "update",
	Short:   "update pigsty yum/apt repo cache",
	Aliases: []string{"u", "cache"},
	Run: func(cmd *cobra.Command, args []string) {
		if config.OSType == config.DistroEL {
			err := repo.RunCmd([]string{"yum", "makecache"})
			if err != nil {
				logrus.Error(err)
				os.Exit(1)
			}
		} else if config.OSType == config.DistroDEB {
			err := repo.RunCmd([]string{"apt", "update"})
			if err != nil {
				logrus.Error(err)
				os.Exit(1)
			}
		} else {
			logrus.Errorf("unsupported OS type: %s", config.OSType)
			os.Exit(1)
		}
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
	repoCmd.AddCommand(repoCacheCmd)
	rootCmd.AddCommand(repoCmd)
}
