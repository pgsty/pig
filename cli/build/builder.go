// Package build provides PostgreSQL extension building functionality
package build

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path"
	"path/filepath"
	"pig/cli/ext"
	"pig/internal/config"
	"sort"
	"strconv"
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

var debianPgLibDir = "/usr/lib/postgresql"

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
		builder.PGVersions = defaultBuilderPGVersions(extension)
	}
	return builder, nil
}

func defaultBuilderPGVersions(extension *ext.Extension) []int {
	declared := extension.GetPGVersions()
	if len(declared) == 0 {
		return nil
	}

	// Prefer distro-specific availability list to avoid false negatives on Debian/RPM
	// (e.g. pg_ver includes versions that are not packaged for this distro).
	var available []int
	for _, v := range declared {
		if extension.Available(v) {
			available = append(available, v)
		}
	}
	if len(available) > 0 {
		declared = available
	}

	// Debian builds typically produce packages only for locally installed PG majors.
	// Use intersection as the default expectation set to avoid reporting errors when
	// the build succeeds but only a subset of versions are present on the system.
	if config.OSType == config.DistroDEB {
		installed := detectInstalledPGVersionsInDir(debianPgLibDir)
		if len(installed) > 0 {
			if inter := intersectSortedInts(declared, installed); len(inter) > 0 {
				return inter
			}
			// If there is no overlap, fall back to installed set; the build system will
			// still decide what it can actually build, but artifact detection should be
			// driven by what's present locally.
			return installed
		}
	}

	return declared
}

func detectInstalledPGVersionsInDir(baseDir string) []int {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return nil
	}

	seen := make(map[int]struct{})
	var versions []int
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		v, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		if v < 10 || v > 99 {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		versions = append(versions, v)
	}
	sort.Ints(versions)
	return versions
}

func intersectSortedInts(a, b []int) []int {
	// a and b are expected to be sorted asc.
	i, j := 0, 0
	var out []int
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			out = append(out, a[i])
			i++
			j++
		case a[i] < b[j]:
			i++
		default:
			j++
		}
	}
	return out
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
	b.checkRustEnvironment()
	b.printBuildInfo()

	// Execute builds based on OS type
	if b.OSType == config.DistroDEB {
		// Debian/Ubuntu builds all PG versions at once
		b.buildForAll()
	} else {
		// EL builds for each PG version separately
		for _, pgVer := range b.PGVersions {
			logrus.Debugf("build %s for PG %d", b.PackageName, pgVer)
			b.buildForPGVersion(pgVer)
		}
	}

	// Display final summary
	b.printSummary()
	if failed := buildFailureLabels(b.OSType, b.PGVersions, b.Builds); len(failed) > 0 {
		return fmt.Errorf("build failed for %s (%s), see log: %s", b.PackageName, strings.Join(failed, ", "), b.LogPath)
	}
	return nil
}

func buildFailureLabels(osType string, pgVersions []int, builds map[int]*BuildTask) []string {
	var failed []string
	switch osType {
	case config.DistroDEB:
		task := builds[0]
		if task == nil || !task.Success {
			failed = append(failed, "ALL")
		}
	default:
		for _, pgVer := range pgVersions {
			task := builds[pgVer]
			if task == nil || !task.Success {
				failed = append(failed, fmt.Sprintf("PG%d", pgVer))
			}
		}
	}
	return failed
}

