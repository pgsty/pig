package ext

import (
	"fmt"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"
	"time"

	"github.com/sirupsen/logrus"
)

// RmExtensions removes extensions and returns a structured Result
// This function is used for YAML/JSON output modes
func RmExtensions(pgVer int, names []string, yes bool) *output.Result {
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
		return output.Fail(output.CodeExtensionUnsupportedOS, "macOS brew removal not supported")
	default:
		return output.Fail(output.CodeExtensionUnsupportedOS, fmt.Sprintf("unsupported OS: %s", config.OSType))
	}

	// Check Catalog is initialized
	if Catalog == nil {
		return output.Fail(output.CodeExtensionCatalogError, "extension catalog not initialized")
	}

	Catalog.LoadAliasMap(config.OSType)

	// Collect packages to remove, tracking each extension
	var allPkgNames []string
	var removed []string
	var failed []*FailedExtItem
	pkgToExt := make(map[string]string) // maps package name to extension name

	for _, name := range names {
		ext, ok := Catalog.ExtNameMap[name]
		if !ok {
			ext, ok = Catalog.ExtPkgMap[name]
		}
		if !ok {
			// try to find in AliasMap (if it is not a postgres extension)
			if pgPkg, ok := Catalog.AliasMap[name]; ok {
				pkgNamesProcessed := processPkgName(pgPkg, pgVer)
				for _, pkg := range pkgNamesProcessed {
					pkgToExt[pkg] = name
				}
				allPkgNames = append(allPkgNames, pkgNamesProcessed...)
				continue
			} else {
				// Extension not found
				failed = append(failed, &FailedExtItem{
					Name:  name,
					Error: "extension not found in catalog",
					Code:  output.CodeExtensionNotFound,
				})
				continue
			}
		}

		pkgName := ext.PackageName(pgVer)
		if pkgName == "" {
			failed = append(failed, &FailedExtItem{
				Name:  name,
				Error: fmt.Sprintf("no package available for extension on PG %d", pgVer),
				Code:  output.CodeExtensionNoPackage,
			})
			continue
		}

		pkgNamesProcessed := processPkgName(pkgName, pgVer)
		for _, pkg := range pkgNamesProcessed {
			pkgToExt[pkg] = ext.Name
		}
		allPkgNames = append(allPkgNames, pkgNamesProcessed...)
	}

	// If no packages to remove, return early
	if len(allPkgNames) == 0 {
		data := &ExtensionRmData{
			PgVersion:   pgVer,
			OSCode:      config.OSCode,
			Arch:        config.OSArch,
			Requested:   names,
			Packages:    []string{},
			Removed:     []string{},
			Failed:      failed,
			DurationMs:  time.Since(startTime).Milliseconds(),
			AutoConfirm: yes,
		}
		return output.Fail(output.CodeExtensionNotFound, "no packages to remove").WithData(data)
	}

	// Build remove command
	var removeCmds []string
	pkgMgr := getPackageManagerCmd("remove")
	switch config.OSType {
	case config.DistroEL:
		removeCmds = append(removeCmds, pkgMgr, "remove")
		if yes {
			removeCmds = append(removeCmds, "-y")
		}
	case config.DistroDEB:
		removeCmds = append(removeCmds, pkgMgr, "remove")
		if yes {
			removeCmds = append(removeCmds, "-y")
		}
	}

	removeCmds = append(removeCmds, allPkgNames...)
	logrus.Debugf("executing remove command: %v", removeCmds)

	// Execute remove command
	err := utils.SudoCommand(removeCmds)

	// Determine which packages were removed successfully
	if err != nil {
		// All packages failed to remove
		for _, pkg := range allPkgNames {
			extName := pkgToExt[pkg]
			if extName == "" {
				extName = pkg
			}
			failed = append(failed, &FailedExtItem{
				Name:    extName,
				Package: pkg,
				Error:   err.Error(),
				Code:    output.CodeExtensionRemoveFailed,
			})
		}
	} else {
		// All packages removed successfully
		for _, pkg := range allPkgNames {
			extName := pkgToExt[pkg]
			if extName == "" {
				extName = pkg
			}
			removed = append(removed, extName)
		}
	}

	data := &ExtensionRmData{
		PgVersion:   pgVer,
		OSCode:      config.OSCode,
		Arch:        config.OSArch,
		Requested:   names,
		Packages:    allPkgNames,
		Removed:     removed,
		Failed:      failed,
		DurationMs:  time.Since(startTime).Milliseconds(),
		AutoConfirm: yes,
	}

	// Determine overall result
	if len(failed) > 0 {
		message := fmt.Sprintf("removed %d extensions, failed %d", len(removed), len(failed))
		result := output.Fail(output.CodeExtensionRemoveFailed, message).WithData(data)
		return result
	}

	message := fmt.Sprintf("Removed %d extensions", len(removed))
	return output.OK(message, data)
}
