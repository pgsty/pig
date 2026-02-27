/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

pb backup structured output result and DTO.
*/
package pgbackrest

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"pig/cli/postgres"
	"pig/internal/output"

	"github.com/sirupsen/logrus"
)

// PbBackupResultData contains backup operation result in a simplified, agent-friendly format.
// This struct is used as the Data field in output.Result for structured output of pb backup.
type PbBackupResultData struct {
	Label           string `json:"label" yaml:"label"`                       // Backup label (e.g., "20250204-120000F")
	Type            string `json:"type" yaml:"type"`                         // Backup type: full, diff, incr
	StartTime       int64  `json:"start_time" yaml:"start_time"`             // Start time (Unix timestamp)
	StopTime        int64  `json:"stop_time" yaml:"stop_time"`               // Stop time (Unix timestamp)
	Size            int64  `json:"size" yaml:"size"`                         // Original size (bytes)
	SizeRepo        int64  `json:"size_repo" yaml:"size_repo"`               // Repository size after compression (bytes)
	DurationSeconds int64  `json:"duration_seconds" yaml:"duration_seconds"` // Duration in seconds
	Stanza          string `json:"stanza" yaml:"stanza"`                     // Stanza name
	LSNStart        string `json:"lsn_start" yaml:"lsn_start"`               // Start LSN
	LSNStop         string `json:"lsn_stop" yaml:"lsn_stop"`                 // Stop LSN
	WALStart        string `json:"wal_start" yaml:"wal_start"`               // Start WAL segment
	WALStop         string `json:"wal_stop" yaml:"wal_stop"`                 // Stop WAL segment
	Prior           string `json:"prior,omitempty" yaml:"prior,omitempty"`   // Prior backup label (for diff/incr backups)
}

var backupLabelRegex = regexp.MustCompile(`(?i)backup label\s*[:=]\s*([0-9]{8}-[0-9]{6}[A-Za-z])`)

// BackupResult creates a structured result for pb backup command.
// It validates preconditions, executes the backup, and returns the result.
// Returns nil-safe Result on all paths.
func BackupResult(cfg *Config, opts *BackupOptions) *output.Result {
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
		return output.Fail(output.CodePbBackupFailed, "Failed to get pgBackRest configuration").
			WithDetail(errMsg)
	}

	// Validate backup type if specified
	if opts.Type != "" && !validBackupTypes[opts.Type] {
		return output.Fail(output.CodePbInvalidBackupType, "Invalid backup type: "+opts.Type).
			WithDetail("Valid types: full, diff, incr")
	}

	// Check primary role (unless --force)
	if !opts.Force {
		roleErr := checkPrimaryRoleResult()
		if roleErr != nil {
			return roleErr
		}
	}

	// Build command arguments
	var args []string
	if opts.Type != "" {
		args = append(args, "--type="+opts.Type)
	}

	// Execute backup command
	startTime := time.Now()
	backupOutput, err := RunPgBackRestOutput(effCfg, "backup", args)
	stopTime := time.Now()
	if err != nil {
		errMsg := combineCommandError(backupOutput, err)
		// Check for specific error conditions
		if containsAny(errMsg, "no prior backup exists", "unable to find backup") {
			return output.Fail(output.CodePbNoBaseBackup, "No base backup exists for incremental backup").
				WithDetail("Run a full backup first: pig pb backup full")
		}
		if containsAny(errMsg, "permission denied", "Permission denied") {
			return output.Fail(output.CodePbPermissionDenied, "Permission denied during backup").
				WithDetail(errMsg)
		}
		return output.Fail(output.CodePbBackupFailed, "Backup failed").
			WithDetail(errMsg)
	}

	// Log backup output to stderr for diagnostic purposes
	if backupOutput != "" {
		logrus.Debugf("pgbackrest backup output: %s", backupOutput)
	}

	// Get backup info to retrieve details of the just-completed backup
	labelHint := parseBackupLabel(backupOutput)
	backupData, err := getLatestBackupInfo(effCfg, startTime, stopTime, labelHint)
	if err != nil {
		// Backup succeeded but we couldn't retrieve required details
		logrus.Warnf("backup succeeded but failed to retrieve details: %v", err)
		return output.Fail(output.CodePbBackupFailed, "Backup completed but failed to retrieve backup info").
			WithDetail(err.Error())
	}

	return output.OK("Backup completed successfully", backupData)
}

