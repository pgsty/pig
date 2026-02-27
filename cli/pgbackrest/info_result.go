/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

pb info structured output result and DTO.
*/
package pgbackrest

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"pig/internal/output"
)

// PbInfoResultData contains pgBackRest backup information in a simplified, agent-friendly format.
// This struct is used as the Data field in output.Result for structured output.
type PbInfoResultData struct {
	Stanza         string           `json:"stanza" yaml:"stanza"`
	Status         StanzaStatus     `json:"status" yaml:"status"`
	Cipher         string           `json:"cipher" yaml:"cipher"`
	DB             []DBSummary      `json:"db,omitempty" yaml:"db,omitempty"`
	Archive        []ArchiveSummary `json:"archive,omitempty" yaml:"archive,omitempty"`
	Repo           []RepoSummary    `json:"repo,omitempty" yaml:"repo,omitempty"`
	Backups        []BackupSummary  `json:"backups,omitempty" yaml:"backups,omitempty"`
	BackupCount    int              `json:"backup_count" yaml:"backup_count"`
	RecoveryWindow *RecoveryWindow  `json:"recovery_window,omitempty" yaml:"recovery_window,omitempty"`
}

// StanzaStatus represents the stanza's overall status (simplified from StatusInfo).
type StanzaStatus struct {
	Code    int    `json:"code" yaml:"code"`
	Message string `json:"message" yaml:"message"`
}

// DBSummary contains database information (simplified from DBInfo).
type DBSummary struct {
	ID       int    `json:"id" yaml:"id"`
	Version  string `json:"version" yaml:"version"`
	SystemID int64  `json:"system_id" yaml:"system_id"`
}

// ArchiveSummary contains WAL archive range information (simplified from ArchiveInfo).
type ArchiveSummary struct {
	ID  string `json:"id" yaml:"id"`
	Min string `json:"min" yaml:"min"`
	Max string `json:"max" yaml:"max"`
}

// RepoSummary contains repository status (simplified from RepoInfo).
type RepoSummary struct {
	Key     int    `json:"key" yaml:"key"`
	Cipher  string `json:"cipher" yaml:"cipher"`
	Code    int    `json:"code" yaml:"code"`
	Message string `json:"message" yaml:"message"`
}

// BackupSummary contains backup information in a simplified, agent-friendly format.
// This is derived from BackupInfo but only includes fields that agents typically need
// for making restore decisions.
type BackupSummary struct {
	Label          string `json:"label" yaml:"label"`
	Type           string `json:"type" yaml:"type"`
	TimestampStart int64  `json:"timestamp_start" yaml:"timestamp_start"`
	TimestampStop  int64  `json:"timestamp_stop" yaml:"timestamp_stop"`
	Size           int64  `json:"size" yaml:"size"`
	SizeRepo       int64  `json:"size_repo" yaml:"size_repo"`
	LSNStart       string `json:"lsn_start" yaml:"lsn_start"`
	LSNStop        string `json:"lsn_stop" yaml:"lsn_stop"`
	WALStart       string `json:"wal_start" yaml:"wal_start"`
	WALStop        string `json:"wal_stop" yaml:"wal_stop"`
	Prior          string `json:"prior,omitempty" yaml:"prior,omitempty"`
	Error          bool   `json:"error" yaml:"error"`
}

// RecoveryWindow describes the time range covered by available backups.
type RecoveryWindow struct {
	FirstBackupLabel string `json:"first_backup_label" yaml:"first_backup_label"`
	LastBackupLabel  string `json:"last_backup_label" yaml:"last_backup_label"`
	FirstTimestamp   int64  `json:"first_timestamp" yaml:"first_timestamp"`
	LastTimestamp    int64  `json:"last_timestamp" yaml:"last_timestamp"`
	DurationSeconds  int64  `json:"duration_seconds" yaml:"duration_seconds"`
}

// InfoResult creates a structured result for pb info command.
// It collects pgBackRest backup information and returns it in a Result structure.
// Returns nil-safe Result on all paths.
func InfoResult(cfg *Config, opts *InfoOptions) *output.Result {
	// Get effective config (validates config file exists, auto-detects stanza)
	effCfg, err := GetEffectiveConfig(cfg)
	if err != nil {
		// Determine specific error type
		errMsg := err.Error()
		if containsAny(errMsg, "config file not found", "config file not accessible") {
			return output.Fail(output.CodePbConfigNotFound, "pgBackRest configuration not found").
				WithDetail(errMsg)
		}
		if containsAny(errMsg, "no stanza found", "cannot detect stanza") {
			return output.Fail(output.CodePbStanzaNotFound, "pgBackRest stanza not found").
				WithDetail(errMsg)
		}
		return output.Fail(output.CodePbInfoFailed, "Failed to get pgBackRest configuration").
			WithDetail(errMsg)
	}

	// Build arguments for pgbackrest info --output=json
	// Suppress console logs to keep JSON output clean.
	args := []string{"--output=json", "--log-level-console=error"}
	if opts != nil && opts.Set != "" {
		args = append(args, "--set="+opts.Set)
	}

	// Execute pgbackrest info and capture JSON output
	jsonOutput, err := RunPgBackRestOutput(effCfg, "info", args)
	if err != nil {
		errMsg := combineCommandError(jsonOutput, err)
		return output.Fail(output.CodePbInfoFailed, "Failed to execute pgbackrest info").
			WithDetail(errMsg)
	}

	// Parse JSON output
	var infos []PgBackRestInfo
	if err := json.Unmarshal([]byte(jsonOutput), &infos); err != nil {
		return output.Fail(output.CodePbInfoFailed, "Failed to parse pgbackrest info output").
			WithDetail(err.Error())
	}

	// Handle empty result (no stanzas)
	if len(infos) == 0 {
		return output.Fail(output.CodePbStanzaNotFound, "No stanza information found").
			WithDetail("pgbackrest info returned empty result")
	}

	// Structured output should embed pgBackRest native JSON (wrapped by Result),
	// so agents can consume the upstream schema directly.
	data := output.NewEmbeddedJSON([]byte(jsonOutput))

	// Preserve existing semantics: if a single stanza reports non-zero status,
	// treat it as a failure (but still include the upstream info payload).
	if len(infos) == 1 && infos[0].Status.Code != 0 {
		code := output.CodePbInfoFailed
		if isStanzaNotFoundMessage(infos[0].Status.Message) {
			code = output.CodePbStanzaNotFound
		}
		return output.Fail(code, infos[0].Status.Message).
			WithData(data)
	}

	return output.OK("pgBackRest backup info retrieved", data)
}

