/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

Tests for pb cmd-layer guards: dash-separator rejection, argument validation,
raw parameter errors, and replayable confirmation-gate commands.
*/
package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"

	"github.com/spf13/cobra"
)

// TestRejectRestoreExtraArgsBeforeDash exercises the dash-separator guard,
// including the stray-positional-before-dash case that previously leaked
// through to pgbackrest.
func TestRejectRestoreExtraArgsBeforeDash(t *testing.T) {
	origFormat := config.OutputFormat
	defer func() { config.OutputFormat = origFormat }()
	config.OutputFormat = config.OUTPUT_TEXT

	parse := func(argv []string) (*cobra.Command, []string) {
		t.Helper()
		c := &cobra.Command{Use: "restore"}
		c.Flags().BoolP("default", "d", false, "")
		if err := c.ParseFlags(argv); err != nil {
			t.Fatalf("parse %v: %v", argv, err)
		}
		return c, c.Flags().Args()
	}

	tests := []struct {
		name    string
		argv    []string
		wantErr bool
	}{
		{"no positionals", []string{"-d"}, false},
		{"all extras after dash", []string{"-d", "--", "--delta"}, false},
		{"stray before dash leaks no more", []string{"-d", "stray", "--", "--delta"}, true},
		{"positionals without dash", []string{"-d", "stray"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, args := parse(tt.argv)
			err := rejectRestoreExtraArgsBeforeDash(c, args, output.CodePbInvalidRestoreParams)
			if (err != nil) != tt.wantErr {
				t.Fatalf("argv %v: err = %v, wantErr = %v", tt.argv, err, tt.wantErr)
			}
		})
	}
}

// TestPbBackupAndLsRejectExtraPositionalArgs verifies stray positionals are
// no longer silently swallowed.
func TestPbBackupAndLsRejectExtraPositionalArgs(t *testing.T) {
	if err := pbBackupCmd.Args(pbBackupCmd, []string{"full"}); err != nil {
		t.Fatalf("single backup type must pass: %v", err)
	}
	if err := pbBackupCmd.Args(pbBackupCmd, []string{"full", "extra"}); err == nil {
		t.Fatal("extra positional after backup type must be rejected")
	}
	if err := pbLsCmd.Args(pbLsCmd, []string{"repo", "extra"}); err == nil {
		t.Fatal("extra positional after ls type must be rejected")
	}
}

// TestPbBackupAnnotationNotIdempotent guards against agents treating backup
// retries as free: every run creates a new backup set.
func TestPbBackupAnnotationNotIdempotent(t *testing.T) {
	if got := pbBackupCmd.Annotations["idempotent"]; got != "false" {
		t.Fatalf("pb backup idempotent annotation = %q, want false", got)
	}
}

// TestPbInfoRawYAMLStructuredParamError verifies raw parameter errors surface
// as pb-scoped structured errors instead of falling through to the generic
// system fallback.
func TestPbInfoRawYAMLStructuredParamError(t *testing.T) {
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

	pbInfoRaw = true
	pbInfoRawOutput = ""
	pbInfoSet = ""
	config.OutputFormat = config.OUTPUT_YAML

	var runErr error
	raw := capturePbStdout(t, func() {
		runErr = pbInfoCmd.RunE(pbInfoCmd, nil)
	})
	if runErr == nil {
		t.Fatal("raw mode with YAML output must fail")
	}
	exitErr, ok := runErr.(*utils.ExitCodeError)
	if !ok {
		t.Fatalf("expected ExitCodeError, got %T: %v", runErr, runErr)
	}
	if want := output.ExitCode(output.CodePbInvalidInfoParams); exitErr.Code != want {
		t.Fatalf("exit code = %d, want %d", exitErr.Code, want)
	}
	if !strings.Contains(raw, "invalid pb info raw parameters") {
		t.Fatalf("expected structured pb param error in output, got %q", raw)
	}
}

// TestPbRestoreStructuredGateSuggestsReplayableCommand verifies the
// confirmation gate emits concrete commands (no "..." placeholders) that
// preserve the user's restore flags.
func TestPbRestoreStructuredGateSuggestsReplayableCommand(t *testing.T) {
	origFormat := config.OutputFormat
	origTime := pbRestoreTime
	origYes := pbRestoreYes
	origPlan := pbRestorePlan
	origConfigPath := pbConfig.ConfigPath
	defer func() {
		config.OutputFormat = origFormat
		pbRestoreTime = origTime
		pbRestoreYes = origYes
		pbRestorePlan = origPlan
		pbConfig.ConfigPath = origConfigPath
	}()

	config.OutputFormat = config.OUTPUT_JSON
	pbRestoreTime = "2025-01-01 00:00:00+08"
	pbRestoreYes = false
	pbRestorePlan = false
	// Point at a missing config so stanza pinning stays environment-independent.
	pbConfig.ConfigPath = filepath.Join(t.TempDir(), "missing.conf")

	var runErr error
	raw := capturePbStdout(t, func() {
		runErr = pbRestoreCmd.RunE(pbRestoreCmd, nil)
	})
	if runErr == nil {
		t.Fatal("structured pb restore without --yes must fail closed")
	}

	var payload struct {
		NextActions []output.NextAction `json:"next_actions"`
	}
	if err := json.Unmarshal(pbBytesTrimSpace([]byte(raw)), &payload); err != nil {
		t.Fatalf("invalid json output: %v raw=%q", err, raw)
	}
	if len(payload.NextActions) == 0 {
		t.Fatalf("expected next_actions in confirmation error, got %q", raw)
	}
	exec := payload.NextActions[0]
	if !exec.Required {
		t.Fatalf("first next action should be the required execute command: %+v", payload.NextActions)
	}
	for _, want := range []string{"pig pb restore", "--time", "2025-01-01 00:00:00+08", "--yes"} {
		if !strings.Contains(exec.Command, want) {
			t.Errorf("execute command %q missing %q", exec.Command, want)
		}
	}
	if strings.Contains(exec.Command, "...") {
		t.Errorf("execute command must not contain placeholder ellipsis: %q", exec.Command)
	}
}

