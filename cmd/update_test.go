package cmd

import "testing"

func TestUpdateMirrorFlagUsesMirrorRegion(t *testing.T) {
	flag := updateCmd.Flags().Lookup("mirror")
	if flag == nil {
		t.Fatal("expected --mirror flag on pig update")
	}
	if flag.Shorthand != "m" {
		t.Fatalf("expected shorthand -m for mirror, got -%s", flag.Shorthand)
	}
	if flag.Hidden {
		t.Fatal("pig update --mirror should be visible")
	}

	origExec := updateExec
	origVersion := updateVersion
	origRegion := updateRegion
	origMirror := flag.Value.String()
	defer func() {
		updateExec = origExec
		updateVersion = origVersion
		updateRegion = origRegion
		_ = flag.Value.Set(origMirror)
	}()

	var gotVersion, gotRegion string
	updateExec = func(version, region string) error {
		gotVersion = version
		gotRegion = region
		return nil
	}

	updateVersion = "v1.2.3"
	updateRegion = "default"
	if err := flag.Value.Set("true"); err != nil {
		t.Fatalf("set --mirror: %v", err)
	}
	if err := updateCmd.RunE(updateCmd, nil); err != nil {
		t.Fatalf("pig update --mirror failed: %v", err)
	}
	if gotVersion != "1.2.3" {
		t.Fatalf("version = %q, want 1.2.3", gotVersion)
	}
	if gotRegion != "china" {
		t.Fatalf("region = %q, want china for pigsty.cc route", gotRegion)
	}
}
