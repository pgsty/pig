package cmd

import (
	"strings"
	"testing"

	stycli "pig/cli/sty"
	"pig/internal/config"
	"pig/internal/output"

	"github.com/spf13/cobra"
)

func TestStyGrafanaCommandsAndFlags(t *testing.T) {
	for _, test := range []struct {
		args []string
		want *cobra.Command
	}{
		{args: []string{"sty", "grafana"}, want: pigstyGrafanaCmd},
		{args: []string{"sty", "gf"}, want: pigstyGrafanaCmd},
		{args: []string{"sty", "gf", "info"}, want: pigstyGrafanaInfoCmd},
		{args: []string{"sty", "grafana", "list"}, want: pigstyGrafanaListCmd},
		{args: []string{"sty", "gf", "list"}, want: pigstyGrafanaListCmd},
		{args: []string{"sty", "gf", "boot"}, want: pigstyGrafanaBootCmd},
		{args: []string{"sty", "grafana", "load"}, want: pigstyGrafanaLoadCmd},
		{args: []string{"sty", "grafana", "init"}, want: pigstyGrafanaInitCmd},
		{args: []string{"sty", "grafana", "dump"}, want: pigstyGrafanaDumpCmd},
		{args: []string{"sty", "gf", "dump"}, want: pigstyGrafanaDumpCmd},
		{args: []string{"sty", "grafana", "clean"}, want: pigstyGrafanaCleanCmd},
		{args: []string{"sty", "gf", "lang"}, want: pigstyGrafanaLangCmd},
		{args: []string{"sty", "gf", "style"}, want: pigstyGrafanaStyleCmd},
	} {
		found, _, err := rootCmd.Find(test.args)
		if err != nil {
			t.Fatalf("Find(%v): %v", test.args, err)
		}
		if found != test.want {
			t.Fatalf("Find(%v) = %v, want %v", test.args, found, test.want)
		}
	}
	if !pigstyGrafanaCmd.HasAlias("gf") {
		t.Fatal("pig sty grafana is missing alias gf")
	}
	for _, name := range []string{"endpoint", "username", "password", "password-file"} {
		if pigstyGrafanaCmd.PersistentFlags().Lookup(name) == nil {
			t.Fatalf("pig sty grafana is missing persistent --%s", name)
		}
	}
	if flag := pigstyGrafanaCmd.PersistentFlags().Lookup("password"); flag == nil || !strings.Contains(flag.Usage, "process arguments") {
		t.Fatalf("pig sty grafana --password warning = %+v", flag)
	}
	for _, name := range []string{"all", "yes", "dry-run"} {
		if pigstyGrafanaCleanCmd.Flags().Lookup(name) == nil {
			t.Fatalf("pig sty grafana clean is missing --%s", name)
		}
	}
	if flag := pigstyGrafanaDumpCmd.Flags().Lookup("dir"); flag == nil || flag.Shorthand != "d" {
		t.Fatalf("pig sty grafana dump --dir flag = %+v", flag)
	}
	if flag := pigstyGrafanaCleanCmd.Flags().Lookup("yes"); flag == nil || flag.Shorthand != "y" {
		t.Fatalf("pig sty grafana clean --yes flag = %+v", flag)
	}
	for _, command := range styCmd.Commands() {
		if command.Name() == "dashboard" || command.HasAlias("dashboard") || command.HasAlias("ds") {
			t.Fatalf("retired dashboard spelling still resolves to %s", command.CommandPath())
		}
	}
}

func TestStyGrafanaDumpDirectoryOptionMapping(t *testing.T) {
	restore := saveStyGrafanaState()
	defer restore()
	config.OutputFormat = config.OUTPUT_TEXT
	pigstyGrafanaDumpDir = "/tmp/grafana-snapshot"

	var got stycli.GrafanaOptions
	pigstyGrafanaDumpExec = func(options stycli.GrafanaOptions) *output.Result {
		got = options
		return output.OK("dumped", &stycli.GrafanaActionData{Kind: stycli.GrafanaDumpKind})
	}
	_ = captureStdout(t, func() {
		if err := pigstyGrafanaDumpCmd.RunE(pigstyGrafanaDumpCmd, nil); err != nil {
			t.Fatal(err)
		}
	})
	if got.DumpDir != pigstyGrafanaDumpDir || got.Path != "" {
		t.Fatalf("Grafana dump options = %+v", got)
	}
}

