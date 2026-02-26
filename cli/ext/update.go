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
	// Safety: never auto-upgrade everything. Without explicit targets, do nothing.
	if len(names) == 0 {
		if pgVer == 0 {
			pgVer = PostgresLatestMajorVersion
		}
		data := &ExtensionUpdateData{
			PgVersion:   pgVer,
			OSCode:      config.OSCode,
			Arch:        config.OSArch,
			Requested:   []string{},
			Packages:    []string{},
			Updated:     []string{},
			Failed:      nil,
			DurationMs:  0,
			AutoConfirm: yes,
		}
		return output.OK("no extensions specified, nothing to update", data)
	}

	prep, result := prepareExtensionPkgOp(preparePkgOpOptions{
		PgVersion:             pgVer,
		Requested:             names,
		ParseVersionSpec:      false,
		MacUnsupportedMessage: "macOS brew update not supported",
	})
	if result != nil {
		return result
	}

	allPkgNames := prep.Packages
	pkgToExt := prep.PkgToExt
	failed := prep.Failed

	// If no packages to update, return early
	if len(allPkgNames) == 0 {
		data := &ExtensionUpdateData{
			PgVersion:   prep.PgVersion,
			OSCode:      config.OSCode,
			Arch:        config.OSArch,
			Requested:   names,
			Packages:    []string{},
			Updated:     []string{},
			Failed:      failed,
			DurationMs:  time.Since(prep.StartTime).Milliseconds(),
			AutoConfirm: yes,
		}
		return output.Fail(output.CodeExtensionNotFound, "no packages to update").WithData(data)
	}

	updateCmds := buildPackageManagerCommand(pkgOpUpdate, yes, allPkgNames)
	logrus.Debugf("executing update command: %v", updateCmds)

	// Execute update command
	err := utils.SudoCommand(updateCmds)

	// Determine which packages were updated successfully
	var updated []string
	if err != nil {
		failed = appendPackageFailures(failed, allPkgNames, pkgToExt, err, output.CodeExtensionUpdateFailed)
	} else {
		// All packages updated successfully
		for _, pkg := range allPkgNames {
			updated = append(updated, pkg)
		}
	}

	data := &ExtensionUpdateData{
		PgVersion:   prep.PgVersion,
		OSCode:      config.OSCode,
		Arch:        config.OSArch,
		Requested:   names,
		Packages:    allPkgNames,
		Updated:     updated,
		Failed:      failed,
		DurationMs:  time.Since(prep.StartTime).Milliseconds(),
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
