package patroni

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"

	"github.com/sirupsen/logrus"
)

func TestResolveConfigFileExplicitAndEnvironmentAreAuthoritative(t *testing.T) {
	oldUser := config.CurrentUser
	t.Cleanup(func() { config.CurrentUser = oldUser })
	config.CurrentUser = "nobody"
	t.Chdir(t.TempDir())

	t.Setenv("PATRONICTL_CONFIG_FILE", "env/patronictl.yml")
	got, err := ResolveConfigFile("explicit/patroni.yml", "postgres")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(mustGetwd(t), "explicit", "patroni.yml")
	if got != want {
		t.Fatalf("explicit path = %q, want %q", got, want)
	}

	got, err = ResolveConfigFile("", "postgres")
	if err != nil {
		t.Fatal(err)
	}
	want = filepath.Join(mustGetwd(t), "env", "patronictl.yml")
	if got != want {
		t.Fatalf("environment path = %q, want %q", got, want)
	}
}

func TestResolveConfigFileDirectConventionalMatrix(t *testing.T) {
	oldReadable := patroniConfigReadable
	oldUser := config.CurrentUser
	t.Cleanup(func() {
		patroniConfigReadable = oldReadable
		config.CurrentUser = oldUser
	})
	t.Setenv("PATRONICTL_CONFIG_FILE", "")
	config.CurrentUser = "postgres"

	tests := []struct {
		name     string
		readable map[string]bool
		want     string
	}{
		{"both", map[string]bool{DefaultConfigPath: true, InfraConfigPath: true}, DefaultConfigPath},
		{"etc only", map[string]bool{DefaultConfigPath: true}, DefaultConfigPath},
		{"infra only", map[string]bool{InfraConfigPath: true}, InfraConfigPath},
		{"neither", map[string]bool{}, DefaultConfigPath},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			patroniConfigReadable = func(path string) bool { return tt.readable[path] }
			got, err := ResolveConfigFile("", "postgres")
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("ResolveConfigFile() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveConfigFileUsesOneDBSUProbe(t *testing.T) {
	oldProbe := patroniDBSUCommandStdout
	oldUser := config.CurrentUser
	t.Cleanup(func() {
		patroniDBSUCommandStdout = oldProbe
		config.CurrentUser = oldUser
	})
	t.Setenv("PATRONICTL_CONFIG_FILE", "")
	config.CurrentUser = "root"

	calls := 0
	patroniDBSUCommandStdout = func(dbsu string, args []string) (string, error) {
		calls++
		if dbsu != "postgres" {
			t.Fatalf("dbsu = %q", dbsu)
		}
		want := []string{"sh", "-c", conventionalConfigProbe}
		if !reflect.DeepEqual(args, want) {
			t.Fatalf("probe args = %#v, want %#v", args, want)
		}
		return "infra", nil
	}
	got, err := ResolveConfigFile("", "postgres")
	if err != nil {
		t.Fatal(err)
	}
	if got != InfraConfigPath || calls != 1 {
		t.Fatalf("got path=%q calls=%d", got, calls)
	}

	wantErr := errors.New("cannot launch su")
	patroniDBSUCommandStdout = func(string, []string) (string, error) { return "", wantErr }
	if _, err := ResolveConfigFile("", "postgres"); !errors.Is(err, wantErr) {
		t.Fatalf("probe failure = %v, want wrapped %v", err, wantErr)
	}
}

func TestPassthroughPrependsOnlyRootOptionsAndDoesNotLogValues(t *testing.T) {
	preservePatroniCommandOptions(t)
	oldLookPath := patroniLookPath
	oldExec := patroniPassthroughDBSUCommand
	oldOut := logrus.StandardLogger().Out
	oldLevel := logrus.GetLevel()
	t.Cleanup(func() {
		patroniLookPath = oldLookPath
		patroniPassthroughDBSUCommand = oldExec
		logrus.SetOutput(oldOut)
		logrus.SetLevel(oldLevel)
	})

	var logs bytes.Buffer
	logrus.SetOutput(&logs)
	logrus.SetLevel(logrus.DebugLevel)
	patroniLookPath = func(string) (string, error) { return "/opt/bin/patronictl", nil }
	var gotDBSU string
	var gotArgs []string
	patroniPassthroughDBSUCommand = func(dbsu string, args []string) error {
		gotDBSU = dbsu
		gotArgs = append([]string(nil), args...)
		return nil
	}

	SetCommandOptions(CommandOptions{ConfigFile: "/secret/config.yml", DCSURL: "https://secret-dcs", Insecure: true})
	if err := ResolveCommandOptions("alice"); err != nil {
		t.Fatal(err)
	}
	native := []string{"query", "-c", "select 'secret sql'", "ttl=secret=value"}
	if err := Passthrough("alice", native); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"/opt/bin/patronictl", "-c", "/secret/config.yml",
		"--dcs-url", "https://secret-dcs", "--insecure",
		"query", "-c", "select 'secret sql'", "ttl=secret=value",
	}
	if gotDBSU != "alice" || !reflect.DeepEqual(gotArgs, want) {
		t.Fatalf("dbsu=%q args=%#v, want dbsu=alice args=%#v", gotDBSU, gotArgs, want)
	}
	if !strings.Contains(logs.String(), "patronictl config selected: source=explicit path=/secret/config.yml") {
		t.Fatalf("debug log missed config selection: %s", logs.String())
	}
	for _, secret := range []string{"secret-dcs", "secret sql", "ttl=secret"} {
		if strings.Contains(logs.String(), secret) {
			t.Fatalf("debug log leaked %q: %s", secret, logs.String())
		}
	}
}

func TestPassthroughNotFoundIsCoded(t *testing.T) {
	oldLookPath := patroniLookPath
	t.Cleanup(func() { patroniLookPath = oldLookPath })
	patroniLookPath = func(string) (string, error) { return "", os.ErrNotExist }
	err := Passthrough("postgres", []string{"list"})
	var coded interface{ StatusCode() int }
	if !errors.As(err, &coded) || coded.StatusCode() != output.CodePtNotFound {
		t.Fatalf("error = %#v, want CodePtNotFound", err)
	}
	if got := utils.ExitCode(err); got != 4 {
		t.Fatalf("exit code = %d, want dependency exit 4", got)
	}
}

func TestPassthroughNativeHelpSkipsConfigResolutionAndRootInjection(t *testing.T) {
	preservePatroniCommandOptions(t)
	oldLookPath := patroniLookPath
	oldProbe := patroniDBSUCommandStdout
	oldExec := patroniPassthroughDBSUCommand
	oldUser := config.CurrentUser
	t.Cleanup(func() {
		patroniLookPath = oldLookPath
		patroniDBSUCommandStdout = oldProbe
		patroniPassthroughDBSUCommand = oldExec
		config.CurrentUser = oldUser
	})
	t.Setenv("PATRONICTL_CONFIG_FILE", "")
	config.CurrentUser = "root"
	patroniLookPath = func(string) (string, error) { return "/opt/bin/patronictl", nil }

	probes := 0
	patroniDBSUCommandStdout = func(string, []string) (string, error) {
		probes++
		return "", errors.New("config probe must not run for native help")
	}

	tests := [][]string{
		{"--help"},
		{"list", "--help"},
		{"restart", "-h"},
	}
	var calls [][]string
	patroniPassthroughDBSUCommand = func(dbsu string, args []string) error {
		if dbsu != "postgres" {
			t.Fatalf("dbsu = %q, want postgres", dbsu)
		}
		calls = append(calls, append([]string(nil), args...))
		return nil
	}
	for _, native := range tests {
		SetCommandOptions(CommandOptions{
			DCSURL:   "https://must-not-be-injected",
			Insecure: true,
		})
		if err := Passthrough("postgres", native); err != nil {
			t.Fatalf("Passthrough(%v): %v", native, err)
		}
	}
	if probes != 0 {
		t.Fatalf("native help performed %d config probes", probes)
	}
	want := make([][]string, 0, len(tests))
	for _, native := range tests {
		want = append(want, append([]string{"/opt/bin/patronictl"}, native...))
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("native help calls = %#v, want %#v", calls, want)
	}
}

func TestPatronictlHelpRequestedRecognizesOnlyCanonicalHelpForms(t *testing.T) {
	tests := []struct {
		args []string
		want bool
	}{
		{[]string{"--help"}, true},
		{[]string{"list", "--help"}, true},
		{[]string{"restart", "-h"}, true},
		{[]string{"query", "-c", "select '--help'"}, false},
		{[]string{"query", "-c", "--help"}, false},
		{[]string{"restart", "pg-meta", "--help"}, false},
		{[]string{"query", "--", "--help"}, false},
		{[]string{"--future-root-option", "--help"}, false},
		{[]string{"list", "--helper"}, false},
	}
	for _, tt := range tests {
		if got := patronictlHelpRequested(tt.args); got != tt.want {
			t.Errorf("patronictlHelpRequested(%v) = %v, want %v", tt.args, got, tt.want)
		}
	}
}

func TestPassthroughResolvesBinaryBeforeDBSUConfigProbe(t *testing.T) {
	preservePatroniCommandOptions(t)
	oldLookPath := patroniLookPath
	oldProbe := patroniDBSUCommandStdout
	oldExec := patroniPassthroughDBSUCommand
	oldUser := config.CurrentUser
	t.Cleanup(func() {
		patroniLookPath = oldLookPath
		patroniDBSUCommandStdout = oldProbe
		patroniPassthroughDBSUCommand = oldExec
		config.CurrentUser = oldUser
	})
	t.Setenv("PATRONICTL_CONFIG_FILE", "")
	config.CurrentUser = "root"
	SetCommandOptions(CommandOptions{})

	var events []string
	patroniLookPath = func(string) (string, error) {
		events = append(events, "lookup")
		return "/bin/patronictl", nil
	}
	patroniDBSUCommandStdout = func(string, []string) (string, error) {
		events = append(events, "probe")
		return "fallback", nil
	}
	patroniPassthroughDBSUCommand = func(string, []string) error {
		events = append(events, "exec")
		return nil
	}
	if err := Passthrough("postgres", []string{"list"}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"lookup", "probe", "exec"}; !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %#v, want %#v", events, want)
	}
}

