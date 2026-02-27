/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

Tests for pb restore structured output result and DTO.
*/
package pgbackrest

import (
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestPbRestoreResultData_JSONSerialization tests JSON serialization of PbRestoreResultData.
func TestPbRestoreResultData_JSONSerialization(t *testing.T) {
	data := &PbRestoreResultData{
		Stanza:          "pg-meta",
		DataDir:         "/pg/data",
		RestoredBackup:  "20250204-120000F",
		TargetType:      "time",
		TargetValue:     "2025-02-04 12:00:00+08",
		Exclusive:       false,
		Promote:         true,
		StartTime:       1738627200,
		StopTime:        1738627800,
		DurationSeconds: 600,
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	var decoded PbRestoreResultData
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if decoded.Stanza != data.Stanza {
		t.Errorf("Stanza mismatch: got %q, want %q", decoded.Stanza, data.Stanza)
	}
	if decoded.DataDir != data.DataDir {
		t.Errorf("DataDir mismatch: got %q, want %q", decoded.DataDir, data.DataDir)
	}
	if decoded.RestoredBackup != data.RestoredBackup {
		t.Errorf("RestoredBackup mismatch: got %q, want %q", decoded.RestoredBackup, data.RestoredBackup)
	}
	if decoded.TargetType != data.TargetType {
		t.Errorf("TargetType mismatch: got %q, want %q", decoded.TargetType, data.TargetType)
	}
	if decoded.TargetValue != data.TargetValue {
		t.Errorf("TargetValue mismatch: got %q, want %q", decoded.TargetValue, data.TargetValue)
	}
	if decoded.Exclusive != data.Exclusive {
		t.Errorf("Exclusive mismatch: got %v, want %v", decoded.Exclusive, data.Exclusive)
	}
	if decoded.Promote != data.Promote {
		t.Errorf("Promote mismatch: got %v, want %v", decoded.Promote, data.Promote)
	}
	if decoded.StartTime != data.StartTime {
		t.Errorf("StartTime mismatch: got %d, want %d", decoded.StartTime, data.StartTime)
	}
	if decoded.StopTime != data.StopTime {
		t.Errorf("StopTime mismatch: got %d, want %d", decoded.StopTime, data.StopTime)
	}
	if decoded.DurationSeconds != data.DurationSeconds {
		t.Errorf("DurationSeconds mismatch: got %d, want %d", decoded.DurationSeconds, data.DurationSeconds)
	}
}

// TestPbRestoreResultData_YAMLSerialization tests YAML serialization of PbRestoreResultData.
func TestPbRestoreResultData_YAMLSerialization(t *testing.T) {
	data := &PbRestoreResultData{
		Stanza:          "pg-meta",
		DataDir:         "/pg/data",
		RestoredBackup:  "",
		TargetType:      "default",
		TargetValue:     "",
		Exclusive:       false,
		Promote:         false,
		StartTime:       1738627200,
		StopTime:        1738627800,
		DurationSeconds: 600,
	}

	yamlBytes, err := yaml.Marshal(data)
	if err != nil {
		t.Fatalf("YAML marshal failed: %v", err)
	}

	var decoded PbRestoreResultData
	if err := yaml.Unmarshal(yamlBytes, &decoded); err != nil {
		t.Fatalf("YAML unmarshal failed: %v", err)
	}

	if decoded.Stanza != data.Stanza {
		t.Errorf("Stanza mismatch: got %q, want %q", decoded.Stanza, data.Stanza)
	}
	if decoded.TargetType != data.TargetType {
		t.Errorf("TargetType mismatch: got %q, want %q", decoded.TargetType, data.TargetType)
	}
	if decoded.DurationSeconds != data.DurationSeconds {
		t.Errorf("DurationSeconds mismatch: got %d, want %d", decoded.DurationSeconds, data.DurationSeconds)
	}
}

// TestPbRestoreResultData_JSONFieldNames verifies JSON field names are snake_case.
func TestPbRestoreResultData_JSONFieldNames(t *testing.T) {
	data := &PbRestoreResultData{
		Stanza:          "test",
		DataDir:         "/data",
		RestoredBackup:  "backup",
		TargetType:      "time",
		TargetValue:     "2025-01-01",
		Exclusive:       true,
		Promote:         true,
		StartTime:       1000,
		StopTime:        2000,
		DurationSeconds: 1000,
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	jsonStr := string(jsonBytes)

	expectedFields := []string{
		`"stanza"`,
		`"data_dir"`,
		`"restored_backup"`,
		`"target_type"`,
		`"target_value"`,
		`"exclusive"`,
		`"promote"`,
		`"start_time"`,
		`"stop_time"`,
		`"duration_seconds"`,
	}

	for _, field := range expectedFields {
		if !containsStr(jsonStr, field) {
			t.Errorf("JSON missing expected field: %s", field)
		}
	}
}

// TestPbRestoreResultData_OmitEmptyFields tests that empty optional fields are omitted.
func TestPbRestoreResultData_OmitEmptyFields(t *testing.T) {
	data := &PbRestoreResultData{
		Stanza:          "pg-meta",
		DataDir:         "/pg/data",
		RestoredBackup:  "", // Should be omitted
		TargetType:      "default",
		TargetValue:     "", // Should be omitted
		Exclusive:       false,
		Promote:         false,
		StartTime:       1000,
		StopTime:        2000,
		DurationSeconds: 1000,
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	jsonStr := string(jsonBytes)

	// These fields should be omitted when empty
	if containsStr(jsonStr, `"restored_backup"`) {
		t.Error("restored_backup should be omitted when empty")
	}
	if containsStr(jsonStr, `"target_value"`) {
		t.Error("target_value should be omitted when empty")
	}
}

// TestDetermineTargetType tests the target type determination logic.
func TestDetermineTargetType(t *testing.T) {
	tests := []struct {
		name     string
		opts     *RestoreOptions
		wantType string
	}{
		{
			name:     "default target",
			opts:     &RestoreOptions{Default: true},
			wantType: "default",
		},
		{
			name:     "immediate target",
			opts:     &RestoreOptions{Immediate: true},
			wantType: "immediate",
		},
		{
			name:     "time target",
			opts:     &RestoreOptions{Time: "2025-01-01 12:00:00"},
			wantType: "time",
		},
		{
			name:     "name target",
			opts:     &RestoreOptions{Name: "my_savepoint"},
			wantType: "name",
		},
		{
			name:     "lsn target",
			opts:     &RestoreOptions{LSN: "0/7C82CB8"},
			wantType: "lsn",
		},
		{
			name:     "xid target",
			opts:     &RestoreOptions{XID: "12345"},
			wantType: "xid",
		},
		{
			name:     "no target specified",
			opts:     &RestoreOptions{},
			wantType: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := determineTargetType(tt.opts)
			if got != tt.wantType {
				t.Errorf("determineTargetType() = %q, want %q", got, tt.wantType)
			}
		})
	}
}

// TestDetermineTargetValue tests the target value determination logic.
func TestDetermineTargetValue(t *testing.T) {
	tests := []struct {
		name           string
		opts           *RestoreOptions
		normalizedTime string
		wantValue      string
	}{
		{
			name:           "default target - no value",
			opts:           &RestoreOptions{Default: true},
			normalizedTime: "",
			wantValue:      "",
		},
		{
			name:           "immediate target - no value",
			opts:           &RestoreOptions{Immediate: true},
			normalizedTime: "",
			wantValue:      "",
		},
		{
			name:           "time target with normalized time",
			opts:           &RestoreOptions{Time: "2025-01-01"},
			normalizedTime: "2025-01-01 00:00:00+08",
			wantValue:      "2025-01-01 00:00:00+08",
		},
		{
			name:           "name target",
			opts:           &RestoreOptions{Name: "my_savepoint"},
			normalizedTime: "",
			wantValue:      "my_savepoint",
		},
		{
			name:           "lsn target",
			opts:           &RestoreOptions{LSN: "0/7C82CB8"},
			normalizedTime: "",
			wantValue:      "0/7C82CB8",
		},
		{
			name:           "xid target",
			opts:           &RestoreOptions{XID: "12345"},
			normalizedTime: "",
			wantValue:      "12345",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := determineTargetValue(tt.opts, tt.normalizedTime)
			if got != tt.wantValue {
				t.Errorf("determineTargetValue() = %q, want %q", got, tt.wantValue)
			}
		})
	}
}

// containsStr is a helper to check if a string contains a substring.
// Named differently from info_result_test.go's contains to avoid redeclaration.
func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestPbRestoreResultData_NilSafe tests that nil receiver is handled safely.
func TestPbRestoreResultData_NilSafe(t *testing.T) {
	var data *PbRestoreResultData
	// Should not panic when marshaling nil
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal of nil failed: %v", err)
	}
	if string(jsonBytes) != "null" {
		t.Errorf("Expected 'null', got %s", string(jsonBytes))
	}
}

// TestDetermineTargetType_NilOpts tests nil options handling.
func TestDetermineTargetType_NilOpts(t *testing.T) {
	result := determineTargetType(nil)
	if result != "" {
		t.Errorf("Expected empty string for nil opts, got %q", result)
	}
}

// TestDetermineTargetValue_NilOpts tests nil options handling.
func TestDetermineTargetValue_NilOpts(t *testing.T) {
	result := determineTargetValue(nil, "")
	if result != "" {
		t.Errorf("Expected empty string for nil opts, got %q", result)
	}
}

// TestPbRestoreResultData_AllTargetTypes tests all target type combinations.
func TestPbRestoreResultData_AllTargetTypes(t *testing.T) {
	tests := []struct {
		name       string
		targetType string
	}{
		{"default", "default"},
		{"immediate", "immediate"},
		{"time", "time"},
		{"name", "name"},
		{"lsn", "lsn"},
		{"xid", "xid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := &PbRestoreResultData{
				Stanza:          "test",
				DataDir:         "/data",
				TargetType:      tt.targetType,
				StartTime:       1000,
				StopTime:        2000,
				DurationSeconds: 1000,
			}

			jsonBytes, err := json.Marshal(data)
			if err != nil {
				t.Fatalf("JSON marshal failed: %v", err)
			}

			var decoded PbRestoreResultData
			if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
				t.Fatalf("JSON unmarshal failed: %v", err)
			}

			if decoded.TargetType != tt.targetType {
				t.Errorf("TargetType mismatch: got %q, want %q", decoded.TargetType, tt.targetType)
			}
		})
	}
}

