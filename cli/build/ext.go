package build

import (
	"fmt"
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

// Build package (extension or normal package)
func BuildExtension(pkgName string, pgVersions string, withSymbol bool) error {
	// Try as PG extension
	if ext, err := ResolvePackage(pkgName); err == nil {
		return buildPGExtension(ext, pgVersions, withSymbol)
	}

	// Build as normal package
	return buildNormalPackage(pkgName, withSymbol)
}

// buildPGExtension builds PG extension packages
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

	// Sort versions from high to low for display
	sort.Sort(sort.Reverse(sort.IntSlice(versions)))

	// Print header
	logrus.Info(strings.Repeat("=", 58))
	logrus.Infof("[BUILDING] %s", extension.Name)
	logrus.Info(strings.Repeat("=", 58))

	// Print build info
	specFile := filepath.Join(config.HomeDir, "rpmbuild", "SPECS", fmt.Sprintf("%s.spec", extension.Pkg))
	logrus.Infof("spec : %s", specFile)

	// Read version from spec file if possible
	specVersion := getSpecVersion(specFile)
	if specVersion != "" {
		logrus.Infof("ver  : %s", specVersion)
	}

	if extension.Source != "" {
		logrus.Infof("src  : %s", extension.Source)
	}

	logPath := filepath.Join(config.HomeDir, "ext", "log", fmt.Sprintf("%s.log", extension.Pkg))
	logrus.Infof("log  : %s", logPath)

	// Display PG versions
	versionStrs := make([]string, len(versions))
	for i, v := range versions {
		versionStrs[i] = fmt.Sprintf("%d", v)
	}
	logrus.Infof("pg   : %s", strings.Join(versionStrs, " "))
	logrus.Info(strings.Repeat("-", 58))

	// Write header to log file
	writeInitialLogHeader(extension, specFile, specVersion, versionStrs)

	// Build for each PG version
	results := make(map[int]*BuildResult)
	successCount := 0
	for i, pgVer := range versions {
		// Show building status (will be overwritten by final result)
		fmt.Printf("[PG%d]  Building %s...", pgVer, extension.Name)

		// Build without showing header
		isFirst := i == 0
		result := buildForPgVersion(extension, pgVer, specFile, withSymbol, isFirst)
		results[pgVer] = result

		// Clear line and print final result
		fmt.Print("\r\033[K")
		if result.Success {
			successCount++
			if result.Artifact != "" && result.Size > 0 {
				logrus.Infof("[PG%d]  PASS %-6s %s", pgVer, formatSize(result.Size), result.Artifact)
			} else {
				logrus.Infof("[PG%d]  PASS", pgVer)
			}
		} else {
			logrus.Errorf("[PG%d] FAIL grep -A60 -B1 %s %s", pgVer, result.Marker, logPath)
		}
	}

	// if success = total
	if successCount == len(versions) {
		logrus.Infof("[DONE] PASS %d of %d packages generated", successCount, len(versions))
	} else {
		logrus.Warnf("[DONE] FAIL %d of %d packages generated, %d missing", successCount, len(versions), len(versions)-successCount)
	}
	logrus.Info(strings.Repeat("=", 58))
	return nil
}

