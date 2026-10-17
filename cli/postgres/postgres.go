/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

Package postgres provides PostgreSQL server management functionality.
This package handles pg_ctl operations, log management, connection management,
and database maintenance tasks.
*/
package postgres

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"pig/cli/ext"
	"pig/internal/utils"

	"github.com/sirupsen/logrus"
)

var postgresDBSUCommandOutput = utils.DBSUCommandStdout

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

// ValidateIdentifier checks if a string is a valid PostgreSQL identifier.
// This is a convenience wrapper around utils.ValidateIdentifier.
func ValidateIdentifier(s string) bool {
	return utils.ValidateIdentifier(s)
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
	output, err := utils.DBSUCommandStdout(dbsu, []string{"cat", pgVersionFile})
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(output))
}

// CheckDataDirAsDBSU checks data directory state as the database superuser.
// This is necessary when the current user may not have permission to read the data directory.
// Returns (exists, initialized bool) where:
//   - exists: true if directory exists
//   - initialized: true if PG_VERSION file exists (indicating initialized database)
func CheckDataDirAsDBSU(dbsu, dataDir string) (exists, initialized bool) {
	// Use test command to check directory and file existence as dbsu
	// test -d checks if directory exists
	if _, err := utils.DBSUCommandStdout(dbsu, []string{"test", "-d", dataDir}); err != nil {
		return false, false // directory doesn't exist
	}

	// test -f checks if PG_VERSION file exists
	if _, err := utils.DBSUCommandStdout(dbsu, []string{"test", "-f", filepath.Join(dataDir, "PG_VERSION")}); err != nil {
		return true, false // directory exists but not initialized
	}

	return true, true
}

// CheckPostgresRunningAsDBSU checks if PostgreSQL is running as the database superuser.
// A failed probe conservatively returns (true, 0): callers must not infer that
// destructive work is safe, or use an unverified PID for signalling.
func CheckPostgresRunningAsDBSU(dbsu, dataDir string) (bool, int) {
	running, pid, _, err := checkPostgresRunningAsDBSUWithError(dbsu, dataDir)
	if err != nil {
		logrus.Debugf("cannot determine PostgreSQL running state: %v", err)
		return true, 0
	}
	return running, pid
}

func postmasterPIDMatchesDataDirAsDBSU(dbsu, pidContent, dataDir string) bool {
	pidDataDir := postmasterPIDDataDir(pidContent)
	if pidDataDir == "" {
		return true
	}
	if filepath.Clean(pidDataDir) == filepath.Clean(dataDir) {
		return true
	}
	pidResolved, pidErr := resolvePathAsDBSU(dbsu, pidDataDir)
	dataResolved, dataErr := resolvePathAsDBSU(dbsu, dataDir)
	if pidErr == nil && dataErr == nil && pidResolved != "" && dataResolved != "" {
		return filepath.Clean(pidResolved) == filepath.Clean(dataResolved)
	}
	return false
}

func postmasterPIDDataDir(pidContent string) string {
	lines := strings.Split(pidContent, "\n")
	if len(lines) < 2 {
		return ""
	}
	return strings.TrimSpace(lines[1])
}

func resolvePathAsDBSU(dbsu, path string) (string, error) {
	out, err := postgresDBSUCommandOutput(dbsu, []string{"readlink", "-f", path})
	return strings.TrimSpace(out), err
}

// ============================================================================
// Output Helpers
// ============================================================================

// PrintHint prints command hint in blue color.
// This is a convenience wrapper around utils.PrintHint.
func PrintHint(cmdArgs []string) {
	utils.PrintHint(cmdArgs)
}

// RunSystemctl runs systemctl command as root (via sudo if needed).
// This is a convenience wrapper around utils.RunSystemctl.
func RunSystemctl(action, service string) error {
	return utils.RunSystemctl(action, service)
}

// RunCommandQuiet runs a command as DBSU and prints output, but does not
// propagate command failures to its diagnostic caller.
func RunCommandQuiet(dbsu string, args []string) {
	output, _ := utils.DBSUCommandOutput(dbsu, args)
	if len(output) > 0 {
		fmt.Print(output)
	}
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
