package build

import (
	"fmt"
	"pig/internal/utils"
	"strings"

	"github.com/sirupsen/logrus"
)

// BuildAll performs complete build pipeline: get source, install deps, build package
func BuildAll(pkgs []string, pgVersions string, withSymbol bool) error {
	if len(pkgs) == 0 {
		return fmt.Errorf("no packages specified")
	}

	// Log the complete pipeline
	logrus.Infof("Starting complete build pipeline for: %s", strings.Join(pkgs, ", "))
	if pgVersions != "" {
		logrus.Infof("Using PG versions: %s", pgVersions)
	}

	// Process each package
	for _, pkg := range pkgs {
		logrus.Info(utils.PadHeader(fmt.Sprintf("Building %s", pkg), 80))

		// Step 1: Download source code (don't force download in build all)
		logrus.Infof("[1/3] Downloading source code for %s", pkg)
		if err := DownloadCodeTarball([]string{pkg}, false); err != nil {
			logrus.Errorf("Failed to download source for %s: %v", pkg, err)
			// Continue with next package on download failure
			continue
		}

		// Step 2: Install build dependencies
		logrus.Infof("[2/3] Installing build dependencies for %s", pkg)
		if err := InstallExtensionDeps([]string{pkg}, pgVersions); err != nil {
			logrus.Errorf("Failed to install dependencies for %s: %v", pkg, err)
			// Continue with next package on dep install failure
			continue
		}

		// Step 3: Build package
		logrus.Infof("[3/3] Building package for %s", pkg)
		if err := BuildPackage(pkg, pgVersions, withSymbol); err != nil {
			logrus.Errorf("Failed to build package %s: %v", pkg, err)
			// Log error but continue with next package
			continue
		}

		logrus.Infof("Successfully completed build pipeline for %s", pkg)
		logrus.Info(utils.PadHeader(fmt.Sprintf("%s complete", pkg), 80))
	}

	logrus.Info(utils.PadHeader("All builds complete", 80))
	return nil
}
