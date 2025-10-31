// Package build provides PostgreSQL extension building functionality
package build

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"pig/cli/ext"
	"pig/internal/config"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// BuildTask represents a single build task for a specific PG version
type BuildTask struct {
	ID        string            // Task ID: Package_PgVersion_Time
	Package   string            // Package name
	Success   bool              // Build succeeded
	LogPath   string            // Log file path for this task
	Artifact  string            // Path to built artifact
	Size      int64             // Artifact size in bytes
	BeginTime time.Time         // Build start time
	EndTime   time.Time         // Build end time
	Error     string            // Error message if failed
	Builder   *ExtensionBuilder // Parent
}

// ExtensionBuilder encapsulates the state and operations for building extensions
type ExtensionBuilder struct {
	// Package information
	PackageName  string         // Original package name from user
	Extension    *ext.Extension // The extension being built
	PGVersions   []int          // PostgreSQL versions to build for
	DebugPackage bool           // Include debug symbols

	// System configuration
	OSType   string // OS type (rpm/deb)
	OSArch   string // System architecture (x86_64/aarch64)
	HomeDir  string // Build home directory
	SpecPath string // Path to spec/control file

	// Logging
	LogDir    string   // Log directory path
	LogPath   string   // Full log path
	LogFile   *os.File // Log file handle
	LogAppend bool     // Append to existing log

	// Build tracking
	Builds      map[int]*BuildTask // Build tasks per PG version
	StartTime   time.Time          // Build start time
	HeaderWidth int                // Width for header separators
}

// NewExtensionBuilder creates a new ExtensionBuilder instance
func NewExtensionBuilder(packageName string) (*ExtensionBuilder, error) {
	extension, err := resolveExtension(packageName)
	if err != nil {
		logrus.Debugf("package %s not found in extension catalog", packageName)
	}

	builder := &ExtensionBuilder{
		PackageName: packageName,
		Extension:   extension, // could be nil, if it is not an extension
		OSType:      config.OSType,
		OSArch:      getElArch(),
		HomeDir:     config.HomeDir,
		LogDir:      filepath.Join(config.HomeDir, "ext", "log"),
		LogAppend:   true,
		Builds:      make(map[int]*BuildTask),
		StartTime:   time.Now(),
		HeaderWidth: 60,
	}

	if extension != nil {
		builder.PGVersions = extension.GetPGVersions()
	}
	return builder, nil
}

// UpdateVersion updates the PG versions to build for
func (b *ExtensionBuilder) UpdateVersion(pgVersions string) error {
	if pgVersions == "" || b.Extension == nil {
		return nil
	}
	if versions, err := parsePgVersions(pgVersions); err != nil {
		return err
	} else {
		b.PGVersions = versions
	}
	return nil
}

// Build executes the build process
func (b *ExtensionBuilder) Build() error {
	if err := b.initLogger(); err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}
	defer b.closeLogger()

	b.printHeader()
	if err := b.validateBuildFiles(); err != nil {
		return err
	}
	b.printBuildInfo()

	// Execute builds for all PG versions
	for _, pgVer := range b.PGVersions {
		logrus.Debugf("build %s for PG %d", b.PackageName, pgVer)
		b.buildForPGVersion(pgVer)
	}

	// Display final summary
	b.printSummary()
	return nil
}

// printHeader prints the build header
func (b *ExtensionBuilder) printHeader() {
	separator := strings.Repeat("=", b.HeaderWidth)
	logrus.Info(separator)
	logrus.Infof("[BUILD %s] %s", strings.ToUpper(b.OSType), b.PackageName)
	logrus.Info(separator)
}

