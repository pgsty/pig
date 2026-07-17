package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"pig/cli/ext"
	stycli "pig/cli/sty"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"
	"strings"

	"github.com/spf13/cobra"
)

var (
	pigstyInitPath   string
	pigstyInitForce  bool
	pigstyInitMirror bool

	pigstyBootRegion  string
	pigstyBootPackage string
	pigstyBootKeep    bool
	pigstyBootMirror  bool

	pigstyConfName           string
	pigstyConfIP             string
	pigstyConfVer            string
	pigstyConfRegion         string
	pigstyConfOutput         string
	pigstyConfSkip           bool
	pigstyConfProxy          bool
	pigstyConfNonInteractive bool
	pigstyConfPort           string
	pigstyConfGenerate       bool
	pigstyConfRaw            bool
	pigstyConfMirror         bool
	pigstyValidateStrict     bool
	pigstyValidateAnsible    bool
	pigstyCheckProfiles      []string
	pigstyCheckSudo          bool
	pigstyCheckPort          int
	pigstyCheckUser          string
	pigstyDashboardEndpoint  string
	pigstyDashboardUsername  string
	pigstyDashboardPassword  string
	pigstyDashboardManifest  string
	pigstyDashboardDumpDir   string
	pigstyDashboardPrune     bool
	pigstyDashboardYes       bool
	pigstyCMDBConnectionFile string
	pigstyCMDBReset          bool
	pigstyCMDBRestore        string
	pigstyCMDBInitYes        bool
	pigstyCMDBLoadRevision   string
	pigstyCMDBLoadPrune      bool
	pigstyCMDBLoadYes        bool
	pigstyCMDBDumpOutput     string
	pigstyCMDBDumpRevision   string
	pigstyCMDBDumpMerge      bool
	pigstyCMDBDumpExpectBase string
	pigstyCMDBDisableStatic  string

	pigstyDownloadDir string
	pigstyVersion     string
	pigstyGetMirror   bool
	pigstyListMirror  bool
)

var styCmd = &cobra.Command{
	Use:         "sty",
	Short:       "Manage Pigsty Installation",
	Annotations: ancsAnn("pig sty", "query", "stable", "safe", true, "safe", "none", "current", 100),
	Aliases:     []string{"s", "pigsty"},
	GroupID:     "pigsty",
	Long: `pig sty -Init (Download), Bootstrap, Configure, and Deploy Pigsty

  pig sty init    [-mpfvd]        # install pigsty (~/pigsty by default)
  pig sty boot    [-rmpk]         # install ansible and prepare offline pkg
  pig sty conf    [-cvmrsoxnpg --raw] # configure pigsty and generate config
  pig sty inventory list [selector]     # list ordered, redacted inventory metadata
  pig sty inventory show [selector]     # show verbatim YAML in text mode
  pig sty inventory edit [selector]     # edit inventory or one YAML fragment
  pig sty inventory validate [file]     # validate a Pigsty inventory
  pig sty validate [file]               # delegate to inventory validate
  pig sty check                         # local readiness; remote profiles are opt-in
  pig sty dashboard list|check|diff     # inspect repository-managed Grafana state
  pig sty dashboard init|dump|clean     # reconcile, export, or remove managed UIDs
  pig sty cmdb init|load|dump|check      # experimental native CMDB control plane
  pig sty cmdb enable|disable            # atomically switch the persistent source
  pig sty deploy                  # use pigsty to deploy everything (CAUTION!)
  pig sty get                     # download pigsty source tarball
  pig sty list                    # list available pigsty versions
`,
	Example: `  Get Started: https://pigsty.io/docs/setup/install/
  pig sty init                 # extract and init ~/pigsty
  pig sty boot                 # install ansible & other deps
  pig sty conf                 # generate pigsty.yml config file
  pig sty deploy               # run the deploy.yml playbook`,
}

var styConfigureLong = fmt.Sprintf(`Configure pigsty with native workflow (default) or raw shell workflow

pig sty conf (aliases: c, configure)
  [-c|--conf <name>]      # config template: [meta|rich|slim|full|supabase|...]
  [--ip <ip>]             # primary IP address (skip with -s)
  [-v|--version <pgver>]  # postgres major version: [%s] (beta: %d)
  [-r|--region <region>]  # upstream repo region: [default|china|europe]
  [-m|--mirror]           # same as --region china
  [-O|--output-file <file>]    # output config file path (default: pigsty.yml)
  [-s|--skip]             # skip IP address probing
  [-p|--port <port>]      # specify SSH port (for ssh accessibility check)
  [-x|--proxy]            # write proxy env from environment
  [-n|--non-interactive]  # non-interactive mode

  [-g|--generate]         # generate random default passwords (RECOMMENDED!)
  [--raw]                 # use raw legacy shell configure workflow

Check https://pigsty.io/docs/setup/install/#configure for details
`, strings.Join(ext.PostgresActiveVersionStrings(), "|"), ext.PostgresBetaMajorVersion)

const styConfigureExample = `
  pig sty conf                       # native Go configure path (default)
  pig sty conf --raw                 # raw shell configure path fallback
  pig sty configure                  # same as 'pig sty conf' (alias)
  pig sty conf -g                    # generate with random passwords (RECOMMENDED!)
  pig sty conf -c rich               # use conf/rich.yml template (with more extensions)
  pig sty conf -c ha/full            # use conf/ha/full.yml 4-node ha template
  pig sty conf -c slim               # use conf/slim.yml template (minimal install)
  pig sty conf -c supabase           # use conf/supabase.yml template (self-hosting)
  pig sty conf -v 17 -c rich         # use conf/rich.yml template with PostgreSQL 17
  pig sty conf -r china -s           # use china region mirrors, skip IP probe
  pig sty conf -m -s                 # use mirror mode, skip IP probe
  pig sty conf -x                    # write proxy env from environment to config
  pig sty conf -c full -g -O ha.yml  # full HA template with random passwords to ha.yml
`

