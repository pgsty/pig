package cmd

import "testing"

func TestPITRSupportsPlanAndDryRun(t *testing.T) {
	if pitrCmd.Flags().Lookup("plan") == nil {
		t.Fatal("--plan flag not found on pitr command")
	}
	if pitrCmd.Flags().Lookup("dry-run") == nil {
		t.Fatal("--dry-run alias not found on pitr command")
	}
}

func TestPgRepackSupportsPlanAndDryRun(t *testing.T) {
	if pgRepackCmd.Flags().Lookup("plan") == nil {
		t.Fatal("--plan flag not found on pg repack command")
	}
	if pgRepackCmd.Flags().Lookup("dry-run") == nil {
		t.Fatal("--dry-run alias not found on pg repack command")
	}
}

func TestPbExpireSupportsPlanAndDryRun(t *testing.T) {
	if pbExpireCmd.Flags().Lookup("plan") == nil {
		t.Fatal("--plan flag not found on pb expire command")
	}
	if pbExpireCmd.Flags().Lookup("dry-run") == nil {
		t.Fatal("--dry-run flag not found on pb expire command")
	}
}
