/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>

Tests for pb info structured output.
*/
package pgbackrest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"pig/internal/output"

	"gopkg.in/yaml.v3"
)

// Sample pgBackRest info JSON output for testing
var samplePgBackRestInfoJSON = `[
  {
    "archive": [
      {
        "database": {"id": 1, "repo-key": 1},
        "id": "17-1",
        "max": "000000010000000000000003",
        "min": "000000010000000000000001"
      }
    ],
    "backup": [
      {
        "annotation": {},
        "archive": {"start": "000000010000000000000001", "stop": "000000010000000000000001"},
        "backrest": {"format": 5, "version": "2.54.0"},
        "database": {"id": 1, "repo-key": 1},
        "error": false,
        "info": {
          "delta": 25719742,
          "repository": {"delta": 3123456, "delta-map": 0, "size": 3123456, "size-map": 0},
          "size": 25719742
        },
        "label": "20250101-120000F",
        "lsn": {"start": "0/2000028", "stop": "0/2000100"},
        "prior": null,
        "reference": null,
        "timestamp": {"start": 1735732800, "stop": 1735732860},
        "type": "full"
      },
      {
        "annotation": {},
        "archive": {"start": "000000010000000000000002", "stop": "000000010000000000000002"},
        "backrest": {"format": 5, "version": "2.54.0"},
        "database": {"id": 1, "repo-key": 1},
        "error": false,
        "info": {
          "delta": 1234567,
          "repository": {"delta": 123456, "delta-map": 0, "size": 3246912, "size-map": 0},
          "size": 25719742
        },
        "label": "20250102-120000I",
        "lsn": {"start": "0/3000028", "stop": "0/3000100"},
        "prior": "20250101-120000F",
        "reference": ["20250101-120000F"],
        "timestamp": {"start": 1735819200, "stop": 1735819260},
        "type": "incr"
      }
    ],
    "cipher": "none",
    "db": [
      {"id": 1, "repo-key": 1, "system-id": 7451234567890123456, "version": "17"}
    ],
    "name": "pg-meta",
    "repo": [
      {"cipher": "none", "key": 1, "status": {"code": 0, "message": "ok"}}
    ],
    "status": {"code": 0, "lock": {"backup": {"held": false}, "restore": {"held": false}}, "message": "ok"}
  }
]`

// TestPbInfoResultDataJSONSerialization tests JSON marshaling/unmarshaling of PbInfoResultData.
func TestPbInfoResultDataJSONSerialization(t *testing.T) {
	data := &PbInfoResultData{
		Stanza: "pg-meta",
		Status: StanzaStatus{Code: 0, Message: "ok"},
		Cipher: "none",
		DB: []DBSummary{
			{ID: 1, Version: "17", SystemID: 7451234567890123456},
		},
		Archive: []ArchiveSummary{
			{ID: "17-1", Min: "000000010000000000000001", Max: "000000010000000000000003"},
		},
		Repo: []RepoSummary{
			{Key: 1, Cipher: "none", Code: 0, Message: "ok"},
		},
		Backups: []BackupSummary{
			{
				Label:          "20250101-120000F",
				Type:           "full",
				TimestampStart: 1735732800,
				TimestampStop:  1735732860,
				Size:           25719742,
				SizeRepo:       3123456,
				LSNStart:       "0/2000028",
				LSNStop:        "0/2000100",
				WALStart:       "000000010000000000000001",
				WALStop:        "000000010000000000000001",
				Error:          false,
			},
		},
		BackupCount: 1,
		RecoveryWindow: &RecoveryWindow{
			FirstBackupLabel: "20250101-120000F",
			LastBackupLabel:  "20250101-120000F",
			FirstTimestamp:   1735732800,
			LastTimestamp:    1735732860,
			DurationSeconds:  60,
		},
	}

	// Marshal to JSON
	jsonBytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal PbInfoResultData to JSON: %v", err)
	}

	// Unmarshal back
	var decoded PbInfoResultData
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal PbInfoResultData from JSON: %v", err)
	}

	// Verify fields
	if decoded.Stanza != data.Stanza {
		t.Errorf("Stanza mismatch: got %q, want %q", decoded.Stanza, data.Stanza)
	}
	if decoded.Status.Code != data.Status.Code {
		t.Errorf("Status.Code mismatch: got %d, want %d", decoded.Status.Code, data.Status.Code)
	}
	if decoded.BackupCount != data.BackupCount {
		t.Errorf("BackupCount mismatch: got %d, want %d", decoded.BackupCount, data.BackupCount)
	}
	if len(decoded.Backups) != len(data.Backups) {
		t.Errorf("Backups length mismatch: got %d, want %d", len(decoded.Backups), len(data.Backups))
	}
	if decoded.RecoveryWindow == nil {
		t.Error("RecoveryWindow should not be nil")
	} else if decoded.RecoveryWindow.DurationSeconds != data.RecoveryWindow.DurationSeconds {
		t.Errorf("RecoveryWindow.DurationSeconds mismatch: got %d, want %d",
			decoded.RecoveryWindow.DurationSeconds, data.RecoveryWindow.DurationSeconds)
	}
}