func TestStyGrafanaListUsesConnectionOptions(t *testing.T) {
	restore := saveStyGrafanaState()
	defer restore()
	config.OutputFormat = config.OUTPUT_TEXT
	config.PigstyHome = "/opt/pigsty"
	config.PigstyConfig = "/etc/pigsty.yml"
	pigstyGrafanaEndpoint = "https://grafana.example/ui"

	var got stycli.GrafanaOptions
	pigstyGrafanaListExec = func(options stycli.GrafanaOptions) *output.Result {
		got = options
		return output.OK("listed", &stycli.GrafanaListData{Kind: stycli.GrafanaListKind, Dashboards: []stycli.GrafanaDashboardListItem{}})
	}
	_ = captureStdout(t, func() {
		if err := pigstyGrafanaListCmd.RunE(pigstyGrafanaListCmd, nil); err != nil {
			t.Fatal(err)
		}
	})
	if got.Home != config.PigstyHome || got.Inventory != config.PigstyConfig || got.Endpoint != pigstyGrafanaEndpoint || got.Path != "" {
		t.Fatalf("Grafana list options = %+v", got)
	}
}

func TestStyGrafanaOptionMapping(t *testing.T) {
	restore := saveStyGrafanaState()
	defer restore()
	config.OutputFormat = config.OUTPUT_TEXT
	config.PigstyHome = "/opt/pigsty"
	config.PigstyConfig = "/etc/pigsty.yml"
	pigstyGrafanaEndpoint = "https://grafana.example/ui"
	pigstyGrafanaUsername = "operator"
	pigstyGrafanaPassword = "raw-secret"

	var got stycli.GrafanaOptions
	pigstyGrafanaInitExec = func(options stycli.GrafanaOptions) *output.Result {
		got = options
		return output.OK("initialized", &stycli.GrafanaActionData{Kind: stycli.GrafanaInitKind})
	}
	_ = captureStdout(t, func() {
		if err := pigstyGrafanaInitCmd.RunE(pigstyGrafanaInitCmd, []string{"/srv/grafana/pgsql"}); err != nil {
			t.Fatal(err)
		}
	})
	if got.Home != config.PigstyHome || got.Inventory != config.PigstyConfig || got.Path != "/srv/grafana/pgsql" || got.Endpoint != pigstyGrafanaEndpoint || got.Username != pigstyGrafanaUsername || got.Password != pigstyGrafanaPassword || got.PasswordFile != "" {
		t.Fatalf("Grafana options = %+v", got)
	}
}

func TestStyGrafanaPasswordFileMapping(t *testing.T) {
	restore := saveStyGrafanaState()
	defer restore()
	pigstyGrafanaPassword = ""
	pigstyGrafanaPasswordFile = "/run/secrets/grafana"
	options := styGrafanaOptions(nil)
	if options.Password != "" || options.PasswordFile != pigstyGrafanaPasswordFile {
		t.Fatalf("Grafana password options = %+v", options)
	}
}

func TestStyGrafanaPreferenceArgumentMapping(t *testing.T) {
	restore := saveStyGrafanaState()
	defer restore()
	config.OutputFormat = config.OUTPUT_TEXT
	var language, style string
	pigstyGrafanaLangExec = func(_ stycli.GrafanaOptions, value string) *output.Result {
		language = value
		return output.OK("language", &stycli.GrafanaActionData{Kind: stycli.GrafanaLangKind})
	}
	pigstyGrafanaStyleExec = func(_ stycli.GrafanaOptions, value string) *output.Result {
		style = value
		return output.OK("style", &stycli.GrafanaActionData{Kind: stycli.GrafanaStyleKind})
	}
	_ = captureStdout(t, func() {
		if err := pigstyGrafanaLangCmd.RunE(pigstyGrafanaLangCmd, []string{"zh-cn"}); err != nil {
			t.Fatal(err)
		}
		if err := pigstyGrafanaStyleCmd.RunE(pigstyGrafanaStyleCmd, []string{"auto"}); err != nil {
			t.Fatal(err)
		}
	})
	if language != "zh-cn" || style != "auto" {
		t.Fatalf("language=%q style=%q", language, style)
	}
}

