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
	"pig/internal/utils"
	"sort"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// BuildExtension builds a single package (extension or normal)
func BuildExtension(pkg string, pgVersions string, withSymbol bool) error {
	if extension, err := ResolvePackage(pkg); err == nil {
		return buildPGExtension(extension, pgVersions, withSymbol)
	}
	return buildNormalPackage(pkg, withSymbol)
}

// BuildExtensions processes multiple packages
func BuildExtensions(packages []string, pgVersions string, withSymbol bool) error {
	if len(packages) == 0 {
		return fmt.Errorf("no packages specified")
	}

	for _, pkg := range packages {
		logrus.Info(strings.Repeat("=", 58))
		logrus.Infof("[BUILD EXT] %s", pkg)
		logrus.Info(strings.Repeat("=", 58))

		if err := BuildExtension(pkg, pgVersions, withSymbol); err != nil {
			logrus.Errorf("Failed to build %s: %v", pkg, err)
			// Continue with next package
		}
	}

	return nil
}

// RunBuildCommand executes a build command with single-line output display
func RunBuildCommand(cmd *exec.Cmd, logName string, append bool, metadata []string, pkgName string, pgVer int) (*BuildResult, error) {
	// Create build logger
	logger, err := NewBuildLogger(logName, append)
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
	done := make(chan error, 2)
	var lastLineShown bool

	// Process stdout
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			// Write to log file
			fmt.Fprintln(logger.logFile, line)

			// Display single line scrolling
			if trimmed := strings.TrimSpace(line); trimmed != "" {
				displayScrollingLine(trimmed, pgVer)
				lastLineShown = true
			}
		}
		done <- scanner.Err()
	}()

	// Process stderr
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			// Write to log file
			fmt.Fprintln(logger.logFile, line)

			// Display single line scrolling
			if trimmed := strings.TrimSpace(line); trimmed != "" {
				displayScrollingLine(trimmed, pgVer)
				lastLineShown = true
			}
		}
		done <- scanner.Err()
	}()

	// Wait for output processing
	for i := 0; i < 2; i++ {
		if err := <-done; err != nil && err != io.EOF {
			// Clear the scrolling line
			if lastLineShown {
				fmt.Print("\r\033[K")
			}
			// Write end marker to log
			logger.WriteMetadata("", "#[END]"+strings.Repeat("=", 52), "", "", "", "")
			return &BuildResult{
				Success: false,
				LogPath: logger.logPath,
			}, err
		}
	}

	// Wait for command completion
	err = cmd.Wait()

	// Clear the scrolling line
	if lastLineShown {
		fmt.Print("\r\033[K")
	}

	// Write end marker to log
	logger.WriteMetadata("", "#[END]"+strings.Repeat("=", 52), "", "", "", "")

	result := &BuildResult{
		Success: err == nil,
		LogPath: logger.logPath,
	}

	return result, err
}

