package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"io"
	"os"
	"path/filepath"
	postgrescli "pig/cli/postgres"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"
	"strings"
	"testing"
	"time"
)

func TestPgCloneCommandIsRegistered(t *testing.T) {
	pgClone, _, err := pgTestRootCmd.Find([]string{"pg", "clone"})
	if err != nil {
		t.Fatalf("pg clone command not found: %v", err)
	}
	if pgClone == nil || pgClone.Name() != "clone" {
		t.Fatalf("pg clone command = %v, want clone", pgClone)
	}
}

func TestPgCloneAcceptsOptionalDestinationDatabase(t *testing.T) {
	pgClone, _, err := pgTestRootCmd.Find([]string{"pg", "clone"})
	if err != nil {
		t.Fatalf("pg clone command not found: %v", err)
	}

	for _, args := range [][]string{{"app"}, {"app", "app_1"}} {
		if err := pgClone.Args(pgClone, args); err != nil {
			t.Fatalf("pg clone Args(%v) returned error: %v", args, err)
		}
	}

	if err := pgClone.Args(pgClone, nil); err == nil {
		t.Fatal("pg clone should reject missing source database")
	}
	if err := pgClone.Args(pgClone, []string{"app", "app_1", "extra"}); err == nil {
		t.Fatal("pg clone should reject extra positional arguments")
	}
}

func TestPgCloneSupportsPlanOnly(t *testing.T) {
	pgClone, _, err := pgTestRootCmd.Find([]string{"pg", "clone"})
	if err != nil {
		t.Fatalf("pg clone command not found: %v", err)
	}
	if pgClone.PersistentFlags().Lookup("plan") == nil {
		t.Fatal("--plan flag not found on pg clone command")
	}
	if pgClone.PersistentFlags().Lookup("dry-run") != nil {
		t.Fatal("--dry-run alias should not exist on pg clone command")
	}
}

func TestPgCloneDoesNotExposeInstanceOnlyFlags(t *testing.T) {
	pgClone, _, err := pgTestRootCmd.Find([]string{"pg", "clone"})
	if err != nil {
		t.Fatalf("pg clone command not found: %v", err)
	}
	for _, name := range []string{"no-start", "replace", "mode", "no-kill", "strategy", "tablespace"} {
		if pgClone.Flags().Lookup(name) != nil {
			t.Fatalf("pg clone should not expose --%s", name)
		}
	}
}

func TestPgCloneExposesMinimalCloneFlags(t *testing.T) {
	pgClone, _, err := pgTestRootCmd.Find([]string{"pg", "clone"})
	if err != nil {
		t.Fatalf("pg clone command not found: %v", err)
	}
	for _, name := range []string{"owner", "conn-limit", "port", "conn-db"} {
		if pgClone.Flags().Lookup(name) == nil {
			t.Fatalf("pg clone should expose --%s", name)
		}
	}
}

func TestPgCloneConnLimitHelpMentionsUnlimited(t *testing.T) {
	pgClone, _, err := pgTestRootCmd.Find([]string{"pg", "clone"})
	if err != nil {
		t.Fatalf("pg clone command not found: %v", err)
	}
	flag := pgClone.Flags().Lookup("conn-limit")
	if flag == nil {
		t.Fatal("pg clone should expose --conn-limit")
	}
	if !strings.Contains(flag.Usage, "-1 = no limit") {
		t.Fatalf("--conn-limit usage = %q, want -1 semantics", flag.Usage)
	}
}

func TestPgForkCommandIsRegistered(t *testing.T) {
	pgFork, _, err := pgTestRootCmd.Find([]string{"pg", "fork"})
	if err != nil {
		t.Fatalf("pg fork command not found: %v", err)
	}
	if pgFork == nil || pgFork.Name() != "fork" {
		t.Fatalf("pg fork command = %v, want fork", pgFork)
	}
}

func TestTopLevelForkIsNotRegistered(t *testing.T) {
	rootFork, _, err := pgTestRootCmd.Find([]string{"fork"})
	if err == nil || rootFork != pgTestRootCmd {
		t.Fatalf("top-level fork should not be registered, got cmd=%v err=%v", rootFork, err)
	}
}

func TestPgForkSupportsPlanOnly(t *testing.T) {
	pgFork, _, err := pgTestRootCmd.Find([]string{"pg", "fork"})
	if err != nil {
		t.Fatalf("pg fork command not found: %v", err)
	}
	if pgFork.PersistentFlags().Lookup("plan") == nil {
		t.Fatal("--plan flag not found on pg fork command")
	}
	if pgFork.PersistentFlags().Lookup("dry-run") != nil {
		t.Fatal("--dry-run alias should not exist on pg fork command")
	}
}

