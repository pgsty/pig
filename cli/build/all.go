package build

import (
	"fmt"
	"os"
	"path/filepath"
	"pig/cli/ext"
	"pig/internal/config"
	"pig/internal/utils"
	"strings"

	"github.com/sirupsen/logrus"
)

const (
	// ArtifactDir is the directory for collecting build artifacts
	ArtifactDir = "/tmp/ext/pkg"
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

	// Setup artifact collection directory
	if err := setupArtifactDir(); err != nil {
		logrus.Errorf("Failed to setup artifact directory: %v", err)
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
	var artifacts []artifactInfo
	successCount := 0

	for _, ext := range validExts {
		extArtifacts, err := buildExtensionPipeline(ext, pgVers, withSymbol)
		if err != nil {
			logrus.Errorf("Failed to build %s: %v", ext.Name, err)
			continue
		}
		artifacts = append(artifacts, extArtifacts...)
		successCount++
	}

	// Display summary
	displayBuildSummary(successCount, len(validExts), artifacts)

	if successCount == 0 {
		return fmt.Errorf("all builds failed")
	}
	return nil
}

// artifactInfo holds information about a build artifact
type artifactInfo struct {
	Name string
	Path string
	Size int64
}

// setupArtifactDir creates the artifact collection directory
func setupArtifactDir() error {
	if err := os.MkdirAll(ArtifactDir, 0755); err != nil {
		return fmt.Errorf("failed to create artifact directory: %w", err)
	}
	return nil
}

// buildExtensionPipeline executes the complete build pipeline for a single extension
func buildExtensionPipeline(ext *ext.Extension, pgVersions []int, withSymbol bool) ([]artifactInfo, error) {
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

	// Step 1: Download source (MUST succeed)
	logrus.Infof("[1/3] Downloading source for %s", ext.Name)
	if err := DownloadCodeTarball([]string{ext.Name}, false); err != nil {
		return nil, fmt.Errorf("source download failed (required): %w", err)
	}

	// Step 2: Install dependencies (MAY fail)
	logrus.Infof("[2/3] Installing dependencies for %s", ext.Name)
	if err := InstallExtensionDeps([]string{ext.Name}, versionStr); err != nil {
		logrus.Warnf("Dependency installation failed (continuing): %v", err)
	}

	// Step 3: Build package (MUST succeed)
	logrus.Infof("[3/3] Building package for %s", ext.Name)
	if err := BuildPackage(ext.Name, versionStr, withSymbol); err != nil {
		return nil, fmt.Errorf("package build failed: %w", err)
	}

	// Collect artifacts
	artifacts := collectArtifacts(ext)

	logrus.Infof("Successfully built %s (%d artifacts)", ext.Name, len(artifacts))
	return artifacts, nil
}

// collectArtifacts finds and moves build artifacts to the collection directory
func collectArtifacts(ext *ext.Extension) []artifactInfo {
	var artifacts []artifactInfo

	// Determine build output directory based on OS type
	var searchDir string
	var pattern string

	switch config.OSType {
	case config.DistroEL:
		// RPM artifacts
		arch := getSystemArch()
		searchDir = filepath.Join(config.HomeDir, "rpmbuild", "RPMS", arch)
		pattern = fmt.Sprintf("*%s*.rpm", ext.Pkg)
	case config.DistroDEB:
		// DEB artifacts
		searchDir = filepath.Join(config.HomeDir, "deb", "pool")
		pattern = fmt.Sprintf("*%s*.deb", ext.Pkg)
	default:
		return artifacts
	}

	// Find matching files
	matches, err := filepath.Glob(filepath.Join(searchDir, pattern))
	if err != nil {
		logrus.Warnf("Failed to find artifacts: %v", err)
		return artifacts
	}

	// Move files to artifact directory
	for _, src := range matches {
		info, err := os.Stat(src)
		if err != nil {
			continue
		}

		dst := filepath.Join(ArtifactDir, filepath.Base(src))

		// Try to move (rename), fallback to copy
		if err := os.Rename(src, dst); err != nil {
			// Cross-device move, need to copy
			if err := copyFile(src, dst); err != nil {
				logrus.Warnf("Failed to collect %s: %v", filepath.Base(src), err)
				continue
			}
			// Remove source after successful copy
			os.Remove(src)
		}

		artifacts = append(artifacts, artifactInfo{
			Name: filepath.Base(dst),
			Path: dst,
			Size: info.Size(),
		})
	}

	return artifacts
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = destination.ReadFrom(source)
	return err
}

// displayBuildSummary shows a formatted summary of the build results
func displayBuildSummary(successCount, totalCount int, artifacts []artifactInfo) {
	logrus.Info(utils.PadHeader("Build Complete", 80))

	// Build status
	if successCount == totalCount {
		logrus.Infof("✓ Successfully built all %d extensions", totalCount)
	} else {
		logrus.Infof("⚠ Built %d/%d extensions", successCount, totalCount)
	}

	// Artifact listing
	if len(artifacts) > 0 {
		logrus.Info(utils.PadHeader("Build Artifacts", 80))
		logrus.Infof("Location: %s", ArtifactDir)
		logrus.Info("")

		// Display artifacts in a table format
		var totalSize int64
		for _, artifact := range artifacts {
			totalSize += artifact.Size
			sizeMB := float64(artifact.Size) / (1024 * 1024)
			logrus.Infof("  %-60s %8.2f MB", artifact.Name, sizeMB)
		}

		logrus.Info("")
		totalSizeMB := float64(totalSize) / (1024 * 1024)
		logrus.Infof("Total: %d files, %.2f MB", len(artifacts), totalSizeMB)
	}

	logrus.Info(utils.PadHeader("", 80))
}

// getSystemArch returns the system architecture
func getSystemArch() string {
	output, err := utils.ShellOutput("uname", "-m")
	if err != nil {
		return "x86_64"
	}
	return strings.TrimSpace(output)
}