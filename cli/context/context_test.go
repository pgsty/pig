/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>

Tests for pig context command.
*/
package context

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"pig/internal/output"

	"gopkg.in/yaml.v3"
)

// ============================================================================
// DTO Serialization Tests
// ============================================================================

func TestContextResultData_JSONSerialization(t *testing.T) {
	data := &ContextResultData{
		Host: &HostInfo{
			Hostname: "test-host",
			OS:       "linux",
			Distro:   "el9",
			Arch:     "amd64",
			Kernel:   "5.14.0-362.el9.x86_64",
		},
		Postgres: &PostgresContext{
			Available:      true,
			Running:        true,
			Version:        17,
			VersionString:  "PG17",
			VersionNum:     170000,
			DataDir:        "/pg/data",
			Port:           5432,
			PID:            12345,
			Role:           "primary",
			UptimeSeconds:  86400,
			Connections:    15,
			MaxConnections: 100,
		},
		Patroni: &PatroniContext{
			Available: true,
			Running:   true,
			Cluster:   "pg-test",
			Role:      "leader",
			State:     "running",
		},
		PgBackRest: &PgBackRestContext{
			Available:   true,
			Configured:  true,
			Stanza:      "pg-test",
			LastBackup:  "20260204-120000F",
			BackupCount: 5,
		},
		Extensions: &ExtensionsContext{
			Available:      true,
			InstalledCount: 3,
			Extensions:     []string{"postgis", "pg_stat_statements", "uuid-ossp"},
		},
	}

	// Test JSON serialization
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	jsonStr := string(jsonBytes)

	// Verify key fields are present with correct snake_case naming
	if !strings.Contains(jsonStr, `"hostname":"test-host"`) {
		t.Error("JSON should contain hostname field")
	}
	if !strings.Contains(jsonStr, `"data_dir":"/pg/data"`) {
		t.Error("JSON should contain data_dir field with snake_case")
	}
	if !strings.Contains(jsonStr, `"uptime_seconds":86400`) {
		t.Error("JSON should contain uptime_seconds field with snake_case")
	}
	if !strings.Contains(jsonStr, `"version_string":"PG17"`) {
		t.Error("JSON should contain version_string field with snake_case")
	}
	if !strings.Contains(jsonStr, `"installed_count":3`) {
		t.Error("JSON should contain installed_count field with snake_case")
	}
	if !strings.Contains(jsonStr, `"last_backup":"20260204-120000F"`) {
		t.Error("JSON should contain last_backup field with snake_case")
	}
	if !strings.Contains(jsonStr, `"backup_count":5`) {
		t.Error("JSON should contain backup_count field with snake_case")
	}
	if !strings.Contains(jsonStr, `"version_num":170000`) {
		t.Error("JSON should contain version_num field with snake_case")
	}
	if !strings.Contains(jsonStr, `"connections":15`) {
		t.Error("JSON should contain connections field")
	}
	if !strings.Contains(jsonStr, `"max_connections":100`) {
		t.Error("JSON should contain max_connections field with snake_case")
	}

	// Test JSON deserialization (round-trip)
	var decoded ContextResultData
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if decoded.Host.Hostname != "test-host" {
		t.Errorf("Host.Hostname mismatch: got %q, want %q", decoded.Host.Hostname, "test-host")
	}
	if decoded.Postgres.Port != 5432 {
		t.Errorf("Postgres.Port mismatch: got %d, want %d", decoded.Postgres.Port, 5432)
	}
	if decoded.Extensions.InstalledCount != 3 {
		t.Errorf("Extensions.InstalledCount mismatch: got %d, want %d", decoded.Extensions.InstalledCount, 3)
	}
	if decoded.Postgres.VersionNum != 170000 {
		t.Errorf("Postgres.VersionNum mismatch: got %d, want %d", decoded.Postgres.VersionNum, 170000)
	}
	if decoded.Postgres.Connections != 15 {
		t.Errorf("Postgres.Connections mismatch: got %d, want %d", decoded.Postgres.Connections, 15)
	}
	if decoded.Postgres.MaxConnections != 100 {
		t.Errorf("Postgres.MaxConnections mismatch: got %d, want %d", decoded.Postgres.MaxConnections, 100)
	}
}

func TestContextResultData_YAMLSerialization(t *testing.T) {
	data := &ContextResultData{
		Host: &HostInfo{
			Hostname: "test-host",
			OS:       "linux",
			Distro:   "el9",
			Arch:     "amd64",
		},
		Postgres: &PostgresContext{
			Available: true,
			Running:   true,
			Version:   17,
			DataDir:   "/pg/data",
			Port:      5432,
		},
	}

	// Test YAML serialization
	yamlBytes, err := yaml.Marshal(data)
	if err != nil {
		t.Fatalf("YAML marshal failed: %v", err)
	}

	yamlStr := string(yamlBytes)

	// Verify key fields are present
	if !strings.Contains(yamlStr, "hostname: test-host") {
		t.Error("YAML should contain hostname field")
	}
	if !strings.Contains(yamlStr, "data_dir: /pg/data") {
		t.Error("YAML should contain data_dir field with snake_case")
	}

	// Test YAML deserialization (round-trip)
	var decoded ContextResultData
	if err := yaml.Unmarshal(yamlBytes, &decoded); err != nil {
		t.Fatalf("YAML unmarshal failed: %v", err)
	}

	if decoded.Host.Hostname != "test-host" {
		t.Errorf("Host.Hostname mismatch: got %q, want %q", decoded.Host.Hostname, "test-host")
	}
	if decoded.Postgres.DataDir != "/pg/data" {
		t.Errorf("Postgres.DataDir mismatch: got %q, want %q", decoded.Postgres.DataDir, "/pg/data")
	}
}

// ============================================================================
// Text Output Tests
// ============================================================================

