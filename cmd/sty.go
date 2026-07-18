package cmd

import (
	"fmt"
	"os"
	"pig/cli/ext"
	stycli "pig/cli/sty"
	"pig/internal/config"
	"pig/internal/output"
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

	pigstyConfName            string
	pigstyConfIP              string
	pigstyConfVer             string
	pigstyConfRegion          string
	pigstyConfOutput          string
	pigstyConfSkip            bool
	pigstyConfProxy           bool
	pigstyConfNonInteractive  bool
	pigstyConfPort            string
	pigstyConfGenerate        bool
	pigstyConfRaw             bool
	pigstyConfMirror          bool
	pigstyValidateStrict      bool
	pigstyValidateAnsible     bool
	pigstyCheckProfiles       []string
	pigstyCheckSudo           bool
	pigstyCheckPort           int
	pigstyCheckUser           string
	pigstyGrafanaEndpoint     string
	pigstyGrafanaUsername     string
	pigstyGrafanaPassword     string
	pigstyGrafanaPasswordFile string
	pigstyGrafanaDumpDir      string
	pigstyGrafanaAll          bool
	pigstyGrafanaYes          bool
	pigstyGrafanaDryRun       bool
	pigstyCMDBDatabase        string
	pigstyCMDBInitYes         bool

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
  pig sty grafana info|list|boot          # inspect or bootstrap Grafana
  pig sty grafana load|init|dump|clean    # manage Grafana dashboards
  pig sty grafana lang|style              # set language or visual style
  pig sty cmdb init|load|dump             # compatibility: existing Pigsty CMDB
  pig sty cmdb enable|disable             # compatibility: switch Inventory source
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

var pigstyCMDBCmd = &cobra.Command{
	Use:         "cmdb",
	Short:       "Compatibility entry for pig inventory cmdb",
	Annotations: inventoryCMDBAnnotations("pig sty cmdb", "query", "volatile", "restricted", true, "low", "none", 100),
	Long: `Compatibility entry for Pigsty's existing PostgreSQL CMDB.

The data primitives are init, load, and dump. Enable and disable only switch
the [defaults] inventory directive between inventory.sh and pigsty.yml.
New automation should use pig inventory cmdb.`,
}

var pigstyCMDBInitCmd = &cobra.Command{
	Use:         "init",
	Short:       "Apply the version-matched Pigsty files/cmdb.sql",
	Annotations: mergeAnn(inventoryCMDBAnnotations("pig sty cmdb init", "action", "volatile", "unsafe", false, "critical", "required", 30000), map[string]string{"plan": "reports replacement scope before confirmation"}),
	Args:        cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		options := styCMDBOptions("")
		options.Yes = pigstyCMDBInitYes
		return runInventoryCMDBInit(cmd, options, "pig sty cmdb init --yes")
	},
}

var pigstyCMDBLoadCmd = &cobra.Command{
	Use:         "load [file]",
	Short:       "Replace existing CMDB declaration rows from static Inventory",
	Annotations: inventoryCMDBAnnotations("pig sty cmdb load", "action", "volatile", "unsafe", false, "high", "recommended", 30000),
	Args:        cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := config.PigstyConfig
		if len(args) == 1 {
			path = args[0]
		}
		return handleAuxResult(inventoryCMDBLoadExec(cmd.Context(), styCMDBOptions(path)))
	},
}

var pigstyCMDBDumpCmd = &cobra.Command{
	Use:         "dump [file]",
	Short:       "Generate canonical YAML from pigsty.raw_config",
	Annotations: inventoryCMDBAnnotations("pig sty cmdb dump", "query", "stable", "restricted", true, "low", "none", 30000),
	Args:        cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := ""
		if len(args) == 1 {
			path = args[0]
		}
		return runInventoryCMDBDump(cmd, styCMDBOptions(path))
	},
}

var pigstyCMDBEnableCmd = &cobra.Command{
	Use:         "enable",
	Short:       "Activate inventory.sh backed by pigsty.inventory",
	Annotations: inventoryCMDBAnnotations("pig sty cmdb enable", "action", "volatile", "unsafe", true, "medium", "none", 10000),
	Args:        cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return handleAuxResult(inventoryCMDBEnableExec(cmd.Context(), styCMDBOptions("")))
	},
}

var pigstyCMDBDisableCmd = &cobra.Command{
	Use:         "disable",
	Short:       "Restore the ansible.cfg Inventory source to pigsty.yml",
	Annotations: inventoryCMDBAnnotations("pig sty cmdb disable", "action", "volatile", "unsafe", true, "medium", "none", 10000),
	Args:        cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		return handleAuxResult(inventoryCMDBDisableExec(styCMDBOptions("")))
	},
}

