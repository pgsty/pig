package install

import (
	"fmt"
	"pig/cli/ext"
	"pig/internal/config"
	"pig/internal/utils"
	"strings"

	"github.com/sirupsen/logrus"
)

// InstallPackages installs packages using native package manager with alias translation
func InstallPackages(pgVer int, names []string, yes bool, noTranslation bool) error {
	logrus.Debugf("installing packages: pgVer=%d, names=%s, yes=%v, noTranslation=%v", pgVer, strings.Join(names, ", "), yes, noTranslation)
	if len(names) == 0 {
		return fmt.Errorf("no package names provided")
	}
	if pgVer == 0 {
		logrus.Debugf("no PostgreSQL version specified, set target version to the latest major version: %d", ext.PostgresLatestMajorVersion)
		pgVer = ext.PostgresLatestMajorVersion
	}

	var installCmds []string
	switch config.OSType {
	case config.DistroEL:
		installCmds = append(installCmds, []string{ext.PackageManagerCmd("install"), "install"}...)
		if yes {
			installCmds = append(installCmds, "-y")
		}
	case config.DistroDEB:
		installCmds = append(installCmds, []string{ext.PackageManagerCmd("install"), "install"}...)
		if yes {
			installCmds = append(installCmds, "-y")
		}
	case config.DistroMAC:
		logrus.Warnf("macOS brew installation is not supported yet")
		return fmt.Errorf("macOS brew installation is not supported yet")
	default:
		return fmt.Errorf("unsupported OS type: %s", config.OSType)
	}

	resolved := ext.ResolveInstallPackages(pgVer, names, noTranslation)
	pkgNames := resolved.Packages

	if len(pkgNames) == 0 {
		return fmt.Errorf("no packages to be installed")
	}
	installCmds = append(installCmds, pkgNames...)
	logrus.Infof("installing packages: %s", strings.Join(installCmds, " "))

	return utils.SudoCommand(installCmds)
}
