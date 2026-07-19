package cmd

import (
	"os"
	inventorycli "pig/cli/inventory"
	"pig/internal/config"
	"pig/internal/output"
	"time"

	"github.com/spf13/cobra"
)

var (
	inventoryValidateStrict  bool
	inventoryValidateAnsible bool
	inventoryValidateTimeout time.Duration
	inventoryListDepth       int
	inventoryEditFrom        string
	inventoryCheckProfiles   []string
	inventoryCheckSudo       bool
	inventoryCheckPort       int
	inventoryCheckUser       string
	inventoryCMDBDatabase    string
	inventoryCMDBInitYes     bool
	inventoryCMDBInitPlan    bool
	inventoryCMDBLoadYes     bool
	inventoryCMDBLoadPlan    bool
	inventoryCMDBLoadStrict  bool
	inventoryCMDBDumpForce   bool
	inventoryCMDBEnablePlan  bool
	inventoryCMDBDisablePlan bool
)

var inventoryCmd = &cobra.Command{
	Use:         "inventory",
	Aliases:     []string{"inv"},
	Short:       "Inspect, edit, validate, check, and exchange Pigsty Inventory",
	Annotations: ancsAnn("pig inventory", "query", "stable", "safe", true, "safe", "none", "current", 100),
	GroupID:     "pigsty",
	Args:        cobra.NoArgs,
	RunE:        showCommandHelp,
}

var inventoryStatusCmd = &cobra.Command{
	Use:         "status",
	Short:       "Inspect the active Inventory source without executing it",
	Annotations: ancsAnn("pig inventory status", "query", "stable", "safe", true, "safe", "none", "current", 100),
	Args:        cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		return handleAuxResult(inventoryStatusExec(config.PigstyInventorySource))
	},
}

var inventoryListCmd = &cobra.Command{
	Use:         "list [selector]",
	Aliases:     []string{"ls"},
	Short:       "List ordered Inventory topology and value kinds",
	Annotations: ancsAnn("pig inventory list", "query", "stable", "safe", true, "safe", "none", "current", 500),
	Args:        cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var depth *int
		if cmd.Flags().Changed("depth") {
			if inventoryListDepth < 0 {
				return handleAuxResult(output.Fail(output.CodeInventoryListInvalidArgs, "inventory list depth must be non-negative"))
			}
			value := inventoryListDepth
			depth = &value
		}
		return handleAuxResult(inventoryListExec(inventorycli.InventoryOptions{
			Path: config.PigstyConfig, Selector: inventorySelector(args), Depth: depth,
		}))
	},
}

var inventoryShowCmd = &cobra.Command{
	Use:         "show [selector]",
	Short:       "Show verbatim Inventory YAML (may contain secrets)",
	Annotations: ancsAnn("pig inventory show", "query", "stable", "safe", true, "low", "none", "current", 500),
	Args:        cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		body, failure := inventoryShowExec(inventorycli.InventoryOptions{
			Path: config.PigstyConfig, Selector: inventorySelector(args),
		})
		if failure != nil {
			return handleAuxResult(failure)
		}
		if _, err := cmd.OutOrStdout().Write(body); err != nil {
			return handleAuxResult(output.Fail(output.CodeInventoryReadFailed, "failed to write inventory output"))
		}
		return nil
	},
}

const inventoryEditLong = `Edit the active static Inventory through a private, revision-checked fragment.

The committed file is replaced atomically and is owned by the user running Pig.
Group/other permissions are removed; stricter owner-only permissions are kept.`

var inventoryEditCmd = &cobra.Command{
	Use:         "edit [selector]",
	Aliases:     []string{"e"},
	Short:       "Edit Inventory or one selected YAML fragment",
	Annotations: ancsAnn("pig inventory edit", "action", "volatile", "unsafe", false, "medium", "none", "current", 30000),
	Args:        cobra.MaximumNArgs(1),
	Long:        inventoryEditLong,
	Example: `  pig inventory edit
  pig inv e vars
  pig inv e pg-meta
  pig inv e pg-meta.vars.pg_databases
  pig inv e 'pg-meta.hosts["10.10.10.10"]'`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleAuxResult(inventoryEditExec(inventorycli.EditOptions{
			Path: config.PigstyConfig, Selector: inventorySelector(args),
			From: inventoryEditFrom, FromSet: cmd.Flags().Changed("from"), Stdin: cmd.InOrStdin(),
		}))
	},
}

