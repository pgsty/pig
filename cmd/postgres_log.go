package cmd

import (
	"pig/cli/postgres"
	"pig/internal/config"
	"pig/internal/output"

	"github.com/spf13/cobra"
)

// ============================================================================
// Log Commands
// ============================================================================

var pgLogCmd = &cobra.Command{
	Use:     "log",
	Short:   "View PostgreSQL log files",
	Aliases: []string{"l"},
	Annotations: map[string]string{
		"name":       "pig postgres log",
		"type":       "query",
		"volatility": "volatile",
		"parallel":   "safe",
		"idempotent": "true",
		"risk":       "safe",
		"confirm":    "none",
		"os_user":    "dbsu",
		"cost":       "500",
	},
	Long: `View and search PostgreSQL log files in /pg/log/postgres directory.

  pig pg log list              # list log files
  pig pg log tail              # tail -f latest log
  pig pg log cat [-n 100]      # show last N lines
  pig pg log less              # open in less
  pig pg log grep <pattern>    # search logs`,
}

var pgLogListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List log files",
	Aliases: []string{"ls"},
	Annotations: map[string]string{
		"name":       "pig postgres log list",
		"type":       "query",
		"volatility": "volatile",
		"parallel":   "safe",
		"idempotent": "true",
		"risk":       "safe",
		"confirm":    "none",
		"os_user":    "dbsu",
		"cost":       "500",
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPgLegacy("pig postgres log list", args, map[string]interface{}{
			"log_dir": postgres.GetLogDir(pgConfig),
		}, func() error {
			return postgres.LogList(postgres.GetLogDir(pgConfig))
		})
	},
}

var pgLogTailCmd = &cobra.Command{
	Use:     "tail [file]",
	Short:   "Tail log file (follow mode)",
	Aliases: []string{"t", "f"},
	Annotations: map[string]string{
		"name":       "pig postgres log tail",
		"type":       "query",
		"volatility": "volatile",
		"parallel":   "safe",
		"idempotent": "true",
		"risk":       "safe",
		"confirm":    "none",
		"os_user":    "dbsu",
		"cost":       "0",
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		file := ""
		if len(args) > 0 {
			file = args[0]
		}
		if config.IsStructuredOutput() {
			return structuredParamError(
				output.MODULE_PG,
				"pig postgres log tail",
				"log tail follow mode is not supported in structured output",
				"use 'pig pg log cat -n N -o json' for structured snapshot",
				args,
				map[string]interface{}{"file": file, "lines": pgLogNum},
			)
		}
		return postgres.LogTail(postgres.GetLogDir(pgConfig), file, pgLogNum)
	},
}

var pgLogCatCmd = &cobra.Command{
	Use:     "cat [file]",
	Short:   "Output log file content",
	Aliases: []string{"c"},
	Annotations: map[string]string{
		"name":       "pig postgres log cat",
		"type":       "query",
		"volatility": "volatile",
		"parallel":   "safe",
		"idempotent": "true",
		"risk":       "safe",
		"confirm":    "none",
		"os_user":    "dbsu",
		"cost":       "500",
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		file := ""
		if len(args) > 0 {
			file = args[0]
		}
		return runPgLegacy("pig postgres log cat", args, map[string]interface{}{
			"log_dir": postgres.GetLogDir(pgConfig),
			"file":    file,
			"lines":   pgLogNum,
		}, func() error {
			return postgres.LogCat(postgres.GetLogDir(pgConfig), file, pgLogNum)
		})
	},
}

var pgLogLessCmd = &cobra.Command{
	Use:     "less [file]",
	Short:   "Open log file in less",
	Aliases: []string{"vi", "v"},
	Annotations: map[string]string{
		"name":       "pig postgres log less",
		"type":       "query",
		"volatility": "volatile",
		"parallel":   "safe",
		"idempotent": "true",
		"risk":       "safe",
		"confirm":    "none",
		"os_user":    "dbsu",
		"cost":       "0",
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		file := ""
		if len(args) > 0 {
			file = args[0]
		}
		if config.IsStructuredOutput() {
			return structuredParamError(
				output.MODULE_PG,
				"pig postgres log less",
				"interactive log viewer is not supported in structured output",
				"use 'pig pg log cat -n N -o json' for structured snapshot",
				args,
				map[string]interface{}{"file": file},
			)
		}
		return postgres.LogLess(postgres.GetLogDir(pgConfig), file)
	},
}

var pgLogGrepCmd = &cobra.Command{
	Use:     "grep <pattern> [file]",
	Short:   "Search log files",
	Aliases: []string{"g", "search"},
	Annotations: map[string]string{
		"name":       "pig postgres log grep",
		"type":       "query",
		"volatility": "volatile",
		"parallel":   "safe",
		"idempotent": "true",
		"risk":       "safe",
		"confirm":    "none",
		"os_user":    "dbsu",
		"cost":       "5000",
	},
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		file := ""
		if len(args) > 1 {
			file = args[1]
		}
		return runPgLegacy("pig postgres log grep", args, map[string]interface{}{
			"log_dir":     postgres.GetLogDir(pgConfig),
			"pattern":     args[0],
			"file":        file,
			"ignore_case": pgLogGrepIgnoreCase,
			"context":     pgLogGrepContext,
		}, func() error {
			return postgres.LogGrep(postgres.GetLogDir(pgConfig), args[0], file, pgLogGrepIgnoreCase, pgLogGrepContext)
		})
	},
}