var pigstyInitCmd = &cobra.Command{
	Use:         "init [version]",
	Short:       "Install Pigsty",
	Annotations: ancsAnn("pig sty init", "action", "volatile", "unsafe", false, "medium", "recommended", "root", 30000),
	Aliases:     []string{"i"},
	Args:        cobra.MaximumNArgs(1),
	Long: `
pig sty init
  -p | --path    : where to install, ~/pigsty by default
  -f | --force   : force overwrite existing pigsty dir
  -m | --mirror  : prefer mirror (pigsty.cc) as primary source
  -v | --version : pigsty version, latest by default
  -d | --dir     : download directory, /tmp by default

`,
	Example: `
  pig sty init                   # install to ~/pigsty with the latest version
  pig sty init -f                # install and OVERWRITE existing pigsty dir
  pig sty init -m                # install from mirror source
  pig sty init -p /tmp/pigsty    # install to another location /tmp/pigsty
  pig sty init -v 3.4            # get & install specific version v3.4.1
  pig sty init 3                 # get & install specific version v3 latest
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModuleSty, "pig sty init", args, map[string]interface{}{
			"path":    pigstyInitPath,
			"force":   pigstyInitForce,
			"mirror":  pigstyInitMirror,
			"version": pigstyVersion,
			"dir":     pigstyDownloadDir,
		}, func() error {
			return pigstyInitExec(stycli.InitOptions{
				TargetPath:  pigstyInitPath,
				Force:       pigstyInitForce,
				Mirror:      pigstyInitMirror,
				Version:     pigstyVersion,
				DownloadDir: pigstyDownloadDir,
				Args:        args,
			})
		})
	},
}

var pigstyInitExec = stycli.Init

var pigstyBootCmd = &cobra.Command{
	Use:         "boot",
	Short:       "Bootstrap Pigsty",
	Annotations: ancsAnn("pig sty boot", "action", "volatile", "unsafe", true, "low", "none", "root", 60000),
	Aliases:     []string{"b", "bootstrap"},
	Args:        cobra.NoArgs,
	Long: `Bootstrap pigsty with ./bootstrap script

pig sty boot
  [-r|--region <region]   [default,china,europe]
  [-m|--mirror]           same as --region china
  [-p|--path <path>]      specify another offline pkg path
  [-k|--keep]             keep existing upstream repo during bootstrap

Check https://pigsty.io/docs/setup/offline/#bootstrap for details
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		bootRegion := pigstyBootRegion
		if pigstyBootMirror {
			bootRegion = "china"
		}
		return runLegacyStructured(legacyModuleSty, "pig sty boot", args, map[string]interface{}{
			"region":   bootRegion,
			"mirror":   pigstyBootMirror,
			"path":     pigstyBootPackage,
			"keep":     pigstyBootKeep,
			"pig_home": config.PigstyHome,
		}, func() error {
			return stycli.Bootstrap(stycli.BootstrapOptions{
				PigstyHome:  config.PigstyHome,
				Region:      bootRegion,
				PackagePath: pigstyBootPackage,
				Keep:        pigstyBootKeep,
			})
		})
	},
}

var pigstyConfCmd = &cobra.Command{
	Use:         "conf",
	Short:       "Configure Pigsty",
	Annotations: ancsAnn("pig sty conf", "action", "volatile", "safe", true, "medium", "recommended", "root", 10000),
	Aliases:     []string{"c", "configure"},
	Args:        cobra.NoArgs,
	Long:        styConfigureLong,
	Example:     styConfigureExample,
	RunE: func(cmd *cobra.Command, args []string) error {
		command := "pig sty conf"
		if cmd != nil {
			if called := strings.TrimSpace(cmd.CalledAs()); called != "" {
				command = "pig sty " + called
			}
		}
		confRegion := pigstyConfRegion
		if pigstyConfMirror {
			confRegion = "china"
		}
		if pigstyConfRaw {
			return runLegacyStructured(legacyModuleSty, command, args, styConfigureParams(confRegion), func() error {
				return stycli.ConfigureRaw(stycli.RawConfigureOptions{
					PigstyHome:     config.PigstyHome,
					Mode:           pigstyConfName,
					PrimaryIP:      pigstyConfIP,
					PGVersion:      pigstyConfVer,
					Region:         confRegion,
					SSHPort:        pigstyConfPort,
					OutputFile:     pigstyConfOutput,
					Skip:           pigstyConfSkip,
					UseProxy:       pigstyConfProxy,
					NonInteractive: pigstyConfNonInteractive,
					Generate:       pigstyConfGenerate,
					Args:           args,
				})
			})
		}
		return runPigstyConfigureNative(command, args, confRegion)
	},
}

var pigstyInventoryCmd = &cobra.Command{
	Use:         "inventory",
	Aliases:     []string{"inv"},
	Short:       "Inspect and edit the active Pigsty Inventory",
	Annotations: ancsAnn("pig sty inventory", "query", "stable", "safe", true, "safe", "none", "current", 100),
}

var pigstyInventoryListCmd = &cobra.Command{
	Use:         "list [selector]",
	Short:       "List ordered Inventory topology and value kinds",
	Annotations: ancsAnn("pig sty inventory list", "query", "stable", "safe", true, "safe", "none", "current", 500),
	Args:        cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleAuxResult(pigstyInventoryListExec(stycli.InventoryOptions{Path: config.PigstyConfig, Selector: styInventorySelector(args)}))
	},
}

var pigstyInventoryShowCmd = &cobra.Command{
	Use:         "show [selector]",
	Short:       "Show verbatim Inventory YAML (may contain secrets)",
	Annotations: ancsAnn("pig sty inventory show", "query", "stable", "safe", true, "low", "none", "current", 500),
	Args:        cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		body, failure := pigstyInventoryShowExec(stycli.InventoryOptions{Path: config.PigstyConfig, Selector: styInventorySelector(args)})
		if failure != nil {
			return handleAuxResult(failure)
		}
		if _, err := cmd.OutOrStdout().Write(body); err != nil {
			return handleAuxResult(output.Fail(output.CodeStyInventoryReadFailed, "failed to write inventory output"))
		}
		return nil
	},
}

const styInventoryEditLong = `Edit the active static Inventory through a private, revision-checked fragment.

The committed file is replaced atomically and is owned by the user running Pig.
Group/other permissions are removed; stricter owner-only permissions are kept.`

var pigstyEditCmd = &cobra.Command{
	Use:         "edit [selector]",
	Short:       "Edit Pigsty inventory or a selected YAML fragment",
	Annotations: ancsAnn("pig sty edit", "action", "volatile", "unsafe", false, "medium", "none", "current", 30000),
	Aliases:     []string{"e"},
	Args:        cobra.MaximumNArgs(1),
	Long:        styInventoryEditLong,
	Example: `  pig sty edit
  pig sty edit vars
  pig sty edit pg-meta
  pig sty edit pg-meta.vars.pg_databases
  pig sty edit 'pg-meta.hosts["10.10.10.10"]'`,
	RunE: runPigstyInventoryEdit,
}

var pigstyInventoryEditCmd = &cobra.Command{
	Use:         "edit [selector]",
	Short:       "Edit Pigsty inventory or a selected YAML fragment",
	Annotations: ancsAnn("pig sty inventory edit", "action", "volatile", "unsafe", false, "medium", "none", "current", 30000),
	Args:        cobra.MaximumNArgs(1),
	Long:        styInventoryEditLong,
	RunE:        runPigstyInventoryEdit,
}

func runPigstyInventoryEdit(_ *cobra.Command, args []string) error {
	return handleAuxResult(pigstyEditExec(stycli.EditOptions{Path: config.PigstyConfig, Selector: styInventorySelector(args)}))
}

func styInventorySelector(args []string) string {
	if len(args) == 1 {
		return args[0]
	}
	return ""
}

