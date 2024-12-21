package pgext

import (
	"fmt"
	"pig/cli/pgsql"
	"pig/cli/utils"
	"pig/internal/config"
	"strings"

	"github.com/sirupsen/logrus"
)

var CategoryList = []string{"TIME", "GIS", "RAG", "FTS", "OLAP", "FEAT", "LANG", "TYPE", "FUNC", "ADMIN", "STAT", "SEC", "FDW", "SIM", "ETL"}

// InstallExtensions installs extensions based on provided names, aliases, or categories
func InstallExtensions(names []string, pg *pgsql.PostgresInstallation) error {
	var installCmds []string
	if config.OSType == config.DistroEL {
		installCmds = append(installCmds, []string{"yum", "install", "-y"}...)
	} else if config.OSType == config.DistroDEB {
		installCmds = append(installCmds, []string{"apt-get", "install", "-y"}...)
	} else {
		return fmt.Errorf("unsupported OS type: %s", config.OSType)
	}

	if err := InitExtensionData(nil); err != nil {
		return err
	}
	if len(names) == 0 {
		return fmt.Errorf("no extension names provided")
	}

	var pkgNames []string
	pkgNameSet := make(map[string]struct{})
	for _, name := range names {
		ext, ok := ExtNameMap[name]
		if !ok {
			ext, ok = ExtAliasMap[name]
		}
		if !ok {
			logrus.Warnf("can not found '%s' in extension name or alias", name)
			continue
		}
		pkgName := ext.PackageName(pg.MajorVersion)
		if pkgName == "" {
			logrus.Warnf("no package found for extension %s", ext.Name)
			continue
		}
		logrus.Debugf("translate extension %s to package name: %s", ext.Name, pkgName)
		if _, exists := pkgNameSet[pkgName]; !exists {
			pkgNames = append(pkgNames, pkgName)
			pkgNameSet[pkgName] = struct{}{}
		}
	}

	if len(pkgNames) == 0 {
		return fmt.Errorf("no packages to be installed")
	}
	installCmds = append(installCmds, pkgNames...)
	logrus.Infof("installing extensions: %s", strings.Join(installCmds, " "))

	return utils.SudoCommand(installCmds)
}

// RemoveExtension
func RemoveExtensions(names []string, pg *pgsql.PostgresInstallation) error {
	var removeCmds []string
	if config.OSType == config.DistroEL {
		removeCmds = append(removeCmds, []string{"yum", "remove", "-y"}...)
	} else if config.OSType == config.DistroDEB {
		removeCmds = append(removeCmds, []string{"apt-get", "remove", "-y"}...)
	} else {
		return fmt.Errorf("unsupported OS type: %s", config.OSType)
	}

	if err := InitExtensionData(nil); err != nil {
		return err
	}
	if len(names) == 0 {
		return fmt.Errorf("no extension names provided")
	}

	var pkgNames []string
	pkgNameSet := make(map[string]struct{})
	for _, name := range names {
		ext, ok := ExtNameMap[name]
		if !ok {
			ext, ok = ExtAliasMap[name]
		}
		if !ok {
			logrus.Warnf("can not found '%s' in extension name or alias", name)
			continue
		}
		pkgName := ext.PackageName(pg.MajorVersion)
		if pkgName == "" {
			logrus.Warnf("no package found for extension %s", ext.Name)
			continue
		}
		logrus.Debugf("translate extension %s to package name: %s", ext.Name, pkgName)
		if _, exists := pkgNameSet[pkgName]; !exists {
			pkgNames = append(pkgNames, pkgName)
			pkgNameSet[pkgName] = struct{}{}
		}
	}

	if len(pkgNames) == 0 {
		return fmt.Errorf("no packages to be removed")
	}
	removeCmds = append(removeCmds, pkgNames...)
	logrus.Infof("removing extensions: %s", strings.Join(removeCmds, " "))

	return utils.SudoCommand(removeCmds)

}
