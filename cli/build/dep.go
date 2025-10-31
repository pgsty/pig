// Package build - deps.go handles build dependency installation
package build

import (
	"fmt"
	"os"
	"path/filepath"
	"pig/internal/config"
	"pig/internal/utils"
	"strings"

	"github.com/sirupsen/logrus"
)

// InstallDeps installs build dependencies for a single package
func InstallDeps(pkg string, pgVersion string) error {
	logrus.Info(strings.Repeat("=", 58))
	if pgVersion != "" {
		logrus.Infof("[DEPENDENCE] %s (PG%s)", pkg, pgVersion)
	} else {
		logrus.Infof("[DEPENDENCE] %s", pkg)
	}
	logrus.Info(strings.Repeat("=", 58))

	switch config.OSType {
	case config.DistroEL:
		return installRpmDep(pkg, pgVersion)
	case config.DistroDEB:
		return installDebDep(pkg, pgVersion)
	default:
		return fmt.Errorf("unsupported OS type: %s", config.OSType)
	}
}

// InstallDepsList processes multiple packages
func InstallDepsList(packages []string, pgVersionsStr string) error {
	if len(packages) == 0 {
		return fmt.Errorf("no packages specified")
	}

	// Parse PG versions if provided
	var pgVersions []string
	if pgVersionsStr != "" {
		pgVersions = strings.Split(pgVersionsStr, ",")
		for i := range pgVersions {
			pgVersions[i] = strings.TrimSpace(pgVersions[i])
		}
	}

	for _, pkg := range packages {
		// Check if package is an extension
		_, err := ResolvePackage(pkg)
		isExtension := err == nil

		if isExtension && len(pgVersions) > 0 {
			// For extensions with specified PG versions, install deps for each version
			for _, pgVer := range pgVersions {
				logrus.Infof("Installing deps for extension %s (PG%s)", pkg, pgVer)
				if err := InstallDeps(pkg, pgVer); err != nil {
					logrus.Errorf("Failed to install deps for %s (PG%s): %v", pkg, pgVer, err)
					// Continue with next version
				}
			}
		} else if isExtension && len(pgVersions) == 0 {
			// For extensions without specified versions, use auto-detection
			if err := InstallDeps(pkg, ""); err != nil {
				logrus.Errorf("Failed to install deps for %s: %v", pkg, err)
			}
		} else {
			// For non-extension packages, install once regardless of PG versions
			if err := InstallDeps(pkg, ""); err != nil {
				logrus.Errorf("Failed to install deps for %s: %v", pkg, err)
			}
		}
	}

	return nil
}

// Install RPM build dependency for single package
func installRpmDep(pkg string, pgVersion string) error {
	specsDir := filepath.Join(config.HomeDir, "rpmbuild", "SPECS")
	if _, err := os.Stat(specsDir); os.IsNotExist(err) {
		return fmt.Errorf("specs directory not found: run 'pig build spec' first")
	}

	// Default PG version
	if pgVersion == "" {
		pgVersion = "16"
	}

	// Determine package name and PG version
	var pkgName string
	var pgVer string

	// Try as extension first
	if ext, err := ResolvePackage(pkg); err == nil {
		pkgName = ext.Pkg
		// Use extension's max PG version if available
		if len(ext.RpmPg) > 0 && pgVersion == "16" {
			pgVer = ext.RpmPg[0]
		} else {
			pgVer = pgVersion
		}
	} else {
		// Treat as normal package
		pkgName = pkg
		pgVer = "16"
	}

	specFile := filepath.Join(specsDir, pkgName+".spec")
	if _, err := os.Stat(specFile); os.IsNotExist(err) {
		logrus.Warnf("Spec file not found: %s (skipping)", specFile)
		return nil
	}

	// Install dependencies
	logrus.Infof("Installing deps for %s (PG%s)", pkgName, pgVer)
	cmd := []string{
		"dnf", "builddep", "-y",
		"--define", fmt.Sprintf("pgmajorversion %s", pgVer),
		specFile,
	}

	if err := utils.SudoCommand(cmd); err != nil {
		return fmt.Errorf("[FAIL] %s build dep missing: %v", pkgName, err)
	}

	logrus.Infof("[DONE] %s build dep complete", pkgName)
	return nil
}

// Install DEB build dependency for single package
func installDebDep(pkg string, pgVersion string) error {
	debDir := filepath.Join(config.HomeDir, "deb")
	if _, err := os.Stat(debDir); os.IsNotExist(err) {
		return fmt.Errorf("deb directory not found: run 'pig build spec' first")
	}

	// Convert package name
	debPkg := strings.ReplaceAll(pkg, "_", "-")
	controlFile := filepath.Join(debDir, debPkg, "debian", "control.in")

	// Try alternate location
	if _, err := os.Stat(controlFile); os.IsNotExist(err) {
		controlFile = filepath.Join(debDir, debPkg, "debian", "control.in1")
		if _, err := os.Stat(controlFile); os.IsNotExist(err) {
			logrus.Warnf("Control file not found for %s (skipping)", pkg)
			return nil
		}
	}

	// Extract and install dependencies
	content, err := os.ReadFile(controlFile)
	if err != nil {
		return fmt.Errorf("failed to read control file: %v", err)
	}

	var deps []string
	for _, line := range strings.Split(string(content), "\n") {
		if strings.HasPrefix(line, "Build-Depends:") {
			depLine := strings.TrimPrefix(line, "Build-Depends:")
			for _, dep := range strings.Split(depLine, ",") {
				dep = strings.TrimSpace(dep)
				// Remove version constraints
				if idx := strings.Index(dep, "("); idx > 0 {
					dep = strings.TrimSpace(dep[:idx])
				}
				if dep != "" && dep != "postgresql-all" && dep != "debhelper-compat" {
					deps = append(deps, dep)
				}
			}
			break
		}
	}

	if len(deps) > 0 {
		logrus.Infof("Installing %d dependencies for %s", len(deps), pkg)
		cmd := append([]string{"apt", "install", "-y"}, deps...)

		if err := utils.SudoCommand(cmd); err != nil {
			return fmt.Errorf("failed to install dependencies: %v", err)
		}
	}

	logrus.Info("✓ Dependencies installed")
	return nil
}