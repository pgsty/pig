package ext

import (
	"fmt"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// InstallExtensions installs extensions based on provided names, aliases, or categories
func InstallExtensions(pgVer int, names []string, yes bool) error {
	logrus.Debugf("installing extensions: version=%d names=%v yes=%v", pgVer, names, yes)
	if len(names) == 0 {
		return fmt.Errorf("no extensions specified")
	}
	if pgVer == 0 {
		logrus.Debugf("using latest postgres version: %d by default", PostgresLatestMajorVersion)
		pgVer = PostgresLatestMajorVersion
	}

	var installCmds []string
	Catalog.LoadAliasMap(config.OSType)
	pkgMgr := getPackageManagerCmd("install")
	switch config.OSType {
	case config.DistroEL:
		installCmds = append(installCmds, pkgMgr, "install")
		if yes {
			installCmds = append(installCmds, "-y")
		}
	case config.DistroDEB:
		installCmds = append(installCmds, pkgMgr, "install")
		if yes {
			installCmds = append(installCmds, "-y")
		}
	case config.DistroMAC:
		return fmt.Errorf("macOS brew installation not supported")
	default:
		return fmt.Errorf("unsupported OS: %s", config.OSType)
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
			ext, ok = Catalog.ExtPkgMap[name]
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
				logrus.Debugf("extension not found in catalog: %s", name)
				continue
			}
		}
		pkgName := ext.PackageName(pgVer)
		if pkgName == "" {
			logrus.Warnf("no package available for extension: %s", ext.Name)
			continue
		}
		logrus.Debugf("resolved package: %s -> %s", ext.Name, pkgName)

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
		return fmt.Errorf("no packages to install")
	}
	installCmds = append(installCmds, pkgNames...)
	logrus.Infof("installing: %s", strings.Join(pkgNames, " "))

	return utils.SudoCommand(installCmds)
}

// getPackageManagerCmd returns the appropriate package manager command for the current OS
// This helper function eliminates code duplication across add/rm/update operations
func getPackageManagerCmd(operation string) string {
	switch config.OSType {
	case config.DistroEL:
		// EL 8/9/10 use dnf, older versions use yum
		if config.OSVersion == "8" || config.OSVersion == "9" || config.OSVersion == "10" {
			return "dnf"
		}
		return "yum"
	case config.DistroDEB:
		return "apt-get"
	default:
		return ""
	}
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

// AddExtensions installs extensions and returns a structured Result
// This function is used for YAML/JSON output modes
func AddExtensions(pgVer int, names []string, yes bool) *output.Result {
	startTime := time.Now()

	if len(names) == 0 {
		return output.Fail(output.CodeExtensionInvalidArgs, "no extensions specified")
	}
	if pgVer == 0 {
		logrus.Debugf("using latest postgres version: %d by default", PostgresLatestMajorVersion)
		pgVer = PostgresLatestMajorVersion
	}

	// Check OS support
	switch config.OSType {
	case config.DistroEL, config.DistroDEB:
		// supported
	case config.DistroMAC:
		return output.Fail(output.CodeExtensionUnsupportedOS, "macOS brew installation not supported")
	default:
		return output.Fail(output.CodeExtensionUnsupportedOS, fmt.Sprintf("unsupported OS: %s", config.OSType))
	}

	// Check Catalog is initialized
	if Catalog == nil {
		return output.Fail(output.CodeExtensionCatalogError, "extension catalog not initialized")
	}

	Catalog.LoadAliasMap(config.OSType)

	// Collect packages to install, tracking each extension
	var allPkgNames []string
	var installed []*InstalledExtItem
	var failed []*FailedExtItem
	pkgToExt := make(map[string]string) // maps package name to extension name

	for _, name := range names {
		// package version is specified in (name=version format)
		var version string
		originalName := name
		if parts := strings.Split(name, "="); len(parts) == 2 {
			name = parts[0]
			version = parts[1]
		}

		ext, ok := Catalog.ExtNameMap[name]
		if !ok {
			ext, ok = Catalog.ExtPkgMap[name]
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
				for _, pkg := range pkgNamesProcessed {
					pkgToExt[pkg] = originalName
				}
				allPkgNames = append(allPkgNames, pkgNamesProcessed...)
				continue
			} else {
				// Extension not found
				failed = append(failed, &FailedExtItem{
					Name:  originalName,
					Error: "extension not found in catalog",
					Code:  output.CodeExtensionNotFound,
				})
				continue
			}
		}

		pkgName := ext.PackageName(pgVer)
		if pkgName == "" {
			failed = append(failed, &FailedExtItem{
				Name:  originalName,
				Error: fmt.Sprintf("no package available for extension on PG %d", pgVer),
				Code:  output.CodeExtensionNoPackage,
			})
			continue
		}

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
		for _, pkg := range pkgNamesProcessed {
			pkgToExt[pkg] = ext.Name
		}
		allPkgNames = append(allPkgNames, pkgNamesProcessed...)
	}

	// If no packages to install, return early
	if len(allPkgNames) == 0 {
		data := &ExtensionAddData{
			PgVersion:   pgVer,
			OSCode:      config.OSCode,
			Arch:        config.OSArch,
			Requested:   names,
			Packages:    []string{},
			Installed:   []*InstalledExtItem{},
			Failed:      failed,
			DurationMs:  time.Since(startTime).Milliseconds(),
			AutoConfirm: yes,
		}
		return output.Fail(output.CodeExtensionNotFound, "no packages to install").WithData(data)
	}

	// Build install command
	var installCmds []string
	pkgMgr := getPackageManagerCmd("install")
	switch config.OSType {
	case config.DistroEL:
		installCmds = append(installCmds, pkgMgr, "install")
		if yes {
			installCmds = append(installCmds, "-y")
		}
	case config.DistroDEB:
		installCmds = append(installCmds, pkgMgr, "install")
		if yes {
			installCmds = append(installCmds, "-y")
		}
	}

	installCmds = append(installCmds, allPkgNames...)
	logrus.Debugf("executing install command: %v", installCmds)

	// Execute install command
	err := utils.SudoCommand(installCmds)

	// Determine which packages were installed successfully
	// For simplicity in structured output, if the command succeeds, all packages are installed
	// If it fails, we report the error
	if err != nil {
		// All packages failed to install
		for _, pkg := range allPkgNames {
			extName := pkgToExt[pkg]
			if extName == "" {
				extName = pkg
			}
			failed = append(failed, &FailedExtItem{
				Name:    extName,
				Package: pkg,
				Error:   err.Error(),
				Code:    output.CodeExtensionInstallFailed,
			})
		}
	} else {
		// All packages installed successfully
		for _, pkg := range allPkgNames {
			extName := pkgToExt[pkg]
			if extName == "" {
				extName = pkg
			}
			installed = append(installed, &InstalledExtItem{
				Name:    extName,
				Package: pkg,
			})
		}
	}

	data := &ExtensionAddData{
		PgVersion:   pgVer,
		OSCode:      config.OSCode,
		Arch:        config.OSArch,
		Requested:   names,
		Packages:    allPkgNames,
		Installed:   installed,
		Failed:      failed,
		DurationMs:  time.Since(startTime).Milliseconds(),
		AutoConfirm: yes,
	}

	// Determine overall result
	if len(failed) > 0 {
		message := fmt.Sprintf("installed %d extensions, failed %d", len(installed), len(failed))
		result := output.Fail(output.CodeExtensionInstallFailed, message).WithData(data)
		return result
	}

	message := fmt.Sprintf("Installed %d extensions", len(installed))
	return output.OK(message, data)
}