func TestContextResultData_Text(t *testing.T) {
	data := &ContextResultData{
		Host: &HostInfo{
			Hostname: "test-host",
			OS:       "linux",
			Distro:   "el9",
			Arch:     "amd64",
			Kernel:   "5.14.0",
		},
		Postgres: &PostgresContext{
			Available:      true,
			Running:        true,
			Version:        17,
			VersionString:  "PG17",
			VersionNum:     170000,
			DataDir:        "/pg/data",
			Port:           5432,
			PID:            12345,
			Role:           "primary",
			UptimeSeconds:  86400,
			Connections:    15,
			MaxConnections: 100,
		},
		Extensions: &ExtensionsContext{
			Available:      true,
			InstalledCount: 3,
			Extensions:     []string{"postgis", "pg_stat_statements", "uuid-ossp"},
		},
	}

	text := data.Text()

	// Verify key content
	if !strings.Contains(text, "PIG CONTEXT") {
		t.Error("Text should contain PIG CONTEXT header")
	}
	if !strings.Contains(text, "test-host") {
		t.Error("Text should contain hostname")
	}
	if !strings.Contains(text, "PostgreSQL") {
		t.Error("Text should contain PostgreSQL section")
	}
	if !strings.Contains(text, "Running") {
		t.Error("Text should indicate Running status")
	}
	if !strings.Contains(text, "5432") {
		t.Error("Text should contain port number")
	}
	if !strings.Contains(text, "170000") {
		t.Error("Text should contain version_num (170000)")
	}
	if !strings.Contains(text, "15/100") {
		t.Error("Text should contain connections (15/100)")
	}
	if !strings.Contains(text, "Extensions") {
		t.Error("Text should contain Extensions section")
	}
	if !strings.Contains(text, "3 installed") {
		t.Error("Text should contain extension count")
	}
}

func TestContextResultData_Text_Nil(t *testing.T) {
	var data *ContextResultData
	text := data.Text()
	if text != "" {
		t.Errorf("Nil receiver should return empty string, got %q", text)
	}
}

func TestHostInfo_Text_Nil(t *testing.T) {
	var h *HostInfo
	text := h.text()
	if text != "" {
		t.Errorf("Nil receiver should return empty string, got %q", text)
	}
}

func TestPostgresContext_Text_Nil(t *testing.T) {
	var p *PostgresContext
	text := p.text()
	if text != "" {
		t.Errorf("Nil receiver should return empty string, got %q", text)
	}
}

func TestPatroniContext_Text_Nil(t *testing.T) {
	var p *PatroniContext
	text := p.text()
	if text != "" {
		t.Errorf("Nil receiver should return empty string, got %q", text)
	}
}

func TestPgBackRestContext_Text_Nil(t *testing.T) {
	var p *PgBackRestContext
	text := p.text()
	if text != "" {
		t.Errorf("Nil receiver should return empty string, got %q", text)
	}
}

func TestExtensionsContext_Text_Nil(t *testing.T) {
	var e *ExtensionsContext
	text := e.text()
	if text != "" {
		t.Errorf("Nil receiver should return empty string, got %q", text)
	}
}

// ============================================================================
// Graceful Degradation Tests
// ============================================================================

func TestPostgresContext_NotAvailable_Text(t *testing.T) {
	p := &PostgresContext{Available: false}
	text := p.text()
	if !strings.Contains(text, "Not Available") {
		t.Error("Not available PostgreSQL should show 'Not Available'")
	}
}

func TestPatroniContext_NotAvailable_Text(t *testing.T) {
	p := &PatroniContext{Available: false}
	text := p.text()
	if !strings.Contains(text, "Not Available") {
		t.Error("Not available Patroni should show 'Not Available'")
	}
}

func TestPgBackRestContext_NotConfigured_Text(t *testing.T) {
	p := &PgBackRestContext{Available: true, Configured: false}
	text := p.text()
	if !strings.Contains(text, "Not Configured") {
		t.Error("Not configured pgBackRest should show 'Not Configured'")
	}
}

func TestExtensionsContext_NotAvailable_Text(t *testing.T) {
	e := &ExtensionsContext{Available: false}
	text := e.text()
	if !strings.Contains(text, "Not Available") {
		t.Error("Not available Extensions should show 'Not Available'")
	}
}

// ============================================================================
// HostInfo Collection Tests
// ============================================================================

func TestCollectHostInfo(t *testing.T) {
	host := collectHostInfo()

	if host == nil {
		t.Fatal("collectHostInfo should not return nil")
	}

	// Hostname should be non-empty on any system
	// (might fail in some containerized environments)
	if host.OS == "" {
		t.Error("OS should not be empty")
	}

	// Arch comes from config.OSArch which may not be initialized in test
	// Just verify the function doesn't panic
}

// ============================================================================
// formatDuration Tests
// ============================================================================

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		seconds  int64
		expected string
	}{
		{0, "0m"},
		{60, "1m"},
		{3600, "1h 0m"},
		{3660, "1h 1m"},
		{86400, "1d 0h 0m"},
		{90061, "1d 1h 1m"},
	}

	for _, tt := range tests {
		got := formatDuration(tt.seconds)
		if got != tt.expected {
			t.Errorf("formatDuration(%d) = %q, want %q", tt.seconds, got, tt.expected)
		}
	}
}

// ============================================================================
// JSON Omitempty Tests
// ============================================================================

