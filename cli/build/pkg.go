package build

import (
	"fmt"
	"os"
	"path/filepath"
	"pig/cli/ext"
	"pig/internal/config"
	"pig/internal/utils"
	"sort"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
)

// BuildPackage builds packages for specified extension
func BuildPackage(pkgName string, pgVersions string, withSymbol bool) error {
	// Route to appropriate build system based on OS type
	switch config.OSType {
	case config.DistroEL:
		return buildRpmPackage(pkgName, pgVersions, withSymbol)
	case config.DistroDEB:
		return buildDebPackage(pkgName, pgVersions)
	case config.DistroMAC:
		return fmt.Errorf("macOS package building not supported")
	default:
		return fmt.Errorf("unsupported operating system: %s", config.OSType)
	}
}

// buildRpmPackage builds RPM packages for EL systems
func buildRpmPackage(pkgName string, pgVersions string, withSymbol bool) error {
	// Extension catalog should already be loaded (global singleton)
	// Just ensure it's loaded for safety
	ext.Catalog.LoadAliasMap(config.OSType)

	// Translate package name (similar to ext add logic)
	extName := translatePkgName(pkgName)

	// Find extension in catalog
	extension, exists := ext.Catalog.ExtNameMap[extName]
	if !exists {
		// Try alias map
		if aliasExt, ok := ext.Catalog.ExtPkgMap[extName]; ok {
			extension = aliasExt
		} else {
			return fmt.Errorf("extension %s not found in catalog", extName)
		}
	}

	// Check if extension supports RPM build
	if extension.RpmRepo == "" || extension.Source == "" {
		logrus.Warnf("extension %s does not support RPM build", extension.Name)
		return fmt.Errorf("extension %s does not support RPM build", extension.Name)
	}

	// Determine PG versions to build
	var pgVers []int
	if pgVersions != "" {
		// Use provided versions
		for _, v := range strings.Split(pgVersions, ",") {
			ver, err := strconv.Atoi(strings.TrimSpace(v))
			if err != nil {
				return fmt.Errorf("invalid PG version: %s", v)
			}
			pgVers = append(pgVers, ver)
		}
	} else {
		// Use extension's default PgVer versions
		if len(extension.PgVer) == 0 {
			return fmt.Errorf("no PG versions specified and extension has no RPM_PG field")
		}
		for _, v := range extension.PgVer {
			ver, err := strconv.Atoi(v)
			if err != nil {
				logrus.Warnf("invalid PG version in RPM_PG: %s", v)
				continue
			}
			pgVers = append(pgVers, ver)
		}
	}

	if len(pgVers) == 0 {
		return fmt.Errorf("no valid PG versions to build")
	}

	// Sort versions from low to high
	sort.Ints(pgVers)

	// Check spec file exists
	homeDir := config.HomeDir
	specFile := filepath.Join(homeDir, "rpmbuild", "SPECS", fmt.Sprintf("%s.spec", extension.Pkg))
	if _, err := os.Stat(specFile); os.IsNotExist(err) {
		return fmt.Errorf("spec file not found: %s", specFile)
	}

	logrus.Infof("Building %s for PG versions: %v", extension.Name, pgVers)
	logrus.Infof("Using spec file: %s", specFile)

	// Print spec file info
	printSpecInfo(specFile)

	// Build for each PG version
	for _, pgVer := range pgVers {
		if err := buildForPgVersion(extension, pgVer, specFile, withSymbol); err != nil {
			logrus.Errorf("Failed to build %s for PG%d: %v", extension.Name, pgVer, err)
			return err
		}
	}

	// List build artifacts
	listBuildArtifacts(extension.Pkg, homeDir)

	return nil
}

// translatePkgName translates user input to standard package name
func translatePkgName(input string) string {
	// Remove common prefixes
	input = strings.TrimPrefix(input, "pg_")
	input = strings.TrimPrefix(input, "pg-")
	input = strings.TrimPrefix(input, "postgresql-")

	// Handle version suffixes (e.g., pg_stat_kcache_17)
	parts := strings.Split(input, "_")
	if len(parts) > 1 {
		lastPart := parts[len(parts)-1]
		if _, err := strconv.Atoi(lastPart); err == nil {
			// Last part is a number, remove it
			input = strings.Join(parts[:len(parts)-1], "_")
		}
	}

	// Convert hyphens to underscores for consistency
	input = strings.ReplaceAll(input, "-", "_")

	return input
}

