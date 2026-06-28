package cmd

import (
	"fmt"
	"testing"

	postgrescli "pig/cli/postgres"
	"pig/internal/output"
)

func TestPgForkCommandIsRegistered(t *testing.T) {
	pgFork, _, err := rootCmd.Find([]string{"pg", "fork"})
	if err != nil {
		t.Fatalf("pg fork command not found: %v", err)
	}
	if pgFork == nil || pgFork.Name() != "fork" {
		t.Fatalf("pg fork command = %v, want fork", pgFork)
	}
}

func TestTopLevelForkIsNotRegistered(t *testing.T) {
	rootFork, _, err := rootCmd.Find([]string{"fork"})
	if err == nil || rootFork != rootCmd {
		t.Fatalf("top-level fork should not be registered, got cmd=%v err=%v", rootFork, err)
	}
}

func TestPgForkSupportsPlanAndDryRun(t *testing.T) {
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
}

func TestPgForkRegistersLifecycleSubcommands(t *testing.T) {
	for _, args := range [][]string{
		{"pg", "fork", "init"},
		{"pg", "fork", "list"},
		{"pg", "fork", "start"},
		{"pg", "fork", "stop"},
		{"pg", "fork", "rm"},
	} {
		cmd, _, err := rootCmd.Find(args)
		if err != nil {
			t.Fatalf("%v command not found: %v", args, err)
		}
		if cmd == nil || cmd.Name() != args[len(args)-1] {
			t.Fatalf("%v resolved to %v", args, cmd)
		}
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
	for _, name := range []string{"list", "force", "run", "src-data", "src-port", "dst-data", "dst-port"} {
		if pgFork.Flags().Lookup(name) == nil {
			t.Fatalf("pg fork should expose --%s", name)
		}
	}
	for _, name := range []string{"no-start", "replace", "mode", "data", "dst", "port"} {
		if pgFork.LocalFlags().Lookup(name) != nil {
			t.Fatalf("pg fork should not expose old --%s flag", name)
		}
	}
}

func TestPgForkInitExposesCreateFlags(t *testing.T) {
	pgForkInit, _, err := rootCmd.Find([]string{"pg", "fork", "init"})
	if err != nil {
		t.Fatalf("pg fork init command not found: %v", err)
	}
	for _, name := range []string{"force", "run", "src-data", "src-port", "dst-data", "dst-port", "timeout"} {
		if pgForkInit.Flags().Lookup(name) == nil {
			t.Fatalf("pg fork init should expose --%s", name)
		}
	}
	for _, name := range []string{"data", "dst", "port"} {
		if pgForkInit.LocalFlags().Lookup(name) != nil {
			t.Fatalf("pg fork init should not expose old --%s flag", name)
		}
	}
}

func TestPgForkLifecycleCommandsExposeDstEscapeHatch(t *testing.T) {
	for _, args := range [][]string{
		{"pg", "fork", "start"},
		{"pg", "fork", "stop"},
		{"pg", "fork", "rm"},
	} {
		cmd, _, err := rootCmd.Find(args)
		if err != nil {
			t.Fatalf("%v command not found: %v", args, err)
		}
		if cmd.Flags().Lookup("dst-data") == nil {
			t.Fatalf("%v should expose --dst-data for unmanaged forks", args)
		}
		if cmd.LocalFlags().Lookup("dst") != nil {
			t.Fatalf("%v should not expose old --dst flag", args)
		}
	}
}

func TestPgForkStartExposesDestinationPortOverride(t *testing.T) {
	pgForkStart, _, err := rootCmd.Find([]string{"pg", "fork", "start"})
	if err != nil {
		t.Fatalf("pg fork start command not found: %v", err)
	}
	if pgForkStart.Flags().Lookup("dst-port") == nil {
		t.Fatal("pg fork start should expose --dst-port")
	}
	if pgForkStart.LocalFlags().Lookup("port") != nil {
		t.Fatal("pg fork start should not expose old --port flag")
	}
}

func TestBuildInstanceOptionsUsesForkSourceAndDestinationFlags(t *testing.T) {
	oldPgData := pgConfig.PgData
	pgConfig.PgData = "/pg/data-parent"
	t.Cleanup(func() {
		pgConfig.PgData = oldPgData
	})

	opts := buildInstanceOptions(&forkCLIOptions{
		sourceData: "/pg/data-source",
		sourcePort: 15431,
		destData:   "/tmp/dev-fork",
		destPort:   15432,
	}, "dev")

	if opts.Instance.SourceData != "/pg/data-source" {
		t.Fatalf("SourceData = %q, want fork --src-data override", opts.Instance.SourceData)
	}
	if opts.Instance.SourcePort != 15431 {
		t.Fatalf("SourcePort = %d, want 15431", opts.Instance.SourcePort)
	}
	if opts.Instance.DestData != "/tmp/dev-fork" {
		t.Fatalf("DestData = %q, want /tmp/dev-fork", opts.Instance.DestData)
	}
	if opts.Instance.DestPort != 15432 {
		t.Fatalf("DestPort = %d, want 15432", opts.Instance.DestPort)
	}
}

func TestForkListStatusUsesMinimalState(t *testing.T) {
	tests := []struct {
		name string
		info postgrescli.ForkInfo
		want string
	}{
		{"orphan", postgrescli.ForkInfo{Orphan: true}, "orphan"},
		{"normal fork", postgrescli.ForkInfo{Target: postgrescli.ForkEndpoint{Started: false}}, "forked"},
		{"started at creation time", postgrescli.ForkInfo{Target: postgrescli.ForkEndpoint{Started: true}}, "forked"},
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
	result := forkErrorResult(&postgrescli.ForkError{
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
