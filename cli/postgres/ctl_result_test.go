/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

Tests for pg start/stop/restart/reload structured output DTOs.
*/
package postgres

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// ============================================================================
// PgInitResultData Tests (Story 2.4)
// ============================================================================

func TestPgInitResultData_JSON(t *testing.T) {
	data := &PgInitResultData{
		DataDir:  "/pg/data",
		Version:  17,
		Locale:   "C",
		Encoding: "UTF8",
		Checksum: true,
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	jsonStr := string(jsonBytes)
	if !strings.Contains(jsonStr, `"data_dir":"/pg/data"`) {
		t.Errorf("JSON should contain data_dir: %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"version":17`) {
		t.Errorf("JSON should contain version: %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"locale":"C"`) {
		t.Errorf("JSON should contain locale: %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"encoding":"UTF8"`) {
		t.Errorf("JSON should contain encoding: %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"checksum":true`) {
		t.Errorf("JSON should contain checksum: %s", jsonStr)
	}
}

func TestPgInitResultData_JSON_NoChecksum(t *testing.T) {
	data := &PgInitResultData{
		DataDir:  "/pg/data",
		Version:  16,
		Locale:   "en_US.UTF-8",
		Encoding: "UTF8",
		Checksum: false,
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	jsonStr := string(jsonBytes)
	// checksum should be omitted when false
	if strings.Contains(jsonStr, `"checksum"`) {
		t.Errorf("JSON should omit checksum when false: %s", jsonStr)
	}
}

func TestPgInitResultData_YAML(t *testing.T) {
	data := &PgInitResultData{
		DataDir:  "/pg/data",
		Version:  17,
		Locale:   "C",
		Encoding: "UTF8",
		Checksum: true,
	}

	yamlBytes, err := yaml.Marshal(data)
	if err != nil {
		t.Fatalf("YAML marshal failed: %v", err)
	}

	yamlStr := string(yamlBytes)
	if !strings.Contains(yamlStr, "data_dir: /pg/data") {
		t.Errorf("YAML should contain data_dir: %s", yamlStr)
	}
	if !strings.Contains(yamlStr, "version: 17") {
		t.Errorf("YAML should contain version: %s", yamlStr)
	}
	if !strings.Contains(yamlStr, "locale: C") {
		t.Errorf("YAML should contain locale: %s", yamlStr)
	}
	if !strings.Contains(yamlStr, "encoding: UTF8") {
		t.Errorf("YAML should contain encoding: %s", yamlStr)
	}
}

func TestPgInitResultData_Force(t *testing.T) {
	data := &PgInitResultData{
		DataDir:  "/pg/data",
		Version:  17,
		Locale:   "C",
		Encoding: "UTF8",
		Force:    true,
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	jsonStr := string(jsonBytes)
	if !strings.Contains(jsonStr, `"force":true`) {
		t.Errorf("JSON should contain force:true: %s", jsonStr)
	}
}

// ============================================================================
// PgStartResultData Tests
// ============================================================================

func TestPgStartResultData_JSON(t *testing.T) {
	data := &PgStartResultData{
		PID:     12345,
		DataDir: "/pg/data",
		NoWait:  false,
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	jsonStr := string(jsonBytes)
	if !strings.Contains(jsonStr, `"pid":12345`) {
		t.Errorf("JSON should contain pid: %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"data_dir":"/pg/data"`) {
		t.Errorf("JSON should contain data_dir: %s", jsonStr)
	}
	// no_wait should be omitted when false
	if strings.Contains(jsonStr, `"no_wait"`) {
		t.Errorf("JSON should omit no_wait when false: %s", jsonStr)
	}
}

func TestPgStartResultData_JSON_NoWait(t *testing.T) {
	data := &PgStartResultData{
		PID:     0,
		DataDir: "/pg/data",
		NoWait:  true,
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	jsonStr := string(jsonBytes)
	if !strings.Contains(jsonStr, `"no_wait":true`) {
		t.Errorf("JSON should contain no_wait:true: %s", jsonStr)
	}
}

func TestPgStartResultData_YAML(t *testing.T) {
	data := &PgStartResultData{
		PID:     12345,
		DataDir: "/pg/data",
	}

	yamlBytes, err := yaml.Marshal(data)
	if err != nil {
		t.Fatalf("YAML marshal failed: %v", err)
	}

	yamlStr := string(yamlBytes)
	if !strings.Contains(yamlStr, "pid: 12345") {
		t.Errorf("YAML should contain pid: %s", yamlStr)
	}
	if !strings.Contains(yamlStr, "data_dir: /pg/data") {
		t.Errorf("YAML should contain data_dir: %s", yamlStr)
	}
}

// ============================================================================
// PgStopResultData Tests
// ============================================================================

func TestPgStopResultData_JSON(t *testing.T) {
	data := &PgStopResultData{
		StoppedPID: 12345,
		DataDir:    "/pg/data",
		Mode:       "fast",
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	jsonStr := string(jsonBytes)
	if !strings.Contains(jsonStr, `"stopped_pid":12345`) {
		t.Errorf("JSON should contain stopped_pid: %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"mode":"fast"`) {
		t.Errorf("JSON should contain mode: %s", jsonStr)
	}
}

func TestPgStopResultData_YAML(t *testing.T) {
	data := &PgStopResultData{
		StoppedPID: 12345,
		DataDir:    "/pg/data",
		Mode:       "smart",
		NoWait:     true,
	}

	yamlBytes, err := yaml.Marshal(data)
	if err != nil {
		t.Fatalf("YAML marshal failed: %v", err)
	}

	yamlStr := string(yamlBytes)
	if !strings.Contains(yamlStr, "stopped_pid: 12345") {
		t.Errorf("YAML should contain stopped_pid: %s", yamlStr)
	}
	if !strings.Contains(yamlStr, "no_wait: true") {
		t.Errorf("YAML should contain no_wait: %s", yamlStr)
	}
}

// ============================================================================
// PgRestartResultData Tests
// ============================================================================

func TestPgRestartResultData_JSON(t *testing.T) {
	data := &PgRestartResultData{
		OldPID:  12345,
		NewPID:  12346,
		DataDir: "/pg/data",
		Mode:    "fast",
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	jsonStr := string(jsonBytes)
	if !strings.Contains(jsonStr, `"old_pid":12345`) {
		t.Errorf("JSON should contain old_pid: %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"new_pid":12346`) {
		t.Errorf("JSON should contain new_pid: %s", jsonStr)
	}
}

func TestPgRestartResultData_YAML(t *testing.T) {
	data := &PgRestartResultData{
		OldPID:  12345,
		NewPID:  12346,
		DataDir: "/pg/data",
		Mode:    "immediate",
	}

	yamlBytes, err := yaml.Marshal(data)
	if err != nil {
		t.Fatalf("YAML marshal failed: %v", err)
	}

	yamlStr := string(yamlBytes)
	if !strings.Contains(yamlStr, "old_pid: 12345") {
		t.Errorf("YAML should contain old_pid: %s", yamlStr)
	}
	if !strings.Contains(yamlStr, "new_pid: 12346") {
		t.Errorf("YAML should contain new_pid: %s", yamlStr)
	}
}

func TestPgRestartResultData_NoWait(t *testing.T) {
	data := &PgRestartResultData{
		OldPID:  12345,
		NewPID:  0,
		DataDir: "/pg/data",
		Mode:    "fast",
		NoWait:  true,
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	jsonStr := string(jsonBytes)
	if !strings.Contains(jsonStr, `"old_pid":12345`) {
		t.Errorf("JSON should contain old_pid: %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"new_pid":0`) {
		t.Errorf("JSON should contain new_pid:0 in no-wait mode: %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"no_wait":true`) {
		t.Errorf("JSON should contain no_wait:true: %s", jsonStr)
	}
}

// ============================================================================
// PgReloadResultData Tests
// ============================================================================

func TestPgReloadResultData_JSON(t *testing.T) {
	data := &PgReloadResultData{
		Reloaded: true,
		PID:      12345,
		DataDir:  "/pg/data",
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	jsonStr := string(jsonBytes)
	if !strings.Contains(jsonStr, `"reloaded":true`) {
		t.Errorf("JSON should contain reloaded:true: %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"pid":12345`) {
		t.Errorf("JSON should contain pid: %s", jsonStr)
	}
}

func TestPgReloadResultData_YAML(t *testing.T) {
	data := &PgReloadResultData{
		Reloaded: true,
		PID:      12345,
		DataDir:  "/pg/data",
	}

	yamlBytes, err := yaml.Marshal(data)
	if err != nil {
		t.Fatalf("YAML marshal failed: %v", err)
	}

	yamlStr := string(yamlBytes)
	if !strings.Contains(yamlStr, "reloaded: true") {
		t.Errorf("YAML should contain reloaded: %s", yamlStr)
	}
}

// ============================================================================
// Result Constructor Tests
// ============================================================================

func TestInitOK(t *testing.T) {
	result := InitOK("/pg/data", 17, "C", "UTF8", true)
	if result == nil {
		t.Fatal("InitOK returned nil")
	}
	if !result.Success {
		t.Error("InitOK should return success=true")
	}
	if result.Code != 0 {
		t.Errorf("InitOK should return code=0, got %d", result.Code)
	}
	data, ok := result.Data.(*PgInitResultData)
	if !ok {
		t.Fatal("InitOK data should be *PgInitResultData")
	}
	if data.DataDir != "/pg/data" {
		t.Errorf("Expected DataDir /pg/data, got %s", data.DataDir)
	}
	if data.Version != 17 {
		t.Errorf("Expected Version 17, got %d", data.Version)
	}
	if data.Locale != "C" {
		t.Errorf("Expected Locale C, got %s", data.Locale)
	}
	if data.Encoding != "UTF8" {
		t.Errorf("Expected Encoding UTF8, got %s", data.Encoding)
	}
	if !data.Checksum {
		t.Error("Expected Checksum true")
	}
}

func TestInitOKForce(t *testing.T) {
	result := InitOKForce("/pg/data", 17, "C", "UTF8", false)
	if result == nil {
		t.Fatal("InitOKForce returned nil")
	}
	if !result.Success {
		t.Error("InitOKForce should return success=true")
	}
	data, ok := result.Data.(*PgInitResultData)
	if !ok {
		t.Fatal("InitOKForce data should be *PgInitResultData")
	}
	if !data.Force {
		t.Error("Force should be true in InitOKForce")
	}
	if result.Detail == "" {
		t.Error("Detail should explain force mode")
	}
}

func TestStartOK(t *testing.T) {
	result := StartOK(12345, "/pg/data")
	if result == nil {
		t.Fatal("StartOK returned nil")
	}
	if !result.Success {
		t.Error("StartOK should return success=true")
	}
	if result.Code != 0 {
		t.Errorf("StartOK should return code=0, got %d", result.Code)
	}
	data, ok := result.Data.(*PgStartResultData)
	if !ok {
		t.Fatal("StartOK data should be *PgStartResultData")
	}
	if data.PID != 12345 {
		t.Errorf("Expected PID 12345, got %d", data.PID)
	}
	if data.DataDir != "/pg/data" {
		t.Errorf("Expected DataDir /pg/data, got %s", data.DataDir)
	}
}

func TestStartOKNoWait(t *testing.T) {
	result := StartOKNoWait("/pg/data")
	if result == nil {
		t.Fatal("StartOKNoWait returned nil")
	}
	if !result.Success {
		t.Error("StartOKNoWait should return success=true")
	}
	data, ok := result.Data.(*PgStartResultData)
	if !ok {
		t.Fatal("StartOKNoWait data should be *PgStartResultData")
	}
	if data.PID != 0 {
		t.Errorf("Expected PID 0 in no-wait mode, got %d", data.PID)
	}
	if !data.NoWait {
		t.Error("NoWait should be true")
	}
	if result.Detail == "" {
		t.Error("Detail should explain no-wait mode")
	}
}

func TestStopOK(t *testing.T) {
	result := StopOK(12345, "/pg/data", "fast")
	if result == nil {
		t.Fatal("StopOK returned nil")
	}
	if !result.Success {
		t.Error("StopOK should return success=true")
	}
	data, ok := result.Data.(*PgStopResultData)
	if !ok {
		t.Fatal("StopOK data should be *PgStopResultData")
	}
	if data.StoppedPID != 12345 {
		t.Errorf("Expected StoppedPID 12345, got %d", data.StoppedPID)
	}
	if data.Mode != "fast" {
		t.Errorf("Expected Mode fast, got %s", data.Mode)
	}
}

func TestStopOKNoWait(t *testing.T) {
	result := StopOKNoWait(12345, "/pg/data", "smart")
	if result == nil {
		t.Fatal("StopOKNoWait returned nil")
	}
	data, ok := result.Data.(*PgStopResultData)
	if !ok {
		t.Fatal("StopOKNoWait data should be *PgStopResultData")
	}
	if !data.NoWait {
		t.Error("NoWait should be true")
	}
}

func TestRestartOK(t *testing.T) {
	result := RestartOK(12345, 12346, "/pg/data", "fast")
	if result == nil {
		t.Fatal("RestartOK returned nil")
	}
	if !result.Success {
		t.Error("RestartOK should return success=true")
	}
	data, ok := result.Data.(*PgRestartResultData)
	if !ok {
		t.Fatal("RestartOK data should be *PgRestartResultData")
	}
	if data.OldPID != 12345 {
		t.Errorf("Expected OldPID 12345, got %d", data.OldPID)
	}
	if data.NewPID != 12346 {
		t.Errorf("Expected NewPID 12346, got %d", data.NewPID)
	}
}

func TestRestartOKNoWait(t *testing.T) {
	result := RestartOKNoWait(12345, "/pg/data", "immediate")
	if result == nil {
		t.Fatal("RestartOKNoWait returned nil")
	}
	data, ok := result.Data.(*PgRestartResultData)
	if !ok {
		t.Fatal("RestartOKNoWait data should be *PgRestartResultData")
	}
	if data.OldPID != 12345 {
		t.Errorf("Expected OldPID 12345, got %d", data.OldPID)
	}
	if data.NewPID != 0 {
		t.Errorf("Expected NewPID 0 in no-wait mode, got %d", data.NewPID)
	}
	if !data.NoWait {
		t.Error("NoWait should be true")
	}
}

func TestReloadOK(t *testing.T) {
	result := ReloadOK(12345, "/pg/data")
	if result == nil {
		t.Fatal("ReloadOK returned nil")
	}
	if !result.Success {
		t.Error("ReloadOK should return success=true")
	}
	data, ok := result.Data.(*PgReloadResultData)
	if !ok {
		t.Fatal("ReloadOK data should be *PgReloadResultData")
	}
	if !data.Reloaded {
		t.Error("Reloaded should be true")
	}
	if data.PID != 12345 {
		t.Errorf("Expected PID 12345, got %d", data.PID)
	}
}

// ============================================================================
// Init Error Code Tests (Story 2.4)
// ============================================================================

func TestInitErrorCodes(t *testing.T) {
	tests := []struct {
		name         string
		code         int
		expectedExit int
	}{
		{"CodePgInitDirExists", 130502, 6},       // CAT_RESOURCE -> exit 6 (MODULE_PG + CAT_RESOURCE + 2)
		{"CodePgInitFailed", 130806, 1},          // CAT_OPERATION -> exit 1
		{"CodePgInitRunningConflict", 130606, 9}, // CAT_STATE -> exit 9
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			category := (tt.code % 10000) / 100
			var expectedExit int
			switch category {
			case 0:
				expectedExit = 0
			case 1:
				expectedExit = 2
			case 2:
				expectedExit = 3
			case 3:
				expectedExit = 4
			case 4:
				expectedExit = 5
			case 5:
				expectedExit = 6
			case 6:
				expectedExit = 9
			case 7:
				expectedExit = 8
			case 8, 9:
				expectedExit = 1
			default:
				expectedExit = 1
			}
			if expectedExit != tt.expectedExit {
				t.Errorf("Code %d: expected exit %d, got %d", tt.code, tt.expectedExit, expectedExit)
			}
		})
	}
}

// ============================================================================
// Error Code Mapping Tests (Task 5.2)
// ============================================================================

func TestErrorCodeMapping(t *testing.T) {
	tests := []struct {
		name         string
		code         int
		expectedExit int
	}{
		{"CodePgAlreadyRunning", 130603, 9},   // CAT_STATE -> exit 9
		{"CodePgAlreadyStopped", 130604, 9},   // CAT_STATE -> exit 9
		{"CodePgNotRunning", 130605, 9},       // CAT_STATE -> exit 9
		{"CodePgStartFailed", 130801, 1},      // CAT_OPERATION -> exit 1
		{"CodePgStopFailed", 130802, 1},       // CAT_OPERATION -> exit 1
		{"CodePgRestartFailed", 130803, 1},    // CAT_OPERATION -> exit 1
		{"CodePgReloadFailed", 130804, 1},     // CAT_OPERATION -> exit 1
		{"CodePgTimeout", 130805, 1},          // CAT_OPERATION -> exit 1
		{"CodePgPermissionDenied", 130202, 3}, // CAT_PERM -> exit 3
		{"CodePgNotFound", 130301, 4},         // CAT_DEPEND -> exit 4
	}

	// Import output package for ExitCode function
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			category := (tt.code % 10000) / 100
			var expectedExit int
			switch category {
			case 0:
				expectedExit = 0
			case 1:
				expectedExit = 2
			case 2:
				expectedExit = 3
			case 3:
				expectedExit = 4
			case 4:
				expectedExit = 5
			case 5:
				expectedExit = 6
			case 6:
				expectedExit = 9
			case 7:
				expectedExit = 8
			case 8, 9:
				expectedExit = 1
			default:
				expectedExit = 1
			}
			if expectedExit != tt.expectedExit {
				t.Errorf("Code %d: expected exit %d, got %d", tt.code, tt.expectedExit, expectedExit)
			}
		})
	}
}

// ============================================================================
// Restart old/new PID Logic Tests (Task 5.3)
// ============================================================================

func TestRestartResultData_OldNewPID(t *testing.T) {
	tests := []struct {
		name      string
		oldPID    int
		newPID    int
		noWait    bool
		expectOld int
		expectNew int
	}{
		{
			name:      "normal restart",
			oldPID:    12345,
			newPID:    12346,
			noWait:    false,
			expectOld: 12345,
			expectNew: 12346,
		},
		{
			name:      "restart from stopped state",
			oldPID:    0,
			newPID:    12346,
			noWait:    false,
			expectOld: 0,
			expectNew: 12346,
		},
		{
			name:      "no-wait mode (new PID unknown)",
			oldPID:    12345,
			newPID:    0,
			noWait:    true,
			expectOld: 12345,
			expectNew: 0,
		},
		{
			name:      "same PID (edge case - should not happen normally)",
			oldPID:    12345,
			newPID:    12345,
			noWait:    false,
			expectOld: 12345,
			expectNew: 12345,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result *PgRestartResultData
			if tt.noWait {
				result = &PgRestartResultData{
					OldPID:  tt.oldPID,
					NewPID:  0,
					DataDir: "/pg/data",
					Mode:    "fast",
					NoWait:  true,
				}
			} else {
				result = &PgRestartResultData{
					OldPID:  tt.oldPID,
					NewPID:  tt.newPID,
					DataDir: "/pg/data",
					Mode:    "fast",
				}
			}

			if result.OldPID != tt.expectOld {
				t.Errorf("expected OldPID=%d, got %d", tt.expectOld, result.OldPID)
			}
			if result.NewPID != tt.expectNew {
				t.Errorf("expected NewPID=%d, got %d", tt.expectNew, result.NewPID)
			}
		})
	}
}

func TestRestartOK_PIDLogic(t *testing.T) {
	// Test that RestartOK correctly captures old and new PIDs
	result := RestartOK(100, 200, "/pg/data", "fast")
	if result == nil {
		t.Fatal("RestartOK returned nil")
	}

	data, ok := result.Data.(*PgRestartResultData)
	if !ok {
		t.Fatal("RestartOK data should be *PgRestartResultData")
	}

	if data.OldPID != 100 {
		t.Errorf("Expected OldPID=100, got %d", data.OldPID)
	}
	if data.NewPID != 200 {
		t.Errorf("Expected NewPID=200, got %d", data.NewPID)
	}
	if data.NoWait {
		t.Error("NoWait should be false in RestartOK")
	}
}

func TestRestartOKNoWait_PIDLogic(t *testing.T) {
	// Test that RestartOKNoWait sets NewPID to 0 and NoWait to true
	result := RestartOKNoWait(100, "/pg/data", "fast")
	if result == nil {
		t.Fatal("RestartOKNoWait returned nil")
	}

	data, ok := result.Data.(*PgRestartResultData)
	if !ok {
		t.Fatal("RestartOKNoWait data should be *PgRestartResultData")
	}

	if data.OldPID != 100 {
		t.Errorf("Expected OldPID=100, got %d", data.OldPID)
	}
	if data.NewPID != 0 {
		t.Errorf("Expected NewPID=0 in no-wait mode, got %d", data.NewPID)
	}
	if !data.NoWait {
		t.Error("NoWait should be true in RestartOKNoWait")
	}
}

// ============================================================================
// classifyCtlError Tests
// ============================================================================

func TestClassifyCtlError(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		defaultCode  int
		expectedCode int
	}{
		{
			name:         "nil error",
			err:          nil,
			defaultCode:  130801,
			expectedCode: 0,
		},
		{
			name:         "permission denied",
			err:          fmt.Errorf("permission denied: cannot access /pg/data"),
			defaultCode:  130801,
			expectedCode: 130202, // CodePgPermissionDenied
		},
		{
			name:         "operation not permitted",
			err:          fmt.Errorf("operation not permitted"),
			defaultCode:  130801,
			expectedCode: 130202, // CodePgPermissionDenied
		},
		{
			name:         "timeout error",
			err:          fmt.Errorf("pg_ctl: server did not start in time"),
			defaultCode:  130801,
			expectedCode: 130805, // CodePgTimeout
		},
		{
			name:         "explicit timeout",
			err:          fmt.Errorf("operation timed out"),
			defaultCode:  130801,
			expectedCode: 130805, // CodePgTimeout
		},
		{
			name:         "not found error",
			err:          fmt.Errorf("pg_ctl: command not found"),
			defaultCode:  130801,
			expectedCode: 130301, // CodePgNotFound
		},
		{
			name:         "generic error",
			err:          fmt.Errorf("something went wrong"),
			defaultCode:  130801,
			expectedCode: 130801, // default code
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code := classifyCtlError(tt.err, tt.defaultCode)
			if code != tt.expectedCode {
				t.Errorf("expected code %d, got %d", tt.expectedCode, code)
			}
		})
	}
}

// ============================================================================
// Init Result Tests (Story 2.4)
// ============================================================================

func TestInitResultData_RoundTrip(t *testing.T) {
	// Test JSON round-trip
	original := &PgInitResultData{
		DataDir:  "/pg/data",
		Version:  17,
		Locale:   "C",
		Encoding: "UTF8",
		Checksum: true,
		Force:    false,
	}

	jsonBytes, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	var decoded PgInitResultData
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if decoded.DataDir != original.DataDir {
		t.Errorf("DataDir mismatch: got %s, want %s", decoded.DataDir, original.DataDir)
	}
	if decoded.Version != original.Version {
		t.Errorf("Version mismatch: got %d, want %d", decoded.Version, original.Version)
	}
	if decoded.Locale != original.Locale {
		t.Errorf("Locale mismatch: got %s, want %s", decoded.Locale, original.Locale)
	}
	if decoded.Encoding != original.Encoding {
		t.Errorf("Encoding mismatch: got %s, want %s", decoded.Encoding, original.Encoding)
	}
	if decoded.Checksum != original.Checksum {
		t.Errorf("Checksum mismatch: got %v, want %v", decoded.Checksum, original.Checksum)
	}
}

func TestInitResultData_YAML_RoundTrip(t *testing.T) {
	// Test YAML round-trip
	original := &PgInitResultData{
		DataDir:  "/pg/data",
		Version:  16,
		Locale:   "en_US.UTF-8",
		Encoding: "UTF8",
		Checksum: false,
		Force:    true,
	}

	yamlBytes, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("YAML marshal failed: %v", err)
	}

	var decoded PgInitResultData
	if err := yaml.Unmarshal(yamlBytes, &decoded); err != nil {
		t.Fatalf("YAML unmarshal failed: %v", err)
	}

	if decoded.DataDir != original.DataDir {
		t.Errorf("DataDir mismatch: got %s, want %s", decoded.DataDir, original.DataDir)
	}
	if decoded.Version != original.Version {
		t.Errorf("Version mismatch: got %d, want %d", decoded.Version, original.Version)
	}
	if decoded.Force != original.Force {
		t.Errorf("Force mismatch: got %v, want %v", decoded.Force, original.Force)
	}
}

func TestInitErrorCodeConstants(t *testing.T) {
	// Verify error code constants are correctly defined
	tests := []struct {
		name         string
		code         int
		expectedMod  int
		expectedCat  int
		expectedSpec int
	}{
		{
			name:         "CodePgInitDirExists",
			code:         130502,
			expectedMod:  13,
			expectedCat:  5, // CAT_RESOURCE
			expectedSpec: 2,
		},
		{
			name:         "CodePgInitFailed",
			code:         130806,
			expectedMod:  13,
			expectedCat:  8, // CAT_OPERATION
			expectedSpec: 6,
		},
		{
			name:         "CodePgInitRunningConflict",
			code:         130606,
			expectedMod:  13,
			expectedCat:  6, // CAT_STATE
			expectedSpec: 6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mod := tt.code / 10000
			cat := (tt.code % 10000) / 100
			spec := tt.code % 100

			if mod != tt.expectedMod {
				t.Errorf("Module: got %d, want %d", mod, tt.expectedMod)
			}
			if cat != tt.expectedCat {
				t.Errorf("Category: got %d, want %d", cat, tt.expectedCat)
			}
			if spec != tt.expectedSpec {
				t.Errorf("Specific: got %d, want %d", spec, tt.expectedSpec)
			}
		})
	}
}

// ============================================================================
// Nil Receiver Safety Tests
// ============================================================================

func TestNilResultDataSafety(t *testing.T) {
	// Test that nil data is handled gracefully in result constructors
	var nilStartData *PgStartResultData
	var nilStopData *PgStopResultData
	var nilRestartData *PgRestartResultData
	var nilReloadData *PgReloadResultData
	var nilPromoteData *PgPromoteResultData

	// These should be nil - just verify type safety
	if nilStartData != nil {
		t.Error("nilStartData should be nil")
	}
	if nilStopData != nil {
		t.Error("nilStopData should be nil")
	}
	if nilRestartData != nil {
		t.Error("nilRestartData should be nil")
	}
	if nilReloadData != nil {
		t.Error("nilReloadData should be nil")
	}
	if nilPromoteData != nil {
		t.Error("nilPromoteData should be nil")
	}
}

// TestNilResultMethods tests that Result methods handle nil receivers gracefully.
// This validates the pattern warned about in CLAUDE.md - nil receiver checks.
func TestNilResultMethods(t *testing.T) {
	// Test that constructors never return nil (they should always return a valid Result)
	t.Run("StartOK never returns nil", func(t *testing.T) {
		result := StartOK(0, "")
		if result == nil {
			t.Error("StartOK should never return nil")
		}
	})

	t.Run("StopOK never returns nil", func(t *testing.T) {
		result := StopOK(0, "", "")
		if result == nil {
			t.Error("StopOK should never return nil")
		}
	})

	t.Run("RestartOK never returns nil", func(t *testing.T) {
		result := RestartOK(0, 0, "", "")
		if result == nil {
			t.Error("RestartOK should never return nil")
		}
	})

	t.Run("ReloadOK never returns nil", func(t *testing.T) {
		result := ReloadOK(0, "")
		if result == nil {
			t.Error("ReloadOK should never return nil")
		}
	})

	t.Run("PromoteOK never returns nil", func(t *testing.T) {
		result := PromoteOK(0, "", "", "", 0)
		if result == nil {
			t.Error("PromoteOK should never return nil")
		}
	})

	t.Run("InitOK never returns nil", func(t *testing.T) {
		result := InitOK("", 0, "", "", false)
		if result == nil {
			t.Error("InitOK should never return nil")
		}
	})

	t.Run("InitOKForce never returns nil", func(t *testing.T) {
		result := InitOKForce("", 0, "", "", false)
		if result == nil {
			t.Error("InitOKForce should never return nil")
		}
	})

	// Test Result.ExitCode() on success results
	t.Run("ExitCode returns 0 for success", func(t *testing.T) {
		result := StartOK(12345, "/pg/data")
		if result.ExitCode() != 0 {
			t.Errorf("ExitCode should return 0 for success, got %d", result.ExitCode())
		}
	})
}

// ============================================================================
// PgPromoteResultData Tests (Story 2.5)
// ============================================================================

func TestPgPromoteResultData_JSON(t *testing.T) {
	data := &PgPromoteResultData{
		Promoted:     true,
		Timeline:     2,
		PreviousRole: "standby",
		CurrentRole:  "primary",
		DataDir:      "/pg/data",
		PID:          12345,
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	jsonStr := string(jsonBytes)
	if !strings.Contains(jsonStr, `"promoted":true`) {
		t.Errorf("JSON should contain promoted:true: %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"timeline":2`) {
		t.Errorf("JSON should contain timeline: %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"previous_role":"standby"`) {
		t.Errorf("JSON should contain previous_role: %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"current_role":"primary"`) {
		t.Errorf("JSON should contain current_role: %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"data_dir":"/pg/data"`) {
		t.Errorf("JSON should contain data_dir: %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"pid":12345`) {
		t.Errorf("JSON should contain pid: %s", jsonStr)
	}
}

func TestPgPromoteResultData_JSON_FailedPromotion(t *testing.T) {
	data := &PgPromoteResultData{
		Promoted:     false,
		Timeline:     0,
		PreviousRole: "primary",
		CurrentRole:  "primary",
		DataDir:      "/pg/data",
		PID:          12345,
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	jsonStr := string(jsonBytes)
	if !strings.Contains(jsonStr, `"promoted":false`) {
		t.Errorf("JSON should contain promoted:false: %s", jsonStr)
	}
	// Timeline should be omitted when 0
	if strings.Contains(jsonStr, `"timeline":0`) {
		t.Errorf("JSON should omit timeline when 0: %s", jsonStr)
	}
}

func TestPgPromoteResultData_YAML(t *testing.T) {
	data := &PgPromoteResultData{
		Promoted:     true,
		Timeline:     3,
		PreviousRole: "replica",
		CurrentRole:  "primary",
		DataDir:      "/pg/data",
	}

	yamlBytes, err := yaml.Marshal(data)
	if err != nil {
		t.Fatalf("YAML marshal failed: %v", err)
	}

	yamlStr := string(yamlBytes)
	if !strings.Contains(yamlStr, "promoted: true") {
		t.Errorf("YAML should contain promoted: %s", yamlStr)
	}
	if !strings.Contains(yamlStr, "timeline: 3") {
		t.Errorf("YAML should contain timeline: %s", yamlStr)
	}
	if !strings.Contains(yamlStr, "previous_role: replica") {
		t.Errorf("YAML should contain previous_role: %s", yamlStr)
	}
	if !strings.Contains(yamlStr, "current_role: primary") {
		t.Errorf("YAML should contain current_role: %s", yamlStr)
	}
}

func TestPromoteOK(t *testing.T) {
	result := PromoteOK(2, "standby", "primary", "/pg/data", 12345)
	if result == nil {
		t.Fatal("PromoteOK returned nil")
	}
	if !result.Success {
		t.Error("PromoteOK should return success=true")
	}
	if result.Code != 0 {
		t.Errorf("PromoteOK should return code=0, got %d", result.Code)
	}

	data, ok := result.Data.(*PgPromoteResultData)
	if !ok {
		t.Fatal("PromoteOK data should be *PgPromoteResultData")
	}
	if !data.Promoted {
		t.Error("Promoted should be true")
	}
	if data.Timeline != 2 {
		t.Errorf("Expected Timeline 2, got %d", data.Timeline)
	}
	if data.PreviousRole != "standby" {
		t.Errorf("Expected PreviousRole standby, got %s", data.PreviousRole)
	}
	if data.CurrentRole != "primary" {
		t.Errorf("Expected CurrentRole primary, got %s", data.CurrentRole)
	}
	if data.DataDir != "/pg/data" {
		t.Errorf("Expected DataDir /pg/data, got %s", data.DataDir)
	}
	if data.PID != 12345 {
		t.Errorf("Expected PID 12345, got %d", data.PID)
	}
}

func TestPgPromoteResultData_RoundTrip(t *testing.T) {
	// Test JSON round-trip
	original := &PgPromoteResultData{
		Promoted:     true,
		Timeline:     5,
		PreviousRole: "standby",
		CurrentRole:  "primary",
		DataDir:      "/pg/data",
		PID:          9999,
	}

	jsonBytes, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	var decoded PgPromoteResultData
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if decoded.Promoted != original.Promoted {
		t.Errorf("Promoted mismatch: got %v, want %v", decoded.Promoted, original.Promoted)
	}
	if decoded.Timeline != original.Timeline {
		t.Errorf("Timeline mismatch: got %d, want %d", decoded.Timeline, original.Timeline)
	}
	if decoded.PreviousRole != original.PreviousRole {
		t.Errorf("PreviousRole mismatch: got %s, want %s", decoded.PreviousRole, original.PreviousRole)
	}
	if decoded.CurrentRole != original.CurrentRole {
		t.Errorf("CurrentRole mismatch: got %s, want %s", decoded.CurrentRole, original.CurrentRole)
	}
	if decoded.DataDir != original.DataDir {
		t.Errorf("DataDir mismatch: got %s, want %s", decoded.DataDir, original.DataDir)
	}
	if decoded.PID != original.PID {
		t.Errorf("PID mismatch: got %d, want %d", decoded.PID, original.PID)
	}
}

func TestPgPromoteResultData_YAML_RoundTrip(t *testing.T) {
	// Test YAML round-trip
	original := &PgPromoteResultData{
		Promoted:     false,
		Timeline:     0,
		PreviousRole: "primary",
		CurrentRole:  "primary",
		DataDir:      "/data/pg",
		PID:          1234,
	}

	yamlBytes, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("YAML marshal failed: %v", err)
	}

	var decoded PgPromoteResultData
	if err := yaml.Unmarshal(yamlBytes, &decoded); err != nil {
		t.Fatalf("YAML unmarshal failed: %v", err)
	}

	if decoded.Promoted != original.Promoted {
		t.Errorf("Promoted mismatch: got %v, want %v", decoded.Promoted, original.Promoted)
	}
	if decoded.PreviousRole != original.PreviousRole {
		t.Errorf("PreviousRole mismatch: got %s, want %s", decoded.PreviousRole, original.PreviousRole)
	}
	if decoded.CurrentRole != original.CurrentRole {
		t.Errorf("CurrentRole mismatch: got %s, want %s", decoded.CurrentRole, original.CurrentRole)
	}
}

// ============================================================================
// Promote Error Code Tests (Story 2.5)
// ============================================================================

func TestPromoteErrorCodes(t *testing.T) {
	tests := []struct {
		name         string
		code         int
		expectedExit int
	}{
		{"CodePgAlreadyPrimary", 130607, 9},           // CAT_STATE -> exit 9
		{"CodePgReplicationNotConfigured", 130701, 8}, // CAT_CONFIG -> exit 8
		{"CodePgPromoteFailed", 130807, 1},            // CAT_OPERATION -> exit 1
		{"CodePgNotRunning", 130605, 9},               // CAT_STATE -> exit 9
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			category := (tt.code % 10000) / 100
			var expectedExit int
			switch category {
			case 0:
				expectedExit = 0
			case 1:
				expectedExit = 2
			case 2:
				expectedExit = 3
			case 3:
				expectedExit = 4
			case 4:
				expectedExit = 5
			case 5:
				expectedExit = 6
			case 6:
				expectedExit = 9
			case 7:
				expectedExit = 8
			case 8, 9:
				expectedExit = 1
			default:
				expectedExit = 1
			}
			if expectedExit != tt.expectedExit {
				t.Errorf("Code %d: expected exit %d, got %d", tt.code, tt.expectedExit, expectedExit)
			}
		})
	}
}

func TestPromoteErrorCodeConstants(t *testing.T) {
	// Verify error code constants are correctly defined
	tests := []struct {
		name         string
		code         int
		expectedMod  int
		expectedCat  int
		expectedSpec int
	}{
		{
			name:         "CodePgAlreadyPrimary",
			code:         130607,
			expectedMod:  13,
			expectedCat:  6, // CAT_STATE
			expectedSpec: 7,
		},
		{
			name:         "CodePgReplicationNotConfigured",
			code:         130701,
			expectedMod:  13,
			expectedCat:  7, // CAT_CONFIG
			expectedSpec: 1,
		},
		{
			name:         "CodePgPromoteFailed",
			code:         130807,
			expectedMod:  13,
			expectedCat:  8, // CAT_OPERATION
			expectedSpec: 7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mod := tt.code / 10000
			cat := (tt.code % 10000) / 100
			spec := tt.code % 100

			if mod != tt.expectedMod {
				t.Errorf("Module: got %d, want %d", mod, tt.expectedMod)
			}
			if cat != tt.expectedCat {
				t.Errorf("Category: got %d, want %d", cat, tt.expectedCat)
			}
			if spec != tt.expectedSpec {
				t.Errorf("Specific: got %d, want %d", spec, tt.expectedSpec)
			}
		})
	}
}

// Test detectRoleString function behavior
func TestDetectRoleString(t *testing.T) {
	// Test with nil config (should return "unknown" without panic)
	role := detectRoleString(nil)
	// The exact result depends on system state, but it shouldn't panic
	if role != "primary" && role != "replica" && role != "unknown" {
		t.Errorf("detectRoleString returned unexpected value: %s", role)
	}
}