// Build PG extension
func buildPGExtension(extension *ext.Extension, pgVersions string, withSymbol bool) error {
	// Validate build support
	if err := ValidateBuildExtension(extension); err != nil {
		return err
	}

	// Parse PG versions
	pgVers, err := ParsePGVersions(pgVersions)
	if err != nil {
		return err
	}

	// Get versions for this extension
	versions := GetPGVersionsForExtension(extension, pgVers)
	if len(versions) == 0 {
		return fmt.Errorf("no valid PG versions for %s", extension.Name)
	}

	// Sort versions from high to low
	sort.Sort(sort.Reverse(sort.IntSlice(versions)))

	// Print build info
	specFile := filepath.Join(config.HomeDir, "rpmbuild", "SPECS", extension.Pkg+".spec")
	logrus.Infof("spec : %s", specFile)

	// Read version from spec file
	if specVersion := getSpecVersion(specFile); specVersion != "" {
		logrus.Infof("ver  : %s", specVersion)
	}

	if extension.Source != "" {
		logrus.Infof("src  : %s", extension.Source)
	}

	logPath := filepath.Join(config.HomeDir, "ext", "log", extension.Pkg+".log")
	logrus.Infof("log  : %s", logPath)
	logrus.Infof("pg   : %s", intSliceToString(versions))
	logrus.Info(strings.Repeat("-", 58))

	// Build for each PG version
	successCount := 0
	for i, pgVer := range versions {
		// Show building status
		fmt.Printf("[PG%d]  Building %s...", pgVer, extension.Name)

		// Generate unique marker
		marker := fmt.Sprintf("%s_%d_%s", extension.Pkg, pgVer, time.Now().Format("20060102150405"))

		// Build
		result := buildExtensionForPG(extension, pgVer, specFile, withSymbol, i == 0, marker)

		// Clear line and print result
		fmt.Print("\r\033[K")
		if result.Success {
			successCount++
			if result.Artifact != "" && result.Size > 0 {
				logrus.Infof("[PG%d]  PASS %-6s %s", pgVer, formatSize(result.Size), result.Artifact)
			} else {
				logrus.Infof("[PG%d]  PASS", pgVer)
			}
		} else {
			logrus.Errorf("[PG%d]  FAIL grep -A60 -B1 %s %s", pgVer, result.Marker, logPath)
		}
	}

	// Summary
	if successCount == len(versions) {
		logrus.Infof("[DONE] PASS %d of %d packages generated", successCount, len(versions))
	} else {
		logrus.Warnf("[DONE] FAIL %d of %d packages generated, %d missing", successCount, len(versions), len(versions)-successCount)
	}

	return nil
}

// Build extension for specific PG version
func buildExtensionForPG(extension *ext.Extension, pgVer int, specFile string, withSymbol bool, isFirst bool, marker string) *BuildResult {
	// Set PATH
	envPATH := fmt.Sprintf("/usr/bin:/usr/pgsql-%d/bin:/root/.cargo/bin:/pg/bin:/usr/share/Modules/bin:/usr/lib64/ccache:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/root/bin:/home/vagrant/.cargo/bin", pgVer)

	// Build command
	args := []string{"rpmbuild", "--define", fmt.Sprintf("pgmajorversion %d", pgVer)}
	if !withSymbol {
		args = append(args, "--define", "debug_package %{nil}")
	}
	args = append(args, "-ba", specFile)

	// Prepare metadata for log
	metadata := []string{
		strings.Repeat("#", 58),
		marker,
		strings.Repeat("#", 58),
		fmt.Sprintf("PKG  : %s", extension.Pkg),
		fmt.Sprintf("PG   : %d", pgVer),
		fmt.Sprintf("TIME : %s", time.Now().Format("2006-01-02 15:04:05 -07")),
		fmt.Sprintf("PATH : %s", envPATH),
		fmt.Sprintf("CMD  : %s", strings.Join(args, " ")),
	}

	// Execute
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Env = append(os.Environ(), "PATH="+envPATH)

	// Set QA_RPATHS for EL10+
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

	// Find artifact
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

	return result
}

// Build normal (non-PG) package
func buildNormalPackage(pkgName string, withSymbol bool) error {
	// Try spec file first
	specFile := filepath.Join(config.HomeDir, "rpmbuild", "SPECS", pkgName+".spec")
	if _, err := os.Stat(specFile); err == nil {
		return buildWithSpec(pkgName, specFile, withSymbol)
	}

	// Try Makefile
	return buildWithMakefile(pkgName)
}

