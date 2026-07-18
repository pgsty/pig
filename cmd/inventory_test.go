package cmd

import (
	"bytes"
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