const styInventoryValidateLong = `Validate one complete static Pigsty Inventory with deterministic native rules.

The validator analyzes declarations only; it does not resolve Ansible effective
variables. --ansible adds a bounded compatibility parse whose stdout is discarded.`

var pigstyInventoryValidateCmd = &cobra.Command{
	Use:         "validate [file]",
	Short:       "Validate a complete static Pigsty Inventory",
	Annotations: ancsAnn("pig sty inventory validate", "query", "stable", "safe", true, "safe", "none", "current", 1000),
	Args:        cobra.MaximumNArgs(1),
	Long:        styInventoryValidateLong,
	Example: `  pig sty inventory validate
  pig sty inventory validate ./pigsty.yml
  pig sty inventory validate --ansible
  pig sty inventory validate --strict -o json`,
	RunE: runPigstyInventoryValidate,
}

var pigstyValidateCmd = &cobra.Command{
	Use:         "validate [file]",
	Short:       "Validate a Pigsty Inventory (inventory validate delegate)",
	Annotations: ancsAnn("pig sty validate", "query", "stable", "safe", true, "safe", "none", "current", 1000),
	Aliases:     []string{"val", "v"},
	Args:        cobra.MaximumNArgs(1),
	Long:        styInventoryValidateLong,
	Example: `  pig sty validate
  pig sty validate ./pigsty.yml
  pig sty validate --ansible
  pig sty validate --strict -o json`,
	RunE: runPigstyInventoryValidate,
}

func runPigstyInventoryValidate(_ *cobra.Command, args []string) error {
	path := config.PigstyConfig
	if len(args) == 1 {
		path = args[0]
	}
	return handleAuxResult(pigstyValidateExec(stycli.ValidateOptions{
		Path: path, Strict: pigstyValidateStrict, Ansible: pigstyValidateAnsible,
	}))
}

func addStyValidateFlags(command *cobra.Command) {
	command.Flags().BoolVar(&pigstyValidateStrict, "strict", false, "treat warnings as validation failures")
	command.Flags().BoolVar(&pigstyValidateAnsible, "ansible", false, "also parse with a bounded ansible-inventory adapter")
}

const styCheckLong = `Check whether the active Inventory and controller appear ready.

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

var pigstyCheckCmd = &cobra.Command{
	Use:         "check",
	Short:       "Check controller and opt-in target readiness",
	Annotations: ancsAnn("pig sty check", "query", "stable", "safe", true, "low", "none", "current", 10000),
	Aliases:     []string{"ck"},
	Args:        cobra.NoArgs,
	Long:        styCheckLong,
	Example: `  pig sty check
  pig sty check --profile ansible
  pig sty check --profile network
  pig sty check --profile ssh --user admin
  pig sty check --profile ssh --sudo`,
	RunE: func(_ *cobra.Command, _ []string) error {
		return handleAuxResult(pigstyCheckExec(stycli.CheckOptions{
			Path: config.PigstyConfig, Home: config.PigstyHome,
			Profiles: pigstyCheckProfiles, Sudo: pigstyCheckSudo,
			Port: pigstyCheckPort, User: pigstyCheckUser,
		}))
	},
}

func styCMDBAnnotations(name, typ, volatility, parallel string, idempotent bool, risk, confirm string, cost int) map[string]string {
	return mergeAnn(
		ancsAnn(name, typ, volatility, parallel, idempotent, risk, confirm, "current", cost),
		map[string]string{"maturity": "experimental"},
	)
}

var pigstyCMDBCmd = &cobra.Command{
	Use:         "cmdb",
	Short:       "Manage the experimental native Pigsty CMDB",
	Annotations: styCMDBAnnotations("pig sty cmdb", "query", "volatile", "restricted", true, "low", "none", 100),
	Long: `Manage the experimental declaration-oriented native CMDB.

This surface never calls the legacy destructive CMDB scripts and never
computes Ansible effective variables. Connection values are accepted only
through --connection-file, PIGSTY_CMDB_DSN, or service=meta.`,
}

var pigstyCMDBInitCmd = &cobra.Command{
	Use:         "init",
	Short:       "Apply native migrations and prepare the private adapter",
	Annotations: mergeAnn(styCMDBAnnotations("pig sty cmdb init", "action", "volatile", "unsafe", false, "critical", "required", 30000), map[string]string{"plan": "reset renders an exact native plan"}),
	Args:        cobra.NoArgs,
	Long: `Apply idempotent native migrations and refresh the private adapter.

--reset selects destructive scope but never grants consent. A reset first
renders an exact value-free plan and requires an interactive terminal or
--yes. --restore is valid only together with --reset and is fully verified
before confirmation.`,
	RunE: runPigstyCMDBInit,
}

var pigstyCMDBLoadCmd = &cobra.Command{
	Use:   "load [static-file]",
	Short: "Plan and append one verified Inventory Snapshot",
	Annotations: mergeAnn(styCMDBAnnotations("pig sty cmdb load", "action", "volatile", "unsafe", false, "high", "required", 30000), map[string]string{
		"args.static-file.desc": "optional static Inventory file",
		"args.static-file.type": "path",
	}),
	Args: cobra.MaximumNArgs(1),
	Long: `Load exactly one validated static Inventory or historical revision.

Declaration removals require both --prune scope and confirmation. Historical
rollback always requires confirmation and creates a new append-only head.`,
	RunE: runPigstyCMDBLoad,
}

var pigstyCMDBDumpCmd = &cobra.Command{
	Use:   "dump",
	Short: "Write a verified canonical or lossless merged Inventory",
	Annotations: mergeAnn(styCMDBAnnotations("pig sty cmdb dump", "action", "stable", "restricted", false, "low", "none", 30000), map[string]string{
		"flags.format.choices": "text,yaml,json,json-pretty",
	}),
	Args: cobra.NoArgs,
	Long: `Write secret-bearing Inventory content only to a new owner-only file.

