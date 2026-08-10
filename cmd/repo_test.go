package cmd

import (
	"errors"
	"strings"
	"testing"

	"pig/internal/config"
	"pig/internal/utils"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestRepoCreateDocumentsMacOSPrivilegeException(t *testing.T) {
	if got := repoCreateCmd.Annotations["os_user"]; got != "root" {
		t.Fatalf("repo create os_user = %q, want root", got)
	}
	for _, want := range []string{
		"Linux defaults to /www/pigsty and requires sudo/root",
		"macOS defaults to the current directory and runs as the current user",
	} {
		if !strings.Contains(repoCreateCmd.Example, want) {
			t.Fatalf("repo create example does not document %q:\n%s", want, repoCreateCmd.Example)
		}
	}
}

func TestRepoSetDoesNotMutateGlobalFlags(t *testing.T) {
	origType := config.OSType
	origVendor := config.OSVendor
	origVersion := config.OSVersionFull
	origRemove := repoAddRemove
	origUpdate := repoAddUpdate
	defer func() {
		config.OSType = origType
		config.OSVendor = origVendor
		config.OSVersionFull = origVersion
		repoAddRemove = origRemove
		repoAddUpdate = origUpdate
	}()

	config.OSType = config.DistroMAC
	config.OSVendor = "macOS"
	config.OSVersionFull = "14.0"
	repoAddRemove = false
	repoAddUpdate = false

	err := repoSetCmd.RunE(repoSetCmd, []string{"all"})
	if err == nil {
		t.Fatal("expected repo set to fail on unsupported platforms")
	}
	var exitErr *utils.ExitCodeError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitCodeError, got %T: %v", err, err)
	}
	if repoAddRemove {
		t.Fatal("repo set should not mutate repo add --remove")
	}
	if repoAddUpdate {
		t.Fatal("repo set should not mutate repo add --update")
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

func TestSiblingFlagStateIsIsolated(t *testing.T) {
	tests := []struct {
		name     string
		flagName string
		commands []*cobra.Command
		values   []string
	}{
		{name: "repo region", flagName: "region", commands: []*cobra.Command{repoAddCmd, repoSetCmd}, values: []string{"cn", "us"}},
		{name: "repo mirror", flagName: "mirror", commands: []*cobra.Command{repoAddCmd, repoSetCmd, buildRepoCmd}, values: []string{"true", "false", "true"}},
		{name: "repo update", flagName: "update", commands: []*cobra.Command{repoAddCmd, repoRmCmd}, values: []string{"true", "false"}},
		{name: "build mirror", flagName: "mirror", commands: []*cobra.Command{buildSpecCmd, buildGetCmd, buildPkgCmd}, values: []string{"true", "false", "true"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags := make([]*pflag.Flag, len(tt.commands))
			expected := make([]string, len(tt.commands))
			for i, command := range tt.commands {
				flag := command.Flags().Lookup(tt.flagName)
				if flag == nil {
					t.Fatalf("%s missing --%s flag", command.CommandPath(), tt.flagName)
				}
				flags[i] = flag
				expected[i] = flag.DefValue
				oldValue, oldChanged := flag.Value.String(), flag.Changed
				t.Cleanup(func() {
					if err := flag.Value.Set(oldValue); err != nil {
						t.Errorf("restore %s --%s: %v", command.CommandPath(), tt.flagName, err)
					}
					flag.Changed = oldChanged
				})
				if err := flag.Value.Set(flag.DefValue); err != nil {
					t.Fatalf("reset %s --%s: %v", command.CommandPath(), tt.flagName, err)
				}
				flag.Changed = false
			}

			for i, command := range tt.commands {
				if err := command.Flags().Set(tt.flagName, tt.values[i]); err != nil {
					t.Fatalf("set %s --%s: %v", command.CommandPath(), tt.flagName, err)
				}
				expected[i] = tt.values[i]
				for j, flag := range flags {
					if got := flag.Value.String(); got != expected[j] {
						t.Fatalf("after %s --%s=%s, sibling %s value = %s, want %s", command.CommandPath(), tt.flagName, tt.values[i], tt.commands[j].CommandPath(), got, expected[j])
					}
				}
			}
		})
	}
}
