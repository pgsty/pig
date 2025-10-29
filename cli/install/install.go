package install

import (
	"fmt"
	"os"
	"pig/cli/ext"
	"pig/internal/config"
	"pig/internal/utils"
	"strconv"
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
		installCmds = append(installCmds, []string{"yum", "install"}...)
		if config.OSVersion == "8" || config.OSVersion == "9" || config.OSVersion == "10" {
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

	// Load alias map for translation if not disabled
	if !noTranslation {
		ext.Catalog.LoadAliasMap(config.OSType)
	}

	for _, name := range names {
		// package version is specified in (name=version format)
		var version string
		if parts := strings.Split(name, "="); len(parts) == 2 {
			name = parts[0]
			version = parts[1]
		}

		var pkgNamesProcessed []string
		translated := false

		// Try translation only if not disabled
		if !noTranslation {
			// First try extension name/alias translation
			if extension, ok := ext.Catalog.ExtNameMap[name]; ok {
				pkgName := extension.PackageName(pgVer)
				if pkgName != "" {
					logrus.Infof("translate extension '%s' to package: %s", name, pkgName)
					pkgNamesProcessed = processPkgName(pkgName, pgVer)
					translated = true
				}
			} else if extension, ok := ext.Catalog.ExtPkgMap[name]; ok {
				pkgName := extension.PackageName(pgVer)
				if pkgName != "" {
					logrus.Infof("translate extension '%s' to package: %s", name, pkgName)
					pkgNamesProcessed = processPkgName(pkgName, pgVer)
					translated = true
				}
			} else if pgPkg, ok := ext.Catalog.AliasMap[name]; ok {
				logrus.Infof("translate alias '%s' to package: %s", name, pgPkg)
				pkgNamesProcessed = processPkgName(pgPkg, pgVer)
				translated = true
			}
		}

		// If no translation found or translation disabled, use original name
		if !translated {
			logrus.Debugf("package '%s' not found in catalog, using as-is", name)
			pkgNamesProcessed = []string{name}
		}

		// Apply version specification
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
	logrus.Infof("installing packages: %s", strings.Join(installCmds, " "))

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
