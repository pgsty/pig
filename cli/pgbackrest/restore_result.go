/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

pb restore structured output result and DTO.
*/
package pgbackrest

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"pig/cli/postgres"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"

	"github.com/sirupsen/logrus"
)

// IsBackupNotFoundError reports whether a pgBackRest error message indicates the
// requested backup set does not exist. Requires backup-specific compound phrases:
// generic substrings like "not found" alone (e.g. "pgbackrest not found",
// "path '/x' does not exist") must NOT classify as backup-not-found, so that
// automation branching on the error code is not sent chasing the wrong cause.
func IsBackupNotFoundError(message string) bool {
	msg := strings.ToLower(message)
	if strings.Contains(msg, "no prior backup exists") ||
		strings.Contains(msg, "unable to find backup") ||
		strings.Contains(msg, "no backup set") {
		return true
	}
	return strings.Contains(msg, "backup set") &&
		(strings.Contains(msg, "not found") ||
			strings.Contains(msg, "does not exist") ||
			strings.Contains(msg, "is not valid"))
}

// PbRestoreResultData contains restore operation result in an agent-friendly format.
// This struct is used as the Data field in output.Result for structured output of pb restore.
type PbRestoreResultData struct {
	Stanza          string `json:"stanza" yaml:"stanza"`                                       // Stanza name
	DataDir         string `json:"data_dir" yaml:"data_dir"`                                   // Restored data directory
	RestoredBackup  string `json:"restored_backup,omitempty" yaml:"restored_backup,omitempty"` // Backup label (if --set specified)
	TargetType      string `json:"target_type" yaml:"target_type"`                             // Recovery target type: default/immediate/time/name/lsn/xid
	TargetValue     string `json:"target_value,omitempty" yaml:"target_value,omitempty"`       // Recovery target value (for time/name/lsn/xid)
	TargetAction    string `json:"target_action,omitempty" yaml:"target_action,omitempty"`     // Recovery target action (pause/promote/shutdown)
	TargetTimeline  string `json:"target_timeline,omitempty" yaml:"target_timeline,omitempty"` // Recovery target timeline
	Exclusive       bool   `json:"exclusive" yaml:"exclusive"`                                 // Whether exclusive mode is enabled
	StartTime       int64  `json:"start_time" yaml:"start_time"`                               // Start time (Unix timestamp)
	StopTime        int64  `json:"stop_time" yaml:"stop_time"`                                 // Stop time (Unix timestamp)
	DurationSeconds int64  `json:"duration_seconds" yaml:"duration_seconds"`                   // Duration in seconds
}

// restoredBackupSetRegex extracts the backup set label pgBackRest actually
// selected from restore INFO output (e.g. "restore backup set 20250101-120000F"
// or an incr/diff label like "20250101-120000F_20250102-130000I").
var restoredBackupSetRegex = regexp.MustCompile(`(?i)restore backup set\s+([0-9]{8}-[0-9]{6}[FDI](?:_[0-9]{8}-[0-9]{6}[FDI])?)`)

