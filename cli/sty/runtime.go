package sty

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"pig/cli/get"
	"pig/cli/license"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"
	"strings"

	"github.com/sirupsen/logrus"
)

type InitOptions struct {
	TargetPath  string
	Force       bool
	Mirror      bool
	AcceptEULA  bool
	Version     string
	DownloadDir string
	Args        []string
}

func Init(opts InitOptions) error {
	version, err := resolveReleaseVersionInput(opts.Version, opts.Args, "latest", output.CodeStyInitInvalidArgs)
	if err != nil {
		return err
	}
	if _, _, _, err := inspectInstallTarget(opts.TargetPath, opts.Force); err != nil {
		return err
	}
	eulaAccepted := opts.AcceptEULA
	if license.Manager.Valid {
		if err := acceptProEULA(eulaAccepted); err != nil {
			return err
		}
		eulaAccepted = true
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
		ver = get.IsValidVersion(version)
	} else {
		version = get.CompleteVersion(version)
		ver = get.IsValidVersion(version)
	}
	if ver == nil {
		logrus.Errorf("invalid pigsty version: %v", version)
		return commandFailedf(output.CodeStyInitInvalidArgs, "invalid pigsty version: %s", version)
	}
	version = ver.Version

	logrus.Infof("Get pigsty src %s from %s to %s", ver.Version, ver.DownloadURL, opts.DownloadDir)
	if err := get.DownloadSrc(version, opts.DownloadDir); err != nil {
		logrus.Errorf("failed to download pigsty src %s: %v", version, err)
		return commandFailed(output.CodeStyInitFailed, fmt.Errorf("failed to download pigsty src %s: %w", version, err))
	}

	srcPath := filepath.Join(opts.DownloadDir, fmt.Sprintf("pigsty-%s.tgz", version))
	srcTarball, err := os.ReadFile(srcPath)
	if err != nil {
		logrus.Errorf("failed to load pigsty src %s: %v", version, err)
		return commandFailed(output.CodeStyInitFailed, fmt.Errorf("failed to load pigsty src %s: %w", version, err))
	}
	if err := installPigsty(srcTarball, opts.TargetPath, installOptions{
		Overwrite: opts.Force, AcceptEULA: eulaAccepted,
	}); err != nil {
		logrus.Errorf("failed to install pigsty src %s: %v", version, err)
		return commandFailed(output.CodeStyInitFailed, err)
	}
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
		logrus.Errorf("pigsty home not found, specify it with -H/--home")
		return commandFailedf(output.CodeStyConfigureResourceMissing, "pigsty home not found")
	}

	configurePath := filepath.Join(opts.PigstyHome, "configure")
	if info, err := os.Stat(configurePath); err != nil || info.IsDir() {
		logrus.Errorf("configure script not found: %s", configurePath)
		return commandFailedf(output.CodeStyConfigureResourceMissing, "configure script not found: %s", configurePath)
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
	logrus.Infof("configure with: %s", strings.Join(cmdArgs, " "))
	return commandFailed(output.CodeStyRawConfigureFailed, utils.CommandInDir(cmdArgs, opts.PigstyHome))
}

type DownloadOptions struct {
	Version     string
	DownloadDir string
	Mirror      bool
	Args        []string
}

func Download(opts DownloadOptions) error {
	version, err := resolveReleaseVersionInput(opts.Version, opts.Args, "", output.CodeStyReleaseInvalidArgs)
	if err != nil {
		return err
	}

	logrus.Info("fetching pigsty version info...")
	get.NetworkCondition()
	if opts.Mirror {
		usePigstyMirrorSource()
	}
	if get.AllVersions == nil {
		logrus.Errorf("Fail to get pigsty version list")
		return commandFailedf(output.CodeStyReleaseMetadataUnavailable, "failed to get pigsty version list")
	}

	if completeVer := get.CompleteVersion(version); completeVer != version {
		logrus.Debugf("Complete pigsty version from %s to %s", version, completeVer)
		version = completeVer
	}

	ver := get.IsValidVersion(version)
	if ver == nil {
		logrus.Errorf("Invalid version: %s", version)
		return commandFailedf(output.CodeStyReleaseInvalidArgs, "invalid version: %s", version)
	}
	version = ver.Version
	logrus.Infof("Get pigsty src %s from %s to %s", ver.Version, ver.DownloadURL, opts.DownloadDir)

	logrus.Debugf("Download pigsty src %s to %s", version, opts.DownloadDir)
	if err := get.DownloadSrc(version, opts.DownloadDir); err != nil {
		logrus.Errorf("failed to download pigsty src: %v", err)
		return commandFailed(output.CodeStyReleaseDownloadFailed, err)
	}
	return nil
}

func resolveReleaseVersionInput(flagValue string, args []string, fallback string, code int) (string, error) {
	version := strings.TrimSpace(flagValue)
	if len(args) > 1 || (len(args) == 1 && version != "") {
		return "", commandFailedf(code, "specify the Pigsty version either positionally or with --version, not both")
	}
	if len(args) == 1 {
		version = strings.TrimSpace(args[0])
	}
	if version == "" {
		version = fallback
	}
	return version, nil
}

type ListOptions struct {
	Mirror bool
	Args   []string
}

func ListVersionsWithOptions(opts ListOptions) error {
	logrus.Info("fetching pigsty version info...")
	get.NetworkCondition()
	if opts.Mirror {
		usePigstyMirrorSource()
	}
	if get.AllVersions == nil {
		logrus.Errorf("Fail to list pigsty versions")
		return commandFailedf(output.CodeStyReleaseMetadataUnavailable, "failed to list pigsty versions")
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
		return commandFailedf(output.CodeStyReleaseInvalidArgs, "invalid version: %s", since)
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
		return commandFailed(output.CodeStyDeployDependencyMissing, err)
	}

	if info, err := os.Stat(opts.PigstyHome); err != nil || !info.IsDir() {
		logrus.Errorf("pigsty home %s not found: %v", opts.PigstyHome, err)
		logrus.Infof("have you run: pig sty init to install pigsty?")
		return commandFailedf(output.CodeStyDeployResourceMissing, "pigsty home not found: %s", opts.PigstyHome)
	}

	if info, err := os.Stat(opts.PigstyConfig); err != nil || !info.Mode().IsRegular() {
		logrus.Errorf("pigsty inventory not found: %s", opts.PigstyConfig)
		logrus.Infof("have you run: pig sty conf to generate pigsty.yml config?")
		return commandFailedf(output.CodeStyDeployResourceMissing, "pigsty inventory not found: %s", opts.PigstyConfig)
	}

	playbookName, err := resolveDeployPlaybook(opts.PigstyHome)
	if err != nil {
		return commandFailed(output.CodeStyDeployResourceMissing, err)
	}

	logrus.Infof("run playbook %s", playbookName)
	logrus.Warnf("IT'S DANGEROUS TO RUN THIS ON INSTALLED SYSTEM!!! Use Ctrl+C to abort")
	return commandFailed(output.CodeStyDeployFailed, utils.CommandInDir([]string{"ansible-playbook", playbookName}, opts.PigstyHome))
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