func TestPgForkRegistersLifecycleSubcommands(t *testing.T) {
	for _, args := range [][]string{
		{"pg", "fork", "init"},
		{"pg", "fork", "list"},
		{"pg", "fork", "start"},
		{"pg", "fork", "stop"},
		{"pg", "fork", "rm"},
	} {
		cmd, _, err := pgTestRootCmd.Find(args)
		if err != nil {
			t.Fatalf("%v command not found: %v", args, err)
		}
		if cmd == nil || cmd.Name() != args[len(args)-1] {
			t.Fatalf("%v resolved to %v", args, cmd)
		}
	}
}

func TestPgForkDoesNotUseRootCommandGroup(t *testing.T) {
	pgFork, _, err := pgTestRootCmd.Find([]string{"pg", "fork"})
	if err != nil {
		t.Fatalf("pg fork command not found: %v", err)
	}
	if pgFork.GroupID != "" {
		t.Fatalf("pg fork GroupID = %q, want empty", pgFork.GroupID)
	}
}

func TestPgForkExposesNamedForkFlags(t *testing.T) {
	pgFork, _, err := pgTestRootCmd.Find([]string{"pg", "fork"})
	if err != nil {
		t.Fatalf("pg fork command not found: %v", err)
	}
	for _, name := range []string{"list", "force", "start", "src-data", "src-port", "dst-data", "dst-port"} {
		if pgFork.Flags().Lookup(name) == nil {
			t.Fatalf("pg fork should expose --%s", name)
		}
	}
	if flag := pgFork.Flags().Lookup("start"); flag == nil {
		t.Fatal("pg fork should expose --start")
	} else if flag.Shorthand != "s" {
		t.Fatalf("pg fork --start shorthand = %q, want s", flag.Shorthand)
	}
	if flag := pgFork.Flags().Lookup("run"); flag == nil || !flag.Hidden {
		t.Fatal("pg fork --run should remain only as hidden compatibility alias")
	}
	for _, name := range []string{"no-start", "replace", "mode", "data", "dst", "port"} {
		if pgFork.LocalFlags().Lookup(name) != nil {
			t.Fatalf("pg fork should not expose old --%s flag", name)
		}
	}
}

func TestPgForkInitExposesCreateFlags(t *testing.T) {
	pgForkInit, _, err := pgTestRootCmd.Find([]string{"pg", "fork", "init"})
	if err != nil {
		t.Fatalf("pg fork init command not found: %v", err)
	}
	for _, name := range []string{"force", "start", "src-data", "src-port", "dst-data", "dst-port", "timeout"} {
		if pgForkInit.Flags().Lookup(name) == nil {
			t.Fatalf("pg fork init should expose --%s", name)
		}
	}
	if flag := pgForkInit.Flags().Lookup("start"); flag == nil {
		t.Fatal("pg fork init should expose --start")
	} else if flag.Shorthand != "s" {
		t.Fatalf("pg fork init --start shorthand = %q, want s", flag.Shorthand)
	}
	if flag := pgForkInit.Flags().Lookup("run"); flag == nil || !flag.Hidden {
		t.Fatal("pg fork init --run should remain only as hidden compatibility alias")
	}
	for _, name := range []string{"data", "dst", "port"} {
		if pgForkInit.LocalFlags().Lookup(name) != nil {
			t.Fatalf("pg fork init should not expose old --%s flag", name)
		}
	}
}

func TestPgForkLifecycleCommandsExposeDstEscapeHatch(t *testing.T) {
	for _, args := range [][]string{
		{"pg", "fork", "start"},
		{"pg", "fork", "stop"},
		{"pg", "fork", "rm"},
	} {
		cmd, _, err := pgTestRootCmd.Find(args)
		if err != nil {
			t.Fatalf("%v command not found: %v", args, err)
		}
		if cmd.Flags().Lookup("dst-data") == nil {
			t.Fatalf("%v should expose --dst-data for unmanaged forks", args)
		}
		if cmd.LocalFlags().Lookup("dst") != nil {
			t.Fatalf("%v should not expose old --dst flag", args)
		}
	}
}

func TestPgForkUsagePrefersPgAlias(t *testing.T) {
	for _, args := range [][]string{
		{"pg", "fork"},
		{"pg", "fork", "init"},
		{"pg", "fork", "list"},
		{"pg", "fork", "start"},
		{"pg", "fork", "stop"},
		{"pg", "fork", "rm"},
	} {
		cmd, _, err := pgTestRootCmd.Find(args)
		if err != nil {
			t.Fatalf("%v command not found: %v", args, err)
		}
		usage := cmd.UsageString()
		if strings.Contains(usage, "pig postgres fork") {
			t.Fatalf("%v usage should not mention pig postgres fork: %s", args, usage)
		}
		if !strings.Contains(usage, "pig pg fork") {
			t.Fatalf("%v usage should mention pig pg fork: %s", args, usage)
		}
	}
}

