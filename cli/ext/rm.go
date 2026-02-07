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
	prep, result := prepareExtensionPkgOp(preparePkgOpOptions{
		PgVersion:             pgVer,
		Requested:             names,
		ParseVersionSpec:      false,
		MacUnsupportedMessage: "macOS brew removal not supported",
	})
	if result != nil {
		return result
	}

	allPkgNames := prep.Packages
	pkgToExt := prep.PkgToExt
	failed := prep.Failed

	// If no packages to remove, return early
	if len(allPkgNames) == 0 {
		data := &ExtensionRmData{
			PgVersion:   prep.PgVersion,
			OSCode:      config.OSCode,
			Arch:        config.OSArch,
			Requested:   names,
			Packages:    []string{},
			Removed:     []string{},
			Failed:      failed,
			DurationMs:  time.Since(prep.StartTime).Milliseconds(),
			AutoConfirm: yes,
		}
		return output.Fail(output.CodeExtensionNotFound, "no packages to remove").WithData(data)
	}

	removeCmds := buildPackageManagerCommand(pkgOpRemove, yes, allPkgNames)
	logrus.Debugf("executing remove command: %v", removeCmds)

	// Execute remove command
	err := utils.SudoCommand(removeCmds)

	// Determine which packages were removed successfully
	var removed []string
	if err != nil {
		failed = appendPackageFailures(failed, allPkgNames, pkgToExt, err, output.CodeExtensionRemoveFailed)
	} else {
		// All packages removed successfully
		for _, pkg := range allPkgNames {
			removed = append(removed, extNameForPackage(pkg, pkgToExt))
		}
	}

	data := &ExtensionRmData{
		PgVersion:   prep.PgVersion,
		OSCode:      config.OSCode,
		Arch:        config.OSArch,
		Requested:   names,
		Packages:    allPkgNames,
		Removed:     removed,
		Failed:      failed,
		DurationMs:  time.Since(prep.StartTime).Milliseconds(),
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
