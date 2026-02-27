/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

Tests for pt failover structured output result, DTO, and plan.
*/
package patroni

import (
	"encoding/json"
	"strings"
	"testing"

	"pig/internal/output"

	"gopkg.in/yaml.v3"
)

// ============================================================================
// PtFailoverResultData Serialization Tests
// ============================================================================

func TestPtFailoverResultData_JSON(t *testing.T) {
	data := &PtFailoverResultData{
		Command:   "patronictl -c /etc/patroni/patroni.yml failover --force",
		Output:    "Successfully failed over to pg-test-2",
		Candidate: "pg-test-2",
		Force:     true,
	}

	b, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	var decoded PtFailoverResultData
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if decoded.Command != data.Command {
		t.Errorf("Command = %q, want %q", decoded.Command, data.Command)
	}
	if decoded.Output != data.Output {
		t.Errorf("Output = %q, want %q", decoded.Output, data.Output)
	}
	if decoded.Candidate != data.Candidate {
		t.Errorf("Candidate = %q, want %q", decoded.Candidate, data.Candidate)
	}
	if decoded.Force != data.Force {
		t.Errorf("Force = %v, want %v", decoded.Force, data.Force)
	}
}

func TestPtFailoverResultData_YAML(t *testing.T) {
	data := &PtFailoverResultData{
		Command:   "patronictl failover --force",
		Output:    "Successfully failed over",
		Candidate: "node2",
		Force:     true,
	}

	b, err := yaml.Marshal(data)
	if err != nil {
		t.Fatalf("YAML marshal failed: %v", err)
	}

	var decoded PtFailoverResultData
	if err := yaml.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("YAML unmarshal failed: %v", err)
	}

	if decoded.Command != data.Command {
		t.Errorf("Command = %q, want %q", decoded.Command, data.Command)
	}
	if decoded.Output != data.Output {
		t.Errorf("Output = %q, want %q", decoded.Output, data.Output)
	}
	if decoded.Force != data.Force {
		t.Errorf("Force = %v, want %v", decoded.Force, data.Force)
	}
}

func TestPtFailoverResultData_JSON_OmitEmpty(t *testing.T) {
	data := &PtFailoverResultData{
		Command: "patronictl failover --force",
		Output:  "ok",
		Force:   true,
	}

	b, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	s := string(b)
	if strings.Contains(s, `"candidate"`) {
		t.Error("JSON should omit empty candidate field")
	}
	// force should always be present (not omitempty)
	if !strings.Contains(s, `"force"`) {
		t.Error("JSON should always include force field")
	}
}

// ============================================================================
// PtFailoverResultData Text Tests
// ============================================================================

func TestPtFailoverResultData_Text_NilReceiver(t *testing.T) {
	var data *PtFailoverResultData
	result := data.Text()
	if result != "" {
		t.Errorf("Text() on nil receiver should return empty string, got %q", result)
	}
}

func TestPtFailoverResultData_Text_NonNil(t *testing.T) {
	data := &PtFailoverResultData{
		Command:   "patronictl failover --force",
		Output:    "Successfully failed over to pg-test-2",
		Candidate: "pg-test-2",
		Force:     true,
	}

	text := data.Text()
	if text == "" {
		t.Error("Text() should return non-empty string")
	}
	if !strings.Contains(text, "pg-test-2") {
		t.Error("Text() should contain candidate name")
	}
	if !strings.Contains(text, "true") {
		t.Error("Text() should contain force value")
	}
}

func TestPtFailoverResultData_Text_MinimalFields(t *testing.T) {
	data := &PtFailoverResultData{
		Command: "patronictl failover --force",
		Output:  "ok",
		Force:   true,
	}

	text := data.Text()
	if text == "" {
		t.Error("Text() should return non-empty even with minimal fields")
	}
}

// ============================================================================
// FailoverResult Precondition Tests
// ============================================================================

func TestFailoverResult_NilOpts(t *testing.T) {
	result := FailoverResult("postgres", nil)
	if result == nil {
		t.Fatal("FailoverResult should never return nil")
	}
	if result.Success {
		t.Error("FailoverResult with nil opts should not succeed")
	}
	// Accept either CodePtNotFound (patronictl missing), CodePtConfigNotFound, or CodePtFailoverNeedForce
	validCodes := map[int]bool{
		output.CodePtNotFound:           true,
		output.CodePtConfigNotFound:     true,
		output.CodePtFailoverNeedForce:  true,
	}
	if !validCodes[result.Code] {
		t.Errorf("Expected a precondition or NeedForce error, got code %d", result.Code)
	}
}