const inventoryCheckLong = `Check whether the active Inventory and controller appear ready.

The default is deterministic and local: it validates the Inventory, checks the
Pigsty project layout, and inspects required local tools. Remote observations
are explicit with --profile network or --profile ssh. SSH runs only the exact
remote command "true"; --sudo separately permits only "sudo -n true".

External command allowlist:
  ansible-playbook --version
  ssh -V
  ansible-inventory -i <private-candidate> --list   (--profile ansible)
  ssh <fixed noninteractive options> <target> true (--profile ssh)
  ssh <fixed noninteractive options> <target> sudo -n true (--sudo)

Remote probes are observational rather than side-effect-free: authentication
logs, alerts, and lockout counters may change. Every probe has its own timeout.`

var inventoryCheckCmd = &cobra.Command{
	Use:         "check",
	Aliases:     []string{"ck"},
	Short:       "Check Inventory, controller, and opt-in target readiness",
	Annotations: ancsAnn("pig inventory check", "query", "stable", "safe", true, "low", "none", "current", 10000),
	Args:        cobra.NoArgs,
	Long:        inventoryCheckLong,
	Example: `  pig inventory check
  pig inv ck --profile ansible
  pig inv ck --profile network
  pig inv ck --profile ssh --user admin
  pig inv ck --profile ssh --sudo`,
	RunE: func(_ *cobra.Command, _ []string) error {
		return handleAuxResult(inventoryCheckExec(inventorycli.CheckOptions{
			Path: config.PigstyConfig, Home: config.PigstyHome,
			Profiles: inventoryCheckProfiles, Sudo: inventoryCheckSudo,
			Port: inventoryCheckPort, User: inventoryCheckUser,
		}))
	},
}

const inventoryValidateLong = `Validate one complete static Pigsty Inventory with deterministic native rules.

The validator analyzes declarations only; it does not resolve Ansible effective
variables. --ansible adds a bounded compatibility parse whose stdout is discarded.`

var inventoryValidateCmd = &cobra.Command{
	Use:         "validate [file]",
	Aliases:     []string{"v"},
	Short:       "Validate one complete static Pigsty Inventory",
	Annotations: ancsAnn("pig inventory validate", "query", "stable", "safe", true, "safe", "none", "current", 1000),
	Args:        cobra.MaximumNArgs(1),
	Long:        inventoryValidateLong,
	Example: `  pig inventory validate
  pig inv v ./pigsty.yml
  pig inv v --ansible
  pig inv v --strict -o json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if cmd.Flags().Changed("timeout") && !inventoryValidateAnsible {
			return handleAuxResult(output.Fail(output.CodeInventoryValidateInvalidArgs, "--timeout requires --ansible"))
		}
		if inventoryValidateTimeout <= 0 {
			return handleAuxResult(output.Fail(output.CodeInventoryValidateInvalidArgs, "--timeout must be positive"))
		}
		path := config.PigstyConfig
		if len(args) == 1 {
			path = args[0]
		}
		timeout := time.Duration(0)
		if inventoryValidateAnsible {
			timeout = inventoryValidateTimeout
		}
		return handleAuxResult(inventoryValidateExec(inventorycli.ValidateOptions{
			Path: path, Strict: inventoryValidateStrict, Ansible: inventoryValidateAnsible,
			Timeout: timeout,
		}))
	},
}

var inventoryDiffCmd = &cobra.Command{
	Use:         "diff <other-file>",
	Short:       "Compare declarations without emitting Inventory values",
	Annotations: ancsAnn("pig inventory diff", "query", "stable", "safe", true, "safe", "none", "current", 1000),
	Args:        cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		return handleAuxResult(inventoryDiffExec(inventorycli.DiffOptions{Path: config.PigstyConfig, OtherPath: args[0]}))
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

Check is read-only. Init, load, and dump are the data primitives. Enable and
disable only guard and switch the [defaults] inventory directive between
inventory.sh and pigsty.yml.`,
}

