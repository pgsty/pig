package patroni

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

// argsHas reports whether `want` appears at args[i] for any i. argsHasInOrder
// is the stricter variant: returns true only if every want appears in args
// in the given order (possibly non-contiguous).
func argsHas(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func withPatroniStdout(t *testing.T, w io.Writer, fn func()) {
	t.Helper()
	old := os.Stdout
	r, pipeW, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = pipeW
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(w, r)
		close(done)
	}()

	fn()

	_ = pipeW.Close()
	os.Stdout = old
	<-done
	_ = r.Close()
}

func argsHasInOrder(args []string, wants ...string) bool {
	i := 0
	for _, a := range args {
		if i < len(wants) && a == wants[i] {
			i++
		}
	}
	return i == len(wants)
}

func TestLogRejectsNonPositiveLines(t *testing.T) {
	for _, n := range []int{0, -1} {
		if err := Log(false, n); err == nil || !strings.Contains(err.Error(), "lines must be positive") {
			t.Fatalf("Log(false, %d) = %v, want positive line count error", n, err)
		}
		if err := LogJSONL(n); err == nil || !strings.Contains(err.Error(), "lines must be positive") {
			t.Fatalf("LogJSONL(%d) = %v, want positive line count error", n, err)
		}
	}
}

func TestLogJSONLUsesSudoJournalctlRowsAndPrintsJSONL(t *testing.T) {
	origRead := patroniReadJournalctlLines
	defer func() { patroniReadJournalctlLines = origRead }()

	gotLimit := 0
	patroniReadJournalctlLines = func(limit int) ([]string, int, error) {
		gotLimit = limit
		return []string{"patroni started", "leader lock acquired"}, 2, nil
	}

	var out bytes.Buffer
	withPatroniStdout(t, &out, func() {
		if err := LogJSONL(2); err != nil {
			t.Fatalf("LogJSONL returned error: %v", err)
		}
	})

	if gotLimit != 2 {
		t.Fatalf("journalctl line limit = %d, want 2", gotLimit)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected two JSONL lines, got %d: %q", len(lines), out.String())
	}
	var row map[string]string
	if err := json.Unmarshal([]byte(lines[0]), &row); err != nil {
		t.Fatalf("invalid JSONL row: %v", err)
	}
	if row["component"] != "patroni" || row["message"] != "patroni started" {
		t.Fatalf("unexpected JSONL row: %v", row)
	}
}

func TestLogJSONLSkipsNoEntriesSentinel(t *testing.T) {
	origRead := patroniReadJournalctlLines
	defer func() { patroniReadJournalctlLines = origRead }()

	patroniReadJournalctlLines = func(limit int) ([]string, int, error) {
		return []string{"-- No entries --", "patroni started"}, 2, nil
	}

	var out bytes.Buffer
	withPatroniStdout(t, &out, func() {
		if err := LogJSONL(2); err != nil {
			t.Fatalf("LogJSONL returned error: %v", err)
		}
	})

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected one JSONL line after skipping sentinel, got %d: %q", len(lines), out.String())
	}
	var row map[string]string
	if err := json.Unmarshal([]byte(lines[0]), &row); err != nil {
		t.Fatalf("invalid JSONL row: %v", err)
	}
	if row["message"] != "patroni started" {
		t.Fatalf("unexpected JSONL message: %v", row)
	}
}