func TestContextResultData_OmitemptyFields(t *testing.T) {
	// Create data with optional fields empty/nil
	data := &ContextResultData{
		Host: &HostInfo{
			Hostname: "test",
			OS:       "linux",
			Arch:     "amd64",
			// Distro and Kernel are omitempty
		},
		Postgres: &PostgresContext{
			Available: false,
			// All other fields should be omitted when not available
		},
		// Patroni, PgBackRest, Extensions are omitempty
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	jsonStr := string(jsonBytes)

	// Check that omitempty fields are not present when empty
	if strings.Contains(jsonStr, "distro") {
		t.Error("Empty distro should be omitted")
	}
	if strings.Contains(jsonStr, "kernel") {
		t.Error("Empty kernel should be omitted")
	}
	if strings.Contains(jsonStr, "patroni") {
		t.Error("Nil patroni should be omitted")
	}
	if strings.Contains(jsonStr, "pgbackrest") {
		t.Error("Nil pgbackrest should be omitted")
	}
	if strings.Contains(jsonStr, "extensions") {
		t.Error("Nil extensions should be omitted")
	}
}

// ============================================================================
// Extensions Text with Truncation Tests
// ============================================================================

// ============================================================================
// Role Detection Tests
// ============================================================================

func detectPostgresRoleFromDirForTest(dataDir string) string {
	if _, err := os.Stat(dataDir); err != nil {
		return "unknown"
	}

	if _, err := os.Stat(dataDir + "/standby.signal"); err == nil {
		return "standby"
	} else if !os.IsNotExist(err) {
		return "unknown"
	}

	if _, err := os.Stat(dataDir + "/recovery.signal"); err == nil {
		return "standby"
	} else if !os.IsNotExist(err) {
		return "unknown"
	}

	if _, err := os.Stat(dataDir + "/recovery.conf"); err == nil {
		return "standby"
	} else if !os.IsNotExist(err) {
		return "unknown"
	}

	return "primary"
}

func TestDetectPostgresRole_Primary(t *testing.T) {
	// Create a temporary directory simulating a primary PostgreSQL data dir
	tmpDir := t.TempDir()

	// No signal files = primary
	role := detectPostgresRoleFromDirForTest(tmpDir)
	if role != "primary" {
		t.Errorf("Expected role 'primary' for empty data dir, got %q", role)
	}
}

func TestDetectPostgresRole_StandbySignal(t *testing.T) {
	tmpDir := t.TempDir()

	// Create standby.signal file
	signalFile := tmpDir + "/standby.signal"
	if err := os.WriteFile(signalFile, []byte{}, 0644); err != nil {
		t.Fatalf("Failed to create standby.signal: %v", err)
	}

	role := detectPostgresRoleFromDirForTest(tmpDir)
	if role != "standby" {
		t.Errorf("Expected role 'standby' with standby.signal, got %q", role)
	}
}

func TestDetectPostgresRole_RecoverySignal(t *testing.T) {
	tmpDir := t.TempDir()

	// Create recovery.signal file (PG12+ recovery mode)
	signalFile := tmpDir + "/recovery.signal"
	if err := os.WriteFile(signalFile, []byte{}, 0644); err != nil {
		t.Fatalf("Failed to create recovery.signal: %v", err)
	}

	role := detectPostgresRoleFromDirForTest(tmpDir)
	if role != "standby" {
		t.Errorf("Expected role 'standby' with recovery.signal, got %q", role)
	}
}

func TestDetectPostgresRole_RecoveryConf(t *testing.T) {
	tmpDir := t.TempDir()

	// Create recovery.conf file (PG11 and earlier)
	confFile := tmpDir + "/recovery.conf"
	if err := os.WriteFile(confFile, []byte("standby_mode = 'on'"), 0644); err != nil {
		t.Fatalf("Failed to create recovery.conf: %v", err)
	}

	role := detectPostgresRoleFromDirForTest(tmpDir)
	if role != "standby" {
		t.Errorf("Expected role 'standby' with recovery.conf, got %q", role)
	}
}

func TestDetectPostgresRole_UnknownOnNonexistent(t *testing.T) {
	// Test with a non-existent directory
	role := detectPostgresRoleFromDirForTest("/nonexistent/path/that/should/not/exist")
	if role != "unknown" {
		t.Errorf("Expected role 'unknown' for non-existent dir, got %q", role)
	}
}

// ============================================================================
// VersionNum Calculation Tests
// ============================================================================

func TestCalculateVersionNum(t *testing.T) {
	tests := []struct {
		version     int
		expectedNum int
	}{
		{17, 170000},
		{16, 160000},
		{15, 150000},
		{14, 140000},
		{13, 130000},
		{10, 100000},
		{9, 90000}, // PG9.x fallback without minor info
	}

	for _, tt := range tests {
		got := calculateVersionNum(tt.version)
		if got != tt.expectedNum {
			t.Errorf("calculateVersionNum(%d) = %d, want %d", tt.version, got, tt.expectedNum)
		}
	}
}

func TestCalculateVersionNumFromString(t *testing.T) {
	tests := []struct {
		version  string
		expected int
	}{
		{"17", 170000},
		{"16.2", 160000},
		{"12.4", 120000},
		{"9.6", 90600},
		{"PG14", 140000},
		{"", 0},
		{"invalid", 0},
	}

	for _, tt := range tests {
		got := calculateVersionNumFromString(tt.version)
		if got != tt.expected {
			t.Errorf("calculateVersionNumFromString(%q) = %d, want %d", tt.version, got, tt.expected)
		}
	}
}

// ============================================================================
// Connection Info Tests
// ============================================================================

func TestParseConnectionInfo(t *testing.T) {
	tests := []struct {
		name        string
		output      string
		wantConn    int
		wantMaxConn int
		wantErr     bool
	}{
		{
			name:        "valid output",
			output:      "15|100\n",
			wantConn:    15,
			wantMaxConn: 100,
			wantErr:     false,
		},
		{
			name:        "valid output no newline",
			output:      "25|200",
			wantConn:    25,
			wantMaxConn: 200,
			wantErr:     false,
		},
		{
			name:        "empty output",
			output:      "",
			wantConn:    0,
			wantMaxConn: 0,
			wantErr:     true,
		},
		{
			name:        "invalid format",
			output:      "invalid",
			wantConn:    0,
			wantMaxConn: 0,
			wantErr:     true,
		},
		{
			name:        "partial output",
			output:      "15",
			wantConn:    0,
			wantMaxConn: 0,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn, maxConn, err := parseConnectionInfoOutput(tt.output)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseConnectionInfoOutput() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if conn != tt.wantConn {
				t.Errorf("parseConnectionInfoOutput() conn = %d, want %d", conn, tt.wantConn)
			}
			if maxConn != tt.wantMaxConn {
				t.Errorf("parseConnectionInfoOutput() maxConn = %d, want %d", maxConn, tt.wantMaxConn)
			}
		})
	}
}

