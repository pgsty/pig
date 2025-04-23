package cmd

import (
	"fmt"
	"pig/cli/do"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// doCmd represents the pig do management command
var doCmd = &cobra.Command{
	Use:     "do",
	Short:   "Run Pigsty Playbook Tasks",
	Aliases: []string{"ru"},
	GroupID: "pigsty",
	Long:    `pig do - ansible playbook to perform admin tasks`,
	Example: `
  pig do pgsql-add  <sel> [ins...]      # add instances to cluster
  pig do pgsql-rm   <sel> [ins...]      # remove instances from cluster
  pig do pgsql-db   <cls> <dbname>      # create/update pgsql database
  pig do pgsql-user <cls> <username>    # create/udpate pgsql user
  pig do pgsql-ext  <sel> [ext...]      # install pgsql extensions
  pig do pgsql-svc  <sel>               # reload pgsql service
  pig do pgsql-hba  <sel>               # refresh pgsql hba
  pig do pgmon-add  <cls>               # add remote monitor target
  pig do pgmon-rm   <cls>               # remove remote monitor target
  
  pig do node-add   <sel>               # add node to pigsty
  pig do node-rm    <sel>               # remove node from pigsty
  pig do node-repo  <sel>               # refresh node repo
  pig do node-pkg   <sel> [pkg...]      # install node package
 
  pig do redis-add  <sel> [port...]     # TBD
  pig do redis-rm   <sel> [port...]     # TBD
  pig do redis-svc  <sel> [port...]     # TBD
  `,
}

// doPgsqlAddCmd - create pgsql cluster/instance
var doPgsqlAddCmd = &cobra.Command{
	Use:     "pgsql-add",
	Short:   "add instances to cluster",
	Aliases: []string{"pg-add", "pa", "pgsql"},
	Long:    `pig do pgsql-add <selector> [ins...]`,
	Example: `
  pig do pgsql-add pg-meta             # init pgsql cluster
  pig do pg-add 10.10.10.10            # init specific instance
  pig do pa 10.10.10.1[2,3]            # init two instances
  pig do pgsql 10.10.10.12,10.10.10.13 # same as above
  `,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cls := args[0]
		command := []string{"pgsql.yml", "-l", cls}
		command = append(command, args[1:]...)
		return do.RunPlaybook(inventory, command)
	},
}

// doPgsqlRmCmd - Remove pgsql cluster/instance
var doPgsqlRmCmd = &cobra.Command{
	Use:     "pgsql-rm",
	Short:   "remove instances from cluster",
	Aliases: []string{"pg-rm", "pr"},
	Long:    `pig do pgsql-rm <selector> [ins...]`,
	Example: `
  pig do pgsql-rm pg-meta          # remove pgsql cluster
  pig do pg-rm    10.10.10.10      # remove specific instance
  pig do pr       10.10.10.1[2,3]  # remove two instances
  pig do pgsql-rm 10.10.10.13 full # also uninstall packages`,

	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cls := args[0]
		command := []string{"pgsql-rm.yml", "-l", cls}
		if len(args) > 1 && args[1] == "full" {
			//
			command = append(command, "-e", "pg_uninstall=true")
		}
		return do.RunPlaybook(inventory, command)
	},
}

// doPgsqlUserCmd - Create a PostgreSQL user
var doPgsqlUserCmd = &cobra.Command{
	Use:     "pgsql-user",
	Short:   "create/update pgsql user",
	Aliases: []string{"pg-user", "pu"},
	Long:    `pig do pgsql-user <cls> <username>`,
	Example: `
  pig do pgsql-user pg-meta dbuser_meta
  pig do pg-user    pg-meta dbuser_view
  pig do pu         pg-test test`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		cls := args[0]
		username := args[1]
		command := []string{"pgsql-user.yml", "-l", cls, "-e", fmt.Sprintf("username=%s", username)}
		command = append(command, args[2:]...)
		return do.RunPlaybook(inventory, command)
	},
}

// doPgsqlDbCmd - Create/Update pgsql database
var doPgsqlDbCmd = &cobra.Command{
	Use:     "pgsql-db",
	Short:   "create/update pgsql database",
	Aliases: []string{"pg-db", "pd"},
	Long:    `pig do pgsql-db <cls> <dbname>`,
	Example: `
  pig do pgsql-db pg-meta meta
  pig do pg-db    pg-test test`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		cls := args[0]
		dbname := args[1]
		command := []string{"pgsql-db.yml", "-l", cls, "-e", fmt.Sprintf("dbname=%s", dbname)}
		command = append(command, args[2:]...)
		return do.RunPlaybook(inventory, command)
	},
}

// doPgsqlExtCmd - Install pgsql extensions
var doPgsqlExtCmd = &cobra.Command{
	Use:     "pgsql-ext",
	Short:   "install pgsql extensions",
	Aliases: []string{"pg-ext", "pe"},
	Long:    `pig do pgsql-ext <cls>`,
	Example: `
  pig do pgsql-ext pg-meta postgis
  pig do pg-ext    pg-test timescaledb
  pig do pe        pg-meta citus pgvector
  `,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		selector := args[0]
		command := []string{"pgsql.yml", "-l", selector, "-t", "pg_extension"}
		if len(args) > 1 {
			packages := strings.Join(args[1:], ",")
			packages = fmt.Sprintf(`{"pg_extensions":["%s"]}`, packages)
			command = append(command, "-e", packages)
		}
		return do.RunPlaybook(inventory, command)
	},
}

