package cmd

import "testing"

func TestBuildRustMirrorFlagVisible(t *testing.T) {
	flag := buildRustCmd.Flags().Lookup("mirror")
	if flag == nil {
		t.Fatalf("pig build rust missing --mirror flag")
	}
	if flag.Shorthand != "m" {
		t.Fatalf("pig build rust --mirror shorthand = %q, want m", flag.Shorthand)
	}
	if flag.Hidden {
		t.Fatalf("pig build rust --mirror should be visible")
	}
}
