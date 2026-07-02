/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

Tests for pb restore structured output result and DTO.
*/
package pgbackrest

import (
	"encoding/json"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"pig/internal/output"

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

func TestValidateRestoreOptionsRejectsInvalidTime(t *testing.T) {
	tests := []string{
		"2025-13-01",
		"2025-01-01 12:00:00junk",
		"25:00:00",
	}

	for _, value := range tests {
		t.Run(value, func(t *testing.T) {
			err := ValidateRestoreOptions(&RestoreOptions{Time: value})
			if err == nil {
				t.Fatalf("ValidateRestoreOptions should reject invalid --time %q", value)
			}
			if !strings.Contains(err.Error(), "invalid time format") {
				t.Fatalf("error should mention invalid time format, got %v", err)
			}
		})
	}
}

func TestValidateRestoreOptionsRejectsTargetExtraArgs(t *testing.T) {
	tests := [][]string{
		{"--type=time"},
		{"--target", "2025-01-01 00:00:00+08"},
		{"--target-action=promote"},
		{"--target-timeline", "latest"},
		{"--pg1-path=/tmp/restore"},
		{"--set=20250101-010101F"},
	}

	for _, args := range tests {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			err := ValidateRestoreOptions(&RestoreOptions{Default: true, ExtraArgs: args})
			if err == nil {
				t.Fatalf("ValidateRestoreOptions should reject conflicting extra args %v", args)
			}
			if !strings.Contains(err.Error(), "conflicts with pig restore flags") {
				t.Fatalf("error should explain conflicting extra args, got %v", err)
			}
		})
	}
}

func TestPatroniManagedRestoreErrorRejectsActiveManagedDataDir(t *testing.T) {
	t.Setenv("PGDATA", "")

	err := patroniManagedRestoreError(DefaultConfig(), &RestoreOptions{Default: true}, true)
	if err == nil {
		t.Fatal("active Patroni should block pb restore for managed PGDATA")
	}
	for _, want := range []string{"Patroni", "/pg/data", "pig pitr"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q should contain %q", err.Error(), want)
		}
	}
}

func TestPatroniManagedRestoreErrorAllowsInactiveOrCustomDataDir(t *testing.T) {
	t.Setenv("PGDATA", "")

	if err := patroniManagedRestoreError(DefaultConfig(), &RestoreOptions{Default: true}, false); err != nil {
		t.Fatalf("inactive Patroni should not block managed restore: %v", err)
	}
	if err := patroniManagedRestoreError(DefaultConfig(), &RestoreOptions{Default: true, DataDir: "/tmp/pig-restore"}, true); err != nil {
		t.Fatalf("active Patroni should not block custom restore target: %v", err)
	}
}