// validateBuildFiles checks if necessary build files exist
func (b *ExtensionBuilder) validateBuildFiles() error {
	switch b.OSType {
	case config.DistroEL:
		if b.Extension != nil {
			b.SpecPath = filepath.Join(config.HomeDir, "rpmbuild", "SPECS", b.Extension.Pkg+".spec")
		}
		if _, err := os.Stat(b.SpecPath); err != nil {
			return fmt.Errorf("build file not found: %s", b.SpecPath)
		}
	case config.DistroDEB:
		if b.Extension != nil {
			b.SpecPath = filepath.Join(config.HomeDir, "deb", b.Extension.Pkg, "debian", "control")
		}
		if _, err := os.Stat(b.SpecPath + ".in"); err != nil {
			logrus.Debugf("control file not found: %s", b.SpecPath+".in")
			if _, err := os.Stat(b.SpecPath); err != nil {
				return fmt.Errorf("control file not found: %s", b.SpecPath)
			}
		} else {
			b.SpecPath = b.SpecPath + ".in"
		}
	default:
		return fmt.Errorf("unsupported OS: %s", b.OSType)
	}
	return nil
}

// printBuildInfo prints build configuration information
func (b *ExtensionBuilder) printBuildInfo() {
	if b.OSType == "rpm" {
		logrus.Infof("spec : %s", b.SpecPath)
	} else {
		logrus.Infof("control : %s", b.SpecPath)
	}

	logrus.Infof("ver  : %s", b.Extension.Version)
	logrus.Infof("src  : %s-%s.tar.gz", b.Extension.Name, b.Extension.Version)
	logrus.Infof("log  : %s/%s.log", b.LogDir, b.Extension.Pkg)
	logrus.Infof("pg   : %s", b.formatPGVersions())
	logrus.Info(strings.Repeat("-", b.HeaderWidth))
}

