package cmd

import (
	"bytes"
	"context"
	"testing"

	stycli "pig/cli/sty"
	"pig/internal/config"
	"pig/internal/output"

	"github.com/spf13/cobra"
)

func TestInventoryRootCommandTree(t *testing.T) {
	for _, test := range []struct {
		args []string
		want *cobra.Command
	}{
		{[]string{"inventory"}, inventoryCmd},
		{[]string{"inv"}, inventoryCmd},
		{[]string{"inventory", "list"}, inventoryListCmd},
		{[]string{"inv", "ls"}, inventoryListCmd},
		{[]string{"inventory", "show"}, inventoryShowCmd},
		{[]string{"inventory", "edit"}, inventoryEditCmd},
		{[]string{"inv", "e"}, inventoryEditCmd},
		{[]string{"inventory", "validate"}, inventoryValidateCmd},
		{[]string{"inv", "v"}, inventoryValidateCmd},
		{[]string{"inventory", "cmdb"}, inventoryCMDBCmd},
		{[]string{"inv", "cmdb", "init"}, inventoryCMDBInitCmd},
		{[]string{"inv", "cmdb", "load"}, inventoryCMDBLoadCmd},
		{[]string{"inv", "cmdb", "dump"}, inventoryCMDBDumpCmd},
		{[]string{"inv", "cmdb", "enable"}, inventoryCMDBEnableCmd},
		{[]string{"inv", "cmdb", "disable"}, inventoryCMDBDisableCmd},
		{[]string{"i"}, installCmd},
	} {
		found, _, err := rootCmd.Find(test.args)
		if err != nil || found != test.want {
			t.Fatalf("Find(%v) = %v, %v; want %v", test.args, found, err, test.want)
		}
	}
	if inventoryCmd.GroupID != "pigsty" {
		t.Fatalf("inventory GroupID = %q, want pigsty", inventoryCmd.GroupID)
	}
	if inventoryCmd.Run != nil || inventoryCmd.RunE != nil {
		t.Fatal("bare inventory must only print help")
	}
	for _, flag := range []string{"strict", "ansible"} {
		if inventoryValidateCmd.Flags().Lookup(flag) == nil {
			t.Fatalf("inventory validate missing --%s", flag)
		}
	}
}

func TestInventoryCMDBCommandOptions(t *testing.T) {
	for _, command := range []*cobra.Command{inventoryCMDBInitCmd, inventoryCMDBLoadCmd, inventoryCMDBDumpCmd, inventoryCMDBEnableCmd} {
		for _, flag := range []string{"database", "data"} {
			if command.Flags().Lookup(flag) == nil {
				t.Fatalf("%s missing --%s", command.CommandPath(), flag)
			}
		}
	}
	if inventoryCMDBInitCmd.Flags().Lookup("database").Shorthand != "d" {
		t.Fatal("inventory cmdb --database must use -d")
	}
	if inventoryCMDBDisableCmd.Flags().Lookup("database") != nil {
		t.Fatal("inventory cmdb disable must not expose an unused database flag")
	}
	if inventoryCMDBInitCmd.Flags().Lookup("yes") == nil {
		t.Fatal("inventory cmdb init missing --yes")
	}
	if inventoryCMDBCmd.Commands()[0].Parent() != inventoryCMDBCmd {
		t.Fatal("CMDB subcommands are not attached")
	}
}