var inventoryCMDBCheckCmd = &cobra.Command{
	Use:         "check [file]",
	Short:       "Verify CMDB projections and optional Inventory equivalence",
	Annotations: inventoryCMDBAnnotations("pig inventory cmdb check", "query", "stable", "restricted", true, "low", "none", 10000),
	Args:        cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		options := inventoryCMDBOptions("")
		if len(args) == 1 {
			options.Path, options.ExplicitPath = args[0], true
		}
		return handleAuxResult(inventoryCMDBCheckExec(cmd.Context(), options))
	},
}

var inventoryCMDBInitCmd = &cobra.Command{
	Use:         "init",
	Short:       "Apply the version-matched Pigsty files/cmdb.sql",
	Annotations: mergeAnn(inventoryCMDBAnnotations("pig inventory cmdb init", "action", "volatile", "unsafe", false, "critical", "required", 30000), map[string]string{"plan": "reports replacement scope before confirmation"}),
	Args:        cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		options := inventoryCMDBOptions("")
		options.Yes, options.Plan = inventoryCMDBInitYes, inventoryCMDBInitPlan
		return runInventoryCMDBInit(cmd, options, "pig inventory cmdb init --yes")
	},
}

var inventoryCMDBLoadCmd = &cobra.Command{
	Use:         "load [file]",
	Short:       "Replace existing CMDB declaration rows from static Inventory",
	Annotations: inventoryCMDBAnnotations("pig inventory cmdb load", "action", "volatile", "unsafe", false, "high", "required", 30000),
	Args:        cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := config.PigstyConfig
		explicit := false
		if len(args) == 1 {
			path, explicit = args[0], true
		}
		options := inventoryCMDBOptions(path)
		options.ExplicitPath = explicit
		options.Yes, options.Plan, options.Strict = inventoryCMDBLoadYes, inventoryCMDBLoadPlan, inventoryCMDBLoadStrict
		return runInventoryCMDBLoad(cmd, options, "pig inventory cmdb load --yes")
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
		options := inventoryCMDBOptions(path)
		options.Force = inventoryCMDBDumpForce
		return runInventoryCMDBDump(cmd, options)
	},
}

var inventoryCMDBEnableCmd = &cobra.Command{
	Use:         "enable",
	Short:       "Activate inventory.sh backed by pigsty.inventory",
	Annotations: inventoryCMDBAnnotations("pig inventory cmdb enable", "action", "volatile", "unsafe", true, "medium", "none", 10000),
	Args:        cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		options := inventoryCMDBOptions("")
		options.Plan = inventoryCMDBEnablePlan
		return handleAuxResult(inventoryCMDBEnableExec(cmd.Context(), options))
	},
}

var inventoryCMDBDisableCmd = &cobra.Command{
	Use:         "disable",
	Short:       "Restore the ansible.cfg Inventory source to pigsty.yml",
	Annotations: inventoryCMDBAnnotations("pig inventory cmdb disable", "action", "volatile", "unsafe", true, "medium", "none", 10000),
	Args:        cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		options := inventoryCMDBOptions("")
		options.Plan = inventoryCMDBDisablePlan
		return handleAuxResult(inventoryCMDBDisableExec(options))
	},
}

func inventoryCMDBOptions(path string) inventorycli.InventoryCMDBOptions {
	return inventorycli.InventoryCMDBOptions{
		Home: config.PigstyHome, Database: inventoryCMDBDatabase, Path: path,
		OSCode: config.OSCode, OSArch: config.OSArch, Source: config.PigstyInventorySource,
	}
}

