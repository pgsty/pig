package ext

import (
	"fmt"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"
	"time"

	"github.com/sirupsen/logrus"
)

// AddExtensions installs extensions and returns a structured Result
// This function is used for YAML/JSON output modes
func AddExtensions(pgVer int, names []string, yes bool) *output.Result {
	prep, result := prepareExtensionPkgOp(preparePkgOpOptions{
		PgVersion:             pgVer,
		Requested:             names,
		ParseVersionSpec:      true,
		MacUnsupportedMessage: "macOS brew installation not supported",
	})
	if result != nil {
		return result
	}

	allPkgNames := prep.Packages
	pkgToExt := prep.PkgToExt
	failed := prep.Failed

	if len(allPkgNames) == 0 {
		data := &ExtensionAddData{
			PgVersion:   prep.PgVersion,
			OSCode:      config.OSCode,
			Arch:        config.OSArch,
			Requested:   names,
			Packages:    []string{},
			Installed:   []*InstalledExtItem{},
			Failed:      failed,
			DurationMs:  time.Since(prep.StartTime).Milliseconds(),
			AutoConfirm: yes,
		}
		return output.Fail(output.CodeExtensionNotFound, "no packages to install").WithData(data)
	}

	installCmds := buildPackageManagerCommand(pkgOpInstall, yes, allPkgNames)
	logrus.Debugf("executing install command: %v", installCmds)

	// Execute install command
	err := utils.SudoCommand(installCmds)

	// Determine which packages were installed successfully
	// For simplicity in structured output, if the command succeeds, all packages are installed
	// If it fails, we report the error
	var installed []*InstalledExtItem
	if err != nil {
		failed = appendPackageFailures(failed, allPkgNames, pkgToExt, err, output.CodeExtensionInstallFailed)
	} else {
		// All packages installed successfully
		for _, pkg := range allPkgNames {
			installed = append(installed, &InstalledExtItem{
				Name:    extNameForPackage(pkg, pkgToExt),
				Package: pkg,
			})
		}
	}

	data := &ExtensionAddData{
		PgVersion:   prep.PgVersion,
		OSCode:      config.OSCode,
		Arch:        config.OSArch,
		Requested:   names,
		Packages:    allPkgNames,
		Installed:   installed,
		Failed:      failed,
		DurationMs:  time.Since(prep.StartTime).Milliseconds(),
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