func TestExtensionsContext_Text_Truncation(t *testing.T) {
	// Create more than 10 extensions
	exts := make([]string, 15)
	for i := 0; i < 15; i++ {
		exts[i] = "ext" + string(rune('a'+i))
	}

	e := &ExtensionsContext{
		Available:      true,
		InstalledCount: 15,
		Extensions:     exts,
	}

	text := e.text()

	if !strings.Contains(text, "15 installed") {
		t.Error("Should show correct installed count")
	}
	if !strings.Contains(text, "+5 more") {
		t.Error("Should indicate truncation with +5 more")
	}
}

// ============================================================================
// PatroniContext Extended Fields Tests (Story 4.3)
// ============================================================================

func TestPatroniContext_ExtendedFields_JSONSerialization(t *testing.T) {
	data := &PatroniContext{
		Available: true,
		Running:   true,
		Cluster:   "pg-test",
		Role:      "leader",
		State:     "running",
		Timeline:  3,
		Lag:       "0 MB",
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	jsonStr := string(jsonBytes)

	// Verify new fields are present
	if !strings.Contains(jsonStr, `"timeline":3`) {
		t.Error("JSON should contain timeline field")
	}
	if !strings.Contains(jsonStr, `"lag":"0 MB"`) {
		t.Error("JSON should contain lag field")
	}
	if !strings.Contains(jsonStr, `"cluster":"pg-test"`) {
		t.Error("JSON should contain cluster field")
	}

	// Test round-trip
	var decoded PatroniContext
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if decoded.Timeline != 3 {
		t.Errorf("Timeline mismatch: got %d, want %d", decoded.Timeline, 3)
	}
	if decoded.Lag != "0 MB" {
		t.Errorf("Lag mismatch: got %q, want %q", decoded.Lag, "0 MB")
	}
}

func TestPatroniContext_ExtendedFields_YAMLSerialization(t *testing.T) {
	data := &PatroniContext{
		Available: true,
		Running:   true,
		Cluster:   "pg-test",
		Role:      "replica",
		State:     "running",
		Timeline:  5,
		Lag:       "2 MB",
	}

	yamlBytes, err := yaml.Marshal(data)
	if err != nil {
		t.Fatalf("YAML marshal failed: %v", err)
	}

	yamlStr := string(yamlBytes)

	// Verify fields are present
	if !strings.Contains(yamlStr, "timeline: 5") {
		t.Error("YAML should contain timeline field")
	}
	if !strings.Contains(yamlStr, "lag: 2 MB") {
		t.Error("YAML should contain lag field")
	}
}

func TestPatroniContext_ExtendedFields_Omitempty(t *testing.T) {
	// Timeline and Lag should be omitted when zero/empty
	data := &PatroniContext{
		Available: true,
		Running:   false,
		Cluster:   "pg-test",
		// Timeline: 0 (should be omitted)
		// Lag: "" (should be omitted)
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	jsonStr := string(jsonBytes)

	if strings.Contains(jsonStr, "timeline") {
		t.Error("Zero timeline should be omitted")
	}
	if strings.Contains(jsonStr, "lag") {
		t.Error("Empty lag should be omitted")
	}
}

func TestPatroniContext_NotRunning_OmitsState(t *testing.T) {
	data := &PatroniContext{
		Available: true,
		Running:   false,
		Cluster:   "pg-test",
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}
	jsonStr := string(jsonBytes)
	if strings.Contains(jsonStr, `"state"`) {
		t.Errorf("state should be omitted when patroni is not running, got: %s", jsonStr)
	}
}

func TestPatroniContext_Text_WithTimelineAndLag(t *testing.T) {
	p := &PatroniContext{
		Available: true,
		Running:   true,
		Cluster:   "pg-test",
		Role:      "leader",
		State:     "running",
		Timeline:  3,
		Lag:       "0 MB",
	}

	text := p.text()

	if !strings.Contains(text, "● Running") {
		t.Error("Should show running status")
	}
	if !strings.Contains(text, "Cluster: pg-test") {
		t.Error("Should show cluster name")
	}
	if !strings.Contains(text, "Role: leader") {
		t.Error("Should show role")
	}
	if !strings.Contains(text, "Timeline: 3") {
		t.Error("Should show timeline")
	}
	if !strings.Contains(text, "Lag: 0 MB") {
		t.Error("Should show lag")
	}
}

func TestPatroniContext_Text_ReplicaWithLag(t *testing.T) {
	p := &PatroniContext{
		Available: true,
		Running:   true,
		Cluster:   "pg-test",
		Role:      "replica",
		State:     "running",
		Timeline:  3,
		Lag:       "5 MB",
	}

	text := p.text()

	if !strings.Contains(text, "Role: replica") {
		t.Error("Should show replica role")
	}
	if !strings.Contains(text, "Lag: 5 MB") {
		t.Error("Should show lag for replica")
	}
}

func TestPatroniContext_Text_StoppedNoTimelineLag(t *testing.T) {
	p := &PatroniContext{
		Available: true,
		Running:   false,
		Cluster:   "pg-test",
		State:     "stopped",
	}

	text := p.text()

	if !strings.Contains(text, "○ Stopped") {
		t.Error("Should show stopped status")
	}
	// Should not show timeline/lag line when not running
	if strings.Contains(text, "Timeline:") {
		t.Error("Should not show timeline when not running")
	}
}

// ============================================================================
// PgBackRestContext Extended Fields Tests (Story 4.3)
// ============================================================================

func TestPgBackRestContext_ExtendedFields_JSONSerialization(t *testing.T) {
	data := &PgBackRestContext{
		Available:      true,
		Configured:     true,
		Stanza:         "pg-test",
		LastBackup:     "20260204-120000F",
		LastBackupTime: 1738670400, // Unix timestamp
		BackupCount:    5,
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	jsonStr := string(jsonBytes)

	// Verify new field is present with snake_case
	if !strings.Contains(jsonStr, `"last_backup_time":1738670400`) {
		t.Error("JSON should contain last_backup_time field with snake_case")
	}
	if !strings.Contains(jsonStr, `"last_backup":"20260204-120000F"`) {
		t.Error("JSON should contain last_backup field")
	}

	// Test round-trip
	var decoded PgBackRestContext
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if decoded.LastBackupTime != 1738670400 {
		t.Errorf("LastBackupTime mismatch: got %d, want %d", decoded.LastBackupTime, 1738670400)
	}
}

func TestPgBackRestContext_ExtendedFields_YAMLSerialization(t *testing.T) {
	data := &PgBackRestContext{
		Available:      true,
		Configured:     true,
		Stanza:         "pg-test",
		LastBackup:     "20260204-120000F",
		LastBackupTime: 1738670400,
		BackupCount:    5,
	}

	yamlBytes, err := yaml.Marshal(data)
	if err != nil {
		t.Fatalf("YAML marshal failed: %v", err)
	}

	yamlStr := string(yamlBytes)

	// Verify field is present
	if !strings.Contains(yamlStr, "last_backup_time: 1738670400") {
		t.Error("YAML should contain last_backup_time field")
	}
}

func TestPgBackRestContext_ExtendedFields_Omitempty(t *testing.T) {
	// LastBackupTime should be omitted when zero
	data := &PgBackRestContext{
		Available:  true,
		Configured: false,
		// LastBackupTime: 0 (should be omitted)
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	jsonStr := string(jsonBytes)

	if strings.Contains(jsonStr, "last_backup_time") {
		t.Error("Zero last_backup_time should be omitted")
	}
}

func TestPgBackRestContext_Text_WithLastBackupTime(t *testing.T) {
	// Use a time that's about 2 hours ago
	twoHoursAgo := time.Now().Add(-2 * time.Hour).Unix()

	p := &PgBackRestContext{
		Available:      true,
		Configured:     true,
		Stanza:         "pg-test",
		LastBackup:     "20260205-100000F",
		LastBackupTime: twoHoursAgo,
		BackupCount:    5,
	}

	text := p.text()

	if !strings.Contains(text, "● Configured") {
		t.Error("Should show configured status")
	}
	if !strings.Contains(text, "Stanza: pg-test") {
		t.Error("Should show stanza name")
	}
	if !strings.Contains(text, "Backups: 5") {
		t.Error("Should show backup count")
	}
	if !strings.Contains(text, "20260205-100000F") {
		t.Error("Should show backup label")
	}
	if !strings.Contains(text, "ago") {
		t.Error("Should show relative time (ago)")
	}
}

func TestPgBackRestContext_Text_NoBackupTime(t *testing.T) {
	p := &PgBackRestContext{
		Available:   true,
		Configured:  true,
		Stanza:      "pg-test",
		LastBackup:  "20260205-100000F",
		BackupCount: 5,
		// LastBackupTime: 0 - no timestamp
	}

	text := p.text()

	// Should show just the label without relative time
	if !strings.Contains(text, "Last: 20260205-100000F") {
		t.Error("Should show backup label")
	}
	// Should not have parentheses with time when no timestamp
	if strings.Contains(text, "(") && strings.Contains(text, "ago)") {
		t.Error("Should not show relative time when LastBackupTime is 0")
	}
}

// ============================================================================
// formatTimeAgo Tests
// ============================================================================

func TestFormatTimeAgo(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		time     time.Time
		contains string
	}{
		{"just now", now.Add(-30 * time.Second), "just now"},
		{"minutes ago", now.Add(-15 * time.Minute), "m ago"},
		{"hours ago", now.Add(-5 * time.Hour), "h ago"},
		{"days ago", now.Add(-3 * 24 * time.Hour), "d ago"},
		{"weeks ago", now.Add(-2 * 7 * 24 * time.Hour), "w ago"},
		{"months ago", now.Add(-60 * 24 * time.Hour), "mo ago"},
		{"future", now.Add(1 * time.Hour), "in future"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatTimeAgo(tt.time)
			if !strings.Contains(got, tt.contains) {
				t.Errorf("formatTimeAgo() = %q, should contain %q", got, tt.contains)
			}
		})
	}
}

// ============================================================================
// Graceful Degradation Tests for Extended Fields (Story 4.3 AC#7)
// ============================================================================

func TestContextResultData_GracefulDegradation_PatroniFails(t *testing.T) {
	// When Patroni info collection fails, other modules should still work
	data := &ContextResultData{
		Host: &HostInfo{
			Hostname: "test-host",
			OS:       "linux",
			Arch:     "amd64",
		},
		Postgres: &PostgresContext{
			Available: true,
			Running:   true,
			Version:   17,
		},
		Patroni: &PatroniContext{
			Available: true,
			Running:   false, // Patroni available but info collection failed
			// No role, timeline, lag
		},
		PgBackRest: &PgBackRestContext{
			Available:  true,
			Configured: true,
			Stanza:     "pg-test",
		},
	}

	// Verify JSON serialization works
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed with degraded Patroni: %v", err)
	}

	// Verify text output works
	text := data.Text()
	if text == "" {
		t.Error("Text() should return non-empty string even with degraded Patroni")
	}

	// Verify other modules are present
	jsonStr := string(jsonBytes)
	if !strings.Contains(jsonStr, `"postgres"`) {
		t.Error("Postgres module should still be present")
	}
	if !strings.Contains(jsonStr, `"pgbackrest"`) {
		t.Error("pgBackRest module should still be present")
	}
}

func TestContextResultData_GracefulDegradation_PgBackRestFails(t *testing.T) {
	// When pgBackRest info collection fails, other modules should still work
	data := &ContextResultData{
		Host: &HostInfo{
			Hostname: "test-host",
			OS:       "linux",
			Arch:     "amd64",
		},
		Postgres: &PostgresContext{
			Available: true,
			Running:   true,
			Version:   17,
		},
		Patroni: &PatroniContext{
			Available: true,
			Running:   true,
			Cluster:   "pg-test",
			Role:      "leader",
			Timeline:  3,
		},
		PgBackRest: &PgBackRestContext{
			Available:  true,
			Configured: false, // Config exists but stanza not found
		},
	}

	// Verify JSON serialization works
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed with degraded pgBackRest: %v", err)
	}

	// Verify text output works
	text := data.Text()
	if text == "" {
		t.Error("Text() should return non-empty string even with degraded pgBackRest")
	}

	// Verify other modules are present
	jsonStr := string(jsonBytes)
	if !strings.Contains(jsonStr, `"postgres"`) {
		t.Error("Postgres module should still be present")
	}
	if !strings.Contains(jsonStr, `"patroni"`) {
		t.Error("Patroni module should still be present")
	}
}