// buildForPgVersion builds the RPM package for a specific PG version
func buildForPgVersion(extension *ext.Extension, pgVer int, specFile string, withSymbol bool, isFirst bool) *BuildResult {
	// Generate unique marker: pkgName_pgVer_timestamp
	marker := fmt.Sprintf("%s_%d_%s", extension.Pkg, pgVer, time.Now().Format("20060102150405"))

	// Set PATH environment variable
	envPATH := fmt.Sprintf("/usr/bin:/usr/pgsql-%d/bin:/root/.cargo/bin:/pg/bin:/usr/share/Modules/bin:/usr/lib64/ccache:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/root/bin:/home/vagrant/.cargo/bin", pgVer)

	// Build rpmbuild command
	args := []string{"rpmbuild"}

	// Add PG version macro
	args = append(args, "--define", fmt.Sprintf("pgmajorversion %d", pgVer))

	// Control debug package generation
	if !withSymbol {
		args = append(args, "--define", "debug_package %{nil}")
	}

	// Add spec file
	args = append(args, "-ba", specFile)

	// Debug log the command
	logrus.Debugf("cmd: %s", strings.Join(args, " "))

	// Set environment
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Env = append(os.Environ(), "PATH="+envPATH)

	// Set QA_RPATHS for EL10+
	if config.OSVersionCode == "el10" {
		cmd.Env = append(cmd.Env, "QA_RPATHS=3")
	}

	// Prepare metadata for log file with marker
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

	// Use unified log file name (one file per package)
	logFileName := fmt.Sprintf("%s.log", extension.Pkg)

	// First PG version creates new file, others append
	append := !isFirst

	result, err := RunBuildCommand(cmd, logFileName, append, metadata, extension.Name, pgVer)
	result.Marker = marker

	if err != nil {
		result.Success = false
	}

	// Find build artifact
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

// Build a normal (non-PG) package
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

	logrus.Info(strings.Repeat("=", 58))
	logrus.Infof("[BUILDING] %s", pkgName)
	logrus.Info(strings.Repeat("=", 58))
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

	// Log metadata with marker
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
	fmt.Printf("INFO[BUILDING] %s", pkgName)

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

	logrus.Info(strings.Repeat("=", 58))
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

	logrus.Info(strings.Repeat("=", 58))
	logrus.Infof("[BUILDING] %s (Makefile)", taskName)
	logrus.Info(strings.Repeat("=", 58))
	logrus.Infof("make : %s", makefilePath)
	logPath := filepath.Join(config.HomeDir, "ext", "log", taskName+".log")
	logrus.Infof("log  : %s", logPath)
	logrus.Info(strings.Repeat("-", 58))

	// Log metadata with marker
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
	fmt.Printf("INFO[BUILDING] %s (Makefile)", taskName)

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

	logrus.Info(strings.Repeat("=", 58))
	return err
}

// formatSize formats file size in human-readable format
func formatSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%dB", size)
	} else if size < 1024*1024 {
		return fmt.Sprintf("%dKB", size/1024)
	} else {
		return fmt.Sprintf("%.1fMB", float64(size)/(1024*1024))
	}
}

// getSpecVersion reads version from spec file
func getSpecVersion(specFile string) string {
	content, err := os.ReadFile(specFile)
	if err != nil {
		return ""
	}

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
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

// getArch returns the current system architecture
func getArch() string {
	output, err := utils.ShellOutput("uname", "-m")
	if err != nil {
		return "x86_64"
	}
	return strings.TrimSpace(output)
}

// writeInitialLogHeader writes the initial header to the log file
func writeInitialLogHeader(extension *ext.Extension, specFile string, specVersion string, pgVersions []string) {
	logName := fmt.Sprintf("%s.log", extension.Pkg)
	logger, err := NewBuildLogger(logName, false)
	if err != nil {
		logrus.Warnf("Failed to create log header: %v", err)
		return
	}
	defer logger.Close()

	// Write header
	logger.WriteMetadata(strings.Repeat("#", 58))
	logger.WriteMetadata(fmt.Sprintf("[BUILDING] %s", extension.Name))
	logger.WriteMetadata(strings.Repeat("#", 58))
	logger.WriteMetadata(fmt.Sprintf("spec : %s", specFile))

	if specVersion != "" {
		logger.WriteMetadata(fmt.Sprintf("ver  : %s", specVersion))
	}

	if extension.Source != "" {
		logger.WriteMetadata(fmt.Sprintf("src  : %s", extension.Source))
	}

	logger.WriteMetadata(fmt.Sprintf("log  : %s", logger.logPath))
	logger.WriteMetadata(fmt.Sprintf("pg   : %s", strings.Join(pgVersions, " ")))
	logger.WriteMetadata("")
}
