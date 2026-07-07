package sty

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"pig/cli/get"
	"pig/internal/config"
	"pig/internal/utils"
	"strings"

	"github.com/sirupsen/logrus"
)

type InitOptions struct {
	TargetPath  string
	Force       bool
	Mirror      bool
	Version     string
	DownloadDir string
	Args        []string
}

func Init(opts InitOptions) error {
	version := strings.TrimSpace(opts.Version)
	if len(opts.Args) > 0 && version == "" {
		version = opts.Args[0]
	}
	if version == "" {
		version = "latest"
	}
	if version != "latest" && len(version) > 0 && version[0] >= '0' && version[0] <= '9' {
		version = "v" + version
	}

	get.NetworkCondition()
	if opts.Mirror {
		usePigstyMirrorSource()
	}
	var ver *get.VersionInfo
	if get.AllVersions == nil {
		logrus.Warnf("Fail to get pigsty version list, use the built-in version %s", config.PigstyVersion)
		if version == "latest" {
			version = config.PigstyVersion
		}
		if len(version) > 0 && version[0] >= '0' && version[0] <= '9' {
			version = "v" + version
		}
		baseURL := config.RepoPigstyIO
		if opts.Mirror {
			baseURL = config.RepoPigstyCC
		}
		ver = &get.VersionInfo{
			Version:     version,
			DownloadURL: fmt.Sprintf("%s/src/pigsty-%s.tgz", baseURL, version),
		}
	} else {
		version = get.CompleteVersion(version)
		ver = get.IsValidVersion(version)
		if ver == nil {
			logrus.Errorf("invalid pigsty version: %v", version)
			return fmt.Errorf("invalid pigsty version: %s", version)
		}
	}

	logrus.Infof("Get pigsty src %s from %s to %s", ver.Version, ver.DownloadURL, opts.DownloadDir)
	if err := get.DownloadSrc(version, opts.DownloadDir); err != nil {
		logrus.Errorf("failed to download pigsty src %s: %v", version, err)
		return fmt.Errorf("failed to download pigsty src %s: %w", version, err)
	}

	srcPath := filepath.Join(opts.DownloadDir, fmt.Sprintf("pigsty-%s.tgz", version))
	srcTarball, err := LoadPigstySrc(srcPath)
	if err != nil {
		logrus.Errorf("failed to load pigsty src %s: %v", version, err)
		return fmt.Errorf("failed to load pigsty src %s: %w", version, err)
	}
	if err := InstallPigsty(srcTarball, opts.TargetPath, opts.Force); err != nil {
		logrus.Errorf("failed to install pigsty src %s: %v", version, err)
		return err
	}
	logrus.Infof("proceed with pig boot & pig conf")
	return nil
}

func usePigstyMirrorSource() {
	get.Source = get.ViaCC
	get.Region = "china"
	for i := range get.AllVersions {
		filename := fmt.Sprintf("pigsty-%s.tgz", get.AllVersions[i].Version)
		get.AllVersions[i].DownloadURL = fmt.Sprintf("%s/src/%s", config.RepoPigstyCC, filename)
	}
}

type BootstrapOptions struct {
	PigstyHome  string
	Region      string
	PackagePath string
	Keep        bool
}

func Bootstrap(opts BootstrapOptions) error {
	if opts.PigstyHome == "" {
		logrus.Errorf("pigsty home & inventory not found, specify the inventory with -i")
		return fmt.Errorf("pigsty home & inventory not found")
	}
	bootstrapPath := filepath.Join(opts.PigstyHome, "bootstrap")
	if _, err := os.Stat(bootstrapPath); os.IsNotExist(err) {
		logrus.Errorf("bootstrap script not found: %s", bootstrapPath)
		return fmt.Errorf("bootstrap script not found: %s", bootstrapPath)
	}

	cmdArgs := []string{bootstrapPath}
	if opts.Region != "" {
		cmdArgs = append(cmdArgs, "-r", opts.Region)
	}
	if opts.PackagePath != "" {
		cmdArgs = append(cmdArgs, "-p", opts.PackagePath)
	}
	if opts.Keep {
		cmdArgs = append(cmdArgs, "-k")
	}
	if err := os.Chdir(opts.PigstyHome); err != nil {
		return fmt.Errorf("failed to change directory to %s: %w", opts.PigstyHome, err)
	}
	logrus.Infof("bootstrap with: %s", strings.Join(cmdArgs, " "))
	if err := utils.ShellCommand(cmdArgs); err != nil {
		logrus.Errorf("bootstrap execution failed: %v", err)
		return err
	}
	return nil
}

type RawConfigureOptions struct {
	PigstyHome string
	Mode       string
	PrimaryIP  string
	PGVersion  string
	Region     string
	SSHPort    string
	OutputFile string

	Skip           bool
	UseProxy       bool
	NonInteractive bool
	Generate       bool
	Args           []string
}