The command-local --output is the destination path. For structured metadata,
use the dump-local spelling of the global -o contract, for example:
  pig sty cmdb dump -o json --output /private/generated.yml`,
	RunE: runPigstyCMDBDump,
}

var pigstyCMDBEnableCmd = &cobra.Command{
	Use:         "enable",
	Short:       "Atomically select the verified native Inventory adapter",
	Annotations: styCMDBAnnotations("pig sty cmdb enable", "action", "volatile", "unsafe", true, "medium", "none", 10000),
	Args:        cobra.NoArgs,
	RunE:        runPigstyCMDBEnable,
}

var pigstyCMDBDisableCmd = &cobra.Command{
	Use:         "disable",
	Short:       "Atomically restore one verified static Inventory source",
	Annotations: styCMDBAnnotations("pig sty cmdb disable", "action", "volatile", "unsafe", true, "medium", "none", 10000),
	Args:        cobra.NoArgs,
	RunE:        runPigstyCMDBDisable,
}

var pigstyCMDBCheckCmd = &cobra.Command{
	Use:         "check",
	Short:       "Read-only verification of native state and active-source parity",
	Annotations: styCMDBAnnotations("pig sty cmdb check", "query", "volatile", "safe", true, "low", "none", 10000),
	Args:        cobra.NoArgs,
	RunE:        runPigstyCMDBCheck,
}

var pigstyCMDBInventoryAdapterCmd = &cobra.Command{
	Use:                "_inventory",
	Short:              "private native Ansible Inventory protocol adapter",
	Annotations:        styCMDBAnnotations("pig sty cmdb _inventory", "query", "volatile", "safe", true, "low", "none", 10000),
	Hidden:             true,
	DisableFlagParsing: true,
	Args:               cobra.ArbitraryArgs,
	RunE:               runPigstyCMDBInventoryAdapter,
}

func runPigstyCMDBInit(cmd *cobra.Command, _ []string) error {
	pigBinary, failure := styCMDBCurrentBinary()
	if failure != nil {
		return handleAuxResult(failure)
	}
	options := stycli.CMDBInitOptions{
		PigstyHome: config.PigstyHome, PigBinary: pigBinary,
		ConnectionFile: pigstyCMDBConnectionFile, Reset: pigstyCMDBReset,
		RestorePath: pigstyCMDBRestore, Yes: pigstyCMDBInitYes,
	}
	if failure := stycli.ValidateCMDBInitOptions(options); failure != nil {
		return handleAuxResult(failure)
	}
	store, openFailure := pigstyCMDBOpenStore(cmd.Context(), stycli.CMDBConnectionOptions{ConnectionFile: pigstyCMDBConnectionFile})
	if openFailure != nil {
		return handleAuxResult(openFailure)
	}
	defer store.Close()
	if !options.Reset {
		return handleAuxResult(pigstyCMDBInitExec(cmd.Context(), store, options))
	}
	operation, planFailure := pigstyCMDBPlanResetExec(cmd.Context(), store, options)
	if planFailure != nil {
		return handleAuxResult(planFailure)
	}
	result := pigstyCMDBApplyResetExec(cmd.Context(), operation, options.Yes)
	return runPigstyCMDBConfirmed(
		cmd, result, "CMDB reset plan",
		"This will back up and replace only the verified pigsty_cmdb schema.",
		"pig sty cmdb init --reset",
		func() *output.Result { return pigstyCMDBApplyResetExec(cmd.Context(), operation, true) },
	)
}

func runPigstyCMDBLoad(cmd *cobra.Command, args []string) error {
	invocation := styCMDBInvocationInventory()
	options := stycli.CMDBLoadOptions{
		Revision: pigstyCMDBLoadRevision, InvocationSource: invocation,
		Prune: pigstyCMDBLoadPrune, Yes: pigstyCMDBLoadYes,
	}
	if len(args) == 1 {
		options.StaticFile = args[0]
	}
	if failure := stycli.ValidateCMDBLoadOptions(options); failure != nil {
		return handleAuxResult(failure)
	}
	if options.Revision == "" {
		selected := invocation
		if options.StaticFile != "" {
			selected = options.StaticFile
		}
		resolution, staticPath, sourceFailure := pigstyCMDBResolveLoadSourceExec(styCMDBSourceOptions(selected))
		if emitErr := styCMDBEmitNotice(cmd, resolution); emitErr != nil {
			return emitErr
		}
		if sourceFailure != nil {
			return handleAuxResult(sourceFailure)
		}
		if options.StaticFile != "" {
			options.StaticFile = staticPath
		} else {
			options.ResolvedStatic = staticPath
		}
	}
	store, openFailure := pigstyCMDBOpenStore(cmd.Context(), stycli.CMDBConnectionOptions{ConnectionFile: pigstyCMDBConnectionFile})
	if openFailure != nil {
		return handleAuxResult(openFailure)
	}
	defer store.Close()
	operation, planFailure := pigstyCMDBPlanLoadExec(cmd.Context(), store, options)
	if planFailure != nil {
		return handleAuxResult(planFailure)
	}
	result := pigstyCMDBApplyLoadExec(cmd.Context(), operation, options)
	return runPigstyCMDBConfirmed(
		cmd, result, "CMDB load plan",
		"This will apply the displayed declaration removal or historical rollback plan.",
		"pig sty cmdb load",
		func() *output.Result {
			confirmed := options
			confirmed.Yes = true
			return pigstyCMDBApplyLoadExec(cmd.Context(), operation, confirmed)
		},
	)
}

func runPigstyCMDBDump(cmd *cobra.Command, _ []string) error {
	options := stycli.CMDBDumpOptions{
		OutputPath: pigstyCMDBDumpOutput, Revision: pigstyCMDBDumpRevision,
		Merge: pigstyCMDBDumpMerge, ExpectBase: pigstyCMDBDumpExpectBase,
		InvocationSource: styCMDBInvocationInventory(),
	}
	if failure := stycli.ValidateCMDBDumpOptions(options); failure != nil {
		return handleAuxResult(failure)
	}
	store, openFailure := pigstyCMDBOpenStore(cmd.Context(), stycli.CMDBConnectionOptions{ConnectionFile: pigstyCMDBConnectionFile})
	if openFailure != nil {
		return handleAuxResult(openFailure)
	}
	defer store.Close()
	return handleAuxResult(pigstyCMDBDumpExec(cmd.Context(), store, options))
}

func runPigstyCMDBEnable(cmd *cobra.Command, _ []string) error {
	return runPigstyCMDBSourceSwitch(cmd, false)
}

func runPigstyCMDBDisable(cmd *cobra.Command, _ []string) error {
	return runPigstyCMDBSourceSwitch(cmd, true)
}

func runPigstyCMDBSourceSwitch(cmd *cobra.Command, disable bool) error {
	pigBinary, failure := styCMDBCurrentBinary()
	if failure != nil {
		return handleAuxResult(failure)
	}
	store, openFailure := pigstyCMDBOpenStore(cmd.Context(), stycli.CMDBConnectionOptions{ConnectionFile: pigstyCMDBConnectionFile})
	if openFailure != nil {
		return handleAuxResult(openFailure)
	}
	defer store.Close()
	options := stycli.CMDBSourceSwitchOptions{
		PigstyHome: config.PigstyHome, PigBinary: pigBinary,
	}
	if disable {
		options.StaticPath = pigstyCMDBDisableStatic
		return handleAuxResult(pigstyCMDBDisableExec(cmd.Context(), store, options))
	}
	return handleAuxResult(pigstyCMDBEnableExec(cmd.Context(), store, options))
}

func runPigstyCMDBCheck(cmd *cobra.Command, _ []string) error {
	pigBinary, failure := styCMDBCurrentBinary()
	if failure != nil {
		return handleAuxResult(failure)
	}
	store, openFailure := pigstyCMDBOpenStore(cmd.Context(), stycli.CMDBConnectionOptions{ConnectionFile: pigstyCMDBConnectionFile})
	if openFailure != nil {
		return handleAuxResult(openFailure)
	}
	defer store.Close()
	result := pigstyCMDBCheckExec(cmd.Context(), store, stycli.CMDBCheckOptions{
		PigstyHome: config.PigstyHome, InvocationInventory: styCMDBInvocationInventory(),
		UserHome: config.HomeDir, UserConfigPath: config.ConfigFile, PigBinary: pigBinary,
	})
	if result != nil {
		data, ok := result.Data.(*stycli.CMDBCheckData)
		if ok && data != nil {
			if emitErr := styCMDBEmitNotice(cmd, stycli.CMDBSourceResolution{
				Effective: data.EffectiveSource, Persistent: data.PersistentSource,
			}); emitErr != nil {
				return emitErr
			}
		}
	}
	return handleAuxResult(result)
}

func runPigstyCMDBInventoryAdapter(cmd *cobra.Command, args []string) error {
	body, failure := pigstyCMDBInventoryAdapterExec(cmd.Context(), stycli.CMDBInventoryAdapterOptions{Arguments: args})
	if failure != nil {
		return styCMDBAdapterFailure(cmd, failure)
	}
	if !json.Valid(body) {
		return styCMDBAdapterFailure(cmd, output.Fail(output.CodeStyCMDBOperationFailed, "CMDB inventory adapter produced invalid protocol output").
			WithData(&stycli.CMDBFailureData{Kind: stycli.CMDBFailureKind, CMDBCode: "CMDB_SNAPSHOT_INVALID"}))
	}
	written, err := cmd.OutOrStdout().Write(body)
	if err != nil || written != len(body) {
		return styCMDBAdapterFailure(cmd, output.Fail(output.CodeStyCMDBOutputFailed, "CMDB inventory adapter stdout write failed").
			WithData(&stycli.CMDBFailureData{Kind: stycli.CMDBFailureKind, CMDBCode: "CMDB_OUTPUT_FAILED"}))
	}
	return nil
}

func styCMDBAdapterFailure(cmd *cobra.Command, result *output.Result) error {
	if result == nil {
		result = output.Fail(output.CodeStyCMDBOperationFailed, "CMDB inventory adapter failed").
			WithData(&stycli.CMDBFailureData{Kind: stycli.CMDBFailureKind, CMDBCode: "CMDB_INTERNAL"})
	}
	if err := output.PrintTo(cmd.ErrOrStderr(), result); err != nil {
		return err
	}
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	return &utils.ExitCodeError{Code: result.ExitCode(), Err: fmt.Errorf("%s", result.Message), Silent: true}
}

func runPigstyCMDBConfirmed(cmd *cobra.Command, result *output.Result, planMessage, warning, action string, confirmed func() *output.Result) error {
	if result == nil || result.Code != output.CodeStyCMDBConfirmationRequired {
		return handleAuxResult(result)
	}
	if config.IsStructuredOutput() || !styCMDBTextInputIsTerminal() {
		return handleAuxResult(result)
	}
	if err := output.Print(output.OK(planMessage, result.Data)); err != nil {
		return err
	}
	if err := highRiskTextConfirm(warning, action); err != nil {
		return err
	}
	if confirmed == nil {
		return handleAuxResult(result)
	}
	return handleAuxResult(confirmed())
}

func styCMDBInvocationInventory() string {
	flag := rootCmd.PersistentFlags().Lookup("inventory")
	if flag == nil || !flag.Changed {
		return ""
	}
	return inventory
}

func styCMDBSourceOptions(invocation string) stycli.CMDBSourceResolveOptions {
	return stycli.CMDBSourceResolveOptions{
		PigstyHome: config.PigstyHome, InvocationInventory: invocation,
		UserHome: config.HomeDir, UserConfigPath: config.ConfigFile,
	}
}

func styCMDBEmitNotice(cmd *cobra.Command, resolution stycli.CMDBSourceResolution) error {
	emitter := stycli.NewCMDBNoticeEmitter(cmd.ErrOrStderr())
	if err := emitter.Emit(resolution.Effective, resolution.Persistent); err != nil {
		return handleAuxResult(output.Fail(output.CodeStyCMDBOperationFailed, "CMDB split-brain notice could not be written").
			WithData(&stycli.CMDBFailureData{Kind: stycli.CMDBFailureKind, CMDBCode: "CMDB_OUTPUT_FAILED"}))
	}
	return nil
}

func styCMDBCurrentBinary() (string, *output.Result) {
	path, err := pigstyCMDBExecutable()
	if err != nil || path == "" {
		return "", output.Fail(output.CodeStyCMDBConfigInvalid, "CMDB current Pig binary is unavailable").
			WithData(&stycli.CMDBFailureData{Kind: stycli.CMDBFailureKind, CMDBCode: "CMDB_RUNTIME_PIG_INVALID"})
	}
	return path, nil
}

func cmdbTextInputIsTerminal() bool {
	info, err := os.Stdin.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

var styCMDBTextInputIsTerminal = cmdbTextInputIsTerminal

var pigstyDashboardCmd = &cobra.Command{
	Use:         "dashboard",
	Aliases:     []string{"ds"},
	Short:       "Manage manifest-owned Grafana dashboards and datasources",
	Annotations: ancsAnn("pig sty dashboard", "query", "volatile", "safe", true, "safe", "none", "current", 100),
	Long: `Manage Grafana through the native HTTP API and a versioned repository manifest.