func countNonEmptyLines(s string) int {
	if strings.TrimSpace(s) == "" {
		return 0
	}
	n := 0
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	return n
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
			b.SpecPath = filepath.Join(config.HomeDir, "debbuild", b.Extension.Pkg, "debian", "control")
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

// checkRustEnvironment validates Rust and PGRX setup for Rust extensions
func (b *ExtensionBuilder) checkRustEnvironment() {
	// Ensure no panic from this function
	defer func() {
		if r := recover(); r != nil {
			logrus.Debugf("panic in checkRustEnvironment: %v", r)
		}
	}()

	// Only check for Rust extensions
	if b.Extension == nil || b.Extension.Lang != "Rust" {
		return
	}

	// Check if cargo is available
	cargoPath, err := exec.LookPath("cargo")
	if err != nil {
		logrus.Errorf("rust cargo is required to build this")
		return
	}
	logrus.Debugf("cargo found at %s", cargoPath)

	// Get installed pgrx version
	cmd := exec.Command("cargo", "pgrx", "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		logrus.Errorf("fail to get pgrx version: %v", err)
		return
	}

	// Parse version from output (e.g., "cargo-pgrx 0.16.1" -> "0.16.1")
	versionOutput := strings.TrimSpace(string(output))
	fields := strings.Fields(versionOutput)
	if len(fields) == 0 {
		logrus.Debugf("empty pgrx version output")
		return
	}
	installedVersion := fields[len(fields)-1] // Take last field
	logrus.Debugf("installed pgrx version: %s", installedVersion)

	// Get expected pgrx version from extension metadata
	if b.Extension.Extra == nil {
		logrus.Debugf("no extra metadata for pgrx version check")
		return
	}

	expectedVersionRaw, ok := b.Extension.Extra["pgrx"]
	if !ok {
		logrus.Debugf("no pgrx version specified in extension metadata")
		return
	}

	expectedVersion, ok := expectedVersionRaw.(string)
	if !ok {
		logrus.Debugf("pgrx version in metadata is not a string")
		return
	}

	expectedVersion = strings.TrimSpace(expectedVersion)
	if expectedVersion == "" {
		logrus.Debugf("empty pgrx version in metadata")
		return
	}

	// Compare versions
	if installedVersion != expectedVersion {
		logrus.Errorf("PGRX version mismatch: extension requires %s but system has %s",
			expectedVersion, installedVersion)
	} else {
		logrus.Debugf("pgrx version matches: %s", installedVersion)
	}
}

// printBuildInfo prints build configuration information
func (b *ExtensionBuilder) printBuildInfo() {
	if b.OSType == "rpm" {
		logrus.Infof("spec : %s", b.SpecPath)
	} else {
		logrus.Infof("control : %s", b.SpecPath)
	}

	logrus.Infof("ver  : %s", b.buildVersionForLog())
	if src := b.buildSourceForLog(); src != "" {
		logrus.Infof("src  : %s", src)
	}
	logrus.Infof("log  : %s/%s.log", b.LogDir, b.Extension.Pkg)
	logrus.Infof("pg   : %s", b.formatPGVersions())
	logrus.Info(strings.Repeat("-", b.HeaderWidth))
}

func (b *ExtensionBuilder) buildVersionForLog() string {
	if b.Extension == nil {
		return ""
	}
	switch b.OSType {
	case config.DistroEL:
		if v := strings.TrimSpace(b.Extension.RpmVer); v != "" {
			return v
		}
	case config.DistroDEB:
		if v := strings.TrimSpace(b.Extension.DebVer); v != "" {
			return v
		}
	}
	return strings.TrimSpace(b.Extension.Version)
}

func (b *ExtensionBuilder) buildSourceForLog() string {
	if b.Extension == nil {
		return ""
	}
	return strings.TrimSpace(b.Extension.Source)
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
		_ = b.LogFile.Sync() // Flush buffer to disk
		_ = b.LogFile.Close()
	}
}