// initLogger initializes the build log file
func (b *ExtensionBuilder) initLogger() error {
	// Ensure log directory exists
	if err := os.MkdirAll(b.LogDir, 0755); err != nil {
		return fmt.Errorf("failed to create log dir %s: %w", b.LogDir, err)
	}

	// Set log file path
	b.LogPath = path.Join(b.LogDir, fmt.Sprintf("%s.log", b.PackageName))

	// Open log file
	var err error
	if b.LogAppend {
		b.LogFile, err = os.OpenFile(b.LogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	} else {
		b.LogFile, err = os.Create(b.LogPath)
	}
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	return nil
}

// closeLogger closes the log file
func (b *ExtensionBuilder) closeLogger() {
	if b.LogFile != nil {
		_ = b.LogFile.Close()
	}
}

// writeLog writes lines to the log file
func (b *ExtensionBuilder) writeLog(lines ...string) {
	if b.LogFile == nil {
		return
	}
	for _, line := range lines {
		fmt.Fprintln(b.LogFile, line)
	}
}

// writeTaskHeader writes task metadata header to log
func (b *ExtensionBuilder) writeTaskHeader(task *BuildTask) {
	b.writeLog("")
	b.writeLog(strings.Repeat("=", b.HeaderWidth))
	b.writeLog(task.ID)
	b.writeLog(strings.Repeat("=", b.HeaderWidth))
	b.writeLog(fmt.Sprintf("Package : %s", task.Package))
	b.writeLog(fmt.Sprintf("Start   : %s", task.BeginTime.Format("2006-01-02 15:04:05")))
	b.writeLog(strings.Repeat("=", b.HeaderWidth))
}

// writeTaskFooter writes task result footer to log
func (b *ExtensionBuilder) writeTaskFooter(task *BuildTask) {
	duration := task.EndTime.Sub(task.BeginTime)
	status := "PASS"
	if !task.Success {
		status = "FAIL"
	}

	b.writeLog("", strings.Repeat("=", b.HeaderWidth))
	b.writeLog(fmt.Sprintf("Build %s, duration %v ms", status, duration.Round(time.Microsecond)))
	b.writeLog(strings.Repeat("=", b.HeaderWidth), "")
}

// formatPGVersions formats PG versions for display
func (b *ExtensionBuilder) formatPGVersions() string {
	parts := make([]string, len(b.PGVersions))
	for i, v := range b.PGVersions {
		parts[i] = fmt.Sprintf("%d", v)
	}
	return strings.Join(parts, " ")
}

// buildForPGVersion builds for a specific PG version
func (b *ExtensionBuilder) buildForPGVersion(pgVer int) {
	// Display progress
	fmt.Printf("[PG%d]  Building %s...", pgVer, b.Extension.Name)

	// Initialize build task
	beginTime := time.Now()
	taskID := fmt.Sprintf("%s_%d_%s", b.PackageName, pgVer, beginTime.Format("20060102150405"))

	task := &BuildTask{
		ID:        taskID,
		Package:   b.PackageName,
		BeginTime: beginTime,
		LogPath:   b.LogPath,
		Builder:   b,
	}
	b.Builds[pgVer] = task

	// Create build command based on OS type
	var cmd *exec.Cmd
	var metadata []string

	switch b.OSType {
	case config.DistroEL:
		cmd, metadata = b.createRPMBuildCommand(pgVer, task)
	case config.DistroDEB:
		cmd, metadata = b.createDEBBuildCommand(pgVer, task)
	default:
		task.Error = fmt.Sprintf("unsupported OS type: %s", b.OSType)
		task.EndTime = time.Now()
		fmt.Print("\r\033[K") // Clear progress line
		logrus.Errorf("[PG%d]  FAIL %s", pgVer, task.Error)
		return
	}

	// Write task header to log
	b.writeTaskHeader(task)

	// Write metadata to log
	if len(metadata) > 0 {
		b.writeLog(metadata...)
		b.writeLog(strings.Repeat("=", b.HeaderWidth))
	}

	// Execute build command
	if err := b.executeBuildCommand(cmd, pgVer, task); err != nil {
		task.Error = err.Error()
	} else {
		task.Success = true
		b.findArtifact(pgVer, task)
	}

	// Set end time
	task.EndTime = time.Now()

	// Write task footer to log
	b.writeTaskFooter(task)

	// Clear progress line and display result
	fmt.Print("\r\033[K")
	if task.Success {
		logrus.Infof("[PG%d] [PASS] %s", pgVer, task.Artifact)
	} else {
		logrus.Errorf("[PG%d] [FAIL] %s", pgVer, fmt.Sprintf("grep -A60 %s %s", task.ID, b.LogPath))
	}
}

// createRPMBuildCommand creates the rpmbuild command
func (b *ExtensionBuilder) createRPMBuildCommand(pgVer int, task *BuildTask) (*exec.Cmd, []string) {
	args := []string{
		"rpmbuild", "-ba",
		"--define", fmt.Sprintf("pgmajorversion %d", pgVer),
		"--define", fmt.Sprintf("pginstdir /usr/pgsql-%d", pgVer),
		"--define", fmt.Sprintf("pgpackageversion %d", pgVer),
	}

	if !b.DebugPackage {
		args = append(args, "--define", "debug_package %{nil}")
	}

	args = append(args, b.SpecPath)

	// Create command
	cmd := exec.Command(args[0], args[1:]...)

	// Set environment
	envPATH := fmt.Sprintf("/usr/pgsql-%d/bin:/usr/local/bin:/usr/bin:/bin", pgVer)
	cmd.Env = append(os.Environ(), "PATH="+envPATH)

	// Special case for EL10
	if config.OSVersionCode == "el10" {
		cmd.Env = append(cmd.Env, "QA_RPATHS=3")
	}

	// Create metadata
	metadata := []string{
		fmt.Sprintf("BUILD: %s PG%d", b.Extension.Name, pgVer),
		fmt.Sprintf("SPEC : %s", b.SpecPath),
		fmt.Sprintf("TIME : %s", time.Now().Format("2006-01-02 15:04:05 -07")),
		fmt.Sprintf("PATH : %s", envPATH),
		fmt.Sprintf("CMD  : %s", strings.Join(args, " ")),
	}

	return cmd, metadata
}

// createDEBBuildCommand creates the dpkg-buildpackage command
func (b *ExtensionBuilder) createDEBBuildCommand(pgVer int, task *BuildTask) (*exec.Cmd, []string) {
	args := []string{
		"dpkg-buildpackage", "-b", "-uc", "-us",
	}

	cmd := exec.Command(args[0], args[1:]...)

	// Set environment
	envPATH := fmt.Sprintf("/usr/lib/postgresql/%d/bin:/usr/local/bin:/usr/bin:/bin", pgVer)
	cmd.Env = append(os.Environ(), "PATH="+envPATH)

	metadata := []string{
		fmt.Sprintf("BUILD: %s PG%d", b.Extension.Name, pgVer),
		fmt.Sprintf("TIME : %s", time.Now().Format("2006-01-02 15:04:05 -07")),
		fmt.Sprintf("PATH : %s", envPATH),
		fmt.Sprintf("CMD  : %s", strings.Join(args, " ")),
	}

	return cmd, metadata
}

// executeBuildCommand executes the build command and captures output
func (b *ExtensionBuilder) executeBuildCommand(cmd *exec.Cmd, pgVer int, task *BuildTask) error {
	// Create pipes
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %v", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %v", err)
	}

	// Start command
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start command: %v", err)
	}

	// Process output
	errorOutput := b.processCommandOutput(stdout, stderr, pgVer)

	// Wait for command to complete
	if err := cmd.Wait(); err != nil {
		if errorOutput != "" {
			return fmt.Errorf("%s", errorOutput)
		}
		return err
	}

	return nil
}

