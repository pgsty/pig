package cmd

import (
	"fmt"
	"pig/cli/do"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	doRemoveWithUninstall bool
)

// doCmd represents the pig do management command
var doCmd = &cobra.Command{
	Use:         "do",
	Short:       "run admin tasks",
	Annotations: ancsAnn("pig do", "query", "stable", "safe", true, "safe", "none", "current", 100),
	Aliases:     []string{"d"},
	GroupID:     "pigsty",
	Long:        `pig do - perform admin tasks with ansible playbook`,
	Example: `
	  pig do pgsql-add  <cls> [ip...]       # add cluster or replicas
	  pig do pgsql-rm   <cls> [ip...]       # remove cluster or replicas
  pig do pgsql-db   <cls> <dbname>      # create/update pgsql database
  pig do pgsql-user <cls> <username>    # create/udpate pgsql user
  pig do pgsql-ext  <cls> [ext...]      # install pgsql extensions
  pig do pgsql-svc  <sel>               # reload pgsql service
  pig do pgsql-hba  <sel>               # refresh pgsql hba
  pig do pgmon-add  <cls>               # add remote monitor target
  pig do pgmon-rm   <cls>               # remove remote monitor target

  pig do node-add   <sel>               # add node to pigsty
  pig do node-rm    <sel>               # remove node from pigsty
  pig do node-repo  <sel>               # refresh node repo
  pig do node-pkg   <sel> [pkg...]      # install node package

  pig do redis-add  <sel> [port...]     # add redis cluster/node/instance
  pig do redis-rm   <sel> [port...]     # remove redis cluster/node/instance
	`,
}

var doPgsqlAddCmd = &cobra.Command{
	Use:         "pgsql-add <cluster> [ip...]",
	Short:       "add instances to cluster",
	Annotations: ancsAnn("pig do pgsql-add", "action", "volatile", "unsafe", false, "medium", "recommended", "root", 300000),
	Aliases:     []string{"pg-add", "pa", "pgsql"},
	Long:        `pig do pgsql-add <cluster> [ip...]`,
	Example: `
	  pig do pgsql-add pg-meta                  # init pgsql cluster
	  pig do pg-add    pg-test 10.10.10.12      # add one replica
	  pig do pa        pg-test 10.10.10.12 10.10.10.13
	  `,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModuleDo, "pig do pgsql-add", args, nil, func() error {
			commands, err := do.BuildPgsqlAddCommands(args[0], args[1:])
			if err != nil {
				return err
			}
			return do.RunPlaybooks(inventory, commands)
		})
	},
}

var doPgsqlRmCmd = &cobra.Command{
	Use:         "pgsql-rm <cluster> [ip...]",
	Short:       "remove instances from cluster",
	Annotations: ancsAnn("pig do pgsql-rm", "action", "volatile", "unsafe", false, "high", "recommended", "root", 300000),
	Aliases:     []string{"pg-rm", "pr"},
	Long:        `pig do pgsql-rm <cluster> [ip...]`,
	Example: `
	  pig do pgsql-rm pg-meta                  # remove pgsql cluster
	  pig do pg-rm    pg-test 10.10.10.13      # remove one replica
	  pig do pr       pg-test 10.10.10.12 10.10.10.13
	  pig do pgsql-rm pg-test --uninstall      # also uninstall packages`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModuleDo, "pig do pgsql-rm", args, map[string]interface{}{
			"uninstall": doRemoveWithUninstall,
		}, func() error {
			commands, err := do.BuildPgsqlRmCommands(args[0], args[1:], doRemoveWithUninstall)
			if err != nil {
				return err
			}
			return do.RunPlaybooks(inventory, commands)
		})
	},
}

var doPgsqlUserCmd = &cobra.Command{
	Use:         "pgsql-user <cls> <username>",
	Short:       "create/update pgsql user",
	Annotations: ancsAnn("pig do pgsql-user", "action", "volatile", "unsafe", false, "low", "recommended", "root", 60000),
	Aliases:     []string{"pg-user", "pu"},
	Long:        `pig do pgsql-user <cls> <username>`,
	Example: `
  pig do pgsql-user pg-meta dbuser_meta
  pig do pg-user    pg-meta dbuser_view
  pig do pu         pg-test test`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModuleDo, "pig do pgsql-user", args, nil, func() error {
			cls := args[0]
			username := args[1]
			command := []string{"pgsql-user.yml", "-l", cls, "-e", fmt.Sprintf("username=%s", username)}
			command = append(command, args[2:]...)
			return do.RunPlaybook(inventory, command)
		})
	},
}