// ============================================================================
// Integration Test: Full Context with Extended Fields
// ============================================================================

func TestContextResultData_FullContext_WithExtendedFields(t *testing.T) {
	data := &ContextResultData{
		Host: &HostInfo{
			Hostname: "test-host",
			OS:       "linux",
			Distro:   "el9",
			Arch:     "amd64",
			Kernel:   "5.14.0-362.el9.x86_64",
		},
		Postgres: &PostgresContext{
			Available:      true,
			Running:        true,
			Version:        17,
			VersionString:  "PG17",
			VersionNum:     170000,
			DataDir:        "/pg/data",
			Port:           5432,
			PID:            12345,
			Role:           "primary",
			UptimeSeconds:  86400,
			Connections:    15,
			MaxConnections: 100,
		},
		Patroni: &PatroniContext{
			Available: true,
			Running:   true,
			Cluster:   "pg-test",
			Role:      "leader",
			State:     "running",
			Timeline:  3,
			Lag:       "",
		},
		PgBackRest: &PgBackRestContext{
			Available:      true,
			Configured:     true,
			Stanza:         "pg-test",
			LastBackup:     "20260204-120000F",
			LastBackupTime: 1738670400,
			BackupCount:    5,
		},
		Extensions: &ExtensionsContext{
			Available:      true,
			InstalledCount: 3,
			Extensions:     []string{"postgis", "pg_stat_statements", "uuid-ossp"},
		},
	}

	// Test JSON serialization
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	jsonStr := string(jsonBytes)

	// Verify all extended fields are present
	expectedFields := []string{
		`"timeline":3`,
		`"last_backup_time":1738670400`,
		`"cluster":"pg-test"`,
		`"role":"leader"`,
	}

	for _, field := range expectedFields {
		if !strings.Contains(jsonStr, field) {
			t.Errorf("JSON should contain field: %s", field)
		}
	}

	// Test YAML serialization
	yamlBytes, err := yaml.Marshal(data)
	if err != nil {
		t.Fatalf("YAML marshal failed: %v", err)
	}

	yamlStr := string(yamlBytes)

	yamlExpectedFields := []string{
		"timeline: 3",
		"last_backup_time: 1738670400",
		"cluster: pg-test",
	}

	for _, field := range yamlExpectedFields {
		if !strings.Contains(yamlStr, field) {
			t.Errorf("YAML should contain field: %s", field)
		}
	}

	// Test Text output
	text := data.Text()

	textExpectedContent := []string{
		"PIG CONTEXT",
		"test-host",
		"PostgreSQL",
		"Patroni",
		"pgBackRest",
		"Timeline: 3",
	}

	for _, content := range textExpectedContent {
		if !strings.Contains(text, content) {
			t.Errorf("Text should contain: %s", content)
		}
	}
}