// TestPbDeleteStructuredAmbiguousStanza verifies multi-stanza hosts refuse
// deletion without an explicit --stanza before any confirmation gating.
func TestPbDeleteStructuredAmbiguousStanza(t *testing.T) {
	origFormat := config.OutputFormat
	origYes := pbDeleteYes
	origPlan := pbDeletePlan
	origConfigPath := pbConfig.ConfigPath
	origStanza := pbConfig.Stanza
	defer func() {
		config.OutputFormat = origFormat
		pbDeleteYes = origYes
		pbDeletePlan = origPlan
		pbConfig.ConfigPath = origConfigPath
		pbConfig.Stanza = origStanza
	}()

	confPath := filepath.Join(t.TempDir(), "pgbackrest.conf")
	conf := "[global]\nrepo1-path=/pg/backup\n\n[pg-meta]\npg1-path=/data/a\n\n[pg-test]\npg1-path=/data/b\n"
	if err := os.WriteFile(confPath, []byte(conf), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	config.OutputFormat = config.OUTPUT_JSON
	pbDeleteYes = true // even with --yes, ambiguity must refuse first
	pbDeletePlan = false
	pbConfig.ConfigPath = confPath
	pbConfig.Stanza = ""

	var runErr error
	raw := capturePbStdout(t, func() {
		runErr = pbDeleteCmd.RunE(pbDeleteCmd, nil)
	})
	if runErr == nil {
		t.Fatal("ambiguous stanza delete must fail")
	}

	var payload struct {
		Code        int                 `json:"code"`
		NextActions []output.NextAction `json:"next_actions"`
	}
	if err := json.Unmarshal(pbBytesTrimSpace([]byte(raw)), &payload); err != nil {
		t.Fatalf("invalid json output: %v raw=%q", err, raw)
	}
	if payload.Code != output.CodePbAmbiguousStanza {
		t.Fatalf("code = %d, want CodePbAmbiguousStanza(%d)", payload.Code, output.CodePbAmbiguousStanza)
	}
	for _, action := range payload.NextActions {
		if strings.Contains(action.Command, "--yes") {
			t.Errorf("ambiguity refusal must not suggest --yes execution: %q", action.Command)
		}
	}
}

// TestPbDeleteStructuredGatePinsSingleStanza verifies the confirmation gate
// suggests a delete command pinned to the (single) resolved stanza.
func TestPbDeleteStructuredGatePinsSingleStanza(t *testing.T) {
	origFormat := config.OutputFormat
	origYes := pbDeleteYes
	origPlan := pbDeletePlan
	origConfigPath := pbConfig.ConfigPath
	origStanza := pbConfig.Stanza
	defer func() {
		config.OutputFormat = origFormat
		pbDeleteYes = origYes
		pbDeletePlan = origPlan
		pbConfig.ConfigPath = origConfigPath
		pbConfig.Stanza = origStanza
	}()

	confPath := filepath.Join(t.TempDir(), "pgbackrest.conf")
	conf := "[global]\nrepo1-path=/pg/backup\n\n[pg-meta]\npg1-path=/data/a\n"
	if err := os.WriteFile(confPath, []byte(conf), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	config.OutputFormat = config.OUTPUT_JSON
	pbDeleteYes = false
	pbDeletePlan = false
	pbConfig.ConfigPath = confPath
	pbConfig.Stanza = ""

	var runErr error
	raw := capturePbStdout(t, func() {
		runErr = pbDeleteCmd.RunE(pbDeleteCmd, nil)
	})
	if runErr == nil {
		t.Fatal("structured pb delete without --yes must fail closed")
	}

	var payload struct {
		Code        int                 `json:"code"`
		NextActions []output.NextAction `json:"next_actions"`
	}
	if err := json.Unmarshal(pbBytesTrimSpace([]byte(raw)), &payload); err != nil {
		t.Fatalf("invalid json output: %v raw=%q", err, raw)
	}
	if payload.Code != output.CodePbConfirmationRequired {
		t.Fatalf("code = %d, want CodePbConfirmationRequired(%d)", payload.Code, output.CodePbConfirmationRequired)
	}
	if len(payload.NextActions) == 0 {
		t.Fatalf("expected next_actions, got %q", raw)
	}
	exec := payload.NextActions[0]
	if !strings.Contains(exec.Command, "--stanza pg-meta") || !strings.Contains(exec.Command, "--yes") {
		t.Errorf("execute command should pin the resolved stanza: %q", exec.Command)
	}
}