UID membership in the active manifest defines ownership. Folder placement does
not confer ownership, copied dashboards are left untouched, and removed UIDs
can be deleted only through tombstone-aware init --prune.`,
}

var pigstyDashboardListCmd = &cobra.Command{
	Use:         "list",
	Short:       "List managed, tombstoned, and unmanaged Grafana resources",
	Annotations: ancsAnn("pig sty dashboard list", "query", "volatile", "safe", true, "safe", "none", "current", 5000),
	Args:        cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		return handleAuxResult(pigstyDashboardListExec(styDashboardOptions()))
	},
}

var pigstyDashboardCheckCmd = &cobra.Command{
	Use:         "check",
	Short:       "Check manifest integrity and Grafana API compatibility",
	Annotations: ancsAnn("pig sty dashboard check", "query", "volatile", "safe", true, "safe", "none", "current", 5000),
	Args:        cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		return handleAuxResult(pigstyDashboardCheckExec(styDashboardOptions()))
	},
}

var pigstyDashboardDiffCmd = &cobra.Command{
	Use:         "diff",
	Short:       "Show normalized repository-versus-live Grafana drift",
	Annotations: ancsAnn("pig sty dashboard diff", "query", "volatile", "safe", true, "safe", "none", "current", 10000),
	Args:        cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		return handleAuxResult(pigstyDashboardDiffExec(styDashboardOptions()))
	},
}

var pigstyDashboardInitCmd = &cobra.Command{
	Use:         "init",
	Short:       "Idempotently upsert active manifest resources",
	Annotations: ancsAnn("pig sty dashboard init", "action", "volatile", "restricted", true, "medium", "recommended", "current", 30000),
	Args:        cobra.NoArgs,
	Long: `Idempotently upsert active manifest resources without a global clean.

