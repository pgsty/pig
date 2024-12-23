package pgext

import (
	"fmt"
	"pig/cli/utils"
	"pig/internal/config"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
)

// InstallExtensions installs extensions based on provided names, aliases, or categories
func InstallExtensions(pgVer int, names []string, yes bool) error {
	var installCmds []string
	if config.OSType == config.DistroEL {
		installCmds = append(installCmds, []string{"yum", "install"}...)
		if yes {
			installCmds = append(installCmds, "-y")
		}
	} else if config.OSType == config.DistroDEB {
		installCmds = append(installCmds, []string{"apt-get", "install"}...)
		if yes {
			installCmds = append(installCmds, "-y")
		}
	} else {
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
		// Check if version is specified (name=version format)
		var version string
		if parts := strings.Split(name, "="); len(parts) == 2 {
			name = parts[0]
			version = parts[1]
		}

		ext, ok := ExtNameMap[name]
		if !ok {
			ext, ok = ExtAliasMap[name]
		}
		if !ok {
			// try to find in PostgresPackageMap (if it is not a postgres extension)
			if pgPkg, ok := PostgresPackageMap[name]; ok {
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
