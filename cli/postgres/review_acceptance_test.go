package postgres

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"pig/cli/ext"
	"pig/internal/output"
)

// Connection argument tests use this process as a live fixture. Only its
// executable identity is substituted; missing PID tests retain real probes.
func mockPostmasterIdentity(t *testing.T) {
	t.Helper()
	previous := postgresDBSUCommandOutput
	t.Cleanup(func() { postgresDBSUCommandOutput = previous })
	postgresDBSUCommandOutput = func(dbsu string, args []string) (string, error) {
		if len(args) == 5 && args[0] == "ps" && args[2] == strconv.Itoa(os.Getpid()) && args[4] == "comm=" {
			return "postgres\n", nil
		}
		return previous(dbsu, args)
	}
}

func TestReviewReusedPIDDoesNotBlockStart(t *testing.T) {
	dbsu := withCurrentUserAsDBSU(t)
	child := exec.Command("sleep", "30")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = child.Process.Kill(); _ = child.Wait() })
	cfg := runtimeFixture(t, dbsu, child.Process.Pid, "")
	bin, marker := t.TempDir(), filepath.Join(t.TempDir(), "started")
	for _, name := range []string{"pg_ctl", "initdb"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nprintf '%s\\n' \"$0\" >> \"$PIG_REVIEW_STARTED\"\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PIG_REVIEW_STARTED", marker)
	oldInit, oldActive, oldPG, oldInstalls := ext.Inited, ext.Active, ext.Postgres, ext.Installs
	t.Cleanup(func() { ext.Inited, ext.Active, ext.Postgres, ext.Installs = oldInit, oldActive, oldPG, oldInstalls })
	ext.Inited, ext.Active = true, &ext.PostgresInstall{BinPath: bin, MajorVersion: 18}
	ext.Postgres, ext.Installs = ext.Active, map[int]*ext.PostgresInstall{18: ext.Active}
	if result := StatusResult(cfg); result.Code != output.CodePgStatusNotRunning {
		t.Fatalf("reused PID not classified as stale: %+v", result)
	}
	if err := Stop(cfg, nil); err != nil {
		t.Fatal(err)
	}
	if err := Start(cfg, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("pg_ctl start was not called: %v", err)
	}
	if err := InitDB(cfg, &InitOptions{Force: true}); err != nil {
		t.Fatalf("independently proven stale PID blocked forced init: %v", err)
	}
	if calls, _ := os.ReadFile(marker); !strings.Contains(string(calls), "initdb") {
		t.Fatal("initdb was not called")
	}
}

func TestReviewStoppedDirectoryCannotReachSQLClient(t *testing.T) {
	dbsu := withCurrentUserAsDBSU(t)
	dead := exec.Command("true")
	if err := dead.Run(); err != nil {
		t.Fatal(err)
	}
	cfg := runtimeFixture(t, dbsu, dead.Process.Pid, "")
	bin, marker := t.TempDir(), filepath.Join(t.TempDir(), "wrong-instance")
	for _, name := range []string{"psql", "pg_repack"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\ntouch \"$PIG_REVIEW_SQL\"\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PIG_REVIEW_SQL", marker)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	oldInit, oldActive, oldPG, oldInstalls := ext.Inited, ext.Active, ext.Postgres, ext.Installs
	t.Cleanup(func() { ext.Inited, ext.Active, ext.Postgres, ext.Installs = oldInit, oldActive, oldPG, oldInstalls })
	ext.Inited, ext.Active = true, &ext.PostgresInstall{BinPath: bin, MajorVersion: 18}
	ext.Postgres, ext.Installs = ext.Active, map[int]*ext.PostgresInstall{18: ext.Active}
	for name, action := range map[string]func() error{
		"ps":     func() error { return Ps(cfg, nil) },
		"kill":   func() error { return Kill(cfg, &KillOptions{Execute: true}) },
		"vacuum": func() error { return Vacuum(cfg, "postgres", nil) },
		"repack": func() error { return Repack(cfg, "postgres", nil) },
		"psql":   func() error { return Psql(cfg, "postgres", &PsqlOptions{Command: "SELECT 1"}) },
	} {
		t.Run(name, func(t *testing.T) {
			_ = os.Remove(marker)
			if err := action(); err == nil {
				t.Error("stale target accepted")
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Error("SQL client reached another instance")
			}
		})
	}
}