func styCMDBOptions(path string) stycli.InventoryCMDBOptions {
	return stycli.InventoryCMDBOptions{
		Home: config.PigstyHome, Database: pigstyCMDBDatabase, Path: path,
		OSCode: config.OSCode, OSArch: config.OSArch,
	}
}
func cmdbTextInputIsTerminal() bool {
	info, err := os.Stdin.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

var styCMDBTextInputIsTerminal = cmdbTextInputIsTerminal

var pigstyGrafanaCmd = &cobra.Command{
	Use:         "grafana",
	Aliases:     []string{"gf"},
	Short:       "Manage Grafana dashboards by local JSON path",
	Annotations: ancsAnn("pig sty grafana", "query", "volatile", "safe", true, "safe", "none", "current", 100),
	Long: `Manage Grafana dashboards through the native HTTP API.

PATH may select the grafana root, one direct folder, or one dashboard JSON.
Without PATH, Pig resolves <PIGSTY_HOME>/files/grafana and never falls back to
the current directory.`,
}

var pigstyGrafanaListCmd = &cobra.Command{
	Use:         "list",
	Short:       "List all dashboards in the active Grafana organization",
	Annotations: ancsAnn("pig sty grafana list", "query", "volatile", "safe", true, "safe", "none", "current", 10000),
	Args:        cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return handleAuxResult(pigstyGrafanaListExec(styGrafanaCommandOptions(cmd, nil)))
	},
}

var pigstyGrafanaInfoCmd = &cobra.Command{
	Use:         "info",
	Short:       "Check Grafana health, authentication, and basic information",
	Annotations: ancsAnn("pig sty grafana info", "query", "volatile", "safe", true, "safe", "none", "current", 10000),
	Args:        cobra.NoArgs,
	Long: `Check the public Grafana health endpoint before authentication.

Without a configured password, info returns a successful health-only result.
With credentials, it also reports the active organization, dashboard/folder
counts, and organization/current-user preferences.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return handleAuxResult(pigstyGrafanaInfoExec(styGrafanaCommandOptions(cmd, nil)))
	},
}

var pigstyGrafanaBootCmd = &cobra.Command{
	Use:         "boot",
	Short:       "Bootstrap Grafana around the existing pigsty dashboard",
	Annotations: ancsAnn("pig sty grafana boot", "action", "volatile", "restricted", true, "medium", "recommended", "current", 30000),
	Args:        cobra.NoArgs,
	Long: `Verify dashboard UID pigsty, rename the current organization to Pigsty,
star that dashboard, and set it as the organization/current-user home.

Boot does not load local JSON and does not change language, theme, timezone, or
other preferences.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return handleAuxResult(pigstyGrafanaBootExec(styGrafanaCommandOptions(cmd, nil)))
	},
}

var pigstyGrafanaLoadCmd = &cobra.Command{
	Use:         "load [path]",
	Short:       "Load dashboards selected by a local path",
	Annotations: ancsAnn("pig sty grafana load", "action", "volatile", "restricted", true, "medium", "recommended", "current", 30000),
	Args:        cobra.MaximumNArgs(1),
	Long: `Validate and load one dashboard JSON, one direct folder, or the complete
dashboard corpus. Load only upserts folders and dashboards: it never bootstraps
the organization, changes preferences, or prunes live resources.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleAuxResult(pigstyGrafanaLoadExec(styGrafanaCommandOptions(cmd, args)))
	},
}

var pigstyGrafanaInitCmd = &cobra.Command{
	Use:         "init [path]",
	Short:       "Load the complete dashboard corpus, then bootstrap Grafana",
	Annotations: ancsAnn("pig sty grafana init", "action", "volatile", "restricted", true, "medium", "recommended", "current", 30000),
	Args:        cobra.MaximumNArgs(1),
	Long: `Validate and load a complete grafana root, then bootstrap the instance.

Init is exactly load followed by boot. It refuses folder and single-dashboard
paths; use load for those. Boot is skipped unless every load operation succeeds.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleAuxResult(pigstyGrafanaInitExec(styGrafanaCommandOptions(cmd, args)))
	},
}

