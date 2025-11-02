// Package build provides functions to build PostgreSQL extensions and packages
package build

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"pig/cli/ext"
	"pig/internal/config"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// BuildExtensions processes multiple packages
func BuildExtensions(packages []string, pgVersions string, debugPkg bool) error {
	if len(packages) == 0 {
		return fmt.Errorf("no packages specified")
	}
	for _, pkg := range packages {
		if err := BuildExtension(pkg, pgVersions, debugPkg); err != nil {
			logrus.Errorf("Failed to build %s: %v", pkg, err)
		}
	}
	return nil
}

// BuildExtension is the main entry point for building a single package
func BuildExtension(pkg string, pgVersions string, debugPkg bool) error {
	if _, err := resolveExtension(pkg); err != nil {
		return BuildMake(pkg, pgVersions, debugPkg)
	}

	// run extension builder for postgres extension
	builder, err := NewExtensionBuilder(pkg)
	if err != nil {
		return err
	}
	builder.DebugPackage = debugPkg
	if err = builder.UpdateVersion(pgVersions); err != nil {
		return err
	}
	return builder.Build()
}

// BuildMake builds packages using Makefile
func BuildMake(pkg string, pgVersions string, debugPkg bool) error {
	// Print header
	headerWidth := 58
	logrus.Info(strings.Repeat("=", headerWidth))
	logrus.Infof("[BUILD MAKE] %s", pkg)
	logrus.Info(strings.Repeat("=", headerWidth))

	// Determine build directory based on OS type
	var makeDir string
	switch config.OSType {
	case "rpm":
		makeDir = filepath.Join(config.HomeDir, "rpmbuild")
	case "deb":
		makeDir = filepath.Join(config.HomeDir, "debbuild")
	default:
		return fmt.Errorf("unsupported OS type for make build: %s", config.OSType)
	}

	// Check if Makefile exists
	makeFile := filepath.Join(makeDir, "Makefile")
	if _, err := os.Stat(makeFile); err != nil {
		return fmt.Errorf("Makefile not found at %s", makeFile)
	}

	logrus.Infof("path   : %s", makeFile)
	logrus.Infof("target : %s", pkg)
	logrus.Info(strings.Repeat("-", headerWidth))

	// Setup logging
	logDir := filepath.Join(config.HomeDir, "ext", "log")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log dir: %w", err)
	}

	logPath := filepath.Join(logDir, pkg+".log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}
	defer logFile.Close()

	// Write metadata to log
	startTime := time.Now()
	metadata := []string{
		fmt.Sprintf("BUILD: %s", pkg),
		fmt.Sprintf("TIME : %s", startTime.Format("2006-01-02 15:04:05 -07")),
		fmt.Sprintf("DIR  : %s", makeDir),
		fmt.Sprintf("CMD  : make %s", pkg),
		strings.Repeat("=", headerWidth),
	}
	for _, line := range metadata {
		fmt.Fprintln(logFile, line)
	}

	// Execute make command
	cmd := exec.Command("make", pkg)
	cmd.Dir = makeDir
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	// Run command
	err = cmd.Run()
	duration := time.Since(startTime)

	// Report result
	if err != nil {
		logrus.Errorf("[MAKE] FAIL %s (%.1fs)", err.Error(), duration.Seconds())
		return fmt.Errorf("make build failed: %w", err)
	}

	logrus.Infof("[MAKE] PASS Build completed successfully (%.1fs)", duration.Seconds())
	return nil
}

// Helper functions (now mostly moved to builder.go)

func resolveExtension(pkg string) (*ext.Extension, error) {
	// Try by name first
	if e, found := ext.Catalog.ExtNameMap[pkg]; found {
		return e, nil
	}
	// Try by package name
	if e, found := ext.Catalog.ExtPkgMap[pkg]; found {
		return e, nil
	}
	return nil, fmt.Errorf("extension not found: %s", pkg)
}
