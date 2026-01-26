package pgbackrest

import (
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"syscall"
	"time"

	"pig/cli/postgres"
	"pig/internal/utils"

	"github.com/sirupsen/logrus"
)

// RestoreOptions holds options for restore command.
type RestoreOptions struct {
	// Recovery targets (mutually exclusive)
	Default   bool   // Recover to end of WAL stream (latest)
	Immediate bool   // Recover to backup consistency point
	Time      string // Recover to specific timestamp
	Name      string // Recover to named restore point
	LSN       string // Recover to specific LSN
	XID       string // Recover to specific transaction ID

	// Backup set selection (can be combined with recovery targets)
	Set string // Recover from specific backup set

	// Other options
	DataDir   string // Target data directory
	Exclusive bool   // Stop before target (exclusive)
	Promote   bool   // Promote after reaching target (target-action=promote)
	Yes       bool   // Skip confirmation and countdown
}

// Pre-compiled regex patterns for validation
var (
	lsnRegex      = regexp.MustCompile(`^[0-9A-Fa-f]+/[0-9A-Fa-f]+$`)
	dateOnlyRegex = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	timeOnlyRegex = regexp.MustCompile(`^\d{2}:\d{2}:\d{2}$`)
	dateTimeRegex = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}`)
)

// Restore performs point-in-time recovery (PITR).
func Restore(cfg *Config, opts *RestoreOptions) error {
	effCfg, err := GetEffectiveConfig(cfg)
	if err != nil {
		return err
	}

	if err := validateRestoreOptions(opts); err != nil {
		return err
	}

	normalizedTime := normalizeTime(opts.Time)

	// Check PostgreSQL is stopped
	if err := checkPostgresStopped(effCfg, opts.DataDir); err != nil {
		return err
	}

	args := buildRestoreArgs(effCfg, opts, normalizedTime)
	printRestorePlan(effCfg, opts, normalizedTime)

	// Confirmation with signal handling
	if !opts.Yes {
		if err := confirmWithCountdown(
			fmt.Sprintf("This will overwrite data in %s", getDataDir(effCfg, opts.DataDir)),
			"restore",
		); err != nil {
			return err
		}
	}

	if err := RunPgBackRest(effCfg, "restore", args, true); err != nil {
		return err
	}

	printPostRestoreHints(opts)
	return nil
}

// validateRestoreOptions validates restore parameters.
func validateRestoreOptions(opts *RestoreOptions) error {
	// Check mutually exclusive targets (Set is NOT a target, can be combined)
	targets := 0
	if opts.Default {
		targets++
	}
	if opts.Immediate {
		targets++
	}
	if opts.Time != "" {
		targets++
	}
	if opts.Name != "" {
		targets++
	}
	if opts.LSN != "" {
		targets++
	}
	if opts.XID != "" {
		targets++
	}

	if targets > 1 {
		return fmt.Errorf("multiple recovery targets specified, choose only one of: --default, --immediate, --time, --name, --lsn, --xid")
	}

	if opts.LSN != "" && !lsnRegex.MatchString(opts.LSN) {
		return fmt.Errorf("invalid LSN format: %s (use: X/X, e.g., 0/7C82CB8)", opts.LSN)
	}

	if opts.XID != "" {
		n, err := strconv.ParseUint(opts.XID, 10, 32)
		if err != nil || n == 0 {
			return fmt.Errorf("invalid XID: %s (must be a positive integer)", opts.XID)
		}
	}

	if opts.Time != "" && !isValidTimeFormat(opts.Time) {
		logrus.Warnf("time format '%s' may not be recognized, proceeding anyway", opts.Time)
	}

	return nil
}

// isValidTimeFormat checks if time string matches any known pattern.
func isValidTimeFormat(t string) bool {
	return dateOnlyRegex.MatchString(t) ||
		timeOnlyRegex.MatchString(t) ||
		dateTimeRegex.MatchString(t)
}

// normalizeTime normalizes time input for pgBackRest.
// - Date only -> adds 00:00:00 with current timezone
// - Time only -> adds today's date
func normalizeTime(t string) string {
	if t == "" {
		return ""
	}

	tzOffset := utils.CurrentTimezoneOffset()

	// Date only: 2025-01-01 -> 2025-01-01 00:00:00+TZ
	if dateOnlyRegex.MatchString(t) {
		return fmt.Sprintf("%s 00:00:00%s", t, tzOffset)
	}

	// Time only: 12:00:00 -> today 12:00:00+TZ
	if timeOnlyRegex.MatchString(t) {
		return fmt.Sprintf("%s %s%s", utils.TodayDate(), t, tzOffset)
	}

	return t
}

// buildRestoreArgs builds pgbackrest restore command arguments.
func buildRestoreArgs(cfg *Config, opts *RestoreOptions, normalizedTime string) []string {
	var args []string

	// Data directory (from option, config, or default)
	dataDir := getDataDir(cfg, opts.DataDir)
	if dataDir != "" {
		args = append(args, "--pg1-path="+dataDir)
	}

	// Backup set (can be combined with recovery targets)
	if opts.Set != "" {
		args = append(args, "--set="+opts.Set)
	}

	// Recovery target
	if opts.Immediate {
		args = append(args, "--type=immediate")
	} else if normalizedTime != "" {
		args = append(args, "--type=time", "--target="+normalizedTime)
	} else if opts.Name != "" {
		args = append(args, "--type=name", "--target="+opts.Name)
	} else if opts.LSN != "" {
		args = append(args, "--type=lsn", "--target="+opts.LSN)
	} else if opts.XID != "" {
		args = append(args, "--type=xid", "--target="+opts.XID)
	}
	// Default: --type=default (recover to end of WAL)

	if opts.Exclusive {
		args = append(args, "--target-exclusive")
	}
	if opts.Promote {
		args = append(args, "--target-action=promote")
	}

	return args
}

// checkPostgresStopped verifies PostgreSQL is not running.
func checkPostgresStopped(cfg *Config, dataDir string) error {
	dir := getDataDir(cfg, dataDir)
	running, pid, err := postgres.CheckPostgresRunning(dir)
	if err != nil {
		logrus.Warnf("cannot check PostgreSQL status: %v", err)
		return nil // Continue anyway - restore will fail if PG is running
	}
	if running {
		return fmt.Errorf("PostgreSQL is still running (PID: %d), stop it first: pig pg stop", pid)
	}
	return nil
}

// getDataDir returns the data directory from option, config, or default.
func getDataDir(cfg *Config, optDataDir string) string {
	if optDataDir != "" {
		return optDataDir
	}
	// Try to get from pgbackrest config
	if cfg != nil && cfg.ConfigPath != "" && cfg.Stanza != "" {
		if pgPath := GetPgPathFromConfig(cfg.ConfigPath, cfg.Stanza); pgPath != "" {
			return pgPath
		}
	}
	if pgData := os.Getenv("PGDATA"); pgData != "" {
		return pgData
	}
	return "/pg/data"
}

// confirmWithCountdown shows a warning and countdown, returns error if cancelled.
func confirmWithCountdown(warning, action string) error {
	fmt.Fprintf(os.Stderr, "\n%sWARNING: %s%s\n", utils.ColorYellow, warning, utils.ColorReset)
	fmt.Fprintln(os.Stderr, "Press Ctrl+C to cancel, or wait for countdown...")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer func() {
		signal.Stop(sigChan)
		close(sigChan)
	}()

	for i := 5; i > 0; i-- {
		select {
		case <-sigChan:
			fmt.Fprintf(os.Stderr, "\n%s cancelled.\n", action)
			return fmt.Errorf("%s cancelled by user", action)
		case <-time.After(time.Second):
			fmt.Fprintf(os.Stderr, "\rStarting %s in %d seconds... ", action, i)
		}
	}
	fmt.Fprintln(os.Stderr)
	return nil
}

// printRestorePlan displays the restore plan to stderr.
func printRestorePlan(cfg *Config, opts *RestoreOptions, normalizedTime string) {
	utils.PrintSection("Restore Plan")
	fmt.Fprintf(os.Stderr, "Stanza:     %s\n", cfg.Stanza)
	fmt.Fprintf(os.Stderr, "Data Dir:   %s\n", getDataDir(cfg, opts.DataDir))

	target := "latest (end of WAL stream)"
	if opts.Immediate {
		target = "backup consistency point"
	} else if normalizedTime != "" {
		target = fmt.Sprintf("time: %s", normalizedTime)
	} else if opts.Name != "" {
		target = fmt.Sprintf("restore point: %s", opts.Name)
	} else if opts.LSN != "" {
		target = fmt.Sprintf("LSN: %s", opts.LSN)
	} else if opts.XID != "" {
		target = fmt.Sprintf("XID: %s", opts.XID)
	}
	fmt.Fprintf(os.Stderr, "Target:     %s\n", target)

	if opts.Set != "" {
		fmt.Fprintf(os.Stderr, "Backup Set: %s\n", opts.Set)
	}
	if opts.Exclusive {
		fmt.Fprintf(os.Stderr, "Exclusive:  yes (stop before target)\n")
	}
	if opts.Promote {
		fmt.Fprintf(os.Stderr, "Promote:    yes (auto-promote after recovery)\n")
	}
}

// printPostRestoreHints displays post-restore instructions to stderr.
func printPostRestoreHints(opts *RestoreOptions) {
	fmt.Fprintf(os.Stderr, "\n%s=== Next Steps ===%s\n", utils.ColorGreen, utils.ColorReset)
	fmt.Fprintln(os.Stderr, "1. Start PostgreSQL:")
	fmt.Fprintln(os.Stderr, "   pig pg start")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "2. Verify data integrity:")
	fmt.Fprintln(os.Stderr, "   pig pg ps")
	fmt.Fprintln(os.Stderr)

	if !opts.Promote {
		fmt.Fprintln(os.Stderr, "3. If satisfied, promote to primary:")
		fmt.Fprintln(os.Stderr, "   pig pg promote")
		fmt.Fprintln(os.Stderr)
	}

	fmt.Fprintln(os.Stderr, "4. Re-create stanza if needed:")
	fmt.Fprintln(os.Stderr, "   pig pb create")
}