// RestoreResult creates a structured result for pb restore command.
// It validates preconditions, executes the restore, and returns the result.
// Returns nil-safe Result on all paths.
//
// IMPORTANT: In structured output mode, --yes is required as an explicit
// confirmation (B05): structured callers never get an interactive prompt, so
// this is re-checked here even though the cmd layer gates earlier.
func RestoreResult(cfg *Config, opts *RestoreOptions) *output.Result {
	if opts == nil {
		opts = &RestoreOptions{}
	}
	if !opts.Yes {
		return output.Fail(output.CodePbConfirmationRequired, "Restore requires --yes flag").
			WithDetail("Use --yes to confirm overwriting the target data directory with the restored backup.")
	}

	// Get effective config (validates config file exists, auto-detects stanza)
	effCfg, err := GetEffectiveConfig(cfg)
	if err != nil {
		return pbConfigErrorResult(err, output.CodePbRestoreFailed, "Failed to get pgBackRest configuration")
	}

	// Validate restore options
	if err := ValidateRestoreOptions(opts); err != nil {
		return output.Fail(output.CodePbInvalidRestoreParams, "Invalid restore parameters").
			WithDetail(err.Error())
	}

	// Normalize time if specified
	normalizedTime := normalizeTime(opts.Time)

	// Get data directory
	dataDir := getDataDir(effCfg, opts.DataDir)

	if err := checkPatroniManagedRestore(effCfg, opts); err != nil {
		return patroniManagedRestoreResult(err)
	}

	// Check PostgreSQL is stopped
	if err := checkPostgresStoppedResult(effCfg, opts.DataDir); err != nil {
		return err
	}

	// Build restore arguments. Force INFO console logging (unless the caller
	// overrides it) so the captured output contains the line naming the backup
	// set pgBackRest actually selected, regardless of the config file's
	// console log level.
	args := ensureConsoleInfoLog(buildRestoreArgs(effCfg, opts, normalizedTime))

	// Record start time
	startTime := time.Now()

	// Execute restore command
	restoreOutput, restoreErr := RunPgBackRestOutput(effCfg, "restore", args)

	// Record stop time
	stopTime := time.Now()
	durationSeconds := int64(stopTime.Sub(startTime).Seconds())

	if restoreErr != nil {
		errMsg := combineCommandError(restoreOutput, restoreErr)

		// Check for specific error conditions
		if IsBackupNotFoundError(errMsg) {
			return output.Fail(output.CodePbBackupNotFound, "Specified backup not found").
				WithDetail(errMsg)
		}
		if containsAny(errMsg, "permission denied", "Permission denied") {
			return output.Fail(output.CodePbPermissionDenied, "Permission denied during restore").
				WithDetail(errMsg)
		}
		return output.Fail(output.CodePbRestoreFailed, "Restore failed").
			WithDetail(errMsg)
	}

	// Log restore output for diagnostics
	if restoreOutput != "" {
		logrus.Debugf("pgbackrest restore output: %s", restoreOutput)
	}

	// Report the backup set pgBackRest actually selected when it can be
	// parsed from the output; fall back to the user-requested --set.
	restoredBackup := parseRestoredBackupSet(restoreOutput)
	if restoredBackup == "" {
		restoredBackup = opts.Set
	}

	// Build result data
	data := &PbRestoreResultData{
		Stanza:          effCfg.Stanza,
		DataDir:         dataDir,
		RestoredBackup:  restoredBackup,
		TargetType:      determineTargetType(opts),
		TargetValue:     determineTargetValue(opts, normalizedTime),
		TargetAction:    determineTargetAction(opts),
		TargetTimeline:  opts.TargetTimeline,
		Exclusive:       opts.Exclusive,
		StartTime:       startTime.Unix(),
		StopTime:        stopTime.Unix(),
		DurationSeconds: durationSeconds,
	}

	return output.OK("Restore completed successfully", data).
		WithNextActions(restoreNextActions(effCfg, opts)...)
}

// ensureConsoleInfoLog appends --log-level-console=info unless the caller
// already set a console log level via extra args (exact option match, so an
// unrelated same-prefix option can never suppress the injection).
func ensureConsoleInfoLog(args []string) []string {
	for _, arg := range args {
		if arg == "--log-level-console" || strings.HasPrefix(arg, "--log-level-console=") {
			return args
		}
	}
	return append(args, "--log-level-console=info")
}

