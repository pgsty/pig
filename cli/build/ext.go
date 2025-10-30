// Package build provides functions to build PostgreSQL extensions and packages
package build

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"pig/cli/ext"
	"pig/internal/config"
	"runtime"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// BuildExtension is the main entry point for building a single package
func BuildExtension(pkg string, pgVersions string, debugPkg bool) error {
	// Validate PG versions if specified
	if pgVersions != "" {
		if _, err := ParsePGVersions(pgVersions); err != nil {
			return fmt.Errorf("invalid PG version spec: %v", err)
		}
	}

	// Check if it's a PostgreSQL extension
	var isExtension bool
	if _, found := ext.Catalog.ExtNameMap[pkg]; found {
		isExtension = true
	} else if _, found := ext.Catalog.ExtPkgMap[pkg]; found {
		isExtension = true
	}

	// Route to appropriate build function
	if !isExtension {
		return BuildMake(pkg, pgVersions, debugPkg)
	}

	// For extensions, route based on OS type
	switch config.OSType {
	case "rpm":
		return BuildRPM(pkg, pgVersions, debugPkg)
	case "deb":
		return BuildDEB(pkg, pgVersions, debugPkg)
	default:
		return fmt.Errorf("unsupported OS type: %s", config.OSType)
	}
}

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

// BuildRPM builds RPM packages for PostgreSQL extensions
func BuildRPM(pkg string, pgVersions string, debugPkg bool) error {
	// Print header
	logrus.Info(strings.Repeat("=", 58))
	logrus.Infof("[BUILD RPM] %s", pkg)
	logrus.Info(strings.Repeat("=", 58))

	// Resolve extension
	extension, err := resolveExtension(pkg)
	if err != nil {
		return err
	}

	// Parse PG versions
	var versions []int
	if pgVersions != "" {
		// Use explicitly provided versions
		versions, err = ParsePGVersions(pgVersions)
		if err != nil {
			return err
		}
	} else {
		// Use extension's default versions
		versions = extension.GetPGVersions()
	}

	// Check spec file
	specPath := filepath.Join(config.HomeDir, "rpmbuild", "SPECS", extension.Pkg+".spec")
	if _, err := os.Stat(specPath); err != nil {
		return fmt.Errorf("spec file not found: %s", specPath)
	}

	// Print build info
	logrus.Infof("spec : %s", specPath)
	logrus.Infof("ver  : %s", extension.Version)
	logrus.Infof("src  : %s-%s.tar.gz", extension.Name, extension.Version)
	logrus.Infof("log  : /home/vagrant/ext/log/%s.log", extension.Pkg)
	logrus.Infof("pg   : %s", formatPGVersions(versions))
	logrus.Info(strings.Repeat("-", 58))

	// Build for each PG version
	successCount := 0
	for i, pgVer := range versions {
		result := buildRPMForPG(extension, pgVer, debugPkg, i == 0)
		if result.Success {
			successCount++
			logrus.Infof("[PG%d]  PASS %s    %s", pgVer,
				formatSize(result.Size), result.Artifact)
		} else {
			logrus.Errorf("[PG%d]  FAIL %s", pgVer, result.Output)
		}
	}

	// Print summary
	totalCount := len(versions)
	if successCount < totalCount {
		logrus.Warnf("[DONE] FAIL %d of %d packages generated, %d missing",
			successCount, totalCount, totalCount-successCount)
	} else {
		logrus.Infof("[DONE] PASS all %d packages generated", totalCount)
	}

	return nil
}

// BuildDEB builds DEB packages for PostgreSQL extensions
func BuildDEB(pkg string, pgVersions string, debugPkg bool) error {
	// Print header
	logrus.Info(strings.Repeat("=", 58))
	logrus.Infof("[BUILD DEB] %s", pkg)
	logrus.Info(strings.Repeat("=", 58))

	// Resolve extension
	extension, err := resolveExtension(pkg)
	if err != nil {
		return err
	}

	// Parse PG versions
	var versions []int
	if pgVersions != "" {
		// Use explicitly provided versions
		versions, err = ParsePGVersions(pgVersions)
		if err != nil {
			return err
		}
	} else {
		// Use extension's default versions
		versions = extension.GetPGVersions()
	}

	logrus.Infof("Building DEB for %s on PG %v", extension.Name, versions)

	// TODO: Implement actual DEB building logic
	// For now, just return an error indicating it's not implemented
	return fmt.Errorf("DEB building not yet implemented")
}

