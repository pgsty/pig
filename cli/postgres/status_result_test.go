/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

Tests for pg status structured output.
*/
package postgres

import (
	"encoding/json"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"pig/internal/config"
	"pig/internal/output"
)

// TestPgStatusResultData_JSON tests JSON serialization of PgStatusResultData
func TestPgStatusResultData_JSON(t *testing.T) {
	tests := []struct {
		name     string
		data     *PgStatusResultData
		wantKeys []string
	}{
		{
			name: "running instance with all fields",
			data: &PgStatusResultData{
				Running:       true,
				PID:           12345,
				Version:       17,
				DataDir:       "/pg/data",
				Port:          5432,
				UptimeSeconds: 3600,
			},
			wantKeys: []string{"running", "pid", "version", "data_dir", "port", "uptime_seconds"},
		},
		{
			name: "not running instance minimal",
			data: &PgStatusResultData{
				Running: false,
				DataDir: "/pg/data",
			},
			wantKeys: []string{"running", "data_dir"},
		},
		{
			name: "not running with version",
			data: &PgStatusResultData{
				Running: false,
				DataDir: "/pg/data",
				Version: 16,
			},
			wantKeys: []string{"running", "data_dir", "version"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.data)
			if err != nil {
				t.Fatalf("JSON marshal failed: %v", err)
			}

			// Verify JSON is valid
			var result map[string]interface{}
			if err := json.Unmarshal(data, &result); err != nil {
				t.Fatalf("JSON unmarshal failed: %v", err)
			}

			// Verify required keys are present
			for _, key := range tt.wantKeys {
				if _, ok := result[key]; !ok {
					t.Errorf("expected key %q not found in JSON", key)
				}
			}

			// Verify running value matches
			if running, ok := result["running"].(bool); ok {
				if running != tt.data.Running {
					t.Errorf("running = %v, want %v", running, tt.data.Running)
				}
			} else {
				t.Error("running field is not a bool")
			}
		})
	}
}

// TestPgStatusResultData_YAML tests YAML serialization of PgStatusResultData
func TestPgStatusResultData_YAML(t *testing.T) {
	tests := []struct {
		name     string
		data     *PgStatusResultData
		wantKeys []string
	}{
		{
			name: "running instance",
			data: &PgStatusResultData{
				Running:       true,
				PID:           12345,
				Version:       17,
				DataDir:       "/pg/data",
				Port:          5432,
				UptimeSeconds: 3600,
			},
			wantKeys: []string{"running", "pid", "version", "data_dir", "port", "uptime_seconds"},
		},
		{
			name: "not running instance",
			data: &PgStatusResultData{
				Running: false,
				DataDir: "/pg/data",
			},
			wantKeys: []string{"running", "data_dir"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := yaml.Marshal(tt.data)
			if err != nil {
				t.Fatalf("YAML marshal failed: %v", err)
			}

			// Verify YAML is valid
			var result map[string]interface{}
			if err := yaml.Unmarshal(data, &result); err != nil {
				t.Fatalf("YAML unmarshal failed: %v", err)
			}

			// Verify required keys are present
			for _, key := range tt.wantKeys {
				if _, ok := result[key]; !ok {
					t.Errorf("expected key %q not found in YAML", key)
				}
			}
		})
	}
}

