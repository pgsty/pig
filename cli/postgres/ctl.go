/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>

pg_ctl operations: init, start, stop, restart, reload, status, promote
*/
package postgres

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"pig/cli/ext"
	"pig/internal/utils"

	"github.com/sirupsen/logrus"
)

// ============================================================================
// InitOptions for pg init command
// ============================================================================

// InitOptions contains options for InitDB
type InitOptions struct {
	Encoding  string
	Locale    string
	Checksum  bool
	Force     bool // Force init, remove existing data directory (DANGEROUS)
	ExtraArgs []string
}

// InitDB initializes a PostgreSQL data directory
func InitDB(cfg *Config, opts *InitOptions) error {
	dataDir := GetPgData(cfg)
	dbsu := GetDbSU(cfg)

	// Check data directory state as dbsu (handles permission issues for non-dbsu users)
	exists, initialized := CheckDataDirAsDBSU(dbsu, dataDir)
	if initialized {
		// Data directory already initialized - this is a dangerous operation
		if opts == nil || !opts.Force {
			return fmt.Errorf("data directory %s already initialized, use --force to overwrite (DANGEROUS)", dataDir)
		}

		// Force mode: check if PostgreSQL is running (NEVER allow overwrite if running)
		running, pid := CheckPostgresRunningAsDBSU(dbsu, dataDir)
		if running {
			return fmt.Errorf("postgresql is running (PID %d) in %s, cannot overwrite running database", pid, dataDir)
		}

		// Safe to remove: not running, user confirmed with --force
		logrus.Warnf("removing existing data directory: %s", dataDir)
		rmArgs := []string{"rm", "-rf", dataDir}
		PrintHint(rmArgs)
		if err := utils.DBSUCommand(dbsu, rmArgs); err != nil {
			return fmt.Errorf("failed to remove existing data directory: %w", err)
		}
		exists = false // directory removed, treat as non-existent
	}

	// Find PostgreSQL (handle nil cfg)
	pgVer := 0
	if cfg != nil {
		pgVer = cfg.PgVersion
	}
	pg, err := ext.FindPostgres(pgVer)
	if err != nil {
		return fmt.Errorf("postgresql not found: %w", err)
	}

	// Build initdb command
	cmdArgs := []string{pg.Initdb(), "-D", dataDir}

	// Encoding (default UTF8)
	enc := DefaultEncoding
	if opts != nil && opts.Encoding != "" {
		enc = opts.Encoding
	}
	cmdArgs = append(cmdArgs, "-E", enc)

	// Locale (default C)
	loc := DefaultLocale
	if opts != nil && opts.Locale != "" {
		loc = opts.Locale
	}
	cmdArgs = append(cmdArgs, "--locale="+loc)

	// Data checksums
	if opts != nil && opts.Checksum {
		cmdArgs = append(cmdArgs, "-k")
	}

	// Extra arguments (after --)
	if opts != nil && len(opts.ExtraArgs) > 0 {
		cmdArgs = append(cmdArgs, opts.ExtraArgs...)
	}

	// Create data directory if needed
	if !exists {
		logrus.Infof("creating directory: %s", dataDir)
		mkdirArgs := []string{"mkdir", "-p", dataDir}
		PrintHint(mkdirArgs)
		if err := utils.DBSUCommand(dbsu, mkdirArgs); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
	}

	logrus.Infof("initializing PostgreSQL %d: %s", pg.MajorVersion, dataDir)
	PrintHint(cmdArgs)
	return utils.DBSUCommand(dbsu, cmdArgs)
}

// ============================================================================
// StartOptions for pg start command
// ============================================================================

// StartOptions contains options for Start
type StartOptions struct {
	LogFile string
	Timeout int
	NoWait  bool
	Options string
	Force   bool
}