func TestPgForkExamplesPreferLongStartFlag(t *testing.T) {
	for _, args := range [][]string{
		{"pg", "fork"},
		{"pg", "fork", "init"},
	} {
		cmd, _, err := pgTestRootCmd.Find(args)
		if err != nil {
			t.Fatalf("%v command not found: %v", args, err)
		}
		usage := cmd.UsageString()
		if !strings.Contains(usage, "--start") {
			t.Fatalf("%v examples should mention --start:\n%s", args, usage)
		}
		if strings.Contains(usage, " init dev -s") {
			t.Fatalf("%v examples should not lead with -s shorthand:\n%s", args, usage)
		}
	}
}

func TestPgForkStartExposesDestinationPortOverride(t *testing.T) {
	pgForkStart, _, err := pgTestRootCmd.Find([]string{"pg", "fork", "start"})
	if err != nil {
		t.Fatalf("pg fork start command not found: %v", err)
	}
	if pgForkStart.Flags().Lookup("dst-port") == nil {
		t.Fatal("pg fork start should expose --dst-port")
	}
	if pgForkStart.LocalFlags().Lookup("port") != nil {
		t.Fatal("pg fork start should not expose old --port flag")
	}
}

func TestRunForkActionPrintsConnectionHint(t *testing.T) {
	origFormat := config.OutputFormat
	t.Cleanup(func() {
		config.OutputFormat = origFormat
	})
	config.OutputFormat = config.OUTPUT_TEXT

	out := captureStderr(t, func() {
		err := runForkAction("fork start", func() (postgrescli.ResultData, error) {
			return postgrescli.ResultData{
				Name:            "dev",
				Destination:     "/pg/data-dev",
				Started:         true,
				DestinationPort: 15432,
				ConnectCommand:  "psql -p 15432 -d postgres",
			}, nil
		})
		if err != nil {
			t.Fatalf("runForkAction returned error: %v", err)
		}
	})
	if !strings.Contains(out, "Started: dev (/pg/data-dev)") || !strings.Contains(out, "Port: 15432") || !strings.Contains(out, "Connect: psql -p 15432 -d postgres") {
		t.Fatalf("connection hint missing from stderr: %q", out)
	}
}

func TestRunForkActionPrintsStopResult(t *testing.T) {
	origFormat := config.OutputFormat
	t.Cleanup(func() {
		config.OutputFormat = origFormat
	})
	config.OutputFormat = config.OUTPUT_TEXT

	out := captureStderr(t, func() {
		err := runForkAction("fork stop", func() (postgrescli.ResultData, error) {
			return postgrescli.ResultData{
				Name:        "dev",
				Destination: "/pg/data-dev",
			}, nil
		})
		if err != nil {
			t.Fatalf("runForkAction returned error: %v", err)
		}
	})
	if !strings.Contains(out, "Stopped: dev (/pg/data-dev)") {
		t.Fatalf("stop result missing from stderr: %q", out)
	}
}

func TestRunForkTargetPlanPrintsLifecycleCommand(t *testing.T) {
	origFormat := config.OutputFormat
	t.Cleanup(func() {
		config.OutputFormat = origFormat
	})
	config.OutputFormat = config.OUTPUT_TEXT

	out := captureForkStdout(t, func() {
		err := runForkTargetPlan("rm", &forkCLIOptions{stopBefore: true}, "dev", "Remove PostgreSQL fork")
		if err != nil {
			t.Fatalf("runForkTargetPlan returned error: %v", err)
		}
	})
	if !strings.Contains(out, "Command: pig pg fork rm dev --stop") {
		t.Fatalf("plan output missing lifecycle command: %q", out)
	}
	if !strings.Contains(out, "Run pig pg fork rm dev --stop") {
		t.Fatalf("plan output missing action command: %q", out)
	}
}

func TestBuildInstanceOptionsUsesForkSourceAndDestinationFlags(t *testing.T) {
	oldPgData := pgConfig.PgData
	pgConfig.PgData = "/pg/data-parent"
	t.Cleanup(func() {
		pgConfig.PgData = oldPgData
	})

	opts := buildInstanceOptions(&forkCLIOptions{
		sourceData: "/pg/data-source",
		sourcePort: 15431,
		destData:   "/tmp/dev-fork",
		destPort:   15432,
	}, "dev")

	if opts.Instance.SourceData != "/pg/data-source" {
		t.Fatalf("SourceData = %q, want fork --src-data override", opts.Instance.SourceData)
	}
	if opts.Instance.SourcePort != 15431 {
		t.Fatalf("SourcePort = %d, want 15431", opts.Instance.SourcePort)
	}
	if opts.Instance.DestData != "/tmp/dev-fork" {
		t.Fatalf("DestData = %q, want /tmp/dev-fork", opts.Instance.DestData)
	}
	if opts.Instance.DestPort != 15432 {
		t.Fatalf("DestPort = %d, want 15432", opts.Instance.DestPort)
	}
}