// processCommandOutput processes stdout and stderr from command
func (b *ExtensionBuilder) processCommandOutput(stdout, stderr io.Reader, pgVer int) string {
	stdoutReader := bufio.NewReader(stdout)
	stderrReader := bufio.NewReader(stderr)

	doneChan := make(chan bool, 2)
	var errorOutput string

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
			b.writeLog(line)

			// Display progress
			if pgVer > 0 {
				fmt.Printf("\r\033[K[PG%d]  %s", pgVer, truncateLine(line, 60))
			}
		}
		doneChan <- true
	}()

	// Read stderr
	go func() {
		var errorLines []string
		for {
			line, err := stderrReader.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					logrus.Debugf("stderr read error: %v", err)
				}
				break
			}
			line = strings.TrimRight(line, "\n\r")
			b.writeLog(line)

			// Check for errors
			if strings.Contains(line, "error:") || strings.Contains(line, "Error:") {
				errorLines = append(errorLines, line)
			}
		}

		if len(errorLines) > 0 {
			errorOutput = errorLines[0] // Use first error line
		}
		doneChan <- true
	}()

	// Wait for both readers
	<-doneChan
	<-doneChan

	return errorOutput
}

// findArtifact finds the build artifact
func (b *ExtensionBuilder) findArtifact(pgVer int, task *BuildTask) {
	var pattern string
	var artifactDir string

	switch b.OSType {
	case "rpm":
		artifactDir = filepath.Join(b.HomeDir, "rpmbuild", "RPMS", b.OSArch)
		pattern = filepath.Join(artifactDir, fmt.Sprintf("%s_%d*.rpm", b.Extension.Pkg, pgVer))
	case "deb":
		artifactDir = filepath.Join(b.HomeDir, "deb", "pool")
		pattern = filepath.Join(artifactDir, fmt.Sprintf("%s_%d*.deb", b.Extension.Pkg, pgVer))
	}

	if files, err := filepath.Glob(pattern); err == nil && len(files) > 0 {
		task.Artifact = files[0]
		if info, err := os.Stat(files[0]); err == nil {
			task.Size = info.Size()
		}
	}
}

// printSummary prints the build summary
func (b *ExtensionBuilder) printSummary() {
	successCount := 0
	for _, task := range b.Builds {
		if task.Success {
			successCount++
		}
	}

	totalCount := len(b.PGVersions)
	duration := time.Since(b.StartTime)

	logrus.Info(strings.Repeat("-", b.HeaderWidth))

	if successCount < totalCount {
		logrus.Warnf("[DONE] FAIL %d of %d packages built (%d failed) in %v",
			successCount, totalCount, totalCount-successCount, duration.Round(time.Second))
	} else {
		logrus.Infof("[DONE] PASS all %d packages built in %v",
			totalCount, duration.Round(time.Second))
	}
}

// GetSuccessCount returns the number of successful builds
func (b *ExtensionBuilder) GetSuccessCount() int {
	count := 0
	for _, task := range b.Builds {
		if task.Success {
			count++
		}
	}
	return count
}

// GetFailedVersions returns the PG versions that failed to build
func (b *ExtensionBuilder) GetFailedVersions() []int {
	var failed []int
	for pgVer, task := range b.Builds {
		if !task.Success {
			failed = append(failed, pgVer)
		}
	}
	return failed
}