// Start starts PostgreSQL server using pg_ctl
func Start(cfg *Config, opts *StartOptions) error {
	dataDir := GetPgData(cfg)
	dbsu := GetDbSU(cfg)

	// Get timeout: opts.Timeout > $PGCTLTIMEOUT > DefaultTimeout
	optTimeout := 0
	if opts != nil {
		optTimeout = opts.Timeout
	}
	timeout := GetTimeout(optTimeout)

	// Check data directory as dbsu (handles permission issues for non-dbsu users)
	_, initialized := CheckDataDirAsDBSU(dbsu, dataDir)
	if !initialized {
		return fmt.Errorf("data directory %s not initialized (run 'pig pg init' first)", dataDir)
	}

	// Check if PostgreSQL is already running as dbsu
	running, pid := CheckPostgresRunningAsDBSU(dbsu, dataDir)
	if running {
		fmt.Printf("%sWARNING: PostgreSQL is already running (PID: %d) in %s%s\n",
			utils.ColorYellow, pid, dataDir, utils.ColorReset)
		if opts == nil || !opts.Force {
			fmt.Printf("%sUse -y to force start anyway%s\n", utils.ColorYellow, utils.ColorReset)
			return fmt.Errorf("postgresql already running, use -y to force")
		}
		fmt.Printf("%sForcing start as requested (-y)%s\n", utils.ColorYellow, utils.ColorReset)
	}

	// Find PostgreSQL
	pg, err := GetPgInstall(cfg)
	if err != nil {
		return fmt.Errorf("postgresql not found: %w", err)
	}

	// Build pg_ctl start command
	cmdArgs := []string{pg.PgCtl(), "start", "-D", dataDir}

	// Log file: only add -l if user explicitly specified it
	// PostgreSQL instances typically have their own log configuration
	if opts != nil && opts.LogFile != "" {
		cmdArgs = append(cmdArgs, "-l", opts.LogFile)
		// Ensure log directory exists
		if idx := strings.LastIndex(opts.LogFile, "/"); idx > 0 {
			logDir := opts.LogFile[:idx]
			if err := utils.DBSUCommand(dbsu, []string{"mkdir", "-p", logDir}); err != nil {
				logrus.Warnf("failed to create log directory %s: %v", logDir, err)
			}
		}
	}

	// Wait options
	if opts != nil && opts.NoWait {
		cmdArgs = append(cmdArgs, "-W")
	} else {
		cmdArgs = append(cmdArgs, "-w", "-t", strconv.Itoa(timeout))
	}

	// Postgres options
	if opts != nil && opts.Options != "" {
		cmdArgs = append(cmdArgs, "-o", opts.Options)
	}

	logrus.Infof("starting PostgreSQL %d: %s", pg.MajorVersion, dataDir)
	PrintHint(cmdArgs)
	return utils.DBSUCommand(dbsu, cmdArgs)
}

// ============================================================================
// StopOptions for pg stop command
// ============================================================================

// StopOptions contains options for Stop
type StopOptions struct {
	Mode    string
	Timeout int
	NoWait  bool
}

// Stop stops PostgreSQL server using pg_ctl
func Stop(cfg *Config, opts *StopOptions) error {
	dataDir := GetPgData(cfg)
	dbsu := GetDbSU(cfg)

	// Get timeout: opts.Timeout > $PGCTLTIMEOUT > DefaultTimeout
	optTimeout := 0
	if opts != nil {
		optTimeout = opts.Timeout
	}
	timeout := GetTimeout(optTimeout)

	// Validate stop mode
	mode := DefaultStopMode
	if opts != nil && opts.Mode != "" {
		mode = strings.ToLower(opts.Mode)
	}
	if mode != "smart" && mode != "fast" && mode != "immediate" {
		return fmt.Errorf("invalid stop mode: %s (use smart/fast/immediate)", mode)
	}

	// Find PostgreSQL
	pg, err := GetPgInstall(cfg)
	if err != nil {
		return fmt.Errorf("postgresql not found: %w", err)
	}

	// Build pg_ctl stop command
	cmdArgs := []string{pg.PgCtl(), "stop", "-D", dataDir, "-m", mode}

	// Wait options
	if opts != nil && opts.NoWait {
		cmdArgs = append(cmdArgs, "-W")
	} else {
		cmdArgs = append(cmdArgs, "-w", "-t", strconv.Itoa(timeout))
	}

	logrus.Infof("stopping PostgreSQL %d (%s): %s", pg.MajorVersion, mode, dataDir)
	PrintHint(cmdArgs)
	return utils.DBSUCommand(dbsu, cmdArgs)
}

