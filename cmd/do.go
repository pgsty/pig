package cmd

import (
	"pig/internal/output"

	"github.com/spf13/cobra"
)

var (
	doRemoveWithUninstall bool
)

func runDoLegacy(command string, args []string, params map[string]interface{}, fn func() error) error {
	return runLegacyStructured(output.MODULE_DO, command, args, params, fn)
}

// doCmd represents the pig do management command
var doCmd = &cobra.Command{
	Use:   "do",
	Short: "run admin tasks",
	Annotations: map[string]string{
		"name":       "pig do",
		"type":       "query",
		"volatility": "stable",
		"parallel":   "safe",
		"idempotent": "true",
		"risk":       "safe",
		"confirm":    "none",
		"os_user":    "current",
		"cost":       "100",
	},
	Aliases: []string{"d"},
	GroupID: "pigsty",
	Long:    `pig do - perform admin tasks with ansible playbook`,
	Example: `
  pig do pgsql-add  <sel> [ins...]      # add instances to cluster
  pig do pgsql-rm   <sel> [ins...]      # remove instances from cluster
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

func init() {
	doPgsqlRmCmd.Flags().BoolVarP(&doRemoveWithUninstall, "uninstall", "u", false, "uninstall packages during removal")
	doRedisRmCmd.Flags().BoolVarP(&doRemoveWithUninstall, "uninstall", "u", false, "uninstall packages during removal")

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