func runInventoryCMDBLoad(cmd *cobra.Command, options inventorycli.InventoryCMDBOptions, confirmationCommand string) error {
	result := inventoryCMDBLoadExec(cmd.Context(), options)
	if result != nil && result.Code == output.CodeInventoryCMDBConfirmationRequired && !options.Yes && !options.Plan &&
		!config.IsStructuredOutput() && inventoryCMDBTextInputIsTerminal() {
		if !pinInventoryCMDBConfirmation(result, &options) {
			return handleAuxResult(result)
		}
		if err := highRiskTextConfirm("This replaces all five Pigsty CMDB declaration tables.", confirmationCommand); err != nil {
			return err
		}
		options.Yes = true
		result = inventoryCMDBLoadExec(cmd.Context(), options)
	}
	return handleAuxResult(result)
}

func runInventoryCMDBInit(cmd *cobra.Command, options inventorycli.InventoryCMDBOptions, confirmationCommand string) error {
	result := inventoryCMDBInitExec(cmd.Context(), options)
	if result != nil && result.Code == output.CodeInventoryCMDBConfirmationRequired && !options.Yes && !options.Plan &&
		!config.IsStructuredOutput() && inventoryCMDBTextInputIsTerminal() {
		if !pinInventoryCMDBConfirmation(result, &options) {
			return handleAuxResult(result)
		}
		if err := highRiskTextConfirm("This replaces the existing pigsty and pglog schemas.", confirmationCommand); err != nil {
			return err
		}
		options.Yes = true
		result = inventoryCMDBInitExec(cmd.Context(), options)
	}
	return handleAuxResult(result)
}

func pinInventoryCMDBConfirmation(result *output.Result, target *inventorycli.InventoryCMDBOptions) bool {
	if result == nil || target == nil {
		return false
	}
	data, ok := result.Data.(*inventorycli.InventoryCMDBData)
	if !ok || data == nil {
		return false
	}
	target.ExpectedSourceRevision = data.SourceRevision
	target.ExpectedTargetDigest = data.TargetDigest
	target.ExpectedDatabaseTarget = data.DatabaseTargetDigest
	switch data.Operation {
	case "init":
		value := data.ExistingSchemas
		target.ExpectedSchemas = &value
	case "load":
	default:
		return false
	}
	return data.SourceRevision != "" && data.DatabaseTargetDigest != "" && (data.Operation != "load" || data.TargetDigest != "") &&
		(data.Operation != "init" || target.ExpectedSchemas != nil)
}

func runInventoryCMDBDump(cmd *cobra.Command, options inventorycli.InventoryCMDBOptions) error {
	body, result := inventoryCMDBDumpExec(cmd.Context(), options)
	if result == nil || !result.Success || body == nil {
		return handleAuxResult(result)
	}
	if _, err := cmd.OutOrStdout().Write(body); err != nil {
		return handleAuxResult(output.Fail(output.CodeInventoryCMDBOutputFailed, "CMDB dump stdout write failed"))
	}
	return nil
}

func inventorySelector(args []string) string {
	if len(args) == 1 {
		return args[0]
	}
	return ""
}