--prune additionally selects only live UIDs in removed-UID tombstone lineage.
That deletion plan requires an interactive confirmation or --yes.`,
	RunE: func(_ *cobra.Command, _ []string) error {
		return runPigstyDashboardConfirmed(
			styDashboardOptions(), pigstyDashboardInitExec,
			"dashboard tombstone deletion plan",
			"This will delete only live Grafana UIDs in the manifest tombstone lineage.",
			"pig sty dashboard init --prune",
		)
	},
}

var pigstyDashboardDumpCmd = &cobra.Command{
	Use:         "dump",
	Short:       "Atomically export managed dashboards to a new directory",
	Annotations: ancsAnn("pig sty dashboard dump", "action", "volatile", "safe", false, "low", "none", "current", 30000),
	Args:        cobra.NoArgs,
	Long: `Export active manifest dashboards through private atomic staging.

Datasource credentials are never exported; dump is not a datasource backup or
restore mechanism. The destination supplied by --dir must not already exist.`,
	RunE: func(_ *cobra.Command, _ []string) error {
		return handleAuxResult(pigstyDashboardDumpExec(styDashboardOptions()))
	},
}

var pigstyDashboardCleanCmd = &cobra.Command{
	Use:         "clean",
	Short:       "Delete active manifest-owned Grafana resources",
	Annotations: ancsAnn("pig sty dashboard clean", "action", "volatile", "unsafe", true, "high", "required", "current", 30000),
	Args:        cobra.NoArgs,
	Long: `Delete only active dashboard and datasource UIDs in the manifest.

