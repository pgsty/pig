package cmd

import (
	"encoding/json"
	"errors"
	"github.com/spf13/cobra"
	"io"
	"os"
	"path/filepath"
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

func TestPatroniStructuredNeedYesGate(t *testing.T) {
	origFormat := config.OutputFormat
	defer func() {
		config.OutputFormat = origFormat
	}()
	config.OutputFormat = config.OUTPUT_JSON

	var runErr error
	raw := capturePtStdout(t, func() {
		runErr = requirePatroniStructuredYes(false, patroni.RestartNeedYesResult())
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
	defer func() {
		config.OutputFormat = origFormat
		patroniPlan = origPlan
		highRiskTextConfirm = origConfirm
		patroniReinitExec = origReinit
		patroniSwitchoverExec = origSwitchover
		patroniFailoverExec = origFailover
		_ = patroniReinitCmd.Flags().Set("yes", "false")
		_ = patroniSwitchoverCmd.Flags().Set("yes", "false")
		_ = patroniFailoverCmd.Flags().Set("yes", "false")
	}()
	config.OutputFormat = config.OUTPUT_TEXT
	patroniPlan = false

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
		warning string
	}{
		{name: "reinit", cmd: patroniReinitCmd, args: []string{"pg-test-2"}, warning: "WIPE and rebuild member pg-test-2"},
		{name: "switchover", cmd: patroniSwitchoverCmd, args: nil, warning: "transfer cluster leadership"},
		{name: "failover", cmd: patroniFailoverCmd, args: nil, warning: "data loss possible"},
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

func TestPatroniSwitchoverSilentExitSilencesCobraOutput(t *testing.T) {
	origFormat := config.OutputFormat
	origPlan := patroniPlan
	origConfirm := highRiskTextConfirm
	origSwitchover := patroniSwitchoverExec
	origSilenceErrors := patroniSwitchoverCmd.SilenceErrors
	origSilenceUsage := patroniSwitchoverCmd.SilenceUsage
	defer func() {
		config.OutputFormat = origFormat
		patroniPlan = origPlan
		highRiskTextConfirm = origConfirm
		patroniSwitchoverExec = origSwitchover
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
	origSilenceErrors := patroniFailoverCmd.SilenceErrors
	origSilenceUsage := patroniFailoverCmd.SilenceUsage
	defer func() {
		config.OutputFormat = origFormat
		patroniPlan = origPlan
		highRiskTextConfirm = origConfirm
		patroniFailoverExec = origFailover
		patroniFailoverCmd.SilenceErrors = origSilenceErrors
		patroniFailoverCmd.SilenceUsage = origSilenceUsage
		_ = patroniFailoverCmd.Flags().Set("yes", "false")
	}()

	config.OutputFormat = config.OUTPUT_TEXT
	patroniPlan = false
	patroniFailoverCmd.SilenceErrors = false
	patroniFailoverCmd.SilenceUsage = false
	_ = patroniFailoverCmd.Flags().Set("yes", "false")
	highRiskTextConfirm = func(warning, action string) error { return nil }
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
		{name: "stop", cmd: patroniStopCmd, target: "pig pt svc stop", alias: "pig pt down"},
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

func TestPatroniServiceAliasesStayMinimal(t *testing.T) {
	tests := []struct {
		name string
		cmd  *cobra.Command
		want []string
	}{
		{name: "pt start", cmd: patroniStartCmd, want: []string{"up"}},
		{name: "pt stop", cmd: patroniStopCmd, want: []string{"down"}},
		{name: "pt restart", cmd: patroniRestartCmd, want: []string{"rst"}},
		{name: "pt reload", cmd: patroniReloadCmd, want: []string{"rl"}},
		{name: "pt status", cmd: patroniStatusCmd, want: []string{"st"}},
		{name: "pt svc start", cmd: patroniSvcStartCmd, want: []string{"up"}},
		{name: "pt svc stop", cmd: patroniSvcStopCmd, want: []string{"down"}},
		{name: "pt svc restart", cmd: patroniSvcRestartCmd, want: []string{"rst"}},
		{name: "pt svc reload", cmd: patroniSvcReloadCmd, want: []string{"rl"}},
		{name: "pt svc status", cmd: patroniSvcStatusCmd, want: []string{"st"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := strings.Join(tt.cmd.Aliases, ","); got != strings.Join(tt.want, ",") {
				t.Fatalf("%s aliases = %v, want %v", tt.name, tt.cmd.Aliases, tt.want)
			}
		})
	}
}

// TestPatroniRemovedShorthands guards B04/B12/B17: -f/-l/-c/-s/-w are gone from
// the pt cluster-op surface while --yes/-y is present on all four T2 commands.
func TestPatroniRemovedShorthands(t *testing.T) {
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
	longOnly := map[*cobra.Command][]string{
		patroniReinitCmd:     {"wait"},
		patroniPauseCmd:      {"wait"},
		patroniResumeCmd:     {"wait"},
		patroniSwitchoverCmd: {"leader", "candidate", "scheduled"},
		patroniFailoverCmd:   {"candidate"},
	}
	for cmd, flags := range longOnly {
		for _, name := range flags {
			f := cmd.Flags().Lookup(name)
			if f == nil {
				t.Errorf("pt %s: flag --%s missing", cmd.Name(), name)
				continue
			}
			if f.Shorthand != "" {
				t.Errorf("pt %s: --%s must be long-only (B12/B17), has -%s", cmd.Name(), name, f.Shorthand)
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