// convertToResultData transforms PgBackRestInfo to agent-friendly PbInfoResultData.
func convertToResultData(info *PgBackRestInfo) *PbInfoResultData {
	if info == nil {
		return &PbInfoResultData{}
	}

	data := &PbInfoResultData{
		Stanza: info.Name,
		Status: StanzaStatus{
			Code:    info.Status.Code,
			Message: info.Status.Message,
		},
		Cipher:      info.Cipher,
		BackupCount: len(info.Backup),
	}

	// Convert DB info
	for _, db := range info.DB {
		data.DB = append(data.DB, DBSummary{
			ID:       db.ID,
			Version:  db.Version,
			SystemID: db.SystemID,
		})
	}

	// Convert Archive info
	for _, arch := range info.Archive {
		data.Archive = append(data.Archive, ArchiveSummary{
			ID:  arch.ID,
			Min: arch.Min,
			Max: arch.Max,
		})
	}

	// Convert Repo info
	for _, repo := range info.Repo {
		data.Repo = append(data.Repo, RepoSummary{
			Key:     repo.Key,
			Cipher:  repo.Cipher,
			Code:    repo.Status.Code,
			Message: repo.Status.Message,
		})
	}

	// Convert Backup info and calculate recovery window
	if len(info.Backup) > 0 {
		// Sort backups by start timestamp (ascending)
		backups := make([]BackupInfo, len(info.Backup))
		copy(backups, info.Backup)
		sort.Slice(backups, func(i, j int) bool {
			return backups[i].Timestamp.Start < backups[j].Timestamp.Start
		})

		for _, b := range backups {
			prior := ""
			if b.Prior != nil {
				prior = *b.Prior
			}
			data.Backups = append(data.Backups, BackupSummary{
				Label:          b.Label,
				Type:           b.Type,
				TimestampStart: b.Timestamp.Start,
				TimestampStop:  b.Timestamp.Stop,
				Size:           b.Info.Size,
				SizeRepo:       b.Info.Repository.Size,
				LSNStart:       b.LSN.Start,
				LSNStop:        b.LSN.Stop,
				WALStart:       b.Archive.Start,
				WALStop:        b.Archive.Stop,
				Prior:          prior,
				Error:          b.Error,
			})
		}

		// Calculate recovery window from sorted backups
		first := backups[0]
		last := backups[len(backups)-1]
		firstTime := time.Unix(first.Timestamp.Start, 0)
		lastTime := time.Unix(last.Timestamp.Stop, 0)

		data.RecoveryWindow = &RecoveryWindow{
			FirstBackupLabel: first.Label,
			LastBackupLabel:  last.Label,
			FirstTimestamp:   first.Timestamp.Start,
			LastTimestamp:    last.Timestamp.Stop,
			DurationSeconds:  int64(lastTime.Sub(firstTime).Seconds()),
		}
	}

	return data
}

// containsAny checks if s contains any of the substrings.
func containsAny(s string, substrings ...string) bool {
	for _, substr := range substrings {
		if len(s) >= len(substr) {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
		}
	}
	return false
}

// combineCommandError merges command output and error message for better diagnostics.
func combineCommandError(output string, err error) string {
	outMsg := strings.TrimSpace(output)
	if err == nil {
		return outMsg
	}
	errMsg := strings.TrimSpace(err.Error())
	if outMsg == "" {
		return errMsg
	}
	if errMsg == "" {
		return outMsg
	}
	if strings.Contains(errMsg, outMsg) {
		return errMsg
	}
	return outMsg + "\n" + errMsg
}

// isStanzaNotFoundMessage checks if a status message indicates stanza is missing.
func isStanzaNotFoundMessage(message string) bool {
	lower := strings.ToLower(message)
	if strings.Contains(lower, "stanza") &&
		(strings.Contains(lower, "not found") || strings.Contains(lower, "missing") || strings.Contains(lower, "does not exist")) {
		return true
	}
	return false
}