func cmdbTextInputIsTerminal() bool {
	info, err := os.Stdin.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

var inventoryCMDBTextInputIsTerminal = cmdbTextInputIsTerminal

func addCMDBDatabaseFlags(target *string, commands ...*cobra.Command) {
	for _, command := range commands {
		command.Flags().StringVarP(target, "database", "d", "", "target database name, URI, or libpq conninfo (default: METADB_URL, then service=meta)")
	}
}

func init() {
	inventoryListCmd.Flags().IntVar(&inventoryListDepth, "depth", 0, "limit traversal depth from the selected node (non-negative)")
	inventoryEditCmd.Flags().StringVar(&inventoryEditFrom, "from", "", "replace the selected fragment from a regular file or stdin (-)")
	inventoryCheckCmd.Flags().StringSliceVarP(&inventoryCheckProfiles, "profile", "p", nil, "additional profile: ansible, network, or ssh")
	inventoryCheckCmd.Flags().BoolVar(&inventoryCheckSudo, "sudo", false, "with the ssh profile, also observe sudo -n true")
	inventoryCheckCmd.Flags().IntVar(&inventoryCheckPort, "port", 22, "TCP/SSH target port (no Inventory effective-var inference)")
	inventoryCheckCmd.Flags().StringVar(&inventoryCheckUser, "user", "", "explicit SSH user (defaults to the controller user)")
	inventoryValidateCmd.Flags().BoolVar(&inventoryValidateStrict, "strict", false, "treat warnings as validation failures")
	inventoryValidateCmd.Flags().BoolVar(&inventoryValidateAnsible, "ansible", false, "also parse with a bounded ansible-inventory adapter")
	inventoryValidateCmd.Flags().DurationVar(&inventoryValidateTimeout, "timeout", 10*time.Second, "timeout for --ansible compatibility validation")
	addCMDBDatabaseFlags(&inventoryCMDBDatabase, inventoryCMDBCheckCmd, inventoryCMDBInitCmd, inventoryCMDBLoadCmd, inventoryCMDBDumpCmd, inventoryCMDBEnableCmd)
	inventoryCMDBInitCmd.Flags().BoolVarP(&inventoryCMDBInitYes, "yes", "y", false, "confirm replacement of existing pigsty and pglog schemas")
	inventoryCMDBInitCmd.Flags().BoolVar(&inventoryCMDBInitPlan, "plan", false, "inspect and report the init plan without mutation")
	inventoryCMDBInitCmd.MarkFlagsMutuallyExclusive("plan", "yes")
	inventoryCMDBLoadCmd.Flags().BoolVarP(&inventoryCMDBLoadYes, "yes", "y", false, "confirm replacement of all CMDB declaration rows")
	inventoryCMDBLoadCmd.Flags().BoolVar(&inventoryCMDBLoadPlan, "plan", false, "validate and report the load plan without mutation")
	inventoryCMDBLoadCmd.Flags().BoolVar(&inventoryCMDBLoadStrict, "strict", false, "treat Inventory validation warnings as failures")
	inventoryCMDBLoadCmd.MarkFlagsMutuallyExclusive("plan", "yes")
	inventoryCMDBDumpCmd.Flags().BoolVarP(&inventoryCMDBDumpForce, "force", "f", false, "replace a differing regular destination")
	inventoryCMDBEnableCmd.Flags().BoolVar(&inventoryCMDBEnablePlan, "plan", false, "inspect the guarded source switch without writing files")
	inventoryCMDBDisableCmd.Flags().BoolVar(&inventoryCMDBDisablePlan, "plan", false, "inspect the guarded source switch without writing files")
	inventoryCMDBCmd.AddCommand(inventoryCMDBCheckCmd, inventoryCMDBInitCmd, inventoryCMDBLoadCmd, inventoryCMDBDumpCmd, inventoryCMDBEnableCmd, inventoryCMDBDisableCmd)
	inventoryCmd.AddCommand(inventoryStatusCmd, inventoryListCmd, inventoryShowCmd, inventoryEditCmd, inventoryValidateCmd, inventoryCheckCmd, inventoryDiffCmd, inventoryCMDBCmd)
}

var (
	inventoryStatusExec      = inventorycli.InventoryStatus
	inventoryListExec        = inventorycli.ListInventory
	inventoryShowExec        = inventorycli.ShowInventory
	inventoryEditExec        = inventorycli.Edit
	inventoryDiffExec        = inventorycli.DiffInventory
	inventoryValidateExec    = inventorycli.ValidateInventory
	inventoryCheckExec       = inventorycli.Check
	inventoryCMDBCheckExec   = inventorycli.InventoryCMDBCheck
	inventoryCMDBInitExec    = inventorycli.InventoryCMDBInit
	inventoryCMDBLoadExec    = inventorycli.InventoryCMDBLoad
	inventoryCMDBDumpExec    = inventorycli.InventoryCMDBDump
	inventoryCMDBEnableExec  = inventorycli.InventoryCMDBEnable
	inventoryCMDBDisableExec = inventorycli.InventoryCMDBDisable
)
