// Package build - pipeline.go contains complete build pipeline for packages
package build

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
)

// BuildPackage runs complete build pipeline for a single package
func BuildPackage(pkg string, pgVersions string, withSymbol bool) error {
	fmt.Printf("\n")
	logrus.Info(strings.Repeat("#", 58))
	logrus.Infof("[BUILD PKG] %s", pkg)
	logrus.Info(strings.Repeat("#", 58))

	// Step 1: Download source
	if err := DownloadSource(pkg, false); err != nil {
		logrus.Debugf("Source download error: %v", err)
		// Continue even if download fails
	}

	// Step 2: Install dependencies
	if err := InstallDeps(pkg, pgVersions); err != nil {
		logrus.Warnf("Dependency install error: %v", err)
		// Continue even if deps fail
	}

	// Step 3: Build package
	if err := BuildExtension(pkg, pgVersions, withSymbol); err != nil {
		logrus.Warnf("Build extension error: %v", err)
		// Continue even if build fail
	}

	logrus.Info(strings.Repeat("#", 58))
	return nil
}

// BuildPackages processes multiple packages sequentially
func BuildPackages(packages []string, pgVersions string, withSymbol bool) error {
	if len(packages) == 0 {
		return fmt.Errorf("no packages specified")
	}

	success := 0
	total := len(packages)

	// Process each package completely before moving to next
	for _, pkg := range packages {
		if err := BuildPackage(pkg, pgVersions, withSymbol); err != nil {
			continue
		}
		success++
		fmt.Printf("\n\n")
	}

	// Final summary
	if success == total {
		logrus.Infof("All %d packages build successfully", total)
	} else if success > 0 {
		logrus.Warnf("%d of %d packages build completed", success, total)
	} else {
		logrus.Errorf("All %d packages build failed", total)
	}
	return nil
}
