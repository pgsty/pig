/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

Tests for pg status structured output.
*/
package postgres

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"
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
				ControlData: map[string]string{
					"Database cluster state": "in production",
				},
			},
			wantKeys: []string{"running", "pid", "version", "data_dir", "port", "uptime_seconds", "control_data"},
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

func TestParsePgControlData(t *testing.T) {
	parsed := ParsePgControlData(samplePgControlData())

	checks := map[string]string{
		"Database system identifier":              "7658473303274012732",
		"Database cluster state":                  "in production",
		"pg_control last modified":                "Sat Jul  4 04:30:46 2026",
		"Latest checkpoint's TimeLineID":          "2",
		"Latest checkpoint's REDO location":       "0/F000028",
		"Latest checkpoint's REDO WAL file":       "00000002000000000000000F",
		"Latest checkpoint's NextXID":             "0:1363",
		"Minimum recovery ending location":        "0/0",
		"End-of-backup record required":           "no",
		"Latest checkpoint's full_page_writes":    "on",
		"Data page checksum version":              "1",
		"wal_level setting":                       "logical",
		"Latest checkpoint's oldestXID's DB":      "1",
		"Latest checkpoint's oldestMulti's DB":    "1",
		"Latest checkpoint's NextMultiXactId":     "1",
		"Latest checkpoint's oldestMultiXid":      "1",
		"Mock authentication nonce":               "9462146558136027301",
		"Latest checkpoint's REDO location extra": "ignored",
	}
	for key, want := range checks {
		if key == "Latest checkpoint's REDO location extra" {
			if _, ok := parsed.Fields[key]; ok {
				t.Fatalf("unexpected synthetic key %q in parsed fields", key)
			}
			continue
		}
		if got := parsed.Fields[key]; got != want {
			t.Fatalf("Fields[%q] = %q, want %q", key, got, want)
		}
	}

	if len(parsed.Rows) == 0 {
		t.Fatal("expected ordered pg_controldata rows")
	}
	if parsed.Rows[0].Key != "pg_control version number" || parsed.Rows[0].Value != "1800" {
		t.Fatalf("first row = %+v, want pg_control version number", parsed.Rows[0])
	}
}

func TestRenderPgControlDataTable(t *testing.T) {
	parsed := ParsePgControlData(samplePgControlData())

	rendered := RenderPgControlDataTable(parsed)

	for _, want := range []string{
		"Key",
		"Value",
		"Database system identifier",
		"Database cluster state",
		"in production",
		"Latest checkpoint's REDO location",
		"0/F000028",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered control table missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "map[") {
		t.Fatalf("rendered control table should not expose Go map formatting:\n%s", rendered)
	}
}

func TestRenderPgStatusCompactSummary(t *testing.T) {
	parsed := ParsePgControlData(samplePgControlData())
	status := &PgStatusResultData{
		Version: 18,
		DataDir: "/pg/data",
	}

	rendered := RenderPgStatusCompactSummary(status, "replica", parsed)
	want := `[pg_controldata status]
PostgreSQL 18  DOWN replica  data=/pg/data
Cluster    7658473303274012732  state="in production"  timeline=2
Checkpoint time="2026-07-04 04:30:46"  redo=0/F000028  wal=00000002000000000000000F
TransactID xid=619 next=1363 oldest=744 db=1 active=1363  mxid=0 next=1 oldest=1 db=1
`

	if rendered != want {
		t.Fatalf("compact summary mismatch:\nwant:\n%s\ngot:\n%s", want, rendered)
	}
	for _, forbidden := range []string{"Safety", "Recovery", "Key", "Value", "wal_level", "full_page_writes", "Cluster    id=", "Age"} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("compact summary should not include %q:\n%s", forbidden, rendered)
		}
	}
}