func TestParseSetPairsClassificationAndPreservation(t *testing.T) {
	scalarKeys := []string{
		"loop_wait", "ttl", "retry_timeout", "primary_race_backoff",
		"maximum_lag_on_failover", "maximum_lag_on_syncnode", "max_timelines_history",
		"primary_start_timeout", "primary_stop_timeout", "synchronous_mode",
		"synchronous_mode_strict", "synchronous_node_count", "failsafe_mode",
		"check_timeline", "member_slots_ttl",
	}
	for _, key := range scalarKeys {
		pairs, err := ParseSetPairs([]string{key + "=1"})
		if err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		if pairs[0].Route != SetRoutePatroni {
			t.Fatalf("%s routed to %q", key, pairs[0].Route)
		}
	}

	tokens := []string{"max_connections=200", "timescaledb.telemetry_level=off", "work_mem=a=b", "empty=", "delete=null"}
	pairs, err := ParseSetPairs(tokens)
	if err != nil {
		t.Fatal(err)
	}
	for i, pair := range pairs {
		if pair.Route != SetRoutePostgreSQL || pair.Token != tokens[i] {
			t.Fatalf("pair[%d] = %#v", i, pair)
		}
	}
}

func TestParseSetPairsRejectsMalformedAndStructuralPaths(t *testing.T) {
	for _, tokens := range [][]string{nil, {}, {"missing"}, {"=value"}, {"  =value"}} {
		if _, err := ParseSetPairs(tokens); err == nil {
			t.Fatalf("ParseSetPairs(%#v) succeeded", tokens)
		}
	}
	for _, token := range []string{
		"postgresql.use_pg_rewind=true",
		"standby_cluster.host=10.0.0.1",
		"slots.replica=physical",
		"ignore_slots.foo=bar",
	} {
		_, err := ParseSetPairs([]string{token})
		if err == nil || !strings.Contains(err.Error(), "pig pt edit-config --set") {
			t.Fatalf("ParseSetPairs(%q) error = %v", token, err)
		}
	}
}