func ConfigureRaw(opts RawConfigureOptions) error {
	if opts.PigstyHome == "" {
		logrus.Errorf("pigsty home & inventory not found, specify the inventory with -i")
		return fmt.Errorf("pigsty home & inventory not found")
	}

	configurePath := filepath.Join(opts.PigstyHome, "configure")
	if _, err := os.Stat(configurePath); os.IsNotExist(err) {
		logrus.Errorf("configure script not found: %s", configurePath)
		return fmt.Errorf("configure script not found: %s", configurePath)
	}

	cmdArgs := []string{configurePath}
	if opts.Mode != "" {
		cmdArgs = append(cmdArgs, "-c", opts.Mode)
	}
	if opts.PrimaryIP != "" {
		cmdArgs = append(cmdArgs, "-i", opts.PrimaryIP)
	}
	if opts.PGVersion != "" {
		cmdArgs = append(cmdArgs, "-v", opts.PGVersion)
	}
	if opts.Region != "" {
		cmdArgs = append(cmdArgs, "-r", opts.Region)
	}
	if opts.OutputFile != "" {
		cmdArgs = append(cmdArgs, "-o", opts.OutputFile)
	}
	if opts.Skip {
		cmdArgs = append(cmdArgs, "-s")
	}
	if opts.UseProxy {
		cmdArgs = append(cmdArgs, "-x")
	}
	if opts.NonInteractive {
		cmdArgs = append(cmdArgs, "-n")
	}
	if opts.SSHPort != "" {
		cmdArgs = append(cmdArgs, "-p", opts.SSHPort)
	}
	if opts.Generate {
		cmdArgs = append(cmdArgs, "-g")
	}
	cmdArgs = append(cmdArgs, opts.Args...)

	if err := os.Chdir(opts.PigstyHome); err != nil {
		return fmt.Errorf("failed to change directory to %s: %w", opts.PigstyHome, err)
	}

	logrus.Infof("configure with: %s", strings.Join(cmdArgs, " "))
	return utils.ShellCommand(cmdArgs)
}

type DownloadOptions struct {
	Version     string
	DownloadDir string
	Mirror      bool
	Args        []string
}

func Download(opts DownloadOptions) error {
	logrus.Info("fetching pigsty version info...")
	get.NetworkCondition()
	if opts.Mirror {
		usePigstyMirrorSource()
	}
	if get.AllVersions == nil {
		logrus.Errorf("Fail to get pigsty version list")
		return fmt.Errorf("failed to get pigsty version list")
	}

	version := strings.TrimSpace(opts.Version)
	if version == "" && len(opts.Args) > 0 {
		version = opts.Args[0]
	}
	if version != "" && version != "latest" && len(version) > 0 && version[0] >= '0' && version[0] <= '9' {
		version = "v" + version
	}

	if completeVer := get.CompleteVersion(version); completeVer != version {
		logrus.Debugf("Complete pigsty version from %s to %s", version, completeVer)
		version = completeVer
	}

	ver := get.IsValidVersion(version)
	if ver == nil {
		logrus.Errorf("Invalid version: %s", version)
		return fmt.Errorf("invalid version: %s", version)
	}
	logrus.Infof("Get pigsty src %s from %s to %s", ver.Version, ver.DownloadURL, opts.DownloadDir)

	logrus.Debugf("Download pigsty src %s to %s", version, opts.DownloadDir)
	if err := get.DownloadSrc(version, opts.DownloadDir); err != nil {
		logrus.Errorf("failed to download pigsty src: %v", err)
		return err
	}
	return nil
}

type ListOptions struct {
	Mirror bool
	Args   []string
}

func ListVersions(args []string) error {
	return ListVersionsWithOptions(ListOptions{Args: args})
}

func ListVersionsWithOptions(opts ListOptions) error {
	logrus.Info("fetching pigsty version info...")
	get.NetworkCondition()
	if opts.Mirror {
		usePigstyMirrorSource()
	}
	if get.AllVersions == nil {
		logrus.Errorf("Fail to list pigsty versions")
		return fmt.Errorf("failed to list pigsty versions")
	}

	var since string
	args := opts.Args
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
}

type DeployOptions struct {
	PigstyHome   string
	PigstyConfig string
}

func Deploy(opts DeployOptions) error {
	if _, err := exec.LookPath("ansible-playbook"); err != nil {
		logrus.Errorf("ansible-playbook command not found: %v", err)
		logrus.Infof("have you run: pig sty boot to install ansible?")
		return err
	}

	if _, err := os.Stat(opts.PigstyHome); os.IsNotExist(err) {
		logrus.Errorf("pigsty home %s not found: %v", opts.PigstyHome, err)
		logrus.Infof("have you run: pig sty init to install pigsty?")
		return err
	}

	if _, err := os.Stat(opts.PigstyConfig); os.IsNotExist(err) {
		logrus.Errorf("pigsty inventory not found: %s", opts.PigstyConfig)
		logrus.Infof("have you run: pig sty conf to generate pigsty.yml config?")
		return err
	}

	playbookName, err := resolveDeployPlaybook(opts.PigstyHome)
	if err != nil {
		return err
	}

	if err := os.Chdir(opts.PigstyHome); err != nil {
		return fmt.Errorf("failed to change directory to %s: %w", opts.PigstyHome, err)
	}
	logrus.Infof("run playbook %s", playbookName)
	logrus.Warnf("IT'S DANGEROUS TO RUN THIS ON INSTALLED SYSTEM!!! Use Ctrl+C to abort")
	return utils.Command([]string{"ansible-playbook", playbookName})
}

func resolveDeployPlaybook(home string) (string, error) {
	deployPath := filepath.Join(home, "deploy.yml")
	installPath := filepath.Join(home, "install.yml")

	if _, err := os.Stat(deployPath); err == nil {
		logrus.Debugf("using deploy.yml playbook: %s", deployPath)
		return "deploy.yml", nil
	}
	if _, err := os.Stat(installPath); err == nil {
		logrus.Warnf("deploy.yml not found, falling back to install.yml for backward compatibility")
		return "install.yml", nil
	}

	logrus.Errorf("pigsty playbook not found: neither deploy.yml nor install.yml exists in %s", home)
	return "", fmt.Errorf("playbook not found")
}
