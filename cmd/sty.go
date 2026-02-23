package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"pig/cli/get"
	"pig/cli/sty"
	"pig/internal/config"
	"pig/internal/utils"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	pigstyInitPath  string
	pigstyInitForce bool

	pigstyBootRegion  string
	pigstyBootPackage string
	pigstyBootKeep    bool

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

	pigstyDownloadDir string
	pigstyVersion     string
)

const styConfigureLong = `Configure pigsty with native workflow (default) or raw shell workflow

pig sty conf (aliases: c, configure)
  [-c|--conf <name>]      # config template: [meta|rich|slim|full|supabase|...]
  [--ip <ip>]             # primary IP address (skip with -s)
  [-v|--version <pgver>]  # postgres major version: [18|17|16|15|14|13]
  [-r|--region <region>]  # upstream repo region: [default|china|europe]
  [-O|--output-file <file>]    # output config file path (default: pigsty.yml)
  [-s|--skip]             # skip IP address probing
  [-p|--port <port>]      # specify SSH port (for ssh accessibility check)
  [-x|--proxy]            # write proxy env from environment
  [-n|--non-interactive]  # non-interactive mode

  [-g|--generate]         # generate random default passwords (RECOMMENDED!)
  [--raw]                 # use raw legacy shell configure workflow

Check https://pigsty.io/docs/setup/install/#configure for details
`

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
  pig sty conf -x                    # write proxy env from environment to config
  pig sty conf -c full -g -O ha.yml  # full HA template with random passwords to ha.yml
`

// styCmd represents the pigsty management command
var styCmd = &cobra.Command{
	Use:         "sty",
	Short:       "Manage Pigsty Installation",
	Annotations: ancsAnn("pig sty", "query", "stable", "safe", true, "safe", "none", "current", 100),
	Aliases:     []string{"s", "pigsty"},
	GroupID:     "pigsty",
	Long: `pig sty -Init (Download), Bootstrap, Configure, and Deploy Pigsty

  pig sty init    [-pfvd]         # install pigsty (~/pigsty by default)
  pig sty boot    [-rpk]          # install ansible and prepare offline pkg
  pig sty conf    [-cvrsoxnpg --raw] # configure pigsty and generate config
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