func TestParseSetPairsDoesNotEchoMalformedSecret(t *testing.T) {
	const secret = "bare-secret-value"
	_, err := ParseSetPairs([]string{secret})
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("malformed set error leaked input: %v", err)
	}
}

func TestBuildSetArgsMixedOrderAndForce(t *testing.T) {
	pairs, err := ParseSetPairs([]string{"ttl=60", "work_mem=a=b", "synchronous_mode=on", "custom.key="})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"edit-config",
		"--set", "ttl=60",
		"--pg", "work_mem=a=b",
		"--set", "synchronous_mode=on",
		"--pg", "custom.key=",
	}
	if got := BuildSetArgs(pairs, false); !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildSetArgs(false) = %#v, want %#v", got, want)
	}
	want = append(want, "--force")
	if got := BuildSetArgs(pairs, true); !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildSetArgs(true) = %#v, want %#v", got, want)
	}
}

func TestApplySetUsesOnePassthroughInvocation(t *testing.T) {
	preservePatroniCommandOptions(t)
	oldLookPath := patroniLookPath
	oldExec := patroniPassthroughDBSUCommand
	t.Cleanup(func() {
		patroniLookPath = oldLookPath
		patroniPassthroughDBSUCommand = oldExec
	})
	patroniLookPath = func(string) (string, error) { return "bin/patronictl", nil }
	SetCommandOptions(CommandOptions{ConfigFile: "/etc/test.yml"})
	if err := ResolveCommandOptions("postgres"); err != nil {
		t.Fatal(err)
	}
	pairs, _ := ParseSetPairs([]string{"ttl=60", "max_connections=200"})
	calls := 0
	var got []string
	patroniPassthroughDBSUCommand = func(_ string, args []string) error {
		calls++
		got = append([]string(nil), args...)
		return nil
	}
	if err := ApplySet("postgres", pairs, false); err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(mustGetwd(t), "bin", "patronictl"), "-c", "/etc/test.yml", "edit-config", "--set", "ttl=60", "--pg", "max_connections=200"}
	if calls != 1 || !reflect.DeepEqual(got, want) {
		t.Fatalf("calls=%d args=%#v, want %#v", calls, got, want)
	}
}

