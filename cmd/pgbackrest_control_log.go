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
	Use:         "check",
	Aliases:     []string{"ck"},
	Short:       "Verify backup repository",
	Annotations: ancsAnn("pig pgbackrest check", "query", "volatile", "safe", true, "safe", "none", "dbsu", 10000),
	Long:        `Verify the backup repository integrity and configuration.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModulePb, "pig pgbackrest check", args, nil, func() error {
			return pgbackrest.Check(pbConfig)
		})
	},
}

var pbStartCmd = &cobra.Command{
	Use:         "start",
	Aliases:     []string{"on"},
	Short:       "Enable pgBackRest operations",
	Annotations: ancsAnn("pig pgbackrest start", "action", "volatile", "restricted", true, "low", "none", "dbsu", 1000),
	Long:        `Allow pgBackRest to perform operations on the stanza.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModulePb, "pig pgbackrest start", args, nil, func() error {
			return pgbackrest.Start(pbConfig)
		})
	},
}

var pbStopForce bool

var pbStopCmd = &cobra.Command{
	Use:         "stop",
	Aliases:     []string{"off"},
	Short:       "Disable pgBackRest operations",
	Annotations: ancsAnn("pig pgbackrest stop", "action", "volatile", "restricted", true, "medium", "recommended", "dbsu", 1000),
	Long:        `Prevent pgBackRest from performing operations on the stanza (for maintenance).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModulePb, "pig pgbackrest stop", args, map[string]interface{}{
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
	Use:         "log [list|tail|cat]",
	Aliases:     []string{"l", "lg"},
	Short:       "View pgBackRest logs",
	Annotations: ancsAnn("pig pgbackrest log", "query", "volatile", "safe", true, "safe", "none", "dbsu", 500),
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

		return runLegacyStructured(legacyModulePb, "pig pgbackrest log", args, map[string]interface{}{
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
