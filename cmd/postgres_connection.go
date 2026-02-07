package cmd

import (
	"pig/cli/postgres"
	"pig/internal/config"
	"pig/internal/output"

	"github.com/spf13/cobra"
)

// ============================================================================
// Connection Commands
// ============================================================================

var pgPsqlCmd = &cobra.Command{
	Use:     "psql [dbname]",
	Short:   "Connect to PostgreSQL database via psql",
	Aliases: []string{"sql", "connect"},
	Annotations: map[string]string{
		"name":       "pig postgres psql",
		"type":       "action",
		"volatility": "volatile",
		"parallel":   "safe",
		"idempotent": "false",
		"risk":       "medium",
		"confirm":    "none",
		"os_user":    "dbsu",
		"cost":       "0",
	},
	Example: `  pig pg psql                    # connect to postgres database
  pig pg psql mydb               # connect to specific database
  pig pg psql mydb -c "SELECT 1" # run single command
  pig pg psql -f script.sql      # run SQL script file`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dbname := ""
		if len(args) > 0 {
			dbname = args[0]
		}
		opts := &postgres.PsqlOptions{
			Command: pgPsqlCommand,
			File:    pgPsqlFile,
		}
		if config.IsStructuredOutput() && pgPsqlCommand == "" && pgPsqlFile == "" {
			return structuredParamError(
				output.MODULE_PG,
				"pig postgres psql",
				"interactive psql session is not supported in structured output",
				"use -c/--command or -f/--file when using -o json/-o yaml",
				args,
				map[string]interface{}{"database": dbname},
			)
		}
		return runPgLegacy("pig postgres psql", args, map[string]interface{}{
			"database": dbname,
			"command":  pgPsqlCommand,
			"file":     pgPsqlFile,
		}, func() error {
			return postgres.Psql(pgConfig, dbname, opts)
		})
	},
}

var pgPsCmd = &cobra.Command{
	Use:     "ps",
	Short:   "Show PostgreSQL connections",
	Aliases: []string{"activity", "act"},
	Annotations: map[string]string{
		"name":       "pig postgres ps",
		"type":       "query",
		"volatility": "volatile",
		"parallel":   "safe",
		"idempotent": "true",
		"risk":       "safe",
		"confirm":    "none",
		"os_user":    "dbsu",
		"cost":       "500",
	},
	Example: `  pig pg ps                      # show client connections
  pig pg ps -a                   # show all connections
  pig pg ps -u admin             # filter by user
  pig pg ps -d mydb              # filter by database`,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := &postgres.PsOptions{
			All:      pgPsAll,
			User:     pgPsUser,
			Database: pgPsDatabase,
		}
		return runPgLegacy("pig postgres ps", args, map[string]interface{}{
			"all":      pgPsAll,
			"user":     pgPsUser,
			"database": pgPsDatabase,
		}, func() error {
			return postgres.Ps(pgConfig, opts)
		})
	},
}

var pgKillCmd = &cobra.Command{
	Use:     "kill",
	Short:   "Kill PostgreSQL connections (dry-run by default)",
	Aliases: []string{"k"},
	Annotations: map[string]string{
		"name":       "pig postgres kill",
		"type":       "action",
		"volatility": "volatile",
		"parallel":   "unsafe",
		"idempotent": "false",
		"risk":       "high",
		"confirm":    "recommended",
		"os_user":    "dbsu",
		"cost":       "1000",
	},
	Example: `  pig pg kill                    # show what would be killed (dry-run)
  pig pg kill -x                 # actually kill connections
  pig pg kill --pid 12345 -x     # kill specific PID
  pig pg kill -u admin -x        # kill connections by user
  pig pg kill -d mydb -x         # kill connections to database
  pig pg kill -s idle -x         # kill idle connections
  pig pg kill --cancel -x        # cancel queries instead of terminate
  pig pg kill -w 5 -x            # repeat every 5 seconds`,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := &postgres.KillOptions{
			Execute: pgKillExecute,
			Pid:     pgKillPid,
			User:    pgKillUser,
			Db:      pgKillDb,
			State:   pgKillState,
			Query:   pgKillQuery,
			All:     pgKillAll,
			Cancel:  pgKillCancel,
			Watch:   pgKillWatch,
		}
		if config.IsStructuredOutput() && pgKillWatch > 0 {
			return structuredParamError(
				output.MODULE_PG,
				"pig postgres kill",
				"watch mode is not supported in structured output",
				"remove --watch/-w when using -o json/-o yaml",
				args,
				map[string]interface{}{"watch": pgKillWatch},
			)
		}
		return runPgLegacy("pig postgres kill", args, map[string]interface{}{
			"execute":  pgKillExecute,
			"pid":      pgKillPid,
			"user":     pgKillUser,
			"database": pgKillDb,
			"state":    pgKillState,
			"query":    pgKillQuery,
			"all":      pgKillAll,
			"cancel":   pgKillCancel,
			"watch":    pgKillWatch,
		}, func() error {
			return postgres.Kill(pgConfig, opts)
		})
	},
}
