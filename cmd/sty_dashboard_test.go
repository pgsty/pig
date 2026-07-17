package cmd

import (
	"strconv"
	"strings"
	"testing"

	stycli "pig/cli/sty"
	"pig/internal/config"
	"pig/internal/output"

	"github.com/spf13/cobra"
)

func TestStyDashboardCommandsAndFlags(t *testing.T) {
	for _, test := range []struct {
		args []string
		want *cobra.Command
	}{
		{args: []string{"sty", "dashboard"}, want: pigstyDashboardCmd},
		{args: []string{"sty", "ds"}, want: pigstyDashboardCmd},
		{args: []string{"sty", "dashboard", "list"}, want: pigstyDashboardListCmd},
		{args: []string{"sty", "dashboard", "check"}, want: pigstyDashboardCheckCmd},
		{args: []string{"sty", "dashboard", "diff"}, want: pigstyDashboardDiffCmd},
		{args: []string{"sty", "dashboard", "init"}, want: pigstyDashboardInitCmd},
		{args: []string{"sty", "dashboard", "dump"}, want: pigstyDashboardDumpCmd},
		{args: []string{"sty", "dashboard", "clean"}, want: pigstyDashboardCleanCmd},
	} {
		found, _, err := rootCmd.Find(test.args)
		if err != nil {
			t.Fatalf("Find(%v): %v", test.args, err)
		}
		if found != test.want {
			t.Fatalf("Find(%v) = %v, want %v", test.args, found, test.want)
		}
	}
	for _, name := range []string{"endpoint", "username", "password-file", "manifest"} {
		if pigstyDashboardCmd.PersistentFlags().Lookup(name) == nil {
			t.Fatalf("pig sty dashboard is missing persistent --%s", name)
		}
	}
	for _, name := range []string{"prune", "yes"} {
		if pigstyDashboardInitCmd.Flags().Lookup(name) == nil {
			t.Fatalf("pig sty dashboard init is missing --%s", name)
		}
	}
	if flag := pigstyDashboardCleanCmd.Flags().Lookup("yes"); flag == nil || flag.Shorthand != "y" {
		t.Fatalf("pig sty dashboard clean --yes flag = %+v", flag)
	}
	if pigstyDashboardDumpCmd.Flags().Lookup("dir") == nil {
		t.Fatal("pig sty dashboard dump is missing --dir")
	}
}

func TestStyDashboardOptionMapping(t *testing.T) {
	restore := saveStyDashboardState()
	defer restore()
	config.OutputFormat = config.OUTPUT_TEXT
	config.PigstyHome = "/opt/pigsty"
	config.PigstyConfig = "/etc/pigsty.yml"
	pigstyDashboardEndpoint = "https://grafana.example/ui"
	pigstyDashboardUsername = "operator"
	pigstyDashboardPassword = "/run/secrets/grafana"
	pigstyDashboardManifest = "/srv/grafana/manifest.json"
	pigstyDashboardDumpDir = "/tmp/grafana-export"
	pigstyDashboardPrune = true
	pigstyDashboardYes = true

	var got stycli.DashboardOptions
	pigstyDashboardListExec = func(options stycli.DashboardOptions) *output.Result {
		got = options
		return output.OK("listed", &stycli.DashboardListData{Kind: stycli.DashboardListKind})
	}
	_ = captureStdout(t, func() {
		if err := pigstyDashboardListCmd.RunE(pigstyDashboardListCmd, nil); err != nil {
			t.Fatal(err)
		}
	})
	if got.Home != config.PigstyHome || got.Inventory != config.PigstyConfig || got.Endpoint != pigstyDashboardEndpoint || got.Username != pigstyDashboardUsername || got.PasswordFile != pigstyDashboardPassword || got.Manifest != pigstyDashboardManifest || got.DumpDir != pigstyDashboardDumpDir || !got.Prune || !got.Yes {
		t.Fatalf("Dashboard options = %+v", got)
	}
}

