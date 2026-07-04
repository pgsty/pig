package pgbackrest

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"pig/cli/postgres"
	"pig/internal/config"
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
	DataDir        string // Target data directory
	Exclusive      bool   // Stop before target (exclusive)
	TargetAction   string // Action at target: pause, promote, shutdown
	TargetTimeline string // Timeline to recover along: latest, current, N, or 0xN
	ExtraArgs      []string
	Yes            bool // Skip confirmation and countdown

	SuppressHints bool // Suppress post-restore hints when restore is orchestrated by another command
}

// Pre-compiled regex patterns for validation
var (
	lsnRegex      = regexp.MustCompile(`^[0-9A-Fa-f]+/[0-9A-Fa-f]+$`)
	dateOnlyRegex = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	timeOnlyRegex = regexp.MustCompile(`^\d{2}:\d{2}:\d{2}$`)
	timelineRegex = regexp.MustCompile(`^(latest|current|[1-9][0-9]*|0x[0-9A-Fa-f]+)$`)
)

// Restore performs point-in-time recovery (PITR).
func Restore(cfg *Config, opts *RestoreOptions) error {
	effCfg, err := GetEffectiveConfig(cfg)
	if err != nil {
		return err
	}

	if err := ValidateRestoreOptions(opts); err != nil {
		return err
	}

	normalizedTime := normalizeTime(opts.Time)

	if err := checkPatroniManagedRestore(effCfg, opts); err != nil {
		return err
	}

	// Check PostgreSQL is stopped
	if err := checkPostgresStopped(effCfg, opts.DataDir); err != nil {
		return err
	}

	args := buildRestoreArgs(effCfg, opts, normalizedTime)
	printRestorePlan(effCfg, opts, normalizedTime)

	// Confirmation with signal handling
	if !opts.Yes {
		if err := utils.Confirm(
			fmt.Sprintf("This will overwrite data in %s", getDataDir(effCfg, opts.DataDir)),
			"restore",
		); err != nil {
			return err
		}
	}

	if err := RunPgBackRest(effCfg, "restore", args, true); err != nil {
		return err
	}

	if !opts.SuppressHints {
		printPostRestoreHints(effCfg, opts)
	}
	return nil
}

// ValidateRestoreOptions validates restore parameters.
func ValidateRestoreOptions(opts *RestoreOptions) error {
	if opts == nil {
		return fmt.Errorf("restore options cannot be nil")
	}

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
	if targets == 0 {
		return fmt.Errorf("no recovery target specified, choose one of: --default, --immediate, --time, --name, --lsn, --xid")
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

	if err := validateRestoreTime(opts.Time); err != nil {
		return err
	}

	if err := ValidateRestoreExtraArgs(opts.ExtraArgs); err != nil {
		return err
	}

	if opts.TargetTimeline != "" && !timelineRegex.MatchString(opts.TargetTimeline) {
		return fmt.Errorf("invalid target timeline: %s (use latest, current, a positive integer, or 0xHEX)", opts.TargetTimeline)
	}

	action, err := restoreTargetAction(opts)
	if err != nil {
		return err
	}
	if action != "" && opts.Default {
		return fmt.Errorf("--target-action cannot be used with --default")
	}
	if opts.Exclusive && opts.Time == "" && opts.LSN == "" && opts.XID == "" {
		return fmt.Errorf("--exclusive requires --time, --lsn, or --xid")
	}

	return nil
}

func validateRestoreTime(value string) error {
	if value == "" {
		return nil
	}
	if isValidTimeFormat(value) {
		return nil
	}
	return fmt.Errorf("invalid time format: %s (use YYYY-MM-DD, HH:MM:SS, or YYYY-MM-DD HH:MM:SS[timezone])", value)
}

// isValidTimeFormat checks if time string matches any known pattern.
func isValidTimeFormat(t string) bool {
	for _, layout := range []string{
		"2006-01-02",
		"15:04:05",
	} {
		if _, err := time.ParseInLocation(layout, t, time.Local); err == nil {
			return true
		}
	}
	for _, layout := range noTZDatetimeLayouts {
		if _, err := time.ParseInLocation(layout, t, time.Local); err == nil {
			return true
		}
	}
	for _, layout := range tzDatetimeLayouts {
		if _, err := time.Parse(layout, t); err == nil {
			return true
		}
	}
	return false
}

// Datetime layouts shared by validation (isValidTimeFormat) and
// normalization (normalizeTime): every accepted input form must have a
// canonicalization path, or validation would admit values that pgBackRest
// rejects at execution time.
var (
	noTZDatetimeLayouts = []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
	}
	tzDatetimeLayouts = []string{
		"2006-01-02 15:04:05-07",
		"2006-01-02 15:04:05-0700",
		"2006-01-02 15:04:05-07:00",
		"2006-01-02 15:04:05Z07",
		"2006-01-02 15:04:05Z0700",
		"2006-01-02 15:04:05Z07:00",
		"2006-01-02T15:04:05-07",
		"2006-01-02T15:04:05-0700",
		"2006-01-02T15:04:05-07:00",
		"2006-01-02T15:04:05Z07",
		"2006-01-02T15:04:05Z0700",
		"2006-01-02T15:04:05Z07:00",
	}
)

