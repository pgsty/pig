package postgres

import (
	"strings"
	"testing"

	"pig/internal/output"
)

// ============================================================================
// BuildRestartPlan Tests
// ============================================================================

func TestBuildRestartPlanFromState_Running(t *testing.T) {
	plan := buildRestartPlanFromState("/pg/data", true, 12345, "fast")
	if plan == nil {
		t.Fatal("buildRestartPlanFromState returned nil")
	}

	// Check command
	if !strings.Contains(plan.Command, "restart") {
		t.Errorf("Plan.Command should contain 'restart', got %q", plan.Command)
	}
	if !strings.Contains(plan.Command, "-m fast") {
		t.Errorf("Plan.Command should contain '-m fast', got %q", plan.Command)
	}

	// Check actions
	if len(plan.Actions) != 2 {
		t.Errorf("Expected 2 actions (stop + start), got %d", len(plan.Actions))
	}
	if !containsAction(plan.Actions, "Stop PostgreSQL") {
		t.Error("Actions should include stop step")
	}
	if !containsAction(plan.Actions, "Start PostgreSQL") {
		t.Error("Actions should include start step")
	}

	// Check affects
	if !containsResourceType(plan.Affects, "directory") {
		t.Error("Affects should include data directory")
	}
	if !containsResourceType(plan.Affects, "connection") {
		t.Error("Affects should include connections when running")
	}
	if !containsResourceType(plan.Affects, "service") {
		t.Error("Affects should include postgresql service when running")
	}

	// Check expected
	if !strings.Contains(plan.Expected, "restarted") {
		t.Errorf("Expected should mention 'restarted', got %q", plan.Expected)
	}
	if !strings.Contains(plan.Expected, "/pg/data") {
		t.Errorf("Expected should mention data dir, got %q", plan.Expected)
	}

	// Check risks
	if len(plan.Risks) == 0 {
		t.Error("Risks should not be empty when instance is running")
	}
	if !containsRisk(plan.Risks, "connection") {
		t.Error("Risks should mention connection termination")
	}
}

func TestBuildRestartPlanFromState_NotRunning(t *testing.T) {
	plan := buildRestartPlanFromState("/pg/data", false, 0, "fast")
	if plan == nil {
		t.Fatal("buildRestartPlanFromState returned nil")
	}

	// Only start action when not running
	if len(plan.Actions) != 1 {
		t.Errorf("Expected 1 action (start only), got %d", len(plan.Actions))
	}
	if !containsAction(plan.Actions, "Start PostgreSQL") {
		t.Error("Actions should include start step")
	}

	// Expected should say "started" not "restarted"
	if !strings.Contains(plan.Expected, "started") {
		t.Errorf("Expected should mention 'started', got %q", plan.Expected)
	}

	// No risks when not running
	if len(plan.Risks) != 0 {
		t.Errorf("Risks should be empty when not running, got %v", plan.Risks)
	}
}

func TestBuildRestartActions(t *testing.T) {
	// Running instance: 2 actions
	actions := buildRestartActions(true, "fast")
	if len(actions) != 2 {
		t.Errorf("Expected 2 actions for running instance, got %d", len(actions))
	}

	// Not running: 1 action
	actions = buildRestartActions(false, "fast")
	if len(actions) != 1 {
		t.Errorf("Expected 1 action for stopped instance, got %d", len(actions))
	}
}

func TestBuildRestartAffects(t *testing.T) {
	// Running instance
	affects := buildRestartAffects("/pg/data", true, 12345)
	if len(affects) != 3 {
		t.Errorf("Expected 3 resources for running instance, got %d", len(affects))
	}

	// Check PID in detail
	found := false
	for _, a := range affects {
		if a.Type == "service" && strings.Contains(a.Detail, "12345") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Service resource should include PID in detail")
	}

	// Not running instance
	affects = buildRestartAffects("/pg/data", false, 0)
	if len(affects) != 1 {
		t.Errorf("Expected 1 resource for stopped instance, got %d", len(affects))
	}
}