// doPgsqlHbaCmd - Refresh pgsql hba
var doPgsqlHbaCmd = &cobra.Command{
	Use:     "pgsql-hba",
	Short:   "refresh pgsql hba",
	Aliases: []string{"pg-hba", "ph"},
	Long:    `pig do pgsql-hba <cls>`,
	Example: `
  pig do pgsql-hba pg-meta
  pig do pg-hba    pg-test
  pig do ph        pg-meta
  `,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cls := args[0]
		command := []string{"pgsql.yml", "-l", cls, "-t", "pg_hba,pg_reload,pgbouncer_hba,pgbouncer_reload"}
		return do.RunPlaybook(inventory, command)
	},
}

// doPgsqlSvcCmd - Refresh pgsql service
var doPgsqlSvcCmd = &cobra.Command{
	Use:     "pgsql-svc",
	Short:   "refresh pgsql service",
	Aliases: []string{"pg-svc", "ps"},
	Long:    `pig do pgsql-svc <cls>`,
	Example: `
  pig do pgsql-svc pg-meta
  pig do pg-svc    pg-test
  pig do ps        pg-meta
  `,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cls := args[0]
		command := []string{"pgsql.yml", "-l", cls, "-t", "pg_service"}
		return do.RunPlaybook(inventory, command)
	},
}

// doPgmonAddCmd - Add remote pg monitor target
var doPgmonAddCmd = &cobra.Command{
	Use:     "pgmon-add",
	Short:   "add remote pg monitor target",
	Aliases: []string{"mon-add", "ma"},
	Long:    `pig do pgmon-add <cls>`,
	Example: `
  pig do pgmon-add pg-foo
  pig do mon-add   pg-bar
  pig do ma        pg-foobar
  `,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cls := args[0]
		command := []string{"pgsql-monitor.yml", "-e", fmt.Sprintf("clsname=%s", cls)}
		return do.RunPlaybook(inventory, command)
	},
}

// doPgmonRmCmd - Remove remote pg monitor target
var doPgmonRmCmd = &cobra.Command{
	Use:     "pgmon-rm",
	Short:   "remove remote pg monitor target",
	Aliases: []string{"mon-rm", "mr"},
	Long:    `pig do pgmon-rm <cls>`,
	Example: `
  pig do pgmon-rm pg-foo
  pig do mon-rm   pg-bar
  pig do mr       pg-foobar
  `,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cls := args[0]
		target := fmt.Sprintf(`/etc/prometheus/targets/pgrds/%s.yml`, cls)
		logrus.Infof("removing pgsql monitor target %s", cls)
		return do.RunAnsible(inventory, []string{"infra", "-m", "file", "-b", "-a", fmt.Sprintf(`path=%s state=absent`, target)})
	},
}

// doNodeAddCmd - Add node to pigsty
var doNodeAddCmd = &cobra.Command{
	Use:     "node-add",
	Short:   "add node to pigsty",
	Aliases: []string{"node", "node-a", "nadd", "na"},
	Long:    `pig do node-add <sel>`,
	Example: `
  pig do node-add pg-test                 # add node by cluster name
  pig do nadd     10.10.10.10             # add node by ip address
  pig do na       10.10.10.10,10.10.10.11 # add multiple nodes
  `,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cls := args[0]
		command := []string{"node.yml", "-l", cls}
		return do.RunPlaybook(inventory, command)
	},
}

// doNodeRmCmd - Remove node from pigsty
var doNodeRmCmd = &cobra.Command{
	Use:     "node-rm",
	Short:   "remove node from pigsty",
	Aliases: []string{"node-r", "nrm"},
	Long:    `pig do node-rm <sel>`,
	Example: `
  pig do node-rm pg-test                 # remove node by cluster name
  pig do node-r  10.10.10.10             # remove node by ip address
  pig do nrm     10.10.10.10,10.10.10.11  # remove multiple nodes
  `,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cls := args[0]
		command := []string{"node-rm.yml", "-l", cls}
		return do.RunPlaybook(inventory, command)
	},
}

// doNodeRepoCmd - Remove node from pigsty
var doNodeRepoCmd = &cobra.Command{
	Use:     "node-repo",
	Short:   "update node repo",
	Aliases: []string{"node-rp", "nrp"},
	Long:    `pig do node-repo <sel> [module]`,
	Example: `
  pig do node-repo pg-meta               # add default local repo to pg-meta
  pig do node-rp   pg-test node          # add node repo to pg-test
  pig do nrp       pg-meta node,infra    # add default local repo to all nodes

  modules: local,infra,pgsql,node,extra,mysql,mongo,redis,haproxy,grafana,kube,...
  `,
	RunE: func(cmd *cobra.Command, args []string) error {
		var selector, module string
		if len(args) >= 1 {
			selector = args[0]
		}
		if len(args) >= 2 {
			module = args[1]
		}
		if len(args) >= 3 {
			cmd.Help()
		}
		command := []string{"node.yml", "-t", "node_repo"}
		if selector != "" {
			command = append(command, "-l", selector)
		}
		if module != "" {
			command = append(command, "-e", fmt.Sprintf("node_repo_modules=%s", module))
		}
		return do.RunPlaybook(inventory, command)
	},
}

