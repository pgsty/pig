package pitr

import (
	"strings"
	"testing"

	"pig/internal/output"
)

func TestBuildPlanBasic(t *testing.T) {
	state := &SystemState{
		PatroniActive: true,
		PGRunning:     true,
		DataDir:       "/pg/data",
		DbSU:          "postgres",
	}
	opts := &Options{
		Time: "2026-01-31 01:00:00",
	}

	plan := BuildPlan(state, opts)
	if plan == nil {
		t.Fatal("BuildPlan returned nil")
	}
	if plan.Command == "" {
		t.Error("Plan.Command should not be empty")
	}
	if !strings.Contains(plan.Command, "-t") {
		t.Errorf("Plan.Command should include -t, got %q", plan.Command)
	}

	if !containsAction(plan.Actions, "Stop Patroni service") {
		t.Error("Plan.Actions should include stopping Patroni")
	}
	if !containsAction(plan.Actions, "Ensure PostgreSQL is stopped") {
		t.Error("Plan.Actions should include stopping PostgreSQL")
	}
	if !containsAction(plan.Actions, "Execute pgBackRest restore") {
		t.Error("Plan.Actions should include pgBackRest restore")
	}

	if !containsResource(plan.Affects, "backup") {
		t.Error("Plan.Affects should include backup info")
	}
	if !containsResource(plan.Affects, "target") {
		t.Error("Plan.Affects should include recovery target")
	}
	if !strings.Contains(plan.Expected, "/pg/data") {
		t.Errorf("Plan.Expected should mention data dir, got %q", plan.Expected)
	}
	if len(plan.Risks) == 0 {
		t.Error("Plan.Risks should not be empty")
	}
}

func TestBuildPlanSkipPatroniNoRestart(t *testing.T) {
	state := &SystemState{
		PatroniActive: true,
		PGRunning:     false,
		DataDir:       "/pg/data",
		DbSU:          "postgres",
	}
	opts := &Options{
		Default:     true,
		SkipPatroni: true,
		NoRestart:   true,
	}

	plan := BuildPlan(state, opts)
	if containsAction(plan.Actions, "Stop Patroni service") {
		t.Error("Plan.Actions should not include Patroni stop when skip is set")
	}
	if containsAction(plan.Actions, "Start PostgreSQL") {
		t.Error("Plan.Actions should not include PostgreSQL start when no-restart is set")
	}
	if !strings.Contains(plan.Expected, "remains stopped") {
		t.Errorf("Plan.Expected should mention stopped state, got %q", plan.Expected)
	}
}

func TestBuildPlanNilInputs(t *testing.T) {
	// Test nil state
	plan := BuildPlan(nil, &Options{Default: true})
	if plan == nil {
		t.Fatal("BuildPlan(nil, opts) should not return nil")
	}
	if len(plan.Actions) != 0 {
		t.Errorf("BuildPlan with nil state should have empty actions, got %d", len(plan.Actions))
	}

	// Test nil opts
	plan = BuildPlan(&SystemState{DataDir: "/pg/data"}, nil)
	if plan == nil {
		t.Fatal("BuildPlan(state, nil) should not return nil")
	}
	if len(plan.Actions) != 0 {
		t.Errorf("BuildPlan with nil opts should have empty actions, got %d", len(plan.Actions))
	}

	// Test both nil
	plan = BuildPlan(nil, nil)
	if plan == nil {
		t.Fatal("BuildPlan(nil, nil) should not return nil")
	}
}