func TestBuildForkTargetOptionsProgressFollowsOutputMode(t *testing.T) {
	origFormat := config.OutputFormat
	t.Cleanup(func() {
		config.OutputFormat = origFormat
	})

	config.OutputFormat = config.OUTPUT_TEXT
	textOpts := buildForkTargetOptions(&forkCLIOptions{}, "dev")
	if !textOpts.Progress {
		t.Fatal("text fork target action should keep human progress output")
	}
	if textOpts.Yes {
		t.Fatal("text fork target action should not auto-skip confirmation")
	}

	config.OutputFormat = config.OUTPUT_JSON
	structuredOpts := buildForkTargetOptions(&forkCLIOptions{}, "dev")
	if structuredOpts.Progress {
		t.Fatal("structured fork target action should suppress human progress output")
	}
	if structuredOpts.Yes {
		t.Fatal("structured fork target action should not auto-skip confirmation")
	}

	for _, cli := range []*forkCLIOptions{{yes: true}, {force: true}} {
		opts := buildForkTargetOptions(cli, "dev")
		if !opts.Yes {
			t.Fatalf("fork target action should honor explicit confirmation flags: %+v", cli)
		}
	}
}

func TestPgCloneStructuredRequiresExplicitYes(t *testing.T) {
	origFormat := config.OutputFormat
	t.Cleanup(func() {
		config.OutputFormat = origFormat
	})
	config.OutputFormat = config.OUTPUT_JSON

	opts := &postgrescli.CloneOptions{SourceDB: "template0", DestDB: "template0_copy"}
	var runErr error
	raw := capturePgStdout(t, func() {
		runErr = runClone(nil, opts)
	})
	if runErr == nil {
		t.Fatal("structured pg clone without --yes should fail")
	}
	if opts.Yes {
		t.Fatal("structured pg clone must not mutate opts.Yes")
	}
	assertPgStructuredConfirmationRequired(t, raw, "pg clone requires explicit confirmation")
}

func TestPgForkInitStructuredRequiresExplicitYesOrForce(t *testing.T) {
	origFormat := config.OutputFormat
	t.Cleanup(func() {
		config.OutputFormat = origFormat
	})
	config.OutputFormat = config.OUTPUT_JSON

	opts := &postgrescli.Options{
		Kind: postgrescli.KindInstance,
		Instance: postgrescli.InstanceOptions{
			Name: "bad name",
		},
	}
	var runErr error
	raw := capturePgStdout(t, func() {
		runErr = runFork(nil, opts)
	})
	if runErr == nil {
		t.Fatal("structured pg fork init without --yes/--force should fail")
	}
	if opts.Yes {
		t.Fatal("structured pg fork init must not mutate opts.Yes")
	}
	assertPgStructuredConfirmationRequired(t, raw, "pg fork init requires explicit confirmation")
}

func TestPgForkRemoveStructuredRequiresExplicitYesOrForce(t *testing.T) {
	origFormat := config.OutputFormat
	t.Cleanup(func() {
		config.OutputFormat = origFormat
	})
	config.OutputFormat = config.OUTPUT_JSON

	cmd := newPgForkRemoveCommand(&forkCLIOptions{})
	var runErr error
	raw := capturePgStdout(t, func() {
		runErr = cmd.RunE(cmd, []string{"bad name"})
	})
	if runErr == nil {
		t.Fatal("structured pg fork rm without --yes/--force should fail")
	}
	assertPgStructuredConfirmationRequired(t, raw, "pg fork rm requires explicit confirmation")
}

func captureForkStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create pipe failed: %v", err)
	}
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = old
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read captured stdout failed: %v", err)
	}
	return string(out)
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create pipe failed: %v", err)
	}
	os.Stderr = w
	fn()
	_ = w.Close()
	os.Stderr = old
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read captured stderr failed: %v", err)
	}
	return string(out)
}

