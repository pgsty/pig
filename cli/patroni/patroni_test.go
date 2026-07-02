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

func TestLogJSONLUsesCatOutputAndPrintsJSONL(t *testing.T) {
	origRun := patroniRunJournalctlOutput
	defer func() { patroniRunJournalctlOutput = origRun }()

	var gotArgs []string
	patroniRunJournalctlOutput = func(args []string) (string, string, error) {
		gotArgs = append([]string(nil), args...)
		return "patroni started\nleader lock acquired\n", "", nil
	}

	var out bytes.Buffer
	withPatroniStdout(t, &out, func() {
		if err := LogJSONL(2); err != nil {
			t.Fatalf("LogJSONL returned error: %v", err)
		}
	})

	if !argsHasInOrder(gotArgs, "-u", "patroni", "-n", "2", "--no-pager", "-o", "cat") {
		t.Fatalf("journalctl args = %v, want patroni line count with cat output", gotArgs)
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
	origRun := patroniRunJournalctlOutput
	defer func() { patroniRunJournalctlOutput = origRun }()

	patroniRunJournalctlOutput = func(args []string) (string, string, error) {
		return "-- No entries --\npatroni started\n", "", nil
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
// CLUSTER_NAME prepend across Restart / Reinit / Switchover / Failover.
// Unlike pause / resume / list, those four patronictl subcommands require
// CLUSTER_NAME as the first positional argument; `-c <config>` does NOT supply
// scope to them. If a future refactor drops the prepend, this test fails fast.
func TestPatronictlPositionalContract(t *testing.T) {
	const cluster = "scope-name"

	for name, args := range map[string][]string{
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

func TestBuildSwitchoverPlanForceOption(t *testing.T) {
	opts := &SwitchoverOptions{
		Force: true,
	}
	plan := BuildSwitchoverPlan(opts)
	if plan == nil {
		t.Fatal("BuildSwitchoverPlan returned nil")
	}
	if !strings.Contains(plan.Command, "--force") {
		t.Errorf("plan.Command should include --force: %q", plan.Command)
	}
	// Risks should mention force mode
	hasForceRisk := false
	for _, risk := range plan.Risks {
		if strings.Contains(risk, "force") || strings.Contains(risk, "skip") {
			hasForceRisk = true
			break
		}
	}
	if !hasForceRisk {
		t.Error("plan.Risks should mention force mode")
	}
}

func TestBuildSwitchoverCommand(t *testing.T) {
	tests := []struct {
		name     string
		opts     *SwitchoverOptions
		contains []string
		excludes []string
	}{
		{
			name:     "nil opts",
			opts:     nil,
			contains: []string{"pig", "pt", "switchover"},
			excludes: []string{"--leader", "--candidate", "--scheduled", "--force"},
		},
		{
			name: "all options",
			opts: &SwitchoverOptions{
				Leader:    "pg-1",
				Candidate: "pg-2",
				Scheduled: "2026-02-03T12:00:00",
				Force:     true,
			},
			contains: []string{"--leader", "pg-1", "--candidate", "pg-2", "--scheduled", "--force"},
			excludes: []string{},
		},
		{
			name: "only candidate",
			opts: &SwitchoverOptions{
				Candidate: "pg-2",
			},
			contains: []string{"--candidate", "pg-2"},
			excludes: []string{"--leader", "--scheduled", "--force"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := buildSwitchoverCommand(tt.opts)
			for _, c := range tt.contains {
				if !strings.Contains(cmd, c) {
					t.Errorf("command should contain %q: %q", c, cmd)
				}
			}
			for _, e := range tt.excludes {
				if strings.Contains(cmd, e) {
					t.Errorf("command should not contain %q: %q", e, cmd)
				}
			}
		})
	}
}