// doNodePkgCmd - Remove node from pigsty
var doNodePkgCmd = &cobra.Command{
	Use:     "node-pkg",
	Short:   "update node package",
	Aliases: []string{"node-p", "np"},
	Long:    `pig do node-pkg <sel> [module]`,
	Example: `
  pig do node-pkg pg-meta openssh       # upgrade openssh on pg-meta
  pig do node-p   pg-test juicefs       # install juicefs on pg-test
  pig do np       all duckdb restic     # install 2 packages on all nodes

  PS: make sure required repo is available on target nodes
  `,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		selector := args[0]
		command := []string{"node.yml", "-l", selector, "-t", "node_pkg_extra"}
		if len(args) > 1 {
			packages := strings.Join(args[1:], ",")
			packages = fmt.Sprintf(`{"node_packages":["%s"]}`, packages)
			command = append(command, "-e", packages)
		}
		return do.RunPlaybook(inventory, command)
	},
}

// doNodeRepoCmd - Remove node from pigsty
var doRepoBuildCmd = &cobra.Command{
	Use:     "repo-build",
	Short:   "rebuild infra repo",
	Aliases: []string{"repo-b", "rb"},
	Long:    `pig do repo-build`,
	Example: `
  pig do repo-build   # rebuild infra repo
  `,
	RunE: func(cmd *cobra.Command, args []string) error {
		command := []string{"infra.yml", "-l", "infra", "-t", "repo_build"}
		return do.RunPlaybook(inventory, command)
	},
}

// doRedisAddCmd - Add redis to pigsty
var doRedisAddCmd = &cobra.Command{
	Use:     "redis-add",
	Short:   "add redis to pigsty",
	Aliases: []string{"redis", "re-add", "ra"},
	Long:    `pig do redis-add <sel> [port...]`,
	Example: `
  pig do redis-add redis-meta                 # init redis cluster
  pig do re-add    redis-test                 # init redis cluster redis-test
  pig do ra        10.10.10.10                # init redis on given node
  pig do ra        10.10.10.11 6379 6380      # init specific redis instances
  `,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		selector := args[0]
		if len(args) == 1 {
			command := []string{"redis.yml", "-l", selector}
			return do.RunPlaybook(inventory, command)
		} else {
			ports := args[1:]
			for _, port := range ports {
				command := []string{"redis.yml", "-l", selector, "-e", fmt.Sprintf("redis_port=%s", port)}
				return do.RunPlaybook(inventory, command)
			}
		}
		return nil
	},
}

// doRedisRmCmd - Remove redis from pigsty
var doRedisRmCmd = &cobra.Command{
	Use:     "redis-rm",
	Short:   "remove redis from pigsty",
	Aliases: []string{"re-rm", "rr"},
	Long:    `pig do redis-rm <sel> [port...]`,
	Example: `
  pig do redis-rm redis-meta                 # remove redis cluster
  pig do re-rm    redis-test                 # remove redis cluster redis-test
  pig do rr       10.10.10.10                # remove redis on given node
  pig do rr       10.10.10.11 6379           # remove one specific redis instance
  pig do rr       10.10.10.11 6379 6380      # remove two specific redis instances
  `,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		selector := args[0]
		if len(args) == 1 {
			command := []string{"redis-rm.yml", "-l", selector}
			return do.RunPlaybook(inventory, command)
		} else {
			ports := args[1:]
			for _, port := range ports {
				command := []string{"redis-rm.yml", "-l", selector, "-e", fmt.Sprintf("redis_port=%s", port)}
				return do.RunPlaybook(inventory, command)
			}
		}
		return nil
	},
}

func init() {
	doCmd.AddCommand(doPgsqlAddCmd)
	doCmd.AddCommand(doPgsqlRmCmd)
	doCmd.AddCommand(doPgsqlUserCmd)
	doCmd.AddCommand(doPgsqlDbCmd)
	doCmd.AddCommand(doPgsqlExtCmd)
	doCmd.AddCommand(doPgsqlHbaCmd)
	doCmd.AddCommand(doPgsqlSvcCmd)
	doCmd.AddCommand(doPgmonAddCmd)
	doCmd.AddCommand(doPgmonRmCmd)

	doCmd.AddCommand(doNodeAddCmd)
	doCmd.AddCommand(doNodeRmCmd)
	doCmd.AddCommand(doNodeRepoCmd)
	doCmd.AddCommand(doNodePkgCmd)
	doCmd.AddCommand(doRepoBuildCmd)

	doCmd.AddCommand(doRedisAddCmd)
	doCmd.AddCommand(doRedisRmCmd)

}
