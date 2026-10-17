package postgres

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"pig/cli/ext"
	"pig/internal/config"
	"pig/internal/output"
)

func runtimeFixture(t *testing.T, dbsu string, pid int, recordedDir string) *Config {
	t.Helper()
	dir := t.TempDir()
	if recordedDir == "" {
		recordedDir = dir
	}
	for name, content := range map[string]string{
		"PG_VERSION":     "18\n",
		"postmaster.pid": fmt.Sprintf("%d\n%s\n1738656000\n6543\n/tmp/pig-target\n", pid, recordedDir),
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return &Config{PgData: dir, DbSU: dbsu}
}

func TestRuntimePermissionDenialBlocksForcedInitAndLifecycle(t *testing.T) {
	dbsu := withCurrentUserAsDBSU(t)
	if err := syscall.Kill(1, 0); !errors.Is(err, syscall.EPERM) {
		t.Skipf("requires an ordinary uid with EPERM for PID 1; got %v", err)
	}
	// EPERM must remain unknown when independent identity is unavailable.
	previous := postgresDBSUCommandOutput
	t.Cleanup(func() { postgresDBSUCommandOutput = previous })
	postgresDBSUCommandOutput = func(dbsu string, args []string) (string, error) {
		if len(args) > 0 && args[0] == "ps" {
			return "", syscall.EPERM
		}
		return previous(dbsu, args)
	}
	cfg := runtimeFixture(t, dbsu, 1, "")
	// This goes through the actual destructive caller, using only a temporary directory.
	err := InitDB(cfg, &InitOptions{Force: true})
	if _, statErr := os.Stat(filepath.Join(cfg.PgData, "PG_VERSION")); statErr != nil {
		t.Fatalf("EPERM allowed forced init to remove PGDATA: err=%v stat=%v", err, statErr)
	}
	if err == nil {
		t.Fatal("forced init accepted an unknown running state")
	}
	if result := InitResult(cfg, &InitOptions{Force: true}); result.Success || result.Code != output.CodePgPermissionDenied {
		t.Fatalf("permission failure reported as a running conflict: %+v", result)
	}
	if role, err := DetectRole(cfg, nil); err == nil || role != nil {
		t.Fatalf("permission failure reported a role: %+v, %v", role, err)
	}
	for name, run := range map[string]func() error{
		"start":   func() error { return Start(cfg, nil) },
		"stop":    func() error { return Stop(cfg, nil) },
		"restart": func() error { return Restart(cfg, nil) },
	} {
		t.Run(name, func(t *testing.T) {
			if err := run(); err == nil {
				t.Fatal("permission failure reported as lifecycle success")
			}
		})
	}
	if result := StatusResult(cfg); result.Success || result.Code == output.CodePgStatusNotRunning {
		t.Fatalf("permission failure reported as stopped: %+v", result)
	}
}

func TestRuntimeRoleUsesVerifiedState(t *testing.T) {
	dbsu := withCurrentUserAsDBSU(t)
	mockPostmasterIdentity(t)
	oldInit, oldInstalls := ext.Inited, ext.Installs
	t.Cleanup(func() { ext.Inited, ext.Installs = oldInit, oldInstalls })
	// Exercise the file fallback without an installed SQL client or process listing.
	// The test binary's name can itself contain "postgres" on some platforms.
	ext.Inited, ext.Installs = true, nil
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "ps"), []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	dead := exec.Command("true")
	if err := dead.Run(); err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"running", "stale", "missing", "invalid"} {
		t.Run(state, func(t *testing.T) {
			pid := os.Getpid()
			if state == "stale" {
				pid = dead.Process.Pid
			}
			cfg := runtimeFixture(t, dbsu, pid, "")
			cfg.PgVersion = 18
			for i := 0; i < 10; i++ {
				if err := os.WriteFile(filepath.Join(cfg.PgData, fmt.Sprintf("fixture-%d", i)), nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(cfg.PgData, "standby.signal"), nil, 0600); err != nil {
				t.Fatal(err)
			}
			pidfile := filepath.Join(cfg.PgData, "postmaster.pid")
			if state == "missing" {
				if err := os.Remove(pidfile); err != nil {
					t.Fatal(err)
				}
			} else if state == "invalid" {
				if err := os.WriteFile(pidfile, []byte("invalid\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			role, err := DetectRole(cfg, nil)
			if state == "invalid" {
				if err == nil || role != nil || detectRoleString(cfg) != "unknown" {
					t.Fatalf("unknown runtime state reported a role: %+v, %v", role, err)
				}
				return
			}
			if err != nil || role.Role != RoleReplica || role.Source != "pgdata" || role.Alive != (state == "running") {
				t.Fatalf("role fallback disagrees with runtime state: %+v, %v", role, err)
			}
		})
	}
}

func TestRuntimeInitDistinguishesUnknownAndRunning(t *testing.T) {
	dbsu := withCurrentUserAsDBSU(t)
	mockPostmasterIdentity(t)
	for _, invalid := range []bool{false, true} {
		t.Run(fmt.Sprintf("invalid=%v", invalid), func(t *testing.T) {
			cfg := runtimeFixture(t, dbsu, os.Getpid(), "")
			want := output.CodePgInitRunningConflict
			if invalid {
				want = output.CodePgInitFailed
				if err := os.WriteFile(filepath.Join(cfg.PgData, "postmaster.pid"), []byte("invalid\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			result := InitResult(cfg, &InitOptions{Force: true})
			if result.Success || result.Code != want {
				t.Fatalf("unexpected init result: %+v", result)
			}
			if invalid && (!strings.Contains(result.Message, "Cannot determine") || strings.Contains(result.Detail, "PID=0")) {
				t.Fatalf("unknown state claimed running: %+v", result)
			}
			if data, err := os.ReadFile(filepath.Join(cfg.PgData, "PG_VERSION")); err != nil || string(data) != "18\n" {
				t.Fatalf("refused init changed PGDATA: %q, %v", data, err)
			}
		})
	}
}

func TestRuntimeStaleAndUnrelatedPIDCallers(t *testing.T) {
	dbsu := withCurrentUserAsDBSU(t)
	child := exec.Command("sleep", "30")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = child.Process.Kill(); _ = child.Wait() })
	dead := exec.Command("true")
	if err := dead.Run(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		pid     int
		dir     string
		unknown bool
	}{
		{"missing-pid", dead.Process.Pid, "", false},
		{"copied-pid", child.Process.Pid, "/unrelated/source", false},
		{"same-user-unrelated", child.Process.Pid, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := runtimeFixture(t, dbsu, tc.pid, tc.dir)
			result := StatusResult(cfg)
			if result.Success || (result.Code == output.CodePgStatusNotRunning) == tc.unknown {
				t.Fatalf("unexpected runtime status: %+v", result)
			}
			if err := Stop(cfg, nil); (err != nil) != tc.unknown {
				t.Fatalf("stop result for stale/unknown PID: %v", err)
			}
			if err := child.Process.Signal(syscall.Signal(0)); err != nil {
				t.Fatalf("unrelated process was signalled: %v", err)
			}
		})
	}
}

func TestRuntimeReusedPrivilegedPIDIsNotUnknown(t *testing.T) {
	dbsu := withCurrentUserAsDBSU(t)
	if err := syscall.Kill(1, 0); !errors.Is(err, syscall.EPERM) {
		t.Skipf("requires EPERM for PID 1; got %v", err)
	}
	name, err := exec.Command("ps", "-p", "1", "-o", "comm=").Output()
	if err != nil || strings.TrimSpace(string(name)) == "" {
		t.Skip("PID 1 identity unavailable")
	}
	if base := filepath.Base(strings.TrimSpace(string(name))); base == "postgres" || base == "postmaster" {
		t.Skip("PID 1 is PostgreSQL")
	}
	cfg := runtimeFixture(t, dbsu, 1, "")
	if result := StatusResult(cfg); result.Code != output.CodePgStatusNotRunning {
		t.Fatalf("independently identified PID reuse became unknown: %+v", result)
	}
	if err := Stop(cfg, nil); err != nil {
		t.Fatalf("stale privileged PID blocked stop: %v", err)
	}
}

func TestRuntimeEscalatedProbeFailureBlocksForcedInit(t *testing.T) {
	dbsu := withCurrentUserAsDBSU(t)
	config.CurrentUser = "ordinary-caller"
	bin := t.TempDir()
	script := `#!/bin/sh
while [ "$1" != -- ]; do shift; done
shift
if [ "$1" = kill ]; then
  printf 'kill: %s\n' "$PIG_PROBE_ERROR" >&2
  exit 1
fi
if [ "$1" = ps ]; then printf 'postgres\n'; exit 0; fi
exec "$@"
`
	if err := os.WriteFile(filepath.Join(bin, "sudo"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, detail := range []string{"Operation not permitted", "Permission denied", "unexpected probe error"} {
		t.Run(detail, func(t *testing.T) {
			t.Setenv("PIG_PROBE_ERROR", detail)
			cfg := runtimeFixture(t, dbsu, os.Getpid(), "")
			if result := StatusResult(cfg); result.Success || result.Code == output.CodePgStatusNotRunning {
				t.Fatalf("probe error became stopped: %+v", result)
			}
			if err := InitDB(cfg, &InitOptions{Force: true}); err == nil {
				t.Fatal("forced init accepted unknown state")
			}
			if _, err := os.Stat(filepath.Join(cfg.PgData, "PG_VERSION")); err != nil {
				t.Fatalf("unknown state allowed removal: %v", err)
			}
		})
	}
}

func TestTimelineNeverFallsBackToDefaultConnection(t *testing.T) {
	dbsu := withCurrentUserAsDBSU(t)
	mockPostmasterIdentity(t)
	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "calls")
	for name, body := range map[string]string{
		"psql":           "printf 'psql %s\\n' \"$*\" >> \"$PIG_TIMELINE_CALLS\"\nprintf '99\\n'\n",
		"pg_controldata": "printf 'control %s\\n' \"$*\" >> \"$PIG_TIMELINE_CALLS\"\n[ \"$PIG_CONTROL_FAIL\" = 1 ] && exit 1\nprintf \"Latest checkpoint's TimeLineID: 7\\n\"\n",
	} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	oldInited, oldActive, oldPostgres := ext.Inited, ext.Active, ext.Postgres
	oldInstalls := ext.Installs
	ext.Inited, ext.Active = true, &ext.PostgresInstall{BinPath: bin, MajorVersion: 18}
	ext.Postgres = ext.Active
	ext.Installs = map[int]*ext.PostgresInstall{18: ext.Active}
	t.Cleanup(func() {
		ext.Inited, ext.Active, ext.Postgres, ext.Installs = oldInited, oldActive, oldPostgres, oldInstalls
	})
	t.Setenv("PIG_TIMELINE_CALLS", log)
	t.Setenv("PGHOST", "wrong-default")
	t.Setenv("PGPORT", "5432")
	dead := exec.Command("true")
	if err := dead.Run(); err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"valid", "missing", "invalid", "invalid-pid", "stopped", "copied", "no-socket", "unknown"} {
		t.Run(state, func(t *testing.T) {
			cfg := runtimeFixture(t, dbsu, os.Getpid(), "")
			pidfile := filepath.Join(cfg.PgData, "postmaster.pid")
			data, _ := os.ReadFile(pidfile)
			switch state {
			case "missing", "unknown":
				_ = os.Remove(pidfile)
			case "invalid":
				_ = os.WriteFile(pidfile, []byte("bad\n"), 0600)
			case "invalid-pid":
				_ = os.WriteFile(pidfile, []byte("bad"+string(data)[strings.IndexByte(string(data), '\n'):]), 0600)
			case "stopped":
				_ = os.WriteFile(pidfile, []byte(fmt.Sprint(dead.Process.Pid)+string(data)[strings.IndexByte(string(data), '\n'):]), 0600)
			case "copied":
				_ = os.WriteFile(pidfile, []byte(strings.ReplaceAll(string(data), cfg.PgData, "/elsewhere")), 0600)
			case "no-socket":
				_ = os.WriteFile(pidfile, []byte(strings.ReplaceAll(string(data), "/tmp/pig-target", "")), 0600)
			}
			t.Setenv("PIG_CONTROL_FAIL", "0")
			want := 7
			if state == "valid" {
				want = 99
			}
			if state == "unknown" {
				want = 0
				t.Setenv("PIG_CONTROL_FAIL", "1")
			}
			_ = os.Remove(log)
			if got := getTimeline(cfg, dbsu); got != want {
				t.Errorf("timeline=%d, want %d", got, want)
			}
			calls, _ := os.ReadFile(log)
			if state != "valid" && (strings.Contains(string(calls), "psql") || !strings.Contains(string(calls), "-D "+cfg.PgData)) {
				t.Fatalf("timeline queried wrong target: %s", calls)
			}
			if state == "valid" && !strings.Contains(string(calls), "-p 6543 -h /tmp/pig-target") {
				t.Fatalf("unbound SQL: %s", calls)
			}
		})
	}
}
