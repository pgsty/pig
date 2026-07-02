package cmd

import (
	"encoding/json"
	"errors"
	"io"
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
	if strings.Contains(pitrCmd.Example, "pig pitr -d --skip-patroni") {
		t.Fatalf("pig pitr help should not show --skip-patroni against the managed default data dir:\n%s", pitrCmd.Example)
	}
	if !strings.Contains(pitrCmd.Example, "leave PostgreSQL and Patroni stopped") {
		t.Fatalf("pig pitr --no-restart example should mention both PostgreSQL and Patroni state:\n%s", pitrCmd.Example)
	}
	skipFlag := pitrCmd.Flags().Lookup("skip-patroni")
	if skipFlag == nil || !strings.Contains(skipFlag.Usage, "custom -D") {
		t.Fatalf("--skip-patroni help should mention custom -D/standalone scope, got %v", skipFlag)
	}
	promoteFlag := pitrCmd.Flags().Lookup("promote")
	if promoteFlag == nil || !strings.Contains(promoteFlag.Usage, "manual recovery target") {
		t.Fatalf("--promote help should mention manual recovery target scope, got %v", promoteFlag)
	}
}

func TestPITRHelpSaysCustomDataDirRequiresNoRestart(t *testing.T) {
	if !strings.Contains(strings.ToLower(pitrCmd.Long), "custom -d side restores require --no-restart") {
		t.Fatalf("help should document custom -D auto-start safety rule:\n%s", pitrCmd.Long)
	}
	if !strings.Contains(pitrCmd.Example, "pig pitr -d -D /tmp/pg-restore --skip-patroni --no-restart") {
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
	defer func() {
		config.OutputFormat = origFormat
		*pitrOpts = origOpts
		pitrCmd.SetOut(origOut)
		pitrCmd.SetErr(origErr)
	}()

	config.OutputFormat = config.OUTPUT_TEXT
	*pitrOpts = pitr.Options{}
	pitrCmd.SetOut(io.Discard)
	pitrCmd.SetErr(io.Discard)

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