// checkPrimaryRoleResult checks if current instance is primary and returns a Result on error.
func checkPrimaryRoleResult() *output.Result {
	pgConfig := postgres.DefaultConfig()
	roleResult, err := postgres.DetectRole(pgConfig, &postgres.RoleOptions{
		Verbose: false,
	})

	if err != nil {
		logrus.Warnf("cannot detect PostgreSQL role: %v", err)
		return output.Fail(output.CodePbBackupFailed, "Cannot verify primary role").
			WithDetail(err.Error() + " - use --force to skip this check")
	}

	switch roleResult.Role {
	case postgres.RolePrimary:
		logrus.Infof("confirmed running on primary instance (source: %s)", roleResult.Source)
		return nil
	case postgres.RoleReplica:
		return output.Fail(output.CodePbNotPrimary, "Backup must run on primary instance").
			WithDetail("Current instance is replica (source: " + roleResult.Source + ")")
	default:
		if !roleResult.Alive {
			return output.Fail(output.CodePbPgNotRunning, "PostgreSQL is not running").
				WithDetail("Start PostgreSQL before performing backup")
		}
		logrus.Warnf("cannot determine instance role (source: %s)", roleResult.Source)
		return output.Fail(output.CodePbBackupFailed, "Cannot confirm primary role").
			WithDetail("Use --force to override")
	}
}

// getLatestBackupInfo retrieves information about the most recent backup.
func getLatestBackupInfo(cfg *Config, windowStart, windowStop time.Time, labelHint string) (*PbBackupResultData, error) {
	// Execute pgbackrest info --output=json
	// Suppress console logs to keep JSON output clean.
	args := []string{"--output=json", "--log-level-console=error"}
	jsonOutput, err := RunPgBackRestOutput(cfg, "info", args)
	if err != nil {
		return nil, err
	}

	// Parse JSON output
	var infos []PgBackRestInfo
	if err := json.Unmarshal([]byte(jsonOutput), &infos); err != nil {
		return nil, err
	}

	if len(infos) == 0 {
		return nil, fmt.Errorf("pgbackrest info returned no stanzas")
	}

	// Find the stanza we used
	var targetInfo *PgBackRestInfo
	for i := range infos {
		if infos[i].Name == cfg.Stanza {
			targetInfo = &infos[i]
			break
		}
	}
	if targetInfo == nil && len(infos) > 0 {
		targetInfo = &infos[0]
	}
	if targetInfo == nil || len(targetInfo.Backup) == 0 {
		return nil, fmt.Errorf("no backups found for stanza %s", cfg.Stanza)
	}

	// Prefer exact label match if we can parse it from output
	if labelHint != "" {
		for _, backup := range targetInfo.Backup {
			if backup.Label == labelHint {
				return buildBackupResultData(targetInfo.Name, &backup), nil
			}
		}
		logrus.Warnf("backup label hint %q not found in info output, falling back to time window", labelHint)
	}

	// Try to select a backup within the execution time window
	windowStartUnix := windowStart.Unix()
	windowStopUnix := windowStop.Unix()
	const windowSkewSeconds = 10
	var windowMatches []BackupInfo
	if windowStartUnix > 0 && windowStopUnix > 0 {
		startBound := windowStartUnix - windowSkewSeconds
		stopBound := windowStopUnix + windowSkewSeconds
		for _, backup := range targetInfo.Backup {
			if backup.Timestamp.Start >= startBound && backup.Timestamp.Stop <= stopBound {
				windowMatches = append(windowMatches, backup)
			}
		}
	}

	if len(windowMatches) > 0 {
		sort.Slice(windowMatches, func(i, j int) bool {
			return windowMatches[i].Timestamp.Stop > windowMatches[j].Timestamp.Stop
		})
		return buildBackupResultData(targetInfo.Name, &windowMatches[0]), nil
	}

	// Fallback: sort backups by stop timestamp (descending) to get the latest
	backups := make([]BackupInfo, len(targetInfo.Backup))
	copy(backups, targetInfo.Backup)
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].Timestamp.Stop > backups[j].Timestamp.Stop
	})

	// Get the most recent backup
	latest := backups[0]

	return buildBackupResultData(targetInfo.Name, &latest), nil
}

func buildBackupResultData(stanza string, latest *BackupInfo) *PbBackupResultData {
	if latest == nil {
		return nil
	}
	data := &PbBackupResultData{
		Label:           latest.Label,
		Type:            latest.Type,
		StartTime:       latest.Timestamp.Start,
		StopTime:        latest.Timestamp.Stop,
		Size:            latest.Info.Size,
		SizeRepo:        latest.Info.Repository.Size,
		DurationSeconds: latest.Timestamp.Stop - latest.Timestamp.Start,
		Stanza:          stanza,
		LSNStart:        latest.LSN.Start,
		LSNStop:         latest.LSN.Stop,
		WALStart:        latest.Archive.Start,
		WALStop:         latest.Archive.Stop,
	}
	if latest.Prior != nil {
		data.Prior = *latest.Prior
	}
	return data
}

func parseBackupLabel(output string) string {
	matches := backupLabelRegex.FindStringSubmatch(output)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

// normalizeBackupType returns a normalized backup type string.
// Empty type means pgBackRest auto-selects (full if none, else incr).
