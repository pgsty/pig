/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>

Package postgres provides PostgreSQL server management functionality.
This package handles pg_ctl operations, log management, connection management,
and database maintenance tasks.
*/
package postgres

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"pig/cli/ext"
	"pig/internal/utils"

	"github.com/sirupsen/logrus"
)

// ============================================================================
// Default Constants
// ============================================================================

const (
	// DefaultPgData is the default PostgreSQL data directory
	DefaultPgData = "/pg/data"

	// DefaultLogDir is the default PostgreSQL csvlog directory
	// PostgreSQL runtime logs (csvlog) are stored here
	// Note: In Pigsty, the log directory is /pg/log/postgres (not "postgresql")
	DefaultLogDir = "/pg/log/postgres"

	// DefaultTimeout is the default pg_ctl timeout in seconds
	DefaultTimeout = 60

	// DefaultStopMode is the default shutdown mode for pg_ctl stop
	DefaultStopMode = "fast"

	// DefaultEncoding is the default database encoding for initdb
	DefaultEncoding = "UTF8"

	// DefaultLocale is the default locale for initdb
	DefaultLocale = "C"

	// DefaultSystemdService is the default systemd service name
	// Note: In Pigsty, the service name is "postgres" (not "postgresql")
	DefaultSystemdService = "postgres"
)

