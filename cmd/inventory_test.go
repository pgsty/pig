package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	inventorycli "pig/cli/inventory"
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
		{[]string{"inventory", "status"}, inventoryStatusCmd},
		{[]string{"inventory", "list"}, inventoryListCmd},
		{[]string{"inv", "ls"}, inventoryListCmd},
		{[]string{"inventory", "show"}, inventoryShowCmd},
		{[]string{"inventory", "edit"}, inventoryEditCmd},
		{[]string{"inv", "e"}, inventoryEditCmd},
		{[]string{"inventory", "validate"}, inventoryValidateCmd},
		{[]string{"inv", "v"}, inventoryValidateCmd},
		{[]string{"inventory", "check"}, inventoryCheckCmd},
		{[]string{"inv", "ck"}, inventoryCheckCmd},
		{[]string{"inventory", "diff"}, inventoryDiffCmd},
		{[]string{"inventory", "cmdb"}, inventoryCMDBCmd},
		{[]string{"inv", "cmdb", "check"}, inventoryCMDBCheckCmd},
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
	if inventoryCmd.Run != nil || inventoryCmd.RunE == nil {
		t.Fatal("bare inventory must use the help-only handler")
	}
	if err := inventoryCmd.ValidateArgs([]string{"unknown"}); err == nil {
		t.Fatal("inventory accepted an unknown positional command")
	}
	for _, flag := range []string{"strict", "ansible", "timeout"} {
		if inventoryValidateCmd.Flags().Lookup(flag) == nil {
			t.Fatalf("inventory validate missing --%s", flag)
		}
	}
	if inventoryListCmd.Flags().Lookup("depth") == nil || inventoryEditCmd.Flags().Lookup("from") == nil {
		t.Fatal("inventory list/edit usability flags are missing")
	}
}

func TestInventoryCheckFlagsAndOptionMapping(t *testing.T) {
	for _, flag := range []string{"profile", "sudo", "port", "user"} {
		if inventoryCheckCmd.Flags().Lookup(flag) == nil {
			t.Fatalf("pig inventory check is missing --%s", flag)
		}
	}
	if shorthand := inventoryCheckCmd.Flags().Lookup("profile").Shorthand; shorthand != "p" {
		t.Fatalf("--profile shorthand = %q, want p", shorthand)
	}

	originalExec := inventoryCheckExec
	originalPath, originalHome := config.PigstyConfig, config.PigstyHome
	originalFormat := config.OutputFormat
	originalProfiles := append([]string(nil), inventoryCheckProfiles...)
	originalSudo, originalPort, originalUser := inventoryCheckSudo, inventoryCheckPort, inventoryCheckUser
	defer func() {
		inventoryCheckExec = originalExec
		config.PigstyConfig, config.PigstyHome = originalPath, originalHome
		config.OutputFormat = originalFormat
		inventoryCheckProfiles = originalProfiles
		inventoryCheckSudo, inventoryCheckPort, inventoryCheckUser = originalSudo, originalPort, originalUser
	}()

	config.PigstyConfig, config.PigstyHome = "/tmp/active.yml", "/tmp/pigsty"
	config.OutputFormat = config.OUTPUT_TEXT
	inventoryCheckProfiles = []string{"ansible", "ssh"}
	inventoryCheckSudo, inventoryCheckPort, inventoryCheckUser = true, 2222, "admin"
	var got inventorycli.CheckOptions
	inventoryCheckExec = func(opts inventorycli.CheckOptions) *output.Result {
		got = opts
		return output.OK("checked", &inventorycli.CheckData{Kind: inventorycli.CheckKind, Ready: true})
	}
	_ = captureStdout(t, func() {
		if err := inventoryCheckCmd.RunE(inventoryCheckCmd, nil); err != nil {
			t.Fatal(err)
		}
	})
	if got.Path != config.PigstyConfig || got.Home != config.PigstyHome || strings.Join(got.Profiles, ",") != "ansible,ssh" || !got.Sudo || got.Port != 2222 || got.User != "admin" {
		t.Fatalf("Check options = %+v", got)
	}
}

func TestV160InventoryCompatibilityFixture(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("testdata", "v1.6.0-inventory-surface.txt"))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		state, command, ok := strings.Cut(line, " ")
		if !ok || state != "present" && state != "absent" {
			t.Fatalf("invalid compatibility fixture line %q", line)
		}
		want[command] = state == "present"
	}
	for command, present := range want {
		if command == "pig sty edit" || command == "pig sty validate" {
			present = false
		}
		args := strings.Fields(strings.TrimPrefix(command, "pig "))
		found, remaining, findErr := rootCmd.Find(args)
		registered := findErr == nil && found != nil && len(remaining) == 0 && found.CommandPath() == command
		if !present && command == "pig inventory" {
			if !registered {
				t.Fatalf("canonical root Inventory introduced after v1.6.0 is not registered: found=%v remaining=%v err=%v", found, remaining, findErr)
			}
			continue
		}
		if registered != present {
			t.Fatalf("v1.6.0 compatibility %q registered=%v, want %v (found=%v remaining=%v err=%v)", command, registered, present, found, remaining, findErr)
		}
	}
}

