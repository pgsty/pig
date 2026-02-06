package cmd

import (
	"errors"
	"testing"

	"pig/cli/pitr"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"
)

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
