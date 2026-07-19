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
	pigstyInitEULA   bool

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
	pigstyGrafanaEndpoint     string
	pigstyGrafanaUsername     string
	pigstyGrafanaPassword     string
	pigstyGrafanaPasswordFile string
	pigstyGrafanaDumpDir      string
	pigstyGrafanaAll          bool
	pigstyGrafanaYes          bool
	pigstyGrafanaDryRun       bool

	pigstyDownloadDir string
	pigstyVersion     string
	pigstyGetMirror   bool
	pigstyListMirror  bool
)

var styCmd = &cobra.Command{
	Use:         "sty",
	Short:       "Manage Pigsty installation and controller services",
	Annotations: ancsAnn("pig sty", "query", "stable", "safe", true, "safe", "none", "current", 100),
	Aliases:     []string{"s", "pigsty"},
	GroupID:     "pigsty",
	Args:        cobra.NoArgs,
	RunE:        showCommandHelp,
	Long: `Manage the local Pigsty installation, releases, configuration, deployment,
and controller-side Grafana dashboards.

  pig sty init                         # install Pigsty (~/pigsty by default)
  pig sty boot                         # install Ansible and prepare offline pkg
  pig sty conf                         # generate pigsty.yml through Inventory APIs
  pig sty deploy                       # immediately run the deployment playbook
  pig sty grafana [info|list|boot|init|load|dump|clean|lang|style]
  pig sty release list                 # list available Pigsty releases
  pig sty release get [version]        # download a Pigsty source tarball
  pig sty list|get                     # deprecated release delegates
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
  [--raw]                 # use legacy shell workflow; -g passwords stay visible

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
  -m | --mirror      : prefer mirror (pigsty.cc) as primary source
       --accept-eula : accept the Pigsty Pro EULA for non-interactive init
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
		return runLegacyStructuredKind(legacyModuleSty, "pig sty init", "pig.sty.init/v1", args, map[string]interface{}{
			"path":        pigstyInitPath,
			"force":       pigstyInitForce,
			"mirror":      pigstyInitMirror,
			"accept_eula": pigstyInitEULA,
			"version":     pigstyVersion,
			"dir":         pigstyDownloadDir,
		}, func() error {
			return pigstyInitExec(stycli.InitOptions{
				TargetPath:  pigstyInitPath,
				Force:       pigstyInitForce,
				Mirror:      pigstyInitMirror,
				AcceptEULA:  pigstyInitEULA,
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
	Annotations: ancsAnn("pig sty boot", "action", "volatile", "unsafe", true, "medium", "none", "root", 60000),
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
		return runLegacyStructuredKind(legacyModuleSty, "pig sty boot", "pig.sty.boot/v1", args, map[string]interface{}{
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
	Annotations: ancsAnn("pig sty conf", "action", "volatile", "unsafe", false, "medium", "recommended", "root", 10000),
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
			return runLegacyStructuredKind(legacyModuleSty, command, "pig.sty.configure.raw/v1", args, styConfigureParams(confRegion), func() error {
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

var pigstyGrafanaCmd = &cobra.Command{
	Use:         "grafana",
	Aliases:     []string{"gf"},
	Short:       "Manage Grafana dashboards by local JSON path",
	Annotations: ancsAnn("pig sty grafana", "query", "volatile", "safe", true, "safe", "none", "current", 100),
	Args:        cobra.NoArgs,
	RunE:        showCommandHelp,
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
	Annotations: ancsAnn("pig sty grafana dump", "action", "volatile", "restricted", false, "low", "none", "current", 30000),
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
		"conf":                  pigstyConfName,
		"ip":                    pigstyConfIP,
		"version":               pigstyConfVer,
		"region":                region,
		"mirror":                pigstyConfMirror,
		"output_file":           pigstyConfOutput,
		"skip":                  pigstyConfSkip,
		"proxy":                 pigstyConfProxy,
		"non_interactive":       pigstyConfNonInteractive,
		"port":                  pigstyConfPort,
		"generate":              pigstyConfGenerate,
		"raw":                   pigstyConfRaw,
		"secret_bearing_output": pigstyConfRaw && pigstyConfGenerate,
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
	Short:       "List available Pigsty releases (compatibility spelling)",
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
	Short:       "Download a Pigsty release (compatibility spelling)",
	Annotations: ancsAnn("pig sty get", "action", "volatile", "restricted", true, "low", "none", "current", 30000),
	Aliases:     []string{"g", "download"},
	Args:        cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPigstyReleaseGet(cmd, args, true)
	},
}

var pigstyReleaseCmd = &cobra.Command{
	Use:         "release",
	Short:       "List and download Pigsty releases",
	Annotations: ancsAnn("pig sty release", "query", "volatile", "safe", true, "safe", "none", "current", 100),
	Args:        cobra.NoArgs,
	RunE:        showCommandHelp,
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
	Annotations: ancsAnn("pig sty release get", "action", "volatile", "restricted", true, "low", "none", "current", 30000),
	Args:        cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPigstyReleaseGet(cmd, args, false)
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

func runPigstyReleaseGet(cmd *cobra.Command, args []string, deprecated bool) error {
	command := "pig sty release get"
	if deprecated {
		command = "pig sty get"
		if !config.IsStructuredOutput() {
			fmt.Fprintln(cmd.ErrOrStderr(), "Deprecated: 'pig sty get' is a compatibility spelling; use 'pig sty release get'.")
		}
	}
	return runLegacyStructuredKind(legacyModuleSty, command, "pig.sty.release.get/v1", args, map[string]interface{}{
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
	pigstyGetExec          = stycli.Download
	pigstyListExec         = stycli.ListVersionsWithOptions
	pigstyGrafanaInfoExec  = stycli.GrafanaInfo
	pigstyGrafanaListExec  = stycli.GrafanaList
	pigstyGrafanaBootExec  = stycli.GrafanaBoot
	pigstyGrafanaLoadExec  = stycli.GrafanaLoad
	pigstyGrafanaInitExec  = stycli.GrafanaInit
	pigstyGrafanaDumpExec  = stycli.GrafanaDump
	pigstyGrafanaCleanExec = stycli.GrafanaClean
	pigstyGrafanaLangExec  = stycli.GrafanaLang
	pigstyGrafanaStyleExec = stycli.GrafanaStyle
	pigstyDeployExec       = stycli.Deploy
)

var pigstyDeployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy Pigsty immediately with Ansible",
	Annotations: mergeAnn(
		ancsAnn("pig sty deploy", "action", "volatile", "unsafe", false, "high", "none", "root", 600000),
		map[string]string{"consent": "invocation"},
	),
	Aliases: []string{"d", "de", "install", "ins"},
	Args:    cobra.NoArgs,
	Long: `Deploy Pigsty using the deploy.yml playbook

This command runs the deploy.yml playbook from your Pigsty installation.
For backward compatibility, if deploy.yml doesn't exist but install.yml does,
it will use install.yml instead.

Aliases: deploy, d, de, install, ins (for backward compatibility)

Invocation is explicit consent: deploy starts immediately without a Pig prompt
or --yes gate. This operation changes the target system; use Ctrl+C to interrupt
execution if the command was invoked by mistake.
`,
	Example: `  pig sty deploy       # run deploy.yml (or install.yml if deploy.yml not found)
  pig sty install      # same as deploy (backward compatible)
  pig sty d            # short alias
  pig sty de           # short alias
  pig sty ins          # short alias`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructuredKind(legacyModuleSty, "pig sty deploy", "pig.sty.deploy/v1", args, nil, func() error {
			return pigstyDeployExec(stycli.DeployOptions{
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
	pigstyInitCmd.Flags().BoolVar(&pigstyInitEULA, "accept-eula", false, "accept the Pigsty Pro EULA non-interactively")
	pigstyInitCmd.Flags().StringVarP(&pigstyVersion, "version", "v", "", "pigsty version string")
	pigstyInitCmd.Flags().StringVarP(&pigstyDownloadDir, "dir", "d", "/tmp", "pigsty download directory")

	pigstyBootCmd.Flags().StringVarP(&pigstyBootRegion, "region", "r", "", "default,china,europe,...")
	pigstyBootCmd.Flags().BoolVarP(&pigstyBootMirror, "mirror", "m", false, "same as --region china")
	pigstyBootCmd.Flags().StringVarP(&pigstyBootPackage, "path", "p", "", "offline package path")
	pigstyBootCmd.Flags().BoolVarP(&pigstyBootKeep, "keep", "k", false, "keep existing repo")
	pigstyBootCmd.MarkFlagsMutuallyExclusive("mirror", "region")

	addStyConfigureFlags(pigstyConfCmd)
	pigstyConfCmd.MarkFlagsMutuallyExclusive("mirror", "region")
	pigstyConfCmd.Flags().BoolVar(&pigstyConfRaw, "raw", false, "use legacy shell configure; generated passwords remain visible")
	pigstyGrafanaCmd.PersistentFlags().StringVar(&pigstyGrafanaEndpoint, "endpoint", "", "Grafana origin and optional path prefix (default http://i.pigsty/ui)")
	pigstyGrafanaCmd.PersistentFlags().StringVar(&pigstyGrafanaUsername, "username", "", "Grafana API username")
	pigstyGrafanaCmd.PersistentFlags().StringVar(&pigstyGrafanaPassword, "password", "", "Grafana API password (insecure: visible in process arguments and shell history)")
	pigstyGrafanaCmd.PersistentFlags().StringVar(&pigstyGrafanaPasswordFile, "password-file", "", "owner-only file containing the Grafana API password")
	pigstyGrafanaCmd.MarkFlagsMutuallyExclusive("password", "password-file")
	pigstyGrafanaDumpCmd.Flags().StringVarP(&pigstyGrafanaDumpDir, "dir", "d", "", "dump the complete dashboard corpus into this directory")
	pigstyGrafanaCleanCmd.Flags().BoolVar(&pigstyGrafanaAll, "all", false, "delete every live dashboard and folder; mutually exclusive with PATH")
	pigstyGrafanaCleanCmd.Flags().BoolVarP(&pigstyGrafanaYes, "yes", "y", false, "confirm the displayed deletion scope")
	pigstyGrafanaCleanCmd.Flags().BoolVar(&pigstyGrafanaDryRun, "dry-run", false, "print the deletion plan without applying it")
	pigstyGetcmd.Flags().StringVarP(&pigstyVersion, "version", "v", "", "pigsty version string")
	pigstyGetcmd.Flags().StringVarP(&pigstyDownloadDir, "dir", "d", "/tmp", "pigsty download directory")
	pigstyGetcmd.Flags().BoolVarP(&pigstyGetMirror, "mirror", "m", false, "prefer mirror (pigsty.cc) as primary source")
	pigstyListcmd.Flags().BoolVarP(&pigstyListMirror, "mirror", "m", false, "prefer mirror (pigsty.cc) as primary source")
	pigstyReleaseGetCmd.Flags().StringVarP(&pigstyVersion, "version", "v", "", "pigsty version string")
	pigstyReleaseGetCmd.Flags().StringVarP(&pigstyDownloadDir, "dir", "d", "/tmp", "pigsty download directory")
	pigstyReleaseGetCmd.Flags().BoolVarP(&pigstyGetMirror, "mirror", "m", false, "prefer mirror (pigsty.cc) as primary source")
	pigstyReleaseListCmd.Flags().BoolVarP(&pigstyListMirror, "mirror", "m", false, "prefer mirror (pigsty.cc) as primary source")

	pigstyGrafanaCmd.AddCommand(
		pigstyGrafanaInfoCmd, pigstyGrafanaListCmd, pigstyGrafanaBootCmd,
		pigstyGrafanaLoadCmd, pigstyGrafanaInitCmd, pigstyGrafanaDumpCmd, pigstyGrafanaCleanCmd,
		pigstyGrafanaLangCmd, pigstyGrafanaStyleCmd,
	)
	pigstyReleaseCmd.AddCommand(pigstyReleaseListCmd, pigstyReleaseGetCmd)
	styCmd.AddCommand(pigstyInitCmd, pigstyBootCmd, pigstyConfCmd, pigstyGrafanaCmd, pigstyDeployCmd, pigstyReleaseCmd, pigstyListcmd, pigstyGetcmd)
}
