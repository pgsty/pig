package cmd

import (
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"pig/cli/pitr"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"
)

func TestPITRHelpDoesNotSuggestDefaultPromote(t *testing.T) {
	if strings.Contains(pitrCmd.Example, "pitr -d -P") {
		t.Fatalf("pig pitr help should not suggest invalid --default --promote example:\n%s", pitrCmd.Example)
	}
}

func TestPITRHelpClarifiesPatroniBoundary(t *testing.T) {
	if strings.Contains(pitrCmd.Long, "automatic Patroni/PostgreSQL lifecycle management") {
		t.Fatalf("pig pitr help should not claim full Patroni lifecycle ownership:\n%s", pitrCmd.Long)
	}
	if !strings.Contains(pitrCmd.Long, "Patroni is left stopped") {
		t.Fatalf("pig pitr help should say Patroni remains outside post-restore ownership:\n%s", pitrCmd.Long)
	}
	if !strings.Contains(pitrCmd.Long, "does not rejoin Patroni") {
		t.Fatalf("pig pitr help should explicitly say it does not rejoin Patroni:\n%s", pitrCmd.Long)
	}
	if !strings.Contains(pitrCmd.Long, "only to keep the target PGDATA offline") {
		t.Fatalf("pig pitr help should say Patroni stop is only a restore-safety action:\n%s", pitrCmd.Long)
	}
	if strings.Contains(pitrCmd.Long+pitrCmd.Example, "--skip-patroni") {
		t.Fatalf("pig pitr help should not mention removed --skip-patroni flag (B10):\n%s", pitrCmd.Example)
	}
	if !strings.Contains(pitrCmd.Example, "leave PostgreSQL and Patroni stopped") {
		t.Fatalf("pig pitr --no-restart example should mention both PostgreSQL and Patroni state:\n%s", pitrCmd.Example)
	}
	if pitrCmd.Flags().Lookup("skip-patroni") != nil {
		t.Fatal("--skip-patroni flag should be removed from pitr (B10)")
	}
	if pitrCmd.Flags().Lookup("promote") != nil {
		t.Fatal("--promote flag should be removed from pitr (B09); use --target-action=promote")
	}
}

func TestPITRHelpSaysCustomDataDirRequiresNoRestart(t *testing.T) {
	if !strings.Contains(strings.ToLower(pitrCmd.Long), "custom -d side restores require --no-restart") {
		t.Fatalf("help should document custom -D auto-start safety rule:\n%s", pitrCmd.Long)
	}
	if !strings.Contains(pitrCmd.Example, "pig pitr -d -D /tmp/pg-restore --no-restart") {
		t.Fatalf("custom -D example should keep --no-restart:\n%s", pitrCmd.Example)
	}
}

func TestPITRHelpDocumentsTimeout(t *testing.T) {
	flag := pitrCmd.Flags().Lookup("timeout")
	if flag == nil {
		t.Fatal("pitr should expose --timeout")
	}
	if !strings.Contains(flag.Usage, "PostgreSQL start/recovery") {
		t.Fatalf("--timeout help should mention start/recovery scope, got %q", flag.Usage)
	}
}

func TestPITRHelpDocumentsExtraArgsSeparator(t *testing.T) {
	help := pitrCmd.Long + "\n" + pitrCmd.Example
	if !strings.Contains(help, "after --") {
		t.Fatalf("pitr help should document that extra pgBackRest args go after --:\n%s", help)
	}
	if !strings.Contains(help, "pig pitr -d -- --delta") {
		t.Fatalf("pitr examples should show -- separator passthrough:\n%s", pitrCmd.Example)
	}
}

func TestPITRSupportsPlanOnly(t *testing.T) {
	if pitrCmd.Flags().Lookup("plan") == nil {
		t.Fatal("--plan flag not found on pitr command")
	}
	if pitrCmd.Flags().Lookup("dry-run") != nil {
		t.Fatal("--dry-run alias should not exist on pitr command")
	}
}

