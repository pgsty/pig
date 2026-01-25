/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>
*/

package cmd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"pig/cli/ext"
	"pig/internal/utils"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// ============================================================================
// Default Constants
// ============================================================================

const (
	DefaultPgData   = "/pg/data"
	DefaultPgLog    = "/pg/log/postgres.log"
	DefaultTimeout  = 60
	DefaultStopMode = "fast"
	DefaultEncoding = "UTF8"
	DefaultLocale   = "C"
)

// ANSI color codes
const (
	ColorReset  = "\033[0m"
	ColorBlue   = "\033[34m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorRed    = "\033[31m"
	ColorCyan   = "\033[36m"
	ColorBold   = "\033[1m"
)

// ============================================================================
// Global Flags (shared by all pg subcommands)
// ============================================================================

var (
	pgVersion int    // -v, --version: PostgreSQL major version
	pgData    string // -D, --data: data directory
	pgDbsu    string // --dbsu: database superuser
	pgSystemd bool   // -S, --systemd: use systemctl instead of pg_ctl
)

const DefaultSystemdService = "postgresql"

// ============================================================================
// Helper Functions
// ============================================================================

// getPgData returns data directory: flag > default (no PGDATA env support)
func getPgData() string {
	if pgData != "" {
		return pgData
	}
	return DefaultPgData
}

// getTimeout returns timeout: flag > $PGCTLTIMEOUT > default
func getTimeout(flag int) int {
	if flag > 0 {
		return flag
	}
	if env := os.Getenv("PGCTLTIMEOUT"); env != "" {
		if t, err := strconv.Atoi(env); err == nil && t > 0 {
			return t
		}
	}
	return DefaultTimeout
}

// getPgInstall finds PostgreSQL installation, optionally inferring version from data dir
func getPgInstall(dataDir string) (*ext.PostgresInstall, error) {
	ver := pgVersion
	if ver == 0 && dataDir != "" {
		if v, err := readPgVersion(dataDir); err == nil {
			ver = v
			logrus.Debugf("inferred PostgreSQL %d from %s", ver, dataDir)
		}
	}
	return ext.FindPostgres(ver)
}

// readPgVersion reads major version from PG_VERSION file
func readPgVersion(dataDir string) (int, error) {
	data, err := os.ReadFile(filepath.Join(dataDir, "PG_VERSION"))
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

// checkDataDir checks if data directory exists and is initialized
func checkDataDir(dataDir string) (exists, initialized bool) {
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

// checkPostgresRunning checks if PostgreSQL is running in the data directory
// Returns (running bool, pid int, err error)
func checkPostgresRunning(dataDir string) (bool, int, error) {
	pidFile := filepath.Join(dataDir, "postmaster.pid")
	data, err := os.ReadFile(pidFile)
	if os.IsNotExist(err) {
		return false, 0, nil // No pid file, not running
	}
	if err != nil {
		return false, 0, err
	}

	// First line of postmaster.pid is the PID
	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 {
		return false, 0, nil
	}

	pid, err := strconv.Atoi(strings.TrimSpace(lines[0]))
	if err != nil {
		return false, 0, nil // Invalid PID, assume not running
	}

	// Check if process exists
	process, err := os.FindProcess(pid)
	if err != nil {
		return false, pid, nil // Can't find process
	}

	// On Unix, FindProcess always succeeds. Send signal 0 to check if process exists
	err = process.Signal(syscall.Signal(0))
	if err != nil {
		return false, pid, nil // Process doesn't exist (stale pid file)
	}

	return true, pid, nil
}

// printHint prints command hint in blue color
func printHint(cmdArgs []string) {
	fmt.Printf("%sHINT: %s%s\n", ColorBlue, strings.Join(cmdArgs, " "), ColorReset)
}

// runSystemctl runs systemctl command as root (via sudo if needed)
func runSystemctl(action, service string) error {
	cmdArgs := []string{"systemctl", action, service}
	printHint(cmdArgs)

	var cmd *exec.Cmd
	if os.Geteuid() == 0 {
		// Already root
		cmd = exec.Command(cmdArgs[0], cmdArgs[1:]...)
	} else {
		// Use sudo
		cmd = exec.Command("sudo", cmdArgs...)
	}

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("systemctl %s failed: %w", action, err)
	}
	return nil
}

// ============================================================================
// Main Command: pig pg
// ============================================================================

var pgCmd = &cobra.Command{
	Use:     "pg",
	Short:   "Manage PostgreSQL Servers",
	Aliases: []string{"p", "pgsql", "postgres"},
	GroupID: "pgext",
	Long: `pig pg - PostgreSQL Server Management (pg_ctl wrapper)

  pig pg init     [-v <version>] [-D <datadir>]   # initialize data directory
  pig pg start    [-D <datadir>] [-l <logfile>]   # start server
  pig pg stop     [-D <datadir>] [-m fast]        # stop server
  pig pg restart  [-D <datadir>] [-m fast]        # restart server
  pig pg reload   [-D <datadir>]                  # reload configuration
  pig pg status   [-D <datadir>]                  # show server status
  pig pg promote  [-D <datadir>]                  # promote standby to primary

Version Detection Priority:
  1. -v/--version flag
  2. Infer from PGDATA/PG_VERSION
  3. pg_ctl in PATH
  4. /usr/pgsql (set by 'pig ext link')
  5. Latest installed version
`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if err := initAll(); err != nil {
			return err
		}
		// Pre-detect PostgreSQL installations
		if err := ext.DetectPostgres(); err != nil {
			logrus.Debugf("DetectPostgres: %v", err)
		}
		return nil
	},
}

// ============================================================================
// Subcommand: pig pg init
// ============================================================================

var (
	pgInitEncoding string
	pgInitLocale   string
	pgInitChecksum bool
)

var pgInitCmd = &cobra.Command{
	Use:   "init [-- initdb-options...]",
	Short: "Initialize PostgreSQL data directory",
	Example: `  pig pg init                      # use default settings
  pig pg init -v 18                # use PostgreSQL 18
  pig pg init -D /data/pg18 -k     # specify datadir with checksums
  pig pg init -- --waldir=/wal     # pass extra options to initdb`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dataDir := getPgData()
		dbsu := utils.GetDBSU(pgDbsu)

		// Check data directory state
		exists, initialized := checkDataDir(dataDir)
		if initialized {
			return fmt.Errorf("data directory %s already initialized", dataDir)
		}

		// Find PostgreSQL
		pg, err := ext.FindPostgres(pgVersion)
		if err != nil {
			return fmt.Errorf("PostgreSQL not found: %w", err)
		}

		// Build initdb command
		cmdArgs := []string{pg.Initdb(), "-D", dataDir}

		// Encoding (default UTF8)
		enc := pgInitEncoding
		if enc == "" {
			enc = DefaultEncoding
		}
		cmdArgs = append(cmdArgs, "-E", enc)

		// Locale (default C)
		loc := pgInitLocale
		if loc == "" {
			loc = DefaultLocale
		}
		cmdArgs = append(cmdArgs, "--locale="+loc)

		// Data checksums
		if pgInitChecksum {
			cmdArgs = append(cmdArgs, "-k")
		}

		// Extra arguments (after --)
		cmdArgs = append(cmdArgs, args...)

		// Create data directory if needed
		if !exists {
			logrus.Infof("creating directory: %s", dataDir)
			mkdirArgs := []string{"mkdir", "-p", dataDir}
			printHint(mkdirArgs)
			if err := utils.DBSUCommand(dbsu, mkdirArgs); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
		}

		logrus.Infof("initializing PostgreSQL %d: %s", pg.MajorVersion, dataDir)
		printHint(cmdArgs)
		return utils.DBSUCommand(dbsu, cmdArgs)
	},
}