func TestForkListStatusReflectsRuntimeState(t *testing.T) {
	tests := []struct {
		name string
		info postgrescli.ForkInfo
		want string
	}{
		{"orphan", postgrescli.ForkInfo{Orphan: true}, "orphan"},
		{"stopped fork", postgrescli.ForkInfo{Target: postgrescli.ForkEndpoint{Started: false}}, "stopped"},
		{"running fork", postgrescli.ForkInfo{Target: postgrescli.ForkEndpoint{Started: true}}, "running"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := forkListStatus(tt.info); got != tt.want {
				t.Fatalf("forkListStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestForkListDiagnosticsFormatRuntimeMetadata(t *testing.T) {
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	info := postgrescli.ForkInfo{
		CreatedAt: "2026-06-29T10:15:00Z",
		Source:    postgrescli.ForkEndpoint{Data: "/pg/data", Port: 5432},
		Target:    postgrescli.ForkEndpoint{Data: "/pg/data-dev", Port: 15432, Started: true, PID: 4242},
		Copy:      postgrescli.ForkCopyInfo{Actual: "cow"},
	}
	row := formatForkListRow(info, now)

	if row.port != "15432" {
		t.Fatalf("row.port = %q, want 15432", row.port)
	}
	if row.pid != "4242" {
		t.Fatalf("row.pid = %q, want 4242", row.pid)
	}
	if row.age != "1h" {
		t.Fatalf("row.age = %q, want 1h", row.age)
	}
	if row.source != "/pg/data:5432" {
		t.Fatalf("row.source = %q, want /pg/data:5432", row.source)
	}
	if row.copy != "cow" {
		t.Fatalf("row.copy = %q, want cow", row.copy)
	}
}

func TestForkListDiagnosticsUseDashForUnknownValues(t *testing.T) {
	info := postgrescli.ForkInfo{Orphan: true}
	row := formatForkListRow(info, time.Now())
	for name, got := range map[string]string{
		"port":   row.port,
		"pid":    row.pid,
		"age":    row.age,
		"source": row.source,
		"copy":   row.copy,
	} {
		if got != "-" {
			t.Fatalf("%s = %q, want -", name, got)
		}
	}
}

func TestForkErrorResultPreservesForkErrorCode(t *testing.T) {
	result := forkErrorResult(&postgrescli.ForkError{
		Code: output.CodeForkInvalidArgs,
		Err:  fmt.Errorf("unsafe destination data directory: /"),
	})
	if result.Success {
		t.Fatal("fork error result should be unsuccessful")
	}
	if result.Code != output.CodeForkInvalidArgs {
		t.Fatalf("result code = %d, want %d", result.Code, output.CodeForkInvalidArgs)
	}
	if result.Message != "unsafe destination data directory: /" {
		t.Fatalf("result message = %q", result.Message)
	}
}

func TestCommonLogCommandsExposeConsistentSnapshotAndTailAPI(t *testing.T) {
	tests := []struct {
		name string
		cmd  *cobra.Command
	}{
		{name: "pg", cmd: pgLogCmd},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.cmd.RunE == nil {
				t.Fatalf("%s log should support a default recent-log action", tt.name)
			}
			if flag := lookupLocalOrPersistentFlag(tt.cmd, "lines"); flag == nil || flag.Shorthand != "n" {
				t.Fatalf("%s log should expose -n/--lines on the parent command", tt.name)
			}
			if flag := tt.cmd.Flags().Lookup("follow"); flag == nil || flag.Shorthand != "f" {
				t.Fatalf("%s log should expose -f/--follow on the parent command", tt.name)
			}
			for _, sub := range []string{"show", "tail"} {
				if found, _, err := tt.cmd.Find([]string{sub}); err != nil || found == tt.cmd {
					t.Fatalf("%s log should expose %q subcommand, found=%v err=%v", tt.name, sub, found, err)
				}
			}
			if found, _, err := tt.cmd.Find([]string{"cat"}); err != nil || found == tt.cmd {
				t.Fatalf("%s log should keep cat as a compatibility alias, found=%v err=%v", tt.name, found, err)
			}
		})
	}
}

func TestPgLogGrepNoMatchSilencesCobraError(t *testing.T) {
	origFormat := config.OutputFormat
	origLogDir := pgConfig.LogDir
	origSilenceErrors := pgLogGrepCmd.SilenceErrors
	origSilenceUsage := pgLogGrepCmd.SilenceUsage
	origIgnoreCase := pgLogGrepIgnoreCase
	origContext := pgLogGrepContext
	defer func() {
		config.OutputFormat = origFormat
		pgConfig.LogDir = origLogDir
		pgLogGrepCmd.SilenceErrors = origSilenceErrors
		pgLogGrepCmd.SilenceUsage = origSilenceUsage
		pgLogGrepIgnoreCase = origIgnoreCase
		pgLogGrepContext = origContext
	}()

	dir := t.TempDir()
	logPath := filepath.Join(dir, "postgresql-2026-07-02.csv")
	if err := os.WriteFile(logPath, []byte("LOG,startup complete\n"), 0644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	config.OutputFormat = config.OUTPUT_TEXT
	pgConfig.LogDir = dir
	pgLogGrepCmd.SilenceErrors = false
	pgLogGrepCmd.SilenceUsage = false
	pgLogGrepIgnoreCase = false
	pgLogGrepContext = 0

	err := pgLogGrepCmd.RunE(pgLogGrepCmd, []string{"ERROR"})
	var exitErr *utils.ExitCodeError
	if !errors.As(err, &exitErr) {
		t.Fatalf("pg log grep returned %T, want ExitCodeError", err)
	}
	if exitErr.Code != 1 || !exitErr.Silent {
		t.Fatalf("pg log grep no-match exit = code %d silent %v, want code 1 silent true", exitErr.Code, exitErr.Silent)
	}
	if !pgLogGrepCmd.SilenceErrors {
		t.Fatal("pg log grep no-match should silence Cobra error printing")
	}
	if !pgLogGrepCmd.SilenceUsage {
		t.Fatal("pg log grep no-match should silence Cobra usage printing")
	}

	if err := pgLogGrepCmd.Args(pgLogGrepCmd, nil); err == nil {
		t.Fatal("pg log grep without pattern should still reject arguments")
	}
	if pgLogGrepCmd.SilenceErrors || pgLogGrepCmd.SilenceUsage {
		t.Fatal("pg log grep argument validation should reset silent no-match flags")
	}
}

func TestJSONLogOutputOnlyAcceptsPlainJSON(t *testing.T) {
	origFormat := config.OutputFormat
	defer func() { config.OutputFormat = origFormat }()

	config.OutputFormat = config.OUTPUT_JSON
	if !isJSONLogOutput() {
		t.Fatal("plain json should enable log JSONL output")
	}

	config.OutputFormat = config.OUTPUT_JSON_PRETTY
	if isJSONLogOutput() {
		t.Fatal("json-pretty should not claim JSONL log output")
	}

	config.OutputFormat = config.OUTPUT_YAML
	if isJSONLogOutput() {
		t.Fatal("yaml should not claim JSONL log output")
	}
}

func TestValidateLogLinesRejectsNonPositiveValues(t *testing.T) {
	for _, n := range []int{0, -1} {
		if err := validateLogLines(n); err == nil || !strings.Contains(err.Error(), "lines must be positive") {
			t.Fatalf("validateLogLines(%d) = %v, want positive line count error", n, err)
		}
	}
	if err := validateLogLines(1); err != nil {
		t.Fatalf("validateLogLines(1) returned error: %v", err)
	}
}

func TestRejectUnsupportedLogOutputFormats(t *testing.T) {
	origFormat := config.OutputFormat
	defer func() { config.OutputFormat = origFormat }()

	for _, format := range []string{config.OUTPUT_YAML, config.OUTPUT_JSON_PRETTY} {
		config.OutputFormat = format
		if err := rejectUnsupportedLogOutputFormat("pig pg log show"); err == nil || !strings.Contains(err.Error(), "-o json") {
			t.Fatalf("rejectUnsupportedLogOutputFormat(%q) = %v, want -o json guidance", format, err)
		}
	}

	for _, format := range []string{config.OUTPUT_TEXT, config.OUTPUT_JSON} {
		config.OutputFormat = format
		if err := rejectUnsupportedLogOutputFormat("pig pg log show"); err != nil {
			t.Fatalf("rejectUnsupportedLogOutputFormat(%q) returned error: %v", format, err)
		}
	}
}

func lookupLocalOrPersistentFlag(cmd *cobra.Command, name string) *pflag.Flag {
	if flag := cmd.Flags().Lookup(name); flag != nil {
		return flag
	}
	return cmd.PersistentFlags().Lookup(name)
}

func TestPgKillPlanJSONContainsPrimitiveContract(t *testing.T) {
	origFormat := config.OutputFormat
	origPlan := pgKillPlan
	origExecute := pgKillExecute
	origUser := pgKillUser
	defer func() {
		config.OutputFormat = origFormat
		pgKillPlan = origPlan
		pgKillExecute = origExecute
		pgKillUser = origUser
	}()

	config.OutputFormat = config.OUTPUT_JSON
	pgKillPlan = true
	pgKillExecute = true
	pgKillUser = "app"

	raw := capturePgStdout(t, func() {
		if err := pgKillCmd.RunE(pgKillCmd, nil); err != nil {
			t.Fatalf("pg kill --plan should not execute or fail: %v", err)
		}
	})

	var plan output.Plan
	if err := json.Unmarshal(pgBytesTrimSpace([]byte(raw)), &plan); err != nil {
		t.Fatalf("invalid plan json: %v raw=%q", err, raw)
	}
	if plan.Boundary != "pg:local-instance" {
		t.Fatalf("boundary = %q, want pg:local-instance", plan.Boundary)
	}
	if plan.Confirmation != "recommended" {
		t.Fatalf("confirmation = %q, want recommended", plan.Confirmation)
	}
	if len(plan.Preconditions) == 0 || !strings.Contains(plan.Preconditions[0].Detail, "app") {
		t.Fatalf("expected filter precondition, got %+v", plan.Preconditions)
	}
}

func TestPgVacuumFullStructuredExecutionRequiresExplicitYes(t *testing.T) {
	origFormat := config.OutputFormat
	origFull := pgMaintFull
	origPlan := pgMaintPlan
	origYes := pgMaintYes
	defer func() {
		config.OutputFormat = origFormat
		pgMaintFull = origFull
		pgMaintPlan = origPlan
		pgMaintYes = origYes
	}()

	config.OutputFormat = config.OUTPUT_JSON
	pgMaintFull = true
	pgMaintPlan = false
	pgMaintYes = false

	var runErr error
	raw := capturePgStdout(t, func() {
		runErr = pgVacuumCmd.RunE(pgVacuumCmd, []string{"app"})
	})
	if runErr == nil {
		t.Fatal("structured VACUUM FULL should require explicit --yes")
	}
	var exitErr *utils.ExitCodeError
	if !errors.As(runErr, &exitErr) {
		t.Fatalf("expected ExitCodeError, got %T: %v", runErr, runErr)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(pgBytesTrimSpace([]byte(raw)), &payload); err != nil {
		t.Fatalf("invalid json output: %v raw=%q", err, raw)
	}
	if success, _ := payload["success"].(bool); success {
		t.Fatalf("expected success=false, got %v", payload)
	}
	if !pgResultDataHasNextAction(payload, "--yes") {
		t.Fatalf("expected envelope next action mentioning --yes, got %v", payload)
	}
}

func TestPgInitForceTextRequiresConfirmationBeforeExecution(t *testing.T) {
	origFormat := config.OutputFormat
	origForce := pgInitForce
	origYes := pgInitYes
	origConfirm := highRiskTextConfirm
	origExec := pgInitCommandExec
	origInitialized := pgInitTargetInitialized
	defer func() {
		config.OutputFormat = origFormat
		pgInitForce = origForce
		pgInitYes = origYes
		highRiskTextConfirm = origConfirm
		pgInitCommandExec = origExec
		pgInitTargetInitialized = origInitialized
	}()

	config.OutputFormat = config.OUTPUT_TEXT
	pgInitForce = true
	pgInitYes = false
	pgInitTargetInitialized = func(*postgrescli.Config) bool { return true }
	confirmErr := fmt.Errorf("confirmation cancelled")
	confirmed := false
	executed := false
	highRiskTextConfirm = func(warning, action string) error {
		confirmed = true
		if !strings.Contains(warning, "overwrite") || !strings.Contains(action, "init") {
			t.Fatalf("unexpected init confirmation warning/action: %q / %q", warning, action)
		}
		return confirmErr
	}
	pgInitCommandExec = func(*postgrescli.Config, *postgrescli.InitOptions) error {
		executed = true
		return nil
	}

	err := pgInitCmd.RunE(pgInitCmd, nil)
	if !errors.Is(err, confirmErr) {
		t.Fatalf("pg init -f error = %v, want confirmation error", err)
	}
	if !confirmed {
		t.Fatal("pg init -f should request text confirmation")
	}
	if executed {
		t.Fatal("pg init -f should not execute after confirmation cancellation")
	}
}

// TestPgInitForceUninitializedDirNeedsNoConfirmation guards B21: the T2 gate
// fires only when --force would overwrite an INITIALIZED data directory;
// wiping nothing must not prompt (principle: simple ops stay simple).
func TestPgInitForceUninitializedDirNeedsNoConfirmation(t *testing.T) {
	origFormat := config.OutputFormat
	origForce := pgInitForce
	origYes := pgInitYes
	origConfirm := highRiskTextConfirm
	origExec := pgInitCommandExec
	origInitialized := pgInitTargetInitialized
	defer func() {
		config.OutputFormat = origFormat
		pgInitForce = origForce
		pgInitYes = origYes
		highRiskTextConfirm = origConfirm
		pgInitCommandExec = origExec
		pgInitTargetInitialized = origInitialized
	}()

	config.OutputFormat = config.OUTPUT_TEXT
	pgInitForce = true
	pgInitYes = false
	pgInitTargetInitialized = func(*postgrescli.Config) bool { return false }
	confirmed := false
	highRiskTextConfirm = func(warning, action string) error {
		confirmed = true
		return nil
	}
	executed := false
	pgInitCommandExec = func(*postgrescli.Config, *postgrescli.InitOptions) error {
		executed = true
		return nil
	}

	if err := pgInitCmd.RunE(pgInitCmd, nil); err != nil {
		t.Fatalf("pg init -f on uninitialized dir should run without confirmation: %v", err)
	}
	if confirmed {
		t.Fatal("pg init -f on uninitialized dir must not prompt")
	}
	if !executed {
		t.Fatal("pg init -f on uninitialized dir should execute")
	}
}

func TestPgInitForceStructuredRequiresExplicitYes(t *testing.T) {
	origFormat := config.OutputFormat
	origForce := pgInitForce
	origYes := pgInitYes
	origExec := pgInitCommandExec
	origInitialized := pgInitTargetInitialized
	defer func() {
		config.OutputFormat = origFormat
		pgInitForce = origForce
		pgInitYes = origYes
		pgInitCommandExec = origExec
		pgInitTargetInitialized = origInitialized
	}()

	config.OutputFormat = config.OUTPUT_JSON
	pgInitForce = true
	pgInitYes = false
	pgInitTargetInitialized = func(*postgrescli.Config) bool { return true }
	executed := false
	pgInitCommandExec = func(*postgrescli.Config, *postgrescli.InitOptions) error {
		executed = true
		return nil
	}

	var runErr error
	raw := capturePgStdout(t, func() {
		runErr = pgInitCmd.RunE(pgInitCmd, nil)
	})
	if runErr == nil {
		t.Fatal("structured pg init -f should require explicit --yes")
	}
	if executed {
		t.Fatal("structured pg init -f should not execute without --yes")
	}
	assertPgStructuredConfirmationRequired(t, raw, "pg init --force requires explicit confirmation")
}

func TestPgPromoteTextRequiresConfirmationBeforeExecution(t *testing.T) {
	origFormat := config.OutputFormat
	origYes := pgPromoteYes
	origPlan := pgPromotePlan
	origConfirm := highRiskTextConfirm
	origExec := pgPromoteCommandExec
	defer func() {
		config.OutputFormat = origFormat
		pgPromoteYes = origYes
		pgPromotePlan = origPlan
		highRiskTextConfirm = origConfirm
		pgPromoteCommandExec = origExec
	}()

	config.OutputFormat = config.OUTPUT_TEXT
	pgPromoteYes = false
	pgPromotePlan = false
	confirmErr := fmt.Errorf("confirmation cancelled")
	executed := false
	highRiskTextConfirm = func(warning, action string) error {
		if !strings.Contains(warning, "promote") || !strings.Contains(action, "promote") {
			t.Fatalf("unexpected promote confirmation warning/action: %q / %q", warning, action)
		}
		return confirmErr
	}
	pgPromoteCommandExec = func(*postgrescli.Config, *postgrescli.PromoteOptions) error {
		executed = true
		return nil
	}

	err := pgPromoteCmd.RunE(pgPromoteCmd, nil)
	if !errors.Is(err, confirmErr) {
		t.Fatalf("pg promote error = %v, want confirmation error", err)
	}
	if executed {
		t.Fatal("pg promote should not execute after confirmation cancellation")
	}
}

func TestPgVacuumFullTextRequiresConfirmationBeforeExecution(t *testing.T) {
	origFormat := config.OutputFormat
	origFull := pgMaintFull
	origPlan := pgMaintPlan
	origYes := pgMaintYes
	origConfirm := highRiskTextConfirm
	origExec := pgVacuumCommandExec
	defer func() {
		config.OutputFormat = origFormat
		pgMaintFull = origFull
		pgMaintPlan = origPlan
		pgMaintYes = origYes
		highRiskTextConfirm = origConfirm
		pgVacuumCommandExec = origExec
	}()

	config.OutputFormat = config.OUTPUT_TEXT
	pgMaintFull = true
	pgMaintPlan = false
	pgMaintYes = false
	confirmErr := fmt.Errorf("confirmation cancelled")
	executed := false
	highRiskTextConfirm = func(warning, action string) error {
		if !strings.Contains(warning, "VACUUM FULL") || !strings.Contains(action, "vacuum") {
			t.Fatalf("unexpected vacuum confirmation warning/action: %q / %q", warning, action)
		}
		return confirmErr
	}
	pgVacuumCommandExec = func(*postgrescli.Config, string, *postgrescli.VacuumOptions) error {
		executed = true
		return nil
	}

	err := pgVacuumCmd.RunE(pgVacuumCmd, []string{"app"})
	if !errors.Is(err, confirmErr) {
		t.Fatalf("pg vacuum --full error = %v, want confirmation error", err)
	}
	if executed {
		t.Fatal("pg vacuum --full should not execute after confirmation cancellation")
	}
}

func capturePgStdout(t *testing.T, fn func()) string {
	t.Helper()
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe error: %v", err)
	}
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = origStdout
	raw, _ := io.ReadAll(r)
	_ = r.Close()
	return string(raw)
}

func assertPgStructuredConfirmationRequired(t *testing.T, raw string, wantMessage string) {
	t.Helper()
	var payload map[string]interface{}
	if err := json.Unmarshal(pgBytesTrimSpace([]byte(raw)), &payload); err != nil {
		t.Fatalf("invalid json output: %v raw=%q", err, raw)
	}
	if success, _ := payload["success"].(bool); success {
		t.Fatalf("expected success=false, got %v", payload)
	}
	if msg, _ := payload["message"].(string); msg != wantMessage {
		t.Fatalf("message = %q, want %q", msg, wantMessage)
	}
	detail, _ := payload["detail"].(string)
	if !strings.Contains(detail, "structured output mode does not prompt interactively") {
		t.Fatalf("detail should explain structured confirmation, got %q", detail)
	}
	if !pgResultDataHasNextAction(payload, "--yes") {
		t.Fatalf("expected envelope next action mentioning --yes, got %v", payload)
	}
}

func pgBytesTrimSpace(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}

func pgResultDataHasNextAction(data map[string]interface{}, needle string) bool {
	items, _ := data["next_actions"].([]interface{})
	for _, item := range items {
		m, _ := item.(map[string]interface{})
		if strings.Contains(pgAsString(m["command"]), needle) || strings.Contains(pgAsString(m["reason"]), needle) {
			return true
		}
	}
	return false
}

func pgAsString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

var pgTestRootCmd = newPgTestRootCommand()

func newPgTestRootCommand() *cobra.Command {
	root := &cobra.Command{Use: "pig"}
	root.AddCommand(pgCmd)
	return root
}
