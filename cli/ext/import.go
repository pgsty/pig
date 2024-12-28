package ext

import (
	"fmt"
	"os"
	"path/filepath"
	"pig/internal/config"
	"pig/internal/utils"
	"strings"

	"github.com/sirupsen/logrus"
)

// ImportExtensions downloads extension packages to local repository
func ImportExtensions(pgVer int, names []string, importPath string) error {
	logrus.Debugf("importing extensions: pgVer=%d, names=%s, path=%s", pgVer, strings.Join(names, ", "), importPath)
	if len(names) == 0 {
		return fmt.Errorf("no extension names provided")
	}
	if pgVer == 0 {
		logrus.Debugf("no PostgreSQL version specified, set target version to the latest major version: %d", PostgresLatestMajorVersion)
		pgVer = PostgresLatestMajorVersion
	}

	// Create import directory if not exists
	if err := os.MkdirAll(importPath, 0755); err != nil {
		return fmt.Errorf("failed to create import directory: %v", err)
	}

	var downloadCmds []string
	Catalog.LoadAliasMap(config.OSType)
	switch config.OSType {
	case config.DistroEL:
		downloadCmds = append(downloadCmds, []string{"yum", "install", "--downloadonly", "--downloaddir=" + importPath}...)
		if config.OSVersion == "8" || config.OSVersion == "9" {
			downloadCmds[0] = "dnf"
		}
	case config.DistroDEB:
		aptCacheDir := filepath.Join(importPath, "apt-cache")
		if err := os.MkdirAll(aptCacheDir, 0755); err != nil {
			return fmt.Errorf("failed to create apt cache directory: %v", err)
		}
		downloadCmds = append(downloadCmds, []string{"apt-get", "install", "-d", "-o", "Dir::Cache=" + aptCacheDir}...)
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
			// try to find in AliasMap (if it is not a postgres extension)
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
		return fmt.Errorf("no packages to be downloaded")
	}
	downloadCmds = append(downloadCmds, pkgNames...)
	logrus.Infof("downloading extensions: %s", strings.Join(downloadCmds, " "))

	return utils.SudoCommand(downloadCmds)
}
