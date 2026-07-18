package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	stycli "pig/cli/sty"
	"pig/internal/config"
	"pig/internal/output"

	"github.com/spf13/cobra"
)

func TestStyCMDBCompatibilityTree(t *testing.T) {
	for _, test := range []struct {
		args []string
		want *cobra.Command
	}{
		{[]string{"sty", "cmdb"}, pigstyCMDBCmd},
		{[]string{"sty", "cmdb", "init"}, pigstyCMDBInitCmd},
		{[]string{"sty", "cmdb", "load"}, pigstyCMDBLoadCmd},
		{[]string{"sty", "cmdb", "dump"}, pigstyCMDBDumpCmd},
		{[]string{"sty", "cmdb", "enable"}, pigstyCMDBEnableCmd},
		{[]string{"sty", "cmdb", "disable"}, pigstyCMDBDisableCmd},
	} {
		found, _, err := rootCmd.Find(test.args)
		if err != nil || found != test.want {
			t.Fatalf("Find(%v) = %v, %v; want %v", test.args, found, err, test.want)
		}
	}
	for _, command := range pigstyCMDBCmd.Commands() {
		if command.Name() == "check" || command.Name() == "_inventory" {
			t.Fatalf("abandoned CMDB command remains registered: %s", command.Name())
		}
	}
}

func TestStyCMDBCompatibilityFlagsAndHelp(t *testing.T) {
	for _, command := range []*cobra.Command{pigstyCMDBInitCmd, pigstyCMDBLoadCmd, pigstyCMDBDumpCmd, pigstyCMDBEnableCmd} {
		for _, flag := range []string{"database", "data"} {
			if command.Flags().Lookup(flag) == nil {
				t.Fatalf("%s missing --%s", command.CommandPath(), flag)
			}
		}
	}
	if pigstyCMDBInitCmd.Flags().Lookup("database").Shorthand != "d" || pigstyCMDBInitCmd.Flags().Lookup("yes") == nil {
		t.Fatal("pig sty cmdb compatibility flags do not match the root contract")
	}
	if pigstyCMDBDisableCmd.Flags().Lookup("database") != nil {
		t.Fatal("pig sty cmdb disable must not expose an unused database flag")
	}
	for _, flag := range []string{"connection-file", "reset", "restore", "revision", "prune", "merge", "expect-base", "static"} {
		if pigstyCMDBCmd.PersistentFlags().Lookup(flag) != nil || pigstyCMDBInitCmd.Flags().Lookup(flag) != nil ||
			pigstyCMDBLoadCmd.Flags().Lookup(flag) != nil || pigstyCMDBDumpCmd.Flags().Lookup(flag) != nil ||
			pigstyCMDBDisableCmd.Flags().Lookup(flag) != nil {
			t.Fatalf("abandoned CMDB flag remains registered: --%s", flag)
		}
	}
	original := pigstyCMDBCmd.OutOrStdout()
	defer pigstyCMDBCmd.SetOut(original)
	var help bytes.Buffer
	pigstyCMDBCmd.SetOut(&help)
	if err := pigstyCMDBCmd.Help(); err != nil {
		t.Fatal(err)
	}
	for _, stale := range []string{"pigsty_cmdb", "revision", "rollback", "merge", "append-only"} {
		if strings.Contains(strings.ToLower(help.String()), stale) {
			t.Fatalf("compatibility help still advertises %q:\n%s", stale, help.String())
		}
	}
}

func TestStyCMDBDelegatesToRootUseCases(t *testing.T) {
	originalInit, originalLoad := inventoryCMDBInitExec, inventoryCMDBLoadExec
	originalDump := inventoryCMDBDumpExec
	originalEnable, originalDisable := inventoryCMDBEnableExec, inventoryCMDBDisableExec
	originalHome, originalConfig := config.PigstyHome, config.PigstyConfig
	originalOSCode, originalOSArch := config.OSCode, config.OSArch
	originalDatabase, originalYes := pigstyCMDBDatabase, pigstyCMDBInitYes
	defer func() {
		inventoryCMDBInitExec, inventoryCMDBLoadExec = originalInit, originalLoad
		inventoryCMDBDumpExec = originalDump
		inventoryCMDBEnableExec, inventoryCMDBDisableExec = originalEnable, originalDisable
		config.PigstyHome, config.PigstyConfig = originalHome, originalConfig
		config.OSCode, config.OSArch = originalOSCode, originalOSArch
		pigstyCMDBDatabase, pigstyCMDBInitYes = originalDatabase, originalYes
		pigstyCMDBDumpCmd.SetOut(nil)
	}()
	config.PigstyHome, config.PigstyConfig = "/tmp/pigsty", "/tmp/pigsty/pigsty.yml"
	config.OSCode, config.OSArch = "d12", "arm64"
	pigstyCMDBDatabase, pigstyCMDBInitYes = "meta-test", true

	var initialized, loaded, enabled, disabled stycli.InventoryCMDBOptions
	inventoryCMDBInitExec = func(_ context.Context, options stycli.InventoryCMDBOptions) *output.Result {
		initialized = options
		return output.OK("initialized", nil)
	}
	inventoryCMDBLoadExec = func(_ context.Context, options stycli.InventoryCMDBOptions) *output.Result {
		loaded = options
		return output.OK("loaded", nil)
	}
	inventoryCMDBDumpExec = func(_ context.Context, options stycli.InventoryCMDBOptions) ([]byte, *output.Result) {
		if options.Path != "" || options.Database != "meta-test" {
			t.Fatalf("dump options = %+v", options)
		}
		return []byte("all: {}\n"), output.OK("dumped", nil)
	}
	inventoryCMDBEnableExec = func(_ context.Context, options stycli.InventoryCMDBOptions) *output.Result {
		enabled = options
		return output.OK("enabled", nil)
	}
	inventoryCMDBDisableExec = func(options stycli.InventoryCMDBOptions) *output.Result {
		disabled = options
		return output.OK("disabled", nil)
	}

	_ = captureStdout(t, func() {
		if err := pigstyCMDBInitCmd.RunE(pigstyCMDBInitCmd, nil); err != nil {
			t.Fatal(err)
		}
		if err := pigstyCMDBLoadCmd.RunE(pigstyCMDBLoadCmd, []string{"/tmp/explicit.yml"}); err != nil {
			t.Fatal(err)
		}
		if err := pigstyCMDBEnableCmd.RunE(pigstyCMDBEnableCmd, nil); err != nil {
			t.Fatal(err)
		}
		if err := pigstyCMDBDisableCmd.RunE(pigstyCMDBDisableCmd, nil); err != nil {
			t.Fatal(err)
		}
	})
	var dump bytes.Buffer
	pigstyCMDBDumpCmd.SetOut(&dump)
	if err := pigstyCMDBDumpCmd.RunE(pigstyCMDBDumpCmd, nil); err != nil {
		t.Fatal(err)
	}
	if !initialized.Yes || initialized.Database != "meta-test" || initialized.Home != config.PigstyHome {
		t.Fatalf("init options = %+v", initialized)
	}
	if loaded.Path != "/tmp/explicit.yml" || loaded.OSCode != "d12" || loaded.OSArch != "arm64" {
		t.Fatalf("load options = %+v", loaded)
	}
	if enabled.Home != config.PigstyHome || disabled.Home != config.PigstyHome || dump.String() != "all: {}\n" {
		t.Fatalf("delegation: enabled=%+v disabled=%+v dump=%q", enabled, disabled, dump.String())
	}
}
