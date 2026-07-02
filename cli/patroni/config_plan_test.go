package patroni

import (
	"strings"
	"testing"

	"pig/internal/output"
)

func TestAnalyzePGConfigPairsFlagsRestartParams(t *testing.T) {
	analysis := AnalyzePGConfigPairs([]string{
		"Shared_Preload_Libraries=pg_stat_statements,auto_explain",
		"shared_buffers=4GB",
		"log_min_duration_statement=250ms",
	})

	if !analysis.RequiresRestart {
		t.Fatal("expected restart to be required when postmaster parameters are present")
	}
	for _, want := range []string{"shared_preload_libraries", "shared_buffers"} {
		if !containsString(analysis.RestartParams, want) {
			t.Fatalf("restart params = %v, want %s", analysis.RestartParams, want)
		}
	}
	if containsString(analysis.RestartParams, "log_min_duration_statement") {
		t.Fatalf("log_min_duration_statement should not be treated as a restart parameter: %v", analysis.RestartParams)
	}
}

func TestAnalyzePGConfigPairsDoesNotRequireRestartForReloadParams(t *testing.T) {
	analysis := AnalyzePGConfigPairs([]string{
		"log_min_duration_statement=250ms",
		"work_mem=64MB",
	})

	if analysis.RequiresRestart {
		t.Fatalf("reload/session parameters should not require restart: %+v", analysis)
	}
	if len(analysis.RestartParams) != 0 {
		t.Fatalf("unexpected restart params: %v", analysis.RestartParams)
	}
}

func TestAnalyzePGConfigPairsKeepsLegacyRestartParams(t *testing.T) {
	analysis := AnalyzePGConfigPairs([]string{"old_snapshot_threshold=1h"})

	if !analysis.RequiresRestart || !containsString(analysis.RestartParams, "old_snapshot_threshold") {
		t.Fatalf("legacy PG14 restart params must stay in static union: %+v", analysis)
	}
}

func TestBuildConfigPlanRestartParamNextActions(t *testing.T) {
	plan := BuildConfigPlan("pg", []string{"shared_preload_libraries=pg_stat_statements"})

	if !planHasRequiredNextAction(plan, "pig pt restart --pending") {
		t.Fatalf("restart parameter plan must require restart next action, got %+v", plan.NextActions)
	}
	if !planHasNextAction(plan, "pig pt list") {
		t.Fatalf("restart parameter plan must ask user to inspect pt list, got %+v", plan.NextActions)
	}
	if planHasNextAction(plan, "pig pt reload") {
		t.Fatalf("restart parameter plan must not suggest reload as the primary next action, got %+v", plan.NextActions)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func planHasNextAction(plan *output.Plan, needle string) bool {
	for _, action := range plan.NextActions {
		if strings.Contains(action.Command, needle) || strings.Contains(action.Reason, needle) {
			return true
		}
	}
	return false
}

func planHasRequiredNextAction(plan *output.Plan, needle string) bool {
	for _, action := range plan.NextActions {
		if action.Required && (strings.Contains(action.Command, needle) || strings.Contains(action.Reason, needle)) {
			return true
		}
	}
	return false
}
