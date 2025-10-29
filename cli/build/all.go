package build

import (
	"fmt"
	"pig/cli/ext"
	"pig/internal/utils"
	"strings"

	"github.com/sirupsen/logrus"
)

// BuildAll performs complete build pipeline: get source, install deps, build package
func BuildAll(pkgs []string, pgVersions string, withSymbol bool) error {
	// Validate and resolve all packages upfront
	extensions, err := ResolvePackages(pkgs)
	if err != nil {
		return err
	}

	// Parse PG versions once
	pgVers, err := ParsePGVersions(pgVersions)
	if err != nil {
		return err
	}

	// Validate all extensions can be built
	var validExts []*ext.Extension
	for _, ext := range extensions {
		if err := ValidateBuildExtension(ext); err != nil {
			logrus.Warnf("Skipping %s: %v", ext.Name, err)
			continue
		}
		validExts = append(validExts, ext)
	}

	if len(validExts) == 0 {
		return fmt.Errorf("no valid extensions to build")
	}

	// Log build plan
	names := make([]string, len(validExts))
	for i, e := range validExts {
		names[i] = e.Name
	}
	logrus.Infof("Building extensions: %s", strings.Join(names, ", "))
	if pgVersions != "" {
		logrus.Infof("Using PG versions: %s", pgVersions)
	}

	// Execute pipeline for each extension
	successCount := 0
	for _, ext := range validExts {
		if err := buildExtensionPipeline(ext, pgVers, withSymbol); err != nil {
			logrus.Errorf("Failed to build %s: %v", ext.Name, err)
			continue
		}
		successCount++
	}

	// Summary
	logrus.Info(utils.PadHeader("Build Complete", 80))
	logrus.Infof("Successfully built %d/%d extensions", successCount, len(validExts))

	if successCount == 0 {
		return fmt.Errorf("all builds failed")
	}
	return nil
}

// buildExtensionPipeline executes the complete build pipeline for a single extension
func buildExtensionPipeline(ext *ext.Extension, pgVersions []int, withSymbol bool) error {
	logrus.Info(utils.PadHeader(fmt.Sprintf("Building %s", ext.Name), 80))

	// Get PG versions for this extension
	versions := GetPGVersionsForExtension(ext, pgVersions)
	versionStr := ""
	if len(versions) > 0 {
		strs := make([]string, len(versions))
		for i, v := range versions {
			strs[i] = fmt.Sprintf("%d", v)
		}
		versionStr = strings.Join(strs, ",")
	}

	// Step 1: Download source
	logrus.Infof("[1/3] Downloading source for %s", ext.Name)
	if err := DownloadCodeTarball([]string{ext.Name}, false); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	// Step 2: Install dependencies
	logrus.Infof("[2/3] Installing dependencies for %s", ext.Name)
	if err := InstallExtensionDeps([]string{ext.Name}, versionStr); err != nil {
		return fmt.Errorf("dependency installation failed: %w", err)
	}

	// Step 3: Build package
	logrus.Infof("[3/3] Building package for %s", ext.Name)
	if err := BuildPackage(ext.Name, versionStr, withSymbol); err != nil {
		return fmt.Errorf("package build failed: %w", err)
	}

	logrus.Infof("Successfully built %s", ext.Name)
	return nil
}
