package cmd

import (
	"encoding/json"
	"errors"
	"github.com/spf13/cobra"
	"io"
	"os"
	"path/filepath"
	"pig/cli/patroni"
	"pig/internal/ancs"
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

func TestPatroniLogCommandsExposeLocalFileAPI(t *testing.T) {
	if patroniLogCmd.RunE == nil {
		t.Fatal("pt log should support a default recent-log action")
	}
	if flag := lookupLocalOrPersistentFlag(patroniLogCmd, "lines"); flag == nil || flag.Shorthand != "n" {
		t.Fatal("pt log should expose -n/--lines on the parent command")
	}
	if flag := patroniLogCmd.Flags().Lookup("follow"); flag == nil || flag.Shorthand != "f" {
		t.Fatal("pt log should expose -f/--follow on the parent command")
	}
	if flag := lookupLocalOrPersistentFlag(patroniLogCmd, "log-dir"); flag == nil {
		t.Fatal("pt log should expose --log-dir")
	}
	for _, sub := range []string{"show", "tail", "grep"} {
		if found, _, err := patroniLogCmd.Find([]string{sub}); err != nil || found == patroniLogCmd {
			t.Fatalf("pt log should expose %q subcommand, found=%v err=%v", sub, found, err)
		}
	}
	if found, _, err := patroniLogCmd.Find([]string{"list"}); err == nil && found != patroniLogCmd {
		t.Fatalf("pt log should not expose multi-file list command, found=%v", found)
	}
	if found, _, err := patroniLogCmd.Find([]string{"cat"}); err != nil || found == patroniLogCmd {
		t.Fatalf("pt log should keep cat as a compatibility alias, found=%v err=%v", found, err)
	}
	if patroniLogCatCmd.Use != "show" {
		t.Fatalf("pt log show use = %q, want show", patroniLogCatCmd.Use)
	}
	if patroniLogCatCmd.Args == nil || patroniLogCatCmd.Args(patroniLogCatCmd, []string{"patroni.log.1"}) == nil {
		t.Fatal("pt log show should only read patroni.log and reject file args")
	}
	if patroniLogTailCmd.Use != "tail" {
		t.Fatalf("pt log tail use = %q, want tail", patroniLogTailCmd.Use)
	}
	if patroniLogTailCmd.Args == nil || patroniLogTailCmd.Args(patroniLogTailCmd, []string{"patroni.log.1"}) == nil {
		t.Fatal("pt log tail should only read patroni.log and reject file args")
	}
	if patroniLogGrepCmd.Use != "grep <pattern>" {
		t.Fatalf("pt log grep use = %q, want grep <pattern>", patroniLogGrepCmd.Use)
	}
	if patroniLogGrepCmd.Args == nil || patroniLogGrepCmd.Args(patroniLogGrepCmd, []string{"ERROR", "patroni.log.1"}) == nil {
		t.Fatal("pt log grep should only search patroni.log and reject file args")
	}
	if flag := patroniLogGrepCmd.Flags().Lookup("lines"); flag == nil || flag.Shorthand != "n" {
		t.Fatal("pt log grep should reuse -n/--lines as the optional search range")
	}
}

func TestPatroniLogGrepNoMatchSilencesCobraError(t *testing.T) {
	origUser := config.CurrentUser
	origFormat := config.OutputFormat
	origDBSU := patroniDBSU
	origLogDir := patroniLogDir
	origSilenceErrors := patroniLogGrepCmd.SilenceErrors
	origSilenceUsage := patroniLogGrepCmd.SilenceUsage
	defer func() {
		config.CurrentUser = origUser
		config.OutputFormat = origFormat
		patroniDBSU = origDBSU
		patroniLogDir = origLogDir
		patroniLogGrepCmd.SilenceErrors = origSilenceErrors
		patroniLogGrepCmd.SilenceUsage = origSilenceUsage
	}()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "patroni.log"), []byte("INFO: startup complete\n"), 0644); err != nil {
		t.Fatalf("write patroni log: %v", err)
	}

	config.CurrentUser = "postgres"
	config.OutputFormat = config.OUTPUT_TEXT
	patroniDBSU = "postgres"
	patroniLogDir = dir
	patroniLogGrepCmd.SilenceErrors = false
	patroniLogGrepCmd.SilenceUsage = false

	err := patroniLogGrepCmd.RunE(patroniLogGrepCmd, []string{"ERROR"})
	var exitErr *utils.ExitCodeError
	if !errors.As(err, &exitErr) {
		t.Fatalf("pt log grep returned %T, want ExitCodeError", err)
	}
	if exitErr.Code != 1 || !exitErr.Silent {
		t.Fatalf("pt log grep no-match exit = code %d silent %v, want code 1 silent true", exitErr.Code, exitErr.Silent)
	}
	if !patroniLogGrepCmd.SilenceErrors {
		t.Fatal("pt log grep no-match should silence Cobra error printing")
	}
	if !patroniLogGrepCmd.SilenceUsage {
		t.Fatal("pt log grep no-match should silence Cobra usage printing")
	}

	if err := patroniLogGrepCmd.Args(patroniLogGrepCmd, nil); err == nil {
		t.Fatal("pt log grep without pattern should still reject arguments")
	}
	if patroniLogGrepCmd.SilenceErrors || patroniLogGrepCmd.SilenceUsage {
		t.Fatal("pt log grep argument validation should reset silent no-match flags")
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

func TestPatroniFailoverAcceptsCandidatePositional(t *testing.T) {
	if patroniFailoverCmd.Args == nil {
		t.Fatal("failover command must validate positional argument count")
	}
	if err := patroniFailoverCmd.Args(patroniFailoverCmd, []string{"pg-nms-2"}); err != nil {
		t.Fatalf("failover should accept one candidate positional: %v", err)
	}
	if err := patroniFailoverCmd.Args(patroniFailoverCmd, []string{"pg-nms-2", "pg-nms-3"}); err == nil {
		t.Fatal("failover must reject multiple candidate positionals")
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
	if !ptPlanHasNextAction(plan, "pig pt restart --pending") {
		t.Fatalf("expected restart next action, got %+v", plan.NextActions)
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

func TestPatroniConfigRejectsNonKeyValueArgs(t *testing.T) {
	origFormat := config.OutputFormat
	origPlan := patroniConfigPlan
	defer func() {
		config.OutputFormat = origFormat
		patroniConfigPlan = origPlan
	}()
	patroniConfigPlan = false

	// Text mode: typo (space instead of '=') must fail naming the offending tokens,
	// never partially apply the valid pairs.
	config.OutputFormat = config.OUTPUT_TEXT
	err := patroniConfigCmd.RunE(patroniConfigCmd, []string{"pg", "shared_buffers=4GB", "work_mem", "256MB"})
	if err == nil {
		t.Fatal("pt config pg with non key=value args should fail")
	}
	for _, tok := range []string{"work_mem", "256MB"} {
		if !strings.Contains(err.Error(), tok) {
			t.Fatalf("error should name invalid token %q, got: %v", tok, err)
		}
	}

	// Structured mode: same rejection as a Fail result.
	config.OutputFormat = config.OUTPUT_JSON
	var runErr error
	raw := capturePtStdout(t, func() {
		runErr = patroniConfigCmd.RunE(patroniConfigCmd, []string{"set", "ttl=60", "loopwait"})
	})
	if runErr == nil {
		t.Fatal("structured pt config set with invalid args should fail")
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(ptBytesTrimSpace([]byte(raw)), &payload); err != nil {
		t.Fatalf("invalid json output: %v raw=%q", err, raw)
	}
	if success, _ := payload["success"].(bool); success {
		t.Fatalf("expected success=false, got %v", payload)
	}
	if !strings.Contains(ptAsString(payload["detail"]), "loopwait") {
		t.Fatalf("expected detail to name invalid token, got %v", payload)
	}
}

func TestPatroniConfigPgTextPrintsRestartNextActions(t *testing.T) {
	origFormat := config.OutputFormat
	origPlan := patroniConfigPlan
	origExec := patroniConfigPGExec
	defer func() {
		config.OutputFormat = origFormat
		patroniConfigPlan = origPlan
		patroniConfigPGExec = origExec
	}()
	config.OutputFormat = config.OUTPUT_TEXT
	patroniConfigPlan = false

	var gotPairs []string
	patroniConfigPGExec = func(dbsu string, pairs []string) error {
		gotPairs = append([]string(nil), pairs...)
		return nil
	}

	var runErr error
	stderr := capturePtStderr(t, func() {
		runErr = patroniConfigCmd.RunE(patroniConfigCmd, []string{"pg", "shared_preload_libraries=pg_stat_statements"})
	})
	if runErr != nil {
		t.Fatalf("pt config pg should execute with stubbed patronictl: %v", runErr)
	}
	if len(gotPairs) != 1 || gotPairs[0] != "shared_preload_libraries=pg_stat_statements" {
		t.Fatalf("patroni config pg pairs = %v", gotPairs)
	}
	for _, want := range []string{
		"requires PostgreSQL restart",
		"shared_preload_libraries",
		"pig pt list",
		"pig pt restart --pending",
	} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("stderr should contain %q, got:\n%s", want, stderr)
		}
	}
	if strings.Contains(stderr, "pig pt reload") {
		t.Fatalf("restart parameter hint must not suggest pt reload, got:\n%s", stderr)
	}
}

func TestPatroniConfigPgStructuredIncludesRestartNextActions(t *testing.T) {
	origFormat := config.OutputFormat
	origPlan := patroniConfigPlan
	origExec := patroniConfigPGExec
	defer func() {
		config.OutputFormat = origFormat
		patroniConfigPlan = origPlan
		patroniConfigPGExec = origExec
	}()
	config.OutputFormat = config.OUTPUT_JSON
	patroniConfigPlan = false

	patroniConfigPGExec = func(dbsu string, pairs []string) error {
		return nil
	}

	var runErr error
	raw := capturePtStdout(t, func() {
		runErr = patroniConfigCmd.RunE(patroniConfigCmd, []string{"pg", "shared_buffers=4GB"})
	})
	if runErr != nil {
		t.Fatalf("pt config pg structured should execute with stubbed patronictl: %v", runErr)
	}
	var payload output.Result
	if err := json.Unmarshal(ptBytesTrimSpace([]byte(raw)), &payload); err != nil {
		t.Fatalf("invalid json output: %v raw=%q", err, raw)
	}
	if !payload.Success {
		t.Fatalf("expected success=true, got %+v", payload)
	}
	if !resultHasRequiredNextAction(payload, "pig pt restart --pending") {
		t.Fatalf("structured restart parameter result must require restart next action, got %+v", payload.NextActions)
	}
	if !resultHasNextAction(payload, "pig pt list") {
		t.Fatalf("structured restart parameter result must include pt list next action, got %+v", payload.NextActions)
	}
}

func TestPatroniConfigPgStructuredFailureOmitsRestartNextActions(t *testing.T) {
	origFormat := config.OutputFormat
	origPlan := patroniConfigPlan
	origExec := patroniConfigPGExec
	defer func() {
		config.OutputFormat = origFormat
		patroniConfigPlan = origPlan
		patroniConfigPGExec = origExec
	}()
	config.OutputFormat = config.OUTPUT_JSON
	patroniConfigPlan = false

	patroniConfigPGExec = func(dbsu string, pairs []string) error {
		return errors.New("patronictl edit-config failed")
	}

	var runErr error
	raw := capturePtStdout(t, func() {
		runErr = patroniConfigCmd.RunE(patroniConfigCmd, []string{"pg", "shared_buffers=4GB"})
	})
	if runErr == nil {
		t.Fatal("pt config pg structured should fail when patronictl fails")
	}
	var payload output.Result
	if err := json.Unmarshal(ptBytesTrimSpace([]byte(raw)), &payload); err != nil {
		t.Fatalf("invalid json output: %v raw=%q", err, raw)
	}
	if payload.Success {
		t.Fatalf("expected success=false, got %+v", payload)
	}
	if len(payload.NextActions) != 0 {
		t.Fatalf("failed config change must not suggest restart next actions, got %+v", payload.NextActions)
	}
}

func TestPatroniConfigHelpDocumentsComplexParameters(t *testing.T) {
	for _, want := range []string{
		"shared_preload_libraries",
		"log_min_duration_statement",
		"postmaster",
		"pig pt restart --pending",
	} {
		if !strings.Contains(patroniConfigCmd.Long+patroniConfigCmd.Example, want) {
			t.Fatalf("pt config help should mention %q\nLong:\n%s\nExample:\n%s", want, patroniConfigCmd.Long, patroniConfigCmd.Example)
		}
	}
}

func TestSplitConfigKVPairs(t *testing.T) {
	pairs, invalid := splitConfigKVPairs([]string{"a=1", "b", "c=3", ""})
	if len(pairs) != 2 || pairs[0] != "a=1" || pairs[1] != "c=3" {
		t.Fatalf("unexpected pairs: %v", pairs)
	}
	if len(invalid) != 2 || invalid[0] != "b" || invalid[1] != "" {
		t.Fatalf("unexpected invalid tokens: %v", invalid)
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

func TestPatroniConfigInvalidActionTextHelpIsSilentError(t *testing.T) {
	origFormat := config.OutputFormat
	origOut := patroniConfigCmd.OutOrStdout()
	origErr := patroniConfigCmd.ErrOrStderr()
	origSilenceUsage := patroniConfigCmd.SilenceUsage
	origSilenceErrors := patroniConfigCmd.SilenceErrors
	defer func() {
		config.OutputFormat = origFormat
		patroniConfigCmd.SetOut(origOut)
		patroniConfigCmd.SetErr(origErr)
		patroniConfigCmd.SilenceUsage = origSilenceUsage
		patroniConfigCmd.SilenceErrors = origSilenceErrors
	}()

	config.OutputFormat = config.OUTPUT_TEXT
	patroniConfigCmd.SetOut(io.Discard)
	patroniConfigCmd.SetErr(io.Discard)
	patroniConfigCmd.SilenceUsage = false
	patroniConfigCmd.SilenceErrors = false

	err := patroniConfigCmd.RunE(patroniConfigCmd, []string{"invalid"})
	if err == nil {
		t.Fatal("expected text error for invalid config action")
	}

	var exitErr *utils.ExitCodeError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitCodeError, got %T: %v", err, err)
	}
	if exitErr.Code != output.ExitCode(output.CodePtInvalidConfigAction) {
		t.Fatalf("unexpected exit code: got %d, want %d",
			exitErr.Code, output.ExitCode(output.CodePtInvalidConfigAction))
	}
	if !exitErr.Silent {
		t.Fatal("invalid-action help path should return a silent exit after printing help")
	}
	if !patroniConfigCmd.SilenceUsage || !patroniConfigCmd.SilenceErrors {
		t.Fatalf("invalid-action help path should silence Cobra duplicate output, got usage=%v errors=%v",
			patroniConfigCmd.SilenceUsage, patroniConfigCmd.SilenceErrors)
	}
}

func TestPatroniStructuredNeedYesGate(t *testing.T) {
	origFormat := config.OutputFormat
	defer func() {
		config.OutputFormat = origFormat
	}()
	config.OutputFormat = config.OUTPUT_JSON

	var runErr error
	raw := capturePtStdout(t, func() {
		runErr = requirePtClusterConfirmation(false, "restart", "high",
			"This will rolling-restart PostgreSQL on ALL cluster members",
			"pig pt restart --yes", "pig pt restart --plan")
	})
	if runErr == nil {
		t.Fatal("structured Patroni command without --yes should fail")
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(ptBytesTrimSpace([]byte(raw)), &payload); err != nil {
		t.Fatalf("invalid json output: %v raw=%q", err, raw)
	}
	if success, _ := payload["success"].(bool); success {
		t.Fatalf("expected success=false, got %v", payload)
	}
	if code, _ := payload["code"].(float64); int(code) != output.CodePtConfirmationRequired {
		t.Fatalf("code = %v, want %d", payload["code"], output.CodePtConfirmationRequired)
	}
	if msg, _ := payload["message"].(string); !strings.Contains(msg, "--yes (-y)") {
		t.Fatalf("gate message must reference --yes (-y), got %q", msg)
	}
	// Refusal next_actions must carry real replayable commands, no placeholders.
	actions, _ := payload["next_actions"].([]interface{})
	if len(actions) < 2 {
		t.Fatalf("refusal should carry execute and plan next actions, got %v", payload["next_actions"])
	}
	first, _ := actions[0].(map[string]interface{})
	if cmd := ptAsString(first["command"]); cmd != "pig pt restart --yes" {
		t.Fatalf("execute next action = %q, want replayable command", cmd)
	}
}

func TestPatroniRestartReinitStructuredRunERequiresYes(t *testing.T) {
	origFormat := config.OutputFormat
	origPlan := patroniPlan
	defer func() {
		config.OutputFormat = origFormat
		patroniPlan = origPlan
		_ = patroniRestartCmd.Flags().Set("yes", "false")
		_ = patroniReinitCmd.Flags().Set("yes", "false")
	}()
	config.OutputFormat = config.OUTPUT_JSON
	patroniPlan = false

	tests := []struct {
		name string
		cmd  *cobra.Command
		args []string
		code int
	}{
		{name: "restart cluster-wide", cmd: patroniRestartCmd, args: nil, code: output.CodePtConfirmationRequired},
		{name: "reinit", cmd: patroniReinitCmd, args: []string{"pg1"}, code: output.CodePtConfirmationRequired},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = tt.cmd.Flags().Set("yes", "false")
			var runErr error
			raw := capturePtStdout(t, func() {
				runErr = tt.cmd.RunE(tt.cmd, tt.args)
			})
			if runErr == nil {
				t.Fatalf("%s without --yes should fail in structured mode", tt.name)
			}
			var payload map[string]interface{}
			if err := json.Unmarshal(ptBytesTrimSpace([]byte(raw)), &payload); err != nil {
				t.Fatalf("invalid json output: %v raw=%q", err, raw)
			}
			if code, _ := payload["code"].(float64); int(code) != tt.code {
				t.Fatalf("code = %v, want %d", payload["code"], tt.code)
			}
		})
	}
}

// TestPatroniRestartMemberBranchSkipsGate (D2): an explicit member argument
// executes directly in both output modes — no gate, no prompt — while
// patronictl always receives --force (B04).
func TestPatroniRestartMemberBranchSkipsGate(t *testing.T) {
	origFormat := config.OutputFormat
	origExec := patroniRestartExec
	origConfirm := highRiskTextConfirm
	defer func() {
		config.OutputFormat = origFormat
		patroniRestartExec = origExec
		highRiskTextConfirm = origConfirm
		_ = patroniRestartCmd.Flags().Set("yes", "false")
	}()

	confirmCalled := false
	highRiskTextConfirm = func(warning, action string) error {
		confirmCalled = true
		return nil
	}

	for _, mode := range []string{config.OUTPUT_TEXT, config.OUTPUT_JSON} {
		t.Run(mode, func(t *testing.T) {
			config.OutputFormat = mode
			confirmCalled = false
			var gotOpts *patroni.RestartOptions
			patroniRestartExec = func(dbsu string, opts *patroni.RestartOptions) error {
				gotOpts = opts
				return nil
			}
			var runErr error
			_ = capturePtStdout(t, func() {
				runErr = patroniRestartCmd.RunE(patroniRestartCmd, []string{"pg-test-1"})
			})
			if runErr != nil {
				t.Fatalf("restart with explicit member should execute directly: %v", runErr)
			}
			if confirmCalled {
				t.Fatal("restart with explicit member must not prompt for confirmation")
			}
			if gotOpts == nil || gotOpts.Member != "pg-test-1" {
				t.Fatalf("restart should execute against member pg-test-1, got %+v", gotOpts)
			}
			if !gotOpts.Force {
				t.Fatal("patronictl must always receive --force (B04)")
			}
		})
	}
}

// TestPatroniClusterWideRestartTextConfirm (D2): cluster-wide rolling restart
// in text mode is T2 — it asks for confirmation unless --yes.
func TestPatroniClusterWideRestartTextConfirm(t *testing.T) {
	origFormat := config.OutputFormat
	origExec := patroniRestartExec
	origConfirm := highRiskTextConfirm
	defer func() {
		config.OutputFormat = origFormat
		patroniRestartExec = origExec
		highRiskTextConfirm = origConfirm
		_ = patroniRestartCmd.Flags().Set("yes", "false")
	}()
	config.OutputFormat = config.OUTPUT_TEXT

	execCalled := false
	patroniRestartExec = func(dbsu string, opts *patroni.RestartOptions) error {
		execCalled = true
		return nil
	}

	// Confirmation rejected: nothing executes.
	var gotWarning string
	highRiskTextConfirm = func(warning, action string) error {
		gotWarning = warning
		return errors.New("cluster-wide restart cancelled by user")
	}
	_ = patroniRestartCmd.Flags().Set("yes", "false")
	if err := patroniRestartCmd.RunE(patroniRestartCmd, nil); err == nil {
		t.Fatal("rejected confirmation must abort cluster-wide restart")
	}
	if execCalled {
		t.Fatal("patronictl restart must not run after rejected confirmation")
	}
	if !strings.Contains(gotWarning, "ALL cluster members") {
		t.Fatalf("warning should mention all cluster members, got %q", gotWarning)
	}

	// --yes skips the prompt and executes with patronictl --force.
	highRiskTextConfirm = func(warning, action string) error {
		t.Fatal("--yes must skip the confirmation prompt")
		return nil
	}
	var gotOpts *patroni.RestartOptions
	patroniRestartExec = func(dbsu string, opts *patroni.RestartOptions) error {
		gotOpts = opts
		return nil
	}
	_ = patroniRestartCmd.Flags().Set("yes", "true")
	if err := patroniRestartCmd.RunE(patroniRestartCmd, nil); err != nil {
		t.Fatalf("cluster-wide restart with --yes should execute: %v", err)
	}
	if gotOpts == nil || !gotOpts.Force {
		t.Fatalf("patronictl must always receive --force (B04), got %+v", gotOpts)
	}
}

// TestPatroniTextConfirmClusterOps (B04): text-mode reinit/switchover/failover
// prompt at the pig layer and, once confirmed, execute with patronictl --force
// so patronictl never prompts.
func TestPatroniTextConfirmClusterOps(t *testing.T) {
	origFormat := config.OutputFormat
	origPlan := patroniPlan
	origConfirm := highRiskTextConfirm
	origReinit := patroniReinitExec
	origSwitchover := patroniSwitchoverExec
	origFailover := patroniFailoverExec
	origPreflight := patroniSwitchPreflight
	defer func() {
		config.OutputFormat = origFormat
		patroniPlan = origPlan
		highRiskTextConfirm = origConfirm
		patroniReinitExec = origReinit
		patroniSwitchoverExec = origSwitchover
		patroniFailoverExec = origFailover
		patroniSwitchPreflight = origPreflight
		_ = patroniReinitCmd.Flags().Set("yes", "false")
		_ = patroniSwitchoverCmd.Flags().Set("yes", "false")
		_ = patroniFailoverCmd.Flags().Set("yes", "false")
		_ = patroniFailoverCmd.Flags().Set("candidate", "")
	}()
	config.OutputFormat = config.OUTPUT_TEXT
	patroniPlan = false
	patroniSwitchPreflight = func(dbsu string) (*patroni.SwitchPreflight, *output.Result) {
		return &patroni.SwitchPreflight{
			Cluster:    "pg-test",
			Leader:     "pg-test-1",
			Candidates: []string{"pg-test-2"},
		}, nil
	}

	var forcedToPatronictl bool
	patroniReinitExec = func(dbsu string, opts *patroni.ReinitOptions) error {
		forcedToPatronictl = opts != nil && opts.Force
		return nil
	}
	patroniSwitchoverExec = func(dbsu string, opts *patroni.SwitchoverOptions) error {
		forcedToPatronictl = opts != nil && opts.Force
		return nil
	}
	patroniFailoverExec = func(dbsu string, opts *patroni.FailoverOptions) error {
		forcedToPatronictl = opts != nil && opts.Force
		return nil
	}

	tests := []struct {
		name    string
		cmd     *cobra.Command
		args    []string
		flags   map[string]string
		warning string
	}{
		{name: "reinit", cmd: patroniReinitCmd, args: []string{"pg-test-2"}, warning: "WIPE and rebuild member pg-test-2"},
		{name: "switchover", cmd: patroniSwitchoverCmd, args: nil, warning: "leadership will transfer"},
		{name: "failover", cmd: patroniFailoverCmd, args: nil,
			flags: map[string]string{"candidate": "pg-test-2"}, warning: "data loss possible"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Rejected confirmation aborts before execution.
			forcedToPatronictl = false
			var gotWarning string
			highRiskTextConfirm = func(warning, action string) error {
				gotWarning = warning
				return errors.New(tt.name + " cancelled by user")
			}
			_ = tt.cmd.Flags().Set("yes", "false")
			for k, v := range tt.flags {
				_ = tt.cmd.Flags().Set(k, v)
			}
			if err := tt.cmd.RunE(tt.cmd, tt.args); err == nil {
				t.Fatalf("%s must abort on rejected confirmation", tt.name)
			}
			if forcedToPatronictl {
				t.Fatalf("%s must not execute after rejected confirmation", tt.name)
			}
			if !strings.Contains(gotWarning, tt.warning) {
				t.Fatalf("%s warning %q should contain %q", tt.name, gotWarning, tt.warning)
			}

			// Accepted confirmation executes with patronictl --force.
			highRiskTextConfirm = func(warning, action string) error { return nil }
			if err := tt.cmd.RunE(tt.cmd, tt.args); err != nil {
				t.Fatalf("%s should execute after accepted confirmation: %v", tt.name, err)
			}
			if !forcedToPatronictl {
				t.Fatalf("%s must pass --force to patronictl after pig-level confirmation (B04)", tt.name)
			}
		})
	}
}

func TestPatroniSwitchPreflightBlocksPausedClusterBeforeConfirm(t *testing.T) {
	origFormat := config.OutputFormat
	origPlan := patroniPlan
	origConfirm := highRiskTextConfirm
	origPreflight := patroniSwitchPreflight
	origSwitchover := patroniSwitchoverExec
	origFailover := patroniFailoverExec
	defer func() {
		config.OutputFormat = origFormat
		patroniPlan = origPlan
		highRiskTextConfirm = origConfirm
		patroniSwitchPreflight = origPreflight
		patroniSwitchoverExec = origSwitchover
		patroniFailoverExec = origFailover
		_ = patroniSwitchoverCmd.Flags().Set("yes", "false")
		_ = patroniFailoverCmd.Flags().Set("yes", "false")
		_ = patroniFailoverCmd.Flags().Set("candidate", "")
	}()
	config.OutputFormat = config.OUTPUT_TEXT
	patroniPlan = false
	patroniSwitchPreflight = func(dbsu string) (*patroni.SwitchPreflight, *output.Result) {
		return &patroni.SwitchPreflight{
			Cluster:    "pg-test",
			Leader:     "pg-test-1",
			Candidates: []string{"pg-test-2"},
			Paused:     true,
		}, nil
	}
	highRiskTextConfirm = func(warning, action string) error {
		t.Fatal("paused cluster must fail before asking for confirmation")
		return nil
	}
	patroniSwitchoverExec = func(dbsu string, opts *patroni.SwitchoverOptions) error {
		t.Fatal("paused cluster must not run switchover")
		return nil
	}
	patroniFailoverExec = func(dbsu string, opts *patroni.FailoverOptions) error {
		t.Fatal("paused cluster must not run failover")
		return nil
	}

	if err := patroniSwitchoverCmd.RunE(patroniSwitchoverCmd, nil); err == nil || !strings.Contains(err.Error(), "pig pt resume") {
		t.Fatalf("paused switchover should mention pig pt resume, got %v", err)
	}
	_ = patroniFailoverCmd.Flags().Set("candidate", "pg-test-2")
	if err := patroniFailoverCmd.RunE(patroniFailoverCmd, nil); err == nil || !strings.Contains(err.Error(), "pig pt resume") {
		t.Fatalf("paused failover should mention pig pt resume, got %v", err)
	}
}

func TestPatroniSwitchConfirmationUsesClusterTopology(t *testing.T) {
	origFormat := config.OutputFormat
	origPlan := patroniPlan
	origConfirm := highRiskTextConfirm
	origPreflight := patroniSwitchPreflight
	origSwitchover := patroniSwitchoverExec
	origFailover := patroniFailoverExec
	defer func() {
		config.OutputFormat = origFormat
		patroniPlan = origPlan
		highRiskTextConfirm = origConfirm
		patroniSwitchPreflight = origPreflight
		patroniSwitchoverExec = origSwitchover
		patroniFailoverExec = origFailover
		_ = patroniSwitchoverCmd.Flags().Set("yes", "false")
		_ = patroniSwitchoverCmd.Flags().Set("candidate", "")
		_ = patroniFailoverCmd.Flags().Set("yes", "false")
		_ = patroniFailoverCmd.Flags().Set("candidate", "")
	}()
	config.OutputFormat = config.OUTPUT_TEXT
	patroniPlan = false
	patroniSwitchPreflight = func(dbsu string) (*patroni.SwitchPreflight, *output.Result) {
		return &patroni.SwitchPreflight{
			Cluster:    "pg-test",
			Leader:     "pg-test-1",
			Candidates: []string{"pg-test-2", "pg-test-3"},
		}, nil
	}
	patroniSwitchoverExec = func(dbsu string, opts *patroni.SwitchoverOptions) error {
		t.Fatal("confirmation rejection must prevent switchover execution")
		return nil
	}
	patroniFailoverExec = func(dbsu string, opts *patroni.FailoverOptions) error {
		t.Fatal("confirmation rejection must prevent failover execution")
		return nil
	}

	var swWarning string
	highRiskTextConfirm = func(warning, action string) error {
		swWarning = warning
		return errors.New("cancelled")
	}
	if err := patroniSwitchoverCmd.RunE(patroniSwitchoverCmd, nil); err == nil {
		t.Fatal("switchover should stop after rejected confirmation")
	}
	for _, want := range []string{"pg-test", "pg-test-1", "pg-test-2, pg-test-3", "pig pt switchover -c <instance>"} {
		if !strings.Contains(swWarning, want) {
			t.Fatalf("switchover warning %q should contain %q", swWarning, want)
		}
	}

	var foWarning string
	highRiskTextConfirm = func(warning, action string) error {
		foWarning = warning
		return errors.New("cancelled")
	}
	_ = patroniFailoverCmd.Flags().Set("candidate", "pg-test-2")
	if err := patroniFailoverCmd.RunE(patroniFailoverCmd, nil); err == nil {
		t.Fatal("failover should stop after rejected confirmation")
	}
	for _, want := range []string{"pg-test", "pg-test-1", "pg-test-2", "data loss possible"} {
		if !strings.Contains(foWarning, want) {
			t.Fatalf("failover warning %q should contain %q", foWarning, want)
		}
	}
}

func TestPatroniSwitchPausedStructuredResultCarriesResumeAction(t *testing.T) {
	origFormat := config.OutputFormat
	origPlan := patroniPlan
	origPreflight := patroniSwitchPreflight
	origSwitchover := patroniSwitchoverExec
	defer func() {
		config.OutputFormat = origFormat
		patroniPlan = origPlan
		patroniSwitchPreflight = origPreflight
		patroniSwitchoverExec = origSwitchover
		_ = patroniSwitchoverCmd.Flags().Set("yes", "false")
	}()
	config.OutputFormat = config.OUTPUT_JSON
	patroniPlan = false
	patroniSwitchPreflight = func(dbsu string) (*patroni.SwitchPreflight, *output.Result) {
		return &patroni.SwitchPreflight{
			Cluster:    "pg-test",
			Leader:     "pg-test-1",
			Candidates: []string{"pg-test-2"},
			Paused:     true,
		}, nil
	}
	patroniSwitchoverExec = func(dbsu string, opts *patroni.SwitchoverOptions) error {
		t.Fatal("paused structured switchover must not execute")
		return nil
	}

	var runErr error
	raw := capturePtStdout(t, func() {
		runErr = patroniSwitchoverCmd.RunE(patroniSwitchoverCmd, nil)
	})
	if runErr == nil {
		t.Fatal("paused structured switchover should fail")
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(ptBytesTrimSpace([]byte(raw)), &payload); err != nil {
		t.Fatalf("invalid json output: %v raw=%q", err, raw)
	}
	if code, _ := payload["code"].(float64); int(code) != output.CodePtClusterPaused {
		t.Fatalf("code = %v, want %d", payload["code"], output.CodePtClusterPaused)
	}
	actions, _ := payload["next_actions"].([]interface{})
	if len(actions) == 0 {
		t.Fatalf("paused result should include resume next action, got %v", payload)
	}
	first, _ := actions[0].(map[string]interface{})
	if cmd := ptAsString(first["command"]); cmd != "pig pt resume" {
		t.Fatalf("first next action = %q, want pig pt resume", cmd)
	}
}

func TestPatroniSwitchoverSilentExitSilencesCobraOutput(t *testing.T) {
	origFormat := config.OutputFormat
	origPlan := patroniPlan
	origConfirm := highRiskTextConfirm
	origSwitchover := patroniSwitchoverExec
	origPreflight := patroniSwitchPreflight
	origSilenceErrors := patroniSwitchoverCmd.SilenceErrors
	origSilenceUsage := patroniSwitchoverCmd.SilenceUsage
	defer func() {
		config.OutputFormat = origFormat
		patroniPlan = origPlan
		highRiskTextConfirm = origConfirm
		patroniSwitchoverExec = origSwitchover
		patroniSwitchPreflight = origPreflight
		patroniSwitchoverCmd.SilenceErrors = origSilenceErrors
		patroniSwitchoverCmd.SilenceUsage = origSilenceUsage
		_ = patroniSwitchoverCmd.Flags().Set("yes", "false")
	}()

	config.OutputFormat = config.OUTPUT_TEXT
	patroniPlan = false
	patroniSwitchoverCmd.SilenceErrors = false
	patroniSwitchoverCmd.SilenceUsage = false
	_ = patroniSwitchoverCmd.Flags().Set("yes", "false")
	highRiskTextConfirm = func(warning, action string) error { return nil }
	patroniSwitchPreflight = func(dbsu string) (*patroni.SwitchPreflight, *output.Result) {
		return &patroni.SwitchPreflight{Cluster: "pg-test", Leader: "pg-test-1", Candidates: []string{"pg-test-2"}}, nil
	}
	patroniSwitchoverExec = func(dbsu string, opts *patroni.SwitchoverOptions) error {
		return &utils.ExitCodeError{
			Code:   1,
			Err:    errors.New("exit status 1: Current cluster topology\n+ table row +\nError: No candidates found to switchover to"),
			Silent: true,
		}
	}

	err := patroniSwitchoverCmd.RunE(patroniSwitchoverCmd, nil)
	var exitErr *utils.ExitCodeError
	if !errors.As(err, &exitErr) {
		t.Fatalf("switchover returned %T, want ExitCodeError: %v", err, err)
	}
	if !exitErr.Silent {
		t.Fatalf("switchover exit should stay silent, got %v", err)
	}
	if !patroniSwitchoverCmd.SilenceErrors {
		t.Fatal("silent switchover subprocess failure should silence Cobra error printing")
	}
	if !patroniSwitchoverCmd.SilenceUsage {
		t.Fatal("silent switchover subprocess failure should silence Cobra usage printing")
	}
}

func TestPatroniFailoverSilentExitSilencesCobraOutput(t *testing.T) {
	origFormat := config.OutputFormat
	origPlan := patroniPlan
	origConfirm := highRiskTextConfirm
	origFailover := patroniFailoverExec
	origPreflight := patroniSwitchPreflight
	origSilenceErrors := patroniFailoverCmd.SilenceErrors
	origSilenceUsage := patroniFailoverCmd.SilenceUsage
	defer func() {
		config.OutputFormat = origFormat
		patroniPlan = origPlan
		highRiskTextConfirm = origConfirm
		patroniFailoverExec = origFailover
		patroniSwitchPreflight = origPreflight
		patroniFailoverCmd.SilenceErrors = origSilenceErrors
		patroniFailoverCmd.SilenceUsage = origSilenceUsage
		_ = patroniFailoverCmd.Flags().Set("yes", "false")
		_ = patroniFailoverCmd.Flags().Set("candidate", "")
	}()

	config.OutputFormat = config.OUTPUT_TEXT
	patroniPlan = false
	patroniFailoverCmd.SilenceErrors = false
	patroniFailoverCmd.SilenceUsage = false
	_ = patroniFailoverCmd.Flags().Set("yes", "false")
	_ = patroniFailoverCmd.Flags().Set("candidate", "pg-test-2")
	highRiskTextConfirm = func(warning, action string) error { return nil }
	patroniSwitchPreflight = func(dbsu string) (*patroni.SwitchPreflight, *output.Result) {
		return &patroni.SwitchPreflight{Cluster: "pg-test", Leader: "pg-test-1", Candidates: []string{"pg-test-2"}}, nil
	}
	patroniFailoverExec = func(dbsu string, opts *patroni.FailoverOptions) error {
		return &utils.ExitCodeError{
			Code:   1,
			Err:    errors.New("exit status 1: Current cluster topology\n+ table row +\nError: No candidates found to failover to"),
			Silent: true,
		}
	}

	err := patroniFailoverCmd.RunE(patroniFailoverCmd, nil)
	var exitErr *utils.ExitCodeError
	if !errors.As(err, &exitErr) {
		t.Fatalf("failover returned %T, want ExitCodeError: %v", err, err)
	}
	if !exitErr.Silent {
		t.Fatalf("failover exit should stay silent, got %v", err)
	}
	if !patroniFailoverCmd.SilenceErrors {
		t.Fatal("silent failover subprocess failure should silence Cobra error printing")
	}
	if !patroniFailoverCmd.SilenceUsage {
		t.Fatal("silent failover subprocess failure should silence Cobra usage printing")
	}
}

// TestPatroniStartStopShortcutsRouteToSvc (B03): top-level start/stop stay
// hidden, but execute the same Patroni service action as pt svc start/stop.
func TestPatroniStartStopShortcutsRouteToSvc(t *testing.T) {
	tests := []struct {
		name   string
		cmd    *cobra.Command
		action string
	}{
		{name: "start", cmd: patroniStartCmd, action: "start"},
		{name: "stop", cmd: patroniStopCmd, action: "stop"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := tt.cmd
			if !cmd.Hidden {
				t.Errorf("pt %s shortcut must remain hidden", cmd.Name())
			}
			recordFile := installFakeSystemctl(t)
			var runErr error
			_ = capturePtStdout(t, func() {
				runErr = cmd.RunE(cmd, nil)
			})
			if runErr != nil {
				t.Fatalf("pt %s shortcut should execute pt svc %s: %v", cmd.Name(), tt.action, runErr)
			}
			recorded, err := os.ReadFile(recordFile)
			if err != nil {
				t.Fatalf("read fake systemctl record: %v", err)
			}
			if got, want := strings.TrimSpace(string(recorded)), "systemctl "+tt.action+" patroni"; got != want {
				t.Errorf("pt %s shortcut command = %q, want %q", cmd.Name(), got, want)
			}
		})
	}
}

func TestPatroniStartStopShortcutHelpMentionsSvcTarget(t *testing.T) {
	tests := []struct {
		name   string
		cmd    *cobra.Command
		target string
		alias  string
	}{
		{name: "start", cmd: patroniStartCmd, target: "pig pt svc start", alias: "pig pt up"},
		{name: "stop", cmd: patroniStopCmd, target: "pig pt svc stop", alias: "pig pt dn"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(tt.cmd.Long, tt.target) {
				t.Fatalf("pt %s long help must mention %q, got %q", tt.name, tt.target, tt.cmd.Long)
			}
			if !strings.Contains(tt.cmd.Example, tt.target) {
				t.Fatalf("pt %s examples must mention %q, got %q", tt.name, tt.target, tt.cmd.Example)
			}
			if strings.Contains(tt.cmd.Example, tt.alias) {
				t.Fatalf("pt %s examples must not advertise alias %q, got %q", tt.name, tt.alias, tt.cmd.Example)
			}
		})
	}
}

func TestPatroniRootHelpUsesHabitualSvcExamples(t *testing.T) {
	for _, want := range []string{
		"pig pt svc start",
		"pig pt svc stop",
		"pig pt svc restart",
		"pig pt svc reload",
		"pig pt svc status",
	} {
		if !strings.Contains(patroniCmd.Long, want) {
			t.Fatalf("pt root long help should mention habitual command %q, got:\n%s", want, patroniCmd.Long)
		}
	}
	if strings.Contains(patroniCmd.Long, "pig pt service start") {
		t.Fatalf("pt root long help should keep operator examples on pt svc, got:\n%s", patroniCmd.Long)
	}
}

func TestPatroniAliasesMatchContract(t *testing.T) {
	tests := []struct {
		name string
		cmd  *cobra.Command
		want []string
	}{
		{name: "pt list", cmd: patroniListCmd, want: []string{"ls"}},
		{name: "pt restart", cmd: patroniRestartCmd, want: []string{"rs"}},
		{name: "pt reload", cmd: patroniReloadCmd, want: []string{"rl"}},
		{name: "pt reinit", cmd: patroniReinitCmd, want: []string{"ri"}},
		{name: "pt switchover", cmd: patroniSwitchoverCmd, want: []string{"so"}},
		{name: "pt failover", cmd: patroniFailoverCmd, want: []string{"fo"}},
		{name: "pt pause", cmd: patroniPauseCmd, want: []string{"p"}},
		{name: "pt resume", cmd: patroniResumeCmd, want: []string{"r"}},
		{name: "pt config", cmd: patroniConfigCmd, want: []string{"c"}},
		{name: "pt start", cmd: patroniStartCmd, want: []string{"up"}},
		{name: "pt stop", cmd: patroniStopCmd, want: []string{"dn"}},
		{name: "pt status", cmd: patroniStatusCmd, want: []string{"st"}},
		{name: "pt log", cmd: patroniLogCmd, want: []string{"l"}},
		{name: "pt log show", cmd: patroniLogCatCmd, want: []string{"cat", "c", "s"}},
		{name: "pt log tail", cmd: patroniLogTailCmd, want: []string{"t", "f", "follow"}},
		{name: "pt log grep", cmd: patroniLogGrepCmd, want: []string{"g", "search"}},
		{name: "pt service", cmd: patroniSvcCmd, want: []string{"svc"}},
		{name: "pt service start", cmd: patroniSvcStartCmd, want: []string{"up"}},
		{name: "pt service stop", cmd: patroniSvcStopCmd, want: []string{"dn"}},
		{name: "pt service restart", cmd: patroniSvcRestartCmd, want: []string{"rs"}},
		{name: "pt service reload", cmd: patroniSvcReloadCmd, want: []string{"rl"}},
		{name: "pt service status", cmd: patroniSvcStatusCmd, want: []string{"st"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := strings.Join(tt.cmd.Aliases, ","); got != strings.Join(tt.want, ",") {
				t.Fatalf("%s aliases = %v, want %v", tt.name, tt.cmd.Aliases, tt.want)
			}
		})
	}
}

func TestPatroniServiceSchemaUsesServicePrimaryName(t *testing.T) {
	schema := ancs.FromCommand(patroniSvcCmd)
	if schema == nil {
		t.Fatal("pt service schema should not be nil")
	}
	if schema.Name != "pig patroni service" {
		t.Fatalf("pt service command schema name = %q, want pig patroni service", schema.Name)
	}
	if schema.Use != "service" {
		t.Fatalf("pt service command schema use = %q, want service", schema.Use)
	}
	if schema.Schema == nil || schema.Schema.Name != "pig patroni service" {
		t.Fatalf("pt service annotation schema name = %+v, want pig patroni service", schema.Schema)
	}
}

func TestPatroniSvcAliasResolvesToServiceCommand(t *testing.T) {
	found, remaining, err := patroniCmd.Find([]string{"svc"})
	if err != nil {
		t.Fatalf("pt svc alias should resolve without error: %v", err)
	}
	if found != patroniSvcCmd {
		t.Fatalf("pt svc resolved to %q, want service command", found.CommandPath())
	}
	if len(remaining) != 0 {
		t.Fatalf("pt svc alias should not leave remaining args, got %v", remaining)
	}
	if found.Name() != "service" {
		t.Fatalf("pt svc alias resolved command name = %q, want service", found.Name())
	}
}

func TestPatroniServiceStructuredCommandUsesServicePrimaryName(t *testing.T) {
	origFormat := config.OutputFormat
	defer func() {
		config.OutputFormat = origFormat
	}()
	config.OutputFormat = config.OUTPUT_JSON

	recordFile := installFakeSystemctl(t)
	var runErr error
	raw := capturePtStdout(t, func() {
		runErr = patroniSvcStartCmd.RunE(patroniSvcStartCmd, nil)
	})
	if runErr != nil {
		t.Fatalf("pt service start structured run should execute fake systemctl: %v", runErr)
	}
	if recorded, err := os.ReadFile(recordFile); err != nil {
		t.Fatalf("read fake systemctl record: %v", err)
	} else if got, want := strings.TrimSpace(string(recorded)), "systemctl start patroni"; got != want {
		t.Fatalf("pt service start systemctl command = %q, want %q", got, want)
	}

	var payload output.Result
	if err := json.Unmarshal(ptBytesTrimSpace([]byte(raw)), &payload); err != nil {
		t.Fatalf("invalid json output: %v raw=%q", err, raw)
	}
	data, ok := payload.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("structured data = %T, want map", payload.Data)
	}
	if cmd := ptAsString(data["command"]); cmd != "pig patroni service start" {
		t.Fatalf("structured service command = %q, want pig patroni service start", cmd)
	}
}