func TestRenderPgStatusCompactSummaryColor(t *testing.T) {
	parsed := ParsePgControlData(samplePgControlData())
	status := &PgStatusResultData{
		Version: 18,
		DataDir: "/pg/data",
	}

	rendered := RenderPgStatusCompactSummaryColor(status, "replica", parsed)

	for _, want := range []string{
		utils.ColorBold + "[pg_controldata status]" + utils.ColorReset,
		utils.ColorRed + "DOWN" + utils.ColorReset,
		utils.ColorOrange + "replica" + utils.ColorReset,
		`state="` + utils.ColorGreen + "in production" + utils.ColorReset + `"`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("colored summary missing %q:\n%q", want, rendered)
		}
	}
}

func TestPgControlStateColorCoversKnownStates(t *testing.T) {
	for _, state := range pgControlDataStates {
		if got := pgControlStateColor(state); got == "" {
			t.Fatalf("pgControlStateColor(%q) returned empty color", state)
		}
	}
	if got := pgControlStateColor("unknown future state"); got != "" {
		t.Fatalf("unknown state color = %q, want empty", got)
	}
}

func TestPgStatusPalette(t *testing.T) {
	checks := map[string]string{
		"up":      pgRunningStateColor("UP"),
		"down":    pgRunningStateColor("DOWN"),
		"primary": pgRoleColor("primary"),
		"replica": pgRoleColor("replica"),
	}
	wants := map[string]string{
		"up":      utils.ColorGreen,
		"down":    utils.ColorRed,
		"primary": utils.ColorDarkBlue,
		"replica": utils.ColorOrange,
	}
	for key, got := range checks {
		if got != wants[key] {
			t.Fatalf("%s color = %q, want %q", key, got, wants[key])
		}
	}
}

func TestStatusResultIncludesControlDataWhenNotRunning(t *testing.T) {
	config.DetectEnvironment()
	if config.CurrentUser == "" {
		t.Skip("current user not detected")
	}

	original := pgControlDataOutput
	t.Cleanup(func() {
		pgControlDataOutput = original
	})
	pgControlDataOutput = func(cfg *Config, dbsu, dataDir string) (string, error) {
		return samplePgControlData(), nil
	}

	dataDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dataDir, "PG_VERSION"), []byte("18\n"), 0o644); err != nil {
		t.Fatalf("write PG_VERSION: %v", err)
	}

	result := StatusResult(&Config{PgData: dataDir, DbSU: config.CurrentUser})
	if result == nil {
		t.Fatal("StatusResult returned nil")
	}
	if result.Success {
		t.Fatal("expected not-running status to remain a failed state result")
	}
	data, ok := result.Data.(*PgStatusResultData)
	if !ok {
		t.Fatalf("result data type = %T, want *PgStatusResultData", result.Data)
	}
	if got := data.ControlData["Database cluster state"]; got != "in production" {
		t.Fatalf("control_data[Database cluster state] = %q, want in production", got)
	}
	if got := data.ControlData["Latest checkpoint's TimeLineID"]; got != "2" {
		t.Fatalf("control_data[Latest checkpoint's TimeLineID] = %q, want 2", got)
	}
}

