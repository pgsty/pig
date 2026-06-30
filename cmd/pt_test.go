package cmd

import (
	"encoding/json"
	"errors"
	"github.com/spf13/cobra"
	"io"
	"os"
	"pig/cli/patroni"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"
	"strings"
	"testing"
)

func TestPatroniLogRejectsExtraArgs(t *testing.T) {
	if patroniLogCmd.Args == nil {
		t.Fatal("pt log should validate positional arguments")
	}
	if err := patroniLogCmd.Args(patroniLogCmd, []string{"bogus"}); err == nil {
		t.Fatal("expected pt log to reject unexpected positional argument")
	}
}

func TestPatroniRestartRejectsExtraPositionals(t *testing.T) {
	if patroniRestartCmd.Args == nil {
		t.Fatal("restart command must validate positional argument count")
	}

	if err := patroniRestartCmd.Args(patroniRestartCmd, nil); err != nil {
		t.Fatalf("restart with no member should be accepted: %v", err)
	}
	if err := patroniRestartCmd.Args(patroniRestartCmd, []string{"pg-nms-1"}); err != nil {
		t.Fatalf("restart with one member should be accepted: %v", err)
	}
	if err := patroniRestartCmd.Args(patroniRestartCmd, []string{"pg-nms", "pg-nms-1"}); err == nil {
		t.Fatal("restart must reject cluster+member positionals instead of silently dropping the second argument")
	}
}

func TestPatroniClusterCommandsRejectIgnoredPositionals(t *testing.T) {
	tests := []struct {
		name string
		cmd  *cobra.Command
	}{
		{name: "reload", cmd: patroniReloadCmd},
		{name: "switchover", cmd: patroniSwitchoverCmd},
		{name: "failover", cmd: patroniFailoverCmd},
		{name: "pause", cmd: patroniPauseCmd},
		{name: "resume", cmd: patroniResumeCmd},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.cmd.Args == nil {
				t.Fatalf("%s command must validate positional argument count", tt.name)
			}
			if err := tt.cmd.Args(tt.cmd, []string{"ignored"}); err == nil {
				t.Fatalf("%s should reject unexpected positional args", tt.name)
			}
		})
	}
}

func TestPatroniListAcceptsOptionalCluster(t *testing.T) {
	if patroniListCmd.Args == nil {
		t.Fatal("list command must validate positional argument count")
	}
	if err := patroniListCmd.Args(patroniListCmd, nil); err != nil {
		t.Fatalf("list without cluster should be accepted: %v", err)
	}
	if err := patroniListCmd.Args(patroniListCmd, []string{"pg-meta"}); err != nil {
		t.Fatalf("list with one cluster should be accepted: %v", err)
	}
	if err := patroniListCmd.Args(patroniListCmd, []string{"pg-meta", "pg-test"}); err == nil {
		t.Fatal("list should reject more than one cluster positional")
	}
}

func TestPatroniConfigPgPlanJSONContainsDiffAndNextActions(t *testing.T) {
	origFormat := config.OutputFormat
	origPlan := patroniConfigPlan
	defer func() {
		config.OutputFormat = origFormat
		patroniConfigPlan = origPlan
	}()

	config.OutputFormat = config.OUTPUT_JSON
	patroniConfigPlan = true

	raw := capturePtStdout(t, func() {
		if err := patroniConfigCmd.RunE(patroniConfigCmd, []string{"pg", "max_connections=200"}); err != nil {
			t.Fatalf("pt config pg --plan should not execute or fail: %v", err)
		}
	})

	var plan output.Plan
	if err := json.Unmarshal(ptBytesTrimSpace([]byte(raw)), &plan); err != nil {
		t.Fatalf("invalid plan json: %v raw=%q", err, raw)
	}
	if plan.Boundary != "pt:dcs-config" {
		t.Fatalf("boundary = %q, want pt:dcs-config", plan.Boundary)
	}
	if len(plan.Preconditions) == 0 || !strings.Contains(plan.Preconditions[0].Detail, "max_connections=200") {
		t.Fatalf("expected config diff in preconditions, got %+v", plan.Preconditions)
	}
	if !ptPlanHasNextAction(plan, "pig pt reload") {
		t.Fatalf("expected reload next action, got %+v", plan.NextActions)
	}
}

func TestPatroniConfigPlanRequiresKeyValuePairs(t *testing.T) {
	origFormat := config.OutputFormat
	origPlan := patroniConfigPlan
	defer func() {
		config.OutputFormat = origFormat
		patroniConfigPlan = origPlan
	}()

	config.OutputFormat = config.OUTPUT_JSON
	patroniConfigPlan = true

	var runErr error
	raw := capturePtStdout(t, func() {
		runErr = patroniConfigCmd.RunE(patroniConfigCmd, []string{"pg"})
	})
	if runErr == nil {
		t.Fatal("pt config pg --plan without key=value should fail")
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(ptBytesTrimSpace([]byte(raw)), &payload); err != nil {
		t.Fatalf("invalid json output: %v raw=%q", err, raw)
	}
	if success, _ := payload["success"].(bool); success {
		t.Fatalf("expected success=false, got %v", payload)
	}
	if !strings.Contains(ptAsString(payload["detail"]), "key=value") {
		t.Fatalf("expected detail to mention key=value, got %v", payload)
	}
}

func TestPatroniConfigInvalidActionStructuredError(t *testing.T) {
	origFormat := config.OutputFormat
	defer func() {
		config.OutputFormat = origFormat
	}()

	config.OutputFormat = config.OUTPUT_JSON

	err := patroniConfigCmd.RunE(patroniConfigCmd, []string{"invalid"})
	if err == nil {
		t.Fatal("expected structured error for invalid config action")
	}

	var exitErr *utils.ExitCodeError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitCodeError, got %T: %v", err, err)
	}
	if exitErr.Code != output.ExitCode(output.CodePtInvalidConfigAction) {
		t.Fatalf("unexpected exit code: got %d, want %d",
			exitErr.Code, output.ExitCode(output.CodePtInvalidConfigAction))
	}
}

func TestPatroniSwitchoverFailoverNextActionsPreserveTargets(t *testing.T) {
	sw := patroni.BuildSwitchoverPlan(&patroni.SwitchoverOptions{
		Leader:    "pg-a",
		Candidate: "pg-b",
		Scheduled: "2026-07-01T12:00:00",
	})
	if !ptPlanHasNextAction(*sw, "--candidate pg-b") || !ptPlanHasNextAction(*sw, "--leader pg-a") {
		t.Fatalf("switchover next_actions should preserve target flags: %+v", sw.NextActions)
	}

	fo := patroni.BuildFailoverPlan(&patroni.FailoverOptions{Candidate: "pg-c"})
	if !ptPlanHasNextAction(*fo, "--candidate pg-c") {
		t.Fatalf("failover next_actions should preserve candidate: %+v", fo.NextActions)
	}
}

func ptPlanHasNextAction(plan output.Plan, needle string) bool {
	for _, action := range plan.NextActions {
		if strings.Contains(action.Command, needle) || strings.Contains(action.Reason, needle) {
			return true
		}
	}
	return false
}

var ptTestCommand = patroniCmd

func capturePtStdout(t *testing.T, fn func()) string {
	t.Helper()
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe error: %v", err)
	}
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = origStdout
	raw, _ := io.ReadAll(r)
	_ = r.Close()
	return string(raw)
}

func ptBytesTrimSpace(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}

func ptAsString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
