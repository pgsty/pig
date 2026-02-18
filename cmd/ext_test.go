package cmd

import (
	"errors"
	"testing"

	"pig/internal/output"
	"pig/internal/utils"
)

func TestExtCommandsRejectUnexpectedArgs(t *testing.T) {
	tests := []struct {
		name string
		cmd  func() error
	}{
		{
			name: "status",
			cmd: func() error {
				return extStatusCmd.Args(extStatusCmd, []string{"unexpected"})
			},
		},
		{
			name: "scan",
			cmd: func() error {
				return extScanCmd.Args(extScanCmd, []string{"unexpected"})
			},
		},
		{
			name: "reload",
			cmd: func() error {
				return extReloadCmd.Args(extReloadCmd, []string{"unexpected"})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.cmd(); err == nil {
				t.Fatalf("%s should reject unexpected args", tt.name)
			}
		})
	}
}

func TestExtAddPlanRequiresTargets(t *testing.T) {
	oldPlan := extAddPlan
	oldYes := extYes
	oldOutput := outputFormat
	defer func() {
		extAddPlan = oldPlan
		extYes = oldYes
		outputFormat = oldOutput
	}()

	extAddPlan = true
	extYes = false
	outputFormat = "text"

	err := extAddCmd.RunE(extAddCmd, []string{})
	if err == nil {
		t.Fatal("expected error for ext add --plan without targets")
	}
	var exitErr *utils.ExitCodeError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitCodeError, got %T: %v", err, err)
	}
	if exitErr.Code != output.ExitCode(output.CodeExtensionInvalidArgs) {
		t.Fatalf("unexpected exit code: got %d want %d", exitErr.Code, output.ExitCode(output.CodeExtensionInvalidArgs))
	}
}

func TestExtRmPlanRequiresTargets(t *testing.T) {
	oldPlan := extRmPlan
	oldYes := extYes
	oldOutput := outputFormat
	defer func() {
		extRmPlan = oldPlan
		extYes = oldYes
		outputFormat = oldOutput
	}()

	extRmPlan = true
	extYes = false
	outputFormat = "text"

	err := extRmCmd.RunE(extRmCmd, []string{})
	if err == nil {
		t.Fatal("expected error for ext rm --plan without targets")
	}
	var exitErr *utils.ExitCodeError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitCodeError, got %T: %v", err, err)
	}
	if exitErr.Code != output.ExitCode(output.CodeExtensionInvalidArgs) {
		t.Fatalf("unexpected exit code: got %d want %d", exitErr.Code, output.ExitCode(output.CodeExtensionInvalidArgs))
	}
}