// BuildMake builds packages using Makefile
func BuildMake(pkg string, pgVersions string, debugPkg bool) error {
	// Print header
	logrus.Info(strings.Repeat("=", 58))
	logrus.Infof("[BUILD MAKE] %s", pkg)
	logrus.Info(strings.Repeat("=", 58))

	// Determine build directory and target based on OS type
	var makeDir string
	var makeTarget string

	switch config.OSType {
	case "rpm":
		makeDir = filepath.Join(config.HomeDir, "rpmbuild")
		makeTarget = pkg  // Use package name as make target
	case "deb":
		makeDir = filepath.Join(config.HomeDir, "deb")
		makeTarget = pkg  // Use package name as make target
	default:
		return fmt.Errorf("unsupported OS type for make build: %s", config.OSType)
	}

	// Check if Makefile exists in the build directory
	makeFile := filepath.Join(makeDir, "Makefile")
	if _, err := os.Stat(makeFile); err != nil {
		return fmt.Errorf("Makefile not found at %s", makeFile)
	}

	logrus.Infof("path : %s", makeFile)
	logrus.Infof("target : %s", makeTarget)
	logrus.Info(strings.Repeat("-", 58))

	// Execute make command with the package name as target
	cmd := exec.Command("make", makeTarget)
	cmd.Dir = makeDir

	// Create build logger
	logFileName := pkg + ".log"
	logger, err := NewBuildLogger(logFileName, false)
	if err != nil {
		return err
	}
	defer logger.Close()

	// Run the build command
	marker := fmt.Sprintf("%s_%s", pkg, time.Now().Format("20060102150405"))
	metadata := []string{
		fmt.Sprintf("BUILD: %s", pkg),
		fmt.Sprintf("TIME : %s", time.Now().Format("2006-01-02 15:04:05 -07")),
		fmt.Sprintf("DIR  : %s", makeDir),
		fmt.Sprintf("CMD  : make %s", makeTarget),
	}

	result, err := RunBuildCommand(cmd, logFileName, false, metadata, pkg, 0)
	if err != nil {
		logrus.Errorf("Failed to run make: %v", err)
		return err
	}
	result.Marker = marker

	if result.Success {
		logrus.Infof("[MAKE] PASS Build completed successfully")
		if result.Artifact != "" {
			logrus.Infof("Artifact: %s", result.Artifact)
		}
	} else {
		logrus.Errorf("[MAKE] FAIL %s", result.Output)
	}

	return nil
}

// Helper function to build RPM for a specific PG version
func buildRPMForPG(extension *ext.Extension, pgVer int, debugPkg bool, isFirst bool) *BuildResult {
	// Display progress
	fmt.Printf("[PG%d]  Building %s...", pgVer, extension.Name)

	// Construct rpmbuild command
	args := []string{
		"rpmbuild", "-ba",
		"--define", fmt.Sprintf("pgmajorversion %d", pgVer),
		"--define", fmt.Sprintf("pginstdir /usr/pgsql-%d", pgVer),
		"--define", fmt.Sprintf("pgpackageversion %d", pgVer),
	}

	if !debugPkg {
		args = append(args, "--define", "debug_package %{nil}")
	}

	specPath := filepath.Join(config.HomeDir, "rpmbuild", "SPECS", extension.Pkg+".spec")
	args = append(args, specPath)

	// Prepare environment
	envPATH := fmt.Sprintf("/usr/pgsql-%d/bin:/usr/local/bin:/usr/bin:/bin", pgVer)

	// Create unique marker for log searching
	marker := fmt.Sprintf("%s_%d_%s", extension.Pkg, pgVer,
		time.Now().Format("20060102150405"))

	// Prepare metadata for logging
	metadata := []string{
		fmt.Sprintf("BUILD: %s PG%d", extension.Name, pgVer),
		fmt.Sprintf("SPEC : %s", specPath),
		fmt.Sprintf("TIME : %s", time.Now().Format("2006-01-02 15:04:05 -07")),
		fmt.Sprintf("PATH : %s", envPATH),
		fmt.Sprintf("CMD  : %s", strings.Join(args, " ")),
	}

	// Execute build command
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Env = append(os.Environ(), "PATH="+envPATH)

	// Special case for EL10
	if config.OSVersionCode == "el10" {
		cmd.Env = append(cmd.Env, "QA_RPATHS=3")
	}

	logFileName := extension.Pkg + ".log"
	result, err := RunBuildCommand(cmd, logFileName, !isFirst, metadata, extension.Name, pgVer)
	if err != nil {
		logrus.Errorf("Failed to run build command: %v", err)
		return &BuildResult{
			Marker:  marker,
			Success: false,
			Output:  err.Error(),
		}
	}
	result.Marker = marker

	// Find artifact if build succeeded
	if result.Success {
		arch := getArch()
		rpmsDir := filepath.Join(config.HomeDir, "rpmbuild", "RPMS", arch)
		pattern := filepath.Join(rpmsDir, fmt.Sprintf("%s_%d*.rpm", extension.Pkg, pgVer))

		if files, err := filepath.Glob(pattern); err == nil && len(files) > 0 {
			result.Artifact = files[0]
			if info, err := os.Stat(files[0]); err == nil {
				result.Size = info.Size()
			}
		}
	}

	// Clear the progress line
	fmt.Print("\r\033[K")

	return result
}

