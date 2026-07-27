package cmd

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"pig/cli/patroni"
	"pig/internal/config"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestPatroniUnknownNativeCommandAndFlagsForwardUnchanged(t *testing.T) {
	var got []string
	withPatroniRunner(t, func(_ string, args []string) error {
		got = append([]string(nil), args...)
		return nil
	})
	want := []string{"future-command", "cluster name", "--future-flag", "a b", "x=y=z", `quote'\"value`}
	if err := executePtTest(t, append([]string{"pt"}, want...)...); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("forwarded args = %#v, want %#v", got, want)
	}
}

func TestPatroniDCSURLShorthand(t *testing.T) {
	var got []string
	withPatroniRunner(t, func(_ string, args []string) error {
		got = append([]string(nil), args...)
		return nil
	})
	if err := executePtTest(t, "pt", "-d", "etcd3://127.0.0.1:2379", "list"); err != nil {
		t.Fatal(err)
	}
	if patroniDCSURL != "etcd3://127.0.0.1:2379" {
		t.Fatalf("DCS URL = %q", patroniDCSURL)
	}
	if !reflect.DeepEqual(got, []string{"list"}) {
		t.Fatalf("native args = %#v", got)
	}
}

func TestPatroniQueryCIsNativeWhileWrapperCSelectsConfig(t *testing.T) {
	var calls [][]string
	withPatroniRunner(t, func(_ string, args []string) error {
		calls = append(calls, append([]string(nil), args...))
		return nil
	})
	if err := executePtTest(t, "pt", "query", "pg-test", "-c", "select now()", "-d", "postgres", "-U", "dbuser"); err != nil {
		t.Fatal(err)
	}

	t.Chdir(t.TempDir())
	if err := executePtTest(t, "pt", "-c", "conf/patroni.yml", "list", "--watch", "5"); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 {
		t.Fatalf("calls = %d", len(calls))
	}
	if !reflect.DeepEqual(calls[0], []string{"query", "pg-test", "-c", "select now()", "-d", "postgres", "-U", "dbuser"}) {
		t.Fatalf("query args = %#v", calls[0])
	}
	wantConfig := filepath.Join(mustCmdGetwd(t), "conf", "patroni.yml")
	selected, err := patroni.ResolveConfigFile("conf/patroni.yml", "postgres")
	if err != nil {
		t.Fatal(err)
	}
	if selected != wantConfig {
		t.Fatalf("selected config = %q, want %q", selected, wantConfig)
	}
	if !reflect.DeepEqual(calls[1], []string{"list", "--watch", "5"}) {
		t.Fatalf("list args = %#v", calls[1])
	}
}

func TestPatroniLeadingDashEscapesRootOptionsAndLocalCollisions(t *testing.T) {
	var calls [][]string
	withPatroniRunner(t, func(_ string, args []string) error {
		calls = append(calls, append([]string(nil), args...))
		return nil
	})

	inputs := []struct {
		input []string
		want  []string
	}{{
		input: []string{"pt", "--", "--future-root-option", "VALUE", "future-command", "--x"},
		want:  []string{"--future-root-option", "VALUE", "future-command", "--x"},
	}}
	for _, collision := range []string{"set", "start", "up", "stop", "dn", "service", "svc", "status", "st", "log", "l"} {
		inputs = append(inputs, struct {
			input []string
			want  []string
		}{
			input: []string{"pt", "--", collision, "opaque=value"},
			want:  []string{collision, "opaque=value"},
		})
	}
	wants := make([][]string, 0, len(inputs))
	for _, test := range inputs {
		input := test.input
		if err := executePtTest(t, input...); err != nil {
			t.Fatalf("%v: %v", input, err)
		}
		wants = append(wants, test.want)
	}
	if !reflect.DeepEqual(calls, wants) {
		t.Fatalf("calls = %#v, want %#v", calls, wants)
	}
}

func TestPatroniHelpBoundary(t *testing.T) {
	calls := 0
	var got [][]string
	withPatroniRunner(t, func(_ string, args []string) error {
		calls++
		got = append(got, append([]string(nil), args...))
		return nil
	})
	if err := executePtTest(t, "pt"); err != nil {
		t.Fatal(err)
	}
	if err := executePtTest(t, "pt", "--help"); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatalf("Pig help invoked runner %d times", calls)
	}
	if err := executePtTest(t, "pt", "list", "--help"); err != nil {
		t.Fatal(err)
	}
	if err := executePtTest(t, "pt", "restart", "-h"); err != nil {
		t.Fatal(err)
	}
	if err := executePtTest(t, "pt", "--", "--help"); err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"list", "--help"}, {"restart", "-h"}, {"--help"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("native help calls = %#v, want %#v", got, want)
	}
}