var doPgsqlDbCmd = &cobra.Command{
	Use:         "pgsql-db <cls> <dbname>",
	Short:       "create/update pgsql database",
	Annotations: ancsAnn("pig do pgsql-db", "action", "volatile", "unsafe", false, "low", "recommended", "root", 60000),
	Aliases:     []string{"pg-db", "pd"},
	Long:        `pig do pgsql-db <cls> <dbname>`,
	Example: `
  pig do pgsql-db pg-meta meta
  pig do pg-db    pg-test test`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModuleDo, "pig do pgsql-db", args, nil, func() error {
			cls := args[0]
			dbname := args[1]
			command := []string{"pgsql-db.yml", "-l", cls, "-e", fmt.Sprintf("dbname=%s", dbname)}
			command = append(command, args[2:]...)
			return do.RunPlaybook(inventory, command)
		})
	},
}

var doPgsqlExtCmd = &cobra.Command{
	Use:         "pgsql-ext <cls> [ext...]",
	Short:       "install pgsql extensions",
	Annotations: ancsAnn("pig do pgsql-ext", "action", "volatile", "unsafe", false, "low", "recommended", "root", 60000),
	Aliases:     []string{"pg-ext", "pe"},
	Long:        `pig do pgsql-ext <cls>`,
	Example: `
  pig do pgsql-ext pg-meta postgis
  pig do pg-ext    pg-test timescaledb
  pig do pe        pg-meta citus pgvector
  `,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModuleDo, "pig do pgsql-ext", args, nil, func() error {
			selector := args[0]
			command := []string{"pgsql.yml", "-l", selector, "-t", "pg_extension"}
			if len(args) > 1 {
				packages := strings.Join(args[1:], ",")
				packages = fmt.Sprintf(`{"pg_extensions":["%s"]}`, packages)
				command = append(command, "-e", packages)
			}
			return do.RunPlaybook(inventory, command)
		})
	},
}

var doPgsqlHbaCmd = &cobra.Command{
	Use:         "pgsql-hba <cls>",
	Short:       "refresh pgsql hba",
	Annotations: ancsAnn("pig do pgsql-hba", "action", "volatile", "unsafe", false, "medium", "recommended", "root", 60000),
	Aliases:     []string{"pg-hba", "ph"},
	Long:        `pig do pgsql-hba <cls>`,
	Example: `
  pig do pgsql-hba pg-meta
  pig do pg-hba    pg-test
  pig do ph        pg-meta
  `,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModuleDo, "pig do pgsql-hba", args, nil, func() error {
			cls := args[0]
			command := []string{"pgsql.yml", "-l", cls, "-t", "pg_hba,pg_reload,pgbouncer_hba,pgbouncer_reload"}
			return do.RunPlaybook(inventory, command)
		})
	},
}

var doPgsqlSvcCmd = &cobra.Command{
	Use:         "pgsql-svc <cls>",
	Short:       "refresh pgsql service",
	Annotations: ancsAnn("pig do pgsql-svc", "action", "volatile", "unsafe", false, "medium", "recommended", "root", 60000),
	Aliases:     []string{"pg-svc", "ps"},
	Long:        `pig do pgsql-svc <cls>`,
	Example: `
  pig do pgsql-svc pg-meta
  pig do pg-svc    pg-test
  pig do ps        pg-meta
  `,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModuleDo, "pig do pgsql-svc", args, nil, func() error {
			selector := args[0]
			command := []string{"pgsql.yml", "-l", selector, "-t", "pg_service"}
			return do.RunPlaybook(inventory, command)
		})
	},
}

var doPgmonAddCmd = &cobra.Command{
	Use:         "pgmon-add <cls>",
	Short:       "add remote pg monitor target",
	Annotations: ancsAnn("pig do pgmon-add", "action", "volatile", "unsafe", false, "low", "recommended", "root", 60000),
	Aliases:     []string{"mon-add", "ma"},
	Long:        `pig do pgmon-add <cls>`,
	Example: `
  pig do pgmon-add pg-foo
  pig do mon-add   pg-bar
  pig do ma        pg-foobar
  `,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModuleDo, "pig do pgmon-add", args, nil, func() error {
			cls := args[0]
			command := []string{"pgsql-monitor.yml", "-e", fmt.Sprintf("clsname=%s", cls)}
			return do.RunPlaybook(inventory, command)
		})
	},
}

