package build

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
)

// Complete build pipeline: get source, install deps, build package
func BuildPackages(packages []string, pgVersions string, withSymbol bool) error {
	if len(packages) == 0 {
		return fmt.Errorf("no packages specified")
	}

	success := 0
	total := len(packages)

	for _, pkg := range packages {
		logrus.Info(strings.Repeat("#", 58))
		logrus.Infof("[PIPELINE] Building: %s", pkg)
		logrus.Info(strings.Repeat("#", 58))

		if err := buildPipeline(pkg, pgVersions, withSymbol); err != nil {
			logrus.Errorf("[PIPELINE] Failed: %s - %v", pkg, err)
			continue
		}

		success++
		logrus.Infof("[BUILDING] %s DONE", pkg)
		logrus.Info(strings.Repeat("=", 58))
	}

	// Summary
	if success == total {
		logrus.Infof("✓ Built all %d packages", total)
	} else if success > 0 {
		logrus.Infof("⚠ Built %d/%d packages", success, total)
	} else {
		logrus.Error("✗ All builds failed")
	}

	if success == 0 {
		return fmt.Errorf("all builds failed")
	}
	return nil
}

// Build pipeline for a single package
func buildPipeline(pkg, pgVersions string, withSymbol bool) error {
	// Step 1: Download source
	logrus.Infof("[1/3] Downloading source")
	if err := DownloadCodeTarball([]string{pkg}, false); err != nil {
		logrus.Debugf("Source download: %v", err)
	}

	// Step 2: Install dependencies
	logrus.Infof("[2/3] Installing dependencies")
	if err := InstallExtensionDeps([]string{pkg}, pgVersions); err != nil {
		logrus.Warnf("Dependency install: %v", err)
	}

	// Step 3: Build package
	logrus.Infof("[3/3] Building package")
	if err := BuildExtension(pkg, pgVersions, withSymbol); err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	return nil
}
