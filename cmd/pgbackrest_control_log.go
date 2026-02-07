package cmd

import (
	"pig/cli/pgbackrest"
	"pig/internal/config"
	"pig/internal/output"

	"github.com/spf13/cobra"
)

// ============================================================================
// Control Commands
// ============================================================================

var pbCheckCmd = &cobra.Command{
	Use:     "check",
	Aliases: []string{"ck"},
	Short:   "Verify backup repository",
	Annotations: map[string]string{
		"name":       "pig pgbackrest check",
		"type":       "query",
		"volatility": "volatile",
		"parallel":   "safe",
		"idempotent": "true",
		"risk":       "safe",
		"confirm":    "none",
		"os_user":    "dbsu",
		"cost":       "10000",
	},
	Long: `Verify the backup repository integrity and configuration.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPbLegacy("pig pgbackrest check", args, nil, func() error {
			return pgbackrest.Check(pbConfig)
		})
	},
}

var pbStartCmd = &cobra.Command{
	Use:     "start",
	Aliases: []string{"on"},
	Short:   "Enable pgBackRest operations",
	Annotations: map[string]string{
		"name":       "pig pgbackrest start",
		"type":       "action",
		"volatility": "volatile",
		"parallel":   "restricted",
		"idempotent": "true",
		"risk":       "low",
		"confirm":    "none",
		"os_user":    "dbsu",
		"cost":       "1000",
	},
	Long: `Allow pgBackRest to perform operations on the stanza.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPbLegacy("pig pgbackrest start", args, nil, func() error {
			return pgbackrest.Start(pbConfig)
		})
	},
}

var pbStopForce bool

var pbStopCmd = &cobra.Command{
	Use:     "stop",
	Aliases: []string{"off"},
	Short:   "Disable pgBackRest operations",
	Annotations: map[string]string{
		"name":       "pig pgbackrest stop",
		"type":       "action",
		"volatility": "volatile",
		"parallel":   "restricted",
		"idempotent": "true",
		"risk":       "medium",
		"confirm":    "recommended",
		"os_user":    "dbsu",
		"cost":       "1000",
	},
	Long: `Prevent pgBackRest from performing operations on the stanza (for maintenance).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPbLegacy("pig pgbackrest stop", args, map[string]interface{}{
			"force": pbStopForce,
		}, func() error {
			return pgbackrest.Stop(pbConfig, &pgbackrest.StopOptions{
				Force: pbStopForce,
			})
		})
	},
}

// ============================================================================
// Log Commands
// ============================================================================

var pbLogLines int

var pbLogCmd = &cobra.Command{
	Use:     "log [list|tail|cat]",
	Aliases: []string{"l", "lg"},
	Short:   "View pgBackRest logs",
	Annotations: map[string]string{
		"name":       "pig pgbackrest log",
		"type":       "query",
		"volatility": "volatile",
		"parallel":   "safe",
		"idempotent": "true",
		"risk":       "safe",
		"confirm":    "none",
		"os_user":    "dbsu",
		"cost":       "500",
	},
	Long: `View pgBackRest log files from /pg/log/pgbackrest/.

Subcommands:
  list  - List available log files (default)
  tail  - Follow latest log file in real-time
  cat   - Display log file contents`,
	Example: `
  pig pb log                       # list log files
  pig pb log list                  # list log files
  pig pb log tail                  # follow latest log
  pig pb log cat                   # show latest log content`,
	RunE: func(cmd *cobra.Command, args []string) error {
		subCmd := "list"
		if len(args) > 0 {
			subCmd = args[0]
		}

		dbsu := pbConfig.DbSU
		if config.IsStructuredOutput() && (subCmd == "tail" || subCmd == "follow" || subCmd == "f") {
			return structuredParamError(
				output.MODULE_PB,
				"pig pgbackrest log",
				"streaming log tail is not supported in structured output",
				"use 'pig pb log cat' in structured mode to get a log snapshot",
				args,
				map[string]interface{}{"subcommand": subCmd},
			)
		}

		return runPbLegacy("pig pgbackrest log", args, map[string]interface{}{
			"subcommand": subCmd,
			"lines":      pbLogLines,
		}, func() error {
			switch subCmd {
			case "list", "ls":
				return pgbackrest.LogList(dbsu)
			case "tail", "follow", "f":
				return pgbackrest.LogTail(dbsu, pbLogLines)
			case "cat", "show":
				filename := ""
				if len(args) > 1 {
					filename = args[1]
				}
				return pgbackrest.LogCat(dbsu, filename, pbLogLines)
			default:
				return pgbackrest.LogList(dbsu)
			}
		})
	},
}