// ============================================================================
// Subcommand: pig pg start
// ============================================================================

var (
	pgStartLog     string
	pgStartTimeout int
	pgStartNoWait  bool
	pgStartOptions string
	pgStartYes     bool
)

var pgStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start PostgreSQL server",
	Example: `  pig pg start                     # start with defaults
  pig pg start -D /data/pg18       # specify data directory
  pig pg start -l /var/log/pg.log  # specify log file
  pig pg start -o "-p 5433"        # pass options to postgres
  pig pg start -y                  # force start (skip running check)
  pig pg start -S                  # use systemctl instead of pg_ctl`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Use systemctl if --systemd is specified
		if pgSystemd {
			logrus.Infof("starting PostgreSQL via systemctl")
			return runSystemctl("start", DefaultSystemdService)
		}

		dataDir := getPgData()
		dbsu := utils.GetDBSU(pgDbsu)
		timeout := getTimeout(pgStartTimeout)

		// Check data directory
		_, initialized := checkDataDir(dataDir)
		if !initialized {
			return fmt.Errorf("data directory %s not initialized (run 'pig pg init' first)", dataDir)
		}

		// Check if PostgreSQL is already running
		running, pid, err := checkPostgresRunning(dataDir)
		if err != nil {
			logrus.Warnf("failed to check running status: %v", err)
		}
		if running {
			fmt.Printf("%sWARNING: PostgreSQL is already running (PID: %d) in %s%s\n",
				ColorYellow, pid, dataDir, ColorReset)
			if !pgStartYes {
				fmt.Printf("%sUse -y to force start anyway%s\n", ColorYellow, ColorReset)
				return fmt.Errorf("PostgreSQL already running, use -y to force")
			}
			fmt.Printf("%sForcing start as requested (-y)%s\n", ColorYellow, ColorReset)
		}

		// Find PostgreSQL
		pg, err := getPgInstall(dataDir)
		if err != nil {
			return fmt.Errorf("PostgreSQL not found: %w", err)
		}

		// Build pg_ctl start command
		cmdArgs := []string{pg.PgCtl(), "start", "-D", dataDir}

		// Log file (default /pg/log/postgres.log)
		logFile := pgStartLog
		if logFile == "" {
			logFile = DefaultPgLog
		}
		cmdArgs = append(cmdArgs, "-l", logFile)

		// Wait options
		if pgStartNoWait {
			cmdArgs = append(cmdArgs, "-W")
		} else {
			cmdArgs = append(cmdArgs, "-w", "-t", strconv.Itoa(timeout))
		}

		// Postgres options
		if pgStartOptions != "" {
			cmdArgs = append(cmdArgs, "-o", pgStartOptions)
		}

		// Ensure log directory exists
		logDir := filepath.Dir(logFile)
		_ = utils.DBSUCommand(dbsu, []string{"mkdir", "-p", logDir})

		logrus.Infof("starting PostgreSQL %d: %s", pg.MajorVersion, dataDir)
		printHint(cmdArgs)
		return utils.DBSUCommand(dbsu, cmdArgs)
	},
}