// ANSI color codes for terminal output
const (
	ColorReset  = "\033[0m"
	ColorBlue   = "\033[34m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorRed    = "\033[31m"
	ColorCyan   = "\033[36m"
	ColorBold   = "\033[1m"
)

// IdentifierRegex validates PostgreSQL identifiers (usernames, database names, schema names, table names)
// Allows alphanumeric, underscore, and dollar sign (PostgreSQL naming rules)
var IdentifierRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_$]*$`)

// ValidateIdentifier checks if a string is a valid PostgreSQL identifier
func ValidateIdentifier(s string) bool {
	if s == "" {
		return true // empty is allowed (means no filter)
	}
	return IdentifierRegex.MatchString(s)
}

// ============================================================================
// Configuration (set by cmd layer via flags)
// ============================================================================

// Config holds the runtime configuration for postgres commands
type Config struct {
	PgVersion int    // PostgreSQL major version
	PgData    string // Data directory
	DbSU      string // Database superuser
	LogDir    string // Log directory (for log commands)
}

// DefaultConfig returns a Config with default values
func DefaultConfig() *Config {
	return &Config{
		PgData: DefaultPgData,
	}
}

// ============================================================================
// Core Helper Functions
// ============================================================================

// GetPgData returns data directory from config or default
func GetPgData(cfg *Config) string {
	if cfg != nil && cfg.PgData != "" {
		return cfg.PgData
	}
	return DefaultPgData
}

// GetLogDir returns log directory from config or default
func GetLogDir(cfg *Config) string {
	if cfg != nil && cfg.LogDir != "" {
		return cfg.LogDir
	}
	return DefaultLogDir
}

// GetTimeout returns timeout: value > $PGCTLTIMEOUT > default
func GetTimeout(value int) int {
	if value > 0 {
		return value
	}
	if env := os.Getenv("PGCTLTIMEOUT"); env != "" {
		if t, err := strconv.Atoi(env); err == nil && t > 0 {
			return t
		}
	}
	return DefaultTimeout
}

// GetDbSU returns the database superuser
func GetDbSU(cfg *Config) string {
	if cfg != nil && cfg.DbSU != "" {
		return utils.GetDBSU(cfg.DbSU)
	}
	return utils.GetDBSU("")
}

// GetPgInstall finds PostgreSQL installation, optionally inferring version from data dir
func GetPgInstall(cfg *Config) (*ext.PostgresInstall, error) {
	ver := 0
	if cfg != nil {
		ver = cfg.PgVersion
	}
	dataDir := GetPgData(cfg)
	dbsu := GetDbSU(cfg)
	if ver == 0 && dataDir != "" {
		if v, err := ReadPgVersionAsDBSU(dbsu, dataDir); err == nil {
			ver = v
			logrus.Debugf("inferred PostgreSQL %d from %s", ver, dataDir)
		}
	}
	return ext.FindPostgres(ver)
}

// ReadPgVersionAsDBSU reads major version from PG_VERSION file as database superuser
func ReadPgVersionAsDBSU(dbsu, dataDir string) (int, error) {
	pgVersionFile := filepath.Join(dataDir, "PG_VERSION")
	output, err := utils.DBSUCommandOutput(dbsu, []string{"cat", pgVersionFile})
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(output))
}

// ReadPgVersion reads major version from PG_VERSION file
// Note: This runs as current user, may fail due to permission issues.
// Use ReadPgVersionAsDBSU for reliable reads when running as non-dbsu user.
func ReadPgVersion(dataDir string) (int, error) {
	data, err := os.ReadFile(filepath.Join(dataDir, "PG_VERSION"))
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

// CheckDataDir checks if data directory exists and is initialized
// Note: This runs as current user, may fail due to permission issues.
// Use CheckDataDirAsDBSU for reliable checks when running as non-dbsu user.
func CheckDataDir(dataDir string) (exists, initialized bool) {
	info, err := os.Stat(dataDir)
	if os.IsNotExist(err) {
		return false, false
	}
	if err != nil || !info.IsDir() {
		return false, false
	}
	_, err = os.Stat(filepath.Join(dataDir, "PG_VERSION"))
	return true, err == nil
}

// CheckDataDirAsDBSU checks data directory state as the database superuser.
// This is necessary when the current user may not have permission to read the data directory.
// Returns (exists, initialized bool) where:
//   - exists: true if directory exists
//   - initialized: true if PG_VERSION file exists (indicating initialized database)
func CheckDataDirAsDBSU(dbsu, dataDir string) (exists, initialized bool) {
	// Use test command to check directory and file existence as dbsu
	// test -d checks if directory exists
	cmd := buildTestCmd(dbsu, "-d", dataDir)
	if err := cmd.Run(); err != nil {
		return false, false // directory doesn't exist
	}

	// test -f checks if PG_VERSION file exists
	cmd = buildTestCmd(dbsu, "-f", filepath.Join(dataDir, "PG_VERSION"))
	if err := cmd.Run(); err != nil {
		return true, false // directory exists but not initialized
	}

	return true, true
}

// CheckPostgresRunningAsDBSU checks if PostgreSQL is running as the database superuser.
// Returns (running bool, pid int) where:
//   - running: true if postmaster.pid exists and process is alive
//   - pid: the PID from postmaster.pid (0 if not running or can't determine)
func CheckPostgresRunningAsDBSU(dbsu, dataDir string) (bool, int) {
	pidFile := filepath.Join(dataDir, "postmaster.pid")

	// Check if postmaster.pid exists
	cmd := buildTestCmd(dbsu, "-f", pidFile)
	if err := cmd.Run(); err != nil {
		return false, 0 // no pid file
	}

	// Read the pid file content as dbsu
	output, err := utils.DBSUCommandOutput(dbsu, []string{"head", "-1", pidFile})
	if err != nil {
		return false, 0
	}

	pid, err := strconv.Atoi(strings.TrimSpace(output))
	if err != nil {
		return false, 0
	}

	// Check if process exists (signal 0 test)
	process, err := os.FindProcess(pid)
	if err != nil {
		return false, pid
	}
	if err := process.Signal(syscall.Signal(0)); err != nil {
		return false, pid // stale pid file
	}

	return true, pid
}

// buildTestCmd creates a command to run 'test' as dbsu
func buildTestCmd(dbsu string, flag, path string) *exec.Cmd {
	args := []string{"test", flag, path}

	if utils.IsDBSU(dbsu) {
		return exec.Command(args[0], args[1:]...)
	}

	if os.Getenv("USER") == "root" || os.Geteuid() == 0 {
		cmdStr := strings.Join(args, " ")
		return exec.Command("su", "-", dbsu, "-c", cmdStr)
	}

	sudoArgs := append([]string{"-inu", dbsu, "--"}, args...)
	return exec.Command("sudo", sudoArgs...)
}

// CheckPostgresRunning checks if PostgreSQL is running in the data directory.
// Returns (running bool, pid int, err error) where:
//   - running=false, pid=0, err=nil: definitely not running (no pid file)
//   - running=false, pid>0, err=nil: pid file exists but process is dead (stale)
//   - running=true, pid>0, err=nil: PostgreSQL is running
//   - running=false, pid=0, err!=nil: cannot determine status (permission denied, etc.)
//
// IMPORTANT: When err != nil, callers should NOT assume PostgreSQL is stopped.
func CheckPostgresRunning(dataDir string) (bool, int, error) {
	pidFile := filepath.Join(dataDir, "postmaster.pid")
	data, err := os.ReadFile(pidFile)
	if os.IsNotExist(err) {
		return false, 0, nil // No pid file, definitely not running
	}
	if os.IsPermission(err) {
		// Permission denied - we cannot determine the status
		// Return error so caller doesn't assume PostgreSQL is stopped
		return false, 0, fmt.Errorf("cannot read %s: permission denied (run as postgres user or root)", pidFile)
	}
	if err != nil {
		// Other errors - also cannot determine status
		return false, 0, fmt.Errorf("cannot read %s: %w", pidFile, err)
	}

	// First line of postmaster.pid is the PID
	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 {
		return false, 0, nil // Empty file, treat as not running
	}

	pid, err := strconv.Atoi(strings.TrimSpace(lines[0]))
	if err != nil {
		return false, 0, nil // Invalid PID format, treat as stale
	}

	// Check if process exists
	process, err := os.FindProcess(pid)
	if err != nil {
		return false, pid, nil // Can't find process, stale pid file
	}

	// On Unix, FindProcess always succeeds. Send signal 0 to check if process exists
	err = process.Signal(syscall.Signal(0))
	if err != nil {
		return false, pid, nil // Process doesn't exist (stale pid file)
	}

	return true, pid, nil
}

// ============================================================================
// Output Helpers
// ============================================================================

// PrintHint prints command hint in blue color
func PrintHint(cmdArgs []string) {
	fmt.Printf("%s$ %s%s\n", ColorBlue, strings.Join(cmdArgs, " "), ColorReset)
}

// RunSystemctl runs systemctl command as root (via sudo if needed)
// Returns ExitCodeError if the command exits with non-zero status.
func RunSystemctl(action, service string) error {
	cmdArgs := []string{"systemctl", action, service}
	PrintHint(cmdArgs)

	var cmd *exec.Cmd
	if os.Geteuid() == 0 {
		cmd = exec.Command(cmdArgs[0], cmdArgs[1:]...)
	} else {
		cmd = exec.Command("sudo", cmdArgs...)
	}

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return &utils.ExitCodeError{Code: exitErr.ExitCode(), Err: err}
		}
		return fmt.Errorf("systemctl %s failed: %w", action, err)
	}
	return nil
}

// RunCommandQuiet runs a command and prints output, does not fail on error
func RunCommandQuiet(dbsu string, args []string) {
	var cmd *exec.Cmd
	if utils.IsDBSU(dbsu) {
		cmd = exec.Command(args[0], args[1:]...)
	} else {
		sudoArgs := append([]string{"-inu", dbsu, "--"}, args...)
		cmd = exec.Command("sudo", sudoArgs...)
	}
	output, _ := cmd.CombinedOutput()
	if len(output) > 0 {
		fmt.Print(string(output))
	}
}

// RunWithSudoFallback runs command directly first, retries with sudo if permission denied
func RunWithSudoFallback(args []string) error {
	// If already root, just run directly
	if os.Geteuid() == 0 {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	// Try running directly first
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		return nil
	}

	// Check if it's a permission error
	stderrStr := stderr.String()
	if strings.Contains(stderrStr, "Permission denied") ||
		strings.Contains(stderrStr, "permission denied") ||
		strings.Contains(stderrStr, "Operation not permitted") {
		// Retry with sudo
		logrus.Debugf("permission denied, retrying with sudo")
		sudoCmd := exec.Command("sudo", args...)
		sudoCmd.Stdin = os.Stdin
		sudoCmd.Stdout = os.Stdout
		sudoCmd.Stderr = os.Stderr
		return sudoCmd.Run()
	}

	// Not a permission error, print the stderr and return the error
	fmt.Fprint(os.Stderr, stderrStr)
	return err
}

// FormatSize formats bytes into human-readable format
func FormatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