func TestSetNextActionsOnlyAnalyzePostgreSQLPairs(t *testing.T) {
	pairs, _ := ParseSetPairs([]string{"ttl=60", "shared_buffers=4GB"})
	pg := PostgreSQLSetTokens(pairs)
	if !reflect.DeepEqual(pg, []string{"shared_buffers=4GB"}) {
		t.Fatalf("PostgreSQLSetTokens() = %#v", pg)
	}
	actions := SetNextActions(pg)
	if len(actions) != 1 || actions[0].Command != "pig pt list" {
		t.Fatalf("SetNextActions() = %#v", actions)
	}
	if !strings.Contains(actions[0].Reason, "pig pt restart CLUSTER --pending") ||
		strings.Contains(actions[0].Reason, "pig pt restart --pending") {
		t.Fatalf("restart guidance must require an explicit cluster: %#v", actions)
	}
	if got := SetNextActions(PostgreSQLSetTokens(pairs[:1])); len(got) != 0 {
		t.Fatalf("Patroni-only actions = %#v", got)
	}
}

func mustGetwd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return wd
}

func preservePatroniCommandOptions(t *testing.T) {
	t.Helper()
	oldOptions := commandOptions
	oldResolved := commandOptionsResolved
	t.Cleanup(func() {
		commandOptions = oldOptions
		commandOptionsResolved = oldResolved
	})
}

func TestDBSUSelectionPriority(t *testing.T) {
	t.Setenv("PIG_DBSU", "env-user")
	if got := utils.GetDBSU("explicit-user"); got != "explicit-user" {
		t.Fatalf("explicit DBSU = %q", got)
	}
	if got := utils.GetDBSU(""); got != "env-user" {
		t.Fatalf("environment DBSU = %q", got)
	}
	t.Setenv("PIG_DBSU", "")
	if got := utils.GetDBSU(""); got != "postgres" {
		t.Fatalf("default DBSU = %q", got)
	}
}