// ============================================================================
// Subcommand: pig pg stop
// ============================================================================

var (
	pgStopMode    string
	pgStopTimeout int
	pgStopNoWait  bool
)

var pgStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop PostgreSQL server",
	Example: `  pig pg stop                      # fast stop (default)
  pig pg stop -m smart             # wait for clients to disconnect
  pig pg stop -m immediate         # immediate shutdown
  pig pg stop -S                   # use systemctl instead of pg_ctl`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Use systemctl if --systemd is specified
		if pgSystemd {
			logrus.Infof("stopping PostgreSQL via systemctl")
			return runSystemctl("stop", DefaultSystemdService)
		}

		dataDir := getPgData()
		dbsu := utils.GetDBSU(pgDbsu)
		timeout := getTimeout(pgStopTimeout)

		// Validate stop mode
		mode := strings.ToLower(pgStopMode)
		if mode == "" {
			mode = DefaultStopMode
		}
		if mode != "smart" && mode != "fast" && mode != "immediate" {
			return fmt.Errorf("invalid stop mode: %s (use smart/fast/immediate)", mode)
		}

		// Find PostgreSQL
		pg, err := getPgInstall(dataDir)
		if err != nil {
			return fmt.Errorf("PostgreSQL not found: %w", err)
		}

		// Build pg_ctl stop command
		cmdArgs := []string{pg.PgCtl(), "stop", "-D", dataDir, "-m", mode}

		// Wait options
		if pgStopNoWait {
			cmdArgs = append(cmdArgs, "-W")
		} else {
			cmdArgs = append(cmdArgs, "-w", "-t", strconv.Itoa(timeout))
		}

		logrus.Infof("stopping PostgreSQL %d (%s): %s", pg.MajorVersion, mode, dataDir)
		printHint(cmdArgs)
		return utils.DBSUCommand(dbsu, cmdArgs)
	},
}

