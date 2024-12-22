package pgext

import (
	"fmt"
	"pig/cli/utils"
	"pig/internal/config"
	"strings"

	"github.com/sirupsen/logrus"
)

// RemoveExtension	remove extension based on provided names, aliases, or categories
func RemoveExtensions(pgVer int, names []string, yes bool) error {
	var removeCmds []string
	switch config.OSType {
	case config.DistroEL:
		removeCmds = append(removeCmds, []string{"yum", "remove"}...)
		if yes {
			removeCmds = append(removeCmds, "-y")
		}
	case config.DistroDEB:
		removeCmds = append(removeCmds, []string{"apt-get", "remove"}...)
		if yes {
			removeCmds = append(removeCmds, "-y")
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
		return fmt.Errorf("no packages to be removed")
	}
	removeCmds = append(removeCmds, pkgNames...)
	logrus.Infof("removing extensions: %s", strings.Join(removeCmds, " "))

	return utils.SudoCommand(removeCmds)
}
