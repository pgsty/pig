package cmd

import (
	"fmt"
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
	pigstyInventoryListExec = stycli.ListInventory
	pigstyInventoryShowExec = stycli.ShowInventory
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

	pigstyGetcmd.Flags().StringVarP(&pigstyVersion, "version", "v", "", "pigsty version string")
	pigstyGetcmd.Flags().StringVarP(&pigstyDownloadDir, "dir", "d", "/tmp", "pigsty download directory")
	pigstyGetcmd.Flags().BoolVarP(&pigstyGetMirror, "mirror", "m", false, "prefer mirror (pigsty.cc) as primary source")
	pigstyListcmd.Flags().BoolVarP(&pigstyListMirror, "mirror", "m", false, "prefer mirror (pigsty.cc) as primary source")
	pigstyReleaseGetCmd.Flags().StringVarP(&pigstyVersion, "version", "v", "", "pigsty version string")
	pigstyReleaseGetCmd.Flags().StringVarP(&pigstyDownloadDir, "dir", "d", "/tmp", "pigsty download directory")
	pigstyReleaseGetCmd.Flags().BoolVarP(&pigstyGetMirror, "mirror", "m", false, "prefer mirror (pigsty.cc) as primary source")
	pigstyReleaseListCmd.Flags().BoolVarP(&pigstyListMirror, "mirror", "m", false, "prefer mirror (pigsty.cc) as primary source")

	pigstyInventoryCmd.AddCommand(pigstyInventoryListCmd, pigstyInventoryShowCmd, pigstyInventoryEditCmd, pigstyInventoryValidateCmd)
	pigstyReleaseCmd.AddCommand(pigstyReleaseListCmd, pigstyReleaseGetCmd)
	styCmd.AddCommand(pigstyInitCmd, pigstyBootCmd, pigstyConfCmd, pigstyInventoryCmd, pigstyEditCmd, pigstyValidateCmd, pigstyDeployCmd, pigstyReleaseCmd, pigstyListcmd, pigstyGetcmd)
}