var pigstyGrafanaDumpCmd = &cobra.Command{
	Use:         "dump [path]",
	Short:       "Export remote dashboards into a local path",
	Annotations: ancsAnn("pig sty grafana dump", "action", "volatile", "safe", false, "low", "none", "current", 30000),
	Args:        cobra.MaximumNArgs(1),
	Long: `Export one dashboard, one Grafana folder, or the complete instance.

Dump writes canonical compact JSON atomically and never deletes local files.
Use -d/--dir to select an arbitrary full-corpus output directory instead of
<PIGSTY_HOME>/files/grafana. --dir and PATH are mutually exclusive. Remote
folderUid values map directly to first-level local directories.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleAuxResult(pigstyGrafanaDumpExec(styGrafanaCommandOptions(cmd, args)))
	},
}

var pigstyGrafanaCleanCmd = &cobra.Command{
	Use:         "clean [path]",
	Short:       "Delete dashboards selected by a local path",
	Annotations: ancsAnn("pig sty grafana clean", "action", "volatile", "unsafe", true, "high", "required", "current", 30000),
	Args:        cobra.MaximumNArgs(1),
	Long: `Delete only dashboard UIDs declared by the selected local path.

Folders are removed only after they are verified empty. --all is mutually
exclusive with PATH and is the only mode that removes every live dashboard and
folder. Clean prints a plan and requires interactive confirmation or --yes.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPigstyGrafanaClean(styGrafanaCommandOptions(cmd, args))
	},
}

var pigstyGrafanaLangCmd = &cobra.Command{
	Use:         "lang <language-code>",
	Short:       "Set the Grafana organization and current-user language",
	Annotations: ancsAnn("pig sty grafana lang", "action", "volatile", "restricted", true, "low", "none", "current", 10000),
	Args:        cobra.ExactArgs(1),
	Example: `  pig sty gf lang en
  pig sty gf lang zh-cn
  pig sty gf lang zh-Hant`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleAuxResult(pigstyGrafanaLangExec(styGrafanaCommandOptions(cmd, nil), args[0]))
	},
}

var pigstyGrafanaStyleCmd = &cobra.Command{
	Use:         "style <dark|light|auto>",
	Short:       "Set the Grafana organization and current-user visual style",
	Annotations: ancsAnn("pig sty grafana style", "action", "volatile", "restricted", true, "low", "none", "current", 10000),
	Args:        cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleAuxResult(pigstyGrafanaStyleExec(styGrafanaCommandOptions(cmd, nil), args[0]))
	},
}

func styGrafanaOptions(args []string) stycli.GrafanaOptions {
	path := ""
	if len(args) > 0 {
		path = args[0]
	}
	return stycli.GrafanaOptions{
		Home: config.PigstyHome, Inventory: config.PigstyConfig, Path: path, DumpDir: pigstyGrafanaDumpDir,
		Endpoint: pigstyGrafanaEndpoint, Username: pigstyGrafanaUsername,
		Password: pigstyGrafanaPassword, PasswordFile: pigstyGrafanaPasswordFile, All: pigstyGrafanaAll,
		Yes: pigstyGrafanaYes, DryRun: pigstyGrafanaDryRun,
	}
}

func styGrafanaCommandOptions(cmd *cobra.Command, args []string) stycli.GrafanaOptions {
	options := styGrafanaOptions(args)
	if cmd != nil {
		options.Context = cmd.Context()
	}
	return options
}