var blockedRestoreExtraArgs = map[string]struct{}{
	"--type":             {},
	"--target":           {},
	"--target-action":    {},
	"--target-exclusive": {},
	"--target-timeline":  {},
	"--set":              {},
	// pig owns target selection via -s/-c/-r: passthrough overrides would
	// silently desync the stanza/config/repo reported in plans and results.
	// --config-path/--config-include-path redirect config loading like
	// --config does.
	"--stanza":              {},
	"--config":              {},
	"--config-path":         {},
	"--config-include-path": {},
	"--repo":                {},
	// --recovery-option can set recovery_target* GUCs, redefining the
	// recovery target behind the declared plan; use pig's target flags.
	"--recovery-option": {},
}

// blockedPgPathArgRegex catches every spelling of the restore data directory
// option: canonical --pg-path, indexed --pgN-path, and the deprecated
// --db[N]-path aliases. Overriding it via passthrough would desync the
// data_dir reported by --plan and JSON results from the directory pgBackRest
// actually overwrites.
var blockedPgPathArgRegex = regexp.MustCompile(`^--(pg|db)\d*-path$`)

// blockedRepoArgRegex blocks the ENTIRE --repo[N]-* option family. Any of
// path/host*/type/s3-*/gcs-*/azure-*/sftp-*/cipher-* redefines where backups
// come from (or how they are read), desyncing the repository the plan
// reported; enumerating "dangerous" members is unwinnable whack-a-mole, so
// the boundary is: repository identity comes from config plus pig's -r/-c
// flags only. Deliberately allowed: --tablespace-map / --link-map /
// --link-all (relocation escape hatches that neither move the declared
// PGDATA nor change the backup source).
var blockedRepoArgRegex = regexp.MustCompile(`^--repo\d*-`)

func ValidateRestoreExtraArgs(args []string) error {
	for _, arg := range args {
		name := restoreExtraArgName(arg)
		_, blocked := blockedRestoreExtraArgs[name]
		if blocked || blockedPgPathArgRegex.MatchString(name) || blockedRepoArgRegex.MatchString(name) {
			return fmt.Errorf("extra restore arg %q conflicts with pig restore flags; use pig's restore target/lifecycle flags instead", arg)
		}
	}
	return nil
}

func checkPatroniManagedRestore(cfg *Config, opts *RestoreOptions) error {
	return patroniManagedRestoreError(cfg, opts, postgres.PatroniActive())
}

func patroniManagedRestoreError(cfg *Config, opts *RestoreOptions, patroniActive bool) error {
	if !patroniActive {
		return nil
	}
	optDataDir := ""
	if opts != nil {
		optDataDir = opts.DataDir
	}
	targetDir := getDataDir(cfg, optDataDir)
	managedDir := getDataDir(cfg, "")
	sameDataDir, err := sameRestoreDataDir(cfg, targetDir, managedDir)
	if err != nil {
		return err
	}
	if !sameDataDir {
		return nil
	}
	return fmt.Errorf("cannot run pb restore while Patroni is active for managed PGDATA %s; use pig pitr for Patroni-aware restore orchestration", managedDir)
}

func restoreExtraArgName(arg string) string {
	if i := strings.Index(arg, "="); i >= 0 {
		return arg[:i]
	}
	return arg
}

// normalizeTimeLocation is the timezone used to complete timezone-less time
// inputs. A package var so DST-sensitive tests can pin a specific zone.
var normalizeTimeLocation = time.Local

// localOffsetSuffix renders the timezone offset of t itself, so DST zones get
// the offset in effect AT THE TARGET DATE (e.g. -05 for a January target even
// when invoked during -04 summer time), not the offset of "now".
func localOffsetSuffix(t time.Time) string {
	_, offset := t.Zone()
	return utils.FormatTimezoneOffset(offset)
}

