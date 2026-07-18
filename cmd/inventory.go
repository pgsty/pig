package cmd

import (
	stycli "pig/cli/sty"
	"pig/internal/config"
	"pig/internal/output"

	"github.com/spf13/cobra"
)

var (
	inventoryValidateStrict  bool
	inventoryValidateAnsible bool
)

var inventoryCmd = &cobra.Command{
	Use:         "inventory",
	Aliases:     []string{"inv"},
	Short:       "Inspect, edit, validate, and exchange Pigsty Inventory",
	Annotations: ancsAnn("pig inventory", "query", "stable", "safe", true, "safe", "none", "current", 100),
	GroupID:     "pigsty",
}

var inventoryListCmd = &cobra.Command{
	Use:         "list [selector]",
	Aliases:     []string{"ls"},
	Short:       "List ordered Inventory topology and value kinds",
	Annotations: ancsAnn("pig inventory list", "query", "stable", "safe", true, "safe", "none", "current", 500),
	Args:        cobra.MaximumNArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		return handleAuxResult(pigstyInventoryListExec(stycli.InventoryOptions{
			Path: config.PigstyConfig, Selector: inventorySelector(args),
		}))
	},
}

var inventoryShowCmd = &cobra.Command{
	Use:         "show [selector]",
	Short:       "Show verbatim Inventory YAML (may contain secrets)",
	Annotations: ancsAnn("pig inventory show", "query", "stable", "safe", true, "low", "none", "current", 500),
	Args:        cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		body, failure := pigstyInventoryShowExec(stycli.InventoryOptions{
			Path: config.PigstyConfig, Selector: inventorySelector(args),
		})
		if failure != nil {
			return handleAuxResult(failure)
		}
		if _, err := cmd.OutOrStdout().Write(body); err != nil {
			return handleAuxResult(output.Fail(output.CodeStyInventoryReadFailed, "failed to write inventory output"))
		}
		return nil
	},
}

var inventoryEditCmd = &cobra.Command{
	Use:         "edit [selector]",
	Aliases:     []string{"e"},
	Short:       "Edit Inventory or one selected YAML fragment",
	Annotations: ancsAnn("pig inventory edit", "action", "volatile", "unsafe", false, "medium", "none", "current", 30000),
	Args:        cobra.MaximumNArgs(1),
	Long:        styInventoryEditLong,
	Example: `  pig inventory edit
  pig inv e vars
  pig inv e pg-meta
  pig inv e pg-meta.vars.pg_databases
  pig inv e 'pg-meta.hosts["10.10.10.10"]'`,
	RunE: func(_ *cobra.Command, args []string) error {
		return handleAuxResult(pigstyEditExec(stycli.EditOptions{
			Path: config.PigstyConfig, Selector: inventorySelector(args),
		}))
	},
}

var inventoryValidateCmd = &cobra.Command{
	Use:         "validate [file]",
	Aliases:     []string{"v"},
	Short:       "Validate one complete static Pigsty Inventory",
	Annotations: ancsAnn("pig inventory validate", "query", "stable", "safe", true, "safe", "none", "current", 1000),
	Args:        cobra.MaximumNArgs(1),
	Long:        styInventoryValidateLong,
	Example: `  pig inventory validate
  pig inv v ./pigsty.yml
  pig inv v --ansible
  pig inv v --strict -o json`,
	RunE: func(_ *cobra.Command, args []string) error {
		path := config.PigstyConfig
		if len(args) == 1 {
			path = args[0]
		}
		return handleAuxResult(pigstyValidateExec(stycli.ValidateOptions{
			Path: path, Strict: inventoryValidateStrict, Ansible: inventoryValidateAnsible,
		}))
	},
}

func inventorySelector(args []string) string {
	if len(args) == 1 {
		return args[0]
	}
	return ""
}

func init() {
	inventoryValidateCmd.Flags().BoolVar(&inventoryValidateStrict, "strict", false, "treat warnings as validation failures")
	inventoryValidateCmd.Flags().BoolVar(&inventoryValidateAnsible, "ansible", false, "also parse with a bounded ansible-inventory adapter")
	inventoryCmd.AddCommand(inventoryListCmd, inventoryShowCmd, inventoryEditCmd, inventoryValidateCmd)
}