// ============================================================================
// Subcommand: pig pg restart
// ============================================================================

var (
	pgRestartMode    string
	pgRestartTimeout int
	pgRestartNoWait  bool
	pgRestartOptions string
)

var pgRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart PostgreSQL server",
	Example: `  pig pg restart                   # fast restart
  pig pg restart -m immediate      # immediate restart
  pig pg restart -o "-p 5433"      # restart with new options
  pig pg restart -S                # use systemctl instead of pg_ctl`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Use systemctl if --systemd is specified
		if pgSystemd {
			logrus.Infof("restarting PostgreSQL via systemctl")
			return runSystemctl("restart", DefaultSystemdService)
		}

		dataDir := getPgData()
		dbsu := utils.GetDBSU(pgDbsu)
		timeout := getTimeout(pgRestartTimeout)

		// Validate stop mode
		mode := strings.ToLower(pgRestartMode)
		if mode == "" {
			mode = DefaultStopMode
		}
		if mode != "smart" && mode != "fast" && mode != "immediate" {
			return fmt.Errorf("invalid stop mode: %s (use smart/fast/immediate)", mode)
		}

		// Find PostgreSQL
		pg, err := getPgInstall(dataDir)
		if err != nil {
			return fmt.Errorf("PostgreSQL not found: %w", err)
		}

		// Build pg_ctl restart command
		cmdArgs := []string{pg.PgCtl(), "restart", "-D", dataDir, "-m", mode}

		// Wait options
		if pgRestartNoWait {
			cmdArgs = append(cmdArgs, "-W")
		} else {
			cmdArgs = append(cmdArgs, "-w", "-t", strconv.Itoa(timeout))
		}

		// Postgres options
		if pgRestartOptions != "" {
			cmdArgs = append(cmdArgs, "-o", pgRestartOptions)
		}

		logrus.Infof("restarting PostgreSQL %d (%s): %s", pg.MajorVersion, mode, dataDir)
		printHint(cmdArgs)
		return utils.DBSUCommand(dbsu, cmdArgs)
	},
}

// ============================================================================
// Subcommand: pig pg reload
// ============================================================================

var pgReloadCmd = &cobra.Command{
	Use:   "reload",
	Short: "Reload PostgreSQL configuration",
	Example: `  pig pg reload                    # reload config (SIGHUP)
  pig pg reload -D /data/pg18      # specify data directory
  pig pg reload -S                 # use systemctl instead of pg_ctl`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Use systemctl if --systemd is specified
		if pgSystemd {
			logrus.Infof("reloading PostgreSQL via systemctl")
			return runSystemctl("reload", DefaultSystemdService)
		}

		dataDir := getPgData()
		dbsu := utils.GetDBSU(pgDbsu)

		// Find PostgreSQL
		pg, err := getPgInstall(dataDir)
		if err != nil {
			return fmt.Errorf("PostgreSQL not found: %w", err)
		}

		// Build pg_ctl reload command
		cmdArgs := []string{pg.PgCtl(), "reload", "-D", dataDir}

		logrus.Infof("reloading PostgreSQL %d: %s", pg.MajorVersion, dataDir)
		printHint(cmdArgs)
		return utils.DBSUCommand(dbsu, cmdArgs)
	},
}

