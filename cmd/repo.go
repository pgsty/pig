package cmd

import (
	"fmt"
	"pig/cli/repo"
	"pig/internal/config"
	"pig/internal/output"
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
)

// repoCmd represents the top-level `repo` command
var repoCmd = &cobra.Command{
	Use:     "repo",
	Short:   "Manage Linux Software Repo (apt/dnf)",
	Aliases: []string{"r"},
	GroupID: "pgext",
	Annotations: map[string]string{
		"name":       "pig repo",
		"type":       "group",
		"volatility": "stable",
		"parallel":   "safe",
		"idempotent": "true",
		"risk":       "safe",
		"confirm":    "none",
		"os_user":    "current",
		"cost":       "100",
	},
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
	Annotations: map[string]string{
		"name":       "pig repo list",
		"type":       "query",
		"volatility": "stable",
		"parallel":   "safe",
		"idempotent": "true",
		"risk":       "safe",
		"confirm":    "none",
		"os_user":    "current",
		"cost":       "100",
	},
	Example: `
  pig repo list                # list available repos on current system
  pig repo list all            # list all unfiltered repo raw data
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		showAll := len(args) > 0 && args[0] == "all"

		// Structured output mode (YAML/JSON)
		format := config.OutputFormat
		if format == config.OUTPUT_YAML || format == config.OUTPUT_JSON || format == config.OUTPUT_JSON_PRETTY {
			result := repo.ListRepos(showAll)
			bytes, err := result.Render(format)
			if err != nil {
				return err
			}
			fmt.Println(string(bytes))
			return nil
		}

		// Text mode: preserve existing behavior
		if len(args) == 0 {
			return repo.List()
		} else if args[0] == "all" {
			return repo.ListAll()
		}
		return nil
	},
}

var repoInfoCmd = &cobra.Command{
	Use:          "info",
	Short:        "get repo detailed information",
	Aliases:      []string{"i"},
	SilenceUsage: true,
	Annotations: map[string]string{
		"name":       "pig repo info",
		"type":       "query",
		"volatility": "stable",
		"parallel":   "safe",
		"idempotent": "true",
		"risk":       "safe",
		"confirm":    "none",
		"os_user":    "current",
		"cost":       "50",
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Structured output mode (YAML/JSON)
		format := config.OutputFormat
		if format == config.OUTPUT_YAML || format == config.OUTPUT_JSON || format == config.OUTPUT_JSON_PRETTY {
			result := repo.GetRepoInfo(args)
			bytes, err := result.Render(format)
			if err != nil {
				return err
			}
			fmt.Println(string(bytes))
			// If operation failed, return error for exit code
			if !result.Success {
				return fmt.Errorf("%s", result.Message)
			}
			return nil
		}

		// Text mode: preserve existing behavior
		if len(args) == 0 {
			logrus.Errorf("repo or module name is required, check available repo list:")
			_ = repo.ListAll()
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
	Annotations: map[string]string{
		"name":       "pig repo add",
		"type":       "action",
		"volatility": "stable",
		"parallel":   "safe",
		"idempotent": "true",
		"risk":       "low",
		"confirm":    "none",
		"os_user":    "root",
		"cost":       "2000",
	},
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
		// Structured output mode (YAML/JSON)
		format := config.OutputFormat
		if format == config.OUTPUT_YAML || format == config.OUTPUT_JSON || format == config.OUTPUT_JSON_PRETTY {
			if config.OSType != config.DistroEL && config.OSType != config.DistroDEB {
				result := output.Fail(output.CodeRepoUnsupportedOS,
					fmt.Sprintf("unsupported platform: %s %s", config.OSVendor, config.OSVersionFull))
				bytes, err := result.Render(format)
				if err != nil {
					return err
				}
				fmt.Println(string(bytes))
				return fmt.Errorf("%s", result.Message)
			}
			if len(args) == 0 {
				args = []string{"all"}
			}
			modules := repo.ExpandModuleArgs(args)
			result := repo.AddRepos(modules, repoRegion, repoRemove, repoUpdate)
			bytes, err := result.Render(format)
			if err != nil {
				return err
			}
			fmt.Println(string(bytes))
			if !result.Success {
				return fmt.Errorf("%s", result.Message)
			}
			return nil
		}

		// Text mode: preserve existing behavior
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

		manager.DetectRegion(repoRegion)
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
	Use:          "set",
	Short:        "wipe, overwrite, and update repository",
	Aliases:      []string{"overwrite"},
	SilenceUsage: true,
	Annotations: map[string]string{
		"name":       "pig repo set",
		"type":       "action",
		"volatility": "stable",
		"parallel":   "restricted",
		"idempotent": "true",
		"risk":       "medium",
		"confirm":    "recommended",
		"os_user":    "root",
		"cost":       "5000",
	},
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
	Annotations: map[string]string{
		"name":       "pig repo rm",
		"type":       "action",
		"volatility": "stable",
		"parallel":   "restricted",
		"idempotent": "true",
		"risk":       "medium",
		"confirm":    "recommended",
		"os_user":    "root",
		"cost":       "2000",
	},
	Example: `
  pig repo rm                      # remove (backup) all existing repo to backup dir
  pig repo rm all --update         # remove module 'all' and update repo cache
  pig repo rm node pigsty -u       # remove module 'node' & 'pigsty' and update repo cache
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		modules := repo.ExpandModuleArgs(args)

		// Structured output mode (YAML/JSON)
		format := config.OutputFormat
		if format == config.OUTPUT_YAML || format == config.OUTPUT_JSON || format == config.OUTPUT_JSON_PRETTY {
			if config.OSType != config.DistroEL && config.OSType != config.DistroDEB {
				result := output.Fail(output.CodeRepoUnsupportedOS,
					fmt.Sprintf("unsupported platform: %s %s", config.OSVendor, config.OSVersionFull))
				bytes, err := result.Render(format)
				if err != nil {
					return err
				}
				fmt.Println(string(bytes))
				return fmt.Errorf("%s", result.Message)
			}
			result := repo.RmRepos(modules, repoUpdate)
			bytes, err := result.Render(format)
			if err != nil {
				return err
			}
			fmt.Println(string(bytes))
			if !result.Success {
				return fmt.Errorf("%s", result.Message)
			}
			return nil
		}

		// Text mode: preserve existing behavior
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
	Annotations: map[string]string{
		"name":       "pig repo update",
		"type":       "action",
		"volatility": "volatile",
		"parallel":   "safe",
		"idempotent": "true",
		"risk":       "safe",
		"confirm":    "none",
		"os_user":    "root",
		"cost":       "10000",
	},
	Example: `
  pig repo update                  # yum makecache or apt update
  `,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Structured output mode (YAML/JSON)
		format := config.OutputFormat
		if format == config.OUTPUT_YAML || format == config.OUTPUT_JSON || format == config.OUTPUT_JSON_PRETTY {
			if config.OSType != config.DistroEL && config.OSType != config.DistroDEB {
				result := output.Fail(output.CodeRepoUnsupportedOS,
					fmt.Sprintf("unsupported platform: %s %s", config.OSVendor, config.OSVersionFull))
				bytes, err := result.Render(format)
				if err != nil {
					return err
				}
				fmt.Println(string(bytes))
				return fmt.Errorf("%s", result.Message)
			}
			result := repo.UpdateCache()
			bytes, err := result.Render(format)
			if err != nil {
				return err
			}
			fmt.Println(string(bytes))
			if !result.Success {
				return fmt.Errorf("%s", result.Message)
			}
			return nil
		}

		// Text mode: preserve existing behavior
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
	Annotations: map[string]string{
		"name":       "pig repo status",
		"type":       "query",
		"volatility": "volatile",
		"parallel":   "safe",
		"idempotent": "true",
		"risk":       "safe",
		"confirm":    "none",
		"os_user":    "current",
		"cost":       "500",
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Structured output mode (YAML/JSON)
		format := config.OutputFormat
		if format == config.OUTPUT_YAML || format == config.OUTPUT_JSON || format == config.OUTPUT_JSON_PRETTY {
			result := repo.GetRepoStatus()
			bytes, err := result.Render(format)
			if err != nil {
				return err
			}
			fmt.Println(string(bytes))
			// If operation failed, return error for exit code
			if !result.Success {
				return fmt.Errorf("%s", result.Message)
			}
			return nil
		}

		// Text mode: preserve existing behavior
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

var repoReloadCmd = &cobra.Command{
	Use:          "reload",
	Short:        "reload repo catalog to the latest version",
	SilenceUsage: true,
	Aliases:      []string{"r", "re"},
	RunE: func(cmd *cobra.Command, args []string) error {
		return repo.ReloadRepoCatalog()
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