// TestPbRestoreResultData_WithBackupSet tests result with backup set specified.
func TestPbRestoreResultData_WithBackupSet(t *testing.T) {
	data := &PbRestoreResultData{
		Stanza:          "pg-meta",
		DataDir:         "/pg/data",
		RestoredBackup:  "20250204-120000F",
		TargetType:      "default",
		StartTime:       1738627200,
		StopTime:        1738627800,
		DurationSeconds: 600,
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	jsonStr := string(jsonBytes)
	if !containsStr(jsonStr, `"restored_backup":"20250204-120000F"`) {
		t.Errorf("JSON should contain restored_backup field, got: %s", jsonStr)
	}
}

// TestPbRestoreResultData_BooleanFields tests that boolean fields are always serialized.
func TestPbRestoreResultData_BooleanFields(t *testing.T) {
	// Test with both false values
	data := &PbRestoreResultData{
		Stanza:          "test",
		DataDir:         "/data",
		TargetType:      "default",
		Exclusive:       false,
		Promote:         false,
		StartTime:       1000,
		StopTime:        2000,
		DurationSeconds: 1000,
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	jsonStr := string(jsonBytes)
	// Boolean fields should be present even when false
	if !containsStr(jsonStr, `"exclusive":false`) {
		t.Errorf("JSON should contain exclusive:false, got: %s", jsonStr)
	}
	if !containsStr(jsonStr, `"promote":false`) {
		t.Errorf("JSON should contain promote:false, got: %s", jsonStr)
	}

	// Test with both true values
	data.Exclusive = true
	data.Promote = true
	jsonBytes, err = json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	jsonStr = string(jsonBytes)
	if !containsStr(jsonStr, `"exclusive":true`) {
		t.Errorf("JSON should contain exclusive:true, got: %s", jsonStr)
	}
	if !containsStr(jsonStr, `"promote":true`) {
		t.Errorf("JSON should contain promote:true, got: %s", jsonStr)
	}
}