// pigstyInitCmd will extract and setup ~/pigsty
var pigstyInitCmd = &cobra.Command{
	Use:         "init",
	Short:       "Install Pigsty",
	Annotations: ancsAnn("pig sty init", "action", "volatile", "unsafe", false, "medium", "recommended", "root", 30000),
	Aliases:     []string{"i"},
	Long: `
pig sty init
  -p | --path    : where to install, ~/pigsty by default
  -f | --force   : force overwrite existing pigsty dir
  -v | --version : pigsty version, latest by default
  -d | --dir     : download directory, /tmp by default

`,
	Example: `
  pig sty init                   # install to ~/pigsty with the latest version
  pig sty init -f                # install and OVERWRITE existing pigsty dir
  pig sty init -p /tmp/pigsty    # install to another location /tmp/pigsty
  pig sty init -v 3.4            # get & install specific version v3.4.1
  pig sty init 3                 # get & install specific version v3 latest
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModuleSty, "pig sty init", args, map[string]interface{}{
			"path":    pigstyInitPath,
			"force":   pigstyInitForce,
			"version": pigstyVersion,
			"dir":     pigstyDownloadDir,
		}, func() error {
			// Handle version from args if provided
			if len(args) > 0 && pigstyVersion == "" {
				pigstyVersion = args[0]
			}

			// Default to latest if no version specified
			if pigstyVersion == "" {
				pigstyVersion = "latest"
			}

			// Ensure version has 'v' prefix if it starts with a number
			if pigstyVersion != "latest" && len(pigstyVersion) > 0 {
				if pigstyVersion[0] >= '0' && pigstyVersion[0] <= '9' {
					pigstyVersion = "v" + pigstyVersion
				}
			}

			// if version is explicit given, always download & install from remote
			get.NetworkCondition()
			var ver *get.VersionInfo
			if get.AllVersions == nil {
				logrus.Warnf("Fail to get pigsty version list, use the built-in version %s", config.PigstyVersion)
				// Use built-in version as fallback
				if pigstyVersion == "latest" {
					pigstyVersion = config.PigstyVersion
				}
				// Ensure fallback version has v prefix if it starts with a number
				if len(pigstyVersion) > 0 && pigstyVersion[0] >= '0' && pigstyVersion[0] <= '9' {
					pigstyVersion = "v" + pigstyVersion
				}
				// Create a fallback VersionInfo for built-in version
				ver = &get.VersionInfo{
					Version:     pigstyVersion,
					DownloadURL: fmt.Sprintf("%s/src/pigsty-%s.tgz", config.RepoPigstyIO, pigstyVersion),
				}
			} else {
				pigstyVersion = get.CompleteVersion(pigstyVersion)
				ver = get.IsValidVersion(pigstyVersion)
				if ver == nil {
					logrus.Errorf("invalid pigsty version: %v", pigstyVersion)
					return fmt.Errorf("invalid pigsty version: %s", pigstyVersion)
				}
			}

			// download according to the version
			logrus.Infof("Get pigsty src %s from %s to %s", ver.Version, ver.DownloadURL, pigstyDownloadDir)
			err := get.DownloadSrc(pigstyVersion, pigstyDownloadDir)
			if err != nil {
				logrus.Errorf("failed to download pigsty src %s: %v", pigstyVersion, err)
				return fmt.Errorf("failed to download pigsty src %s: %w", pigstyVersion, err)
			}

			// read, then extract & install it
			srcTarball, err := sty.LoadPigstySrc(filepath.Join(pigstyDownloadDir, fmt.Sprintf("pigsty-%s.tgz", pigstyVersion)))
			if err != nil {
				logrus.Errorf("failed to load pigsty src %s: %v", pigstyVersion, err)
				return fmt.Errorf("failed to load pigsty src %s: %w", pigstyVersion, err)
			}
			err = sty.InstallPigsty(srcTarball, pigstyInitPath, pigstyInitForce)
			if err != nil {
				logrus.Errorf("failed to install pigsty src %s: %v", pigstyVersion, err)
				return err
			}
			logrus.Infof("proceed with pig boot & pig conf")
			return nil
		})
	},
}

var pigstyBootCmd = &cobra.Command{
	Use:         "boot",
	Short:       "Bootstrap Pigsty",
	Annotations: ancsAnn("pig sty boot", "action", "volatile", "unsafe", true, "low", "none", "root", 60000),
	Aliases:     []string{"b", "bootstrap"},
	Long: `Bootstrap pigsty with ./bootstrap script

pig sty boot
  [-r|--region <region]   [default,china,europe]
  [-p|--path <path>]      specify another offline pkg path
  [-k|--keep]             keep existing upstream repo during bootstrap

Check https://pigsty.io/docs/setup/offline/#bootstrap for details
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModuleSty, "pig sty boot", args, map[string]interface{}{
			"region":   pigstyBootRegion,
			"path":     pigstyBootPackage,
			"keep":     pigstyBootKeep,
			"pig_home": config.PigstyHome,
		}, func() error {
			if len(args) > 0 {
				return fmt.Errorf("unexpected argument: %v", args)
			}
			if config.PigstyHome == "" {
				logrus.Errorf("pigsty home & inventory not found, specify the inventory with -i")
				return fmt.Errorf("pigsty home & inventory not found")
			}
			bootstrapPath := filepath.Join(config.PigstyHome, "bootstrap")
			if _, err := os.Stat(bootstrapPath); os.IsNotExist(err) {
				logrus.Errorf("bootstrap script not found: %s", bootstrapPath)
				return fmt.Errorf("bootstrap script not found: %s", bootstrapPath)
			}

			cmdArgs := []string{bootstrapPath}
			if pigstyBootRegion != "" {
				cmdArgs = append(cmdArgs, "-r", pigstyBootRegion)
			}
			if pigstyBootPackage != "" {
				cmdArgs = append(cmdArgs, "-p", pigstyBootPackage)
			}
			if pigstyBootKeep {
				cmdArgs = append(cmdArgs, "-k")
			}
			cmdArgs = append(cmdArgs, args...)
			if err := os.Chdir(config.PigstyHome); err != nil {
				return fmt.Errorf("failed to change directory to %s: %w", config.PigstyHome, err)
			}
			logrus.Infof("bootstrap with: %s", strings.Join(cmdArgs, " "))
			err := utils.ShellCommand(cmdArgs)
			if err != nil {
				logrus.Errorf("bootstrap execution failed: %v", err)
				return err
			}
			return nil
		})
	},
}

