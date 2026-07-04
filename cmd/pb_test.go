package cmd

import (
	"encoding/json"
	"errors"
	"github.com/spf13/cobra"
	"io"
	"os"
	"path/filepath"
	"pig/cli/pgbackrest"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"
	"strings"
	"testing"
)

func TestPbLogUnknownActionIsRejected(t *testing.T) {
	if pbLogCmd.Args == nil {
		t.Fatal("pb log should validate positional arguments")
	}
	err := pbLogCmd.Args(pbLogCmd, []string{"taill"})
	if err == nil {
		t.Fatal("expected unknown pb log action to be rejected as an unexpected argument")
	}
	if !strings.Contains(err.Error(), "accepts 0 arg") && !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("expected explicit argument rejection, got %v", err)
	}
}

func TestPbLogLatestOnlySubcommandsRejectFileArgs(t *testing.T) {
	tests := []struct {
		name string
		cmd  *cobra.Command
	}{
		{name: "show", cmd: pbLogCatCmd},
		{name: "tail", cmd: pbLogTailCmd},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.cmd.Args == nil {
				t.Fatalf("pb log %s should validate positional arguments", tt.name)
			}
			if err := tt.cmd.Args(tt.cmd, []string{"pg-meta-backup.log"}); err == nil {
				t.Fatalf("expected pb log %s to reject file arguments", tt.name)
			}
		})
	}
}

func TestPbAliasesMatchContract(t *testing.T) {
	tests := []struct {
		name string
		cmd  *cobra.Command
		want []string
	}{
		{name: "pb info", cmd: pbInfoCmd, want: []string{"i"}},
		{name: "pb list", cmd: pbLsCmd, want: []string{"ls"}},
		{name: "pb backup", cmd: pbBackupCmd, want: []string{"b"}},
		{name: "pb restore", cmd: pbRestoreCmd, want: []string{"r"}},
		{name: "pb expire", cmd: pbExpireCmd, want: []string{"e"}},
		{name: "pb create", cmd: pbCreateCmd, want: []string{"c"}},
		{name: "pb upgrade", cmd: pbUpgradeCmd, want: []string{"u"}},
		{name: "pb delete", cmd: pbDeleteCmd, want: []string{"d"}},
		{name: "pb check", cmd: pbCheckCmd, want: []string{"ck"}},
		{name: "pb start", cmd: pbStartCmd, want: []string{"up"}},
		{name: "pb stop", cmd: pbStopCmd, want: []string{"dw"}},
		{name: "pb log", cmd: pbLogCmd, want: []string{"l"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := strings.Join(tt.cmd.Aliases, ","); got != strings.Join(tt.want, ",") {
				t.Fatalf("%s aliases = %v, want %v", tt.name, tt.cmd.Aliases, tt.want)
			}
		})
	}
}

func TestPbListUsesMeaningfulPrimaryCommand(t *testing.T) {
	if pbLsCmd.Use != "list [type]" {
		t.Fatalf("pb list use = %q, want %q", pbLsCmd.Use, "list [type]")
	}
}

func TestPbSpecOverviewMatchesAliasContract(t *testing.T) {
	raw, err := os.ReadFile("../docs/spec/pb.md")
	if err != nil {
		t.Fatalf("read pb spec: %v", err)
	}
	spec := string(raw)
	for _, want := range []string{
		"| `pb info` | `i` |",
		"| `pb list` | `ls` |",
		"| `pb backup` | `b` |",
		"| `pb restore` | `r` |",
		"| `pb expire` | `e` |",
		"| `pb create` | `c` |",
		"| `pb upgrade` | `u` |",
		"| `pb delete` | `d` |",
		"| `pb check` | `ck` |",
		"| `pb start` | `up` |",
		"| `pb stop` | `dw` |",
		"| `pb log` | `l` |",
	} {
		if !strings.Contains(spec, want) {
			t.Fatalf("pb spec missing alias contract row containing %q", want)
		}
	}
	for _, forbidden := range []string{
		"| `pb ls` |",
		"`l, lg`",
		"pb ls cluster",
		"pb ls cls",
		"aliases: `cluster`, `cls`",
	} {
		if strings.Contains(spec, forbidden) {
			t.Fatalf("pb spec still contains deprecated contract fragment %q", forbidden)
		}
	}
}