// Build using spec file
func buildWithSpec(pkgName, specFile string, withSymbol bool) error {
	// Generate marker
	marker := fmt.Sprintf("%s_%s", pkgName, time.Now().Format("20060102150405"))

	logrus.Infof("spec : %s", specFile)
	logPath := filepath.Join(config.HomeDir, "ext", "log", pkgName+".log")
	logrus.Infof("log  : %s", logPath)
	logrus.Info(strings.Repeat("-", 58))

	// Build command
	args := []string{"rpmbuild", "--define", "pgmajorversion 16"}
	if !withSymbol {
		args = append(args, "--define", "debug_package %{nil}")
	}
	args = append(args, "-ba", specFile)

	// Log metadata
	metadata := []string{
		strings.Repeat("#", 58),
		marker,
		strings.Repeat("#", 58),
		fmt.Sprintf("PKG  : %s", pkgName),
		fmt.Sprintf("TIME : %s", time.Now().Format("2006-01-02 15:04:05 -07")),
		fmt.Sprintf("PATH : %s", os.Getenv("PATH")),
		fmt.Sprintf("CMD  : %s", strings.Join(args, " ")),
	}

	// Show building status
	fmt.Printf("Building %s...", pkgName)

	// Execute
	cmd := exec.Command(args[0], args[1:]...)
	result, err := RunBuildCommand(cmd, pkgName+".log", false, metadata, pkgName, 0)
	result.Marker = marker

	// Clear line and show result
	fmt.Print("\r\033[K")
	if result.Success {
		arch := getArch()
		pattern := filepath.Join(config.HomeDir, "rpmbuild", "RPMS", arch, pkgName+"*.rpm")
		if files, _ := filepath.Glob(pattern); len(files) > 0 {
			for _, file := range files {
				if info, _ := os.Stat(file); info != nil {
					logrus.Infof("[DONE] PASS %-6s %s", formatSize(info.Size()), file)
				}
			}
		}
	} else {
		logrus.Errorf("[DONE] FAIL grep -A60 -B1 %s %s", marker, logPath)
	}

	return err
}

// Build using Makefile
func buildWithMakefile(taskName string) error {
	// Generate marker
	marker := fmt.Sprintf("%s_%s", taskName, time.Now().Format("20060102150405"))

	// Find Makefile
	var makefilePath string
	if config.OSType == config.DistroEL {
		makefilePath = filepath.Join(config.HomeDir, "rpmbuild", "Makefile")
	} else if config.OSType == config.DistroDEB {
		makefilePath = filepath.Join(config.HomeDir, "deb", "Makefile")
	} else {
		return fmt.Errorf("unsupported OS type")
	}

	if _, err := os.Stat(makefilePath); os.IsNotExist(err) {
		return fmt.Errorf("no spec file or Makefile found for %s", taskName)
	}

	logrus.Infof("make : %s", makefilePath)
	logPath := filepath.Join(config.HomeDir, "ext", "log", taskName+".log")
	logrus.Infof("log  : %s", logPath)
	logrus.Info(strings.Repeat("-", 58))

	// Log metadata
	metadata := []string{
		strings.Repeat("#", 58),
		marker,
		strings.Repeat("#", 58),
		fmt.Sprintf("PKG  : %s", taskName),
		fmt.Sprintf("TIME : %s", time.Now().Format("2006-01-02 15:04:05 -07")),
		fmt.Sprintf("MAKE : %s", makefilePath),
		fmt.Sprintf("CMD  : make %s", taskName),
	}

	// Show building status
	fmt.Printf("Building %s (Makefile)...", taskName)

	// Execute
	cmd := exec.Command("make", taskName)
	cmd.Dir = filepath.Dir(makefilePath)

	result, err := RunBuildCommand(cmd, taskName+".log", false, metadata, taskName, 0)
	result.Marker = marker

	// Clear line and show result
	fmt.Print("\r\033[K")
	if result.Success {
		logrus.Infof("[DONE] PASS")
	} else {
		logrus.Errorf("[DONE] FAIL grep -A60 -B1 %s %s", marker, logPath)
	}

	return err
}

// Helper functions
func formatSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%dB", size)
	} else if size < 1024*1024 {
		return fmt.Sprintf("%dKB", size/1024)
	} else {
		return fmt.Sprintf("%.1fMB", float64(size)/(1024*1024))
	}
}

func getSpecVersion(specFile string) string {
	content, err := os.ReadFile(specFile)
	if err != nil {
		return ""
	}

	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Version:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				return parts[1]
			}
		}
	}
	return ""
}

func getArch() string {
	output, err := utils.ShellOutput("uname", "-m")
	if err != nil {
		return "x86_64"
	}
	return strings.TrimSpace(output)
}

func intSliceToString(slice []int) string {
	strs := make([]string, len(slice))
	for i, v := range slice {
		strs[i] = fmt.Sprintf("%d", v)
	}
	return strings.Join(strs, " ")
}
