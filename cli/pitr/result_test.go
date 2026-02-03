package pitr

import (
	"encoding/json"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestPITRResultData(t *testing.T) {
	start := time.Date(2026, 1, 31, 1, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Second)

	state := &SystemState{DataDir: "/pg/data"}
	opts := &Options{Time: "2026-01-31 01:00:00", Set: "20240101-010101F", Promote: true}

	data := newPITRResultData(state, opts, true, true, start, end)
	if data.Target == "" || data.DataDir == "" {
		t.Fatalf("unexpected empty fields: %+v", data)
	}
	if data.BackupSet != "20240101-010101F" {
		t.Errorf("backup_set = %q, want %q", data.BackupSet, "20240101-010101F")
	}
	if data.DurationSeconds <= 0 {
		t.Errorf("duration_seconds = %f, want >0", data.DurationSeconds)
	}

	if _, err := yaml.Marshal(data); err != nil {
		t.Fatalf("yaml marshal failed: %v", err)
	}
	if _, err := json.Marshal(data); err != nil {
		t.Fatalf("json marshal failed: %v", err)
	}
}

func TestPITRResultDataDefaultBackup(t *testing.T) {
	start := time.Now()
	end := start.Add(1 * time.Second)
	data := newPITRResultData(&SystemState{DataDir: "/pg/data"}, &Options{Default: true}, false, false, start, end)
	if data.BackupSet != "latest" {
		t.Errorf("backup_set = %q, want %q", data.BackupSet, "latest")
	}
}

func TestPITRResultDataNilState(t *testing.T) {
	start := time.Now()
	end := start.Add(1 * time.Second)
	data := newPITRResultData(nil, &Options{Default: true}, false, false, start, end)
	if data.DataDir != "" {
		t.Errorf("DataDir should be empty with nil state, got %q", data.DataDir)
	}
	if data.BackupSet != "latest" {
		t.Errorf("backup_set = %q, want %q", data.BackupSet, "latest")
	}
}

func TestPITRResultDataNilOpts(t *testing.T) {
	start := time.Now()
	end := start.Add(1 * time.Second)
	data := newPITRResultData(&SystemState{DataDir: "/pg/data"}, nil, false, false, start, end)
	if data.Target != "unknown" {
		t.Errorf("Target should be 'unknown' with nil opts, got %q", data.Target)
	}
	if data.BackupSet != "latest" {
		t.Errorf("backup_set = %q, want %q", data.BackupSet, "latest")
	}
	if data.Promote != false {
		t.Error("Promote should be false with nil opts")
	}
	if data.Exclusive != false {
		t.Error("Exclusive should be false with nil opts")
	}
}

func TestPITRResultDataNegativeDuration(t *testing.T) {
	start := time.Now()
	end := start.Add(-1 * time.Second) // End before start
	data := newPITRResultData(&SystemState{DataDir: "/pg/data"}, &Options{Default: true}, false, false, start, end)
	if data.DurationSeconds != 0 {
		t.Errorf("duration_seconds should be 0 for negative duration, got %f", data.DurationSeconds)
	}
}

func TestPITRResultDataFlags(t *testing.T) {
	start := time.Now()
	end := start.Add(1 * time.Second)

	// Test Promote flag
	data := newPITRResultData(&SystemState{DataDir: "/pg/data"}, &Options{Default: true, Promote: true}, false, false, start, end)
	if !data.Promote {
		t.Error("Promote should be true")
	}

	// Test Exclusive flag
	data = newPITRResultData(&SystemState{DataDir: "/pg/data"}, &Options{Default: true, Exclusive: true}, false, false, start, end)
	if !data.Exclusive {
		t.Error("Exclusive should be true")
	}

	// Test PatroniStopped and PostgresRestarted
	data = newPITRResultData(&SystemState{DataDir: "/pg/data"}, &Options{Default: true}, true, true, start, end)
	if !data.PatroniStopped {
		t.Error("PatroniStopped should be true")
	}
	if !data.PostgresRestarted {
		t.Error("PostgresRestarted should be true")
	}
}

func TestPITRResultDataJSONTags(t *testing.T) {
	start := time.Date(2026, 1, 31, 1, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Second)

	state := &SystemState{DataDir: "/pg/data"}
	opts := &Options{Time: "2026-01-31 01:00:00", Promote: true, Exclusive: true}

	data := newPITRResultData(state, opts, true, true, start, end)
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("json marshal failed: %v", err)
	}

	jsonStr := string(jsonBytes)
	// Verify snake_case field names
	expectedFields := []string{
		"data_dir",
		"backup_set",
		"patroni_stopped",
		"postgres_restarted",
		"started_at",
		"completed_at",
		"duration_seconds",
	}
	for _, field := range expectedFields {
		if !contains(jsonStr, field) {
			t.Errorf("JSON should contain field %q: %s", field, jsonStr)
		}
	}
}

func TestPITRResultDataYAMLTags(t *testing.T) {
	start := time.Date(2026, 1, 31, 1, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Second)

	state := &SystemState{DataDir: "/pg/data"}
	opts := &Options{Default: true}

	data := newPITRResultData(state, opts, false, true, start, end)
	yamlBytes, err := yaml.Marshal(data)
	if err != nil {
		t.Fatalf("yaml marshal failed: %v", err)
	}

	yamlStr := string(yamlBytes)
	// Verify snake_case field names
	expectedFields := []string{
		"data_dir:",
		"backup_set:",
		"patroni_stopped:",
		"postgres_restarted:",
	}
	for _, field := range expectedFields {
		if !contains(yamlStr, field) {
			t.Errorf("YAML should contain field %q: %s", field, yamlStr)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