func TestResolvePbInfoRawOutput(t *testing.T) {
	origRawOutput := pbInfoRawOutput
	origFormat := config.OutputFormat
	defer func() {
		pbInfoRawOutput = origRawOutput
		config.OutputFormat = origFormat
	}()

	tests := []struct {
		name      string
		rawOutput string
		format    string
		want      string
		wantErr   bool
	}{
		{name: "explicit json", rawOutput: "json", format: config.OUTPUT_TEXT, want: "json", wantErr: false},
		{name: "explicit text", rawOutput: "text", format: config.OUTPUT_JSON, want: "text", wantErr: false},
		{name: "explicit uppercase normalized", rawOutput: "JSON", format: config.OUTPUT_TEXT, want: "json", wantErr: false},
		{name: "explicit invalid", rawOutput: "yaml", format: config.OUTPUT_TEXT, want: "", wantErr: true},
		{name: "inherit json output", rawOutput: "", format: config.OUTPUT_JSON, want: "json", wantErr: false},
		{name: "inherit json-pretty output", rawOutput: "", format: config.OUTPUT_JSON_PRETTY, want: "json", wantErr: false},
		{name: "inherit yaml output unsupported", rawOutput: "", format: config.OUTPUT_YAML, want: "", wantErr: true},
		{name: "text output default", rawOutput: "", format: config.OUTPUT_TEXT, want: "", wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pbInfoRawOutput = tt.rawOutput
			config.OutputFormat = tt.format
			got, err := resolvePbInfoRawOutput()
			if (err != nil) != tt.wantErr {
				t.Fatalf("resolvePbInfoRawOutput() error=%v, wantErr=%v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("resolvePbInfoRawOutput()=%q, want %q", got, tt.want)
			}
		})
	}
}

func TestPbInfoRawOutputFlagDoesNotShadowGlobalOutput(t *testing.T) {
	if f := pbInfoCmd.Flags().Lookup("output"); f != nil {
		t.Fatalf("pb info local --output should not exist, found %q", f.Name)
	}
	if f := pbInfoCmd.Flags().ShorthandLookup("o"); f != nil {
		t.Fatalf("pb info local -o should not exist, found %q", f.Name)
	}
	if f := pbInfoCmd.Flags().Lookup("raw-output"); f == nil {
		t.Fatal("pb info --raw-output flag should exist")
	}
}

func TestPbInfoRawOutputValidationStructuredMode(t *testing.T) {
	origRaw := pbInfoRaw
	origRawOutput := pbInfoRawOutput
	origSet := pbInfoSet
	origFormat := config.OutputFormat
	defer func() {
		pbInfoRaw = origRaw
		pbInfoRawOutput = origRawOutput
		pbInfoSet = origSet
		config.OutputFormat = origFormat
	}()

	pbInfoRaw = false
	pbInfoRawOutput = "json"
	pbInfoSet = ""
	config.OutputFormat = config.OUTPUT_JSON

	err := pbInfoCmd.RunE(pbInfoCmd, nil)
	if err == nil {
		t.Fatal("expected error for --raw-output without --raw in structured mode")
	}

	var exitCodeErr *utils.ExitCodeError
	if !errors.As(err, &exitCodeErr) {
		t.Fatalf("expected ExitCodeError, got %T: %v", err, err)
	}
	wantCode := output.ExitCode(output.CodePbInvalidInfoParams)
	if exitCodeErr.Code != wantCode {
		t.Fatalf("unexpected exit code: got %d, want %d", exitCodeErr.Code, wantCode)
	}
}

func TestPbExpireSetTextRequiresConfirmationBeforeExecution(t *testing.T) {
	origFormat := config.OutputFormat
	origSet := pbExpireSet
	origPlan := pbExpirePlan
	origYes := pbExpireYes
	origConfirm := highRiskTextConfirm
	origExec := pbExpireCommandExec
	defer func() {
		config.OutputFormat = origFormat
		pbExpireSet = origSet
		pbExpirePlan = origPlan
		pbExpireYes = origYes
		highRiskTextConfirm = origConfirm
		pbExpireCommandExec = origExec
	}()

	config.OutputFormat = config.OUTPUT_TEXT
	pbExpireSet = "20250101-010101F"
	pbExpirePlan = false
	pbExpireYes = false
	confirmErr := errors.New("confirmation cancelled")
	executed := false
	highRiskTextConfirm = func(warning, action string) error {
		if !strings.Contains(warning, pbExpireSet) || !strings.Contains(action, "expire") {
			t.Fatalf("unexpected expire confirmation warning/action: %q / %q", warning, action)
		}
		return confirmErr
	}
	pbExpireCommandExec = func(*pgbackrest.Config, *pgbackrest.ExpireOptions) error {
		executed = true
		return nil
	}

	err := pbExpireCmd.RunE(pbExpireCmd, nil)
	if !errors.Is(err, confirmErr) {
		t.Fatalf("pb expire --set error = %v, want confirmation error", err)
	}
	if executed {
		t.Fatal("pb expire --set should not execute after confirmation cancellation")
	}
}