// normalizeTime normalizes time input for pgBackRest.
//   - Date only -> adds 00:00:00 with local timezone
//   - Time only -> adds today's date with local timezone
//   - Datetime without timezone -> appends local timezone
//   - Datetime with timezone -> canonical separator/offset spelling (input
//     offset preserved, never shifted to local time)
//
// Every timezone-less input gets the operator's local offset (as of the
// target date) appended; otherwise pgBackRest/PostgreSQL would interpret it
// in the server timezone, silently shifting the recovery point when the two
// differ. Every output is canonicalized to the space-separated
// "YYYY-MM-DD HH:MM:SS±HH[:MM]" form pgBackRest documents for --target: it
// parses the value itself for backup-set selection and rejects the T
// separator and bare "Z"/"±HHMM" offset spellings ("[029] time format must
// be ...", verified against pgBackRest 2.58).
func normalizeTime(t string) string {
	if t == "" {
		return ""
	}
	loc := normalizeTimeLocation

	// Date only: 2025-01-01 -> 2025-01-01 00:00:00+TZ
	if dateOnlyRegex.MatchString(t) {
		if parsed, err := time.ParseInLocation("2006-01-02", t, loc); err == nil {
			return parsed.Format("2006-01-02 15:04:05") + localOffsetSuffix(parsed)
		}
		return t
	}

	// Time only: 12:00:00 -> today 12:00:00+TZ. The target date is constructed
	// explicitly: parsing a bare clock time would land in year 0, where Go
	// resolves LMT offsets (e.g. +08:05 for Asia/Shanghai).
	if timeOnlyRegex.MatchString(t) {
		if parsed, err := time.ParseInLocation("15:04:05", t, loc); err == nil {
			now := time.Now().In(loc)
			target := time.Date(now.Year(), now.Month(), now.Day(),
				parsed.Hour(), parsed.Minute(), parsed.Second(), 0, loc)
			return target.Format("2006-01-02 15:04:05") + localOffsetSuffix(target)
		}
		return t
	}

	// Datetime without timezone: 2025-01-01 12:00:00 -> 2025-01-01 12:00:00+TZ
	for _, layout := range noTZDatetimeLayouts {
		if parsed, err := time.ParseInLocation(layout, t, loc); err == nil {
			return parsed.Format("2006-01-02 15:04:05") + localOffsetSuffix(parsed)
		}
	}

	// Datetime WITH timezone: canonicalize into the documented space-separated
	// "YYYY-MM-DD HH:MM:SS±HH[:MM]" form, preserving the INPUT's offset (never
	// shifted to local time). pgBackRest 2.x rejects the T separator and bare
	// "Z"/"±HHMM" spellings with "[029] time format must be ...", so passing
	// them through would fail at execution despite passing pig's validation.
	for _, layout := range tzDatetimeLayouts {
		if parsed, err := time.Parse(layout, t); err == nil {
			return parsed.Format("2006-01-02 15:04:05") + localOffsetSuffix(parsed)
		}
	}

	return t
}

// NormalizeRestoreTime normalizes restore target time values for replayable
// commands. It is exported so orchestrated restore surfaces can use the same
// deterministic time contract as pb restore plans.
func NormalizeRestoreTime(value string) string {
	return normalizeTime(value)
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
	if opts.TargetAction != "" {
		args = append(args, "--target-action="+opts.TargetAction)
	}
	if opts.TargetTimeline != "" {
		args = append(args, "--target-timeline="+opts.TargetTimeline)
	}
	if len(opts.ExtraArgs) > 0 {
		args = append(args, opts.ExtraArgs...)
	}

	return args
}

func restoreTargetAction(opts *RestoreOptions) (string, error) {
	if opts == nil {
		return "", nil
	}
	if opts.TargetAction != "" {
		switch opts.TargetAction {
		case "pause", "promote", "shutdown":
		default:
			return "", fmt.Errorf("invalid target action: %s (use pause, promote, or shutdown)", opts.TargetAction)
		}
	}
	return opts.TargetAction, nil
}