// buildPath constructs the PATH environment variable properly
func (b *ExtensionBuilder) buildPath(pgVer int) string {
	// Get current PATH
	currentPath := os.Getenv("PATH")
	pathParts := strings.Split(currentPath, ":")

	// Deduplicate function
	dedup := func(paths []string) []string {
		seen := make(map[string]bool)
		result := []string{}
		for _, p := range paths {
			if p != "" && !seen[p] {
				seen[p] = true
				result = append(result, p)
			}
		}
		return result
	}

	// Build the new PATH components
	var newPaths []string

	// 1. PostgreSQL bin directory (always first)
	switch b.OSType {
	case config.DistroEL:
		newPaths = append(newPaths, fmt.Sprintf("/usr/pgsql-%d/bin", pgVer))
	case config.DistroDEB:
		newPaths = append(newPaths, fmt.Sprintf("/usr/lib/postgresql/%d/bin", pgVer))
	default:
		// For macOS or other systems, try to detect PostgreSQL location
		// You may need to customize this based on your macOS setup
		newPaths = append(newPaths, fmt.Sprintf("/usr/local/opt/postgresql@%d/bin", pgVer))
	}

	// 2. Cargo bin directory (expand home directory)
	if currentUser, err := user.Current(); err == nil {
		cargoPath := filepath.Join(currentUser.HomeDir, ".cargo", "bin")
		if _, err := os.Stat(cargoPath); err == nil {
			newPaths = append(newPaths, cargoPath)
		}
	}

	// 3. Additional directories (in order of priority)
	additionalPaths := []string{
		"/usr/share/Modules/bin",
		"/usr/lib64/ccache",
		"/usr/local/sbin",
		"/usr/local/bin",
		"/usr/sbin",
		"/usr/bin",
		"/root/bin",
	}

	// Combine all paths: new paths first, then existing PATH (deduped)
	allPaths := append(newPaths, additionalPaths...)
	allPaths = append(allPaths, pathParts...)

	// Deduplicate while preserving order
	finalPaths := dedup(allPaths)

	return strings.Join(finalPaths, ":")
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

	// Flush to disk immediately after task completion
	if b.LogFile != nil {
		_ = b.LogFile.Sync()
	}
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

// buildForAll builds all PG versions at once (Debian/Ubuntu)
func (b *ExtensionBuilder) buildForAll() {
	// Display progress
	fmt.Printf("[ALL]  Building %s for all PG versions...", b.Extension.Name)

	// Initialize build task (use pgVer=0 as special marker for "all versions")
	beginTime := time.Now()
	taskID := fmt.Sprintf("%s_all_%s", b.PackageName, beginTime.Format("20060102150405"))

	task := &BuildTask{
		ID:        taskID,
		Package:   b.PackageName,
		BeginTime: beginTime,
		LogPath:   b.LogPath,
		Builder:   b,
	}
	b.Builds[0] = task // Use 0 as key for "all versions" build

	// Build directory
	buildDir := filepath.Join(b.HomeDir, "debbuild", b.Extension.Pkg)

	// Create command
	cmd := exec.Command("make")
	cmd.Dir = buildDir

	// Set environment
	envPATH := os.Getenv("PATH")
	cmd.Env = append(os.Environ(), "PATH="+envPATH)

	// Prepare metadata
	metadata := []string{
		fmt.Sprintf("BUILD: %s (all PG versions)", b.Extension.Name),
		fmt.Sprintf("DIR  : %s", buildDir),
		fmt.Sprintf("TIME : %s", time.Now().Format("2006-01-02 15:04:05 -07")),
		fmt.Sprintf("PATH : %s", envPATH),
		"CMD  : make",
	}

	// Write task header to log
	b.writeTaskHeader(task)

	// Write metadata to log
	if len(metadata) > 0 {
		b.writeLog(metadata...)
		b.writeLog(strings.Repeat("=", b.HeaderWidth))
	}

	// Execute build command
	if err := b.executeBuildCommand(cmd, 0, task); err != nil {
		task.Error = err.Error()
	} else {
		// Build success is determined by command exit code. Artifact discovery is
		// best-effort and should not flip an otherwise successful build to failure.
		task.Success = true
		b.findDebianArtifacts(task)

		expected := len(b.PGVersions)
		found := countNonEmptyLines(task.Artifact)
		if found == 0 {
			logrus.Warnf("Build command succeeded but no package artifacts were discovered under %s",
				filepath.Join(b.HomeDir, "ext", "pkg"))
		} else if expected > 0 && found < expected {
			logrus.Warnf("Build command succeeded with partial package artifacts: got %d of %d",
				found, expected)
		}
	}

	// Set end time
	task.EndTime = time.Now()

	// Write task footer to log
	b.writeTaskFooter(task)

	// Clear progress line and display result
	fmt.Print("\r\033[K")
	if task.Success {
		if task.Artifact != "" {
			logrus.Infof("[ALL] [PASS] Built packages:")
			// Print each artifact
			artifacts := strings.Split(task.Artifact, "\n")
			for _, artifact := range artifacts {
				if artifact != "" {
					logrus.Infof("  - %s", artifact)
				}
			}
		} else {
			logrus.Infof("[ALL] [PASS] Build command succeeded")
		}
	} else {
		logrus.Errorf("[ALL] [FAIL] %s", fmt.Sprintf("grep -A60 %s %s", task.ID, b.LogPath))
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
	envPATH := b.buildPath(pgVer)
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

// createDEBBuildCommand creates the make command for DEB build
func (b *ExtensionBuilder) createDEBBuildCommand(pgVer int, task *BuildTask) (*exec.Cmd, []string) {
	// Build in the extension directory
	buildDir := filepath.Join(b.HomeDir, "debbuild", b.Extension.Pkg)

	cmd := exec.Command("make")
	cmd.Dir = buildDir

	// Set environment
	envPATH := b.buildPath(pgVer)
	cmd.Env = append(os.Environ(), "PATH="+envPATH)

	metadata := []string{
		fmt.Sprintf("BUILD: %s PG%d", b.Extension.Name, pgVer),
		fmt.Sprintf("DIR  : %s", buildDir),
		fmt.Sprintf("TIME : %s", time.Now().Format("2006-01-02 15:04:05 -07")),
		fmt.Sprintf("PATH : %s", envPATH),
		"CMD  : make",
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
			} else if pgVer == 0 && b.OSType == config.DistroDEB {
				// Debian all-versions build
				fmt.Printf("\r\033[K[ALL]  %s", truncateLine(line, 60))
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
	var globPattern string
	var globPaths []string

	switch b.OSType {
	case "rpm":
		globPattern = b.resolvePackageGlob(b.Extension.RpmPkg, pgVer, "rpm")
		rpmDir := filepath.Join(b.HomeDir, "rpmbuild", "RPMS")
		globPaths = []string{
			filepath.Join(rpmDir, b.OSArch, globPattern),
			filepath.Join(rpmDir, "noarch", globPattern),
			filepath.Join(rpmDir, "*", globPattern),
		}
	case "deb":
		globPattern = b.resolvePackageGlob(b.Extension.DebPkg, pgVer, "deb")
		debDir := filepath.Join(b.HomeDir, "ext", "pkg")
		globPaths = []string{
			filepath.Join(debDir, globPattern),
			filepath.Join(debDir, "pool", "*", "*", "*", globPattern),
		}
	default:
		return
	}

	// Find all matching files across possible output layouts.
	seen := make(map[string]struct{})
	var candidates []string
	for _, p := range globPaths {
		matches, err := filepath.Glob(p)
		if err != nil {
			logrus.Debugf("glob pattern error: %v", err)
			continue
		}
		for _, m := range matches {
			if _, ok := seen[m]; ok {
				continue
			}
			seen[m] = struct{}{}
			candidates = append(candidates, m)
		}
	}
	if len(candidates) == 0 {
		logrus.Debugf("no artifacts found matching %s (PG%d)", globPattern, pgVer)
		return
	}

	// Select the shortest filename (usually the main package)
	artifact := candidates[0]
	for _, candidate := range candidates[1:] {
		if len(filepath.Base(candidate)) < len(filepath.Base(artifact)) {
			artifact = candidate
		}
	}

	task.Artifact = artifact
	if info, err := os.Stat(artifact); err == nil {
		task.Size = info.Size()
	}
}

// findDebianArtifacts finds all build artifacts for Debian (all PG versions)
func (b *ExtensionBuilder) findDebianArtifacts(task *BuildTask) {
	artifactDir := filepath.Join(b.HomeDir, "ext", "pkg")

	// Check if directory exists
	if _, err := os.Stat(artifactDir); err != nil {
		logrus.Debugf("artifact directory not found: %s", artifactDir)
		return
	}

	// Find packages for each PG version
	var foundArtifacts []string
	var missingVersions []int
	var totalSize int64

	for _, pgVer := range b.PGVersions {
		// Build glob pattern for this PG version
		globPattern := b.resolvePackageGlob(b.Extension.DebPkg, pgVer, "deb")

		// Try a few likely layouts under ~/ext/pkg.
		globPaths := []string{
			filepath.Join(artifactDir, globPattern),
			filepath.Join(artifactDir, "pool", "*", "*", "*", globPattern),
		}

		seen := make(map[string]struct{})
		var candidates []string
		for _, p := range globPaths {
			matches, err := filepath.Glob(p)
			if err != nil {
				logrus.Debugf("glob pattern error for PG%d: %v", pgVer, err)
				continue
			}
			for _, m := range matches {
				if _, ok := seen[m]; ok {
					continue
				}
				seen[m] = struct{}{}
				candidates = append(candidates, m)
			}
		}

		if len(candidates) == 0 {
			logrus.Debugf("no artifacts found for PG%d matching %s", pgVer, globPattern)
			missingVersions = append(missingVersions, pgVer)
			continue
		}

		// Select the shortest filename (usually the main package)
		artifact := candidates[0]
		for _, candidate := range candidates[1:] {
			if len(filepath.Base(candidate)) < len(filepath.Base(artifact)) {
				artifact = candidate
			}
		}

		foundArtifacts = append(foundArtifacts, artifact)
		if info, err := os.Stat(artifact); err == nil {
			totalSize += info.Size()
		}
	}

	// Store results
	if len(foundArtifacts) > 0 {
		task.Artifact = strings.Join(foundArtifacts, "\n")
		task.Size = totalSize
	}

	// Log missing versions if any
	if len(missingVersions) > 0 {
		var missingStrs []string
		for _, v := range missingVersions {
			missingStrs = append(missingStrs, fmt.Sprintf("%d", v))
		}
		logrus.Warnf("Missing packages for PG versions: %s", strings.Join(missingStrs, ", "))
	}
}

// resolvePackageGlob resolves the package glob pattern from pkg pattern
// Example: "acl_$v*" with pgVer=18 -> "acl_18*"
// Example: "timescaledb-tsl_$v" with pgVer=18 -> "timescaledb-tsl_18*" (adds * if missing)
func (b *ExtensionBuilder) resolvePackageGlob(pkgPattern string, pgVer int, osType string) string {
	var defaultPattern string
	if osType == "rpm" {
		defaultPattern = fmt.Sprintf("%s_%d*.rpm", b.Extension.Pkg, pgVer)
	} else {
		defaultPattern = fmt.Sprintf("%s_%d*.deb", b.Extension.Pkg, pgVer)
	}

	if pkgPattern == "" {
		return defaultPattern
	}

	// Split by whitespace and take first element
	fields := strings.Fields(pkgPattern)
	if len(fields) == 0 {
		return defaultPattern
	}
	pattern := fields[0]

	// Replace $v with major version
	pattern = strings.ReplaceAll(pattern, "$v", fmt.Sprintf("%d", pgVer))

	// Add * suffix if not present
	if !strings.HasSuffix(pattern, "*") {
		pattern = pattern + "*"
	}

	// Add file extension if not present
	if osType == "rpm" && !strings.Contains(pattern, ".rpm") {
		pattern = pattern + ".rpm"
	} else if osType == "deb" && !strings.Contains(pattern, ".deb") {
		pattern = pattern + ".deb"
	}

	return pattern
}

// printSummary prints the build summary
func (b *ExtensionBuilder) printSummary() {
	logrus.Info(strings.Repeat("-", b.HeaderWidth))

	duration := time.Since(b.StartTime)
	totalCount := len(b.PGVersions)

	// Handle Debian all-versions build differently
	if b.OSType == config.DistroDEB {
		task := b.Builds[0]
		if task != nil && task.Success {
			// Count how many artifacts were found
			artifactCount := countNonEmptyLines(task.Artifact)
			if artifactCount == 0 {
				logrus.Warnf("[DONE] PASS build command succeeded, but no packages were discovered in %v",
					duration.Round(time.Second))
				return
			}

			if totalCount == 0 {
				logrus.Infof("[DONE] PASS build command succeeded, discovered %d package(s) in %v",
					artifactCount, duration.Round(time.Second))
				return
			}

			if artifactCount == totalCount {
				logrus.Infof("[DONE] PASS all %d packages built in %v",
					totalCount, duration.Round(time.Second))
			} else {
				logrus.Warnf("[DONE] PASS build command succeeded, discovered %d of %d package(s) in %v",
					artifactCount, totalCount, duration.Round(time.Second))
			}
		} else {
			logrus.Errorf("[DONE] FAIL build failed in %v", duration.Round(time.Second))
		}
	} else {
		// EL per-version build
		successCount := 0
		for _, task := range b.Builds {
			if task.Success {
				successCount++
			}
		}

		if successCount < totalCount {
			logrus.Warnf("[DONE] FAIL %d of %d packages built (%d failed) in %v",
				successCount, totalCount, totalCount-successCount, duration.Round(time.Second))
		} else {
			logrus.Infof("[DONE] PASS all %d packages built in %v",
				totalCount, duration.Round(time.Second))
		}
	}
}