func TestPgRepackSupportsPlanOnly(t *testing.T) {
	pgRepackCmd, _, err := rootCmd.Find([]string{"pg", "repack"})
	if err != nil {
		t.Fatalf("pg repack command not found: %v", err)
	}
	if pgRepackCmd.Flags().Lookup("plan") == nil {
		t.Fatal("--plan flag not found on pg repack command")
	}
	if pgRepackCmd.Flags().Lookup("dry-run") != nil {
		t.Fatal("--dry-run alias should not exist on pg repack command")
	}
}

func TestPbExpireSupportsPlanOnly(t *testing.T) {
	pbExpireCmd, _, err := rootCmd.Find([]string{"pb", "expire"})
	if err != nil {
		t.Fatalf("pb expire command not found: %v", err)
	}
	if pbExpireCmd.Flags().Lookup("plan") == nil {
		t.Fatal("--plan flag not found on pb expire command")
	}
	if pbExpireCmd.Flags().Lookup("dry-run") != nil {
		t.Fatal("--dry-run alias should not exist on pb expire command")
	}
}

func TestPITRMissingTargetStructuredError(t *testing.T) {
	origFormat := config.OutputFormat
	origOpts := *pitrOpts
	defer func() {
		config.OutputFormat = origFormat
		*pitrOpts = origOpts
	}()

	config.OutputFormat = config.OUTPUT_JSON
	*pitrOpts = pitr.Options{}

	err := pitrCmd.RunE(pitrCmd, nil)
	if err == nil {
		t.Fatal("expected structured error when PITR target is missing")
	}

	var exitErr *utils.ExitCodeError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitCodeError, got %T: %v", err, err)
	}
	if exitErr.Code != output.ExitCode(output.CodePITRInvalidArgs) {
		t.Fatalf("unexpected exit code: got %d, want %d",
			exitErr.Code, output.ExitCode(output.CodePITRInvalidArgs))
	}
}

func TestPITRMissingTargetTextReturnsInvalidArgs(t *testing.T) {
	origFormat := config.OutputFormat
	origOpts := *pitrOpts
	origOut := pitrCmd.OutOrStdout()
	origErr := pitrCmd.ErrOrStderr()
	origSilenceUsage := pitrCmd.SilenceUsage
	origSilenceErrors := pitrCmd.SilenceErrors
	defer func() {
		config.OutputFormat = origFormat
		*pitrOpts = origOpts
		pitrCmd.SetOut(origOut)
		pitrCmd.SetErr(origErr)
		pitrCmd.SilenceUsage = origSilenceUsage
		pitrCmd.SilenceErrors = origSilenceErrors
	}()

	config.OutputFormat = config.OUTPUT_TEXT
	*pitrOpts = pitr.Options{}
	pitrCmd.SetOut(io.Discard)
	pitrCmd.SetErr(io.Discard)
	pitrCmd.SilenceUsage = false
	pitrCmd.SilenceErrors = false

	err := pitrCmd.RunE(pitrCmd, nil)
	if err == nil {
		t.Fatal("expected text mode error when PITR target is missing")
	}

	var exitErr *utils.ExitCodeError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitCodeError, got %T: %v", err, err)
	}
	if exitErr.Code != output.ExitCode(output.CodePITRInvalidArgs) {
		t.Fatalf("unexpected exit code: got %d, want %d",
			exitErr.Code, output.ExitCode(output.CodePITRInvalidArgs))
	}
	if !exitErr.Silent {
		t.Fatal("missing-target text error should be silent after help is printed")
	}
	if !pitrCmd.SilenceUsage {
		t.Fatal("missing-target text error should silence Cobra usage after printing help")
	}
	if !pitrCmd.SilenceErrors {
		t.Fatal("missing-target text error should silence Cobra error after printing help")
	}
}