func TestReviewForkUnknownStartDoesNotWriteSuccess(t *testing.T) {
	dbsu := withCurrentUserAsDBSU(t)
	cfg := runtimeFixture(t, dbsu, 1, "")
	if err := os.WriteFile(filepath.Join(cfg.PgData, "postmaster.pid"), []byte("invalid\n"), 0600); err != nil {
		t.Fatal(err)
	}
	metadata := fmt.Sprintf(`{"kind":"pg_fork","target":{"data":%q,"port":15432}}`, cfg.PgData)
	path := filepath.Join(cfg.PgData, "fork.json")
	if err := os.WriteFile(path, []byte(metadata), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := StartFork(ForkTargetOptions{DbSU: dbsu, DestData: cfg.PgData})
	if err == nil || result.Started || result.Already {
		t.Errorf("unknown fork reported success: %+v %v", result, err)
	}
	if got, _ := os.ReadFile(path); string(got) != metadata {
		t.Error("unknown runtime state changed fork metadata")
	}
	forks := []ForkInfo{{Target: ForkEndpoint{Data: cfg.PgData, Started: true, PID: 1}}}
	RefreshForkRuntimeStates(forks, dbsu)
	if !forks[0].RuntimeUnknown || forks[0].Target.Started || forks[0].Target.PID != 0 || forkListStatus(forks[0]) != "unknown" {
		t.Fatalf("unknown list state lost: %+v", forks[0])
	}
	for _, action := range []func(ForkTargetOptions) (ResultData, error){StopFork, RemoveFork} {
		if _, err := action(ForkTargetOptions{DbSU: dbsu, DestData: cfg.PgData, Force: true, StopBefore: true}); err == nil {
			t.Fatal("unknown fork state accepted")
		}
		if got, err := os.ReadFile(path); err != nil || string(got) != metadata {
			t.Fatalf("unknown fork state allowed a metadata change or removal: %q %v", got, err)
		}
	}
}

func TestReviewForkReplacementOwnershipAndEmptyTarget(t *testing.T) {
	dbsu := withCurrentUserAsDBSU(t)
	oldProbe, oldPort := forkProbeSourceDataDir, forkPortFree
	t.Cleanup(func() { forkProbeSourceDataDir, forkPortFree = oldProbe, oldPort })
	forkProbeSourceDataDir = func(string, int) (string, error) { return "", fmt.Errorf("offline") }
	forkPortFree = func(int) bool { return true }
	for _, kind := range []string{"empty", "unmanaged", "managed", "unknown"} {
		t.Run(kind, func(t *testing.T) {
			src, dst := t.TempDir(), t.TempDir()
			if err := os.WriteFile(filepath.Join(src, "PG_VERSION"), []byte("18"), 0600); err != nil {
				t.Fatal(err)
			}
			if kind != "empty" {
				if err := os.WriteFile(filepath.Join(dst, "PG_VERSION"), []byte("18"), 0600); err != nil {
					t.Fatal(err)
				}
				metadata := fmt.Sprintf(`{"kind":"pg_fork","managed":%t,"name":"review","target":{"data":%q}}`, kind == "managed", dst)
				if err := os.WriteFile(filepath.Join(dst, "fork.json"), []byte(metadata), 0600); err != nil {
					t.Fatal(err)
				}
				if kind == "unknown" {
					if err := os.WriteFile(filepath.Join(dst, "postmaster.pid"), []byte("invalid\n"), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			_, err := precheckInstance(&Options{Kind: KindInstance, DbSU: dbsu, Replace: true, Instance: InstanceOptions{SourceData: src, DestData: dst, SourcePort: 5432, DestPort: 15432}})
			if (err != nil) != (kind == "managed" || kind == "unknown") {
				t.Fatalf("replacement %s: %v", kind, err)
			}
			if kind == "unknown" && (strings.Contains(err.Error(), "PID: 0") || !strings.Contains(err.Error(), "cannot determine")) {
				t.Fatalf("unknown target reported as running: %v", err)
			}
		})
	}
}

func TestReviewForkForcePreservesOrdinaryDirectory(t *testing.T) {
	dbsu := withCurrentUserAsDBSU(t)
	src, dst := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "PG_VERSION"), []byte("18"), 0600); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dst, "keep.log")
	if err := os.WriteFile(marker, []byte("unrelated logs"), 0600); err != nil {
		t.Fatal(err)
	}
	oldProbe, oldPort := forkProbeSourceDataDir, forkPortFree
	t.Cleanup(func() { forkProbeSourceDataDir, forkPortFree = oldProbe, oldPort })
	forkProbeSourceDataDir = func(string, int) (string, error) { return "", fmt.Errorf("offline") }
	forkPortFree = func(int) bool { return true }
	result := ExecuteResult(&Options{Kind: KindInstance, DbSU: dbsu, Replace: true, Yes: true, Instance: InstanceOptions{Name: "review", SourceData: src, DestData: dst, SourcePort: 5432, DestPort: 15432}})
	if result.Success {
		t.Error("ordinary destination was accepted")
	}
	if data, err := os.ReadFile(marker); err != nil || !strings.Contains(string(data), "unrelated") {
		t.Fatalf("forced fork removed an unrelated directory: result=%+v err=%v", result, err)
	}
}