Unmanaged copies and tombstoned resources are outside this command's scope.
Placement folders are deleted only after they are empty.`,
	RunE: func(_ *cobra.Command, _ []string) error {
		return runPigstyDashboardConfirmed(
			styDashboardOptions(), pigstyDashboardCleanExec,
			"dashboard managed-resource deletion plan",
			"This will delete active manifest-owned Grafana dashboards and datasources.",
			"pig sty dashboard clean",
		)
	},
}

func styDashboardOptions() stycli.DashboardOptions {
	return stycli.DashboardOptions{
		Home: config.PigstyHome, Inventory: config.PigstyConfig,
		Manifest: pigstyDashboardManifest, Endpoint: pigstyDashboardEndpoint,
		Username: pigstyDashboardUsername, PasswordFile: pigstyDashboardPassword,
		DumpDir: pigstyDashboardDumpDir, Prune: pigstyDashboardPrune, Yes: pigstyDashboardYes,
	}
}

func runPigstyDashboardConfirmed(options stycli.DashboardOptions, execute func(stycli.DashboardOptions) *output.Result, planMessage, warning, action string) error {
	result := execute(options)
	if result == nil || result.Code != output.CodeStyDashboardConfirmationRequired || config.IsStructuredOutput() {
		return handleAuxResult(result)
	}
	if err := output.Print(output.OK(planMessage, result.Data)); err != nil {
		return err
	}
	if err := highRiskTextConfirm(warning, action); err != nil {
		return err
	}
	options.Yes = true
	return handleAuxResult(execute(options))
}

func styConfigureParams(region string) map[string]interface{} {
	return map[string]interface{}{
		"conf":            pigstyConfName,
		"ip":              pigstyConfIP,
		"version":         pigstyConfVer,
		"region":          region,
		"mirror":          pigstyConfMirror,
		"output_file":     pigstyConfOutput,
		"skip":            pigstyConfSkip,
		"proxy":           pigstyConfProxy,
		"non_interactive": pigstyConfNonInteractive,
		"port":            pigstyConfPort,
		"generate":        pigstyConfGenerate,
		"raw":             pigstyConfRaw,
	}
}

func runPigstyConfigureNative(command string, args []string, region string) error {
	if len(args) > 0 {
		return structuredParamError(
			legacyModuleSty, command, "invalid arguments",
			fmt.Sprintf("unexpected arguments: %s", strings.Join(args, " ")),
			args, styConfigureParams(region),
		)
	}
	opts := stycli.ConfigureOptions{
		PigstyHome:     config.PigstyHome,
		Mode:           pigstyConfName,
		PrimaryIP:      pigstyConfIP,
		PGVersion:      pigstyConfVer,
		Region:         region,
		SSHPort:        pigstyConfPort,
		OutputFile:     pigstyConfOutput,
		Skip:           pigstyConfSkip,
		UseProxy:       pigstyConfProxy,
		NonInteractive: pigstyConfNonInteractive,
		Generate:       pigstyConfGenerate,
	}
	return handleAuxResult(stycli.ConfigureNative(opts))
}

func addStyConfigureFlags(cmd *cobra.Command) {
	if cmd == nil {
		return
	}
	cmd.Flags().StringVarP(&pigstyConfName, "conf", "c", "", "config template name")
	cmd.Flags().StringVar(&pigstyConfIP, "ip", "", "primary ip address")
	cmd.Flags().StringVarP(&pigstyConfVer, "version", "v", "", "postgres major version")
	cmd.Flags().StringVarP(&pigstyConfRegion, "region", "r", "", "upstream repo region")
	cmd.Flags().BoolVarP(&pigstyConfMirror, "mirror", "m", false, "same as --region china")
	cmd.Flags().StringVarP(&pigstyConfOutput, "output-file", "O", "", "output config file path")
	cmd.Flags().BoolVarP(&pigstyConfSkip, "skip", "s", false, "skip ip probe")
	cmd.Flags().BoolVarP(&pigstyConfProxy, "proxy", "x", false, "write proxy env from environment")
	cmd.Flags().BoolVarP(&pigstyConfNonInteractive, "non-interactive", "n", false, "non-interactive mode")
	cmd.Flags().StringVarP(&pigstyConfPort, "port", "p", "", "SSH port (only used if set)")
	cmd.Flags().BoolVarP(&pigstyConfGenerate, "generate", "g", false, "generate random passwords")
}

var pigstyListcmd = &cobra.Command{
	Use:         "list [version]",
	Short:       "list pigsty available versions",
	Annotations: ancsAnn("pig sty list", "query", "volatile", "safe", true, "safe", "none", "current", 5000),
	Aliases:     []string{"l", "ls"},
	Args:        cobra.MaximumNArgs(1),
	Long: `List available pigsty versions

	pig sty list [-v version]

	Check https://pigsty.io/docs/releasenote for details
	`,
	Example: `
  pig sty list           # list available versions
  pig sty list 3.0.0     # list available versions since 3.0.0
  pig sty list v2        # list available versions since v2
  pig sty list all       # list all available versions
	`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPigstyReleaseList(cmd, args, true)
	},
}

var pigstyGetcmd = &cobra.Command{
	Use:         "get [version]",
	Short:       "download pigsty available versions",
	Annotations: ancsAnn("pig sty get", "action", "volatile", "safe", true, "low", "none", "current", 30000),
	Aliases:     []string{"g", "download"},
	Args:        cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPigstyReleaseGet(args)
	},
}

var pigstyReleaseCmd = &cobra.Command{
	Use:         "release",
	Short:       "List and download Pigsty releases",
	Annotations: ancsAnn("pig sty release", "query", "volatile", "safe", true, "safe", "none", "current", 100),
}

var pigstyReleaseListCmd = &cobra.Command{
	Use:         "list [version]",
	Aliases:     []string{"ls"},
	Short:       "List available Pigsty releases",
	Annotations: ancsAnn("pig sty release list", "query", "volatile", "safe", true, "safe", "none", "current", 5000),
	Args:        cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPigstyReleaseList(cmd, args, false)
	},
}

var pigstyReleaseGetCmd = &cobra.Command{
	Use:         "get [version]",
	Short:       "Download a Pigsty release",
	Annotations: ancsAnn("pig sty release get", "action", "volatile", "safe", true, "low", "none", "current", 30000),
	Args:        cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPigstyReleaseGet(args)
	},
}

func runPigstyReleaseList(cmd *cobra.Command, args []string, deprecated bool) error {
	command := "pig sty release list"
	kind := "pig.sty.release.list/v1"
	if deprecated && !config.IsStructuredOutput() {
		fmt.Fprintln(cmd.ErrOrStderr(), "Deprecated: 'pig sty list' keeps its 1.x release meaning; use 'pig sty release list'.")
	}
	if deprecated {
		// Preserve the existing 1.x structured payload while the text path emits
		// its stderr-only migration notice.
		command, kind = "pig sty list", ""
	}
	return runLegacyStructuredKind(legacyModuleSty, command, kind, args, map[string]interface{}{
		"mirror": pigstyListMirror,
	}, func() error {
		return pigstyListExec(stycli.ListOptions{Mirror: pigstyListMirror, Args: args})
	})
}

func runPigstyReleaseGet(args []string) error {
	// The compatibility spelling and canonical spelling intentionally return
	// the same result, including the historical command identifier.
	return runLegacyStructuredKind(legacyModuleSty, "pig sty get", "pig.sty.release.get/v1", args, map[string]interface{}{
		"version": pigstyVersion,
		"dir":     pigstyDownloadDir,
		"mirror":  pigstyGetMirror,
	}, func() error {
		return pigstyGetExec(stycli.DownloadOptions{
			Version: pigstyVersion, DownloadDir: pigstyDownloadDir, Mirror: pigstyGetMirror, Args: args,
		})
	})
}

var (
	pigstyGetExec                   = stycli.Download
	pigstyListExec                  = stycli.ListVersionsWithOptions
	pigstyEditExec                  = stycli.Edit
	pigstyValidateExec              = stycli.ValidateInventory
	pigstyCheckExec                 = stycli.Check
	pigstyInventoryListExec         = stycli.ListInventory
	pigstyInventoryShowExec         = stycli.ShowInventory
	pigstyDashboardListExec         = stycli.DashboardList
	pigstyDashboardCheckExec        = stycli.DashboardCheck
	pigstyDashboardDiffExec         = stycli.DashboardDiff
	pigstyDashboardInitExec         = stycli.DashboardInit
	pigstyDashboardDumpExec         = stycli.DashboardDump
	pigstyDashboardCleanExec        = stycli.DashboardClean
	pigstyCMDBOpenStore             = stycli.OpenCMDBStore
	pigstyCMDBInitExec              = stycli.CMDBInit
	pigstyCMDBPlanResetExec         = stycli.PlanCMDBReset
	pigstyCMDBApplyResetExec        = stycli.ApplyCMDBReset
	pigstyCMDBResolveLoadSourceExec = stycli.ResolveCMDBLoadSource
	pigstyCMDBPlanLoadExec          = stycli.PlanCMDBLoad
	pigstyCMDBApplyLoadExec         = stycli.ApplyCMDBLoad
	pigstyCMDBDumpExec              = stycli.CMDBDump
	pigstyCMDBEnableExec            = stycli.CMDBEnable
	pigstyCMDBDisableExec           = stycli.CMDBDisable
	pigstyCMDBCheckExec             = stycli.CMDBCheck
	pigstyCMDBInventoryAdapterExec  = stycli.CMDBInventoryAdapter
	pigstyCMDBExecutable            = os.Executable
)

var pigstyDeployCmd = &cobra.Command{
	Use:         "deploy",
	Short:       "run pigsty deploy.yml playbook",
	Annotations: ancsAnn("pig sty deploy", "action", "volatile", "unsafe", false, "high", "required", "root", 600000),
	Aliases:     []string{"d", "de", "install", "ins"},
	Args:        cobra.NoArgs,
	Long: `Deploy Pigsty using the deploy.yml playbook

This command runs the deploy.yml playbook from your Pigsty installation.
For backward compatibility, if deploy.yml doesn't exist but install.yml does,
it will use install.yml instead.

Aliases: deploy, d, de, install, ins (for backward compatibility)

WARNING: This operation makes changes to your system. Use with caution!
`,
	Example: `  pig sty deploy       # run deploy.yml (or install.yml if deploy.yml not found)
  pig sty install      # same as deploy (backward compatible)
  pig sty d            # short alias
  pig sty de           # short alias
  pig sty ins          # short alias`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModuleSty, "pig sty deploy", args, nil, func() error {
			return stycli.Deploy(stycli.DeployOptions{
				PigstyHome:   config.PigstyHome,
				PigstyConfig: config.PigstyConfig,
			})
		})
	},
}

