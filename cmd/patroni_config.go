package cmd

import (
	"pig/cli/patroni"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"
	"strings"

	"github.com/spf13/cobra"
)

// patroniConfigCmd: pig pt config <action> [key=value ...]
var patroniConfigCmd = &cobra.Command{
	Use:     "config <action> [key=value ...]",
	Aliases: []string{"cfg", "c"},
	Short:   "Show or edit cluster config",
	Long: `Manage Patroni cluster configuration.

Actions:
  edit              Interactive config editor
  show              Display current configuration
  set  key=value    Set Patroni config (ttl, loop_wait, etc.)
  pg   key=value    Set PostgreSQL config (max_connections, etc.)`,
	Example: `
  pig pt config edit                      # Interactive edit
  pig pt config show                      # Show current config
  pig pt config show -o json              # Show config as structured JSON
  pig pt config set ttl=60                # Set Patroni config
  pig pt config set ttl=60 loop_wait=15   # Set multiple values
  pig pt config pg max_connections=200    # Set PostgreSQL config
  pig pt config pg shared_buffers=4GB work_mem=256MB`,
	Annotations: mergeAnn(
		ancsAnn("pig patroni config", "action", "volatile", "restricted", false, "medium", "recommended", "dbsu", 3000),
		map[string]string{
			"args.action.desc": "config action to perform",
			"args.action.type": "enum",
		},
	),
	RunE: func(cmd *cobra.Command, args []string) error {
		dbsu := utils.GetDBSU(patroniDBSU)

		if len(args) == 0 {
			// No args: structured output defaults to show, text mode shows help
			if config.IsStructuredOutput() {
				result := patroni.ConfigShowResult(dbsu)
				return handleAuxResult(result)
			}
			return cmd.Help()
		}

		action := args[0]
		kvPairs := args[1:]

		// Filter out non key=value args (should all be k=v after action)
		var filteredKV []string
		for _, arg := range kvPairs {
			if strings.Contains(arg, "=") {
				filteredKV = append(filteredKV, arg)
			}
		}

		switch action {
		case "show":
			if config.IsStructuredOutput() {
				result := patroni.ConfigShowResult(dbsu)
				return handleAuxResult(result)
			}
			return patroni.ConfigShow(dbsu)
		case "edit":
			if config.IsStructuredOutput() {
				return structuredParamError(
					output.MODULE_PT,
					"pig patroni config",
					"interactive config edit is not supported in structured output",
					"use 'pig pt config show -o json' for read-only structured output",
					args,
					map[string]interface{}{"action": action},
				)
			}
			return patroni.ConfigEdit(dbsu)
		case "set":
			return runLegacyStructured(legacyModulePt, "pig patroni config set", args, map[string]interface{}{
				"action": action,
				"pairs":  filteredKV,
			}, func() error {
				return patroni.ConfigSet(dbsu, filteredKV)
			})
		case "pg":
			return runLegacyStructured(legacyModulePt, "pig patroni config pg", args, map[string]interface{}{
				"action": action,
				"pairs":  filteredKV,
			}, func() error {
				return patroni.ConfigPG(dbsu, filteredKV)
			})
		default:
			if config.IsStructuredOutput() {
				return handleAuxResult(
					output.Fail(output.CodePtInvalidConfigAction, "invalid config action").
						WithDetail("unknown action: " + action + " (valid: show, edit, set, pg)"),
				)
			}
			return cmd.Help()
		}
	},
}
