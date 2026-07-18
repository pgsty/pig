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
	inventoryCMDBDatabase    string
	inventoryCMDBInitYes     bool
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

func inventoryCMDBAnnotations(name, typ, volatility, parallel string, idempotent bool, risk, confirm string, cost int) map[string]string {
	return mergeAnn(ancsAnn(name, typ, volatility, parallel, idempotent, risk, confirm, "current", cost), map[string]string{"maturity": "experimental"})
}

var inventoryCMDBCmd = &cobra.Command{
	Use:         "cmdb",
	Short:       "Exchange Inventory with Pigsty's existing PostgreSQL CMDB",
	Annotations: inventoryCMDBAnnotations("pig inventory cmdb", "query", "volatile", "restricted", true, "low", "none", 100),
	Long: `Use the existing files/cmdb.sql schema and Inventory projections.

The data primitives are init, load, and dump. Enable and disable only switch
the [defaults] inventory directive between inventory.sh and pigsty.yml.`,
}

var inventoryCMDBInitCmd = &cobra.Command{
	Use:         "init",
	Short:       "Apply the version-matched Pigsty files/cmdb.sql",
	Annotations: mergeAnn(inventoryCMDBAnnotations("pig inventory cmdb init", "action", "volatile", "unsafe", false, "critical", "required", 30000), map[string]string{"plan": "reports replacement scope before confirmation"}),
	Args:        cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		options := inventoryCMDBOptions("")
		options.Yes = inventoryCMDBInitYes
		result := inventoryCMDBInitExec(cmd.Context(), options)
		if result != nil && result.Code == output.CodeStyCMDBConfirmationRequired && !options.Yes &&
			!config.IsStructuredOutput() && styCMDBTextInputIsTerminal() {
			if err := highRiskTextConfirm("This replaces the existing pigsty and pglog schemas.", "pig inventory cmdb init --yes"); err != nil {
				return err
			}
			options.Yes = true
			result = inventoryCMDBInitExec(cmd.Context(), options)
		}
		return handleAuxResult(result)
	},
}

var inventoryCMDBLoadCmd = &cobra.Command{
	Use:         "load [file]",
	Short:       "Replace existing CMDB declaration rows from static Inventory",
	Annotations: inventoryCMDBAnnotations("pig inventory cmdb load", "action", "volatile", "unsafe", false, "high", "recommended", 30000),
	Args:        cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := config.PigstyConfig
		if len(args) == 1 {
			path = args[0]
		}
		return handleAuxResult(inventoryCMDBLoadExec(cmd.Context(), inventoryCMDBOptions(path)))
	},
}

var inventoryCMDBDumpCmd = &cobra.Command{
	Use:         "dump [file]",
	Short:       "Generate canonical YAML from pigsty.raw_config",
	Annotations: inventoryCMDBAnnotations("pig inventory cmdb dump", "query", "stable", "restricted", true, "low", "none", 30000),
	Args:        cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := ""
		if len(args) == 1 {
			path = args[0]
		}
		body, result := inventoryCMDBDumpExec(cmd.Context(), inventoryCMDBOptions(path))
		if result == nil || !result.Success {
			return handleAuxResult(result)
		}
		if body == nil {
			return handleAuxResult(result)
		}
		if _, err := cmd.OutOrStdout().Write(body); err != nil {
			return handleAuxResult(output.Fail(output.CodeStyCMDBOutputFailed, "CMDB dump stdout write failed"))
		}
		return nil
	},
}

var inventoryCMDBEnableCmd = &cobra.Command{
	Use:         "enable",
	Short:       "Activate inventory.sh backed by pigsty.inventory",
	Annotations: inventoryCMDBAnnotations("pig inventory cmdb enable", "action", "volatile", "unsafe", true, "medium", "none", 10000),
	Args:        cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return handleAuxResult(inventoryCMDBEnableExec(cmd.Context(), inventoryCMDBOptions("")))
	},
}

var inventoryCMDBDisableCmd = &cobra.Command{
	Use:         "disable",
	Short:       "Restore the ansible.cfg Inventory source to pigsty.yml",
	Annotations: inventoryCMDBAnnotations("pig inventory cmdb disable", "action", "volatile", "unsafe", true, "medium", "none", 10000),
	Args:        cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		return handleAuxResult(inventoryCMDBDisableExec(inventoryCMDBOptions("")))
	},
}

func inventoryCMDBOptions(path string) stycli.InventoryCMDBOptions {
	return stycli.InventoryCMDBOptions{
		Home: config.PigstyHome, Database: inventoryCMDBDatabase, Path: path,
		OSCode: config.OSCode, OSArch: config.OSArch,
	}
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
	inventoryCMDBCmd.PersistentFlags().StringVarP(&inventoryCMDBDatabase, "database", "d", "", "target database name, URI, or libpq conninfo (default: METADB_URL, then service=meta)")
	inventoryCMDBCmd.PersistentFlags().StringVar(&inventoryCMDBDatabase, "data", "", "compatibility alias for --database")
	_ = inventoryCMDBCmd.PersistentFlags().MarkDeprecated("data", "use --database")
	inventoryCMDBInitCmd.Flags().BoolVar(&inventoryCMDBInitYes, "yes", false, "confirm replacement of existing pigsty and pglog schemas")
	inventoryCMDBCmd.AddCommand(inventoryCMDBInitCmd, inventoryCMDBLoadCmd, inventoryCMDBDumpCmd, inventoryCMDBEnableCmd, inventoryCMDBDisableCmd)
	inventoryCmd.AddCommand(inventoryListCmd, inventoryShowCmd, inventoryEditCmd, inventoryValidateCmd, inventoryCMDBCmd)
}

var (
	inventoryCMDBInitExec    = stycli.InventoryCMDBInit
	inventoryCMDBLoadExec    = stycli.InventoryCMDBLoad
	inventoryCMDBDumpExec    = stycli.InventoryCMDBDump
	inventoryCMDBEnableExec  = stycli.InventoryCMDBEnable
	inventoryCMDBDisableExec = stycli.InventoryCMDBDisable
)