func TestBuildRestartExpected(t *testing.T) {
	// Running
	expected := buildRestartExpected("/pg/data", true)
	if !strings.Contains(expected, "restarted") {
		t.Errorf("Expected 'restarted' for running instance, got %q", expected)
	}

	// Not running
	expected = buildRestartExpected("/pg/data", false)
	if !strings.Contains(expected, "started") && strings.Contains(expected, "restarted") {
		t.Errorf("Expected 'started' for stopped instance, got %q", expected)
	}
}

func TestBuildRestartRisks(t *testing.T) {
	// Running
	risks := buildRestartRisks(true)
	if len(risks) == 0 {
		t.Error("Should have risks when running")
	}

	// Not running
	risks = buildRestartRisks(false)
	if risks != nil {
		t.Errorf("Should have no risks when not running, got %v", risks)
	}
}

func TestBuildRestartCommand(t *testing.T) {
	tests := []struct {
		mode     string
		contains string
	}{
		{"fast", "-m fast"},
		{"smart", "-m smart"},
		{"immediate", "-m immediate"},
	}

	for _, tt := range tests {
		cmd := buildRestartCommand(tt.mode)
		if !strings.Contains(cmd, tt.contains) {
			t.Errorf("buildRestartCommand(%q) should contain %q, got %q", tt.mode, tt.contains, cmd)
		}
		if !strings.Contains(cmd, "pig pg restart") {
			t.Errorf("buildRestartCommand should contain 'pig pg restart', got %q", cmd)
		}
	}
}

// ============================================================================
// BuildStopPlan Tests
// ============================================================================

func TestBuildStopPlanFromState_Running(t *testing.T) {
	plan := buildStopPlanFromState("/pg/data", true, 12345, "fast")
	if plan == nil {
		t.Fatal("buildStopPlanFromState returned nil")
	}

	// Check command
	if !strings.Contains(plan.Command, "stop") {
		t.Errorf("Plan.Command should contain 'stop', got %q", plan.Command)
	}
	if !strings.Contains(plan.Command, "-m fast") {
		t.Errorf("Plan.Command should contain '-m fast', got %q", plan.Command)
	}

	// Check actions
	if len(plan.Actions) != 1 {
		t.Errorf("Expected 1 action for running instance, got %d", len(plan.Actions))
	}
	if !containsAction(plan.Actions, "Stop PostgreSQL") {
		t.Error("Actions should include stop step")
	}

	// Check affects
	if !containsResourceType(plan.Affects, "directory") {
		t.Error("Affects should include data directory")
	}
	if !containsResourceType(plan.Affects, "connection") {
		t.Error("Affects should include connections when running")
	}

	// Check expected
	if !strings.Contains(plan.Expected, "stopped") {
		t.Errorf("Expected should mention 'stopped', got %q", plan.Expected)
	}

	// Check risks
	if len(plan.Risks) == 0 {
		t.Error("Risks should not be empty when instance is running")
	}
}

func TestBuildStopPlanFromState_NotRunning(t *testing.T) {
	plan := buildStopPlanFromState("/pg/data", false, 0, "fast")
	if plan == nil {
		t.Fatal("buildStopPlanFromState returned nil")
	}

	// No actions when already stopped
	if len(plan.Actions) != 0 {
		t.Errorf("Expected 0 actions for stopped instance, got %d", len(plan.Actions))
	}

	// Expected should indicate already stopped
	if !strings.Contains(plan.Expected, "already stopped") {
		t.Errorf("Expected should mention 'already stopped', got %q", plan.Expected)
	}

	// No risks when not running
	if len(plan.Risks) != 0 {
		t.Errorf("Risks should be empty when not running, got %v", plan.Risks)
	}
}

func TestBuildStopActions(t *testing.T) {
	// Running instance: 1 action
	actions := buildStopActions(true, "fast")
	if len(actions) != 1 {
		t.Errorf("Expected 1 action for running instance, got %d", len(actions))
	}

	// Not running: no actions
	actions = buildStopActions(false, "fast")
	if len(actions) != 0 {
		t.Errorf("Expected 0 actions for stopped instance, got %d", len(actions))
	}
}