func TestFailoverResult_ForceNotSet(t *testing.T) {
	opts := &FailoverOptions{Force: false}
	result := FailoverResult("postgres", opts)
	if result == nil {
		t.Fatal("FailoverResult should never return nil")
	}
	if result.Success {
		t.Error("FailoverResult without --force should not succeed")
	}
	validCodes := map[int]bool{
		output.CodePtNotFound:          true,
		output.CodePtConfigNotFound:    true,
		output.CodePtFailoverNeedForce: true,
	}
	if !validCodes[result.Code] {
		t.Errorf("Expected a precondition or NeedForce error, got code %d", result.Code)
	}
}

// ============================================================================
// Command Building Tests
// ============================================================================

func TestBuildFailoverResultArgs_BasicForce(t *testing.T) {
	opts := &FailoverOptions{Force: true}
	args := buildFailoverResultArgs("/usr/bin/patronictl", opts)
	assertContainsStr(t, args, "--force")
	assertContainsStr(t, args, "failover")
	assertContainsStr(t, args, "-c")
	assertContainsStr(t, args, DefaultConfigPath)
}

func TestBuildFailoverResultArgs_WithCandidate(t *testing.T) {
	opts := &FailoverOptions{
		Force:     true,
		Candidate: "pg-test-2",
	}
	args := buildFailoverResultArgs("/usr/bin/patronictl", opts)
	assertContainsStr(t, args, "--candidate")
	assertContainsStr(t, args, "pg-test-2")
}

func TestBuildFailoverResultArgs_NilOpts(t *testing.T) {
	args := buildFailoverResultArgs("/usr/bin/patronictl", nil)
	assertContainsStr(t, args, "--force")
	assertContainsStr(t, args, "failover")
	// Should not contain candidate
	for _, a := range args {
		if a == "--candidate" {
			t.Error("nil opts should not include --candidate")
		}
	}
}

func TestBuildFailoverResultArgs_NoCandidate(t *testing.T) {
	opts := &FailoverOptions{Force: true}
	args := buildFailoverResultArgs("/usr/bin/patronictl", opts)
	for _, a := range args {
		if a == "--candidate" {
			t.Error("empty candidate should not include --candidate")
		}
	}
}

// ============================================================================
// BuildFailoverPlan Tests
// ============================================================================

func TestBuildFailoverPlan_NilOpts(t *testing.T) {
	plan := BuildFailoverPlan(nil)
	if plan == nil {
		t.Fatal("BuildFailoverPlan(nil) should not return nil")
	}
	if !strings.Contains(plan.Command, "failover") {
		t.Errorf("plan.Command missing failover: %q", plan.Command)
	}
	if len(plan.Actions) == 0 {
		t.Error("plan.Actions should not be empty even with nil opts")
	}
	if plan.Expected == "" {
		t.Error("plan.Expected should not be empty")
	}
	if len(plan.Risks) < 4 {
		t.Errorf("plan.Risks should have at least 4 data loss warnings, got %d", len(plan.Risks))
	}
}

func TestBuildFailoverPlan_EmptyOpts(t *testing.T) {
	opts := &FailoverOptions{}
	plan := BuildFailoverPlan(opts)
	if plan == nil {
		t.Fatal("BuildFailoverPlan returned nil")
	}
	if strings.Contains(plan.Command, "--candidate") {
		t.Errorf("plan.Command should not include --candidate when empty: %q", plan.Command)
	}
	if strings.Contains(plan.Command, "--force") {
		t.Errorf("plan.Command should not include --force when not set: %q", plan.Command)
	}
}

func TestBuildFailoverPlan_FullOpts(t *testing.T) {
	opts := &FailoverOptions{
		Candidate: "pg-test-2",
		Force:     true,
	}

	plan := BuildFailoverPlan(opts)
	if plan == nil {
		t.Fatal("BuildFailoverPlan returned nil")
	}
	if !strings.Contains(plan.Command, "failover") {
		t.Errorf("plan.Command missing failover: %q", plan.Command)
	}
	if !strings.Contains(plan.Command, "--candidate") {
		t.Errorf("plan.Command missing --candidate: %q", plan.Command)
	}
	if !strings.Contains(plan.Command, "--force") {
		t.Errorf("plan.Command missing --force: %q", plan.Command)
	}

	if len(plan.Actions) == 0 {
		t.Error("plan.Actions should not be empty")
	}
	if len(plan.Affects) < 2 {
		t.Errorf("plan.Affects should have at least 2 entries (cluster + candidate), got %d", len(plan.Affects))
	}
	if !strings.Contains(plan.Expected, "pg-test-2") {
		t.Errorf("plan.Expected should mention candidate: %q", plan.Expected)
	}
}

