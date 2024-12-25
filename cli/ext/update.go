package ext

import (
	"fmt"
	"pig/cli/utils"
	"pig/internal/config"
	"strings"

	"github.com/sirupsen/logrus"
)

// UpdateExtensions will upgrade extensions based on provided names, aliases, or categories
func UpdateExtensions(pgVer int, names []string, yes bool) error {
	logrus.Debugf("updating extensions: pgVer=%d, names=%s, yes=%v", pgVer, strings.Join(names, ", "), yes)
	if len(names) == 0 {
		return fmt.Errorf("no extension names provided")
	}
	if pgVer == 0 {
		logrus.Debugf("no PostgreSQL version specified, set target version to the latest major version: %d", PostgresLatestMajorVersion)
		pgVer = PostgresLatestMajorVersion
	}

	var updateCmds []string
	Catalog.LoadAliasMap(config.OSType)
	switch config.OSType {
	case config.DistroEL:
		updateCmds = append(updateCmds, []string{"yum", "update"}...)
		if config.OSVersion == "8" || config.OSVersion == "9" {
			updateCmds[0] = "dnf"
		}
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

	var pkgNames []string
	for _, name := range names {
		ext, ok := Catalog.ExtNameMap[name]
		if !ok {
			ext, ok = Catalog.ExtAliasMap[name]
		}

		if !ok {
			// try to find in PostgresPackageMap (if it is not a postgres extension)
			if pgPkg, ok := Catalog.AliasMap[name]; ok {
				pkgNames = append(pkgNames, processPkgName(pgPkg, pgVer)...)
				continue
			} else {
				logrus.Debugf("cannot find '%s' in extension name or alias", name)
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