// TestPgStatusResultData_OmitEmpty tests that omitempty works correctly
func TestPgStatusResultData_OmitEmpty(t *testing.T) {
	data := &PgStatusResultData{
		Running: false,
		DataDir: "/pg/data",
		// PID, Version, Port, UptimeSeconds are zero values
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	// These should be omitted due to omitempty
	omitFields := []string{"pid", "version", "port", "uptime_seconds"}
	for _, field := range omitFields {
		if _, ok := result[field]; ok {
			t.Errorf("field %q should be omitted when zero", field)
		}
	}

	// These should always be present
	requiredFields := []string{"running", "data_dir"}
	for _, field := range requiredFields {
		if _, ok := result[field]; !ok {
			t.Errorf("required field %q missing", field)
		}
	}
}

// TestStatusResult_NilConfig tests StatusResult with nil config
func TestStatusResult_NilConfig(t *testing.T) {
	config.DetectEnvironment()
	if config.CurrentUser == "" {
		t.Skip("current user not detected")
	}
	t.Setenv("PIG_DBSU", config.CurrentUser)

	// StatusResult should handle nil config safely, using defaults
	result := StatusResult(nil)

	if result == nil {
		t.Fatal("StatusResult returned nil result")
	}

	// Result should have data even on failure
	if result.Data == nil {
		t.Error("Result.Data should not be nil")
	}

	// Check that data_dir uses default
	if data, ok := result.Data.(*PgStatusResultData); ok {
		if data.DataDir != DefaultPgData {
			t.Errorf("DataDir = %q, want %q", data.DataDir, DefaultPgData)
		}
	}
}

// TestStatusResult_ErrorCodes tests that StatusResult returns correct error codes
func TestStatusResult_ErrorCodes(t *testing.T) {
	config.DetectEnvironment()
	if config.CurrentUser == "" {
		t.Skip("current user not detected")
	}
	// Test with non-existent data directory
	cfg := &Config{
		PgData: "/nonexistent/path/that/should/not/exist/12345",
		DbSU:   config.CurrentUser,
	}

	result := StatusResult(cfg)

	if result == nil {
		t.Fatal("StatusResult returned nil")
	}

	// Should fail with data directory not found
	if result.Success {
		t.Error("expected failure for non-existent data directory")
	}

	// Code should be CodePgStatusDataDirNotFound or CodePgStatusNotInitialized
	if result.Code != output.CodePgStatusDataDirNotFound &&
		result.Code != output.CodePgStatusNotInitialized {
		t.Errorf("unexpected error code: %d", result.Code)
	}

	// Data should still be populated
	if result.Data == nil {
		t.Error("Result.Data should not be nil even on failure")
	}

	if data, ok := result.Data.(*PgStatusResultData); ok {
		if data.Running {
			t.Error("Running should be false for non-existent directory")
		}
		if data.DataDir != cfg.PgData {
			t.Errorf("DataDir = %q, want %q", data.DataDir, cfg.PgData)
		}
	}
}

// TestParsePostmasterPidInfo tests parsing of postmaster.pid content
func TestParsePostmasterPidInfo(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantPort    int
		wantUnix    int64
		expectZero  bool
	}{
		{
			name: "epoch timestamp",
			content: "12345\n/pg/data\n1738656000\n5432\n/var/run/postgresql\n127.0.0.1\n12345\n",
			wantPort: 5432,
			wantUnix: 1738656000,
		},
		{
			name: "timestamp string",
			content: "12345\n/pg/data\n2025-02-04 00:00:00 UTC\n5433\n/var/run/postgresql\n127.0.0.1\n12345\n",
			wantPort: 5433,
			wantUnix: time.Date(2025, 2, 4, 0, 0, 0, 0, time.UTC).Unix(),
		},
		{
			name:       "insufficient lines",
			content:    "12345\n/pg/data\n",
			expectZero: true,
		},
		{
			name:       "invalid port and time",
			content:    "12345\n/pg/data\nnot-a-time\nnot-a-port\n",
			expectZero: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			port, startTime := parsePostmasterPidInfo(tt.content)
			if tt.expectZero {
				if port != 0 {
					t.Errorf("port = %d, want 0", port)
				}
				if !startTime.IsZero() {
					t.Errorf("startTime should be zero, got %v", startTime)
				}
				return
			}
			if port != tt.wantPort {
				t.Errorf("port = %d, want %d", port, tt.wantPort)
			}
			if startTime.Unix() != tt.wantUnix {
				t.Errorf("startTime unix = %d, want %d", startTime.Unix(), tt.wantUnix)
			}
		})
	}
}

// TestPgStatusResultData_RoundTrip tests JSON round-trip
func TestPgStatusResultData_RoundTrip(t *testing.T) {
	original := &PgStatusResultData{
		Running:       true,
		PID:           12345,
		Version:       17,
		DataDir:       "/pg/data",
		Port:          5432,
		UptimeSeconds: 3600,
	}

	// Marshal to JSON
	jsonBytes, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	// Unmarshal back
	var restored PgStatusResultData
	if err := json.Unmarshal(jsonBytes, &restored); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	// Verify values match
	if restored.Running != original.Running {
		t.Errorf("Running = %v, want %v", restored.Running, original.Running)
	}
	if restored.PID != original.PID {
		t.Errorf("PID = %d, want %d", restored.PID, original.PID)
	}
	if restored.Version != original.Version {
		t.Errorf("Version = %d, want %d", restored.Version, original.Version)
	}
	if restored.DataDir != original.DataDir {
		t.Errorf("DataDir = %q, want %q", restored.DataDir, original.DataDir)
	}
	if restored.Port != original.Port {
		t.Errorf("Port = %d, want %d", restored.Port, original.Port)
	}
	if restored.UptimeSeconds != original.UptimeSeconds {
		t.Errorf("UptimeSeconds = %d, want %d", restored.UptimeSeconds, original.UptimeSeconds)
	}
}

// TestResultWithPgStatusData tests output.Result with PgStatusResultData
func TestResultWithPgStatusData(t *testing.T) {
	statusData := &PgStatusResultData{
		Running:       true,
		PID:           12345,
		Version:       17,
		DataDir:       "/pg/data",
		Port:          5432,
		UptimeSeconds: 3600,
	}

	// Create success result
	result := output.OK("PostgreSQL is running", statusData)

	if result == nil {
		t.Fatal("OK returned nil")
	}
	if !result.Success {
		t.Error("Success should be true")
	}
	if result.Code != 0 {
		t.Errorf("Code = %d, want 0", result.Code)
	}
	if result.Data == nil {
		t.Error("Data should not be nil")
	}

	// Verify JSON rendering
	jsonBytes, err := result.JSON()
	if err != nil {
		t.Fatalf("JSON() failed: %v", err)
	}

	var rendered map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &rendered); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	// Verify data is included
	if _, ok := rendered["data"]; !ok {
		t.Error("data field missing from rendered JSON")
	}
}

// TestResultWithPgStatusData_Failure tests failure result
func TestResultWithPgStatusData_Failure(t *testing.T) {
	statusData := &PgStatusResultData{
		Running: false,
		DataDir: "/pg/data",
	}

	// Create failure result
	result := output.Fail(output.CodePgStatusNotRunning, "PostgreSQL is not running").
		WithData(statusData)

	if result == nil {
		t.Fatal("Fail returned nil")
	}
	if result.Success {
		t.Error("Success should be false")
	}
	if result.Code != output.CodePgStatusNotRunning {
		t.Errorf("Code = %d, want %d", result.Code, output.CodePgStatusNotRunning)
	}

	// Verify exit code is correct (state error = 9)
	exitCode := result.ExitCode()
	if exitCode != 9 {
		t.Errorf("ExitCode() = %d, want 9 (state error)", exitCode)
	}
}