// ============================================================================
// RestartOptions for pg restart command
// ============================================================================

// RestartOptions contains options for Restart
type RestartOptions struct {
	Mode    string
	Timeout int
	NoWait  bool
	Options string
}

// Restart restarts PostgreSQL server using pg_ctl
func Restart(cfg *Config, opts *RestartOptions) error {
	dataDir := GetPgData(cfg)
	dbsu := GetDbSU(cfg)

	// Get timeout: opts.Timeout > $PGCTLTIMEOUT > DefaultTimeout
	optTimeout := 0
	if opts != nil {
		optTimeout = opts.Timeout
	}
	timeout := GetTimeout(optTimeout)

	// Validate stop mode
	mode := DefaultStopMode
	if opts != nil && opts.Mode != "" {
		mode = strings.ToLower(opts.Mode)
	}
	if mode != "smart" && mode != "fast" && mode != "immediate" {
		return fmt.Errorf("invalid stop mode: %s (use smart/fast/immediate)", mode)
	}

	// Find PostgreSQL
	pg, err := GetPgInstall(cfg)
	if err != nil {
		return fmt.Errorf("postgresql not found: %w", err)
	}

	// Build pg_ctl restart command
	cmdArgs := []string{pg.PgCtl(), "restart", "-D", dataDir, "-m", mode}

	// Wait options
	if opts != nil && opts.NoWait {
		cmdArgs = append(cmdArgs, "-W")
	} else {
		cmdArgs = append(cmdArgs, "-w", "-t", strconv.Itoa(timeout))
	}

	// Postgres options
	if opts != nil && opts.Options != "" {
		cmdArgs = append(cmdArgs, "-o", opts.Options)
	}

	logrus.Infof("restarting PostgreSQL %d (%s): %s", pg.MajorVersion, mode, dataDir)
	PrintHint(cmdArgs)
	return utils.DBSUCommand(dbsu, cmdArgs)
}

// Reload reloads PostgreSQL configuration using pg_ctl
func Reload(cfg *Config) error {
	dataDir := GetPgData(cfg)
	dbsu := GetDbSU(cfg)

	// Find PostgreSQL
	pg, err := GetPgInstall(cfg)
	if err != nil {
		return fmt.Errorf("postgresql not found: %w", err)
	}

	// Build pg_ctl reload command
	cmdArgs := []string{pg.PgCtl(), "reload", "-D", dataDir}

	logrus.Infof("reloading PostgreSQL %d: %s", pg.MajorVersion, dataDir)
	PrintHint(cmdArgs)
	return utils.DBSUCommand(dbsu, cmdArgs)
}

// Status shows PostgreSQL server status
func Status(cfg *Config) error {
	dataDir := GetPgData(cfg)
	dbsu := GetDbSU(cfg)

	fmt.Printf("%s══════════════════════════════════════════════════════════════════%s\n", utils.ColorCyan, utils.ColorReset)
	fmt.Printf("%s PostgreSQL Status Summary%s\n", utils.ColorBold, utils.ColorReset)
	fmt.Printf("%s══════════════════════════════════════════════════════════════════%s\n", utils.ColorCyan, utils.ColorReset)

	// 1. pg_ctl status
	fmt.Printf("\n%s[pg_ctl status]%s\n", utils.ColorBold, utils.ColorReset)
	pg, err := GetPgInstall(cfg)
	if err != nil {
		fmt.Printf("  %sPostgreSQL not found: %v%s\n", utils.ColorYellow, err, utils.ColorReset)
	} else {
		cmdArgs := []string{pg.PgCtl(), "status", "-D", dataDir}
		PrintHint(cmdArgs)
		RunCommandQuiet(dbsu, cmdArgs)
	}

	// 2. PostgreSQL processes (ps)
	fmt.Printf("\n%s[PostgreSQL Processes]%s\n", utils.ColorBold, utils.ColorReset)
	PrintHint([]string{"ps", "-u", dbsu, "-o", "pid,ppid,start,command"})
	showPostgresProcesses(dbsu)

	// 3. Related services (systemctl)
	fmt.Printf("\n%s[Related Services]%s\n", utils.ColorBold, utils.ColorReset)
	showServiceStatus("postgres") // pigsty postgres service name
	showServiceStatus("patroni")
	showServiceStatus("pgbouncer")
	showServiceStatus("pgbackrest")
	showServiceStatus("vip-manager")
	showServiceStatus("haproxy")

	fmt.Printf("\n%s══════════════════════════════════════════════════════════════════%s\n", utils.ColorCyan, utils.ColorReset)
	return nil
}