func TestBuildRestartArgs(t *testing.T) {
	const cluster = "pg-nms"

	tests := []struct {
		name        string
		opts        *RestartOptions
		wantPrefix  []string // first N args, in order
		wantInOrder []string // must appear in args in this order
		notWant     []string // must NOT appear anywhere
	}{
		{
			name:       "nil opts → just restart + cluster",
			opts:       nil,
			wantPrefix: []string{"restart", cluster},
			notWant:    []string{"--force", "--pending", "--role"},
		},
		{
			name:       "pending + force, no member",
			opts:       &RestartOptions{Pending: true, Force: true},
			wantPrefix: []string{"restart", cluster},
			wantInOrder: []string{
				"restart", cluster, "--force", "--pending",
			},
			notWant: []string{"--role"},
		},
		{
			name:       "specific member + force",
			opts:       &RestartOptions{Member: "pg-nms-1", Force: true},
			wantPrefix: []string{"restart", cluster},
			wantInOrder: []string{
				"restart", cluster, "pg-nms-1", "--force",
			},
		},
		{
			name:       "role filter",
			opts:       &RestartOptions{Role: "replica", Force: true},
			wantPrefix: []string{"restart", cluster},
			wantInOrder: []string{
				"restart", cluster, "--role", "replica", "--force",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildRestartArgs(cluster, tt.opts)

			for i, w := range tt.wantPrefix {
				if i >= len(got) || got[i] != w {
					t.Errorf("prefix mismatch at %d: want %q, got args=%v", i, w, got)
				}
			}
			if len(tt.wantInOrder) > 0 && !argsHasInOrder(got, tt.wantInOrder...) {
				t.Errorf("want subsequence %v in args, got %v", tt.wantInOrder, got)
			}
			for _, n := range tt.notWant {
				if argsHas(got, n) {
					t.Errorf("did not want %q in args, got %v", n, got)
				}
			}
		})
	}
}

func TestBuildReloadArgs(t *testing.T) {
	const cluster = "pg-nms"

	// --force is mandatory (B04): patronictl reload prompts without it.
	got := buildReloadArgs(cluster)
	if len(got) != 3 || got[0] != "reload" || got[1] != cluster || got[2] != "--force" {
		t.Errorf("want [reload %s --force], got %v", cluster, got)
	}
}

func TestBuildReinitArgs(t *testing.T) {
	const cluster = "pg-nms"

	got := buildReinitArgs(cluster, &ReinitOptions{Member: "pg-nms-2", Force: true, Wait: true})
	if !argsHasInOrder(got, "reinit", cluster, "pg-nms-2", "--force", "--wait") {
		t.Errorf("want reinit %s pg-nms-2 --force --wait in order, got %v", cluster, got)
	}

	got = buildReinitArgs(cluster, nil)
	if len(got) != 2 || got[0] != "reinit" || got[1] != cluster {
		t.Errorf("nil opts: want [reinit %s], got %v", cluster, got)
	}
}

func TestBuildSwitchoverArgs(t *testing.T) {
	const cluster = "pg-nms"

	got := buildSwitchoverArgs(cluster, &SwitchoverOptions{
		Leader:    "pg-nms-1",
		Candidate: "pg-nms-2",
		Force:     true,
		Scheduled: "2026-05-13T16:30:00",
	})
	if !argsHasInOrder(got, "switchover", cluster, "--leader", "pg-nms-1", "--candidate", "pg-nms-2") {
		t.Errorf("want switchover %s --leader pg-nms-1 --candidate pg-nms-2 in order, got %v", cluster, got)
	}
	if !argsHas(got, "--force") || !argsHas(got, "--scheduled") {
		t.Errorf("want --force and --scheduled in args, got %v", got)
	}

	got = buildSwitchoverArgs(cluster, nil)
	if len(got) != 2 || got[0] != "switchover" || got[1] != cluster {
		t.Errorf("nil opts: want [switchover %s], got %v", cluster, got)
	}
}

func TestBuildFailoverArgs(t *testing.T) {
	const cluster = "pg-nms"

	got := buildFailoverArgs(cluster, &FailoverOptions{Candidate: "pg-nms-2", Force: true})
	if !argsHasInOrder(got, "failover", cluster, "--candidate", "pg-nms-2", "--force") {
		t.Errorf("want failover %s --candidate pg-nms-2 --force in order, got %v", cluster, got)
	}

	got = buildFailoverArgs(cluster, nil)
	if len(got) != 2 || got[0] != "failover" || got[1] != cluster {
		t.Errorf("nil opts: want [failover %s], got %v", cluster, got)
	}
}

// TestPatronictlPositionalContract documents the constraint that motivated the
// CLUSTER_NAME prepend across Reload / Restart / Reinit / Switchover / Failover.
// Unlike pause / resume / list, these patronictl subcommands require
// CLUSTER_NAME as the first positional argument; `-c <config>` does NOT supply
// scope to them. If a future refactor drops the prepend, this test fails fast.
func TestPatronictlPositionalContract(t *testing.T) {
	const cluster = "scope-name"

	for name, args := range map[string][]string{
		"reload":     buildReloadArgs(cluster),
		"restart":    buildRestartArgs(cluster, nil),
		"reinit":     buildReinitArgs(cluster, nil),
		"switchover": buildSwitchoverArgs(cluster, nil),
		"failover":   buildFailoverArgs(cluster, nil),
	} {
		if len(args) < 2 || args[1] != cluster {
			t.Errorf("%s: cluster name must appear at args[1], got %v", name, args)
		}
	}
}

