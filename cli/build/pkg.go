// Package build - pipeline.go contains complete build pipeline for packages
package build

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
)

// BuildPackage runs complete build pipeline for a single package
func BuildPackage(pkg string, pgVersions string, withSymbol bool, mirror bool) error {
	fmt.Printf("\n")
	logrus.Info(strings.Repeat("#", 58))
	logrus.Infof("[BUILD PKG] %s", pkg)
	logrus.Info(strings.Repeat("#", 58))

	// Step 1: Download source
	if err := DownloadSource(pkg, false, mirror); err != nil {
		// Source is a hard dependency for building; do not proceed when missing.
		return fmt.Errorf("source download failed for %s: %w", pkg, err)
	}

	// Step 2: Install dependencies
	if err := InstallDeps(pkg, pgVersions); err != nil {
		// Deps may already be installed; keep going, but surface the error clearly.
		logrus.Warnf("Dependency install error for %s: %v", pkg, err)
	}

	// Step 3: Build package
	if err := BuildExtension(pkg, pgVersions, withSymbol); err != nil {
		return fmt.Errorf("build failed for %s: %w", pkg, err)
	}

	logrus.Info(strings.Repeat("#", 58))
	return nil
}

// BuildPackages processes multiple packages sequentially
func BuildPackages(packages []string, pgVersions string, withSymbol bool, mirror bool) error {
	if len(packages) == 0 {
		return fmt.Errorf("no packages specified")
	}

	success := 0
	total := len(packages)
	var failed []string

	// Process each package completely before moving to next
	for _, pkg := range packages {
		if err := BuildPackage(pkg, pgVersions, withSymbol, mirror); err != nil {
			failed = append(failed, pkg)
			logrus.Errorf("Failed to build %s: %v", pkg, err)
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

	if len(failed) > 0 {
		return fmt.Errorf("%d of %d package(s) failed: %s", len(failed), total, strings.Join(failed, ", "))
	}
	return nil
}
