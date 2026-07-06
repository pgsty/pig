package cmd

import (
	"errors"
	"testing"

	"pig/internal/config"
	"pig/internal/utils"

	"github.com/spf13/cobra"
)

func TestRepoSetDoesNotMutateGlobalFlags(t *testing.T) {
	origType := config.OSType
	origVendor := config.OSVendor
	origVersion := config.OSVersionFull
	origRemove := repoRemove
	origUpdate := repoUpdate
	defer func() {
		config.OSType = origType
		config.OSVendor = origVendor
		config.OSVersionFull = origVersion
		repoRemove = origRemove
		repoUpdate = origUpdate
	}()

	config.OSType = config.DistroMAC
	config.OSVendor = "macOS"
	config.OSVersionFull = "14.0"
	repoRemove = false
	repoUpdate = false

	err := repoSetCmd.RunE(repoSetCmd, []string{"all"})
	if err == nil {
		t.Fatal("expected repo set to fail on unsupported platforms")
	}
	var exitErr *utils.ExitCodeError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitCodeError, got %T: %v", err, err)
	}
	if repoRemove {
		t.Fatal("repo set should not mutate repoRemove")
	}
	if repoUpdate {
		t.Fatal("repo set should not mutate repoUpdate")
	}
}

func TestMirrorFlagVisibleOnRepoAndBuildRepo(t *testing.T) {
	for _, cmd := range []*cobra.Command{repoAddCmd, repoSetCmd, buildRepoCmd} {
		flag := cmd.Flags().Lookup("mirror")
		if flag == nil {
			t.Fatalf("%s missing --mirror flag", cmd.CommandPath())
		}
		if flag.Shorthand != "m" {
			t.Fatalf("%s --mirror shorthand = %q, want m", cmd.CommandPath(), flag.Shorthand)
		}
		if flag.Hidden {
			t.Fatalf("%s --mirror should be visible", cmd.CommandPath())
		}
		if oldFlag := cmd.Flags().Lookup("pgdg-proxy"); oldFlag != nil {
			t.Fatalf("%s should not keep old --pgdg-proxy flag", cmd.CommandPath())
		}
	}
}
