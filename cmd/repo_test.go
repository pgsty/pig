package cmd

import (
	"errors"
	"testing"

	"pig/internal/config"
	"pig/internal/utils"
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