// checkPostgresStopped verifies PostgreSQL is not running.
// Prints a WARNING and returns error if PostgreSQL is running.
// Uses DBSU privilege escalation to read postmaster.pid file.
func checkPostgresStopped(cfg *Config, dataDir string) error {
	dir := getDataDir(cfg, dataDir)
	dbsu := cfg.DbSU
	if dbsu == "" {
		dbsu = utils.GetDBSU("")
	}

	logrus.Debugf("checking PostgreSQL status: dataDir=%s, dbsu=%s, currentUser=%s",
		dir, dbsu, config.CurrentUser)

	// Use DBSU-aware function to check PostgreSQL status
	running, pid := postgres.CheckPostgresRunningAsDBSU(dbsu, dir)
	logrus.Debugf("PostgreSQL check result: running=%v, pid=%d", running, pid)

	if running {
		fmt.Fprintf(os.Stderr, "\n%sWARNING: PostgreSQL is running (PID: %d) in %s%s\n", utils.ColorYellow, pid, dir, utils.ColorReset)
		fmt.Fprintf(os.Stderr, "%sPlease stop PostgreSQL first: pig pg stop%s\n\n", utils.ColorYellow, utils.ColorReset)
		return fmt.Errorf("cannot restore while PostgreSQL is running")
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
		if pgPath := GetPgPathFromConfig(cfg.ConfigPath, cfg.Stanza, cfg.DbSU); pgPath != "" {
			return pgPath
		}
	}
	if pgData := os.Getenv("PGDATA"); pgData != "" {
		return pgData
	}
	return "/pg/data"
}

// ResolveDataDir returns the effective data directory for restore callers.
func ResolveDataDir(cfg *Config, optDataDir string) string {
	return getDataDir(cfg, optDataDir)
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
	if action, _ := restoreTargetAction(opts); action != "" {
		fmt.Fprintf(os.Stderr, "Action:     %s\n", action)
	}
	if opts.TargetTimeline != "" {
		fmt.Fprintf(os.Stderr, "Timeline:   %s\n", opts.TargetTimeline)
	}
}

// printPostRestoreHints displays post-restore instructions to stderr.
func printPostRestoreHints(cfg *Config, opts *RestoreOptions) {
	fmt.Fprintf(os.Stderr, "\n%s=== Next Steps ===%s\n", utils.ColorGreen, utils.ColorReset)
	if opts == nil {
		opts = &RestoreOptions{}
	}

	// Check if using custom data directory
	dataDir := getDataDir(cfg, opts.DataDir)
	managedDir := getDataDir(cfg, "")
	sameDataDir, sameErr := sameRestoreDataDir(cfg, dataDir, managedDir)
	isCustomDataDir := sameErr != nil || !sameDataDir
	action, _ := restoreTargetAction(opts)
	needsManualPromote := action != "promote" && !opts.Default

	if isCustomDataDir {
		// Simplified hints for custom data directory
		fmt.Fprintln(os.Stderr, "1. Start PostgreSQL with custom data directory:")
		fmt.Fprintf(os.Stderr, "   pg_ctl -D %s -o \"-p 5433\" start\n", QuoteShellArg(dataDir))
		fmt.Fprintln(os.Stderr)

		fmt.Fprintln(os.Stderr, "2. Verify side restore status:")
		fmt.Fprintf(os.Stderr, "   pg_ctl -D %s status\n", QuoteShellArg(dataDir))
		fmt.Fprintln(os.Stderr)

		nextStep := 3
		if needsManualPromote {
			fmt.Fprintf(os.Stderr, "%d. If satisfied, promote to primary:\n", nextStep)
			fmt.Fprintf(os.Stderr, "   pg_ctl -D %s promote\n", QuoteShellArg(dataDir))
			fmt.Fprintln(os.Stderr)
			nextStep++
		}

		fmt.Fprintf(os.Stderr, "%d. Re-create stanza if needed:\n", nextStep)
		fmt.Fprintf(os.Stderr, "   %s\n", restoreSideStanzaCreateCommand(cfg, dataDir))
	} else {
		// Detailed hints for default data directory
		fmt.Fprintln(os.Stderr, "1. Start PostgreSQL:")
		fmt.Fprintf(os.Stderr, "   %s\n", restorePigPgCommand("start", managedDir))
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "2. Verify data integrity:")
		fmt.Fprintf(os.Stderr, "   %s\n", restorePigPgCommand("status", managedDir))
		fmt.Fprintln(os.Stderr)

		nextStep := 3
		if needsManualPromote {
			fmt.Fprintf(os.Stderr, "%d. If satisfied, promote to primary:\n", nextStep)
			fmt.Fprintf(os.Stderr, "   %s\n", restorePigPgCommand("promote", managedDir))
			fmt.Fprintln(os.Stderr)
			nextStep++
		}

		fmt.Fprintf(os.Stderr, "%d. Re-create stanza if needed:\n", nextStep)
		fmt.Fprintf(os.Stderr, "   %s\n", restorePigPBCreateCommand(cfg))
	}
}
