/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

pb restore structured output result and DTO.
*/
package pgbackrest

import (
	"fmt"
	"time"

	"pig/cli/postgres"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"

	"github.com/sirupsen/logrus"
)

// PbRestoreResultData contains restore operation result in an agent-friendly format.
// This struct is used as the Data field in output.Result for structured output of pb restore.
type PbRestoreResultData struct {
	Stanza          string `json:"stanza" yaml:"stanza"`                                         // Stanza name
	DataDir         string `json:"data_dir" yaml:"data_dir"`                                     // Restored data directory
	RestoredBackup  string `json:"restored_backup,omitempty" yaml:"restored_backup,omitempty"`   // Backup label (if --set specified)
	TargetType      string `json:"target_type" yaml:"target_type"`                               // Recovery target type: default/immediate/time/name/lsn/xid
	TargetValue     string `json:"target_value,omitempty" yaml:"target_value,omitempty"`         // Recovery target value (for time/name/lsn/xid)
	Exclusive       bool   `json:"exclusive" yaml:"exclusive"`                                   // Whether exclusive mode is enabled
	Promote         bool   `json:"promote" yaml:"promote"`                                       // Whether auto-promote is enabled
	StartTime       int64  `json:"start_time" yaml:"start_time"`                                 // Start time (Unix timestamp)
	StopTime        int64  `json:"stop_time" yaml:"stop_time"`                                   // Stop time (Unix timestamp)
	DurationSeconds int64  `json:"duration_seconds" yaml:"duration_seconds"`                     // Duration in seconds
}

// RestoreResult creates a structured result for pb restore command.
// It validates preconditions, executes the restore, and returns the result.
// Returns nil-safe Result on all paths.
func RestoreResult(cfg *Config, opts *RestoreOptions) *output.Result {
	// Get effective config (validates config file exists, auto-detects stanza)
	effCfg, err := GetEffectiveConfig(cfg)
	if err != nil {
		errMsg := err.Error()
		if containsAny(errMsg, "config file not found", "config file not accessible") {
			return output.Fail(output.CodePbConfigNotFound, "pgBackRest configuration not found").
				WithDetail(errMsg)
		}
		if containsAny(errMsg, "no stanza found", "cannot detect stanza") {
			return output.Fail(output.CodePbStanzaNotFound, "pgBackRest stanza not found").
				WithDetail(errMsg)
		}
		return output.Fail(output.CodePbRestoreFailed, "Failed to get pgBackRest configuration").
			WithDetail(errMsg)
	}

	// Validate restore options
	if err := validateRestoreOptions(opts); err != nil {
		return output.Fail(output.CodePbInvalidRestoreParams, "Invalid restore parameters").
			WithDetail(err.Error())
	}

	// Normalize time if specified
	normalizedTime := normalizeTime(opts.Time)

	// Get data directory
	dataDir := getDataDir(effCfg, opts.DataDir)

	// Check PostgreSQL is stopped
	if err := checkPostgresStoppedResult(effCfg, opts.DataDir); err != nil {
		return err
	}

	// Build restore arguments
	args := buildRestoreArgs(effCfg, opts, normalizedTime)

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
		if containsAny(errMsg, "backup set", "not found", "does not exist", "unable to find backup") {
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

	// Build result data
	data := &PbRestoreResultData{
		Stanza:          effCfg.Stanza,
		DataDir:         dataDir,
		RestoredBackup:  opts.Set,
		TargetType:      determineTargetType(opts),
		TargetValue:     determineTargetValue(opts, normalizedTime),
		Exclusive:       opts.Exclusive,
		Promote:         opts.Promote,
		StartTime:       startTime.Unix(),
		StopTime:        stopTime.Unix(),
		DurationSeconds: durationSeconds,
	}

	return output.OK("Restore completed successfully", data)
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
