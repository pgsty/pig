/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>

Tests for pt switchover structured output result and DTO.
*/
package patroni

import (
	"encoding/json"
	"testing"

	"pig/internal/output"

	"gopkg.in/yaml.v3"
)

// ============================================================================
// PtSwitchoverResultData Tests
// ============================================================================

func TestPtSwitchoverResultData_JSON(t *testing.T) {
	data := &PtSwitchoverResultData{
		Command:   "patronictl -c /etc/patroni/patroni.yml switchover --force",
		Output:    "Successfully switched over to pg-test-2",
		Leader:    "pg-test-1",
		Candidate: "pg-test-2",
	}

	b, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	var decoded PtSwitchoverResultData
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if decoded.Command != data.Command {
		t.Errorf("Command = %q, want %q", decoded.Command, data.Command)
	}
	if decoded.Output != data.Output {
		t.Errorf("Output = %q, want %q", decoded.Output, data.Output)
	}
	if decoded.Leader != data.Leader {
		t.Errorf("Leader = %q, want %q", decoded.Leader, data.Leader)
	}
	if decoded.Candidate != data.Candidate {
		t.Errorf("Candidate = %q, want %q", decoded.Candidate, data.Candidate)
	}
}

func TestPtSwitchoverResultData_YAML(t *testing.T) {
	data := &PtSwitchoverResultData{
		Command:   "patronictl switchover --force",
		Output:    "Successfully switched over",
		Leader:    "node1",
		Candidate: "node2",
	}

	b, err := yaml.Marshal(data)
	if err != nil {
		t.Fatalf("YAML marshal failed: %v", err)
	}

	var decoded PtSwitchoverResultData
	if err := yaml.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("YAML unmarshal failed: %v", err)
	}

	if decoded.Command != data.Command {
		t.Errorf("Command = %q, want %q", decoded.Command, data.Command)
	}
	if decoded.Output != data.Output {
		t.Errorf("Output = %q, want %q", decoded.Output, data.Output)
	}
}

func TestPtSwitchoverResultData_JSON_OmitEmpty(t *testing.T) {
	data := &PtSwitchoverResultData{
		Command: "patronictl switchover --force",
		Output:  "ok",
	}

	b, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	s := string(b)
	if containsField(s, "leader") {
		t.Error("JSON should omit empty leader field")
	}
	if containsField(s, "candidate") {
		t.Error("JSON should omit empty candidate field")
	}
}

func TestPtSwitchoverResultData_Text_NilReceiver(t *testing.T) {
	var data *PtSwitchoverResultData
	result := data.Text()
	if result != "" {
		t.Errorf("Text() on nil receiver should return empty string, got %q", result)
	}
}

func TestPtSwitchoverResultData_Text_NonNil(t *testing.T) {
	data := &PtSwitchoverResultData{
		Command:   "patronictl switchover --force",
		Output:    "Successfully switched over to pg-test-2",
		Leader:    "pg-test-1",
		Candidate: "pg-test-2",
	}

	text := data.Text()
	if text == "" {
		t.Error("Text() should return non-empty string")
	}
	if !containsField(text, "pg-test-1") {
		t.Error("Text() should contain leader name")
	}
	if !containsField(text, "pg-test-2") {
		t.Error("Text() should contain candidate name")
	}
}

func TestPtSwitchoverResultData_Text_MinimalFields(t *testing.T) {
	data := &PtSwitchoverResultData{
		Command: "patronictl switchover --force",
		Output:  "ok",
	}

	text := data.Text()
	if text == "" {
		t.Error("Text() should return non-empty even with minimal fields")
	}
}

// ============================================================================
// SwitchoverResult Precondition Tests
// ============================================================================

func TestSwitchoverResult_NilOpts(t *testing.T) {
	// SwitchoverResult with nil opts should return NeedForce error
	// (since Force is false when opts is nil)
	// Note: This test may return CodePtNotFound if patronictl is not installed,
	// which is also acceptable behavior.
	result := SwitchoverResult("postgres", nil)
	if result == nil {
		t.Fatal("SwitchoverResult should never return nil")
	}
	if result.Success {
		t.Error("SwitchoverResult with nil opts should not succeed")
	}
	// Accept either CodePtNotFound (patronictl missing) or CodePtSwitchoverNeedForce
	if result.Code != output.CodePtNotFound &&
		result.Code != output.CodePtConfigNotFound &&
		result.Code != output.CodePtSwitchoverNeedForce {
		t.Errorf("Expected CodePtNotFound, CodePtConfigNotFound, or CodePtSwitchoverNeedForce, got %d", result.Code)
	}
}

func TestSwitchoverResult_ForceNotSet(t *testing.T) {
	// With Force=false, should return CodePtSwitchoverNeedForce
	// (unless patronictl or config is missing, which takes priority)
	opts := &SwitchoverOptions{Force: false}
	result := SwitchoverResult("postgres", opts)
	if result == nil {
		t.Fatal("SwitchoverResult should never return nil")
	}
	if result.Success {
		t.Error("SwitchoverResult without --force should not succeed")
	}
	// Accept precondition errors (patronictl/config missing) or NeedForce
	validCodes := map[int]bool{
		output.CodePtNotFound:            true,
		output.CodePtConfigNotFound:      true,
		output.CodePtSwitchoverNeedForce: true,
	}
	if !validCodes[result.Code] {
		t.Errorf("Expected a precondition or NeedForce error, got code %d", result.Code)
	}
}