func TestStyGrafanaTextConfirmationPinsPlan(t *testing.T) {
	restore := saveStyGrafanaState()
	defer restore()
	config.OutputFormat = config.OUTPUT_TEXT
	var calls []stycli.GrafanaOptions
	pigstyGrafanaCleanExec = func(options stycli.GrafanaOptions) *output.Result {
		calls = append(calls, options)
		data := &stycli.GrafanaActionData{
			Kind: stycli.GrafanaCleanKind, PlanDigest: "fixed-plan",
			Results: []stycli.GrafanaObjectResult{{ObjectKind: "dashboard", UID: "owned", Action: "delete", Status: "planned"}},
		}
		if !options.Yes {
			return output.Fail(output.CodeStyGrafanaConfirmationRequired, "confirmation required").WithData(data)
		}
		return output.OK("cleaned", data)
	}
	prompts := 0
	highRiskTextConfirm = func(warning, action string) error {
		prompts++
		if !strings.Contains(warning, "declared by the selected local path") || action != "the displayed Grafana clean plan" {
			t.Fatalf("warning=%q action=%q", warning, action)
		}
		return nil
	}
	_ = captureStdout(t, func() {
		if err := pigstyGrafanaCleanCmd.RunE(pigstyGrafanaCleanCmd, []string{"/srv/grafana/pgsql"}); err != nil {
			t.Fatal(err)
		}
	})
	if len(calls) != 2 || calls[0].Yes || !calls[1].Yes || calls[1].PlanDigest != "fixed-plan" || prompts != 1 {
		t.Fatalf("calls=%+v prompts=%d", calls, prompts)
	}
}

func TestStyGrafanaStructuredConfirmationFailsClosed(t *testing.T) {
	restore := saveStyGrafanaState()
	defer restore()
	config.OutputFormat = config.OUTPUT_JSON
	prompts := 0
	pigstyGrafanaCleanExec = func(options stycli.GrafanaOptions) *output.Result {
		if options.Yes {
			t.Fatal("structured output synthesized confirmation")
		}
		return output.Fail(output.CodeStyGrafanaConfirmationRequired, "confirmation required").WithData(&stycli.GrafanaActionData{Kind: stycli.GrafanaCleanKind})
	}
	highRiskTextConfirm = func(string, string) error {
		prompts++
		return nil
	}
	raw := captureStdout(t, func() {
		if err := pigstyGrafanaCleanCmd.RunE(pigstyGrafanaCleanCmd, []string{"/srv/grafana/pgsql"}); err == nil {
			t.Fatal("structured clean without --yes succeeded")
		}
	})
	if prompts != 0 || !strings.Contains(raw, `"kind":"`+stycli.GrafanaCleanKind+`"`) || strings.Contains(raw, `"next_actions"`) {
		t.Fatalf("prompts=%d output=%q", prompts, raw)
	}
}

func saveStyGrafanaState() func() {
	originalInfo := pigstyGrafanaInfoExec
	originalList := pigstyGrafanaListExec
	originalBoot := pigstyGrafanaBootExec
	originalLoad := pigstyGrafanaLoadExec
	originalInit := pigstyGrafanaInitExec
	originalDump := pigstyGrafanaDumpExec
	originalClean := pigstyGrafanaCleanExec
	originalLang := pigstyGrafanaLangExec
	originalStyle := pigstyGrafanaStyleExec
	originalConfirm := highRiskTextConfirm
	originalFormat := config.OutputFormat
	originalHome, originalInventory := config.PigstyHome, config.PigstyConfig
	originalEndpoint, originalUsername := pigstyGrafanaEndpoint, pigstyGrafanaUsername
	originalPassword, originalPasswordFile, originalDumpDir := pigstyGrafanaPassword, pigstyGrafanaPasswordFile, pigstyGrafanaDumpDir
	originalAll, originalYes, originalDryRun := pigstyGrafanaAll, pigstyGrafanaYes, pigstyGrafanaDryRun
	return func() {
		pigstyGrafanaInfoExec = originalInfo
		pigstyGrafanaListExec = originalList
		pigstyGrafanaBootExec = originalBoot
		pigstyGrafanaLoadExec = originalLoad
		pigstyGrafanaInitExec = originalInit
		pigstyGrafanaDumpExec = originalDump
		pigstyGrafanaCleanExec = originalClean
		pigstyGrafanaLangExec = originalLang
		pigstyGrafanaStyleExec = originalStyle
		highRiskTextConfirm = originalConfirm
		config.OutputFormat = originalFormat
		config.PigstyHome, config.PigstyConfig = originalHome, originalInventory
		pigstyGrafanaEndpoint, pigstyGrafanaUsername = originalEndpoint, originalUsername
		pigstyGrafanaPassword, pigstyGrafanaPasswordFile = originalPassword, originalPasswordFile
		pigstyGrafanaDumpDir = originalDumpDir
		pigstyGrafanaAll, pigstyGrafanaYes, pigstyGrafanaDryRun = originalAll, originalYes, originalDryRun
	}
}