// showPostgresProcesses shows postgres processes for DBSU user
func showPostgresProcesses(dbsu string) {
	cmd := exec.Command("ps", "-u", dbsu, "-o", "pid,ppid,start,command")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		fmt.Printf("  %s(no processes found)%s\n", utils.ColorYellow, utils.ColorReset)
		return
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) <= 1 {
		fmt.Printf("  %s(no processes found)%s\n", utils.ColorYellow, utils.ColorReset)
		return
	}

	// Print header
	if len(lines) > 0 {
		fmt.Printf("  %s\n", lines[0])
	}

	// Filter and print postgres-related processes
	count := 0
	for _, line := range lines[1:] {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "postgres") || strings.Contains(lower, "postmaster") {
			fmt.Printf("  %s\n", line)
			count++
		}
	}

	if count == 0 {
		fmt.Printf("  %s(no postgres processes)%s\n", utils.ColorYellow, utils.ColorReset)
	}
}

// showServiceStatus shows systemd service status (silent on error)
func showServiceStatus(serviceName string) {
	// Check if systemctl exists
	if _, err := exec.LookPath("systemctl"); err != nil {
		return
	}

	cmd := exec.Command("systemctl", "is-active", serviceName)
	output, _ := cmd.Output()
	status := strings.TrimSpace(string(output))

	// Determine color based on status
	var statusColor string
	switch status {
	case "active":
		statusColor = utils.ColorGreen
	case "inactive":
		statusColor = utils.ColorYellow
	case "failed":
		statusColor = utils.ColorRed
	default:
		// Service doesn't exist or unknown status, skip silently
		if status == "" || status == "unknown" {
			return
		}
		statusColor = utils.ColorYellow
	}

	fmt.Printf("  %-16s %s%s%s\n", serviceName+":", statusColor, status, utils.ColorReset)
}

// PromoteOptions contains options for Promote
type PromoteOptions struct {
	Timeout int
	NoWait  bool
}

// Promote promotes standby to primary
func Promote(cfg *Config, opts *PromoteOptions) error {
	dataDir := GetPgData(cfg)
	dbsu := GetDbSU(cfg)

	// Get timeout: opts.Timeout > $PGCTLTIMEOUT > DefaultTimeout
	optTimeout := 0
	if opts != nil {
		optTimeout = opts.Timeout
	}
	timeout := GetTimeout(optTimeout)

	// Find PostgreSQL
	pg, err := GetPgInstall(cfg)
	if err != nil {
		return fmt.Errorf("postgresql not found: %w", err)
	}

	// Build pg_ctl promote command
	cmdArgs := []string{pg.PgCtl(), "promote", "-D", dataDir}

	// Wait options
	if opts != nil && opts.NoWait {
		cmdArgs = append(cmdArgs, "-W")
	} else {
		cmdArgs = append(cmdArgs, "-w", "-t", strconv.Itoa(timeout))
	}

	logrus.Infof("promoting PostgreSQL %d: %s", pg.MajorVersion, dataDir)
	PrintHint(cmdArgs)
	return utils.DBSUCommand(dbsu, cmdArgs)
}