func TestInventoryCMDBCommandOptions(t *testing.T) {
	for _, command := range []*cobra.Command{inventoryCMDBCheckCmd, inventoryCMDBInitCmd, inventoryCMDBLoadCmd, inventoryCMDBDumpCmd, inventoryCMDBEnableCmd} {
		if command.Flags().Lookup("database") == nil {
			t.Fatalf("%s missing --database", command.CommandPath())
		}
		if command.Flags().Lookup("data") != nil {
			t.Fatalf("%s retains unreleased --data", command.CommandPath())
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
	for _, item := range []struct {
		command *cobra.Command
		flags   []string
	}{
		{inventoryCMDBInitCmd, []string{"plan", "yes"}},
		{inventoryCMDBLoadCmd, []string{"plan", "yes", "strict"}},
		{inventoryCMDBDumpCmd, []string{"force"}},
		{inventoryCMDBEnableCmd, []string{"plan"}},
		{inventoryCMDBDisableCmd, []string{"plan"}},
	} {
		for _, name := range item.flags {
			if item.command.Flags().Lookup(name) == nil {
				t.Fatalf("%s missing --%s", item.command.CommandPath(), name)
			}
		}
	}
	if inventoryCMDBInitCmd.Flags().Lookup("yes").Shorthand != "y" || inventoryCMDBLoadCmd.Flags().Lookup("yes").Shorthand != "y" ||
		inventoryCMDBDumpCmd.Flags().Lookup("force").Shorthand != "f" {
		t.Fatal("CMDB confirmation/overwrite shorthands differ from the contract")
	}
	if inventoryCMDBCmd.Commands()[0].Parent() != inventoryCMDBCmd {
		t.Fatal("CMDB subcommands are not attached")
	}
}

func TestInventoryCMDBCommandsDelegate(t *testing.T) {
	originalCheck := inventoryCMDBCheckExec
	originalInit, originalLoad := inventoryCMDBInitExec, inventoryCMDBLoadExec
	originalDump := inventoryCMDBDumpExec
	originalEnable, originalDisable := inventoryCMDBEnableExec, inventoryCMDBDisableExec
	originalHome, originalConfig, originalSource := config.PigstyHome, config.PigstyConfig, config.PigstyInventorySource
	originalOSCode, originalOSArch := config.OSCode, config.OSArch
	originalDatabase, originalYes := inventoryCMDBDatabase, inventoryCMDBInitYes
	defer func() {
		inventoryCMDBCheckExec = originalCheck
		inventoryCMDBInitExec, inventoryCMDBLoadExec = originalInit, originalLoad
		inventoryCMDBDumpExec = originalDump
		inventoryCMDBEnableExec, inventoryCMDBDisableExec = originalEnable, originalDisable
		config.PigstyHome, config.PigstyConfig, config.PigstyInventorySource = originalHome, originalConfig, originalSource
		config.OSCode, config.OSArch = originalOSCode, originalOSArch
		inventoryCMDBDatabase, inventoryCMDBInitYes = originalDatabase, originalYes
		inventoryCMDBDumpCmd.SetOut(nil)
	}()
	config.PigstyHome, config.PigstyConfig = "/tmp/pigsty", "/tmp/pigsty/pigsty.yml"
	config.PigstyInventorySource = config.InventorySourceDescriptor{Type: config.InventorySourceStaticYAML, Path: config.PigstyConfig}
	config.OSCode, config.OSArch = "el9", "amd64"
	inventoryCMDBDatabase, inventoryCMDBInitYes = "meta-test", true

	var checked inventorycli.InventoryCMDBOptions
	inventoryCMDBCheckExec = func(_ context.Context, options inventorycli.InventoryCMDBOptions) *output.Result {
		checked = options
		return output.OK("checked", nil)
	}
	_ = captureStdout(t, func() {
		if err := inventoryCMDBCheckCmd.RunE(inventoryCMDBCheckCmd, []string{"/tmp/check.yml"}); err != nil {
			t.Fatal(err)
		}
	})
	if !checked.ExplicitPath || checked.Path != "/tmp/check.yml" || checked.Database != "meta-test" {
		t.Fatalf("check options = %+v", checked)
	}

	var initialized inventorycli.InventoryCMDBOptions
	inventoryCMDBInitExec = func(_ context.Context, options inventorycli.InventoryCMDBOptions) *output.Result {
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

	var loaded inventorycli.InventoryCMDBOptions
	inventoryCMDBLoadExec = func(_ context.Context, options inventorycli.InventoryCMDBOptions) *output.Result {
		loaded = options
		return output.OK("loaded", nil)
	}
	_ = captureStdout(t, func() {
		if err := inventoryCMDBLoadCmd.RunE(inventoryCMDBLoadCmd, []string{"/tmp/explicit.yml"}); err != nil {
			t.Fatal(err)
		}
	})
	if loaded.Path != "/tmp/explicit.yml" || !loaded.ExplicitPath || loaded.OSCode != "el9" || loaded.OSArch != "amd64" ||
		loaded.Source.Type != config.InventorySourceStaticYAML {
		t.Fatalf("load options = %+v", loaded)
	}

	inventoryCMDBDumpExec = func(_ context.Context, options inventorycli.InventoryCMDBOptions) ([]byte, *output.Result) {
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

	var enabled, disabled inventorycli.InventoryCMDBOptions
	inventoryCMDBEnableExec = func(_ context.Context, options inventorycli.InventoryCMDBOptions) *output.Result {
		enabled = options
		return output.OK("enabled", nil)
	}
	inventoryCMDBDisableExec = func(options inventorycli.InventoryCMDBOptions) *output.Result {
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

func TestInventoryCMDBLoadConfirmationIsTTYOnlyAndRechecksPlan(t *testing.T) {
	originalExec := inventoryCMDBLoadExec
	originalTerminal := inventoryCMDBTextInputIsTerminal
	originalConfirm := highRiskTextConfirm
	originalFormat := config.OutputFormat
	defer func() {
		inventoryCMDBLoadExec = originalExec
		inventoryCMDBTextInputIsTerminal = originalTerminal
		highRiskTextConfirm = originalConfirm
		config.OutputFormat = originalFormat
	}()
	config.OutputFormat = config.OUTPUT_TEXT
	inventoryCMDBTextInputIsTerminal = func() bool { return true }
	confirmed := 0
	highRiskTextConfirm = func(warning, action string) error {
		confirmed++
		if !strings.Contains(warning, "five") || action != "pig inventory cmdb load --yes" {
			t.Fatalf("confirmation = %q / %q", warning, action)
		}
		return nil
	}
	calls := 0
	plan := &inventorycli.InventoryCMDBData{
		Operation: "load", SourceRevision: "sha256:source", TargetDigest: "sha256:target", DatabaseTargetDigest: "sha256:database",
	}
	expectedDigest := plan.ConfirmationDigest()
	inventoryCMDBLoadExec = func(_ context.Context, options inventorycli.InventoryCMDBOptions) *output.Result {
		calls++
		if !options.Yes {
			return output.Fail(output.CodeInventoryCMDBConfirmationRequired, "confirmation required").WithData(plan)
		}
		if options.ExpectedPlanDigest != expectedDigest {
			t.Fatalf("confirmation did not pin the displayed load plan: %+v", options)
		}
		return output.OK("loaded", nil)
	}
	_ = captureStdout(t, func() {
		if err := runInventoryCMDBConfirmed(inventoryCMDBLoadCmd, inventorycli.InventoryCMDBOptions{}, inventoryCMDBLoadExec,
			"This replaces all five Pigsty CMDB declaration tables.", "pig inventory cmdb load --yes"); err != nil {
			t.Fatal(err)
		}
	})
	if calls != 2 || confirmed != 1 {
		t.Fatalf("interactive calls=%d confirmations=%d", calls, confirmed)
	}

	config.OutputFormat = config.OUTPUT_JSON
	calls, confirmed = 0, 0
	_ = captureStdout(t, func() {
		if err := runInventoryCMDBConfirmed(inventoryCMDBLoadCmd, inventorycli.InventoryCMDBOptions{}, inventoryCMDBLoadExec,
			"This replaces all five Pigsty CMDB declaration tables.", "pig inventory cmdb load --yes"); err == nil {
			t.Fatal("structured changed load executed without --yes")
		}
	})
	if calls != 1 || confirmed != 0 {
		t.Fatalf("structured calls=%d confirmations=%d", calls, confirmed)
	}

	config.OutputFormat = config.OUTPUT_TEXT
	calls, confirmed = 0, 0
	_ = captureStdout(t, func() {
		if err := runInventoryCMDBConfirmed(inventoryCMDBLoadCmd, inventorycli.InventoryCMDBOptions{Plan: true}, inventoryCMDBLoadExec,
			"This replaces all five Pigsty CMDB declaration tables.", "pig inventory cmdb load --yes"); err == nil {
			t.Fatal("synthetic confirmation failure in plan mode should remain a failure")
		}
	})
	if calls != 1 || confirmed != 0 {
		t.Fatalf("plan calls=%d confirmations=%d", calls, confirmed)
	}
}

func TestPinInventoryCMDBInitConfirmation(t *testing.T) {
	options := inventorycli.InventoryCMDBOptions{}
	result := output.Fail(output.CodeInventoryCMDBConfirmationRequired, "confirmation required").WithData(&inventorycli.InventoryCMDBData{
		Operation: "init", SourceRevision: "sha256:baseline", DatabaseTargetDigest: "sha256:database", ExistingSchemas: 2,
	})
	if !pinInventoryCMDBConfirmation(result, &options) {
		t.Fatal("init confirmation plan was not pinnable")
	}
	data := result.Data.(*inventorycli.InventoryCMDBData)
	if options.ExpectedPlanDigest == "" || options.ExpectedPlanDigest != data.ConfirmationDigest() {
		t.Fatalf("pinned init options = %+v", options)
	}
}

func TestInventoryRootDelegatesToExistingUseCases(t *testing.T) {
	originalStatus := inventoryStatusExec
	originalDiff := inventoryDiffExec
	originalList, originalShow := inventoryListExec, inventoryShowExec
	originalEdit, originalValidate := inventoryEditExec, inventoryValidateExec
	originalPath, originalFormat := config.PigstyConfig, config.OutputFormat
	originalSource := config.PigstyInventorySource
	originalStrict, originalAnsible := inventoryValidateStrict, inventoryValidateAnsible
	defer func() {
		inventoryStatusExec = originalStatus
		inventoryDiffExec = originalDiff
		inventoryListExec, inventoryShowExec = originalList, originalShow
		inventoryEditExec, inventoryValidateExec = originalEdit, originalValidate
		config.PigstyConfig, config.OutputFormat = originalPath, originalFormat
		config.PigstyInventorySource = originalSource
		inventoryValidateStrict, inventoryValidateAnsible = originalStrict, originalAnsible
		inventoryShowCmd.SetOut(nil)
	}()

	config.PigstyConfig, config.OutputFormat = "/tmp/active.yml", config.OUTPUT_TEXT
	config.PigstyInventorySource = config.InventorySourceDescriptor{
		Tier: config.InventorySourceFlag, Type: config.InventorySourceStaticYAML,
		Path: config.PigstyConfig, Exists: true, Readable: true,
	}

	var inspected config.InventorySourceDescriptor
	inventoryStatusExec = func(source config.InventorySourceDescriptor) *output.Result {
		inspected = source
		return output.OK("inspected", nil)
	}
	_ = captureStdout(t, func() {
		if err := inventoryStatusCmd.RunE(inventoryStatusCmd, nil); err != nil {
			t.Fatal(err)
		}
	})
	if inspected.Path != config.PigstyConfig || inspected.Type != config.InventorySourceStaticYAML {
		t.Fatalf("status source = %+v", inspected)
	}

	var listed inventorycli.InventoryOptions
	inventoryListExec = func(options inventorycli.InventoryOptions) *output.Result {
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

	var compared inventorycli.DiffOptions
	inventoryDiffExec = func(options inventorycli.DiffOptions) *output.Result {
		compared = options
		return output.OK("compared", nil)
	}
	_ = captureStdout(t, func() {
		if err := inventoryDiffCmd.RunE(inventoryDiffCmd, []string{"/tmp/other.yml"}); err != nil {
			t.Fatal(err)
		}
	})
	if compared.Path != config.PigstyConfig || compared.OtherPath != "/tmp/other.yml" {
		t.Fatalf("diff options = %+v", compared)
	}

	inventoryShowExec = func(options inventorycli.InventoryOptions) ([]byte, *output.Result) {
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

	var edited inventorycli.EditOptions
	inventoryEditExec = func(options inventorycli.EditOptions) *output.Result {
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
	var validated inventorycli.ValidateOptions
	inventoryValidateExec = func(options inventorycli.ValidateOptions) *output.Result {
		validated = options
		return output.OK("validated", nil)
	}
	_ = captureStdout(t, func() {
		if err := inventoryValidateCmd.RunE(inventoryValidateCmd, []string{"/tmp/explicit.yml"}); err != nil {
			t.Fatal(err)
		}
	})
	if validated.Path != "/tmp/explicit.yml" || !validated.Strict || !validated.Ansible || validated.Timeout <= 0 {
		t.Fatalf("validate options = %+v", validated)
	}
}