// pigstyConfCmd is the single configure command with aliases.
var pigstyConfCmd = &cobra.Command{
	Use:         "conf",
	Short:       "Configure Pigsty",
	Annotations: ancsAnn("pig sty conf", "action", "volatile", "safe", true, "medium", "recommended", "root", 10000),
	Aliases:     []string{"c", "configure"},
	Long:        styConfigureLong,
	Example:     styConfigureExample,
	RunE: func(cmd *cobra.Command, args []string) error {
		command := "pig sty conf"
		if cmd != nil {
			if called := strings.TrimSpace(cmd.CalledAs()); called != "" {
				command = "pig sty " + called
			}
		}
		if pigstyConfRaw {
			return runLegacyStructured(legacyModuleSty, command, args, styConfigureParams(), func() error {
				return runPigstyConfigureRaw(args)
			})
		}
		return runPigstyConfigureNative(command, args)
	},
}

func styConfigureParams() map[string]interface{} {
	return map[string]interface{}{
		"conf":            pigstyConfName,
		"ip":              pigstyConfIP,
		"version":         pigstyConfVer,
		"region":          pigstyConfRegion,
		"output_file":     pigstyConfOutput,
		"skip":            pigstyConfSkip,
		"proxy":           pigstyConfProxy,
		"non_interactive": pigstyConfNonInteractive,
		"port":            pigstyConfPort,
		"generate":        pigstyConfGenerate,
		"raw":             pigstyConfRaw,
	}
}

func runPigstyConfigureNative(command string, args []string) error {
	if len(args) > 0 {
		return structuredParamError(
			legacyModuleSty, command, "invalid arguments",
			fmt.Sprintf("unexpected arguments: %s", strings.Join(args, " ")),
			args, styConfigureParams(),
		)
	}
	opts := sty.ConfigureOptions{
		PigstyHome:     config.PigstyHome,
		Mode:           pigstyConfName,
		PrimaryIP:      pigstyConfIP,
		PGVersion:      pigstyConfVer,
		Region:         pigstyConfRegion,
		SSHPort:        pigstyConfPort,
		OutputFile:     pigstyConfOutput,
		Skip:           pigstyConfSkip,
		UseProxy:       pigstyConfProxy,
		NonInteractive: pigstyConfNonInteractive,
		Generate:       pigstyConfGenerate,
	}
	return handleAuxResult(sty.ConfigureNative(opts))
}

func runPigstyConfigureRaw(args []string) error {
	if config.PigstyHome == "" {
		logrus.Errorf("pigsty home & inventory not found, specify the inventory with -i")
		return fmt.Errorf("pigsty home & inventory not found")
	}

	configurePath := filepath.Join(config.PigstyHome, "configure")
	if _, err := os.Stat(configurePath); os.IsNotExist(err) {
		logrus.Errorf("configure script not found: %s", configurePath)
		return fmt.Errorf("configure script not found: %s", configurePath)
	}

	cmdArgs := []string{configurePath}
	if pigstyConfName != "" {
		cmdArgs = append(cmdArgs, "-c", pigstyConfName)
	}
	if pigstyConfIP != "" {
		cmdArgs = append(cmdArgs, "-i", pigstyConfIP)
	}
	if pigstyConfVer != "" {
		cmdArgs = append(cmdArgs, "-v", pigstyConfVer)
	}
	if pigstyConfRegion != "" {
		cmdArgs = append(cmdArgs, "-r", pigstyConfRegion)
	}
	if pigstyConfOutput != "" {
		cmdArgs = append(cmdArgs, "-o", pigstyConfOutput)
	}
	if pigstyConfSkip {
		cmdArgs = append(cmdArgs, "-s")
	}
	if pigstyConfProxy {
		cmdArgs = append(cmdArgs, "-x")
	}
	if pigstyConfNonInteractive {
		cmdArgs = append(cmdArgs, "-n")
	}
	if pigstyConfPort != "" {
		cmdArgs = append(cmdArgs, "-p", pigstyConfPort)
	}
	if pigstyConfGenerate {
		cmdArgs = append(cmdArgs, "-g")
	}
	cmdArgs = append(cmdArgs, args...)

	if err := os.Chdir(config.PigstyHome); err != nil {
		return fmt.Errorf("failed to change directory to %s: %w", config.PigstyHome, err)
	}

	logrus.Infof("configure with: %s", strings.Join(cmdArgs, " "))
	return utils.ShellCommand(cmdArgs)
}

