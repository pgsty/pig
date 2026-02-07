package ext

import (
	"fmt"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"
	"time"

	"github.com/sirupsen/logrus"
)

// getPackageManagerCmd returns the appropriate package manager command for the current OS
// This helper function eliminates code duplication across add/rm/update operations
func getPackageManagerCmd(operation string) string {
	return PackageManagerCmd(operation)
}

// processPkgName processes the package name and returns the list of package names according to the given version
func processPkgName(pkgName string, pgVer int) []string {
	return ProcessPkgName(pkgName, pgVer)
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

	// Collect packages to install, tracking each extension
	var installed []*InstalledExtItem
	var failed []*FailedExtItem
	resolved := ResolveExtensionPackages(pgVer, names, true)
	for _, name := range resolved.NotFound {
		failed = append(failed, &FailedExtItem{
			Name:  name,
			Error: "extension not found in catalog",
			Code:  output.CodeExtensionNotFound,
		})
	}
	for _, name := range resolved.NoPackage {
		failed = append(failed, &FailedExtItem{
			Name:  name,
			Error: fmt.Sprintf("no package available for extension on PG %d", pgVer),
			Code:  output.CodeExtensionNoPackage,
		})
	}
	allPkgNames := resolved.Packages
	pkgToExt := resolved.PackageOwner

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
