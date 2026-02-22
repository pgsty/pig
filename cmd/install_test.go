package cmd

import (
	"errors"
	"testing"

	"pig/internal/output"
	"pig/internal/utils"
)

func TestInstallPlanFlagExists(t *testing.T) {
	if installCmd.Flags().Lookup("plan") == nil {
		t.Fatal("--plan flag not found on install command")
	}
	if installCmd.Flags().Lookup("dry-run") != nil {
		t.Fatal("--dry-run should not exist on install command")
	}
}

func TestInstallPlanRequiresTargets(t *testing.T) {
	oldPlan := installPlan
	oldYes := installYes
	oldNoTranslation := installNoTranslation
	defer func() {
		installPlan = oldPlan
		installYes = oldYes
		installNoTranslation = oldNoTranslation
	}()

	installPlan = true
	installYes = false
	installNoTranslation = false

	err := installCmd.RunE(installCmd, []string{})
	if err == nil {
		t.Fatal("expected error for install --plan without targets")
	}

	var exitErr *utils.ExitCodeError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitCodeError, got %T: %v", err, err)
	}
	if exitErr.Code != output.ExitCode(output.CodeExtensionInvalidArgs) {
		t.Fatalf("unexpected exit code: got %d want %d", exitErr.Code, output.ExitCode(output.CodeExtensionInvalidArgs))
	}
}