func TestPatroniManagedRestoreResultUsesStateCode(t *testing.T) {
	err := patroniManagedRestoreError(DefaultConfig(), &RestoreOptions{Default: true}, true)
	result := patroniManagedRestoreResult(err)
	if result == nil {
		t.Fatal("patroniManagedRestoreResult returned nil")
	}
	if result.Success {
		t.Fatalf("Patroni restore guard result should fail: %+v", result)
	}
	if result.Code != output.CodePbPatroniActive {
		t.Fatalf("result code = %d, want %d", result.Code, output.CodePbPatroniActive)
	}
	if !strings.Contains(result.Detail, "pig pitr") {
		t.Fatalf("result detail should point to pig pitr, got %q", result.Detail)
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

func TestValidateRestoreOptionsRejectsDefaultOnlyTargetModifiers(t *testing.T) {
	tests := []struct {
		name string
		opts *RestoreOptions
		want string
	}{
		{
			name: "default promote",
			opts: &RestoreOptions{Default: true, Promote: true},
			want: "--promote",
		},
		{
			name: "default exclusive",
			opts: &RestoreOptions{Default: true, Exclusive: true},
			want: "--exclusive",
		},
		{
			name: "name exclusive",
			opts: &RestoreOptions{Name: "restore_point", Exclusive: true},
			want: "--exclusive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRestoreOptions(tt.opts)
			if err == nil {
				t.Fatal("ValidateRestoreOptions should reject invalid target modifier combination")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ValidateRestoreOptions error = %q, want it to mention %q", err.Error(), tt.want)
			}
		})
	}
}

func TestValidateRestoreOptionsAcceptsTargetModifiersWithSupportedTargets(t *testing.T) {
	tests := []struct {
		name string
		opts *RestoreOptions
	}{
		{name: "time exclusive", opts: &RestoreOptions{Time: "2026-01-01 00:00:00+00", Exclusive: true}},
		{name: "lsn exclusive", opts: &RestoreOptions{LSN: "0/7C82CB8", Exclusive: true}},
		{name: "xid exclusive", opts: &RestoreOptions{XID: "12345", Exclusive: true}},
		{name: "name promote", opts: &RestoreOptions{Name: "restore_point", Promote: true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateRestoreOptions(tt.opts); err != nil {
				t.Fatalf("ValidateRestoreOptions should accept combination: %v", err)
			}
		})
	}
}

func TestBuildRestoreArgsIncludesTargetTimelineAndAction(t *testing.T) {
	opts := &RestoreOptions{Time: "2026-01-01 00:00:00+00"}
	setStringField(t, opts, "TargetTimeline", "current")
	setStringField(t, opts, "TargetAction", "shutdown")

	args := buildRestoreArgs(DefaultConfig(), opts, opts.Time)

	if !containsArg(args, "--target-timeline=current") {
		t.Fatalf("restore args should include target timeline, got %v", args)
	}
	if !containsArg(args, "--target-action=shutdown") {
		t.Fatalf("restore args should include target action, got %v", args)
	}
}

func TestBuildRestoreArgsAppendsExtraArgsAfterRestoreArgs(t *testing.T) {
	opts := &RestoreOptions{
		Time:           "2026-01-01 00:00:00+00",
		Set:            "20260101-000000F",
		TargetTimeline: "current",
	}
	setStringSliceField(t, opts, "ExtraArgs", []string{"--delta", "--process-max=4"})

	args := buildRestoreArgs(DefaultConfig(), opts, opts.Time)
	wantTail := []string{"--delta", "--process-max=4"}
	if len(args) < len(wantTail) {
		t.Fatalf("restore args too short: got %v", args)
	}
	if gotTail := args[len(args)-len(wantTail):]; !reflect.DeepEqual(gotTail, wantTail) {
		t.Fatalf("extra args should be appended at the end, got tail %v from %v", gotTail, args)
	}
	if !containsArg(args, "--set=20260101-000000F") || !containsArg(args, "--target-timeline=current") {
		t.Fatalf("restore args should retain built restore args before extra args, got %v", args)
	}
}

func TestValidateRestoreOptionsRejectsInvalidTimelineAndAction(t *testing.T) {
	t.Run("invalid timeline", func(t *testing.T) {
		opts := &RestoreOptions{Time: "2026-01-01 00:00:00+00"}
		setStringField(t, opts, "TargetTimeline", "branch-two")

		err := ValidateRestoreOptions(opts)
		if err == nil || !strings.Contains(err.Error(), "timeline") {
			t.Fatalf("ValidateRestoreOptions error = %v, want timeline validation", err)
		}
	})

	t.Run("invalid action", func(t *testing.T) {
		opts := &RestoreOptions{Time: "2026-01-01 00:00:00+00"}
		setStringField(t, opts, "TargetAction", "resume")

		err := ValidateRestoreOptions(opts)
		if err == nil || !strings.Contains(err.Error(), "target action") {
			t.Fatalf("ValidateRestoreOptions error = %v, want target action validation", err)
		}
	})
}

func setStringField(t *testing.T, target interface{}, fieldName, value string) {
	t.Helper()
	field := reflect.ValueOf(target).Elem().FieldByName(fieldName)
	if !field.IsValid() {
		t.Fatalf("%T should expose %s", target, fieldName)
	}
	if !field.CanSet() {
		t.Fatalf("%T.%s is not settable", target, fieldName)
	}
	field.SetString(value)
}

func setStringSliceField(t *testing.T, target interface{}, fieldName string, value []string) {
	t.Helper()
	field := reflect.ValueOf(target).Elem().FieldByName(fieldName)
	if !field.IsValid() {
		t.Fatalf("%T should expose %s", target, fieldName)
	}
	if !field.CanSet() {
		t.Fatalf("%T.%s is not settable", target, fieldName)
	}
	field.Set(reflect.ValueOf(value))
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func TestPrintPostRestoreHintsDefaultDoesNotSuggestPromote(t *testing.T) {
	output := capturePgBackRestStderr(t, func() {
		printPostRestoreHints(DefaultConfig(), &RestoreOptions{
			Default: true,
			DataDir: "/tmp/pig-pitr-restore",
		})
	})

	if !strings.Contains(output, "pg_ctl -D /tmp/pig-pitr-restore start") {
		t.Fatalf("default restore hints should include custom pg_ctl start, got:\n%s", output)
	}
	if strings.Contains(output, "promote") {
		t.Fatalf("default restore hints should not suggest manual promote, got:\n%s", output)
	}
}

func TestPrintPostRestoreHintsDefaultDataDirRenumbersStanzaStep(t *testing.T) {
	output := capturePgBackRestStderr(t, func() {
		printPostRestoreHints(DefaultConfig(), &RestoreOptions{Default: true})
	})

	if !strings.Contains(output, "3. Re-create stanza if needed:") {
		t.Fatalf("default restore hints should renumber stanza step to 3, got:\n%s", output)
	}
	if strings.Contains(output, "4. Re-create stanza if needed:") {
		t.Fatalf("default restore hints should not skip from step 2 to 4, got:\n%s", output)
	}
}

func TestPrintPostRestoreHintsManualTargetSuggestsPromote(t *testing.T) {
	output := capturePgBackRestStderr(t, func() {
		printPostRestoreHints(DefaultConfig(), &RestoreOptions{
			Time:    "2026-01-31 01:00:00",
			DataDir: "/tmp/pig-pitr-restore",
		})
	})

	if !strings.Contains(output, "pg_ctl -D /tmp/pig-pitr-restore promote") {
		t.Fatalf("manual target restore hints should suggest promote, got:\n%s", output)
	}
}

func capturePgBackRestStderr(t *testing.T, fn func()) string {
	t.Helper()
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	os.Stderr = w
	fn()
	if err := w.Close(); err != nil {
		t.Fatalf("failed to close stderr pipe: %v", err)
	}
	os.Stderr = oldStderr
	defer r.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("failed to read stderr pipe: %v", err)
	}
	return string(data)
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

// TestIsBackupNotFoundError guards the compound classifier: generic substrings
// like "not found" alone must not classify as backup-not-found (they previously
// misrouted automation via OR-semantics containsAny).
func TestIsBackupNotFoundError(t *testing.T) {
	tests := []struct {
		message string
		want    bool
	}{
		{"ERROR: [037]: no prior backup exists", true},
		{"unable to find backup set for stanza", true},
		{"no backup set found to restore", true},
		{"backup set 'foo' not found", true},
		{"backup set 19000101-000000F does not exist", true},
		{"backup set 19000101-000000F is not valid", true},
		// Former false positives under OR-semantics:
		{"pgbackrest not found (install with: pig ext add pgbackrest)", false},
		{"path '/nonexistent' does not exist", false},
		{"config file not found", false},
		{"unable to find primary cluster", false},
		{"restore process failed with timeout", false},
	}
	for _, tt := range tests {
		if got := IsBackupNotFoundError(tt.message); got != tt.want {
			t.Errorf("IsBackupNotFoundError(%q) = %v, want %v", tt.message, got, tt.want)
		}
	}
}