func TestPatroniNativeStructuredOutputBoundary(t *testing.T) {
	calls := 0
	withPatroniRunner(t, func(_ string, _ []string) error {
		calls++
		return nil
	})
	for _, args := range [][]string{
		{"-o", "json", "pt", "list"},
		{"pt", "-o", "json", "list"},
	} {
		if err := executePtTest(t, args...); err == nil {
			t.Fatalf("%v succeeded", args)
		}
	}
	if calls != 0 {
		t.Fatalf("structured native requests executed %d times", calls)
	}

	var got []string
	withPatroniRunner(t, func(_ string, args []string) error {
		got = append([]string(nil), args...)
		return nil
	})
	if err := executePtTest(t, "pt", "list", "-o", "json", "--format", "yaml"); err != nil {
		t.Fatal(err)
	}
	want := []string{"list", "-o", "json", "--format", "yaml"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("output-looking native args = %#v, want %#v", got, want)
	}
}

func TestPatroniEarlyOutputScanStopsAtNativeBoundary(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"-o", "json", "pt", "list"}, "json"},
		{[]string{"pt", "-o", "yaml", "list"}, "yaml"},
		{[]string{"pt", "list", "-o", "json"}, "text"},
		{[]string{"pt", "query", "-c", "select 1", "-o", "json"}, "text"},
		{[]string{"pt", "-d", "etcd3://127.0.0.1:2379", "list", "-o", "json"}, "text"},
		{[]string{"pt", "-detcd3://127.0.0.1:2379", "list", "-o", "json"}, "text"},
		{[]string{"pt", "set", "work_mem=4MB", "-o", "json"}, "json"},
	}
	for _, tt := range tests {
		if got := outputFormatFromArgs(tt.args); got != tt.want {
			t.Fatalf("outputFormatFromArgs(%v) = %q, want %q", tt.args, got, tt.want)
		}
	}
	native := []string{"pt", "list", "--help", "-o", "json"}
	if got := reorderOutputBeforeHelp(native); !reflect.DeepEqual(got, native) {
		t.Fatalf("native argv reordered: %#v", got)
	}
}

func TestPatroniSetCommandValidationTranslationPlanAndConfirmation(t *testing.T) {
	var calls int
	var gotPairs []patroni.SetPair
	var gotForce bool
	oldSet := patroniSetExec
	t.Cleanup(func() { patroniSetExec = oldSet })
	patroniSetExec = func(_ string, pairs []patroni.SetPair, force bool) error {
		calls++
		gotPairs = append([]patroni.SetPair(nil), pairs...)
		gotForce = force
		return nil
	}

	for _, args := range [][]string{
		{"pt", "set"},
		{"pt", "set", "missing"},
		{"pt", "set", "=bad"},
		{"pt", "set", "postgresql.use_pg_rewind=true"},
	} {
		if err := executePtTest(t, args...); err == nil {
			t.Fatalf("%v succeeded", args)
		}
	}
	if calls != 0 {
		t.Fatalf("invalid set executed %d times", calls)
	}

	if err := executePtTest(t, "pt", "set", "ttl=60", "work_mem=a=b", "--yes"); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || !gotForce || len(gotPairs) != 2 || gotPairs[0].Route != patroni.SetRoutePatroni || gotPairs[1].Route != patroni.SetRoutePostgreSQL {
		t.Fatalf("calls=%d force=%v pairs=%#v", calls, gotForce, gotPairs)
	}

	if err := executePtTest(t, "pt", "set", "ttl=60"); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || gotForce {
		t.Fatalf("default set calls=%d force=%v", calls, gotForce)
	}

	if err := executePtTest(t, "pt", "set", "ttl=60", "--yes", "--plan"); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("plan mutated configuration: calls=%d", calls)
	}

	if err := executePtTest(t, "pt", "set", "ttl=60", "-o", "json"); err == nil {
		t.Fatal("structured set without --yes succeeded")
	}
	if calls != 2 {
		t.Fatalf("unconfirmed structured set executed: calls=%d", calls)
	}
	if err := executePtTest(t, "pt", "set", "ttl=60", "--yes", "-o", "json"); err != nil {
		t.Fatal(err)
	}
	if calls != 3 || !gotForce {
		t.Fatalf("confirmed structured set calls=%d force=%v", calls, gotForce)
	}
}

func TestPatroniSetRestartHintRequiresExplicitCluster(t *testing.T) {
	raw := captureStderr(t, func() {
		printPatroniSetHints([]string{"shared_buffers=4GB"})
	})
	if !strings.Contains(raw, "$ pig pt list") ||
		!strings.Contains(raw, "pig pt restart CLUSTER --pending") {
		t.Fatalf("restart hint is incomplete: %q", raw)
	}
	if !strings.Contains(raw, "If changed") ||
		!strings.Contains(raw, "If the change was applied") ||
		!strings.Contains(raw, "If members report pending restart") {
		t.Fatalf("restart hint claims an unverified change: %q", raw)
	}
	if strings.Contains(raw, "$ pig pt restart --pending") {
		t.Fatalf("restart hint emitted invalid replayable command: %q", raw)
	}
	raw = captureStderr(t, func() {
		printPatroniSetHints([]string{"work_mem=4MB"})
	})
	if !strings.Contains(raw, "If the change was applied") {
		t.Fatalf("non-restart hint claims an unverified change: %q", raw)
	}
}