func TestPatroniOperationWrappersUseResolvedClusterName(t *testing.T) {
	patroniTestDepsMu.Lock()
	oldGetClusterName := patroniGetClusterName
	oldRunPatronictl := patroniRunPatronictl
	t.Cleanup(func() {
		patroniGetClusterName = oldGetClusterName
		patroniRunPatronictl = oldRunPatronictl
		patroniTestDepsMu.Unlock()
	})

	var gotResolveDBSU string
	patroniGetClusterName = func(dbsu string) (string, error) {
		gotResolveDBSU = dbsu
		return "pg-nms", nil
	}

	tests := []struct {
		name string
		run  func() error
		want []string
	}{
		{
			name: "reload",
			run:  func() error { return Reload("postgres") },
			want: []string{"reload", "pg-nms"},
		},
		{
			name: "restart",
			run:  func() error { return Restart("postgres", &RestartOptions{Member: "pg-nms-1", Force: true}) },
			want: []string{"restart", "pg-nms", "pg-nms-1", "--force"},
		},
		{
			name: "reinit",
			run:  func() error { return Reinit("postgres", &ReinitOptions{Member: "pg-nms-2", Force: true}) },
			want: []string{"reinit", "pg-nms", "pg-nms-2", "--force"},
		},
		{
			name: "switchover",
			run:  func() error { return Switchover("postgres", &SwitchoverOptions{Candidate: "pg-nms-2", Force: true}) },
			want: []string{"switchover", "pg-nms", "--candidate", "pg-nms-2", "--force"},
		},
		{
			name: "failover",
			run:  func() error { return Failover("postgres", &FailoverOptions{Candidate: "pg-nms-2", Force: true}) },
			want: []string{"failover", "pg-nms", "--candidate", "pg-nms-2", "--force"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			var gotRunDBSU string
			gotResolveDBSU = ""
			patroniRunPatronictl = func(dbsu string, args []string) error {
				gotRunDBSU = dbsu
				got = append([]string(nil), args...)
				return nil
			}
			if err := tt.run(); err != nil {
				t.Fatalf("%s returned error: %v", tt.name, err)
			}
			if gotResolveDBSU != "postgres" {
				t.Fatalf("%s resolved cluster with dbsu=%q, want postgres", tt.name, gotResolveDBSU)
			}
			if gotRunDBSU != "postgres" {
				t.Fatalf("%s ran patronictl with dbsu=%q, want postgres", tt.name, gotRunDBSU)
			}
			if !argsHasInOrder(got, tt.want...) {
				t.Fatalf("%s args = %v, want subsequence %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestListWrapperUsesOptionalCluster(t *testing.T) {
	patroniTestDepsMu.Lock()
	oldRunPatronictl := patroniRunPatronictl
	t.Cleanup(func() {
		patroniRunPatronictl = oldRunPatronictl
		patroniTestDepsMu.Unlock()
	})

	var captured []string
	patroniRunPatronictl = func(dbsu string, args []string) error {
		captured = append([]string(nil), args...)
		return nil
	}

	if err := List("postgres", "pg-meta", true, 3); err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if !argsHasInOrder(captured, "list", "pg-meta", "-e", "-t", "-W", "-w", "3") {
		t.Fatalf("captured args = %v, want list pg-meta -e -t -W -w 3", captured)
	}
}

func TestGetClusterNameUsesDBSUFallbackHook(t *testing.T) {
	patroniTestDepsMu.Lock()
	oldReadFile := patroniReadFile
	oldDBSUCommandOutput := patroniDBSUCommandOutput
	t.Cleanup(func() {
		patroniReadFile = oldReadFile
		patroniDBSUCommandOutput = oldDBSUCommandOutput
		patroniTestDepsMu.Unlock()
	})

	var gotDBSU string
	var gotArgs []string
	patroniReadFile = func(name string) ([]byte, error) {
		return nil, errors.New("permission denied")
	}
	patroniDBSUCommandOutput = func(dbsu string, args []string) (string, error) {
		gotDBSU = dbsu
		gotArgs = append([]string(nil), args...)
		return "scope: pg-fallback\n", nil
	}

	cluster, err := GetClusterName("dba")
	if err != nil {
		t.Fatalf("GetClusterName returned error: %v", err)
	}
	if cluster != "pg-fallback" {
		t.Fatalf("cluster = %q, want pg-fallback", cluster)
	}
	if gotDBSU != "dba" {
		t.Fatalf("DBSU fallback dbsu = %q, want dba", gotDBSU)
	}
	if !argsHasInOrder(gotArgs, "cat", DefaultConfigPath) {
		t.Fatalf("DBSU fallback args = %v, want cat %s", gotArgs, DefaultConfigPath)
	}
}

func TestBuildSwitchoverPlan(t *testing.T) {
	opts := &SwitchoverOptions{
		Leader:    "pg-1",
		Candidate: "pg-2",
		Scheduled: "2026-02-03T12:00:00",
	}

	plan := BuildSwitchoverPlan(opts)
	if plan == nil {
		t.Fatal("BuildSwitchoverPlan returned nil")
	}
	if !strings.Contains(plan.Command, "switchover") {
		t.Errorf("plan.Command missing switchover: %q", plan.Command)
	}
	if !strings.Contains(plan.Command, "--candidate") {
		t.Errorf("plan.Command missing candidate: %q", plan.Command)
	}

	if len(plan.Actions) == 0 {
		t.Error("plan.Actions should not be empty")
	}
	if len(plan.Affects) == 0 {
		t.Error("plan.Affects should not be empty")
	}
	if plan.Expected == "" {
		t.Error("plan.Expected should not be empty")
	}
	if len(plan.Risks) == 0 {
		t.Error("plan.Risks should not be empty")
	}
}

func TestBuildSwitchoverPlanNilOpts(t *testing.T) {
	plan := BuildSwitchoverPlan(nil)
	if plan == nil {
		t.Fatal("BuildSwitchoverPlan(nil) should not return nil")
	}
	if !strings.Contains(plan.Command, "switchover") {
		t.Errorf("plan.Command missing switchover: %q", plan.Command)
	}
	if len(plan.Actions) == 0 {
		t.Error("plan.Actions should not be empty even with nil opts")
	}
	if plan.Expected == "" {
		t.Error("plan.Expected should not be empty")
	}
}

func TestBuildSwitchoverPlanEmptyOpts(t *testing.T) {
	opts := &SwitchoverOptions{}
	plan := BuildSwitchoverPlan(opts)
	if plan == nil {
		t.Fatal("BuildSwitchoverPlan returned nil")
	}
	// Should not include optional flags when not set
	if strings.Contains(plan.Command, "--leader") {
		t.Errorf("plan.Command should not include --leader when empty: %q", plan.Command)
	}
	if strings.Contains(plan.Command, "--candidate") {
		t.Errorf("plan.Command should not include --candidate when empty: %q", plan.Command)
	}
}

// TestBuildSwitchoverPlanCommandForms: plan.Command is the replayable preview
// form (--plan), the first next action is the --yes execute form.
func TestBuildSwitchoverPlanCommandForms(t *testing.T) {
	plan := BuildSwitchoverPlan(&SwitchoverOptions{Candidate: "pg-2", Force: true})
	if plan == nil {
		t.Fatal("BuildSwitchoverPlan returned nil")
	}
	if !strings.HasSuffix(plan.Command, "--plan") || strings.Contains(plan.Command, "--yes") {
		t.Errorf("plan.Command should be the --plan preview form: %q", plan.Command)
	}
	if len(plan.NextActions) == 0 {
		t.Fatal("plan should carry next actions")
	}
	execute := plan.NextActions[0]
	if !execute.Required || !strings.Contains(execute.Command, "--yes") || strings.Contains(execute.Command, "--plan") {
		t.Errorf("first next action should be the required --yes execute form: %+v", execute)
	}
}

func TestSwitchoverCommand(t *testing.T) {
	if got := SwitchoverCommand(nil, false, false); got != "pig pt switchover" {
		t.Errorf("nil opts = %q, want %q", got, "pig pt switchover")
	}

	opts := &SwitchoverOptions{Leader: "pg-1", Candidate: "pg-2", Scheduled: "2026-02-03T12:00:00"}
	got := SwitchoverCommand(opts, false, true)
	want := "pig pt switchover --leader pg-1 --candidate pg-2 --scheduled 2026-02-03T12:00:00 --yes"
	if got != want {
		t.Errorf("execute form = %q, want %q", got, want)
	}
	if got := SwitchoverCommand(opts, true, false); !strings.HasSuffix(got, "--plan") || strings.Contains(got, "--yes") {
		t.Errorf("plan form should end with --plan and omit --yes: %q", got)
	}

	// Force never leaks into rendered commands; --yes is an explicit marker.
	if got := SwitchoverCommand(&SwitchoverOptions{Force: true}, false, false); strings.Contains(got, "--yes") || strings.Contains(got, "--force") {
		t.Errorf("Force option must not leak into command: %q", got)
	}

	// Values with spaces stay shell-safe (copy-paste replayable).
	quoted := SwitchoverCommand(&SwitchoverOptions{Scheduled: "2026-02-03 12:00:00"}, false, true)
	if !strings.Contains(quoted, "'2026-02-03 12:00:00'") {
		t.Errorf("scheduled value with spaces should be quoted: %q", quoted)
	}
}

func TestRestartCommand(t *testing.T) {
	if got := RestartCommand(nil, false, false); got != "pig pt restart" {
		t.Errorf("nil opts = %q, want %q", got, "pig pt restart")
	}
	opts := &RestartOptions{Member: "pg-1", Role: "replica", Pending: true, Force: true}
	if got, want := RestartCommand(opts, false, true), "pig pt restart pg-1 --role replica --pending --yes"; got != want {
		t.Errorf("execute form = %q, want %q", got, want)
	}
	if got, want := RestartCommand(&RestartOptions{}, true, false), "pig pt restart --plan"; got != want {
		t.Errorf("plan form = %q, want %q", got, want)
	}
}

func TestReinitCommand(t *testing.T) {
	opts := &ReinitOptions{Member: "pg-2", Wait: true, Force: true}
	if got, want := ReinitCommand(opts, false, true), "pig pt reinit pg-2 --wait --yes"; got != want {
		t.Errorf("execute form = %q, want %q", got, want)
	}
	if got, want := ReinitCommand(opts, true, false), "pig pt reinit pg-2 --wait --plan"; got != want {
		t.Errorf("plan form = %q, want %q", got, want)
	}
}

// TestBuildRestartPlanConfirmationTiers mirrors the D2 conditional tier: an
// unscoped rolling restart needs consent, a pinned member or pending apply
// executes directly.
func TestBuildRestartPlanConfirmationTiers(t *testing.T) {
	tests := []struct {
		name    string
		opts    *RestartOptions
		confirm string
	}{
		{name: "cluster-wide", opts: nil, confirm: "required"},
		{name: "role filtered", opts: &RestartOptions{Role: "replica"}, confirm: "required"},
		{name: "explicit member", opts: &RestartOptions{Member: "pg-1"}, confirm: "none"},
		{name: "pending apply", opts: &RestartOptions{Pending: true}, confirm: "none"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := BuildRestartPlan(tt.opts)
			if plan == nil {
				t.Fatal("BuildRestartPlan returned nil")
			}
			if plan.Confirmation != tt.confirm {
				t.Fatalf("confirmation = %q, want %q", plan.Confirmation, tt.confirm)
			}
			if !strings.HasSuffix(plan.Command, "--plan") {
				t.Fatalf("plan.Command should be the preview form: %q", plan.Command)
			}
			if len(plan.NextActions) == 0 || !plan.NextActions[0].Required {
				t.Fatalf("first next action should be the required execute form: %+v", plan.NextActions)
			}
			// The execute action carries --yes exactly when the scope is gated:
			// a confirmation-free plan must not point at a --yes command.
			hasYes := strings.Contains(plan.NextActions[0].Command, "--yes")
			if gated := tt.confirm == "required"; hasYes != gated {
				t.Fatalf("execute action --yes = %v, want %v (confirmation=%q): %+v",
					hasYes, gated, tt.confirm, plan.NextActions[0])
			}
		})
	}
}