// TestPbInfoResultDataYAMLSerialization tests YAML marshaling/unmarshaling of PbInfoResultData.
func TestPbInfoResultDataYAMLSerialization(t *testing.T) {
	data := &PbInfoResultData{
		Stanza: "pg-meta",
		Status: StanzaStatus{Code: 0, Message: "ok"},
		Cipher: "aes-256-cbc",
		DB: []DBSummary{
			{ID: 1, Version: "16", SystemID: 7451234567890123456},
		},
		BackupCount: 2,
	}

	// Marshal to YAML
	yamlBytes, err := yaml.Marshal(data)
	if err != nil {
		t.Fatalf("Failed to marshal PbInfoResultData to YAML: %v", err)
	}

	// Unmarshal back
	var decoded PbInfoResultData
	if err := yaml.Unmarshal(yamlBytes, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal PbInfoResultData from YAML: %v", err)
	}

	// Verify fields
	if decoded.Stanza != data.Stanza {
		t.Errorf("Stanza mismatch: got %q, want %q", decoded.Stanza, data.Stanza)
	}
	if decoded.Cipher != data.Cipher {
		t.Errorf("Cipher mismatch: got %q, want %q", decoded.Cipher, data.Cipher)
	}
	if decoded.BackupCount != data.BackupCount {
		t.Errorf("BackupCount mismatch: got %d, want %d", decoded.BackupCount, data.BackupCount)
	}
}

// TestConvertToResultData tests the conversion from PgBackRestInfo to PbInfoResultData.
func TestConvertToResultData(t *testing.T) {
	// Parse sample JSON
	var infos []PgBackRestInfo
	if err := json.Unmarshal([]byte(samplePgBackRestInfoJSON), &infos); err != nil {
		t.Fatalf("Failed to parse sample JSON: %v", err)
	}

	if len(infos) != 1 {
		t.Fatalf("Expected 1 stanza, got %d", len(infos))
	}

	// Convert to result data
	data := convertToResultData(&infos[0])

	// Verify stanza name
	if data.Stanza != "pg-meta" {
		t.Errorf("Stanza mismatch: got %q, want %q", data.Stanza, "pg-meta")
	}

	// Verify status
	if data.Status.Code != 0 {
		t.Errorf("Status.Code mismatch: got %d, want 0", data.Status.Code)
	}
	if data.Status.Message != "ok" {
		t.Errorf("Status.Message mismatch: got %q, want %q", data.Status.Message, "ok")
	}

	// Verify cipher
	if data.Cipher != "none" {
		t.Errorf("Cipher mismatch: got %q, want %q", data.Cipher, "none")
	}

	// Verify DB info
	if len(data.DB) != 1 {
		t.Fatalf("Expected 1 DB entry, got %d", len(data.DB))
	}
	if data.DB[0].Version != "17" {
		t.Errorf("DB Version mismatch: got %q, want %q", data.DB[0].Version, "17")
	}

	// Verify archive info
	if len(data.Archive) != 1 {
		t.Fatalf("Expected 1 archive entry, got %d", len(data.Archive))
	}
	if data.Archive[0].Min != "000000010000000000000001" {
		t.Errorf("Archive.Min mismatch: got %q", data.Archive[0].Min)
	}

	// Verify backup count
	if data.BackupCount != 2 {
		t.Errorf("BackupCount mismatch: got %d, want 2", data.BackupCount)
	}

	// Verify backups are sorted by timestamp (ascending)
	if len(data.Backups) != 2 {
		t.Fatalf("Expected 2 backups, got %d", len(data.Backups))
	}
	if data.Backups[0].Label != "20250101-120000F" {
		t.Errorf("First backup should be full backup, got %q", data.Backups[0].Label)
	}
	if data.Backups[1].Label != "20250102-120000I" {
		t.Errorf("Second backup should be incr backup, got %q", data.Backups[1].Label)
	}

	// Verify backup details
	firstBackup := data.Backups[0]
	if firstBackup.Type != "full" {
		t.Errorf("First backup type mismatch: got %q, want %q", firstBackup.Type, "full")
	}
	if firstBackup.Size != 25719742 {
		t.Errorf("First backup size mismatch: got %d", firstBackup.Size)
	}
	if firstBackup.Prior != "" {
		t.Errorf("First backup should have no prior, got %q", firstBackup.Prior)
	}

	secondBackup := data.Backups[1]
	if secondBackup.Type != "incr" {
		t.Errorf("Second backup type mismatch: got %q, want %q", secondBackup.Type, "incr")
	}
	if secondBackup.Prior != "20250101-120000F" {
		t.Errorf("Second backup prior mismatch: got %q", secondBackup.Prior)
	}

	// Verify recovery window
	if data.RecoveryWindow == nil {
		t.Fatal("RecoveryWindow should not be nil")
	}
	if data.RecoveryWindow.FirstBackupLabel != "20250101-120000F" {
		t.Errorf("RecoveryWindow.FirstBackupLabel mismatch: got %q", data.RecoveryWindow.FirstBackupLabel)
	}
	if data.RecoveryWindow.LastBackupLabel != "20250102-120000I" {
		t.Errorf("RecoveryWindow.LastBackupLabel mismatch: got %q", data.RecoveryWindow.LastBackupLabel)
	}
	// Duration should be from first backup start to last backup stop
	expectedDuration := int64(1735819260 - 1735732800) // 86460 seconds
	if data.RecoveryWindow.DurationSeconds != expectedDuration {
		t.Errorf("RecoveryWindow.DurationSeconds mismatch: got %d, want %d",
			data.RecoveryWindow.DurationSeconds, expectedDuration)
	}
}