func samplePgControlData() string {
	return `pg_control version number:            1800
Catalog version number:               202506291
Database system identifier:           7658473303274012732
Database cluster state:               in production
pg_control last modified:             Sat Jul  4 04:30:46 2026
Latest checkpoint location:           0/F000060
Latest checkpoint's REDO location:    0/F000028
Latest checkpoint's REDO WAL file:    00000002000000000000000F
Latest checkpoint's TimeLineID:       2
Latest checkpoint's PrevTimeLineID:   2
Time of latest checkpoint:            Sat Jul  4 04:30:46 2026
Latest checkpoint's full_page_writes: on
Latest checkpoint's NextXID:          0:1363
Latest checkpoint's oldestXID:        744
Latest checkpoint's oldestXID's DB:   1
Latest checkpoint's oldestActiveXID:  1363
Latest checkpoint's NextOID:          24576
Latest checkpoint's NextMultiXactId:  1
Latest checkpoint's NextMultiOffset:  0
Latest checkpoint's oldestMultiXid:   1
Latest checkpoint's oldestMulti's DB: 1
Minimum recovery ending location:     0/0
Min recovery ending loc's timeline:   0
Backup start location:                0/0
Backup end location:                  0/0
End-of-backup record required:        no
wal_level setting:                    logical
wal_log_hints setting:                on
Data page checksum version:           1
Mock authentication nonce:            9462146558136027301
`
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

func TestParsePostmasterPidInfo(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		wantPort      int
		wantSocketDir string
		wantUnix      int64
		wantErr       bool
	}{
		{
			name:          "epoch timestamp",
			content:       testPostmasterPid("/pg/data", "1738656000", "5432", "/var/run/postgresql"),
			wantPort:      5432,
			wantSocketDir: "/var/run/postgresql",
			wantUnix:      1738656000,
		},
		{
			name:          "timestamp string",
			content:       testPostmasterPid("/pg/data", "2025-02-04 00:00:00 UTC", "5433", "/var/run/postgresql"),
			wantPort:      5433,
			wantSocketDir: "/var/run/postgresql",
			wantUnix:      time.Date(2025, 2, 4, 0, 0, 0, 0, time.UTC).Unix(),
		},
		{
			name:     "empty socket dir",
			content:  testPostmasterPid("/pg/data", "1738656000", "6543", ""),
			wantPort: 6543,
			wantUnix: 1738656000,
		},
		{
			name:          "invalid start time keeps port binding",
			content:       testPostmasterPid("/pg/data", "not-a-start-time", "6543", "/tmp/pgsocket"),
			wantPort:      6543,
			wantSocketDir: "/tmp/pgsocket",
		},
		{
			name:    "insufficient lines",
			content: "12345\n/pg/data\n1738656000\n",
			wantErr: true,
		},
		{
			name:    "invalid port",
			content: "12345\n/pg/data\nnot-a-time\nnot-a-port\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := ParsePostmasterPidInfo(tt.content)
			if tt.wantErr {
				if err == nil {
					t.Fatal("ParsePostmasterPidInfo should return an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParsePostmasterPidInfo returned error: %v", err)
			}
			if info.Port != tt.wantPort {
				t.Errorf("Port = %d, want %d", info.Port, tt.wantPort)
			}
			if info.SocketDir != tt.wantSocketDir {
				t.Errorf("SocketDir = %q, want %q", info.SocketDir, tt.wantSocketDir)
			}
			if tt.wantUnix > 0 && info.StartTime.Unix() != tt.wantUnix {
				t.Errorf("StartTime unix = %d, want %d", info.StartTime.Unix(), tt.wantUnix)
			}
			if tt.wantUnix == 0 && !info.StartTime.IsZero() {
				t.Errorf("StartTime = %v, want zero time", info.StartTime)
			}
		})
	}
}

func TestReadPostmasterPidInfoAsDBSU(t *testing.T) {
	dataDir := t.TempDir()
	writeTestPostmasterPid(t, dataDir, "1738656000", "6543", "/tmp/pgsocket")

	info, err := ReadPostmasterPidInfoAsDBSU(config.CurrentUser, dataDir)
	if err != nil {
		t.Fatalf("ReadPostmasterPidInfoAsDBSU returned error: %v", err)
	}
	if info.Port != 6543 || info.SocketDir != "/tmp/pgsocket" {
		t.Fatalf("unexpected postmaster info: %+v", info)
	}
}

func writeTestPostmasterPid(t *testing.T, dataDir, startTime, port, socketDir string) {
	t.Helper()
	content := testPostmasterPid(dataDir, startTime, port, socketDir)
	if err := os.WriteFile(filepath.Join(dataDir, "postmaster.pid"), []byte(content), 0o644); err != nil {
		t.Fatalf("write postmaster.pid: %v", err)
	}
}

func testPostmasterPid(dataDir, startTime, port, socketDir string) string {
	return "12345\n" + dataDir + "\n" + startTime + "\n" + port + "\n" + socketDir + "\n127.0.0.1\n12345\n"
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
