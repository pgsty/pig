package cmd

import (
	"fmt"
	"testing"

	forkpkg "pig/cli/fork"
	"pig/internal/output"
)

func TestPgForkAndCloneCommandsAreRegistered(t *testing.T) {
	pgFork, _, err := rootCmd.Find([]string{"pg", "fork"})
	if err != nil {
		t.Fatalf("pg fork command not found: %v", err)
	}
	if pgFork == nil || pgFork.Name() != "fork" {
		t.Fatalf("pg fork command = %v, want fork", pgFork)
	}

	pgClone, _, err := rootCmd.Find([]string{"pg", "clone"})
	if err != nil {
		t.Fatalf("pg clone command not found: %v", err)
	}
	if pgClone == nil || pgClone.Name() != "clone" {
		t.Fatalf("pg clone command = %v, want clone", pgClone)
	}
}

func TestTopLevelForkIsNotRegistered(t *testing.T) {
	rootFork, _, err := rootCmd.Find([]string{"fork"})
	if err == nil || rootFork != rootCmd {
		t.Fatalf("top-level fork should not be registered, got cmd=%v err=%v", rootFork, err)
	}
}

func TestPgForkAndCloneSupportPlanAndDryRun(t *testing.T) {
	pgFork, _, err := rootCmd.Find([]string{"pg", "fork"})
	if err != nil {
		t.Fatalf("pg fork command not found: %v", err)
	}
	if pgFork.PersistentFlags().Lookup("plan") == nil {
		t.Fatal("--plan flag not found on pg fork command")
	}
	if pgFork.PersistentFlags().Lookup("dry-run") == nil {
		t.Fatal("--dry-run alias not found on pg fork command")
	}

	pgClone, _, err := rootCmd.Find([]string{"pg", "clone"})
	if err != nil {
		t.Fatalf("pg clone command not found: %v", err)
	}
	if pgClone.PersistentFlags().Lookup("plan") == nil {
		t.Fatal("--plan flag not found on pg clone command")
	}
	if pgClone.PersistentFlags().Lookup("dry-run") == nil {
		t.Fatal("--dry-run alias not found on pg clone command")
	}
}

func TestPgForkDoesNotUseRootCommandGroup(t *testing.T) {
	pgFork, _, err := rootCmd.Find([]string{"pg", "fork"})
	if err != nil {
		t.Fatalf("pg fork command not found: %v", err)
	}
	if pgFork.GroupID != "" {
		t.Fatalf("pg fork GroupID = %q, want empty", pgFork.GroupID)
	}
}

func TestPgForkExposesNamedForkFlags(t *testing.T) {
	pgFork, _, err := rootCmd.Find([]string{"pg", "fork"})
	if err != nil {
		t.Fatalf("pg fork command not found: %v", err)
	}
	for _, name := range []string{"list", "force", "run", "port", "data", "src-port", "src-data"} {
		if pgFork.Flags().Lookup(name) == nil {
			t.Fatalf("pg fork should expose --%s", name)
		}
	}
	for _, name := range []string{"no-start", "replace", "mode", "dst", "dst-port"} {
		if pgFork.Flags().Lookup(name) != nil {
			t.Fatalf("pg fork should not expose old --%s flag", name)
		}
	}
}

func TestPgCloneDoesNotExposeInstanceOnlyFlags(t *testing.T) {
	pgClone, _, err := rootCmd.Find([]string{"pg", "clone"})
	if err != nil {
		t.Fatalf("pg clone command not found: %v", err)
	}
	for _, name := range []string{"no-start", "replace", "mode"} {
		if pgClone.Flags().Lookup(name) != nil {
			t.Fatalf("pg clone should not expose --%s", name)
		}
	}
}

func TestForkListStatusUsesMinimalState(t *testing.T) {
	tests := []struct {
		name string
		info forkpkg.ForkInfo
		want string
	}{
		{"orphan", forkpkg.ForkInfo{Orphan: true}, "orphan"},
		{"normal fork", forkpkg.ForkInfo{Target: forkpkg.ForkEndpoint{Started: false}}, "forked"},
		{"started at creation time", forkpkg.ForkInfo{Target: forkpkg.ForkEndpoint{Started: true}}, "forked"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := forkListStatus(tt.info); got != tt.want {
				t.Fatalf("forkListStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestForkErrorResultPreservesForkErrorCode(t *testing.T) {
	result := forkErrorResult(&forkpkg.ForkError{
		Code: output.CodeForkInvalidArgs,
		Err:  fmt.Errorf("unsafe destination data directory: /"),
	})
	if result.Success {
		t.Fatal("fork error result should be unsuccessful")
	}
	if result.Code != output.CodeForkInvalidArgs {
		t.Fatalf("result code = %d, want %d", result.Code, output.CodeForkInvalidArgs)
	}
	if result.Message != "unsafe destination data directory: /" {
		t.Fatalf("result message = %q", result.Message)
	}
}
