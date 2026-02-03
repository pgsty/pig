package patroni

import (
	"strings"
	"testing"
)

func TestBuildSwitchoverPlan(t *testing.T) {
	opts := &SwitchoverOptions{
		Leader:    "pg-1",
		Candidate: "pg-2",
		Scheduled: "2026-02-03T12:00:00",
	}

	plan := BuildSwitchoverPlan(opts)
	if plan == nil {
		t.Fatal("BuildSwitchoverPlan returned nil")
	}
	if !strings.Contains(plan.Command, "switchover") {
		t.Errorf("plan.Command missing switchover: %q", plan.Command)
	}
	if !strings.Contains(plan.Command, "--candidate") {
		t.Errorf("plan.Command missing candidate: %q", plan.Command)
	}

	if len(plan.Actions) == 0 {
		t.Error("plan.Actions should not be empty")
	}
	if len(plan.Affects) == 0 {
		t.Error("plan.Affects should not be empty")
	}
	if plan.Expected == "" {
		t.Error("plan.Expected should not be empty")
	}
	if len(plan.Risks) == 0 {
		t.Error("plan.Risks should not be empty")
	}
}

func TestBuildSwitchoverPlanNilOpts(t *testing.T) {
	plan := BuildSwitchoverPlan(nil)
	if plan == nil {
		t.Fatal("BuildSwitchoverPlan(nil) should not return nil")
	}
	if !strings.Contains(plan.Command, "switchover") {
		t.Errorf("plan.Command missing switchover: %q", plan.Command)
	}
	if len(plan.Actions) == 0 {
		t.Error("plan.Actions should not be empty even with nil opts")
	}
	if plan.Expected == "" {
		t.Error("plan.Expected should not be empty")
	}
}

func TestBuildSwitchoverPlanEmptyOpts(t *testing.T) {
	opts := &SwitchoverOptions{}
	plan := BuildSwitchoverPlan(opts)
	if plan == nil {
		t.Fatal("BuildSwitchoverPlan returned nil")
	}
	// Should not include optional flags when not set
	if strings.Contains(plan.Command, "--leader") {
		t.Errorf("plan.Command should not include --leader when empty: %q", plan.Command)
	}
	if strings.Contains(plan.Command, "--candidate") {
		t.Errorf("plan.Command should not include --candidate when empty: %q", plan.Command)
	}
}

func TestBuildSwitchoverPlanForceOption(t *testing.T) {
	opts := &SwitchoverOptions{
		Force: true,
	}
	plan := BuildSwitchoverPlan(opts)
	if plan == nil {
		t.Fatal("BuildSwitchoverPlan returned nil")
	}
	if !strings.Contains(plan.Command, "--force") {
		t.Errorf("plan.Command should include --force: %q", plan.Command)
	}
	// Risks should mention force mode
	hasForceRisk := false
	for _, risk := range plan.Risks {
		if strings.Contains(risk, "force") || strings.Contains(risk, "skip") {
			hasForceRisk = true
			break
		}
	}
	if !hasForceRisk {
		t.Error("plan.Risks should mention force mode")
	}
}

func TestBuildSwitchoverCommand(t *testing.T) {
	tests := []struct {
		name     string
		opts     *SwitchoverOptions
		contains []string
		excludes []string
	}{
		{
			name:     "nil opts",
			opts:     nil,
			contains: []string{"pig", "pt", "switchover"},
			excludes: []string{"--leader", "--candidate", "--scheduled", "--force"},
		},
		{
			name: "all options",
			opts: &SwitchoverOptions{
				Leader:    "pg-1",
				Candidate: "pg-2",
				Scheduled: "2026-02-03T12:00:00",
				Force:     true,
			},
			contains: []string{"--leader", "pg-1", "--candidate", "pg-2", "--scheduled", "--force"},
			excludes: []string{},
		},
		{
			name: "only candidate",
			opts: &SwitchoverOptions{
				Candidate: "pg-2",
			},
			contains: []string{"--candidate", "pg-2"},
			excludes: []string{"--leader", "--scheduled", "--force"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := buildSwitchoverCommand(tt.opts)
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