var doPgmonRmCmd = &cobra.Command{
	Use:         "pgmon-rm <cls>",
	Short:       "remove remote pg monitor target",
	Annotations: ancsAnn("pig do pgmon-rm", "action", "volatile", "unsafe", false, "medium", "recommended", "root", 60000),
	Aliases:     []string{"mon-rm", "mr"},
	Long:        `pig do pgmon-rm <cls>`,
	Example: `
  pig do pgmon-rm pg-foo
  pig do mon-rm   pg-bar
  pig do mr       pg-foobar
  `,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModuleDo, "pig do pgmon-rm", args, nil, func() error {
			cls := args[0]
			target := fmt.Sprintf(`/etc/prometheus/targets/pgrds/%s.yml`, cls)
			logrus.Infof("removing pgsql monitor target %s", cls)
			return do.RunAnsible(inventory, []string{"infra", "-m", "file", "-b", "-a", fmt.Sprintf(`path=%s state=absent`, target)})
		})
	},
}

var doNodeAddCmd = &cobra.Command{
	Use:         "node-add <sel> [sel...]",
	Short:       "add node to pigsty",
	Annotations: ancsAnn("pig do node-add", "action", "volatile", "unsafe", false, "medium", "recommended", "root", 300000),
	Aliases:     []string{"node", "node-a", "nadd", "na"},
	Long:        `pig do node-add <sel> [sel...]`,
	Example: `
  pig do node-add pg-test                 # add node by cluster name
  pig do nadd     10.10.10.10             # add node by ip address
  pig do na       10.10.10.10,10.10.10.11 # add multiple nodes
  `,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModuleDo, "pig do node-add", args, nil, func() error {
			command, err := do.BuildNodeAddCommand(args)
			if err != nil {
				return err
			}
			return do.RunPlaybook(inventory, command)
		})
	},
}

var doNodeRmCmd = &cobra.Command{
	Use:         "node-rm <sel> [sel...]",
	Short:       "remove node from pigsty",
	Annotations: ancsAnn("pig do node-rm", "action", "volatile", "unsafe", false, "high", "recommended", "root", 300000),
	Aliases:     []string{"node-r", "nrm"},
	Long:        `pig do node-rm <sel> [sel...]`,
	Example: `
  pig do node-rm pg-test                 # remove node by cluster name
  pig do node-r  10.10.10.10             # remove node by ip address
  pig do nrm     10.10.10.10,10.10.10.11  # remove multiple nodes
  `,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModuleDo, "pig do node-rm", args, nil, func() error {
			command, err := do.BuildNodeRmCommand(args)
			if err != nil {
				return err
			}
			return do.RunPlaybook(inventory, command)
		})
	},
}

var doNodeRepoCmd = &cobra.Command{
	Use:         "node-repo [sel] [module]",
	Short:       "update node repo",
	Annotations: ancsAnn("pig do node-repo", "action", "volatile", "unsafe", false, "low", "recommended", "root", 60000),
	Aliases:     []string{"node-rp", "nrp"},
	Long:        `pig do node-repo [sel] [module]`,
	Example: `
  pig do node-repo pg-meta               # add default local repo to pg-meta
  pig do node-rp   pg-test node          # add node repo to pg-test
  pig do nrp       pg-meta node,infra    # add default local repo to all nodes

	  modules: local,infra,pgsql,node,extra,mysql,mongo,redis,haproxy,grafana,kube,...
	  `,
	Args: cobra.MaximumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModuleDo, "pig do node-repo", args, nil, func() error {
			command, err := do.BuildNodeRepoCommand(args)
			if err != nil {
				return err
			}
			return do.RunPlaybook(inventory, command)
		})
	},
}

