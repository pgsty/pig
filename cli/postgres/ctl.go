/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

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
	"pig/internal/config"
	"pig/internal/utils"

	"github.com/sirupsen/logrus"
)

// ============================================================================
// InitOptions for pg init command
// ============================================================================

// InitOptions contains options for InitDB
type InitOptions struct {
	Encoding        string
	Locale          string
	Checksum        bool
	NoDataChecksums bool
	Force           bool // Force init, remove existing data directory (DANGEROUS)
	ExtraArgs       []string
}

// InitDBSettings describes the effective initdb policy selected by pig pg init.
type InitDBSettings struct {
	Encoding       string
	LocaleProvider string
	Locale         string
	DataChecksums  bool
	Warnings       []string
}

// InitDB initializes a PostgreSQL data directory
func InitDB(cfg *Config, opts *InitOptions) error {
	if err := ValidateInitOptions(opts); err != nil {
		return err
	}

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

	cmdArgs, settings := buildInitDBArgs(pg.Initdb(), dataDir, pg.MajorVersion, opts, detectInitDBLocaleAvailable())
	if !config.IsStructuredOutput() {
		for _, warning := range settings.Warnings {
			utils.PrintWarn("%s", warning)
		}
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

// ValidateInitOptions rejects initdb options that would override pig pg init policy.
func ValidateInitOptions(opts *InitOptions) error {
	if opts == nil {
		return nil
	}
	switch {
	case opts.Encoding != "":
		return newInitPolicyError("--encoding/-E")
	case opts.Locale != "":
		return newInitPolicyError("--locale")
	case opts.Checksum:
		return newInitPolicyError("--data-checksum/-k")
	}
	for _, arg := range opts.ExtraArgs {
		if blocked := blockedInitDBPolicyArg(arg); blocked != "" {
			return newInitPolicyError(blocked)
		}
	}
	return nil
}

func newInitPolicyError(option string) error {
	return fmt.Errorf("pig pg init does not accept %s; use initdb directly for custom locale, encoding, or checksum settings", option)
}

func blockedInitDBPolicyArg(arg string) string {
	if arg == "" {
		return ""
	}
	lower := strings.ToLower(arg)
	name := lower
	if idx := strings.Index(name, "="); idx >= 0 {
		name = name[:idx]
	}
	switch {
	case lower == "-e" || strings.HasPrefix(lower, "-e"):
		return arg
	case lower == "-k":
		return arg
	case name == "--encoding",
		name == "--locale",
		name == "--locale-provider",
		name == "--builtin-locale",
		name == "--icu-locale",
		name == "--icu-rules",
		name == "--data-checksum",
		name == "--data-checksums",
		name == "--no-data-checksums":
		return arg
	case strings.HasPrefix(name, "--lc-"):
		return arg
	}
	return ""
}

func buildInitDBArgs(initdbPath, dataDir string, pgVersion int, opts *InitOptions, localeAvailable bool) ([]string, InitDBSettings) {
	settings := InitDBSettings{
		Encoding:      DefaultEncoding,
		DataChecksums: true,
	}
	settings.LocaleProvider, settings.Locale, settings.Warnings = selectInitDBLocale(pgVersion, localeAvailable)
	if opts != nil && opts.NoDataChecksums {
		settings.DataChecksums = false
	}

	cmdArgs := []string{initdbPath, "-D", dataDir, "--encoding=" + settings.Encoding}
	if settings.LocaleProvider != "" {
		cmdArgs = append(cmdArgs, "--locale-provider="+settings.LocaleProvider)
	}
	cmdArgs = append(cmdArgs, "--locale="+settings.Locale)

	switch {
	case settings.DataChecksums && pgVersion < 18:
		cmdArgs = append(cmdArgs, "--data-checksums")
	case !settings.DataChecksums && pgVersion >= 18:
		cmdArgs = append(cmdArgs, "--no-data-checksums")
	}
	if opts != nil && len(opts.ExtraArgs) > 0 {
		cmdArgs = append(cmdArgs, opts.ExtraArgs...)
	}
	return cmdArgs, settings
}

func selectInitDBLocale(pgVersion int, localeAvailable bool) (provider string, locale string, warnings []string) {
	if pgVersion >= 17 {
		return "builtin", "C.UTF-8", nil
	}
	if localeAvailable {
		return "", "C.UTF-8", nil
	}
	return "", DefaultLocale, []string{"C.UTF-8 locale is unavailable; falling back to C locale"}
}

func detectInitDBLocaleAvailable() bool {
	out, err := utils.ShellOutput("locale", "-a")
	if err != nil {
		return false
	}
	lower := strings.ToLower(out)
	return strings.Contains(lower, "c.utf8") || strings.Contains(lower, "c.utf-8")
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
}

// test seams for start/stop state checks (allow stubbing in unit tests)
var (
	ctlCheckDataDir = CheckDataDirAsDBSU
	ctlCheckRunning = CheckPostgresRunningAsDBSU
)

const DefaultStartLogFile = "/tmp/pig-pg-start.log"

func buildStartArgs(pgCtl, dataDir string, timeout int, opts *StartOptions) []string {
	cmdArgs := []string{pgCtl, "start", "-D", dataDir}

	logFile := DefaultStartLogFile
	if opts != nil && opts.LogFile != "" {
		logFile = opts.LogFile
	}
	cmdArgs = append(cmdArgs, "-l", logFile)

	if opts != nil && opts.NoWait {
		cmdArgs = append(cmdArgs, "-W")
	} else {
		cmdArgs = append(cmdArgs, "-w", "-t", strconv.Itoa(timeout))
	}

	if opts != nil && opts.Options != "" {
		cmdArgs = append(cmdArgs, "-o", opts.Options)
	}

	return cmdArgs
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
	_, initialized := ctlCheckDataDir(dbsu, dataDir)
	if !initialized {
		return fmt.Errorf("data directory %s not initialized (run 'pig pg init' first)", dataDir)
	}

	// Idempotent success (B06/B22): already running is not an error
	running, pid := ctlCheckRunning(dbsu, dataDir)
	if running {
		if config.IsStructuredOutput() {
			utils.PrintInfo("PostgreSQL is already running (pid %d)", pid)
		} else {
			fmt.Printf("PostgreSQL is already running (pid %d)\n", pid)
		}
		return nil
	}

	// Find PostgreSQL
	pg, err := GetPgInstall(cfg)
	if err != nil {
		return fmt.Errorf("postgresql not found: %w", err)
	}

	logFile := DefaultStartLogFile
	if opts != nil && opts.LogFile != "" {
		logFile = opts.LogFile
		if idx := strings.LastIndex(logFile, "/"); idx > 0 {
			logDir := logFile[:idx]
			if err := utils.DBSUCommand(dbsu, []string{"mkdir", "-p", logDir}); err != nil {
				logrus.Warnf("failed to create log directory %s: %v", logDir, err)
			}
		}
	}

	cmdArgs := buildStartArgs(pg.PgCtl(), dataDir, timeout, opts)
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

	// Idempotent success (B22): already stopped is not an error
	running, _ := ctlCheckRunning(dbsu, dataDir)
	if !running {
		if config.IsStructuredOutput() {
			utils.PrintInfo("PostgreSQL is already stopped")
		} else {
			fmt.Println("PostgreSQL is already stopped")
		}
		return nil
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
	warnPatroniLifecycleRisk("stop", dataDir)
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

	running, _, _, err := ctlCheckRunningState(dbsu, dataDir)
	if err != nil {
		return fmt.Errorf("failed to check PostgreSQL status in %s: %w", dataDir, err)
	}
	if !running {
		return fmt.Errorf("postgresql is not running in %s; use 'pig pg start' to start a stopped server", dataDir)
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
	warnPatroniLifecycleRisk("restart", dataDir)
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

// PatroniActive reports whether the local Patroni systemd service is active.
func PatroniActive() bool {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return false
	}
	return exec.Command("systemctl", "is-active", "--quiet", "patroni").Run() == nil
}

func warnPatroniLifecycleRisk(action, dataDir string) {
	if !PatroniActive() {
		return
	}
	utils.PrintWarn("%s", patroniLifecycleRiskWarning(action, dataDir))
}

func patroniLifecycleRiskWarning(action, dataDir string) string {
	if dataDir == "" {
		dataDir = DefaultPgData
	}
	return fmt.Sprintf("Patroni is active; pig pg %s uses pg_ctl directly on %s and does not coordinate DCS, failover, or client routing. Prefer pig pt switchover/failover or pig pitr when Patroni manages this PGDATA", action, dataDir)
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
	warnPatroniLifecycleRisk("promote", dataDir)
	PrintHint(cmdArgs)
	return utils.DBSUCommand(dbsu, cmdArgs)
}
