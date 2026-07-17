package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	stycli "pig/cli/sty"
	"pig/internal/cmdb"
	"pig/internal/config"
	"pig/internal/output"

	"github.com/spf13/cobra"
)

func TestStyCMDBCommandTreeFlagsAndExperimentalAnnotations(t *testing.T) {
	for _, test := range []struct {
		args []string
		want *cobra.Command
	}{
		{args: []string{"sty", "cmdb"}, want: pigstyCMDBCmd},
		{args: []string{"sty", "cmdb", "init"}, want: pigstyCMDBInitCmd},
		{args: []string{"sty", "cmdb", "load"}, want: pigstyCMDBLoadCmd},
		{args: []string{"sty", "cmdb", "dump"}, want: pigstyCMDBDumpCmd},
		{args: []string{"sty", "cmdb", "enable"}, want: pigstyCMDBEnableCmd},
		{args: []string{"sty", "cmdb", "disable"}, want: pigstyCMDBDisableCmd},
		{args: []string{"sty", "cmdb", "check"}, want: pigstyCMDBCheckCmd},
		{args: []string{"sty", "cmdb", "_inventory"}, want: pigstyCMDBInventoryAdapterCmd},
	} {
		found, _, err := rootCmd.Find(test.args)
		if err != nil || found != test.want {
			t.Fatalf("Find(%v) = %v, %v; want %v", test.args, found, err, test.want)
		}
	}
	for _, command := range []*cobra.Command{
		pigstyCMDBCmd, pigstyCMDBInitCmd, pigstyCMDBLoadCmd, pigstyCMDBDumpCmd,
		pigstyCMDBEnableCmd, pigstyCMDBDisableCmd, pigstyCMDBCheckCmd,
	} {
		if command.Annotations["maturity"] != "experimental" {
			t.Fatalf("%s maturity = %q", command.CommandPath(), command.Annotations["maturity"])
		}
	}
	if !pigstyCMDBInventoryAdapterCmd.Hidden || !pigstyCMDBInventoryAdapterCmd.DisableFlagParsing {
		t.Fatalf("private adapter visibility/parsing = hidden:%v disabled:%v", pigstyCMDBInventoryAdapterCmd.Hidden, pigstyCMDBInventoryAdapterCmd.DisableFlagParsing)
	}

	assertFlags := func(command *cobra.Command, names ...string) {
		t.Helper()
		for _, name := range names {
			if command.Flags().Lookup(name) == nil {
				t.Fatalf("%s missing --%s", command.CommandPath(), name)
			}
		}
	}
	if pigstyCMDBCmd.PersistentFlags().Lookup("connection-file") == nil {
		t.Fatal("pig sty cmdb is missing --connection-file")
	}
	assertFlags(pigstyCMDBInitCmd, "reset", "restore", "yes")
	assertFlags(pigstyCMDBLoadCmd, "revision", "prune", "yes")
	assertFlags(pigstyCMDBDumpCmd, "output", "format", "revision", "merge", "expect-base")
	assertFlags(pigstyCMDBDisableCmd, "static")
	if flag := pigstyCMDBDumpCmd.Flags().Lookup("output"); flag.Shorthand != "" {
		t.Fatalf("command-local dump --output unexpectedly owns global -o: %+v", flag)
	}
	if flag := pigstyCMDBDumpCmd.Flags().Lookup("format"); flag.Shorthand != "o" {
		t.Fatalf("dump structured format flag = %+v", flag)
	}

	originalOut := pigstyCMDBCmd.OutOrStdout()
	defer pigstyCMDBCmd.SetOut(originalOut)
	var help bytes.Buffer
	pigstyCMDBCmd.SetOut(&help)
	if err := pigstyCMDBCmd.Help(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(help.String(), "_inventory") {
		t.Fatalf("private adapter appeared in public help: %s", help.String())
	}
}

func TestStyCMDBDumpParsesFormatAndDestinationTogether(t *testing.T) {
	restore := saveStyCMDBState()
	defer restore()
	changed := make(map[string]bool)
	for _, name := range []string{"output", "format", "revision", "merge", "expect-base"} {
		changed[name] = pigstyCMDBDumpCmd.Flags().Lookup(name).Changed
	}
	defer func() {
		for name, value := range changed {
			pigstyCMDBDumpCmd.Flags().Lookup(name).Changed = value
		}
	}()
	pigstyCMDBDumpOutput = ""
	outputFormat = config.OUTPUT_TEXT
	if err := pigstyCMDBDumpCmd.ParseFlags([]string{"-o", "json", "--output", "/private/generated.yml"}); err != nil {
		t.Fatal(err)
	}
	if outputFormat != config.OUTPUT_JSON || pigstyCMDBDumpOutput != "/private/generated.yml" {
		t.Fatalf("format=%q destination=%q", outputFormat, pigstyCMDBDumpOutput)
	}
	if got := outputFormatFromArgs([]string{"sty", "cmdb", "dump", "-o", "json", "--output", "/private/generated.yml"}); got != config.OUTPUT_JSON {
		t.Fatalf("early dump output format = %q", got)
	}
	if got := outputFormatFromArgs([]string{"sty", "cmdb", "dump", "--format=json", "--output=/private/generated.yml"}); got != config.OUTPUT_JSON {
		t.Fatalf("early long dump output format = %q", got)
	}
}

func TestStyCMDBParameterAmbiguityFailsBeforeConnection(t *testing.T) {
	restore := saveStyCMDBState()
	defer restore()
	config.OutputFormat = config.OUTPUT_TEXT
	pigstyCMDBExecutable = func() (string, error) { return "/bin/pig", nil }
	opens := 0
	pigstyCMDBOpenStore = func(context.Context, stycli.CMDBConnectionOptions) (*cmdb.Store, *output.Result) {
		opens++
		return nil, nil
	}

	pigstyCMDBLoadRevision = "not-a-token"
	_ = captureStdout(t, func() {
		if err := pigstyCMDBLoadCmd.RunE(pigstyCMDBLoadCmd, []string{"static.yml"}); err == nil {
			t.Fatal("ambiguous load succeeded")
		}
	})
	rootCmd.PersistentFlags().Lookup("inventory").Changed = true
	inventory = "/explicit/inventory.yml"
	_ = captureStdout(t, func() {
		if err := pigstyCMDBLoadCmd.RunE(pigstyCMDBLoadCmd, nil); err == nil {
			t.Fatal("revision with invocation Inventory succeeded")
		}
	})
	rootCmd.PersistentFlags().Lookup("inventory").Changed = false
	inventory = ""
	pigstyCMDBLoadRevision = ""
	pigstyCMDBLoadRevision = "not-a-token"
	_ = captureStdout(t, func() {
		if err := pigstyCMDBLoadCmd.RunE(pigstyCMDBLoadCmd, nil); err == nil {
			t.Fatal("invalid revision token succeeded")
		}
	})
	pigstyCMDBLoadRevision = ""
	pigstyCMDBRestore = "/private/restore.ndjson"
	pigstyCMDBReset = false
	_ = captureStdout(t, func() {
		if err := pigstyCMDBInitCmd.RunE(pigstyCMDBInitCmd, nil); err == nil {
			t.Fatal("restore without reset succeeded")
		}
	})
	pigstyCMDBRestore = ""
	pigstyCMDBDumpOutput = "/private/out.yml"
	pigstyCMDBDumpMerge = true
	pigstyCMDBDumpRevision = "not-a-token"
	_ = captureStdout(t, func() {
		if err := pigstyCMDBDumpCmd.RunE(pigstyCMDBDumpCmd, nil); err == nil {
			t.Fatal("ambiguous merge dump succeeded")
		}
	})
	pigstyCMDBDumpMerge = false
	pigstyCMDBDumpRevision = "not-a-token"
	_ = captureStdout(t, func() {
		if err := pigstyCMDBDumpCmd.RunE(pigstyCMDBDumpCmd, nil); err == nil {
			t.Fatal("invalid dump revision token succeeded")
		}
	})
	if opens != 0 {
		t.Fatalf("parameter failures opened PostgreSQL %d times", opens)
	}
}

func TestStyCMDBStructuredSplitBrainNoticeStaysOffStdout(t *testing.T) {
	restore := saveStyCMDBState()
	defer restore()
	config.OutputFormat = config.OUTPUT_JSON
	pigstyCMDBResolveLoadSourceExec = func(stycli.CMDBSourceResolveOptions) (stycli.CMDBSourceResolution, string, *output.Result) {
		return stycli.CMDBSourceResolution{
			Effective: stycli.CMDBSourceDescriptor{Kind: stycli.CMDBSourceStatic, Origin: stycli.CMDBSourceEnvironment},
			Persistent: stycli.CMDBSourceDescriptor{
				Kind: stycli.CMDBSourceNative, Origin: stycli.CMDBSourceAnsibleConfig, Persistent: true,
			},
		}, "/absolute/static.yml", nil
	}
	pigstyCMDBOpenStore = func(context.Context, stycli.CMDBConnectionOptions) (*cmdb.Store, *output.Result) {
		return nil, nil
	}
	pigstyCMDBPlanLoadExec = func(context.Context, *cmdb.Store, stycli.CMDBLoadOptions) (*stycli.CMDBLoadOperation, *output.Result) {
		return new(stycli.CMDBLoadOperation), nil
	}
	pigstyCMDBApplyLoadExec = func(context.Context, *stycli.CMDBLoadOperation, stycli.CMDBLoadOptions) *output.Result {
		return output.OK("loaded", &stycli.CMDBLoadData{Kind: cmdb.LoadKind})
	}
	originalErr := pigstyCMDBLoadCmd.ErrOrStderr()
	defer pigstyCMDBLoadCmd.SetErr(originalErr)
	var stderr bytes.Buffer
	pigstyCMDBLoadCmd.SetErr(&stderr)
	raw := captureStdout(t, func() {
		if err := pigstyCMDBLoadCmd.RunE(pigstyCMDBLoadCmd, nil); err != nil {
			t.Fatal(err)
		}
	})
	if !json.Valid([]byte(raw)) || strings.Contains(raw, "NOTICE:") {
		t.Fatalf("structured stdout = %q", raw)
	}
	if got := stderr.String(); got != stycli.CMDBStaticShadowsPersistentNotice+"\n" {
		t.Fatalf("structured stderr = %q", got)
	}
}

func TestStyCMDBLoadTextConfirmationUsesOneSealedPlanAndOneNotice(t *testing.T) {
	restore := saveStyCMDBState()
	defer restore()
	config.OutputFormat = config.OUTPUT_TEXT
	styCMDBTextInputIsTerminal = func() bool { return true }
	pigstyCMDBLoadPrune = true
	pigstyCMDBConnectionFile = "/private/connection"
	pigstyCMDBResolveLoadSourceExec = func(stycli.CMDBSourceResolveOptions) (stycli.CMDBSourceResolution, string, *output.Result) {
		return stycli.CMDBSourceResolution{
			Effective: stycli.CMDBSourceDescriptor{Kind: stycli.CMDBSourceStatic, Origin: stycli.CMDBSourceCLI},
			Persistent: stycli.CMDBSourceDescriptor{
				Kind: stycli.CMDBSourceNative, Origin: stycli.CMDBSourceAnsibleConfig, Persistent: true,
			},
		}, "/absolute/static.yml", nil
	}
	var opened stycli.CMDBConnectionOptions
	pigstyCMDBOpenStore = func(_ context.Context, options stycli.CMDBConnectionOptions) (*cmdb.Store, *output.Result) {
		opened = options
		return nil, nil
	}
	operation := new(stycli.CMDBLoadOperation)
	plans := 0
	var planned stycli.CMDBLoadOptions
	pigstyCMDBPlanLoadExec = func(_ context.Context, _ *cmdb.Store, options stycli.CMDBLoadOptions) (*stycli.CMDBLoadOperation, *output.Result) {
		plans++
		planned = options
		return operation, nil
	}
	var appliedOperations []*stycli.CMDBLoadOperation
	var yesValues []bool
	pigstyCMDBApplyLoadExec = func(_ context.Context, got *stycli.CMDBLoadOperation, options stycli.CMDBLoadOptions) *output.Result {
		appliedOperations = append(appliedOperations, got)
		yesValues = append(yesValues, options.Yes)
		data := &stycli.CMDBLoadData{Kind: cmdb.LoadKind, RequiresConfirmation: true}
		if !options.Yes {
			return output.Fail(output.CodeStyCMDBConfirmationRequired, "confirmation required").WithData(data)
		}
		return output.OK("loaded", data)
	}
	prompts := 0
	highRiskTextConfirm = func(warning, action string) error {
		prompts++
		if !strings.Contains(warning, "declaration removal") || action != "pig sty cmdb load" {
			t.Fatalf("prompt warning=%q action=%q", warning, action)
		}
		return nil
	}
	originalErr := pigstyCMDBLoadCmd.ErrOrStderr()
	defer pigstyCMDBLoadCmd.SetErr(originalErr)
	var stderr bytes.Buffer
	pigstyCMDBLoadCmd.SetErr(&stderr)
	raw := captureStdout(t, func() {
		if err := pigstyCMDBLoadCmd.RunE(pigstyCMDBLoadCmd, []string{"relative.yml"}); err != nil {
			t.Fatal(err)
		}
	})
	if opened.ConnectionFile != "/private/connection" || plans != 1 || planned.StaticFile != "/absolute/static.yml" || !planned.Prune {
		t.Fatalf("open=%+v plans=%d planned=%+v", opened, plans, planned)
	}
	if len(appliedOperations) != 2 || appliedOperations[0] != operation || appliedOperations[1] != operation ||
		len(yesValues) != 2 || yesValues[0] || !yesValues[1] || prompts != 1 {
		t.Fatalf("operations=%v yes=%v prompts=%d", appliedOperations, yesValues, prompts)
	}
	if got := stderr.String(); got != stycli.CMDBStaticShadowsPersistentNotice+"\n" {
		t.Fatalf("split-brain stderr = %q", got)
	}
	if !strings.Contains(raw, "CMDB load plan") || !strings.Contains(raw, "loaded") {
		t.Fatalf("confirmation stdout = %q", raw)
	}
}

func TestStyCMDBResetTextConfirmationUsesOneSealedPlan(t *testing.T) {
	restore := saveStyCMDBState()
	defer restore()
	config.OutputFormat = config.OUTPUT_TEXT
	config.PigstyHome = "/srv/pigsty"
	styCMDBTextInputIsTerminal = func() bool { return true }
	pigstyCMDBExecutable = func() (string, error) { return "/opt/pig/bin/pig", nil }
	pigstyCMDBConnectionFile = "/private/connection"
	pigstyCMDBReset = true
	pigstyCMDBRestore = "/private/restore.ndjson"
	pigstyCMDBOpenStore = func(context.Context, stycli.CMDBConnectionOptions) (*cmdb.Store, *output.Result) {
		return nil, nil
	}
	operation := new(stycli.CMDBResetOperation)
	plans := 0
	var planned stycli.CMDBInitOptions
	pigstyCMDBPlanResetExec = func(_ context.Context, _ *cmdb.Store, options stycli.CMDBInitOptions) (*stycli.CMDBResetOperation, *output.Result) {
		plans++
		planned = options
		return operation, nil
	}
	var operations []*stycli.CMDBResetOperation
	var yesValues []bool
	pigstyCMDBApplyResetExec = func(_ context.Context, got *stycli.CMDBResetOperation, yes bool) *output.Result {
		operations = append(operations, got)
		yesValues = append(yesValues, yes)
		data := &stycli.CMDBInitData{Kind: "pig.sty.cmdb.init/v1", Reset: true, RequiresConfirm: true}
		if !yes {
			return output.Fail(output.CodeStyCMDBConfirmationRequired, "confirmation required").WithData(data)
		}
		return output.OK("reset", data)
	}
	prompts := 0
	highRiskTextConfirm = func(warning, action string) error {
		prompts++
		if !strings.Contains(warning, "pigsty_cmdb") || action != "pig sty cmdb init --reset" {
			t.Fatalf("prompt warning=%q action=%q", warning, action)
		}
		return nil
	}
	_ = captureStdout(t, func() {
		if err := pigstyCMDBInitCmd.RunE(pigstyCMDBInitCmd, nil); err != nil {
			t.Fatal(err)
		}
	})
	if plans != 1 || planned.PigstyHome != "/srv/pigsty" || planned.PigBinary != "/opt/pig/bin/pig" ||
		planned.ConnectionFile != "/private/connection" || !planned.Reset || planned.RestorePath != "/private/restore.ndjson" {
		t.Fatalf("plans=%d options=%+v", plans, planned)
	}
	if len(operations) != 2 || operations[0] != operation || operations[1] != operation ||
		len(yesValues) != 2 || yesValues[0] || !yesValues[1] || prompts != 1 {
		t.Fatalf("operations=%v yes=%v prompts=%d", operations, yesValues, prompts)
	}
}

func TestStyCMDBConfirmationFailsClosedForStructuredAndNonTTY(t *testing.T) {
	for _, test := range []struct {
		name       string
		format     string
		terminal   bool
		structured bool
	}{
		{name: "structured", format: config.OUTPUT_JSON, terminal: true, structured: true},
		{name: "non-tty", format: config.OUTPUT_TEXT, terminal: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			restore := saveStyCMDBState()
			defer restore()
			config.OutputFormat = test.format
			styCMDBTextInputIsTerminal = func() bool { return test.terminal }
			prompts := 0
			highRiskTextConfirm = func(string, string) error { prompts++; return nil }
			confirmedCalls := 0
			result := output.Fail(output.CodeStyCMDBConfirmationRequired, "confirmation required").
				WithData(&stycli.CMDBInitData{Kind: "pig.sty.cmdb.init/v1", Reset: true})
			raw := captureStdout(t, func() {
				err := runPigstyCMDBConfirmed(pigstyCMDBInitCmd, result, "plan", "warning", "action", func() *output.Result {
					confirmedCalls++
					return output.OK("unexpected", nil)
				})
				if err == nil {
					t.Fatal("unconfirmed operation succeeded")
				}
			})
			if prompts != 0 || confirmedCalls != 0 {
				t.Fatalf("prompts=%d confirmed=%d", prompts, confirmedCalls)
			}
			if test.structured && !strings.Contains(raw, `"code":`+strconv.Itoa(output.CodeStyCMDBConfirmationRequired)) {
				t.Fatalf("structured confirmation output = %q", raw)
			}
		})
	}
}