func TestPatroniWaitFlagsUseWWhenAvailable(t *testing.T) {
	tests := []struct {
		name string
		cmd  *cobra.Command
	}{
		{name: "pt reinit", cmd: patroniReinitCmd},
		{name: "pt pause", cmd: patroniPauseCmd},
		{name: "pt resume", cmd: patroniResumeCmd},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := tt.cmd.Flags().Lookup("wait")
			if flag == nil {
				t.Fatalf("%s must expose --wait", tt.name)
			}
			if flag.Shorthand != "w" {
				t.Fatalf("%s --wait shorthand = %q, want %q", tt.name, flag.Shorthand, "w")
			}
		})
	}
}

// TestPatroniClusterOpShorthands guards B04/B12/B17: -f remains removed,
// and approved pt shortcuts stay exposed.
func TestPatroniClusterOpShorthands(t *testing.T) {
	removed := map[*cobra.Command][]string{
		patroniRestartCmd:    {"force"},
		patroniReinitCmd:     {"force"},
		patroniSwitchoverCmd: {"force"},
		patroniFailoverCmd:   {"force"},
	}
	for cmd, flags := range removed {
		for _, name := range flags {
			if cmd.Flags().Lookup(name) != nil {
				t.Errorf("pt %s: flag --%s should be removed (B04)", cmd.Name(), name)
			}
		}
		yes := cmd.Flags().Lookup("yes")
		if yes == nil || yes.Shorthand != "y" {
			t.Errorf("pt %s: --yes/-y gate flag missing (B04)", cmd.Name())
		}
	}
	shortcuts := map[*cobra.Command]map[string]string{
		patroniReinitCmd: {
			"wait": "w",
		},
		patroniSwitchoverCmd: {
			"leader":    "l",
			"candidate": "c",
			"scheduled": "s",
		},
		patroniFailoverCmd: {
			"candidate": "c",
		},
		patroniPauseCmd: {
			"wait": "w",
		},
		patroniResumeCmd: {
			"wait": "w",
		},
	}
	for cmd, flags := range shortcuts {
		for name, shorthand := range flags {
			f := cmd.Flags().Lookup(name)
			if f == nil {
				t.Errorf("pt %s: flag --%s missing", cmd.Name(), name)
				continue
			}
			if f.Shorthand != shorthand {
				t.Errorf("pt %s: --%s shorthand = %q, want %q", cmd.Name(), name, f.Shorthand, shorthand)
			}
		}
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

func resultHasNextAction(result output.Result, needle string) bool {
	for _, action := range result.NextActions {
		if strings.Contains(action.Command, needle) || strings.Contains(action.Reason, needle) {
			return true
		}
	}
	return false
}

func resultHasRequiredNextAction(result output.Result, needle string) bool {
	for _, action := range result.NextActions {
		if action.Required && (strings.Contains(action.Command, needle) || strings.Contains(action.Reason, needle)) {
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

func capturePtStderr(t *testing.T, fn func()) string {
	t.Helper()
	origStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe error: %v", err)
	}
	os.Stderr = w
	fn()
	_ = w.Close()
	os.Stderr = origStderr
	raw, _ := io.ReadAll(r)
	_ = r.Close()
	return string(raw)
}

func installFakeSystemctl(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	recordFile := filepath.Join(dir, "systemctl.args")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" > " + recordFile + "\n"

	for _, name := range []string{"systemctl", "sudo"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
			t.Fatalf("write fake %s: %v", name, err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return recordFile
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

// TestPatroniRestartPendingSkipsGate (D2): --pending applies restarts already
// scoped by a prior config change, so it executes directly in both modes.
func TestPatroniRestartPendingSkipsGate(t *testing.T) {
	origFormat := config.OutputFormat
	origPlan := patroniPlan
	origExec := patroniRestartExec
	origConfirm := highRiskTextConfirm
	defer func() {
		config.OutputFormat = origFormat
		patroniPlan = origPlan
		patroniRestartExec = origExec
		highRiskTextConfirm = origConfirm
		_ = patroniRestartCmd.Flags().Set("yes", "false")
		_ = patroniRestartCmd.Flags().Set("pending", "false")
	}()
	patroniPlan = false
	highRiskTextConfirm = func(warning, action string) error {
		t.Fatal("--pending restart must not prompt for confirmation")
		return nil
	}
	_ = patroniRestartCmd.Flags().Set("yes", "false")
	_ = patroniRestartCmd.Flags().Set("pending", "true")

	for _, mode := range []string{config.OUTPUT_TEXT, config.OUTPUT_JSON} {
		t.Run(mode, func(t *testing.T) {
			config.OutputFormat = mode
			var gotOpts *patroni.RestartOptions
			patroniRestartExec = func(dbsu string, opts *patroni.RestartOptions) error {
				gotOpts = opts
				return nil
			}
			var runErr error
			_ = capturePtStdout(t, func() {
				runErr = patroniRestartCmd.RunE(patroniRestartCmd, nil)
			})
			if runErr != nil {
				t.Fatalf("pending restart should execute directly: %v", runErr)
			}
			if gotOpts == nil || !gotOpts.Pending || !gotOpts.Force {
				t.Fatalf("pending restart should run with --pending and --force, got %+v", gotOpts)
			}
		})
	}
}

// TestPatroniRestartPlanPreview: --plan renders a side-effect-free plan whose
// confirmation tier matches D2 and whose commands are replayable.
func TestPatroniRestartPlanPreview(t *testing.T) {
	origFormat := config.OutputFormat
	origPlan := patroniPlan
	origExec := patroniRestartExec
	defer func() {
		config.OutputFormat = origFormat
		patroniPlan = origPlan
		patroniRestartExec = origExec
	}()
	config.OutputFormat = config.OUTPUT_JSON
	patroniPlan = true
	patroniRestartExec = func(dbsu string, opts *patroni.RestartOptions) error {
		t.Fatal("--plan must not execute patronictl restart")
		return nil
	}

	raw := capturePtStdout(t, func() {
		if err := patroniRestartCmd.RunE(patroniRestartCmd, nil); err != nil {
			t.Fatalf("pt restart --plan should not fail: %v", err)
		}
	})
	var plan output.Plan
	if err := json.Unmarshal(ptBytesTrimSpace([]byte(raw)), &plan); err != nil {
		t.Fatalf("invalid plan json: %v raw=%q", err, raw)
	}
	if plan.Confirmation != "required" {
		t.Fatalf("cluster-wide restart plan confirmation = %q, want required", plan.Confirmation)
	}
	if !strings.HasSuffix(plan.Command, "--plan") {
		t.Fatalf("plan.Command should be the preview form: %q", plan.Command)
	}
	if !ptPlanHasNextAction(plan, "pig pt restart --yes") {
		t.Fatalf("plan should carry the --yes execute action, got %+v", plan.NextActions)
	}
}

// TestPatroniFailoverRequiresCandidate: Patroni fails over only to an explicit
// candidate, so pig fails fast in both output modes.
func TestPatroniFailoverRequiresCandidate(t *testing.T) {
	origFormat := config.OutputFormat
	origPlan := patroniPlan
	origExec := patroniFailoverExec
	defer func() {
		config.OutputFormat = origFormat
		patroniPlan = origPlan
		patroniFailoverExec = origExec
		_ = patroniFailoverCmd.Flags().Set("candidate", "")
	}()
	patroniPlan = false
	patroniFailoverExec = func(dbsu string, opts *patroni.FailoverOptions) error {
		t.Fatal("failover without --candidate must not execute")
		return nil
	}
	_ = patroniFailoverCmd.Flags().Set("candidate", "")

	config.OutputFormat = config.OUTPUT_TEXT
	if err := patroniFailoverCmd.RunE(patroniFailoverCmd, nil); err == nil || !strings.Contains(err.Error(), "--candidate") {
		t.Fatalf("text mode should fail mentioning --candidate, got %v", err)
	}

	config.OutputFormat = config.OUTPUT_JSON
	var runErr error
	raw := capturePtStdout(t, func() {
		runErr = patroniFailoverCmd.RunE(patroniFailoverCmd, nil)
	})
	if runErr == nil {
		t.Fatal("structured failover without --candidate should fail")
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(ptBytesTrimSpace([]byte(raw)), &payload); err != nil {
		t.Fatalf("invalid json output: %v raw=%q", err, raw)
	}
	if code, _ := payload["code"].(float64); int(code) != output.GenericParamError(output.MODULE_PT) {
		t.Fatalf("code = %v, want %d", payload["code"], output.GenericParamError(output.MODULE_PT))
	}
}

// TestPatroniRestartRejectsInvalidRole: --role is validated against the
// documented enum before anything executes.
func TestPatroniRestartRejectsInvalidRole(t *testing.T) {
	origFormat := config.OutputFormat
	origPlan := patroniPlan
	origExec := patroniRestartExec
	defer func() {
		config.OutputFormat = origFormat
		patroniPlan = origPlan
		patroniRestartExec = origExec
		_ = patroniRestartCmd.Flags().Set("role", "")
	}()
	config.OutputFormat = config.OUTPUT_TEXT
	patroniPlan = false
	patroniRestartExec = func(dbsu string, opts *patroni.RestartOptions) error {
		t.Fatal("invalid role must not execute")
		return nil
	}
	_ = patroniRestartCmd.Flags().Set("role", "bogus")

	if err := patroniRestartCmd.RunE(patroniRestartCmd, nil); err == nil || !strings.Contains(err.Error(), "bogus") {
		t.Fatalf("invalid role should fail fast, got %v", err)
	}
}

// TestPatroniConfigNoArgsDefaultsToShow: `pig pt config` routes to show-config
// in text mode too (parity with structured mode and the spec).
func TestPatroniConfigNoArgsDefaultsToShow(t *testing.T) {
	origFormat := config.OutputFormat
	origDBSU := patroniDBSU
	origUser := config.CurrentUser
	defer func() {
		config.OutputFormat = origFormat
		patroniDBSU = origDBSU
		config.CurrentUser = origUser
	}()
	config.OutputFormat = config.OUTPUT_TEXT
	// Run patronictl directly (IsDBSU short-circuit), no sudo/su indirection.
	config.CurrentUser = "testuser"
	patroniDBSU = "testuser"

	dir := t.TempDir()
	record := filepath.Join(dir, "patronictl.args")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" > " + record + "\n"
	if err := os.WriteFile(filepath.Join(dir, "patronictl"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake patronictl: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	if err := patroniConfigCmd.RunE(patroniConfigCmd, nil); err != nil {
		t.Fatalf("pt config without args should default to show: %v", err)
	}
	got, err := os.ReadFile(record)
	if err != nil {
		t.Fatalf("fake patronictl was not invoked: %v", err)
	}
	if !strings.Contains(string(got), "show-config") {
		t.Fatalf("expected show-config invocation, got %q", got)
	}
}
