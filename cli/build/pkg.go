// Package build - pipeline.go contains complete build pipeline for packages
package build

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
)

// BuildPackage runs complete build pipeline for a single package
func BuildPackage(pkg string, pgVersions string, withSymbol bool) error {
	fmt.Println()
	logrus.Info(strings.Repeat("=", 58))
	logrus.Infof("[PIPELINE] %s", pkg)
	logrus.Info(strings.Repeat("=", 58))

	// Step 1: Download source
	if err := DownloadSource(pkg, false); err != nil {
		logrus.Debugf("Source download: %v", err)
		// Continue even if download fails
	}

	// Step 2: Install dependencies
	if err := InstallDeps(pkg, pgVersions); err != nil {
		logrus.Warnf("Dependency install: %v", err)
		// Continue even if deps fail
	}

	// Step 3: Build package
	if err := BuildExtension(pkg, pgVersions, withSymbol); err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	logrus.Infof("[PIPELINE] %s completed", pkg)
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
			logrus.Errorf("[PIPELINE] %s failed: %v", pkg, err)
			continue
		}
		success++
	}

	// Final summary
	fmt.Println()
	logrus.Info(strings.Repeat("=", 58))
	if success == total {
		logrus.Infof("[DONE] All %d packages completed successfully", total)
	} else if success > 0 {
		logrus.Warnf("[DONE] %d of %d packages completed", success, total)
	} else {
		logrus.Errorf("[DONE] All %d packages failed", total)
	}
	logrus.Info(strings.Repeat("=", 58))

	if success == 0 {
		return fmt.Errorf("all builds failed")
	}
	return nil
}