// Helper functions

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

func formatPGVersions(versions []int) string {
	parts := make([]string, len(versions))
	for i, v := range versions {
		parts[i] = fmt.Sprintf("%d", v)
	}
	return strings.Join(parts, " ")
}

func formatSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%dB", size)
	} else if size < 1024*1024 {
		return fmt.Sprintf("%dKB", size/1024)
	} else {
		return fmt.Sprintf("%dMB", size/(1024*1024))
	}
}

func getArch() string {
	arch := runtime.GOARCH
	switch arch {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	default:
		return arch
	}
}

// RunBuildCommand executes a build command with real-time output processing
func RunBuildCommand(cmd *exec.Cmd, logName string, appendMode bool, metadata []string, pkgName string, pgVer int) (*BuildResult, error) {
	// Create build logger
	logger, err := NewBuildLogger(logName, appendMode)
	if err != nil {
		return nil, err
	}
	defer logger.Close()

	// Write metadata to log
	if len(metadata) > 0 {
		for _, line := range metadata {
			logger.WriteMetadata(line)
		}
		logger.WriteMetadata(strings.Repeat("=", 58))
	}

	// Create pipes
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start command
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start command: %w", err)
	}

	// Process output
	result := &BuildResult{
		Success: true,
		LogPath: logger.logPath,
	}

	// Create readers
	stdoutReader := bufio.NewReader(stdout)
	stderrReader := bufio.NewReader(stderr)

	// Channel to signal completion
	doneChan := make(chan bool, 2)

	// Read stdout
	go func() {
		for {
			line, err := stdoutReader.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					logrus.Debugf("stdout read error: %v", err)
				}
				break
			}
			line = strings.TrimRight(line, "\n\r")
			logger.WriteMetadata(line)

			// Display progress line with clear (only show for PG versions > 0)
			if pgVer > 0 {
				fmt.Printf("\r\033[K[PG%d]  %s", pgVer, truncateLine(line, 60))
			}
		}
		doneChan <- true
	}()

	// Read stderr
	go func() {
		var errorOutput []string
		for {
			line, err := stderrReader.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					logrus.Debugf("stderr read error: %v", err)
				}
				break
			}
			line = strings.TrimRight(line, "\n\r")
			errorOutput = append(errorOutput, line)
			logger.WriteMetadata(line)

			// Check for build errors
			if strings.Contains(line, "error:") || strings.Contains(line, "Error:") {
				result.Success = false
				if len(result.Output) == 0 {
					result.Output = line
				}
			}
		}

		// If there was stderr output and command failed, use it as output
		if !result.Success && len(errorOutput) > 0 && result.Output == "" {
			result.Output = strings.Join(errorOutput, " ")
		}
		doneChan <- true
	}()

	// Wait for readers to finish
	<-doneChan
	<-doneChan

	// Wait for command to complete
	if err := cmd.Wait(); err != nil {
		result.Success = false
		if result.Output == "" {
			result.Output = err.Error()
		}
	}

	return result, nil
}

// truncateLine truncates a line to specified length
func truncateLine(line string, maxLen int) string {
	if len(line) <= maxLen {
		return line
	}
	return line[:maxLen-3] + "..."
}
