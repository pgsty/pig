package pgext

import (
	"fmt"
	"pig/cli/utils"
	"pig/internal/config"
	"strings"

	"github.com/sirupsen/logrus"
)

// RemoveExtension	remove extension based on provided names, aliases, or categories
func UpdateExtensions(pgVer int, names []string, yes bool) error {
	var updateCmds []string
	switch config.OSType {
	case config.DistroEL:
		updateCmds = append(updateCmds, []string{"yum", "update"}...)
		if yes {
			updateCmds = append(updateCmds, "-y")
		}
	case config.DistroDEB:
		updateCmds = append(updateCmds, []string{"apt-get", "upgrade"}...)
		if yes {
			updateCmds = append(updateCmds, "-y")
		}
	default:
		return fmt.Errorf("unsupported OS type: %s", config.OSType)
	}

	if err := InitExtension(nil); err != nil {
		return err
	}
	if len(names) == 0 {
		return fmt.Errorf("no extension names provided")
	}

	var pkgNames []string
	for _, name := range names {
		ext, ok := ExtNameMap[name]
		if !ok {
			ext, ok = ExtAliasMap[name]
		}

		if !ok {
			// try to find in PostgresPackageMap (if it is not a postgres extension)
			if pgPkg, ok := PostgresPackageMap[name]; ok {
				pkgNames = append(pkgNames, processPkgName(pgPkg, pgVer)...)
				continue
			} else {
				logrus.Debugf("can not found '%s' in extension name or alias", name)
				continue
			}
		}
		pkgName := ext.PackageName(pgVer)
		if pkgName == "" {
			logrus.Warnf("no package found for extension %s", ext.Name)
			continue
		}
		logrus.Debugf("translate extension %s to package name: %s", ext.Name, pkgName)
		pkgNames = append(pkgNames, processPkgName(pkgName, pgVer)...)
	}

	if len(pkgNames) == 0 {
		return fmt.Errorf("no packages to be updated")
	}
	updateCmds = append(updateCmds, pkgNames...)
	logrus.Infof("updating extensions: %s", strings.Join(updateCmds, " "))

	return utils.SudoCommand(updateCmds)
}
