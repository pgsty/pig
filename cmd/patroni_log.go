package cmd

import (
	"pig/cli/patroni"
	"pig/internal/config"
	"pig/internal/output"

	"github.com/spf13/cobra"
)

// patroniLogCmd: pig pt log
var patroniLogCmd = &cobra.Command{
	Use:         "log",
	Aliases:     []string{"l", "lg"},
	Short:       "View patroni logs",
	Annotations: ancsAnn("pig patroni log", "query", "volatile", "safe", true, "safe", "none", "dbsu", 500),
	Long:        `View patroni service logs using journalctl.`,
	Example: `
  pig pt log          # View recent logs
  pig pt log -f       # Follow logs
  pig pt log -n 100   # Show last 100 lines`,
	RunE: func(cmd *cobra.Command, args []string) error {
		follow, _ := cmd.Flags().GetBool("follow")
		lines, _ := cmd.Flags().GetString("lines")
		if config.IsStructuredOutput() && follow {
			return structuredParamError(
				output.MODULE_PT,
				"pig patroni log",
				"log follow mode is not supported in structured output",
				"use 'pig pt log -n N -o json' without --follow for structured snapshot",
				args,
				map[string]interface{}{"follow": follow, "lines": lines},
			)
		}
		return runLegacyStructured(legacyModulePt, "pig patroni log", args, map[string]interface{}{
			"follow": follow,
			"lines":  lines,
		}, func() error {
			return patroni.Log(follow, lines)
		})
	},
}