func TestBuildCommand(t *testing.T) {
	tests := []struct {
		name     string
		opts     *Options
		contains []string
		excludes []string
	}{
		{
			name:     "nil opts",
			opts:     nil,
			contains: []string{"pig", "pitr"},
			excludes: []string{"-t", "-d", "--plan"},
		},
		{
			name:     "default target",
			opts:     &Options{Default: true},
			contains: []string{"pig", "pitr", "-d"},
			excludes: []string{"-t", "-I"},
		},
		{
			name:     "time target",
			opts:     &Options{Time: "2026-01-31 01:00:00"},
			contains: []string{"-t"},
			excludes: []string{"-d", "-I"},
		},
		{
			name:     "immediate target",
			opts:     &Options{Immediate: true},
			contains: []string{"-I"},
			excludes: []string{"-d", "-t"},
		},
		{
			name:     "with backup set",
			opts:     &Options{Default: true, Set: "20240101-010101F"},
			contains: []string{"-b", "20240101-010101F"},
			excludes: []string{},
		},
		{
			name:     "with flags",
			opts:     &Options{Default: true, SkipPatroni: true, NoRestart: true, Exclusive: true, Promote: true},
			contains: []string{"--skip-patroni", "--no-restart", "-X", "-P"},
			excludes: []string{},
		},
		{
			name:     "plan mode",
			opts:     &Options{Default: true, Plan: true},
			contains: []string{"--plan"},
			excludes: []string{},
		},
		{
			name:     "lsn target",
			opts:     &Options{LSN: "0/1234567"},
			contains: []string{"-l", "0/1234567"},
			excludes: []string{"-d", "-t"},
		},
		{
			name:     "xid target",
			opts:     &Options{XID: "12345"},
			contains: []string{"-x", "12345"},
			excludes: []string{"-d", "-t"},
		},
		{
			name:     "name target",
			opts:     &Options{Name: "my_restore_point"},
			contains: []string{"-n", "my_restore_point"},
			excludes: []string{"-d", "-t"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := buildCommand(tt.opts)
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

func TestGetTargetDescription(t *testing.T) {
	tests := []struct {
		name     string
		opts     *Options
		expected string
	}{
		{"default", &Options{Default: true}, "Latest (end of WAL stream)"},
		{"immediate", &Options{Immediate: true}, "Backup consistency point"},
		{"time", &Options{Time: "2026-01-31"}, "Time: 2026-01-31"},
		{"name", &Options{Name: "my_point"}, "Restore point: my_point"},
		{"lsn", &Options{LSN: "0/1234"}, "LSN: 0/1234"},
		{"xid", &Options{XID: "999"}, "XID: 999"},
		{"none", &Options{}, "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getTargetDescription(tt.opts)
			if got != tt.expected {
				t.Errorf("getTargetDescription() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestIsPlanMode(t *testing.T) {
	tests := []struct {
		name     string
		opts     *Options
		expected bool
	}{
		{"nil opts", nil, false},
		{"plan true", &Options{Plan: true}, true},
		{"dry-run true", &Options{DryRun: true}, true},
		{"both false", &Options{}, false},
		{"both true", &Options{Plan: true, DryRun: true}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isPlanMode(tt.opts)
			if got != tt.expected {
				t.Errorf("isPlanMode() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestBuildActions(t *testing.T) {
	// Test with nil inputs
	actions := buildActions(nil, nil)
	if actions != nil {
		t.Errorf("buildActions(nil, nil) should return nil, got %v", actions)
	}

	actions = buildActions(&SystemState{}, nil)
	if actions != nil {
		t.Errorf("buildActions(state, nil) should return nil, got %v", actions)
	}

	actions = buildActions(nil, &Options{})
	if actions != nil {
		t.Errorf("buildActions(nil, opts) should return nil, got %v", actions)
	}

	// Test normal case
	state := &SystemState{PatroniActive: true, PGRunning: true, DataDir: "/pg/data"}
	opts := &Options{Default: true}
	actions = buildActions(state, opts)
	if len(actions) < 3 {
		t.Errorf("buildActions should return at least 3 actions, got %d", len(actions))
	}
}

func TestBuildAffects(t *testing.T) {
	// Test with nil inputs
	affects := buildAffects(nil, nil)
	if affects != nil {
		t.Errorf("buildAffects(nil, nil) should return nil, got %v", affects)
	}

	// Test normal case
	state := &SystemState{PatroniActive: true, DataDir: "/pg/data"}
	opts := &Options{Default: true}
	affects = buildAffects(state, opts)
	if len(affects) < 2 {
		t.Errorf("buildAffects should return at least 2 resources, got %d", len(affects))
	}

	// Test with specific backup set
	opts = &Options{Default: true, Set: "20240101-010101F"}
	affects = buildAffects(state, opts)
	hasBackup := false
	for _, a := range affects {
		if a.Type == "backup" && a.Name == "20240101-010101F" {
			hasBackup = true
			break
		}
	}
	if !hasBackup {
		t.Error("buildAffects should include specified backup set")
	}
}

func TestBuildExpected(t *testing.T) {
	// Test with nil inputs
	expected := buildExpected(nil, nil)
	if expected != "" {
		t.Errorf("buildExpected(nil, nil) should return empty, got %q", expected)
	}

	// Test normal case
	state := &SystemState{DataDir: "/pg/data"}
	opts := &Options{Default: true}
	expected = buildExpected(state, opts)
	if !strings.Contains(expected, "/pg/data") {
		t.Errorf("buildExpected should contain data dir, got %q", expected)
	}

	// Test with NoRestart
	opts = &Options{Default: true, NoRestart: true}
	expected = buildExpected(state, opts)
	if !strings.Contains(expected, "stopped") {
		t.Errorf("buildExpected with NoRestart should mention stopped, got %q", expected)
	}

	// Test with Promote
	opts = &Options{Default: true, Promote: true}
	expected = buildExpected(state, opts)
	if !strings.Contains(expected, "promote") {
		t.Errorf("buildExpected with Promote should mention promote, got %q", expected)
	}
}

func TestBuildRisks(t *testing.T) {
	// Test with nil inputs
	risks := buildRisks(nil, nil)
	if risks != nil {
		t.Errorf("buildRisks(nil, nil) should return nil, got %v", risks)
	}

	// Test base risks
	state := &SystemState{DataDir: "/pg/data"}
	opts := &Options{Default: true}
	risks = buildRisks(state, opts)
	if len(risks) == 0 {
		t.Error("buildRisks should return at least one risk")
	}

	// Test with Patroni active
	state = &SystemState{PatroniActive: true, DataDir: "/pg/data"}
	opts = &Options{Default: true}
	risks = buildRisks(state, opts)
	hasPatroniRisk := false
	for _, r := range risks {
		if strings.Contains(r, "Patroni") {
			hasPatroniRisk = true
			break
		}
	}
	if !hasPatroniRisk {
		t.Error("buildRisks with Patroni active should mention Patroni")
	}

	// Test with SkipPatroni
	opts = &Options{Default: true, SkipPatroni: true}
	risks = buildRisks(state, opts)
	hasSkipRisk := false
	for _, r := range risks {
		if strings.Contains(r, "not stopped") {
			hasSkipRisk = true
			break
		}
	}
	if !hasSkipRisk {
		t.Error("buildRisks with SkipPatroni should warn about Patroni not stopped")
	}

	// Test with Exclusive
	opts = &Options{Default: true, Exclusive: true}
	risks = buildRisks(state, opts)
	hasExclusiveRisk := false
	for _, r := range risks {
		if strings.Contains(r, "Exclusive") || strings.Contains(r, "before target") {
			hasExclusiveRisk = true
			break
		}
	}
	if !hasExclusiveRisk {
		t.Error("buildRisks with Exclusive should mention exclusive mode")
	}
}

func TestQuoteIfNeeded(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"with space", `"with space"`},
		{"with\ttab", `"with\ttab"`},
		{"no-special", "no-special"},
	}

	for _, tt := range tests {
		got := quoteIfNeeded(tt.input)
		if got != tt.expected {
			t.Errorf("quoteIfNeeded(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func containsAction(actions []output.Action, needle string) bool {
	for _, action := range actions {
		if strings.Contains(action.Description, needle) {
			return true
		}
	}
	return false
}

func containsResource(resources []output.Resource, resType string) bool {
	for _, res := range resources {
		if res.Type == resType {
			return true
		}
	}
	return false
}