// ============================================================================
// Subcommand: pig pg status
// ============================================================================

var pgStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show PostgreSQL server status",
	Example: `  pig pg status                    # check server status
  pig pg status -D /data/pg18      # specify data directory`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dataDir := getPgData()
		dbsu := utils.GetDBSU(pgDbsu)

		fmt.Printf("%s══════════════════════════════════════════════════════════════════%s\n", ColorCyan, ColorReset)
		fmt.Printf("%s PostgreSQL Status Summary%s\n", ColorBold, ColorReset)
		fmt.Printf("%s══════════════════════════════════════════════════════════════════%s\n", ColorCyan, ColorReset)

		// 1. pg_ctl status
		fmt.Printf("\n%s[pg_ctl status]%s\n", ColorBold, ColorReset)
		pg, err := getPgInstall(dataDir)
		if err != nil {
			fmt.Printf("  %sPostgreSQL not found: %v%s\n", ColorYellow, err, ColorReset)
		} else {
			cmdArgs := []string{pg.PgCtl(), "status", "-D", dataDir}
			printHint(cmdArgs)
			runCommandQuiet(dbsu, cmdArgs)
		}

		// 2. PostgreSQL processes (ps)
		fmt.Printf("\n%s[PostgreSQL Processes]%s\n", ColorBold, ColorReset)
		printHint([]string{"ps", "-u", dbsu, "-o", "pid,ppid,start,command"})
		showPostgresProcesses(dbsu)

		// 3. Related services (systemctl)
		fmt.Printf("\n%s[Related Services]%s\n", ColorBold, ColorReset)
		showServiceStatus("postgresql")
		showServiceStatus("patroni")
		showServiceStatus("pgbouncer")
		showServiceStatus("pgbackrest")
		showServiceStatus("vip-manager")
		showServiceStatus("haproxy")

		fmt.Printf("\n%s══════════════════════════════════════════════════════════════════%s\n", ColorCyan, ColorReset)
		return nil
	},
}

// runCommandQuiet runs a command and prints output, does not fail on error
func runCommandQuiet(dbsu string, args []string) {
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

// showPostgresProcesses shows postgres processes for DBSU user
func showPostgresProcesses(dbsu string) {
	cmd := exec.Command("ps", "-u", dbsu, "-o", "pid,ppid,start,command")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		fmt.Printf("  %s(no processes found)%s\n", ColorYellow, ColorReset)
		return
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) <= 1 {
		fmt.Printf("  %s(no processes found)%s\n", ColorYellow, ColorReset)
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
		fmt.Printf("  %s(no postgres processes)%s\n", ColorYellow, ColorReset)
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
		statusColor = ColorGreen
	case "inactive":
		statusColor = ColorYellow
	case "failed":
		statusColor = ColorRed
	default:
		// Service doesn't exist or unknown status, skip silently
		if status == "" || status == "unknown" {
			return
		}
		statusColor = ColorYellow
	}

	fmt.Printf("  %-16s %s%s%s\n", serviceName+":", statusColor, status, ColorReset)
}

// ============================================================================
// Subcommand: pig pg promote
// ============================================================================

var (
	pgPromoteTimeout int
	pgPromoteNoWait  bool
)

var pgPromoteCmd = &cobra.Command{
	Use:   "promote",
	Short: "Promote standby to primary",
	Example: `  pig pg promote                   # promote standby
  pig pg promote -D /data/pg18     # specify data directory`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dataDir := getPgData()
		dbsu := utils.GetDBSU(pgDbsu)
		timeout := getTimeout(pgPromoteTimeout)

		// Find PostgreSQL
		pg, err := getPgInstall(dataDir)
		if err != nil {
			return fmt.Errorf("PostgreSQL not found: %w", err)
		}

		// Build pg_ctl promote command
		cmdArgs := []string{pg.PgCtl(), "promote", "-D", dataDir}

		// Wait options
		if pgPromoteNoWait {
			cmdArgs = append(cmdArgs, "-W")
		} else {
			cmdArgs = append(cmdArgs, "-w", "-t", strconv.Itoa(timeout))
		}

		logrus.Infof("promoting PostgreSQL %d: %s", pg.MajorVersion, dataDir)
		printHint(cmdArgs)
		return utils.DBSUCommand(dbsu, cmdArgs)
	},
}