func TestStyDashboardStructuredConfirmationFailsClosed(t *testing.T) {
	restore := saveStyDashboardState()
	defer restore()
	config.OutputFormat = config.OUTPUT_JSON
	pigstyDashboardPrune = true
	pigstyDashboardYes = false
	calls := 0
	prompts := 0
	pigstyDashboardInitExec = func(options stycli.DashboardOptions) *output.Result {
		calls++
		if options.Yes {
			t.Fatal("structured mode synthesized --yes")
		}
		return output.Fail(output.CodeStyDashboardConfirmationRequired, "confirmation required").
			WithData(&stycli.DashboardActionData{Kind: stycli.DashboardInitKind, DryRun: true, Prune: true})
	}
	highRiskTextConfirm = func(string, string) error {
		prompts++
		return nil
	}
	raw := captureStdout(t, func() {
		if err := pigstyDashboardInitCmd.RunE(pigstyDashboardInitCmd, nil); err == nil {
			t.Fatal("structured prune without --yes succeeded")
		}
	})
	if calls != 1 || prompts != 0 {
		t.Fatalf("structured confirmation calls=%d prompts=%d", calls, prompts)
	}
	if !strings.Contains(raw, `"code":`+strconv.Itoa(output.CodeStyDashboardConfirmationRequired)) || !strings.Contains(raw, `"kind":"`+stycli.DashboardInitKind+`"`) {
		t.Fatalf("structured confirmation output = %q", raw)
	}
}

func TestStyDashboardTextConfirmationRerunsExactPlanWithYes(t *testing.T) {
	restore := saveStyDashboardState()
	defer restore()
	config.OutputFormat = config.OUTPUT_TEXT
	pigstyDashboardYes = false
	var yesValues []bool
	pigstyDashboardCleanExec = func(options stycli.DashboardOptions) *output.Result {
		yesValues = append(yesValues, options.Yes)
		data := &stycli.DashboardActionData{Kind: stycli.DashboardCleanKind, Results: []stycli.DashboardObjectResult{{ObjectKind: "dashboard", UID: "managed", Action: "delete", Status: "planned"}}}
		if !options.Yes {
			data.DryRun = true
			return output.Fail(output.CodeStyDashboardConfirmationRequired, "confirmation required").WithData(data)
		}
		data.Results[0].Status = "applied"
		return output.OK("cleaned", data)
	}
	prompts := 0
	highRiskTextConfirm = func(warning, action string) error {
		prompts++
		if !strings.Contains(warning, "active manifest-owned") || action != "pig sty dashboard clean" {
			t.Fatalf("prompt warning=%q action=%q", warning, action)
		}
		return nil
	}
	raw := captureStdout(t, func() {
		if err := pigstyDashboardCleanCmd.RunE(pigstyDashboardCleanCmd, nil); err != nil {
			t.Fatal(err)
		}
	})
	if len(yesValues) != 2 || yesValues[0] || !yesValues[1] || prompts != 1 {
		t.Fatalf("text confirmation yes=%v prompts=%d", yesValues, prompts)
	}
	if !strings.Contains(raw, "dashboard managed-resource deletion plan") || !strings.Contains(raw, "cleaned") {
		t.Fatalf("text confirmation output = %q", raw)
	}
}

func saveStyDashboardState() func() {
	originalList := pigstyDashboardListExec
	originalCheck := pigstyDashboardCheckExec
	originalDiff := pigstyDashboardDiffExec
	originalInit := pigstyDashboardInitExec
	originalDump := pigstyDashboardDumpExec
	originalClean := pigstyDashboardCleanExec
	originalConfirm := highRiskTextConfirm
	originalFormat := config.OutputFormat
	originalHome, originalInventory := config.PigstyHome, config.PigstyConfig
	originalEndpoint, originalUsername := pigstyDashboardEndpoint, pigstyDashboardUsername
	originalPassword, originalManifest := pigstyDashboardPassword, pigstyDashboardManifest
	originalDumpDir := pigstyDashboardDumpDir
	originalPrune, originalYes := pigstyDashboardPrune, pigstyDashboardYes
	return func() {
		pigstyDashboardListExec = originalList
		pigstyDashboardCheckExec = originalCheck
		pigstyDashboardDiffExec = originalDiff
		pigstyDashboardInitExec = originalInit
		pigstyDashboardDumpExec = originalDump
		pigstyDashboardCleanExec = originalClean
		highRiskTextConfirm = originalConfirm
		config.OutputFormat = originalFormat
		config.PigstyHome, config.PigstyConfig = originalHome, originalInventory
		pigstyDashboardEndpoint, pigstyDashboardUsername = originalEndpoint, originalUsername
		pigstyDashboardPassword, pigstyDashboardManifest = originalPassword, originalManifest
		pigstyDashboardDumpDir = originalDumpDir
		pigstyDashboardPrune, pigstyDashboardYes = originalPrune, originalYes
	}
}