func TestBuildFailoverPlan_RisksContainDataLossWarning(t *testing.T) {
	plan := BuildFailoverPlan(nil)
	if plan == nil {
		t.Fatal("BuildFailoverPlan returned nil")
	}

	hasDataLoss := false
	for _, risk := range plan.Risks {
		if strings.Contains(risk, "DATA LOSS") {
			hasDataLoss = true
			break
		}
	}
	if !hasDataLoss {
		t.Error("plan.Risks must contain DATA LOSS warning for failover")
	}
}

func TestBuildFailoverPlan_ForceAddsRisk(t *testing.T) {
	opts := &FailoverOptions{Force: true}
	plan := BuildFailoverPlan(opts)
	if plan == nil {
		t.Fatal("BuildFailoverPlan returned nil")
	}

	hasForceRisk := false
	for _, risk := range plan.Risks {
		if strings.Contains(risk, "force") || strings.Contains(risk, "skip") {
			hasForceRisk = true
			break
		}
	}
	if !hasForceRisk {
		t.Error("plan.Risks should mention force mode when Force=true")
	}
}

// ============================================================================
// buildFailoverCommand Tests
// ============================================================================

func TestBuildFailoverCommand(t *testing.T) {
	tests := []struct {
		name     string
		opts     *FailoverOptions
		contains []string
		excludes []string
	}{
		{
			name:     "nil opts",
			opts:     nil,
			contains: []string{"pig", "pt", "failover"},
			excludes: []string{"--candidate", "--force"},
		},
		{
			name: "all options",
			opts: &FailoverOptions{
				Candidate: "pg-2",
				Force:     true,
			},
			contains: []string{"--candidate", "pg-2", "--force"},
			excludes: []string{},
		},
		{
			name: "only candidate",
			opts: &FailoverOptions{
				Candidate: "pg-2",
			},
			contains: []string{"--candidate", "pg-2"},
			excludes: []string{"--force"},
		},
		{
			name: "only force",
			opts: &FailoverOptions{
				Force: true,
			},
			contains: []string{"--force"},
			excludes: []string{"--candidate"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := buildFailoverCommand(tt.opts)
			for _, c := range tt.contains {
				if !strings.Contains(cmd, c) {
					t.Errorf("command should contain %q: %q", c, cmd)
				}
			}
			for _, e := range tt.excludes {
				if strings.Contains(cmd, e) {
					t.Errorf("command should not contain %q: %q", e, cmd)
				}
			}
		})
	}
}

// ============================================================================
// Result Integration with output.Result Tests
// ============================================================================

func TestFailoverResultData_InResult(t *testing.T) {
	data := &PtFailoverResultData{
		Command:   "patronictl failover --force",
		Output:    "Successfully failed over",
		Candidate: "pg-test-2",
		Force:     true,
	}

	result := output.OK("Failover completed successfully", data)
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
	if dataMap["force"] != true {
		t.Errorf("force = %v, want true", dataMap["force"])
	}
}

func TestFailoverFailResult(t *testing.T) {
	data := &PtFailoverResultData{
		Command: "patronictl failover --force",
		Output:  "Error: No candidates available for failover",
		Force:   true,
	}

	result := output.Fail(output.CodePtFailoverFailed, "Failover failed").
		WithDetail("exit status 1").WithData(data)

	if result.Success {
		t.Error("Result should not be successful")
	}
	if result.Code != output.CodePtFailoverFailed {
		t.Errorf("Code = %d, want %d", result.Code, output.CodePtFailoverFailed)
	}
	if result.Detail != "exit status 1" {
		t.Errorf("Detail = %q, want %q", result.Detail, "exit status 1")
	}
}

func TestFailoverNeedForceResult(t *testing.T) {
	result := output.Fail(output.CodePtFailoverNeedForce,
		"failover requires --force (-f) flag in structured output mode")

	if result.Success {
		t.Error("Result should not be successful")
	}
	if result.Code != output.CodePtFailoverNeedForce {
		t.Errorf("Code = %d, want %d", result.Code, output.CodePtFailoverNeedForce)
	}

	// Verify exit code mapping (CAT_PARAM → exit 2)
	exitCode := result.ExitCode()
	if exitCode != 2 {
		t.Errorf("ExitCode = %d, want 2 (param error)", exitCode)
	}
}

func TestFailoverFailedExitCode(t *testing.T) {
	result := output.Fail(output.CodePtFailoverFailed, "Failover failed")

	// Verify exit code mapping (CAT_OPERATION → exit 1)
	exitCode := result.ExitCode()
	if exitCode != 1 {
		t.Errorf("ExitCode = %d, want 1 (operation error)", exitCode)
	}
}

// ============================================================================
// Helpers
// ============================================================================

func assertContainsStr(t *testing.T, args []string, expected string) {
	t.Helper()
	for _, a := range args {
		if a == expected {
			return
		}
	}
	t.Errorf("args %v does not contain %q", args, expected)
}