// ============================================================================
// Command Registration
// ============================================================================

func init() {
	// Global flags for all pg subcommands
	pgCmd.PersistentFlags().IntVarP(&pgVersion, "version", "v", 0, "PostgreSQL major version")
	pgCmd.PersistentFlags().StringVarP(&pgData, "data", "D", "", "data directory (default: /pg/data)")
	pgCmd.PersistentFlags().StringVar(&pgDbsu, "dbsu", "", "database superuser (default: $PIG_DBSU or postgres)")
	pgCmd.PersistentFlags().BoolVarP(&pgSystemd, "systemd", "S", false, "use systemctl instead of pg_ctl (run as root)")

	// init subcommand flags
	pgInitCmd.Flags().StringVarP(&pgInitEncoding, "encoding", "E", "", "database encoding (default: UTF8)")
	pgInitCmd.Flags().StringVar(&pgInitLocale, "locale", "", "locale setting (default: C)")
	pgInitCmd.Flags().BoolVarP(&pgInitChecksum, "data-checksum", "k", false, "enable data checksums")

	// start subcommand flags
	pgStartCmd.Flags().StringVarP(&pgStartLog, "log", "l", "", "log file (default: /pg/log/postgres.log)")
	pgStartCmd.Flags().IntVarP(&pgStartTimeout, "timeout", "t", 0, "wait timeout in seconds")
	pgStartCmd.Flags().BoolVarP(&pgStartNoWait, "no-wait", "W", false, "do not wait for startup")
	pgStartCmd.Flags().StringVarP(&pgStartOptions, "options", "o", "", "options passed to postgres")
	pgStartCmd.Flags().BoolVarP(&pgStartYes, "yes", "y", false, "force start even if already running")

	// stop subcommand flags
	pgStopCmd.Flags().StringVarP(&pgStopMode, "mode", "m", "fast", "shutdown mode: smart/fast/immediate")
	pgStopCmd.Flags().IntVarP(&pgStopTimeout, "timeout", "t", 0, "wait timeout in seconds")
	pgStopCmd.Flags().BoolVarP(&pgStopNoWait, "no-wait", "W", false, "do not wait for shutdown")

	// restart subcommand flags
	pgRestartCmd.Flags().StringVarP(&pgRestartMode, "mode", "m", "fast", "shutdown mode: smart/fast/immediate")
	pgRestartCmd.Flags().IntVarP(&pgRestartTimeout, "timeout", "t", 0, "wait timeout in seconds")
	pgRestartCmd.Flags().BoolVarP(&pgRestartNoWait, "no-wait", "W", false, "do not wait for restart")
	pgRestartCmd.Flags().StringVarP(&pgRestartOptions, "options", "o", "", "options passed to postgres")

	// promote subcommand flags
	pgPromoteCmd.Flags().IntVarP(&pgPromoteTimeout, "timeout", "t", 0, "wait timeout in seconds")
	pgPromoteCmd.Flags().BoolVarP(&pgPromoteNoWait, "no-wait", "W", false, "do not wait for promotion")

	// Register subcommands
	pgCmd.AddCommand(pgInitCmd)
	pgCmd.AddCommand(pgStartCmd)
	pgCmd.AddCommand(pgStopCmd)
	pgCmd.AddCommand(pgRestartCmd)
	pgCmd.AddCommand(pgReloadCmd)
	pgCmd.AddCommand(pgStatusCmd)
	pgCmd.AddCommand(pgPromoteCmd)
}