func runPigstyGrafanaClean(options stycli.GrafanaOptions) error {
	result := pigstyGrafanaCleanExec(options)
	if result == nil || result.Code != output.CodeStyGrafanaConfirmationRequired || config.IsStructuredOutput() {
		return handleAuxResult(result)
	}
	if err := output.Print(output.OK("Grafana deletion plan", result.Data)); err != nil {
		return err
	}
	warning := "This will delete only Grafana dashboard UIDs declared by the selected local path."
	action := "the displayed Grafana clean plan"
	if options.All {
		warning = "This will delete every Grafana dashboard and folder in the selected organization."
		action = "the displayed all-instance Grafana clean plan"
	}
	if err := highRiskTextConfirm(warning, action); err != nil {
		return err
	}
	if data, ok := result.Data.(*stycli.GrafanaActionData); ok {
		options.PlanDigest = data.PlanDigest
	}
	options.Yes = true
	return handleAuxResult(pigstyGrafanaCleanExec(options))
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
	pigstyGetExec           = stycli.Download
	pigstyListExec          = stycli.ListVersionsWithOptions
	pigstyEditExec          = stycli.Edit
	pigstyValidateExec      = stycli.ValidateInventory
	pigstyCheckExec         = stycli.Check
	pigstyInventoryListExec = stycli.ListInventory
	pigstyInventoryShowExec = stycli.ShowInventory
	pigstyGrafanaInfoExec   = stycli.GrafanaInfo
	pigstyGrafanaListExec   = stycli.GrafanaList
	pigstyGrafanaBootExec   = stycli.GrafanaBoot
	pigstyGrafanaLoadExec   = stycli.GrafanaLoad
	pigstyGrafanaInitExec   = stycli.GrafanaInit
	pigstyGrafanaDumpExec   = stycli.GrafanaDump
	pigstyGrafanaCleanExec  = stycli.GrafanaClean
	pigstyGrafanaLangExec   = stycli.GrafanaLang
	pigstyGrafanaStyleExec  = stycli.GrafanaStyle
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
	pigstyGrafanaCmd.PersistentFlags().StringVar(&pigstyGrafanaEndpoint, "endpoint", "", "Grafana origin and optional path prefix (default http://i.pigsty/ui)")
	pigstyGrafanaCmd.PersistentFlags().StringVar(&pigstyGrafanaUsername, "username", "", "Grafana API username")
	pigstyGrafanaCmd.PersistentFlags().StringVar(&pigstyGrafanaPassword, "password", "", "Grafana API password (insecure: visible in process arguments and shell history)")
	pigstyGrafanaCmd.PersistentFlags().StringVar(&pigstyGrafanaPasswordFile, "password-file", "", "owner-only file containing the Grafana API password")
	pigstyGrafanaCmd.MarkFlagsMutuallyExclusive("password", "password-file")
	pigstyGrafanaDumpCmd.Flags().StringVarP(&pigstyGrafanaDumpDir, "dir", "d", "", "dump the complete dashboard corpus into this directory")
	pigstyGrafanaCleanCmd.Flags().BoolVar(&pigstyGrafanaAll, "all", false, "delete every live dashboard and folder; mutually exclusive with PATH")
	pigstyGrafanaCleanCmd.Flags().BoolVarP(&pigstyGrafanaYes, "yes", "y", false, "confirm the displayed deletion scope")
	pigstyGrafanaCleanCmd.Flags().BoolVar(&pigstyGrafanaDryRun, "dry-run", false, "print the deletion plan without applying it")
	addCMDBDatabaseFlags(&pigstyCMDBDatabase, pigstyCMDBInitCmd, pigstyCMDBLoadCmd, pigstyCMDBDumpCmd, pigstyCMDBEnableCmd)
	pigstyCMDBInitCmd.Flags().BoolVarP(&pigstyCMDBInitYes, "yes", "y", false, "confirm replacement of existing pigsty and pglog schemas")
	pigstyGetcmd.Flags().StringVarP(&pigstyVersion, "version", "v", "", "pigsty version string")
	pigstyGetcmd.Flags().StringVarP(&pigstyDownloadDir, "dir", "d", "/tmp", "pigsty download directory")
	pigstyGetcmd.Flags().BoolVarP(&pigstyGetMirror, "mirror", "m", false, "prefer mirror (pigsty.cc) as primary source")
	pigstyListcmd.Flags().BoolVarP(&pigstyListMirror, "mirror", "m", false, "prefer mirror (pigsty.cc) as primary source")
	pigstyReleaseGetCmd.Flags().StringVarP(&pigstyVersion, "version", "v", "", "pigsty version string")
	pigstyReleaseGetCmd.Flags().StringVarP(&pigstyDownloadDir, "dir", "d", "/tmp", "pigsty download directory")
	pigstyReleaseGetCmd.Flags().BoolVarP(&pigstyGetMirror, "mirror", "m", false, "prefer mirror (pigsty.cc) as primary source")
	pigstyReleaseListCmd.Flags().BoolVarP(&pigstyListMirror, "mirror", "m", false, "prefer mirror (pigsty.cc) as primary source")

	pigstyInventoryCmd.AddCommand(pigstyInventoryListCmd, pigstyInventoryShowCmd, pigstyInventoryEditCmd, pigstyInventoryValidateCmd)
	pigstyGrafanaCmd.AddCommand(
		pigstyGrafanaInfoCmd, pigstyGrafanaListCmd, pigstyGrafanaBootCmd,
		pigstyGrafanaLoadCmd, pigstyGrafanaInitCmd, pigstyGrafanaDumpCmd, pigstyGrafanaCleanCmd,
		pigstyGrafanaLangCmd, pigstyGrafanaStyleCmd,
	)
	pigstyCMDBCmd.AddCommand(pigstyCMDBInitCmd, pigstyCMDBLoadCmd, pigstyCMDBDumpCmd, pigstyCMDBEnableCmd, pigstyCMDBDisableCmd)
	pigstyReleaseCmd.AddCommand(pigstyReleaseListCmd, pigstyReleaseGetCmd)
	styCmd.AddCommand(pigstyInitCmd, pigstyBootCmd, pigstyConfCmd, pigstyInventoryCmd, pigstyEditCmd, pigstyValidateCmd, pigstyCheckCmd, pigstyGrafanaCmd, pigstyCMDBCmd, pigstyDeployCmd, pigstyReleaseCmd, pigstyListcmd, pigstyGetcmd)
}