func addStyConfigureFlags(cmd *cobra.Command) {
	if cmd == nil {
		return
	}
	cmd.Flags().StringVarP(&pigstyConfName, "conf", "c", "", "config template name")
	cmd.Flags().StringVar(&pigstyConfIP, "ip", "", "primary ip address")
	cmd.Flags().StringVarP(&pigstyConfVer, "version", "v", "", "postgres major version")
	cmd.Flags().StringVarP(&pigstyConfRegion, "region", "r", "", "upstream repo region")
	cmd.Flags().StringVarP(&pigstyConfOutput, "output-file", "O", "", "output config file path")
	cmd.Flags().BoolVarP(&pigstyConfSkip, "skip", "s", false, "skip ip probe")
	cmd.Flags().BoolVarP(&pigstyConfProxy, "proxy", "x", false, "write proxy env from environment")
	cmd.Flags().BoolVarP(&pigstyConfNonInteractive, "non-interactive", "n", false, "non-interactive mode")
	cmd.Flags().StringVarP(&pigstyConfPort, "port", "p", "", "SSH port (only used if set)")
	cmd.Flags().BoolVarP(&pigstyConfGenerate, "generate", "g", false, "generate random passwords")
}

var pigstyListcmd = &cobra.Command{
	Use:         "list",
	Short:       "list pigsty available versions",
	Annotations: ancsAnn("pig sty list", "query", "volatile", "safe", true, "safe", "none", "current", 5000),
	Aliases:     []string{"l", "ls"},
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
		return runLegacyStructured(legacyModuleSty, "pig sty list", args, nil, func() error {
			logrus.Info("fetching pigsty version info...")
			get.NetworkCondition()
			if get.AllVersions == nil {
				logrus.Errorf("Fail to list pigsty versions")
				return fmt.Errorf("failed to list pigsty versions")
			}
			var since string
			if len(args) > 0 {
				if len(args) > 1 {
					logrus.Warnf("expect at most one version string, unexpected args: %s", strings.Join(args, " "))
				}
				if args[0] == "all" {
					since = "v1.0.0"
					logrus.Debugf("list all versions, set since to %s", since)
				} else {
					since = get.CompleteVersion(args[0])
					logrus.Debugf("complete given %s to %s", args[0], since)
				}
			} else {
				since = "v3.0.0"
				logrus.Debugf("no version given, fallback to %s", since)
			}

			if !get.ValidVersion(since) {
				logrus.Errorf("invalid version: %s", since)
				return fmt.Errorf("invalid version: %s", since)
			}
			logrus.Debugf("the since version %s given", since)

			logrus.Infof("pigsty versions since %s ,from %s", since, get.Source)
			get.PrintAllVersions(since)
			return nil
		})
	},
}

var pigstyGetcmd = &cobra.Command{
	Use:         "get",
	Short:       "download pigsty available versions",
	Annotations: ancsAnn("pig sty get", "action", "volatile", "safe", true, "low", "none", "current", 30000),
	Aliases:     []string{"g", "download"},
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModuleSty, "pig sty get", args, map[string]interface{}{
			"version": pigstyVersion,
			"dir":     pigstyDownloadDir,
		}, func() error {
			get.NetworkCondition()
			if get.AllVersions == nil {
				logrus.Errorf("Fail to get pigsty version list")
				return fmt.Errorf("failed to get pigsty version list")
			}
			if pigstyVersion == "" && len(args) > 0 {
				pigstyVersion = args[0]
			}

			// Ensure version has 'v' prefix if it starts with a number
			if pigstyVersion != "" && pigstyVersion != "latest" && len(pigstyVersion) > 0 {
				if pigstyVersion[0] >= '0' && pigstyVersion[0] <= '9' {
					pigstyVersion = "v" + pigstyVersion
				}
			}

			if completeVer := get.CompleteVersion(pigstyVersion); completeVer != pigstyVersion {
				logrus.Debugf("Complete pigsty version from %s to %s", pigstyVersion, completeVer)
				pigstyVersion = completeVer
			}

			ver := get.IsValidVersion(pigstyVersion)
			if ver == nil {
				logrus.Errorf("Invalid version: %s", pigstyVersion)
				return fmt.Errorf("invalid version: %s", pigstyVersion)
			} else {
				logrus.Infof("Get pigsty src %s from %s to %s", ver.Version, ver.DownloadURL, pigstyDownloadDir)
			}

			logrus.Debugf("Download pigsty src %s to %s", pigstyVersion, pigstyDownloadDir)
			err := get.DownloadSrc(pigstyVersion, pigstyDownloadDir)
			if err != nil {
				logrus.Errorf("failed to download pigsty src: %v", err)
				return err
			}
			return nil
		})
	},
}

