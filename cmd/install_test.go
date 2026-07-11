package cmd

import "testing"

func TestInstallPlanFlagExists(t *testing.T) {
	if installCmd.Flags().Lookup("plan") == nil {
		t.Fatal("--plan flag not found on install command")
	}
	if installCmd.Flags().Lookup("dry-run") != nil {
		t.Fatal("--dry-run should not exist on install command")
	}
}

func TestInstallRequiresTargets(t *testing.T) {
	if err := installCmd.Args(installCmd, nil); err == nil {
		t.Fatal("install should require at least one target")
	}
}