// TestConvertToResultDataNil tests conversion with nil input.
func TestConvertToResultDataNil(t *testing.T) {
	data := convertToResultData(nil)
	if data == nil {
		t.Fatal("convertToResultData(nil) should not return nil")
	}
	if data.Stanza != "" {
		t.Errorf("Stanza should be empty for nil input, got %q", data.Stanza)
	}
	if data.BackupCount != 0 {
		t.Errorf("BackupCount should be 0 for nil input, got %d", data.BackupCount)
	}
}

// TestConvertToResultDataEmptyBackups tests conversion with no backups.
func TestConvertToResultDataEmptyBackups(t *testing.T) {
	info := &PgBackRestInfo{
		Name:   "pg-empty",
		Status: StatusInfo{Code: 0, Message: "ok"},
		Cipher: "none",
		Backup: []BackupInfo{}, // Empty
	}

	data := convertToResultData(info)

	if data.Stanza != "pg-empty" {
		t.Errorf("Stanza mismatch: got %q", data.Stanza)
	}
	if data.BackupCount != 0 {
		t.Errorf("BackupCount should be 0, got %d", data.BackupCount)
	}
	if data.RecoveryWindow != nil {
		t.Error("RecoveryWindow should be nil when there are no backups")
	}
	if len(data.Backups) != 0 {
		t.Errorf("Backups should be empty, got %d", len(data.Backups))
	}
}

// TestBackupSummarySerialization tests BackupSummary JSON serialization.
func TestBackupSummarySerialization(t *testing.T) {
	backup := BackupSummary{
		Label:          "20250101-120000F",
		Type:           "full",
		TimestampStart: 1735732800,
		TimestampStop:  1735732860,
		Size:           25719742,
		SizeRepo:       3123456,
		LSNStart:       "0/2000028",
		LSNStop:        "0/2000100",
		WALStart:       "000000010000000000000001",
		WALStop:        "000000010000000000000001",
		Prior:          "",
		Error:          false,
	}

	jsonBytes, err := json.Marshal(backup)
	if err != nil {
		t.Fatalf("Failed to marshal BackupSummary: %v", err)
	}

	// Verify prior field is omitted when empty
	jsonStr := string(jsonBytes)
	if contains(jsonStr, `"prior":""`) {
		t.Error("Empty prior field should be omitted from JSON")
	}

	// Unmarshal and verify
	var decoded BackupSummary
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal BackupSummary: %v", err)
	}
	if decoded.Label != backup.Label {
		t.Errorf("Label mismatch: got %q, want %q", decoded.Label, backup.Label)
	}
}

// TestContainsAny tests the containsAny helper function.
func TestContainsAny(t *testing.T) {
	tests := []struct {
		s          string
		substrings []string
		want       bool
	}{
		{"config file not found", []string{"not found", "missing"}, true},
		{"config file not found", []string{"missing", "error"}, false},
		{"cannot detect stanza", []string{"no stanza found", "cannot detect stanza"}, true},
		{"", []string{"foo"}, false},
		{"foo", []string{}, false},
		{"abcdef", []string{"bcd"}, true},
	}

	for _, tt := range tests {
		got := containsAny(tt.s, tt.substrings...)
		if got != tt.want {
			t.Errorf("containsAny(%q, %v) = %v, want %v", tt.s, tt.substrings, got, tt.want)
		}
	}
}

func TestInfoResult_ConfigNotFound(t *testing.T) {
	cfg := &Config{
		ConfigPath: filepath.Join(t.TempDir(), "missing.conf"),
	}
	result := InfoResult(cfg, &InfoOptions{})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Fatalf("expected failure result, got success: %v", result)
	}
	if result.Code != output.CodePbConfigNotFound {
		t.Fatalf("expected CodePbConfigNotFound, got %d", result.Code)
	}
}

func TestInfoResult_StanzaNotFound(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "pgbackrest.conf")
	content := []byte("[global]\nrepo1-path=/tmp\n")
	if err := os.WriteFile(configPath, content, 0o644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg := &Config{
		ConfigPath: configPath,
	}
	result := InfoResult(cfg, &InfoOptions{})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Fatalf("expected failure result, got success: %v", result)
	}
	if result.Code != output.CodePbStanzaNotFound {
		t.Fatalf("expected CodePbStanzaNotFound, got %d", result.Code)
	}
}

// Helper function for string contains check
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