func TestPbExpireSetStructuredRequiresExplicitYes(t *testing.T) {
	origFormat := config.OutputFormat
	origSet := pbExpireSet
	origPlan := pbExpirePlan
	origYes := pbExpireYes
	origExec := pbExpireCommandExec
	defer func() {
		config.OutputFormat = origFormat
		pbExpireSet = origSet
		pbExpirePlan = origPlan
		pbExpireYes = origYes
		pbExpireCommandExec = origExec
	}()

	config.OutputFormat = config.OUTPUT_JSON
	pbExpireSet = "20250101-010101F"
	pbExpirePlan = false
	pbExpireYes = false
	executed := false
	pbExpireCommandExec = func(*pgbackrest.Config, *pgbackrest.ExpireOptions) error {
		executed = true
		return nil
	}

	var runErr error
	raw := capturePbStdout(t, func() {
		runErr = pbExpireCmd.RunE(pbExpireCmd, nil)
	})
	if runErr == nil {
		t.Fatal("structured pb expire --set should require explicit --yes")
	}
	if executed {
		t.Fatal("structured pb expire --set should not execute without --yes")
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(pbBytesTrimSpace([]byte(raw)), &payload); err != nil {
		t.Fatalf("invalid json output: %v raw=%q", err, raw)
	}
	if success, _ := payload["success"].(bool); success {
		t.Fatalf("expected success=false, got %v", payload)
	}
	if msg, _ := payload["message"].(string); !strings.Contains(msg, "pb expire --set requires explicit confirmation") {
		t.Fatalf("unexpected confirmation message %q in payload %v", msg, payload)
	}
}

func TestPbRestorePlanJSONContainsPrimitiveContract(t *testing.T) {
	origFormat := config.OutputFormat
	origDefault := pbRestoreDefault
	origPlan := pbRestorePlan
	origYes := pbRestoreYes
	defer func() {
		config.OutputFormat = origFormat
		pbRestoreDefault = origDefault
		pbRestorePlan = origPlan
		pbRestoreYes = origYes
	}()

	config.OutputFormat = config.OUTPUT_JSON
	pbRestoreDefault = true
	pbRestorePlan = true
	pbRestoreYes = false

	raw := capturePbStdout(t, func() {
		if err := pbRestoreCmd.RunE(pbRestoreCmd, nil); err != nil {
			t.Fatalf("pb restore --plan should not execute or fail: %v", err)
		}
	})

	var plan output.Plan
	if err := json.Unmarshal(pbBytesTrimSpace([]byte(raw)), &plan); err != nil {
		t.Fatalf("invalid plan json: %v raw=%q", err, raw)
	}
	if plan.Boundary != "pb:pgbackrest-only" {
		t.Fatalf("boundary = %q, want pb:pgbackrest-only", plan.Boundary)
	}
	if plan.Confirmation != "required" {
		t.Fatalf("confirmation = %q, want required", plan.Confirmation)
	}
	if len(plan.Preconditions) == 0 {
		t.Fatalf("expected restore preconditions in plan: %+v", plan)
	}
	if !pbPlanHasNextAction(plan, "pig pitr") {
		t.Fatalf("expected plan next_actions to point users at pig pitr, got %+v", plan.NextActions)
	}
}

func TestPbRestorePlanJSONRejectsExtraArgsBeforeDash(t *testing.T) {
	origFormat := config.OutputFormat
	origDefault := pbRestoreDefault
	origImmediate := pbRestoreImmediate
	origTime := pbRestoreTime
	origName := pbRestoreName
	origLSN := pbRestoreLSN
	origXID := pbRestoreXID
	origSet := pbRestoreSet
	origDataDir := pbRestoreDataDir
	origExclusive := pbRestoreExclusive
	origTargetAction := pbRestoreTargetAction
	origTargetTimeline := pbRestoreTargetTimeline
	origPlan := pbRestorePlan
	origYes := pbRestoreYes
	defer func() {
		config.OutputFormat = origFormat
		pbRestoreDefault = origDefault
		pbRestoreImmediate = origImmediate
		pbRestoreTime = origTime
		pbRestoreName = origName
		pbRestoreLSN = origLSN
		pbRestoreXID = origXID
		pbRestoreSet = origSet
		pbRestoreDataDir = origDataDir
		pbRestoreExclusive = origExclusive
		pbRestoreTargetAction = origTargetAction
		pbRestoreTargetTimeline = origTargetTimeline
		pbRestorePlan = origPlan
		pbRestoreYes = origYes
	}()

	config.OutputFormat = config.OUTPUT_JSON
	pbRestoreDefault = true
	pbRestoreImmediate = false
	pbRestoreTime = ""
	pbRestoreName = ""
	pbRestoreLSN = ""
	pbRestoreXID = ""
	pbRestoreSet = ""
	pbRestoreDataDir = ""
	pbRestoreExclusive = false
	pbRestoreTargetAction = ""
	pbRestoreTargetTimeline = ""
	pbRestorePlan = true
	pbRestoreYes = false

	var runErr error
	raw := capturePbStdout(t, func() {
		runErr = pbRestoreCmd.RunE(pbRestoreCmd, []string{"--delta"})
	})
	if runErr == nil {
		t.Fatal("pb restore --plan should reject extra args that are not after --")
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(pbBytesTrimSpace([]byte(raw)), &payload); err != nil {
		t.Fatalf("invalid json output: %v raw=%q", err, raw)
	}
	if success, _ := payload["success"].(bool); success {
		t.Fatalf("expected success=false, got %v", payload)
	}
	if !strings.Contains(asString(payload["detail"]), "after --") {
		t.Fatalf("detail should mention -- separator, got %v", payload)
	}
}

func TestPbRestoreHelpDocumentsExtraArgsSeparator(t *testing.T) {
	help := pbRestoreCmd.Long + "\n" + pbRestoreCmd.Example
	if !strings.Contains(help, "after --") {
		t.Fatalf("pb restore help should document that extra pgBackRest args go after --:\n%s", help)
	}
	if !strings.Contains(help, "pig pb restore -d -- --delta") {
		t.Fatalf("pb restore examples should show -- separator passthrough:\n%s", pbRestoreCmd.Example)
	}
}

func TestPbRestoreStructuredExecutionRequiresExplicitYes(t *testing.T) {
	origFormat := config.OutputFormat
	origDefault := pbRestoreDefault
	origPlan := pbRestorePlan
	origYes := pbRestoreYes
	defer func() {
		config.OutputFormat = origFormat
		pbRestoreDefault = origDefault
		pbRestorePlan = origPlan
		pbRestoreYes = origYes
	}()

	config.OutputFormat = config.OUTPUT_JSON
	pbRestoreDefault = true
	pbRestorePlan = false
	pbRestoreYes = false

	var runErr error
	raw := capturePbStdout(t, func() {
		runErr = pbRestoreCmd.RunE(pbRestoreCmd, nil)
	})
	if runErr == nil {
		t.Fatal("structured pb restore execution should require explicit --yes")
	}
	var exitErr *utils.ExitCodeError
	if !errors.As(runErr, &exitErr) {
		t.Fatalf("expected ExitCodeError, got %T: %v", runErr, runErr)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(pbBytesTrimSpace([]byte(raw)), &payload); err != nil {
		t.Fatalf("invalid json output: %v raw=%q", err, raw)
	}
	if success, _ := payload["success"].(bool); success {
		t.Fatalf("expected success=false, got %v", payload)
	}
	if !pbResultDataHasNextAction(payload, "--yes") {
		t.Fatalf("expected envelope next action mentioning --yes, got %v", payload)
	}
}

func TestPbRestorePlanRejectsInvalidRestoreOptions(t *testing.T) {
	origFormat := config.OutputFormat
	origDefault := pbRestoreDefault
	origPlan := pbRestorePlan
	origTargetAction := pbRestoreTargetAction
	defer func() {
		config.OutputFormat = origFormat
		pbRestoreDefault = origDefault
		pbRestorePlan = origPlan
		pbRestoreTargetAction = origTargetAction
	}()

	config.OutputFormat = config.OUTPUT_JSON
	pbRestoreDefault = true
	pbRestorePlan = true
	pbRestoreTargetAction = "promote"

	var runErr error
	raw := capturePbStdout(t, func() {
		runErr = pbRestoreCmd.RunE(pbRestoreCmd, nil)
	})
	if runErr == nil {
		t.Fatalf("pb restore --plan should reject invalid restore options, output=%q", raw)
	}
	var exitErr *utils.ExitCodeError
	if !errors.As(runErr, &exitErr) {
		t.Fatalf("expected ExitCodeError, got %T: %v", runErr, runErr)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(pbBytesTrimSpace([]byte(raw)), &payload); err != nil {
		t.Fatalf("invalid json output: %v raw=%q", err, raw)
	}
	if success, _ := payload["success"].(bool); success {
		t.Fatalf("expected success=false, got %v", payload)
	}
	if !strings.Contains(pbAsString(payload["detail"]), "--target-action") {
		t.Fatalf("expected detail to mention --target-action, got %v", payload)
	}
}

func TestPbRestoreMissingTargetStructuredError(t *testing.T) {
	origFormat := config.OutputFormat
	origDefault := pbRestoreDefault
	origImmediate := pbRestoreImmediate
	origTime := pbRestoreTime
	origName := pbRestoreName
	origLSN := pbRestoreLSN
	origXID := pbRestoreXID
	defer func() {
		config.OutputFormat = origFormat
		pbRestoreDefault = origDefault
		pbRestoreImmediate = origImmediate
		pbRestoreTime = origTime
		pbRestoreName = origName
		pbRestoreLSN = origLSN
		pbRestoreXID = origXID
	}()

	config.OutputFormat = config.OUTPUT_JSON
	pbRestoreDefault = false
	pbRestoreImmediate = false
	pbRestoreTime = ""
	pbRestoreName = ""
	pbRestoreLSN = ""
	pbRestoreXID = ""

	err := pbRestoreCmd.RunE(pbRestoreCmd, nil)
	if err == nil {
		t.Fatal("expected structured error when restore target is missing")
	}

	var exitErr *utils.ExitCodeError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitCodeError, got %T: %v", err, err)
	}
	if exitErr.Code != output.ExitCode(output.CodePbInvalidRestoreParams) {
		t.Fatalf("unexpected exit code: got %d, want %d",
			exitErr.Code, output.ExitCode(output.CodePbInvalidRestoreParams))
	}
}

func TestPbRestoreMissingTargetTextReturnsInvalidArgs(t *testing.T) {
	origFormat := config.OutputFormat
	origDefault := pbRestoreDefault
	origImmediate := pbRestoreImmediate
	origTime := pbRestoreTime
	origName := pbRestoreName
	origLSN := pbRestoreLSN
	origXID := pbRestoreXID
	origOut := pbRestoreCmd.OutOrStdout()
	origErr := pbRestoreCmd.ErrOrStderr()
	origSilenceUsage := pbRestoreCmd.SilenceUsage
	origSilenceErrors := pbRestoreCmd.SilenceErrors
	defer func() {
		config.OutputFormat = origFormat
		pbRestoreDefault = origDefault
		pbRestoreImmediate = origImmediate
		pbRestoreTime = origTime
		pbRestoreName = origName
		pbRestoreLSN = origLSN
		pbRestoreXID = origXID
		pbRestoreCmd.SetOut(origOut)
		pbRestoreCmd.SetErr(origErr)
		pbRestoreCmd.SilenceUsage = origSilenceUsage
		pbRestoreCmd.SilenceErrors = origSilenceErrors
	}()

	config.OutputFormat = config.OUTPUT_TEXT
	pbRestoreDefault = false
	pbRestoreImmediate = false
	pbRestoreTime = ""
	pbRestoreName = ""
	pbRestoreLSN = ""
	pbRestoreXID = ""
	pbRestoreCmd.SetOut(io.Discard)
	pbRestoreCmd.SetErr(io.Discard)
	pbRestoreCmd.SilenceUsage = false
	pbRestoreCmd.SilenceErrors = false

	err := pbRestoreCmd.RunE(pbRestoreCmd, nil)
	if err == nil {
		t.Fatal("expected text mode error when restore target is missing")
	}

	var exitErr *utils.ExitCodeError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitCodeError, got %T: %v", err, err)
	}
	if exitErr.Code != output.ExitCode(output.CodePbInvalidRestoreParams) {
		t.Fatalf("unexpected exit code: got %d, want %d",
			exitErr.Code, output.ExitCode(output.CodePbInvalidRestoreParams))
	}
	if !exitErr.Silent {
		t.Fatal("missing-target help path should return a silent exit after printing help")
	}
	if !pbRestoreCmd.SilenceUsage || !pbRestoreCmd.SilenceErrors {
		t.Fatalf("missing-target help path should silence Cobra duplicate output, got usage=%v errors=%v",
			pbRestoreCmd.SilenceUsage, pbRestoreCmd.SilenceErrors)
	}
}

func TestPbRestoreTextRuntimeErrorAfterValidationIsSilent(t *testing.T) {
	origFormat := config.OutputFormat
	origDefault := pbRestoreDefault
	origImmediate := pbRestoreImmediate
	origTime := pbRestoreTime
	origName := pbRestoreName
	origLSN := pbRestoreLSN
	origXID := pbRestoreXID
	origSet := pbRestoreSet
	origDataDir := pbRestoreDataDir
	origExclusive := pbRestoreExclusive
	origTargetAction := pbRestoreTargetAction
	origTargetTimeline := pbRestoreTargetTimeline
	origPlan := pbRestorePlan
	origYes := pbRestoreYes
	origConfigPath := pbConfig.ConfigPath
	origSilenceUsage := pbRestoreCmd.SilenceUsage
	origSilenceErrors := pbRestoreCmd.SilenceErrors
	defer func() {
		config.OutputFormat = origFormat
		pbRestoreDefault = origDefault
		pbRestoreImmediate = origImmediate
		pbRestoreTime = origTime
		pbRestoreName = origName
		pbRestoreLSN = origLSN
		pbRestoreXID = origXID
		pbRestoreSet = origSet
		pbRestoreDataDir = origDataDir
		pbRestoreExclusive = origExclusive
		pbRestoreTargetAction = origTargetAction
		pbRestoreTargetTimeline = origTargetTimeline
		pbRestorePlan = origPlan
		pbRestoreYes = origYes
		pbConfig.ConfigPath = origConfigPath
		pbRestoreCmd.SilenceUsage = origSilenceUsage
		pbRestoreCmd.SilenceErrors = origSilenceErrors
	}()

	config.OutputFormat = config.OUTPUT_TEXT
	pbRestoreDefault = true
	pbRestoreImmediate = false
	pbRestoreTime = ""
	pbRestoreName = ""
	pbRestoreLSN = ""
	pbRestoreXID = ""
	pbRestoreSet = ""
	pbRestoreDataDir = ""
	pbRestoreExclusive = false
	pbRestoreTargetAction = ""
	pbRestoreTargetTimeline = ""
	pbRestorePlan = false
	pbRestoreYes = true
	pbConfig.ConfigPath = filepath.Join(t.TempDir(), "missing.conf")
	pbRestoreCmd.SilenceUsage = false
	pbRestoreCmd.SilenceErrors = false

	err := pbRestoreCmd.RunE(pbRestoreCmd, nil)
	var exitErr *utils.ExitCodeError
	if !errors.As(err, &exitErr) {
		t.Fatalf("pb restore runtime error = %T, want ExitCodeError: %v", err, err)
	}
	if !exitErr.Silent {
		t.Fatalf("pb restore runtime error should be silent, got %v", err)
	}
	if !pbRestoreCmd.SilenceUsage || !pbRestoreCmd.SilenceErrors {
		t.Fatalf("pb restore runtime error should silence Cobra output, got usage=%v errors=%v",
			pbRestoreCmd.SilenceUsage, pbRestoreCmd.SilenceErrors)
	}
}

func capturePbStdout(t *testing.T, fn func()) string {
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

func pbBytesTrimSpace(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}

func pbPlanHasNextAction(plan output.Plan, needle string) bool {
	for _, action := range plan.NextActions {
		if strings.Contains(action.Command, needle) || strings.Contains(action.Reason, needle) {
			return true
		}
	}
	return false
}

func pbResultDataHasNextAction(data map[string]interface{}, needle string) bool {
	items, _ := data["next_actions"].([]interface{})
	for _, item := range items {
		m, _ := item.(map[string]interface{})
		if strings.Contains(pbAsString(m["command"]), needle) || strings.Contains(pbAsString(m["reason"]), needle) {
			return true
		}
	}
	return false
}

func pbAsString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func TestRestoreHelpDoesNotSuggestDefaultPromote(t *testing.T) {
	restoreCmd, _, err := pbTestCommand.Find([]string{"restore"})
	if err != nil {
		t.Fatalf("restore command not found: %v", err)
	}
	if strings.Contains(restoreCmd.Example, "restore -d -P") {
		t.Fatalf("pig pb restore help should not suggest invalid --default --promote example:\n%s", restoreCmd.Example)
	}
}

var pbTestCommand = pbCmd
