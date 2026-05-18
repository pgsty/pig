package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestPatroniRestartRejectsExtraPositionals(t *testing.T) {
	if patroniRestartCmd.Args == nil {
		t.Fatal("restart command must validate positional argument count")
	}

	if err := patroniRestartCmd.Args(patroniRestartCmd, nil); err != nil {
		t.Fatalf("restart with no member should be accepted: %v", err)
	}
	if err := patroniRestartCmd.Args(patroniRestartCmd, []string{"pg-nms-1"}); err != nil {
		t.Fatalf("restart with one member should be accepted: %v", err)
	}
	if err := patroniRestartCmd.Args(patroniRestartCmd, []string{"pg-nms", "pg-nms-1"}); err == nil {
		t.Fatal("restart must reject cluster+member positionals instead of silently dropping the second argument")
	}
}

func TestPatroniClusterCommandsRejectIgnoredPositionals(t *testing.T) {
	tests := []struct {
		name string
		cmd  *cobra.Command
	}{
		{name: "reload", cmd: patroniReloadCmd},
		{name: "switchover", cmd: patroniSwitchoverCmd},
		{name: "failover", cmd: patroniFailoverCmd},
		{name: "pause", cmd: patroniPauseCmd},
		{name: "resume", cmd: patroniResumeCmd},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.cmd.Args == nil {
				t.Fatalf("%s command must validate positional argument count", tt.name)
			}
			if err := tt.cmd.Args(tt.cmd, []string{"ignored"}); err == nil {
				t.Fatalf("%s should reject unexpected positional args", tt.name)
			}
		})
	}
}

func TestPatroniListAcceptsOptionalCluster(t *testing.T) {
	if patroniListCmd.Args == nil {
		t.Fatal("list command must validate positional argument count")
	}
	if err := patroniListCmd.Args(patroniListCmd, nil); err != nil {
		t.Fatalf("list without cluster should be accepted: %v", err)
	}
	if err := patroniListCmd.Args(patroniListCmd, []string{"pg-meta"}); err != nil {
		t.Fatalf("list with one cluster should be accepted: %v", err)
	}
	if err := patroniListCmd.Args(patroniListCmd, []string{"pg-meta", "pg-test"}); err == nil {
		t.Fatal("list should reject more than one cluster positional")
	}
}