func init() {
	pigstyInitCmd.Flags().StringVarP(&pigstyInitPath, "path", "p", "~/pigsty", "target directory")
	pigstyInitCmd.Flags().BoolVarP(&pigstyInitForce, "force", "f", false, "overwrite existing pigsty (false by default)")
	pigstyInitCmd.Flags().BoolVarP(&pigstyInitMirror, "mirror", "m", false, "prefer mirror (pigsty.cc) as primary source")
	pigstyInitCmd.Flags().StringVarP(&pigstyVersion, "version", "v", "", "pigsty version string")
	pigstyInitCmd.Flags().StringVarP(&pigstyDownloadDir, "dir", "d", "/tmp", "pigsty download directory")

	pigstyBootCmd.Flags().StringVarP(&pigstyBootRegion, "region", "r", "", "default,china,europe,...")
	pigstyBootCmd.Flags().BoolVarP(&pigstyBootMirror, "mirror", "m", false, "same as --region china")
	pigstyBootCmd.Flags().StringVarP(&pigstyBootPackage, "path", "p", "", "offline package path")
	pigstyBootCmd.Flags().BoolVarP(&pigstyBootKeep, "keep", "k", false, "keep existing repo")

	addStyConfigureFlags(pigstyConfCmd)
	pigstyConfCmd.Flags().BoolVar(&pigstyConfRaw, "raw", false, "use raw shell configure implementation")
	addStyValidateFlags(pigstyInventoryValidateCmd)
	addStyValidateFlags(pigstyValidateCmd)
	pigstyCheckCmd.Flags().StringSliceVarP(&pigstyCheckProfiles, "profile", "p", nil, "additional profile: ansible, network, or ssh")
	pigstyCheckCmd.Flags().BoolVar(&pigstyCheckSudo, "sudo", false, "with the ssh profile, also observe sudo -n true")
	pigstyCheckCmd.Flags().IntVar(&pigstyCheckPort, "port", 22, "TCP/SSH target port (no Inventory effective-var inference)")
	pigstyCheckCmd.Flags().StringVar(&pigstyCheckUser, "user", "", "explicit SSH user (defaults to the controller user)")
	pigstyDashboardCmd.PersistentFlags().StringVar(&pigstyDashboardEndpoint, "endpoint", "", "Grafana origin and optional path prefix")
	pigstyDashboardCmd.PersistentFlags().StringVar(&pigstyDashboardUsername, "username", "", "Grafana API username")
	pigstyDashboardCmd.PersistentFlags().StringVar(&pigstyDashboardPassword, "password-file", "", "owner-only file containing the Grafana API password")
	pigstyDashboardCmd.PersistentFlags().StringVar(&pigstyDashboardManifest, "manifest", "", "explicit versioned Grafana manifest path")
	pigstyDashboardInitCmd.Flags().BoolVar(&pigstyDashboardPrune, "prune", false, "delete only live UIDs in removed-UID tombstone lineage")
	pigstyDashboardInitCmd.Flags().BoolVarP(&pigstyDashboardYes, "yes", "y", false, "confirm the tombstone deletion plan")
	pigstyDashboardDumpCmd.Flags().StringVar(&pigstyDashboardDumpDir, "dir", "", "new destination directory for the atomic export")
	pigstyDashboardCleanCmd.Flags().BoolVarP(&pigstyDashboardYes, "yes", "y", false, "confirm deletion of active manifest-owned resources")
	pigstyCMDBCmd.PersistentFlags().StringVar(&pigstyCMDBConnectionFile, "connection-file", "", "owner-only PostgreSQL conninfo or URI file")
	pigstyCMDBInitCmd.Flags().BoolVar(&pigstyCMDBReset, "reset", false, "select verified native schema reset scope")
	pigstyCMDBInitCmd.Flags().StringVar(&pigstyCMDBRestore, "restore", "", "verified private native reset bundle")
	pigstyCMDBInitCmd.Flags().BoolVarP(&pigstyCMDBInitYes, "yes", "y", false, "confirm the exact reset plan")
	pigstyCMDBLoadCmd.Flags().StringVar(&pigstyCMDBLoadRevision, "revision", "", "exact historical CMDB revision token")
	pigstyCMDBLoadCmd.Flags().BoolVar(&pigstyCMDBLoadPrune, "prune", false, "select displayed declaration removals")
	pigstyCMDBLoadCmd.Flags().BoolVarP(&pigstyCMDBLoadYes, "yes", "y", false, "confirm the exact load or rollback plan")
	pigstyCMDBDumpCmd.Flags().StringVar(&pigstyCMDBDumpOutput, "output", "", "new owner-only Inventory destination")
	pigstyCMDBDumpCmd.Flags().StringVarP(&outputFormat, "format", "o", "text", "result format: text, yaml, json, json-pretty")
	pigstyCMDBDumpCmd.Flags().StringVar(&pigstyCMDBDumpRevision, "revision", "", "exact historical CMDB revision token")
	pigstyCMDBDumpCmd.Flags().BoolVar(&pigstyCMDBDumpMerge, "merge", false, "perform provenance-sensitive three-way merge")
	pigstyCMDBDumpCmd.Flags().StringVar(&pigstyCMDBDumpExpectBase, "expect-base", "", "exact expected static merge-base token")
	pigstyCMDBDisableCmd.Flags().StringVar(&pigstyCMDBDisableStatic, "static", "", "literal verified static fallback file")

	pigstyGetcmd.Flags().StringVarP(&pigstyVersion, "version", "v", "", "pigsty version string")
	pigstyGetcmd.Flags().StringVarP(&pigstyDownloadDir, "dir", "d", "/tmp", "pigsty download directory")
	pigstyGetcmd.Flags().BoolVarP(&pigstyGetMirror, "mirror", "m", false, "prefer mirror (pigsty.cc) as primary source")
	pigstyListcmd.Flags().BoolVarP(&pigstyListMirror, "mirror", "m", false, "prefer mirror (pigsty.cc) as primary source")
	pigstyReleaseGetCmd.Flags().StringVarP(&pigstyVersion, "version", "v", "", "pigsty version string")
	pigstyReleaseGetCmd.Flags().StringVarP(&pigstyDownloadDir, "dir", "d", "/tmp", "pigsty download directory")
	pigstyReleaseGetCmd.Flags().BoolVarP(&pigstyGetMirror, "mirror", "m", false, "prefer mirror (pigsty.cc) as primary source")
	pigstyReleaseListCmd.Flags().BoolVarP(&pigstyListMirror, "mirror", "m", false, "prefer mirror (pigsty.cc) as primary source")

	pigstyInventoryCmd.AddCommand(pigstyInventoryListCmd, pigstyInventoryShowCmd, pigstyInventoryEditCmd, pigstyInventoryValidateCmd)
	pigstyDashboardCmd.AddCommand(pigstyDashboardListCmd, pigstyDashboardCheckCmd, pigstyDashboardDiffCmd, pigstyDashboardInitCmd, pigstyDashboardDumpCmd, pigstyDashboardCleanCmd)
	pigstyCMDBCmd.AddCommand(pigstyCMDBInitCmd, pigstyCMDBLoadCmd, pigstyCMDBDumpCmd, pigstyCMDBEnableCmd, pigstyCMDBDisableCmd, pigstyCMDBCheckCmd, pigstyCMDBInventoryAdapterCmd)
	pigstyReleaseCmd.AddCommand(pigstyReleaseListCmd, pigstyReleaseGetCmd)
	styCmd.AddCommand(pigstyInitCmd, pigstyBootCmd, pigstyConfCmd, pigstyInventoryCmd, pigstyEditCmd, pigstyValidateCmd, pigstyCheckCmd, pigstyDashboardCmd, pigstyCMDBCmd, pigstyDeployCmd, pigstyReleaseCmd, pigstyListcmd, pigstyGetcmd)
}
