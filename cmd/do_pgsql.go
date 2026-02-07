package cmd

import (
	"fmt"
	"pig/cli/do"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var doPgsqlAddCmd = &cobra.Command{
	Use:         "pgsql-add",
	Short:       "add instances to cluster",
	Annotations: ancsAnn("pig do pgsql-add", "action", "volatile", "unsafe", false, "medium", "recommended", "root", 300000),
	Aliases:     []string{"pg-add", "pa", "pgsql"},
	Long:        `pig do pgsql-add <selector> [ins...]`,
	Example: `
  pig do pgsql-add pg-meta             # init pgsql cluster
  pig do pg-add 10.10.10.10            # init specific instance
  pig do pa 10.10.10.1[2,3]            # init two instances
  pig do pgsql 10.10.10.12,10.10.10.13 # same as above
  `,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModuleDo, "pig do pgsql-add", args, nil, func() error {
			selector := args[0]
			command := []string{"pgsql.yml", "-l", selector}
			command = append(command, args[1:]...)
			return do.RunPlaybook(inventory, command)
		})
	},
}

// doPgsqlRmCmd - Remove pgsql cluster/instance
var doPgsqlRmCmd = &cobra.Command{
	Use:         "pgsql-rm",
	Short:       "remove instances from cluster",
	Annotations: ancsAnn("pig do pgsql-rm", "action", "volatile", "unsafe", false, "high", "recommended", "root", 300000),
	Aliases:     []string{"pg-rm", "pr"},
	Long:        `pig do pgsql-rm <selector> [ins...]`,
	Example: `
  pig do pgsql-rm pg-meta          # remove pgsql cluster
  pig do pg-rm    10.10.10.10      # remove specific instance
  pig do pr       10.10.10.1[2,3]  # remove two instances
  pig do pgsql-rm 10.10.10.13 full # also uninstall packages`,

	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModuleDo, "pig do pgsql-rm", args, map[string]interface{}{
			"uninstall": doRemoveWithUninstall,
		}, func() error {
			selector := args[0]
			command := []string{"pgsql-rm.yml", "-l", selector}
			if doRemoveWithUninstall {
				command = append(command, "-e", "pg_uninstall=true")
			}
			return do.RunPlaybook(inventory, command)
		})
	},
}

// doPgsqlUserCmd - Create a PostgreSQL user
var doPgsqlUserCmd = &cobra.Command{
	Use:         "pgsql-user",
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

// doPgsqlDbCmd - Create/Update pgsql database
var doPgsqlDbCmd = &cobra.Command{
	Use:         "pgsql-db",
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

// doPgsqlExtCmd - Install pgsql extensions
var doPgsqlExtCmd = &cobra.Command{
	Use:         "pgsql-ext",
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

// doPgsqlHbaCmd - Refresh pgsql hba
var doPgsqlHbaCmd = &cobra.Command{
	Use:         "pgsql-hba",
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

// doPgsqlSvcCmd - Refresh pgsql service
var doPgsqlSvcCmd = &cobra.Command{
	Use:         "pgsql-svc",
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

// doPgmonAddCmd - Add remote pg monitor target
var doPgmonAddCmd = &cobra.Command{
	Use:         "pgmon-add",
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

// doPgmonRmCmd - Remove remote pg monitor target
var doPgmonRmCmd = &cobra.Command{
	Use:         "pgmon-rm",
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