// printSpecInfo prints basic info from spec file
func printSpecInfo(specFile string) {
	logrus.Info(utils.PadHeader(fmt.Sprintf("%s spec file", filepath.Base(specFile[:len(specFile)-5])), 80))

	// Read spec file and extract key fields
	content, err := os.ReadFile(specFile)
	if err != nil {
		logrus.Warnf("Failed to read spec file: %v", err)
		return
	}

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Version:") ||
			strings.HasPrefix(line, "Release:") ||
			strings.HasPrefix(line, "Summary:") ||
			strings.HasPrefix(line, "License:") ||
			strings.HasPrefix(line, "URL:") ||
			strings.HasPrefix(line, "Source0:") {
			logrus.Info(line)
		}
	}
}

// buildForPgVersion builds the RPM package for a specific PG version
func buildForPgVersion(extension *ext.Extension, pgVer int, specFile string, withSymbol bool) error {
	logrus.Info(utils.PadHeader(fmt.Sprintf("%s for PG%d", extension.Name, pgVer), 80))

	// Set PATH environment variable
	os.Setenv("PATH", fmt.Sprintf("/usr/bin:/usr/pgsql-%d/bin:/root/.cargo/bin:/pg/bin:/usr/share/Modules/bin:/usr/lib64/ccache:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/root/bin:/home/vagrant/.cargo/bin", pgVer))

	// Build rpmbuild command
	args := []string{"rpmbuild"}

	// Add PG version macro
	args = append(args, "--define", fmt.Sprintf("pgmajorversion %d", pgVer))

	// Control debug package generation
	if !withSymbol {
		// Disable debug package generation
		args = append(args, "--define", "debug_package %{nil}")
	}

	// Add spec file
	args = append(args, "-ba", specFile)

	// Print command
	logrus.Infof("$ %s", strings.Join(args, " "))

	// Execute command
	if err := utils.Command(args); err != nil {
		return fmt.Errorf("rpmbuild failed: %w", err)
	}

	logrus.Infof("Successfully built %s for PG%d", extension.Name, pgVer)
	return nil
}

// listBuildArtifacts lists the RPM files generated
func listBuildArtifacts(pkgName string, homeDir string) {
	arch := getArch()
	rpmsDir := filepath.Join(homeDir, "rpmbuild", "RPMS", arch)

	logrus.Info(utils.PadHeader(fmt.Sprintf("%s rpms", pkgName), 80))
	logrus.Infof("Build artifacts in: %s", rpmsDir)

	// List files matching the package name
	files, err := filepath.Glob(filepath.Join(rpmsDir, fmt.Sprintf("*%s*.rpm", pkgName)))
	if err != nil {
		logrus.Warnf("Failed to list RPM files: %v", err)
		return
	}

	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			continue
		}
		logrus.Infof("  %s (%.2f MB)", filepath.Base(file), float64(info.Size())/(1024*1024))
	}

	logrus.Info(utils.PadHeader(fmt.Sprintf("%s done", pkgName), 80))
}

// buildDebPackage builds DEB packages for Debian/Ubuntu systems (stub)
func buildDebPackage(pkgName string, pgVersions string) error {
	// Work directory for Debian builds
	workDir := config.HomeDir + "/deb/"
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		return fmt.Errorf("deb directory not found at %s, please run `pig build spec` first", workDir)
	}

	// Extension catalog should already be loaded
	ext.Catalog.LoadAliasMap(config.OSType)

	// Translate package name
	extName := translatePkgName(pkgName)

	// Find extension in catalog
	extension, exists := ext.Catalog.ExtNameMap[extName]
	if !exists {
		if aliasExt, ok := ext.Catalog.ExtPkgMap[extName]; ok {
			extension = aliasExt
		} else {
			return fmt.Errorf("extension %s not found in catalog", extName)
		}
	}

	// Check if extension supports DEB build
	if extension.DebRepo == "" || extension.Source == "" {
		logrus.Warnf("extension %s does not support DEB build", extension.Name)
		return fmt.Errorf("extension %s does not support DEB build", extension.Name)
	}

	// Change to work directory
	os.Chdir(workDir)

	logrus.Infof("Building DEB package for %s in %s", extension.Name, workDir)
	logrus.Infof("################ %s build begin in %s", extension.Pkg, workDir)

	// Execute make command for the package
	if err := utils.Command([]string{"make", extension.Pkg}); err != nil {
		logrus.Errorf("################ %s build failed: %v", extension.Pkg, err)
		return fmt.Errorf("failed to build DEB package for %s: %w", extension.Name, err)
	}

	logrus.Infof("################ %s build success", extension.Pkg)
	return nil
}

// getArch returns the current system architecture
func getArch() string {
	output, err := utils.ShellOutput("uname", "-m")
	if err != nil {
		return "x86_64" // default
	}
	return strings.TrimSpace(output)
}