func TestPatroniLocalDispatchAndNativeRestartConfig(t *testing.T) {
	oldSystemctl := patroniSystemctlExec
	t.Cleanup(func() { patroniSystemctlExec = oldSystemctl })
	var actions []string
	patroniSystemctlExec = func(action string) error {
		actions = append(actions, action)
		return nil
	}
	var native [][]string
	withPatroniRunner(t, func(_ string, args []string) error {
		native = append(native, append([]string(nil), args...))
		return nil
	})

	for _, args := range [][]string{
		{"pt", "start"},
		{"pt", "dn"},
		{"pt", "svc", "restart"},
		{"pt", "service", "rl"},
		{"pt", "svc", "st"},
	} {
		if err := executePtTest(t, args...); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	wantActions := []string{"start", "stop", "restart", "reload", "status"}
	if !reflect.DeepEqual(actions, wantActions) {
		t.Fatalf("service actions = %#v, want %#v", actions, wantActions)
	}

	for _, args := range [][]string{
		{"pt", "restart", "pg-test", "--pending"},
		{"pt", "config", "pg", "work_mem=4MB"},
	} {
		if err := executePtTest(t, args...); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	wantNative := [][]string{
		{"restart", "pg-test", "--pending"},
		{"config", "pg", "work_mem=4MB"},
	}
	if !reflect.DeepEqual(native, wantNative) {
		t.Fatalf("native calls = %#v, want %#v", native, wantNative)
	}

	for name, want := range map[string]*cobra.Command{
		"status": patroniStatusCmd,
		"st":     patroniStatusCmd,
		"log":    patroniLogCmd,
		"l":      patroniLogCmd,
	} {
		found, _, err := patroniCmd.Find([]string{name})
		if err != nil || found != want {
			t.Fatalf("Find(%q) = %v, %v; want %v", name, found, err, want)
		}
	}
}

func TestPatroniServiceDoesNotResolveConfig(t *testing.T) {
	oldSystemctl := patroniSystemctlExec
	t.Cleanup(func() { patroniSystemctlExec = oldSystemctl })
	patroniSystemctlExec = func(string) error { return nil }
	t.Setenv("PATRONICTL_CONFIG_FILE", "relative-should-not-be-resolved.yml")
	if err := executePtTestWithoutConfigEnv(t, "pt", "svc", "start"); err != nil {
		t.Fatal(err)
	}
	if got := patroni.ConfigFile(); got != patroni.DefaultConfigPath {
		t.Fatalf("service resolved config to %q", got)
	}
}

func withPatroniRunner(t *testing.T, runner func(string, []string) error) {
	t.Helper()
	old := patroniPassthroughExec
	patroniPassthroughExec = runner
	t.Cleanup(func() { patroniPassthroughExec = old })
}

func executePtTest(t *testing.T, args ...string) error {
	t.Helper()
	t.Setenv("PATRONICTL_CONFIG_FILE", "/tmp/pig-pt-test.yml")
	return executePtTestWithoutConfigEnv(t, args...)
}

func executePtTestWithoutConfigEnv(t *testing.T, args ...string) error {
	t.Helper()
	resetPtTestState()
	rootCmd.SetOut(io.Discard)
	rootCmd.SetErr(io.Discard)
	rootCmd.SilenceErrors = true
	rootCmd.SilenceUsage = true
	rootCmd.SetArgs(args)
	err := rootCmd.Execute()
	for _, child := range rootCmd.Commands() {
		if child.Name() == "help" {
			child.Args = cobra.ArbitraryArgs
		}
	}
	return err
}

func resetPtTestState() {
	resetPtCommandFlags(rootCmd)
	patroniDBSU = ""
	patroniSetYes = false
	patroniSetPlan = false
	patroniLogFollow = false
	patroniLogLines = 50
	patroniLogDir = ""
	patroniConfigFile = ""
	patroniDCSURL = ""
	patroniInsecure = false
	outputFormat = "text"
	config.OutputFormat = config.OUTPUT_TEXT
}

func resetPtCommandFlags(command *cobra.Command) {
	reset := func(flag *pflag.Flag) {
		_ = flag.Value.Set(flag.DefValue)
		flag.Changed = false
	}
	command.Flags().VisitAll(reset)
	command.PersistentFlags().VisitAll(reset)
	for _, child := range command.Commands() {
		resetPtCommandFlags(child)
	}
}

func mustCmdGetwd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return wd
}