func TestStyCMDBHiddenAdapterKeepsRawStdoutAndSanitizedStderr(t *testing.T) {
	restore := saveStyCMDBState()
	defer restore()
	config.OutputFormat = config.OUTPUT_TEXT
	originalOut := pigstyCMDBInventoryAdapterCmd.OutOrStdout()
	originalErr := pigstyCMDBInventoryAdapterCmd.ErrOrStderr()
	originalSilenceUsage := pigstyCMDBInventoryAdapterCmd.SilenceUsage
	originalSilenceErrors := pigstyCMDBInventoryAdapterCmd.SilenceErrors
	defer func() {
		pigstyCMDBInventoryAdapterCmd.SetOut(originalOut)
		pigstyCMDBInventoryAdapterCmd.SetErr(originalErr)
		pigstyCMDBInventoryAdapterCmd.SilenceUsage = originalSilenceUsage
		pigstyCMDBInventoryAdapterCmd.SilenceErrors = originalSilenceErrors
	}()

	var stdout, stderr bytes.Buffer
	pigstyCMDBInventoryAdapterCmd.SetOut(&stdout)
	pigstyCMDBInventoryAdapterCmd.SetErr(&stderr)
	pigstyCMDBInventoryAdapterExec = func(_ context.Context, options stycli.CMDBInventoryAdapterOptions) ([]byte, *output.Result) {
		if len(options.Arguments) != 1 || options.Arguments[0] != "--list" {
			t.Fatalf("adapter arguments = %v", options.Arguments)
		}
		return []byte(`{"_meta":{"hostvars":{}}}`), nil
	}
	if err := pigstyCMDBInventoryAdapterCmd.RunE(pigstyCMDBInventoryAdapterCmd, []string{"--list"}); err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); got != `{"_meta":{"hostvars":{}}}` || stderr.Len() != 0 {
		t.Fatalf("adapter stdout=%q stderr=%q", got, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	secret := "inventory-private-value"
	pigstyCMDBInventoryAdapterExec = func(context.Context, stycli.CMDBInventoryAdapterOptions) ([]byte, *output.Result) {
		return []byte(secret), output.Fail(output.CodeStyCMDBConfigInvalid, "CMDB adapter request failed").
			WithData(&stycli.CMDBFailureData{Kind: stycli.CMDBFailureKind, CMDBCode: "CMDB_PARAMETER_INVALID"})
	}
	err := pigstyCMDBInventoryAdapterCmd.RunE(pigstyCMDBInventoryAdapterCmd, []string{"--host"})
	if err == nil || stdout.Len() != 0 || strings.Contains(stderr.String(), secret) || !strings.Contains(stderr.String(), "CMDB_PARAMETER_INVALID") {
		t.Fatalf("adapter failure err=%v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
	}
}

func saveStyCMDBState() func() {
	originalOpen := pigstyCMDBOpenStore
	originalInit := pigstyCMDBInitExec
	originalPlanReset := pigstyCMDBPlanResetExec
	originalApplyReset := pigstyCMDBApplyResetExec
	originalResolveLoad := pigstyCMDBResolveLoadSourceExec
	originalPlanLoad := pigstyCMDBPlanLoadExec
	originalApplyLoad := pigstyCMDBApplyLoadExec
	originalDump := pigstyCMDBDumpExec
	originalEnable := pigstyCMDBEnableExec
	originalDisable := pigstyCMDBDisableExec
	originalCheck := pigstyCMDBCheckExec
	originalAdapter := pigstyCMDBInventoryAdapterExec
	originalExecutable := pigstyCMDBExecutable
	originalTerminal := styCMDBTextInputIsTerminal
	originalConfirm := highRiskTextConfirm
	originalFormat := config.OutputFormat
	originalOutputFormat := outputFormat
	originalHome, originalConfig, originalUserHome := config.PigstyHome, config.ConfigFile, config.HomeDir
	originalConnection := pigstyCMDBConnectionFile
	originalReset, originalRestore, originalInitYes := pigstyCMDBReset, pigstyCMDBRestore, pigstyCMDBInitYes
	originalLoadRevision, originalLoadPrune, originalLoadYes := pigstyCMDBLoadRevision, pigstyCMDBLoadPrune, pigstyCMDBLoadYes
	originalDumpOutput, originalDumpRevision := pigstyCMDBDumpOutput, pigstyCMDBDumpRevision
	originalDumpMerge, originalDumpExpectBase := pigstyCMDBDumpMerge, pigstyCMDBDumpExpectBase
	originalDisableStatic := pigstyCMDBDisableStatic
	originalInventory := inventory
	inventoryFlag := rootCmd.PersistentFlags().Lookup("inventory")
	originalInventoryChanged := inventoryFlag != nil && inventoryFlag.Changed
	return func() {
		pigstyCMDBOpenStore = originalOpen
		pigstyCMDBInitExec = originalInit
		pigstyCMDBPlanResetExec = originalPlanReset
		pigstyCMDBApplyResetExec = originalApplyReset
		pigstyCMDBResolveLoadSourceExec = originalResolveLoad
		pigstyCMDBPlanLoadExec = originalPlanLoad
		pigstyCMDBApplyLoadExec = originalApplyLoad
		pigstyCMDBDumpExec = originalDump
		pigstyCMDBEnableExec = originalEnable
		pigstyCMDBDisableExec = originalDisable
		pigstyCMDBCheckExec = originalCheck
		pigstyCMDBInventoryAdapterExec = originalAdapter
		pigstyCMDBExecutable = originalExecutable
		styCMDBTextInputIsTerminal = originalTerminal
		highRiskTextConfirm = originalConfirm
		config.OutputFormat = originalFormat
		outputFormat = originalOutputFormat
		config.PigstyHome, config.ConfigFile, config.HomeDir = originalHome, originalConfig, originalUserHome
		pigstyCMDBConnectionFile = originalConnection
		pigstyCMDBReset, pigstyCMDBRestore, pigstyCMDBInitYes = originalReset, originalRestore, originalInitYes
		pigstyCMDBLoadRevision, pigstyCMDBLoadPrune, pigstyCMDBLoadYes = originalLoadRevision, originalLoadPrune, originalLoadYes
		pigstyCMDBDumpOutput, pigstyCMDBDumpRevision = originalDumpOutput, originalDumpRevision
		pigstyCMDBDumpMerge, pigstyCMDBDumpExpectBase = originalDumpMerge, originalDumpExpectBase
		pigstyCMDBDisableStatic = originalDisableStatic
		inventory = originalInventory
		if inventoryFlag != nil {
			inventoryFlag.Changed = originalInventoryChanged
		}
	}
}