func TestInventoryCMDBCommandsDelegate(t *testing.T) {
	originalInit, originalLoad := inventoryCMDBInitExec, inventoryCMDBLoadExec
	originalDump := inventoryCMDBDumpExec
	originalEnable, originalDisable := inventoryCMDBEnableExec, inventoryCMDBDisableExec
	originalHome, originalConfig := config.PigstyHome, config.PigstyConfig
	originalOSCode, originalOSArch := config.OSCode, config.OSArch
	originalDatabase, originalYes := inventoryCMDBDatabase, inventoryCMDBInitYes
	defer func() {
		inventoryCMDBInitExec, inventoryCMDBLoadExec = originalInit, originalLoad
		inventoryCMDBDumpExec = originalDump
		inventoryCMDBEnableExec, inventoryCMDBDisableExec = originalEnable, originalDisable
		config.PigstyHome, config.PigstyConfig = originalHome, originalConfig
		config.OSCode, config.OSArch = originalOSCode, originalOSArch
		inventoryCMDBDatabase, inventoryCMDBInitYes = originalDatabase, originalYes
		inventoryCMDBDumpCmd.SetOut(nil)
	}()
	config.PigstyHome, config.PigstyConfig = "/tmp/pigsty", "/tmp/pigsty/pigsty.yml"
	config.OSCode, config.OSArch = "el9", "amd64"
	inventoryCMDBDatabase, inventoryCMDBInitYes = "meta-test", true

	var initialized stycli.InventoryCMDBOptions
	inventoryCMDBInitExec = func(_ context.Context, options stycli.InventoryCMDBOptions) *output.Result {
		initialized = options
		return output.OK("initialized", nil)
	}
	_ = captureStdout(t, func() {
		if err := inventoryCMDBInitCmd.RunE(inventoryCMDBInitCmd, nil); err != nil {
			t.Fatal(err)
		}
	})
	if initialized.Home != config.PigstyHome || initialized.Database != "meta-test" || !initialized.Yes {
		t.Fatalf("init options = %+v", initialized)
	}

	var loaded stycli.InventoryCMDBOptions
	inventoryCMDBLoadExec = func(_ context.Context, options stycli.InventoryCMDBOptions) *output.Result {
		loaded = options
		return output.OK("loaded", nil)
	}
	_ = captureStdout(t, func() {
		if err := inventoryCMDBLoadCmd.RunE(inventoryCMDBLoadCmd, []string{"/tmp/explicit.yml"}); err != nil {
			t.Fatal(err)
		}
	})
	if loaded.Path != "/tmp/explicit.yml" || loaded.OSCode != "el9" || loaded.OSArch != "amd64" {
		t.Fatalf("load options = %+v", loaded)
	}

	inventoryCMDBDumpExec = func(_ context.Context, options stycli.InventoryCMDBOptions) ([]byte, *output.Result) {
		if options.Path != "" || options.Database != "meta-test" {
			t.Fatalf("dump options = %+v", options)
		}
		return []byte("all: {}\n"), output.OK("dumped", nil)
	}
	var dump bytes.Buffer
	inventoryCMDBDumpCmd.SetOut(&dump)
	if err := inventoryCMDBDumpCmd.RunE(inventoryCMDBDumpCmd, nil); err != nil {
		t.Fatal(err)
	}
	if dump.String() != "all: {}\n" {
		t.Fatalf("dump stdout = %q", dump.String())
	}

	var enabled, disabled stycli.InventoryCMDBOptions
	inventoryCMDBEnableExec = func(_ context.Context, options stycli.InventoryCMDBOptions) *output.Result {
		enabled = options
		return output.OK("enabled", nil)
	}
	inventoryCMDBDisableExec = func(options stycli.InventoryCMDBOptions) *output.Result {
		disabled = options
		return output.OK("disabled", nil)
	}
	_ = captureStdout(t, func() {
		if err := inventoryCMDBEnableCmd.RunE(inventoryCMDBEnableCmd, nil); err != nil {
			t.Fatal(err)
		}
		if err := inventoryCMDBDisableCmd.RunE(inventoryCMDBDisableCmd, nil); err != nil {
			t.Fatal(err)
		}
	})
	if enabled.Home != config.PigstyHome || disabled.Home != config.PigstyHome {
		t.Fatalf("source options: enabled=%+v disabled=%+v", enabled, disabled)
	}
}

func TestInventoryRootDelegatesToExistingUseCases(t *testing.T) {
	originalList, originalShow := pigstyInventoryListExec, pigstyInventoryShowExec
	originalEdit, originalValidate := pigstyEditExec, pigstyValidateExec
	originalPath, originalFormat := config.PigstyConfig, config.OutputFormat
	originalStrict, originalAnsible := inventoryValidateStrict, inventoryValidateAnsible
	defer func() {
		pigstyInventoryListExec, pigstyInventoryShowExec = originalList, originalShow
		pigstyEditExec, pigstyValidateExec = originalEdit, originalValidate
		config.PigstyConfig, config.OutputFormat = originalPath, originalFormat
		inventoryValidateStrict, inventoryValidateAnsible = originalStrict, originalAnsible
		inventoryShowCmd.SetOut(nil)
	}()

	config.PigstyConfig, config.OutputFormat = "/tmp/active.yml", config.OUTPUT_TEXT

	var listed stycli.InventoryOptions
	pigstyInventoryListExec = func(options stycli.InventoryOptions) *output.Result {
		listed = options
		return output.OK("listed", nil)
	}
	_ = captureStdout(t, func() {
		if err := inventoryListCmd.RunE(inventoryListCmd, []string{"pg-meta"}); err != nil {
			t.Fatal(err)
		}
	})
	if listed.Path != config.PigstyConfig || listed.Selector != "pg-meta" {
		t.Fatalf("list options = %+v", listed)
	}

	pigstyInventoryShowExec = func(options stycli.InventoryOptions) ([]byte, *output.Result) {
		if options.Path != config.PigstyConfig || options.Selector != "vars" {
			t.Fatalf("show options = %+v", options)
		}
		return []byte("secret: exact\n"), nil
	}
	var shown bytes.Buffer
	inventoryShowCmd.SetOut(&shown)
	if err := inventoryShowCmd.RunE(inventoryShowCmd, []string{"vars"}); err != nil {
		t.Fatal(err)
	}
	if shown.String() != "secret: exact\n" {
		t.Fatalf("show output = %q", shown.String())
	}

	var edited stycli.EditOptions
	pigstyEditExec = func(options stycli.EditOptions) *output.Result {
		edited = options
		return output.OK("edited", nil)
	}
	_ = captureStdout(t, func() {
		if err := inventoryEditCmd.RunE(inventoryEditCmd, []string{"vars"}); err != nil {
			t.Fatal(err)
		}
	})
	if edited.Path != config.PigstyConfig || edited.Selector != "vars" {
		t.Fatalf("edit options = %+v", edited)
	}

	inventoryValidateStrict, inventoryValidateAnsible = true, true
	var validated stycli.ValidateOptions
	pigstyValidateExec = func(options stycli.ValidateOptions) *output.Result {
		validated = options
		return output.OK("validated", nil)
	}
	_ = captureStdout(t, func() {
		if err := inventoryValidateCmd.RunE(inventoryValidateCmd, []string{"/tmp/explicit.yml"}); err != nil {
			t.Fatal(err)
		}
	})
	if validated.Path != "/tmp/explicit.yml" || !validated.Strict || !validated.Ansible {
		t.Fatalf("validate options = %+v", validated)
	}
}
