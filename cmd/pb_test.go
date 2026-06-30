package cmd

import (
	"encoding/json"
	"errors"
	"github.com/spf13/cobra"
	"io"
	"os"
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

func TestPbRestorePlanJSONPassesPositionalExtraArgs(t *testing.T) {
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
	origPromote := pbRestorePromote
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
		pbRestorePromote = origPromote
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
	pbRestorePromote = false
	pbRestoreTargetAction = ""
	pbRestoreTargetTimeline = ""
	pbRestorePlan = true
	pbRestoreYes = false

	raw := capturePbStdout(t, func() {
		if err := pbRestoreCmd.RunE(pbRestoreCmd, []string{"--delta"}); err != nil {
			t.Fatalf("pb restore --plan should accept positional pgBackRest args: %v", err)
		}
	})

	var plan output.Plan
	if err := json.Unmarshal(pbBytesTrimSpace([]byte(raw)), &plan); err != nil {
		t.Fatalf("invalid plan json: %v raw=%q", err, raw)
	}
	if !strings.Contains(plan.Command, "--delta") {
		t.Fatalf("plan command should include positional pgBackRest arg --delta, got %q", plan.Command)
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
	data, _ := payload["data"].(map[string]interface{})
	if !pbResultDataHasNextAction(data, "--yes") {
		t.Fatalf("expected next action mentioning --yes, got data=%v", data)
	}
}

func TestPbRestorePlanRejectsInvalidRestoreOptions(t *testing.T) {
	origFormat := config.OutputFormat
	origDefault := pbRestoreDefault
	origPlan := pbRestorePlan
	origPromote := pbRestorePromote
	defer func() {
		config.OutputFormat = origFormat
		pbRestoreDefault = origDefault
		pbRestorePlan = origPlan
		pbRestorePromote = origPromote
	}()

	config.OutputFormat = config.OUTPUT_JSON
	pbRestoreDefault = true
	pbRestorePlan = true
	pbRestorePromote = true

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
	if !strings.Contains(pbAsString(payload["detail"]), "--promote") {
		t.Fatalf("expected detail to mention --promote, got %v", payload)
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