func TestPITRStructuredExecutionRequiresExplicitYes(t *testing.T) {
	origFormat := config.OutputFormat
	origOpts := *pitrOpts
	defer func() {
		config.OutputFormat = origFormat
		*pitrOpts = origOpts
	}()

	config.OutputFormat = config.OUTPUT_JSON
	*pitrOpts = pitr.Options{Default: true, Yes: false}

	var runErr error
	raw := captureStdout(t, func() {
		runErr = pitrCmd.RunE(pitrCmd, nil)
	})
	if runErr == nil {
		t.Fatal("structured pitr execution should require explicit --yes")
	}
	var exitErr *utils.ExitCodeError
	if !errors.As(runErr, &exitErr) {
		t.Fatalf("expected ExitCodeError, got %T: %v", runErr, runErr)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(bytesTrimSpace([]byte(raw)), &payload); err != nil {
		t.Fatalf("invalid json output: %v raw=%q", err, raw)
	}
	if success, _ := payload["success"].(bool); success {
		t.Fatalf("expected success=false, got %v", payload)
	}
	if !resultDataHasNextAction(payload, "--yes") {
		t.Fatalf("expected envelope next action mentioning --yes, got %v", payload)
	}
}

func TestPITRStructuredConfirmationUsesNeutralBoundaryAndReplayableCommands(t *testing.T) {
	origFormat := config.OutputFormat
	origOpts := *pitrOpts
	defer func() {
		config.OutputFormat = origFormat
		*pitrOpts = origOpts
	}()

	config.OutputFormat = config.OUTPUT_JSON
	*pitrOpts = pitr.Options{
		Default:    true,
		NoRestart:  true,
		DataDir:    "/data/side restore",
		Stanza:     "pg-prod",
		ConfigPath: "/etc/pg backrest/custom.conf",
		Yes:        false,
	}

	var runErr error
	raw := captureStdout(t, func() {
		runErr = pitrCmd.RunE(pitrCmd, nil)
	})
	if runErr == nil {
		t.Fatal("structured pitr execution should require explicit --yes")
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(bytesTrimSpace([]byte(raw)), &payload); err != nil {
		t.Fatalf("invalid json output: %v raw=%q", err, raw)
	}
	data, _ := payload["data"].(map[string]interface{})
	operation, _ := data["operation"].(map[string]interface{})
	if got := asString(operation["boundary"]); got != "pitr:restore" {
		t.Fatalf("confirmation boundary = %q, want pitr:restore payload=%v", got, payload)
	}
	if !resultDataHasNextAction(payload, "pig pitr -d --no-restart -s pg-prod -c '/etc/pg backrest/custom.conf' -D '/data/side restore' --yes") {
		t.Fatalf("confirmation next actions should include replayable --yes command, got %v", payload)
	}
	if !resultDataHasNextAction(payload, "--plan") {
		t.Fatalf("confirmation next actions should include replayable --plan command, got %v", payload)
	}
	if !resultDataHasNextAction(payload, "pig pb restore --stanza pg-prod --config '/etc/pg backrest/custom.conf' --dbsu postgres --default --data '/data/side restore' --plan") {
		t.Fatalf("confirmation next actions should include replayable primitive restore plan, got %v", payload)
	}
}

func TestPITRStructuredRejectsExtraArgsBeforeDash(t *testing.T) {
	origFormat := config.OutputFormat
	origOpts := *pitrOpts
	defer func() {
		config.OutputFormat = origFormat
		*pitrOpts = origOpts
	}()

	config.OutputFormat = config.OUTPUT_JSON
	*pitrOpts = pitr.Options{Default: true, Plan: true}

	var runErr error
	raw := captureStdout(t, func() {
		runErr = pitrCmd.RunE(pitrCmd, []string{"--delta"})
	})
	if runErr == nil {
		t.Fatal("pitr should reject extra args that are not after --")
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(bytesTrimSpace([]byte(raw)), &payload); err != nil {
		t.Fatalf("invalid json output: %v raw=%q", err, raw)
	}
	if success, _ := payload["success"].(bool); success {
		t.Fatalf("expected success=false, got %v", payload)
	}
	if !strings.Contains(asString(payload["detail"]), "after --") {
		t.Fatalf("detail should mention -- separator, got %v", payload)
	}
}

func TestPITRPlanPrecheckStructuredUsesTypedCode(t *testing.T) {
	origFormat := config.OutputFormat
	origOpts := *pitrOpts
	origSilenceUsage := pitrCmd.SilenceUsage
	defer func() {
		config.OutputFormat = origFormat
		*pitrOpts = origOpts
		pitrCmd.SilenceUsage = origSilenceUsage
	}()

	config.OutputFormat = config.OUTPUT_JSON
	*pitrOpts = pitr.Options{
		Default:    true,
		Plan:       true,
		ConfigPath: filepath.Join(t.TempDir(), "missing-pgbackrest.conf"),
	}

	var runErr error
	raw := captureStdout(t, func() {
		runErr = pitrCmd.RunE(pitrCmd, nil)
	})
	if runErr == nil {
		t.Fatal("expected structured plan precheck error")
	}
	var exitErr *utils.ExitCodeError
	if !errors.As(runErr, &exitErr) {
		t.Fatalf("expected ExitCodeError, got %T: %v", runErr, runErr)
	}
	if exitErr.Code != output.ExitCode(output.CodePITRPrecheckFailed) {
		t.Fatalf("exit code = %d, want %d", exitErr.Code, output.ExitCode(output.CodePITRPrecheckFailed))
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(bytesTrimSpace([]byte(raw)), &payload); err != nil {
		t.Fatalf("invalid json output: %v raw=%q", err, raw)
	}
	if got := int(payload["code"].(float64)); got != output.CodePITRPrecheckFailed {
		t.Fatalf("structured code = %d, want %d payload=%v", got, output.CodePITRPrecheckFailed, payload)
	}
}

func TestPITRTextRuntimeErrorSilencesUsageAfterValidation(t *testing.T) {
	origFormat := config.OutputFormat
	origOpts := *pitrOpts
	origSilenceUsage := pitrCmd.SilenceUsage
	origSilenceErrors := pitrCmd.SilenceErrors
	origOut := pitrCmd.OutOrStdout()
	origErr := pitrCmd.ErrOrStderr()
	defer func() {
		config.OutputFormat = origFormat
		*pitrOpts = origOpts
		pitrCmd.SilenceUsage = origSilenceUsage
		pitrCmd.SilenceErrors = origSilenceErrors
		pitrCmd.SetOut(origOut)
		pitrCmd.SetErr(origErr)
	}()

	config.OutputFormat = config.OUTPUT_TEXT
	pitrCmd.SetOut(io.Discard)
	pitrCmd.SetErr(io.Discard)

	pitrCmd.SilenceUsage = false
	*pitrOpts = pitr.Options{
		Default:    true,
		Plan:       true,
		ConfigPath: filepath.Join(t.TempDir(), "missing-pgbackrest.conf"),
	}
	if err := pitrCmd.RunE(pitrCmd, nil); err == nil {
		t.Fatal("expected runtime/precheck error")
	}
	if !pitrCmd.SilenceUsage {
		t.Fatal("runtime/precheck errors after valid args should silence Cobra usage")
	}

	pitrCmd.SilenceUsage = false
	pitrCmd.SilenceErrors = false
	*pitrOpts = pitr.Options{}
	if err := pitrCmd.RunE(pitrCmd, nil); err == nil {
		t.Fatal("expected missing-target error")
	}
	if !pitrCmd.SilenceUsage {
		t.Fatal("missing target should silence Cobra usage after printing help")
	}
	if !pitrCmd.SilenceErrors {
		t.Fatal("missing target should silence Cobra error after printing help")
	}
}

func TestPITRStructuredModeDoesNotAutoConfirm(t *testing.T) {
	opts := &pitr.Options{Default: true}

	preparePITRStructuredOptions(opts)

	if opts.Yes {
		t.Fatal("structured PITR should not imply destructive confirmation")
	}
	if !opts.Quiet {
		t.Fatal("structured PITR should suppress human progress logs")
	}
}

func bytesTrimSpace(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}

func resultDataHasNextAction(data map[string]interface{}, needle string) bool {
	items, _ := data["next_actions"].([]interface{})
	for _, item := range items {
		m, _ := item.(map[string]interface{})
		if strings.Contains(asString(m["command"]), needle) || strings.Contains(asString(m["reason"]), needle) {
			return true
		}
	}
	return false
}

func asString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
