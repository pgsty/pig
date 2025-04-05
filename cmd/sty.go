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
	pigstyConfSkip           bool
	pigstyConfProxy          bool
	pigstyConfNonInteractive bool

	pigstyDownloadDir string
	pigstyVersion     string
)

// styCmd represents the pigsty management command
var styCmd = &cobra.Command{
	Use:     "sty",
	Short:   "Manage Pigsty Installation",
	Aliases: []string{"s", "pigsty"},
	GroupID: "pigsty",
	Long: `pig sty -Init (Download), Bootstrap, Configure, and Install Pigsty

  pig sty init    [-pfvd]      # install pigsty (~/pigsty by default)
  pig sty boot    [-rpk]       # install ansible and prepare offline pkg
  pig sty conf    [-civrsxn]   # configure pigsty and generate config
  pig sty install              # use pigsty to install & provisioning env (DANGEROUS!)
  pig sty get                  # download pigsty source tarball
  pig sty list                 # list available pigsty versions
`,
	Example: `  Get Started: https://pigsty.io/docs/setup/install/
  pig sty init                 # extract and init ~/pigsty
  pig sty boot                 # install ansible & other deps
  pig sty conf                 # generate pigsty.yml config file
  pig sty install              # run pigsty/install.yml playbook`,
}

// pigstyInitCmd will extract and setup ~/pigsty
var pigstyInitCmd = &cobra.Command{
	Use:     "init",
	Short:   "Install Pigsty",
	Aliases: []string{"i"},
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
		if pigstyVersion == "" {
			pigstyVersion = "latest"
			// logrus.Debugf("install embedded pigsty %s to %s with force=%v", config.PigstyVersion, pigstyInitPath, pigstyInitForce)
			// err := sty.InstallPigsty(nil, pigstyInitPath, pigstyInitForce)
			// if err != nil {
			// 	logrus.Errorf("failed to install pigsty: %v", err)
			// }
			// return nil
		}

		// if version is explicit given, always download & install from remote
		get.NetworkCondition()
		if get.AllVersions == nil {
			logrus.Errorf("Fail to get pigsty version list")
			os.Exit(1)
		}

		pigstyVersion = get.CompleteVersion(pigstyVersion)
		if ver := get.IsValidVersion(pigstyVersion); ver == nil {
			logrus.Errorf("invalid pigsty version: %v", pigstyVersion)
			return nil
		} else {
			logrus.Infof("Get pigsty src %s from %s to %s", ver.Version, ver.DownloadURL, pigstyDownloadDir)
			err := get.DownloadSrc(pigstyVersion, pigstyDownloadDir)
			if err != nil {
				logrus.Errorf("failed to download pigsty src %s: %v", pigstyVersion, err)
				os.Exit(2)
			}
		}

		// downloaded, then extract & install it
		srcTarball, err := sty.LoadPigstySrc(filepath.Join(pigstyDownloadDir, fmt.Sprintf("pigsty-%s.tgz", pigstyVersion)))
		if err != nil {
			logrus.Errorf("failed to load pigsty src %s: %v", pigstyVersion, err)
			os.Exit(3)
		}
		err = sty.InstallPigsty(srcTarball, pigstyInitPath, pigstyInitForce)
		if err != nil {
			logrus.Errorf("failed to install pigsty src %s: %v", pigstyVersion, err)
		}
		logrus.Infof("proceed with pig boot & pig conf")
		return nil
	},
}

