package ext

import (
	"fmt"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"
	"time"

	"github.com/sirupsen/logrus"
)

// UpgradeExtensions updates extensions and returns a structured Result
// This function is used for YAML/JSON output modes
func UpgradeExtensions(pgVer int, names []string, yes bool) *output.Result {
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
		return output.Fail(output.CodeExtensionUnsupportedOS, "macOS brew update not supported")
	default:
		return output.Fail(output.CodeExtensionUnsupportedOS, fmt.Sprintf("unsupported OS: %s", config.OSType))
	}

	// Check Catalog is initialized
	if Catalog == nil {
		return output.Fail(output.CodeExtensionCatalogError, "extension catalog not initialized")
	}

	// Collect packages to update, tracking each extension
	var updated []string
	var failed []*FailedExtItem
	resolved := ResolveExtensionPackages(pgVer, names, false)
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

	// If no packages to update, return early
	if len(allPkgNames) == 0 {
		data := &ExtensionUpdateData{
			PgVersion:   pgVer,
			OSCode:      config.OSCode,
			Arch:        config.OSArch,
			Requested:   names,
			Packages:    []string{},
			Updated:     []string{},
			Failed:      failed,
			DurationMs:  time.Since(startTime).Milliseconds(),
			AutoConfirm: yes,
		}
		return output.Fail(output.CodeExtensionNotFound, "no packages to update").WithData(data)
	}

	// Build update command
	var updateCmds []string
	pkgMgr := getPackageManagerCmd("update")
	switch config.OSType {
	case config.DistroEL:
		updateCmds = append(updateCmds, pkgMgr, "update")
		if yes {
			updateCmds = append(updateCmds, "-y")
		}
	case config.DistroDEB:
		updateCmds = append(updateCmds, pkgMgr, "install", "--only-upgrade")
		if yes {
			updateCmds = append(updateCmds, "-y")
		}
	}

	updateCmds = append(updateCmds, allPkgNames...)
	logrus.Debugf("executing update command: %v", updateCmds)

	// Execute update command
	err := utils.SudoCommand(updateCmds)

	// Determine which packages were updated successfully
	if err != nil {
		// All packages failed to update
		for _, pkg := range allPkgNames {
			extName := pkgToExt[pkg]
			if extName == "" {
				extName = pkg
			}
			failed = append(failed, &FailedExtItem{
				Name:    extName,
				Package: pkg,
				Error:   err.Error(),
				Code:    output.CodeExtensionUpdateFailed,
			})
		}
	} else {
		// All packages updated successfully
		for _, pkg := range allPkgNames {
			extName := pkgToExt[pkg]
			if extName == "" {
				extName = pkg
			}
			updated = append(updated, extName)
		}
	}

	data := &ExtensionUpdateData{
		PgVersion:   pgVer,
		OSCode:      config.OSCode,
		Arch:        config.OSArch,
		Requested:   names,
		Packages:    allPkgNames,
		Updated:     updated,
		Failed:      failed,
		DurationMs:  time.Since(startTime).Milliseconds(),
		AutoConfirm: yes,
	}

	// Determine overall result
	if len(failed) > 0 {
		message := fmt.Sprintf("updated %d extensions, failed %d", len(updated), len(failed))
		result := output.Fail(output.CodeExtensionUpdateFailed, message).WithData(data)
		return result
	}

	message := fmt.Sprintf("Updated %d extensions", len(updated))
	return output.OK(message, data)
}