var doNodePkgCmd = &cobra.Command{
	Use:         "node-pkg <sel> [pkg...]",
	Short:       "update node package",
	Annotations: ancsAnn("pig do node-pkg", "action", "volatile", "unsafe", false, "low", "recommended", "root", 60000),
	Aliases:     []string{"node-p", "np"},
	Long:        `pig do node-pkg <sel> [module]`,
	Example: `
  pig do node-pkg pg-meta openssh       # upgrade openssh on pg-meta
  pig do node-p   pg-test juicefs       # install juicefs on pg-test
  pig do np       all duckdb restic     # install 2 packages on all nodes

  PS: make sure required repo is available on target nodes
  `,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModuleDo, "pig do node-pkg", args, nil, func() error {
			selector := args[0]
			command := []string{"node.yml", "-l", selector, "-t", "node_pkg_extra"}
			if len(args) > 1 {
				packages := strings.Join(args[1:], ",")
				packages = fmt.Sprintf(`{"node_packages":["%s"]}`, packages)
				command = append(command, "-e", packages)
			}
			return do.RunPlaybook(inventory, command)
		})
	},
}

var doRepoBuildCmd = &cobra.Command{
	Use:         "repo-build",
	Short:       "rebuild infra repo",
	Annotations: ancsAnn("pig do repo-build", "action", "volatile", "unsafe", false, "low", "recommended", "root", 60000),
	Aliases:     []string{"repo-b", "rb"},
	Long:        `pig do repo-build`,
	Example: `
  pig do repo-build   # rebuild infra repo
  `,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModuleDo, "pig do repo-build", args, nil, func() error {
			command := []string{"infra.yml", "-l", "infra", "-t", "repo_build"}
			return do.RunPlaybook(inventory, command)
		})
	},
}

var doRedisAddCmd = &cobra.Command{
	Use:         "redis-add <sel> [port...]",
	Short:       "add redis to pigsty",
	Annotations: ancsAnn("pig do redis-add", "action", "volatile", "unsafe", false, "medium", "recommended", "root", 300000),
	Aliases:     []string{"redis", "re-add", "ra"},
	Long:        `pig do redis-add <sel> [port...]`,
	Example: `
  pig do redis-add redis-meta                 # init redis cluster
  pig do re-add    redis-test                 # init redis cluster redis-test
  pig do ra        10.10.10.10                # init redis on given node
  pig do ra        10.10.10.11 6379 6380      # init specific redis instances
  `,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModuleDo, "pig do redis-add", args, nil, func() error {
			commands, err := do.BuildRedisAddCommands(args[0], args[1:])
			if err != nil {
				return err
			}
			return do.RunPlaybooks(inventory, commands)
		})
	},
}

var doRedisRmCmd = &cobra.Command{
	Use:         "redis-rm <sel> [port...]",
	Short:       "remove redis from pigsty",
	Annotations: ancsAnn("pig do redis-rm", "action", "volatile", "unsafe", false, "high", "recommended", "root", 300000),
	Aliases:     []string{"re-rm", "rr"},
	Long:        `pig do redis-rm <sel> [port...]`,
	Example: `
  pig do redis-rm redis-meta                 # remove redis cluster
  pig do re-rm    redis-test                 # remove redis cluster redis-test
  pig do rr       10.10.10.10                # remove redis on given node
  pig do rr       10.10.10.11 6379           # remove one specific redis instance
  pig do rr       10.10.10.11 6379 6380      # remove two specific redis instances
  `,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModuleDo, "pig do redis-rm", args, map[string]interface{}{
			"uninstall": doRemoveWithUninstall,
		}, func() error {
			commands, err := do.BuildRedisRmCommands(args[0], args[1:], doRemoveWithUninstall)
			if err != nil {
				return err
			}
			return do.RunPlaybooks(inventory, commands)
		})
	},
}

func init() {
	doPgsqlRmCmd.Flags().BoolVarP(&doRemoveWithUninstall, "uninstall", "u", false, "uninstall packages during removal")
	doRedisRmCmd.Flags().BoolVarP(&doRemoveWithUninstall, "uninstall", "u", false, "uninstall packages during node or cluster removal")

	doCmd.AddCommand(
		doPgsqlAddCmd,
		doPgsqlRmCmd,
		doPgsqlUserCmd,
		doPgsqlDbCmd,
		doPgsqlExtCmd,
		doPgsqlHbaCmd,
		doPgsqlSvcCmd,
		doPgmonAddCmd,
		doPgmonRmCmd,
		doNodeAddCmd,
		doNodeRmCmd,
		doNodeRepoCmd,
		doNodePkgCmd,
		doRepoBuildCmd,
		doRedisAddCmd,
		doRedisRmCmd,
	)
}