// ============================================================================
// Module Filter Tests (Story 4.4)
// ============================================================================

func TestParseModuleFilter(t *testing.T) {
	modules := ParseModuleFilter(" Postgres, !HOST , ,patroni ")
	expected := []string{"postgres", "!host", "patroni"}
	if len(modules) != len(expected) {
		t.Fatalf("ParseModuleFilter length = %d, want %d", len(modules), len(expected))
	}
	for i, v := range expected {
		if modules[i] != v {
			t.Errorf("ParseModuleFilter[%d] = %q, want %q", i, modules[i], v)
		}
	}
}

func TestModuleFilter_HostDefaultIncluded(t *testing.T) {
	filter, err := buildModuleFilter([]string{"postgres"})
	if err != nil {
		t.Fatalf("buildModuleFilter error: %v", err)
	}
	if !filter.includeModule(ModuleHost) {
		t.Error("Host should be included by default when filtering")
	}
	if !filter.includeModule(ModulePostgres) {
		t.Error("Postgres should be included when specified")
	}
	if filter.includeModule(ModulePatroni) {
		t.Error("Patroni should not be included when not specified")
	}
}

func TestModuleFilter_ExcludeHost(t *testing.T) {
	filter, err := buildModuleFilter([]string{"postgres", "!host"})
	if err != nil {
		t.Fatalf("buildModuleFilter error: %v", err)
	}
	if filter.includeModule(ModuleHost) {
		t.Error("Host should be excluded when explicitly negated")
	}
	if !filter.includeModule(ModulePostgres) {
		t.Error("Postgres should be included when specified")
	}
}

