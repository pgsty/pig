package ext

import (
	"fmt"
	"pig/internal/config"
	"pig/internal/utils"
	"strings"

	"github.com/sirupsen/logrus"
)

// RemoveExtensions will remove extension based on provided names, aliases, or categories
func RemoveExtensions(pgVer int, names []string, yes bool) error {
	logrus.Debugf("removing extensions: pgVer=%d, names=%s, yes=%v", pgVer, strings.Join(names, ", "), yes)
	if len(names) == 0 {
		return fmt.Errorf("no extension names provided")
	}
	if pgVer == 0 {
		logrus.Debugf("no PostgreSQL version specified, set target version to the latest major version: %d", PostgresLatestMajorVersion)
		pgVer = PostgresLatestMajorVersion
	}

	var removeCmds []string
	Catalog.LoadAliasMap(config.OSType)
	switch config.OSType {
	case config.DistroEL:
		removeCmds = append(removeCmds, []string{"yum", "remove"}...)
		if config.OSVersion == "8" || config.OSVersion == "9" {
			removeCmds[0] = "dnf"
		}
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

	var pkgNames []string
	for _, name := range names {
		ext, ok := Catalog.ExtNameMap[name]
		if !ok {
			ext, ok = Catalog.ExtPkgMap[name]
		}

		if !ok {
			// try to find in PostgresPackageMap (if it is not a postgres extension)
			if pgPkg, ok := Catalog.AliasMap[name]; ok {
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