func TestBuildStopAffects(t *testing.T) {
	// Running instance
	affects := buildStopAffects("/pg/data", true, 12345)
	if len(affects) != 3 {
		t.Errorf("Expected 3 resources for running instance, got %d", len(affects))
	}

	// Not running instance
	affects = buildStopAffects("/pg/data", false, 0)
	if len(affects) != 1 {
		t.Errorf("Expected 1 resource for stopped instance, got %d", len(affects))
	}
}

func TestBuildStopExpected(t *testing.T) {
	// Running
	expected := buildStopExpected("/pg/data", true)
	if !strings.Contains(expected, "stopped") || strings.Contains(expected, "already") {
		t.Errorf("Expected 'stopped' without 'already' for running instance, got %q", expected)
	}

	// Not running
	expected = buildStopExpected("/pg/data", false)
	if !strings.Contains(expected, "already stopped") {
		t.Errorf("Expected 'already stopped' for stopped instance, got %q", expected)
	}
}

func TestBuildStopRisks(t *testing.T) {
	// Running with fast mode
	risks := buildStopRisks(true, "fast")
	if len(risks) < 2 {
		t.Errorf("Should have at least 2 base risks, got %d", len(risks))
	}

	// Running with smart mode
	risks = buildStopRisks(true, "smart")
	if !containsRisk(risks, "wait") {
		t.Error("Smart mode should mention waiting for clients")
	}

	// Running with immediate mode
	risks = buildStopRisks(true, "immediate")
	if !containsRisk(risks, "recovery") {
		t.Error("Immediate mode should mention recovery")
	}

	// Not running
	risks = buildStopRisks(false, "fast")
	if risks != nil {
		t.Errorf("Should have no risks when not running, got %v", risks)
	}
}

func TestBuildStopCommand(t *testing.T) {
	tests := []struct {
		mode     string
		contains string
	}{
		{"fast", "-m fast"},
		{"smart", "-m smart"},
		{"immediate", "-m immediate"},
	}

	for _, tt := range tests {
		cmd := buildStopCommand(tt.mode)
		if !strings.Contains(cmd, tt.contains) {
			t.Errorf("buildStopCommand(%q) should contain %q, got %q", tt.mode, tt.contains, cmd)
		}
		if !strings.Contains(cmd, "pig pg stop") {
			t.Errorf("buildStopCommand should contain 'pig pg stop', got %q", cmd)
		}
	}
}

// ============================================================================
// Plan Structure Tests
// ============================================================================

func TestPlanFieldsCompleteness(t *testing.T) {
	// Test restart plan
	restartPlan := buildRestartPlanFromState("/pg/data", true, 12345, "fast")
	validatePlanFields(t, restartPlan, "restart")

	// Test stop plan
	stopPlan := buildStopPlanFromState("/pg/data", true, 12345, "fast")
	validatePlanFields(t, stopPlan, "stop")
}

func validatePlanFields(t *testing.T, plan *output.Plan, planType string) {
	if plan.Command == "" {
		t.Errorf("%s plan: Command should not be empty", planType)
	}
	if len(plan.Actions) == 0 && planType == "restart" {
		t.Errorf("%s plan: Actions should not be empty for restart", planType)
	}
	if len(plan.Affects) == 0 {
		t.Errorf("%s plan: Affects should not be empty", planType)
	}
	if plan.Expected == "" {
		t.Errorf("%s plan: Expected should not be empty", planType)
	}
	// Risks can be empty for some states
}

// ============================================================================
// Helper Functions
// ============================================================================

func containsAction(actions []output.Action, needle string) bool {
	for _, action := range actions {
		if strings.Contains(action.Description, needle) {
			return true
		}
	}
	return false
}

func containsResourceType(resources []output.Resource, resType string) bool {
	for _, res := range resources {
		if res.Type == resType {
			return true
		}
	}
	return false
}

func containsRisk(risks []string, needle string) bool {
	for _, risk := range risks {
		if strings.Contains(strings.ToLower(risk), strings.ToLower(needle)) {
			return true
		}
	}
	return false
}