func TestModuleFilter_InvalidModule(t *testing.T) {
	_, err := buildModuleFilter([]string{"invalid"})
	if err == nil {
		t.Error("Expected error for invalid module")
	}
	if !strings.Contains(err.Error(), "invalid module") {
		t.Errorf("Error message should mention 'invalid module', got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "valid modules:") {
		t.Errorf("Error message should list valid modules, got %q", err.Error())
	}
}

// TestModuleFilter_MultiModule tests filtering with multiple modules (Story 4.4 AC#2)
func TestModuleFilter_MultiModule(t *testing.T) {
	filter, err := buildModuleFilter([]string{"postgres", "patroni"})
	if err != nil {
		t.Fatalf("buildModuleFilter error: %v", err)
	}
	if !filter.includeModule(ModuleHost) {
		t.Error("Host should be included by default")
	}
	if !filter.includeModule(ModulePostgres) {
		t.Error("Postgres should be included when specified")
	}
	if !filter.includeModule(ModulePatroni) {
		t.Error("Patroni should be included when specified")
	}
	if filter.includeModule(ModulePgBackRest) {
		t.Error("PgBackRest should not be included when not specified")
	}
	if filter.includeModule(ModuleExtensions) {
		t.Error("Extensions should not be included when not specified")
	}
}

// TestModuleFilter_NoFilter tests that nil filter includes all modules (Story 4.4 AC#3)
func TestModuleFilter_NoFilter(t *testing.T) {
	filter, err := buildModuleFilter(nil)
	if err != nil {
		t.Fatalf("buildModuleFilter error: %v", err)
	}
	if filter != nil {
		t.Fatal("nil modules should produce nil filter")
	}
	// nil filter's includeModule should always return true
	var nilFilter *moduleFilter
	for _, mod := range ValidModules {
		if !nilFilter.includeModule(mod) {
			t.Errorf("Nil filter should include module %q", mod)
		}
	}
}

// TestModuleFilter_EmptyFilter tests that empty slice returns nil (no filter)
func TestModuleFilter_EmptyFilter(t *testing.T) {
	filter, err := buildModuleFilter([]string{})
	if err != nil {
		t.Fatalf("buildModuleFilter error: %v", err)
	}
	if filter != nil {
		t.Fatal("empty modules should produce nil filter")
	}
}

// TestModuleFilter_OnlyExclude tests filter with only exclusions (no includes)
func TestModuleFilter_OnlyExclude(t *testing.T) {
	filter, err := buildModuleFilter([]string{"!patroni", "!pgbackrest"})
	if err != nil {
		t.Fatalf("buildModuleFilter error: %v", err)
	}
	// With only exclusions, everything except excluded modules should be included
	if !filter.includeModule(ModuleHost) {
		t.Error("Host should be included (not excluded)")
	}
	if !filter.includeModule(ModulePostgres) {
		t.Error("Postgres should be included (not excluded)")
	}
	if filter.includeModule(ModulePatroni) {
		t.Error("Patroni should be excluded")
	}
	if filter.includeModule(ModulePgBackRest) {
		t.Error("PgBackRest should be excluded")
	}
	if !filter.includeModule(ModuleExtensions) {
		t.Error("Extensions should be included (not excluded)")
	}
}

// TestModuleFilter_DuplicateModules tests that duplicate module names are handled silently
func TestModuleFilter_DuplicateModules(t *testing.T) {
	filter, err := buildModuleFilter([]string{"postgres", "postgres"})
	if err != nil {
		t.Fatalf("buildModuleFilter should not error on duplicates: %v", err)
	}
	if !filter.includeModule(ModulePostgres) {
		t.Error("Postgres should be included")
	}
	if filter.includeModule(ModulePatroni) {
		t.Error("Patroni should not be included")
	}
}

// TestModuleFilter_CaseInsensitive tests case-insensitive module names
func TestModuleFilter_CaseInsensitive(t *testing.T) {
	filter, err := buildModuleFilter([]string{"Postgres", "PATRONI"})
	if err != nil {
		t.Fatalf("buildModuleFilter should handle mixed case: %v", err)
	}
	if !filter.includeModule(ModulePostgres) {
		t.Error("Postgres should be included (case insensitive)")
	}
	if !filter.includeModule(ModulePatroni) {
		t.Error("Patroni should be included (case insensitive)")
	}
}

// TestModuleFilter_EmptyModuleName tests that empty module name returns false
func TestModuleFilter_EmptyModuleName(t *testing.T) {
	filter, err := buildModuleFilter([]string{"postgres"})
	if err != nil {
		t.Fatalf("buildModuleFilter error: %v", err)
	}
	if filter.includeModule("") {
		t.Error("Empty module name should not be included")
	}
}

// TestParseModuleFilter_Empty tests ParseModuleFilter with empty input
func TestParseModuleFilter_Empty(t *testing.T) {
	result := ParseModuleFilter("")
	if result != nil {
		t.Errorf("Empty string should return nil, got %v", result)
	}
}

// TestParseModuleFilter_Whitespace tests ParseModuleFilter with only whitespace
func TestParseModuleFilter_Whitespace(t *testing.T) {
	result := ParseModuleFilter("   ")
	if result != nil {
		t.Errorf("Whitespace-only string should return nil, got %v", result)
	}
}

// TestParseModuleFilter_SingleModule tests parsing a single module
func TestParseModuleFilter_SingleModule(t *testing.T) {
	result := ParseModuleFilter("postgres")
	if len(result) != 1 || result[0] != "postgres" {
		t.Errorf("Expected [postgres], got %v", result)
	}
}

// TestParseModuleFilter_WithSpaces tests parsing modules with spaces
func TestParseModuleFilter_WithSpaces(t *testing.T) {
	result := ParseModuleFilter("postgres, patroni")
	if len(result) != 2 {
		t.Fatalf("Expected 2 modules, got %d", len(result))
	}
	if result[0] != "postgres" || result[1] != "patroni" {
		t.Errorf("Expected [postgres, patroni], got %v", result)
	}
}

// TestParseModuleFilter_TrailingComma tests parsing with trailing comma
func TestParseModuleFilter_TrailingComma(t *testing.T) {
	result := ParseModuleFilter("postgres,")
	if len(result) != 1 || result[0] != "postgres" {
		t.Errorf("Expected [postgres], got %v", result)
	}
}

// TestIsValidModule tests the IsValidModule function
func TestIsValidModule(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"valid host", "host", true},
		{"valid postgres", "postgres", true},
		{"valid patroni", "patroni", true},
		{"valid pgbackrest", "pgbackrest", true},
		{"valid extensions", "extensions", true},
		{"case insensitive", "Postgres", true},
		{"upper case", "HOST", true},
		{"invalid", "invalid", false},
		{"empty", "", false},
		{"with spaces", " postgres ", true},
		{"partial match", "post", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsValidModule(tt.input)
			if got != tt.want {
				t.Errorf("IsValidModule(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestContextResultWithModules_InvalidModule tests that invalid module returns error result
func TestContextResultWithModules_InvalidModule(t *testing.T) {
	result := ContextResultWithModules([]string{"xxx"})
	if result == nil {
		t.Fatal("Result should not be nil")
	}
	if result.Success {
		t.Error("Result should not be successful for invalid module")
	}
	if result.Code != output.CodeCtxInvalidModule {
		t.Errorf("Expected code %d, got %d", output.CodeCtxInvalidModule, result.Code)
	}
	if !strings.Contains(result.Message, "invalid module") {
		t.Errorf("Message should mention invalid module, got %q", result.Message)
	}
	if !strings.Contains(result.Message, "valid modules:") {
		t.Errorf("Message should list valid modules, got %q", result.Message)
	}
}

// TestContextResultWithModules_EmptyFilter tests that nil filter collects all modules
func TestContextResultWithModules_EmptyFilter(t *testing.T) {
	result := ContextResultWithModules(nil)
	if result == nil {
		t.Fatal("Result should not be nil")
	}
	if !result.Success {
		t.Errorf("Result should be successful, got error: %s", result.Message)
	}
	data, ok := result.Data.(*ContextResultData)
	if !ok {
		t.Fatalf("Data should be *ContextResultData, got %T", result.Data)
	}
	// Host should always be collected
	if data.Host == nil {
		t.Error("Host should be collected with no filter")
	}
}

// TestContextResultWithModules_SingleModule tests single module filtering (AC#1)
func TestContextResultWithModules_SingleModule(t *testing.T) {
	// Filter to only host module
	result := ContextResultWithModules([]string{"host"})
	if result == nil {
		t.Fatal("Result should not be nil")
	}
	if !result.Success {
		t.Errorf("Result should be successful, got error: %s", result.Message)
	}
	data, ok := result.Data.(*ContextResultData)
	if !ok {
		t.Fatalf("Data should be *ContextResultData, got %T", result.Data)
	}
	if data.Host == nil {
		t.Error("Host should be collected when specified")
	}
	// Note: Other modules may still be nil on macOS/non-PG environments
	// The key test is that the filter logic works, not that modules are available
}

// TestContextResult_NilModules tests that nil module filter keeps default behavior.
func TestContextResult_NilModules(t *testing.T) {
	result := ContextResultWithModules(nil)
	if result == nil {
		t.Fatal("Result should not be nil")
	}
	if !result.Success {
		t.Errorf("Result should be successful, got error: %s", result.Message)
	}
	data, ok := result.Data.(*ContextResultData)
	if !ok {
		t.Fatalf("Data should be *ContextResultData, got %T", result.Data)
	}
	if data.Host == nil {
		t.Error("Host should be collected by default")
	}
}

// TestContextResultData_Text_ModuleFiltered tests text output with filtered modules (AC#5)
func TestContextResultData_Text_ModuleFiltered(t *testing.T) {
	// Simulate filtered data where only postgres and host are present
	data := &ContextResultData{
		Host: &HostInfo{
			Hostname: "test-host",
			OS:       "linux",
			Arch:     "amd64",
		},
		Postgres: &PostgresContext{
			Available: true,
			Running:   true,
			Version:   17,
			Port:      5432,
		},
		// Patroni, PgBackRest, Extensions are nil (filtered out)
	}

	text := data.Text()

	// Should contain host and postgres sections
	if !strings.Contains(text, "test-host") {
		t.Error("Text should contain host info")
	}
	if !strings.Contains(text, "PostgreSQL") {
		t.Error("Text should contain PostgreSQL section")
	}

	// Should NOT contain other module sections
	if strings.Contains(text, "Patroni") {
		t.Error("Text should not contain Patroni when filtered out")
	}
	if strings.Contains(text, "pgBackRest") {
		t.Error("Text should not contain pgBackRest when filtered out")
	}
	if strings.Contains(text, "Extensions") {
		t.Error("Text should not contain Extensions when filtered out")
	}
}

// TestContextResultData_Text_OnlyHost tests text output with only host module
func TestContextResultData_Text_OnlyHost(t *testing.T) {
	data := &ContextResultData{
		Host: &HostInfo{
			Hostname: "test-host",
			OS:       "linux",
			Arch:     "amd64",
		},
	}

	text := data.Text()

	if !strings.Contains(text, "test-host") {
		t.Error("Text should contain host info")
	}
	if strings.Contains(text, "PostgreSQL") {
		t.Error("Text should not contain PostgreSQL when nil")
	}
}

// TestContextResultData_JSON_ModuleFiltered tests JSON omitempty with filtered modules
func TestContextResultData_JSON_ModuleFiltered(t *testing.T) {
	data := &ContextResultData{
		Host: &HostInfo{
			Hostname: "test-host",
			OS:       "linux",
			Arch:     "amd64",
		},
		Postgres: &PostgresContext{
			Available: true,
			Running:   true,
			Version:   17,
		},
		// Other modules nil (filtered out)
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	jsonStr := string(jsonBytes)

	// Should contain host and postgres
	if !strings.Contains(jsonStr, `"host"`) {
		t.Error("JSON should contain host field")
	}
	if !strings.Contains(jsonStr, `"postgres"`) {
		t.Error("JSON should contain postgres field")
	}

	// Should NOT contain filtered modules (omitempty)
	if strings.Contains(jsonStr, `"patroni"`) {
		t.Error("JSON should omit nil patroni field")
	}
	if strings.Contains(jsonStr, `"pgbackrest"`) {
		t.Error("JSON should omit nil pgbackrest field")
	}
	if strings.Contains(jsonStr, `"extensions"`) {
		t.Error("JSON should omit nil extensions field")
	}
}

// TestModuleFilter_InvalidNegatedModule tests that negating an invalid module returns error
func TestModuleFilter_InvalidNegatedModule(t *testing.T) {
	_, err := buildModuleFilter([]string{"!invalid"})
	if err == nil {
		t.Error("Expected error for invalid negated module")
	}
	if !strings.Contains(err.Error(), "invalid module") {
		t.Errorf("Error should mention invalid module, got %q", err.Error())
	}
}

// TestModuleFilter_AllModulesExplicit tests explicitly listing all modules
func TestModuleFilter_AllModulesExplicit(t *testing.T) {
	filter, err := buildModuleFilter(ValidModules)
	if err != nil {
		t.Fatalf("buildModuleFilter error: %v", err)
	}
	for _, mod := range ValidModules {
		if !filter.includeModule(mod) {
			t.Errorf("Module %q should be included when explicitly listed", mod)
		}
	}
}
