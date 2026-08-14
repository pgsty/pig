package install

import (
	"fmt"
	"os"
	"os/exec"
	"pig/cli/ext"
	"pig/internal/config"
	"pig/internal/utils"
	"strings"

	"github.com/sirupsen/logrus"
)

// PackageOptions keeps the existing installer as the single place that
// resolves aliases and builds package-manager argv. LocalOnly is used by
// offline bootstrap so enabled network repositories cannot be consulted.
type PackageOptions struct {
	Yes           bool
	NoTranslation bool
	LocalOnly     bool
	APTSourceList string
	Quiet         bool
}

var (
	runPackageInstall        = utils.SudoCommand
	runPackageInstallSummary = func(args []string) error { return utils.SudoCommandSummary(args, 4) }
	runPackageQuery          = func(name string, args ...string) ([]byte, error) {
		cmd := exec.Command(name, args...)
		cmd.Env = append(os.Environ(), "LC_ALL=C")
		return cmd.CombinedOutput()
	}
)

// InstallPackages installs packages using native package manager with alias translation
func InstallPackages(pgVer int, names []string, yes bool, noTranslation bool) error {
	return InstallPackagesWithOptions(pgVer, names, PackageOptions{Yes: yes, NoTranslation: noTranslation})
}

// InstallPackagesWithOptions installs packages with an optional strict local
// repository scope.
func InstallPackagesWithOptions(pgVer int, names []string, opts PackageOptions) error {
	yes, noTranslation := opts.Yes, opts.NoTranslation
	logrus.Debugf("installing packages: pgVer=%d, names=%s, yes=%v, noTranslation=%v", pgVer, strings.Join(names, ", "), yes, noTranslation)
	if len(names) == 0 {
		return fmt.Errorf("no package names provided")
	}
	if pgVer == 0 {
		logrus.Debugf("no PostgreSQL version specified, set target version to the latest major version: %d", ext.PostgresLatestMajorVersion())
		pgVer = ext.PostgresLatestMajorVersion()
	}

	if config.OSType == config.DistroMAC {
		logrus.Warnf("macOS brew installation is not supported yet")
		return fmt.Errorf("macOS brew installation is not supported yet")
	}
	manager := ext.PackageManagerCmd()
	if manager == "" {
		return fmt.Errorf("unsupported OS type: %s", config.OSType)
	}
	scopeArgs := []string(nil)
	runInstall := runPackageInstall
	if opts.Quiet {
		runInstall = runPackageInstallSummary
	}
	if config.OSType == config.DistroDEB {
		scopeArgs = append(scopeArgs, "-o", "Acquire::Retries=3")
	}
	if opts.LocalOnly {
		switch config.OSType {
		case config.DistroEL:
			scopeArgs = append(scopeArgs, "--disablerepo=*", "--enablerepo=pigsty-local")
		case config.DistroDEB:
			if opts.APTSourceList == "" {
				opts.APTSourceList = "/etc/apt/sources.list.d/local.list"
			}
			scopeArgs = append(scopeArgs,
				"-o", "Dir::Etc::sourcelist="+opts.APTSourceList,
				"-o", "Dir::Etc::sourceparts=-")
		}
		cacheCmd := append([]string{manager}, scopeArgs...)
		if config.OSType == config.DistroEL {
			cacheCmd = append(cacheCmd, "makecache")
		} else {
			cacheCmd = append(cacheCmd, "update")
		}
		if err := runInstall(cacheCmd); err != nil {
			return fmt.Errorf("failed to refresh local repository: %w", err)
		}
	}
	installCmds := append([]string{manager}, scopeArgs...)
	switch config.OSType {
	case config.DistroEL:
		installCmds = append(installCmds, "install")
		if yes {
			installCmds = append(installCmds, "-y")
		}
	case config.DistroDEB:
		installCmds = append(installCmds, "install")
		if yes {
			installCmds = append(installCmds, "-y")
		}
	default:
		return fmt.Errorf("unsupported OS type: %s", config.OSType)
	}

	resolved := ext.ResolveInstallPackages(pgVer, names, noTranslation)
	pkgNames := resolved.Packages

	if len(pkgNames) == 0 {
		return fmt.Errorf("no packages to be installed")
	}
	installCmds = append(installCmds, pkgNames...)
	if opts.Quiet {
		logrus.Debugf("installing packages quietly: %s", strings.Join(installCmds, " "))
	} else {
		logrus.Infof("installing packages: %s", strings.Join(installCmds, " "))
	}

	if err := runInstall(installCmds); err != nil {
		return err
	}
	if opts.LocalOnly {
		if err := verifyLocalPackagesAvailable(manager, pkgNames, opts); err != nil {
			return err
		}
	}
	return nil
}

// verifyLocalPackagesAvailable distinguishes an installed package from one the
// selected local repository can actually provide. Bootstrap calls this after
// its transaction so dnf/yum query tooling requested in the same package set is
// available even on a minimal host.
func verifyLocalPackagesAvailable(manager string, packages []string, opts PackageOptions) error {
	if config.OSType == config.DistroEL {
		return verifyELLocalPackagesAvailable(manager, packages)
	}
	var name string
	var args []string
	switch config.OSType {
	case config.DistroDEB:
		if opts.APTSourceList == "" {
			opts.APTSourceList = "/etc/apt/sources.list.d/local.list"
		}
		name = "apt-cache"
		args = []string{
			"-o", "Dir::Etc::sourcelist=" + opts.APTSourceList,
			"-o", "Dir::Etc::sourceparts=-",
			"madison",
		}
	default:
		return fmt.Errorf("unsupported OS type: %s", config.OSType)
	}
	args = append(args, packages...)
	output, err := runPackageQuery(name, args...)
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail == "" {
			return fmt.Errorf("failed to query local repository packages: %w", err)
		}
		return fmt.Errorf("failed to query local repository packages: %w: %s", err, detail)
	}
	available := make(map[string]bool, len(packages))
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if config.OSType == config.DistroDEB {
			if fields := strings.SplitN(line, "|", 2); len(fields) == 2 {
				available[strings.TrimSpace(fields[0])] = true
			}
		} else {
			available[line] = true
		}
	}
	missing := make([]string, 0)
	for _, pkg := range packages {
		if !available[pkg] {
			missing = append(missing, pkg)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("local repository does not provide required packages: %s", strings.Join(missing, ", "))
	}
	return nil
}

func verifyELLocalPackagesAvailable(manager string, packages []string) error {
	missing := make([]string, 0)
	for _, pkg := range packages {
		name := manager
		args := []string{"-q", "--disablerepo=*", "--enablerepo=pigsty-local", "repoquery", "--available", "--whatprovides", pkg, "--qf=%{name}"}
		if manager == "yum" {
			name = "repoquery"
			args = []string{"--repoid=pigsty-local", "--whatprovides", pkg, "--qf=%{name}"}
		}
		output, err := runPackageQuery(name, args...)
		if err != nil {
			detail := strings.TrimSpace(string(output))
			if detail == "" {
				return fmt.Errorf("failed to query local repository provider for %s: %w", pkg, err)
			}
			return fmt.Errorf("failed to query local repository provider for %s: %w: %s", pkg, err, detail)
		}
		if strings.TrimSpace(string(output)) == "" {
			missing = append(missing, pkg)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("local repository does not provide required packages: %s", strings.Join(missing, ", "))
	}
	return nil
}