var pigstyBootCmd = &cobra.Command{
	Use:     "boot",
	Short:   "Bootstrap Pigsty",
	Aliases: []string{"b", "bootstrap"},
	Long: `Bootstrap pigsty with ./bootstrap script

pig sty boot
  [-r|--region <region]   [default,china,europe]
  [-p|--path <path>]      specify another offline pkg path
  [-k|--keep]             keep existing upstream repo during bootstrap

Check https://pigsty.io/docs/setup/offline/#bootstrap for details
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			return fmt.Errorf("unexpected argument: %v", args)
		}
		if config.PigstyHome == "" {
			logrus.Errorf("pigsty home & inventory not found, specify the inventory with -i")
			os.Exit(1)
		}
		bootstrapPath := filepath.Join(config.PigstyHome, "bootstrap")
		if _, err := os.Stat(bootstrapPath); os.IsNotExist(err) {
			logrus.Errorf("bootstrap script not found: %s", bootstrapPath)
			os.Exit(1)
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
		os.Chdir(config.PigstyHome)
		logrus.Infof("bootstrap with: %s", strings.Join(cmdArgs, " "))
		err := utils.ShellCommand(cmdArgs)
		if err != nil {
			logrus.Errorf("bootstrap execution failed: %v", err)
			os.Exit(1)
		}
		return nil
	},
}

// A thin wrapper around the configure script
var pigstyConfCmd = &cobra.Command{
	Use:     "conf",
	Short:   "Configure Pigsty",
	Aliases: []string{"c", "configure"},
	Long: `Configure pigsty with ./configure

pig sty conf
  [-c|--conf <name>       # [meta|dual|trio|full|prod]
  [--ip <ip>]             # primary IP address (skip with -s)
  [-v|--version <pgver>   # [17|16|15|14|13]
  [-r|--region <region>   # [default|china|europe]
  [-s|--skip]             # skip IP address probing
  [-x|--proxy]            # write proxy env from environment
  [-n|--non-interactive]  # non-interactively mode

Check https://pigsty.io/docs/setup/install/#configure for details
`,
	Example: `
  pig sty conf                       # use the default conf/meta.yml config
  pig sty conf -c rich -x            # use the rich.yml template, add your proxy env to config
  pig sty conf -c supa --ip=10.9.8.7 # use the supa template with 10.9.8.7 as primary IP
  pig sty conf -c full -v 16         # use the 4-node full template with pg16 as default
  pig sty conf -c oss -s             # use the oss template, skip IP probing and replacement
  pig sty conf -c slim -s -r china   # use the 2-node slim template, designate china as region  
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if config.PigstyHome == "" {
			logrus.Errorf("pigsty home & inventory not found, specify the inventory with -i")
			os.Exit(1)
		}
		configurePath := filepath.Join(config.PigstyHome, "configure")
		if _, err := os.Stat(configurePath); os.IsNotExist(err) {
			logrus.Errorf("configure script not found: %s", configurePath)
			os.Exit(1)
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
		if pigstyConfSkip {
			cmdArgs = append(cmdArgs, "-s")
		}
		if pigstyConfProxy {
			cmdArgs = append(cmdArgs, "-p")
		}
		cmdArgs = append(cmdArgs, args...)
		if err := os.Chdir(config.PigstyHome); err != nil {
			logrus.Errorf("failed to change directory to %s: %v", config.PigstyHome, err)
		}

		logrus.Infof("configure with: %s", strings.Join(cmdArgs, " "))
		return utils.ShellCommand(cmdArgs)
	},
}

var pigstyListcmd = &cobra.Command{
	Use:     "list",
	Short:   "list pigsty available versions",
	Aliases: []string{"l", "ls"},
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
		logrus.Info("fetching pigsty version info...")
		get.NetworkCondition()
		if get.AllVersions == nil {
			logrus.Errorf("Fail to list pigsty versions")
			os.Exit(1)
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
		} else {
			logrus.Debugf("the since version %s given", since)
		}

		logrus.Infof("pigsty versions since %s ,from %s", since, get.Source)
		get.PirntAllVersions(since)
		return nil
	},
}

var pigstyGetcmd = &cobra.Command{
	Use:     "get",
	Short:   "download pigsty available versions",
	Aliases: []string{"g", "download"},
	RunE: func(cmd *cobra.Command, args []string) error {
		get.NetworkCondition()
		if get.AllVersions == nil {
			logrus.Errorf("Fail to get pigsty version list")
			os.Exit(1)
		}
		if pigstyVersion == "" && len(args) > 0 {
			pigstyVersion = args[0]
		}

		if completeVer := get.CompleteVersion(pigstyVersion); completeVer != pigstyVersion {
			logrus.Debugf("Complete pigsty version from %s to %s", pigstyVersion, completeVer)
			pigstyVersion = completeVer
		}

		ver := get.IsValidVersion(pigstyVersion)
		if ver == nil {
			logrus.Errorf("Invalid version: %s", pigstyVersion)
			os.Exit(1)
		} else {
			logrus.Infof("Get pigsty src %s from %s to %s", ver.Version, ver.DownloadURL, pigstyDownloadDir)
		}

		logrus.Debugf("Download pigsty src %s to %s", pigstyVersion, pigstyDownloadDir)
		err := get.DownloadSrc(pigstyVersion, pigstyDownloadDir)
		if err != nil {
			logrus.Errorf("failed to download pigsty src: %v", err)
		}
		return nil
	},
}

var pigstyInstallcmd = &cobra.Command{
	Use:     "install",
	Short:   "run pigsty install.yml playbook",
	Aliases: []string{"ins", "install"},
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPigstyInstall()
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

	// check install.yml playbook exists
	playbookPath := filepath.Join(config.PigstyHome, "install.yml")
	if _, err := os.Stat(playbookPath); os.IsNotExist(err) {
		logrus.Errorf("pigsty playbook install.yml not found: %s", playbookPath)
		return err
	}

	// run the install.yml playbook
	os.Chdir(config.PigstyHome)
	logrus.Infof("run playbook %s", playbookPath)
	logrus.Warnf("IT'S DANGEROUS TO RUN THIS ON INSTALLED SYSTEM!!! Use Ctrl+C to abort")
	return utils.Command([]string{"ansible-playbook", "install.yml"})
}

func init() {
	pigstyInitCmd.Flags().StringVarP(&pigstyInitPath, "path", "p", "~/pigsty", "target directory")
	pigstyInitCmd.Flags().BoolVarP(&pigstyInitForce, "force", "f", false, "overwrite existing pigsty (false by default)")
	pigstyInitCmd.Flags().StringVarP(&pigstyVersion, "version", "v", "", "pigsty version string")
	pigstyInitCmd.Flags().StringVarP(&pigstyDownloadDir, "dir", "d", "/tmp", "pigsty download directory")

	pigstyBootCmd.Flags().StringVarP(&pigstyBootRegion, "region", "r", "", "default,china,europe,...")
	pigstyBootCmd.Flags().StringVarP(&pigstyBootPackage, "path", "p", "", "offline package path")
	pigstyBootCmd.Flags().BoolVarP(&pigstyBootKeep, "keep", "k", false, "keep existing repo")

	pigstyConfCmd.Flags().StringVarP(&pigstyConfName, "conf", "c", "", "config template name")
	pigstyConfCmd.Flags().StringVar(&pigstyConfIP, "ip", "", "primary ip address")
	pigstyConfCmd.Flags().StringVarP(&pigstyConfVer, "version", "v", "", "postgres major version")
	pigstyConfCmd.Flags().StringVarP(&pigstyConfRegion, "region", "r", "", "upstream repo region")
	pigstyConfCmd.Flags().BoolVarP(&pigstyConfSkip, "skip", "s", false, "skip ip probe")
	pigstyConfCmd.Flags().BoolVarP(&pigstyConfProxy, "proxy", "p", false, "configure proxy env")
	pigstyConfCmd.Flags().BoolVarP(&pigstyConfNonInteractive, "non-interactive", "n", false, "configure non-interactive")

	pigstyGetcmd.Flags().StringVarP(&pigstyVersion, "version", "v", "", "pigsty version string")
	pigstyGetcmd.Flags().StringVarP(&pigstyDownloadDir, "dir", "d", "/tmp", "pigsty download directory")

	styCmd.AddCommand(pigstyInitCmd, pigstyBootCmd, pigstyConfCmd, pigstyListcmd, pigstyGetcmd, pigstyInstallcmd)

}
