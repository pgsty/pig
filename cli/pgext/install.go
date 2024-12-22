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
		pkgNames = append(pkgNames, processPkgName(pkgName, pgVer)...)
		logrus.Debugf("translate extension %s to package name: %s", ext.Name, pkgName)
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