var pigstyDeployCmd = &cobra.Command{
	Use:         "deploy",
	Short:       "run pigsty deploy.yml playbook",
	Annotations: ancsAnn("pig sty deploy", "action", "volatile", "unsafe", false, "high", "required", "root", 600000),
	Aliases:     []string{"d", "de", "install", "ins"},
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
			return runPigstyInstall()
		})
	},
}

func runPigstyInstall() error {
	// check ansible-playbook command available
	if _, err := exec.LookPath("ansible-playbook"); err != nil {
		logrus.Errorf("ansible-playbook command not found: %v", err)
		logrus.Infof("have you run: pig sty boot to install ansible?")
		return err
	}

	// check pigsty home exists
	if _, err := os.Stat(config.PigstyHome); os.IsNotExist(err) {
		logrus.Errorf("pigsty home %s not found: %v", config.PigstyHome, err)
		logrus.Infof("have you run: pig sty init to install pigsty?")
		return err
	}

	// check pigsty.yml config exists
	if _, err := os.Stat(config.PigstyConfig); os.IsNotExist(err) {
		logrus.Errorf("pigsty inventory not found: %s", config.PigstyConfig)
		logrus.Infof("have you run: pig sty conf to generate pigsty.yml config?")
		return err
	}

	// check for deploy.yml playbook first, fallback to install.yml for backward compatibility
	var playbookName string
	deployPath := filepath.Join(config.PigstyHome, "deploy.yml")
	installPath := filepath.Join(config.PigstyHome, "install.yml")

	if _, err := os.Stat(deployPath); err == nil {
		// deploy.yml exists, use it
		playbookName = "deploy.yml"
		logrus.Debugf("using deploy.yml playbook: %s", deployPath)
	} else if _, err := os.Stat(installPath); err == nil {
		// deploy.yml not found, fallback to install.yml
		playbookName = "install.yml"
		logrus.Warnf("deploy.yml not found, falling back to install.yml for backward compatibility")
	} else {
		// neither exists
		logrus.Errorf("pigsty playbook not found: neither deploy.yml nor install.yml exists in %s", config.PigstyHome)
		return fmt.Errorf("playbook not found")
	}

	// run the playbook
	if err := os.Chdir(config.PigstyHome); err != nil {
		return fmt.Errorf("failed to change directory to %s: %w", config.PigstyHome, err)
	}
	logrus.Infof("run playbook %s", playbookName)
	logrus.Warnf("IT'S DANGEROUS TO RUN THIS ON INSTALLED SYSTEM!!! Use Ctrl+C to abort")
	return utils.Command([]string{"ansible-playbook", playbookName})
}

func init() {
	pigstyInitCmd.Flags().StringVarP(&pigstyInitPath, "path", "p", "~/pigsty", "target directory")
	pigstyInitCmd.Flags().BoolVarP(&pigstyInitForce, "force", "f", false, "overwrite existing pigsty (false by default)")
	pigstyInitCmd.Flags().StringVarP(&pigstyVersion, "version", "v", "", "pigsty version string")
	pigstyInitCmd.Flags().StringVarP(&pigstyDownloadDir, "dir", "d", "/tmp", "pigsty download directory")

	pigstyBootCmd.Flags().StringVarP(&pigstyBootRegion, "region", "r", "", "default,china,europe,...")
	pigstyBootCmd.Flags().StringVarP(&pigstyBootPackage, "path", "p", "", "offline package path")
	pigstyBootCmd.Flags().BoolVarP(&pigstyBootKeep, "keep", "k", false, "keep existing repo")

	addStyConfigureFlags(pigstyConfCmd)
	pigstyConfCmd.Flags().BoolVar(&pigstyConfRaw, "raw", false, "use raw shell configure implementation")

	pigstyGetcmd.Flags().StringVarP(&pigstyVersion, "version", "v", "", "pigsty version string")
	pigstyGetcmd.Flags().StringVarP(&pigstyDownloadDir, "dir", "d", "/tmp", "pigsty download directory")

	styCmd.AddCommand(pigstyInitCmd, pigstyBootCmd, pigstyConfCmd, pigstyDeployCmd, pigstyListcmd, pigstyGetcmd)

}