// ============================================================================
// Command Building Tests
// ============================================================================

func TestBuildSwitchoverResultArgs_BasicForce(t *testing.T) {
	opts := &SwitchoverOptions{Force: true}
	args := buildSwitchoverResultArgs("/usr/bin/patronictl", opts)
	assertContains(t, args, "--force")
}

func TestBuildSwitchoverResultArgs_WithLeader(t *testing.T) {
	opts := &SwitchoverOptions{
		Force:  true,
		Leader: "pg-test-1",
	}
	args := buildSwitchoverResultArgs("/usr/bin/patronictl", opts)
	assertContains(t, args, "--leader")
	assertContains(t, args, "pg-test-1")
}

func TestBuildSwitchoverResultArgs_WithCandidate(t *testing.T) {
	opts := &SwitchoverOptions{
		Force:     true,
		Candidate: "pg-test-2",
	}
	args := buildSwitchoverResultArgs("/usr/bin/patronictl", opts)
	assertContains(t, args, "--candidate")
	assertContains(t, args, "pg-test-2")
}

func TestBuildSwitchoverResultArgs_WithScheduled(t *testing.T) {
	opts := &SwitchoverOptions{
		Force:     true,
		Scheduled: "2024-06-01T12:00:00",
	}
	args := buildSwitchoverResultArgs("/usr/bin/patronictl", opts)
	assertContains(t, args, "--scheduled")
	assertContains(t, args, "2024-06-01T12:00:00")
}

func TestBuildSwitchoverResultArgs_AllOptions(t *testing.T) {
	opts := &SwitchoverOptions{
		Leader:    "pg-test-1",
		Candidate: "pg-test-2",
		Force:     true,
		Scheduled: "2024-06-01T12:00:00",
	}
	args := buildSwitchoverResultArgs("/usr/bin/patronictl", opts)
	assertContains(t, args, "--force")
	assertContains(t, args, "--leader")
	assertContains(t, args, "pg-test-1")
	assertContains(t, args, "--candidate")
	assertContains(t, args, "pg-test-2")
	assertContains(t, args, "--scheduled")
	assertContains(t, args, "2024-06-01T12:00:00")

	// Verify config path is included
	assertContains(t, args, "-c")
	assertContains(t, args, DefaultConfigPath)
}

// ============================================================================
// Result Integration with output.Result Tests
// ============================================================================

func TestSwitchoverResultData_InResult(t *testing.T) {
	data := &PtSwitchoverResultData{
		Command:   "patronictl switchover --force",
		Output:    "Successfully switched over",
		Candidate: "pg-test-2",
	}

	result := output.OK("Switchover completed successfully", data)
	if !result.Success {
		t.Error("Result should be successful")
	}
	if result.Code != 0 {
		t.Errorf("Result code should be 0, got %d", result.Code)
	}

	// Verify data round-trips through JSON
	b, err := result.JSON()
	if err != nil {
		t.Fatalf("JSON render failed: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	dataMap, ok := decoded["data"].(map[string]interface{})
	if !ok {
		t.Fatal("data field should be a map")
	}
	if dataMap["command"] != data.Command {
		t.Errorf("command = %v, want %v", dataMap["command"], data.Command)
	}
}

func TestSwitchoverFailResult(t *testing.T) {
	data := &PtSwitchoverResultData{
		Command: "patronictl switchover --force",
		Output:  "Error: No candidate found",
	}

	result := output.Fail(output.CodePtSwitchoverFailed, "Switchover failed").
		WithDetail("exit status 1").WithData(data)

	if result.Success {
		t.Error("Result should not be successful")
	}
	if result.Code != output.CodePtSwitchoverFailed {
		t.Errorf("Code = %d, want %d", result.Code, output.CodePtSwitchoverFailed)
	}
	if result.Detail != "exit status 1" {
		t.Errorf("Detail = %q, want %q", result.Detail, "exit status 1")
	}
}

func TestSwitchoverNeedForceResult(t *testing.T) {
	result := output.Fail(output.CodePtSwitchoverNeedForce,
		"switchover requires --force (-f) flag in structured output mode")

	if result.Success {
		t.Error("Result should not be successful")
	}
	if result.Code != output.CodePtSwitchoverNeedForce {
		t.Errorf("Code = %d, want %d", result.Code, output.CodePtSwitchoverNeedForce)
	}

	// Verify exit code mapping (CAT_PARAM → exit 2)
	exitCode := result.ExitCode()
	if exitCode != 2 {
		t.Errorf("ExitCode = %d, want 2 (param error)", exitCode)
	}
}

// ============================================================================
// Helpers
// ============================================================================

func containsField(s, field string) bool {
	return len(s) > 0 && len(field) > 0 && jsonContains(s, field)
}

func jsonContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func assertContains(t *testing.T, args []string, expected string) {
	t.Helper()
	for _, a := range args {
		if a == expected {
			return
		}
	}
	t.Errorf("args %v does not contain %q", args, expected)
}
