/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

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

func TestSwitchoverResultUsesResolvedClusterName(t *testing.T) {
	var captured []string
	stubPatroniResultDeps(t, "pg-nms", nil, &captured)

	result := SwitchoverResult("postgres", &SwitchoverOptions{
		Force:     true,
		Candidate: "pg-nms-2",
	})
	if !result.Success {
		t.Fatalf("SwitchoverResult should succeed with stubbed deps, got code=%d detail=%q", result.Code, result.Detail)
	}
	assertArgPrefix(t, captured, []string{"/usr/bin/patronictl", "-c", DefaultConfigPath, "switchover", "pg-nms", "--force"})
	assertContains(t, captured, "--candidate")
	assertContains(t, captured, "pg-nms-2")
}

func TestSwitchoverResultRequiresForce(t *testing.T) {
	tests := []struct {
		name string
		opts *SwitchoverOptions
	}{
		{name: "nil opts", opts: nil},
		{name: "force false", opts: &SwitchoverOptions{Force: false}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var captured []string
			stubPatroniResultDeps(t, "pg-nms", nil, &captured)

			result := SwitchoverResult("postgres", tt.opts)
			if result.Code != output.CodePtConfirmationRequired {
				t.Fatalf("code = %d, want %d", result.Code, output.CodePtConfirmationRequired)
			}
			if captured != nil {
				t.Fatalf("patronictl should not execute without --force, captured=%v", captured)
			}
		})
	}
}

func TestSwitchoverResultRejectsInvalidResolvedClusterName(t *testing.T) {
	tests := []struct {
		name    string
		cluster string
		code    int
	}{
		{name: "empty", cluster: "", code: output.CodePtScopeMissing},
		{name: "whitespace", cluster: "   ", code: output.CodePtScopeMissing},
		{name: "flag-like", cluster: "--force", code: output.CodePtConfigResolveFailed},
		{name: "internal whitespace", cluster: "pg test", code: output.CodePtConfigResolveFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var captured []string
			stubPatroniResultDeps(t, tt.cluster, nil, &captured)

			result := SwitchoverResult("postgres", &SwitchoverOptions{Force: true})
			if result.Success {
				t.Fatal("SwitchoverResult should fail for invalid resolved cluster")
			}
			if result.Code != tt.code {
				t.Fatalf("code = %d, want %d", result.Code, tt.code)
			}
			if captured != nil {
				t.Fatalf("patronictl should not execute for invalid cluster, captured=%v", captured)
			}
		})
	}
}

// ============================================================================
// Command Building Tests
// ============================================================================

func TestBuildSwitchoverResultArgs_BasicForce(t *testing.T) {
	opts := &SwitchoverOptions{Force: true}
	args := buildSwitchoverResultArgs("/usr/bin/patronictl", "pg-nms", opts)
	assertArgPrefix(t, args, []string{"/usr/bin/patronictl", "-c", DefaultConfigPath, "switchover", "pg-nms", "--force"})
	assertContains(t, args, "--force")
}

func TestBuildSwitchoverResultArgs_WithLeader(t *testing.T) {
	opts := &SwitchoverOptions{
		Force:  true,
		Leader: "pg-test-1",
	}
	args := buildSwitchoverResultArgs("/usr/bin/patronictl", "pg-nms", opts)
	assertContains(t, args, "--leader")
	assertContains(t, args, "pg-test-1")
}

func TestBuildSwitchoverResultArgs_WithCandidate(t *testing.T) {
	opts := &SwitchoverOptions{
		Force:     true,
		Candidate: "pg-test-2",
	}
	args := buildSwitchoverResultArgs("/usr/bin/patronictl", "pg-nms", opts)
	assertContains(t, args, "--candidate")
	assertContains(t, args, "pg-test-2")
}

func TestBuildSwitchoverResultArgs_WithScheduled(t *testing.T) {
	opts := &SwitchoverOptions{
		Force:     true,
		Scheduled: "2024-06-01T12:00:00",
	}
	args := buildSwitchoverResultArgs("/usr/bin/patronictl", "pg-nms", opts)
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
	args := buildSwitchoverResultArgs("/usr/bin/patronictl", "pg-nms", opts)
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

func TestSwitchoverNeedYesResult(t *testing.T) {
	result := output.Fail(output.CodePtConfirmationRequired,
		"switchover requires --yes (-y) flag in structured output mode")

	if result.Success {
		t.Error("Result should not be successful")
	}
	if result.Code != output.CodePtConfirmationRequired {
		t.Errorf("Code = %d, want %d", result.Code, output.CodePtConfirmationRequired)
	}

	// Verify exit code mapping (CAT_CONFIRM → exit 7)
	exitCode := result.ExitCode()
	if exitCode != 7 {
		t.Errorf("ExitCode = %d, want 7 (confirmation required)", exitCode)
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

func assertArgPrefix(t *testing.T, args []string, expected []string) {
	t.Helper()
	if len(args) < len(expected) {
		t.Fatalf("args %v shorter than expected prefix %v", args, expected)
	}
	for i, want := range expected {
		if args[i] != want {
			t.Fatalf("args[%d] = %q, want %q; args=%v", i, args[i], want, args)
		}
	}
}
