package postgres

import (
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

// A shell function records copied commands without invoking PIG or PostgreSQL.
func pgPlanCommandArgs(t *testing.T, command string) []string {
	t.Helper()
	cmd := exec.Command("sh", "-c", "pig() { printf '%s\\000' \"$@\"; }\n"+command)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("parse plan command: %v", err)
	}
	return strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
}

func TestKillPlanCommandsPreserveTargetAndFilters(t *testing.T) {
	for _, dataDir := range []string{"", DefaultPgData, "/tmp/pg $TARGET's data"} {
		t.Run(dataDir, func(t *testing.T) {
			cfg := &Config{PgVersion: 18, PgData: dataDir, DbSU: "fixture-dbsu"}
			for _, execute := range []bool{false, true} {
				opts := &KillOptions{Execute: execute, Pid: 42, User: "app", Db: "report", State: "idle in transaction", Query: "long query", All: true, Cancel: true, Watch: 5}
				plan := BuildKillPlan(cfg, opts)
				commands := []string{plan.Command, plan.Verifications[0].Detail}
				for _, action := range plan.NextActions {
					commands = append(commands, action.Command)
				}
				for _, command := range commands {
					args := pgPlanCommandArgs(t, command)
					assertArgPair(t, args, "--version", "18")
					assertArgPair(t, args, "--dbsu", "fixture-dbsu")
					if dataDir != "" {
						assertArgPair(t, args, "-D", dataDir)
					} else if strings.Contains(command, " -D ") {
						t.Fatalf("plan invents explicit SQL target: %s", command)
					}
					if len(args) > 1 && args[1] == "kill" {
						for flag, value := range map[string]string{"--pid": "42", "--user": "app", "--database": "report", "--state": "idle in transaction", "--query": "long query", "--watch": "5"} {
							assertArgPair(t, args, flag, value)
						}
						for _, flag := range []string{"--all", "--cancel"} {
							if !strings.Contains(command, flag) {
								t.Fatalf("command lost %s: %s", flag, command)
							}
						}
					}
				}
				if !execute && !strings.Contains(plan.NextActions[0].Command, "--execute") {
					t.Fatalf("reviewed plan has no execution command: %+v", plan.NextActions)
				}
			}
		})
	}
	plan := BuildKillPlan(nil, nil)
	if plan.Command != "pig pg kill --plan" || plan.NextActions[0].Command != "pig pg kill --execute" || plan.NextActions[1].Command != "pig pg ps" {
		t.Fatalf("default kill plan changed: %+v", plan)
	}
}

func TestBuildInitCommandPreservesOptionsAndArgumentBoundary(t *testing.T) {
	cfg := &Config{PgData: "/tmp/pg $TARGET's data", PgVersion: 18, DbSU: "fixture-dbsu"}
	opts := &InitOptions{Encoding: "UTF8", Locale: "C.UTF-8", NoDataChecksums: true, Force: true, ExtraArgs: []string{"--waldir=/tmp/wal path", "--pwfile=/tmp/$secret"}}
	got := pgPlanCommandArgs(t, BuildInitCommand(cfg, opts, true))
	want := []string{"pg", "init", "--version", "18", "-D", cfg.PgData, "--dbsu", cfg.DbSU, "--encoding", "UTF8", "--locale", "C.UTF-8", "--no-data-checksums", "--force", "--yes", "--", "--waldir=/tmp/wal path", "--pwfile=/tmp/$secret"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("init retry argv = %q, want %q", got, want)
	}
	if got := BuildInitCommand(nil, nil, false); got != "pig pg init" {
		t.Fatalf("default init retry = %q", got)
	}
	if got := BuildInitCommand(&Config{PgData: DefaultPgData}, &InitOptions{Checksum: true}, false); got != "pig pg init -D /pg/data --data-checksum" {
		t.Fatalf("explicit default/checksum init retry = %q", got)
	}
}