// parseRestoredBackupSet extracts the selected backup set label from restore output.
func parseRestoredBackupSet(output string) string {
	matches := restoredBackupSetRegex.FindStringSubmatch(output)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// restoreNextActions returns the structured counterpart of the text-mode
// post-restore hints (printPostRestoreHints): start, verify, promote for
// managed restores, and re-create the stanza for side restores.
func restoreNextActions(cfg *Config, opts *RestoreOptions) []output.NextAction {
	if opts == nil {
		opts = &RestoreOptions{}
	}
	dataDir := getDataDir(cfg, opts.DataDir)
	managedDir := getDataDir(cfg, "")
	action := determineTargetAction(opts)
	needsSideManualPromote := action != "promote" && !opts.Default
	needsManagedPromote := action != "promote"
	sameDataDir, sameErr := sameRestoreDataDir(cfg, dataDir, managedDir)
	isCustomDataDir := sameErr != nil || !sameDataDir

	if isCustomDataDir {
		quotedDataDir := QuoteShellArg(dataDir)
		actions := []output.NextAction{
			{Command: fmt.Sprintf("pg_ctl -D %s -o \"-p 5433\" start", quotedDataDir), Reason: "start PostgreSQL with the restored data directory on an alternate port", Required: true},
			{Command: fmt.Sprintf("pg_ctl -D %s status", quotedDataDir), Reason: "verify PostgreSQL is running on the side restore directory", Required: false},
		}
		if needsSideManualPromote {
			actions = append(actions, output.NextAction{
				Command: fmt.Sprintf("pg_ctl -D %s promote", quotedDataDir), Reason: "promote to primary once the restored state is verified", Required: false})
		}
		return append(actions, output.NextAction{
			Command: restoreSideStanzaCreateCommand(cfg, dataDir), Reason: "re-create the pgBackRest stanza if needed", Required: false})
	}

	actions := []output.NextAction{
		{Command: restorePigPgCommand("start", managedDir), Reason: "start PostgreSQL on the restored data directory", Required: true},
		{Command: restorePigPgCommand("status", managedDir), Reason: "verify PostgreSQL is running on the restored data directory", Required: false},
	}
	if needsManagedPromote {
		actions = append(actions, output.NextAction{
			Command: restorePigPgCommand("promote", managedDir), Reason: "promote to primary after accepting the restored state", Required: false})
	}
	return actions
}

func sameRestoreDataDir(cfg *Config, targetDir string, managedDir string) (bool, error) {
	if filepath.Clean(targetDir) == filepath.Clean(managedDir) {
		return true, nil
	}
	targetResolved, targetErr := resolveRestorePathAsDBSU(cfg, targetDir)
	managedResolved, managedErr := resolveRestorePathAsDBSU(cfg, managedDir)
	if targetErr != nil || managedErr != nil || targetResolved == "" || managedResolved == "" {
		return false, fmt.Errorf("cannot determine whether restore target %s is managed PGDATA %s", targetDir, managedDir)
	}
	return filepath.Clean(targetResolved) == filepath.Clean(managedResolved), nil
}

func resolveRestorePathAsDBSU(cfg *Config, path string) (string, error) {
	dbsu := utils.GetDBSU("")
	if cfg != nil && cfg.DbSU != "" {
		dbsu = cfg.DbSU
	}
	out, err := utils.DBSUCommandOutput(dbsu, []string{"readlink", "-f", path})
	return strings.TrimSpace(out), err
}

func restorePigPgCommand(subcommand string, dataDir string) string {
	parts := []string{"pig", "pg", subcommand}
	if dataDir != "" && filepath.Clean(dataDir) != filepath.Clean(postgres.DefaultPgData) {
		parts = append(parts, "-D", QuoteShellArg(dataDir))
	}
	return strings.Join(parts, " ")
}

func restoreSideStanzaCreateCommand(cfg *Config, dataDir string) string {
	parts := []string{"pgbackrest"}
	if cfg != nil {
		if cfg.Stanza != "" {
			parts = append(parts, "--stanza="+QuoteShellArg(cfg.Stanza))
		}
		if cfg.ConfigPath != "" && cfg.ConfigPath != DefaultConfigPath {
			parts = append(parts, "--config="+QuoteShellArg(cfg.ConfigPath))
		}
	}
	parts = append(parts, "--pg1-path="+QuoteShellArg(dataDir), "stanza-create")
	return strings.Join(parts, " ")
}

func determineTargetAction(opts *RestoreOptions) string {
	action, err := restoreTargetAction(opts)
	if err != nil {
		return ""
	}
	return action
}

func patroniManagedRestoreResult(err error) *output.Result {
	if err == nil {
		return nil
	}
	return output.Fail(output.CodePbPatroniActive, "Patroni is active for managed PGDATA").
		WithDetail(err.Error())
}

// checkPostgresStoppedResult checks if PostgreSQL is stopped and returns a Result on error.
func checkPostgresStoppedResult(cfg *Config, dataDir string) *output.Result {
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
		return output.Fail(output.CodePbPgRunning, "PostgreSQL is running").
			WithDetail(fmt.Sprintf("PostgreSQL must be stopped before restore. PID: %d. Run: pig pg stop", pid))
	}
	return nil
}

// determineTargetType returns the target type string based on RestoreOptions.
func determineTargetType(opts *RestoreOptions) string {
	if opts == nil {
		return ""
	}
	if opts.Default {
		return "default"
	}
	if opts.Immediate {
		return "immediate"
	}
	if opts.Time != "" {
		return "time"
	}
	if opts.Name != "" {
		return "name"
	}
	if opts.LSN != "" {
		return "lsn"
	}
	if opts.XID != "" {
		return "xid"
	}
	return ""
}

// determineTargetValue returns the target value based on RestoreOptions.
func determineTargetValue(opts *RestoreOptions, normalizedTime string) string {
	if opts == nil {
		return ""
	}
	if opts.Time != "" {
		return normalizedTime
	}
	if opts.Name != "" {
		return opts.Name
	}
	if opts.LSN != "" {
		return opts.LSN
	}
	if opts.XID != "" {
		return opts.XID
	}
	return ""
}
