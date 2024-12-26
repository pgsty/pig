package ext

import (
	"fmt"
	"os"
	"pig/internal/config"
	"pig/internal/utils"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
)

// InstallExtensions installs extensions based on provided names, aliases, or categories
func InstallExtensions(pgVer int, names []string, yes bool) error {
	logrus.Debugf("installing extensions: pgVer=%d, names=%s, yes=%v", pgVer, strings.Join(names, ", "), yes)
	if len(names) == 0 {
		return fmt.Errorf("no extension names provided")
	}
	if pgVer == 0 {
		logrus.Debugf("no PostgreSQL version specified, set target version to the latest major version: %d", PostgresLatestMajorVersion)
		pgVer = PostgresLatestMajorVersion
	}

	var installCmds []string
	Catalog.LoadAliasMap(config.OSType)
	switch config.OSType {
	case config.DistroEL:
		installCmds = append(installCmds, []string{"yum", "install"}...)
		if config.OSVersion == "8" || config.OSVersion == "9" {
			installCmds[0] = "dnf"
		}
		if yes {
			installCmds = append(installCmds, "-y")
		}
	case config.DistroDEB:
		installCmds = append(installCmds, []string{"apt-get", "install"}...)
		if yes {
			installCmds = append(installCmds, "-y")
		}
	case config.DistroMAC:
		logrus.Warnf("macOS brew installation is not supported yet")
		os.Exit(1)
	default:
		return fmt.Errorf("unsupported OS type: %s", config.OSType)
	}

	var pkgNames []string
	for _, name := range names {
		// package version is specified in (name=version format)
		var version string
		if parts := strings.Split(name, "="); len(parts) == 2 {
			name = parts[0]
			version = parts[1]
		}
		ext, ok := Catalog.ExtNameMap[name]
		if !ok {
			ext, ok = Catalog.ExtAliasMap[name]
		}
		if !ok {
			// try to find in AliasMap (if it is not a postgres extension)
			if pgPkg, ok := Catalog.AliasMap[name]; ok {
				pkgNamesProcessed := processPkgName(pgPkg, pgVer)
				if version != "" {
					for i, pkg := range pkgNamesProcessed {
						if config.OSType == config.DistroEL {
							pkgNamesProcessed[i] = fmt.Sprintf("%s-%s", pkg, version)
						} else if config.OSType == config.DistroDEB {
							pkgNamesProcessed[i] = fmt.Sprintf("%s=%s*", pkg, version)
						}
					}
				}
				pkgNames = append(pkgNames, pkgNamesProcessed...)
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

		pkgNamesProcessed := processPkgName(pkgName, pgVer)
		if version != "" {
			for i, pkg := range pkgNamesProcessed {
				if config.OSType == config.DistroEL {
					pkgNamesProcessed[i] = fmt.Sprintf("%s-%s", pkg, version)
				} else if config.OSType == config.DistroDEB {
					pkgNamesProcessed[i] = fmt.Sprintf("%s=%s*", pkg, version)
				}
			}
		}
		pkgNames = append(pkgNames, pkgNamesProcessed...)
	}

	if len(pkgNames) == 0 {
		return fmt.Errorf("no packages to be installed")
	}
	installCmds = append(installCmds, pkgNames...)
	logrus.Infof("installing extensions: %s", strings.Join(installCmds, " "))

	return utils.SudoCommand(installCmds)
}

// processPkgName processes the package name and returns the list of package names according to the given version
func processPkgName(pkgName string, pgVer int) []string {
	if pkgName == "" {
		return []string{}
	}
	parts := strings.Split(strings.Replace(strings.TrimSpace(pkgName), ",", " ", -1), " ")
	var pkgNames []string
	pkgNameSet := make(map[string]struct{})
	for _, part := range parts {
		partStr := strings.ReplaceAll(part, "$v", strconv.Itoa(pgVer))
		if _, exists := pkgNameSet[partStr]; !exists {
			pkgNames = append(pkgNames, partStr)
			pkgNameSet[partStr] = struct{}{}
		}
	}
	return pkgNames
}
