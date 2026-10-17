//go:build linux

package postgres

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"pig/cli/ext"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"
)

func linuxTestPostgresBin() string {
	if path := os.Getenv("PIG_TEST_PG_BIN"); path != "" {
		return path
	}
	return "/usr/lib/postgresql/18/bin"
}

func TestLinuxLivePostgresPermissionDenied(t *testing.T) {
	rawPID := os.Getenv("PIG_TEST_POSTGRES_PID")
	if rawPID == "" {
		t.Skip("requires a live PostgreSQL process owned by another user")
	}
	pid, err := strconv.Atoi(rawPID)
	if err != nil || pid <= 0 {
		t.Fatalf("invalid fixture PID: %q", rawPID)
	}
	if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.EPERM) {
		t.Fatalf("fixture must produce real EPERM: %v", err)
	}
	// Both kill(0) and ps are real here. Only the pid file belongs to the caller.
	cfg := runtimeFixture(t, withCurrentUserAsDBSU(t), pid, "")
	if result := StatusResult(cfg); result.Success || result.Code != output.CodePgStatusPermissionDenied {
		t.Fatalf("live PostgreSQL with EPERM reported as stopped or known: %+v", result)
	}
	if err := InitDB(cfg, &InitOptions{Force: true}); err == nil {
		t.Fatal("unknown live process allowed forced initialization")
	}
	if contents, err := os.ReadFile(filepath.Join(cfg.PgData, "PG_VERSION")); err != nil || string(contents) != "18\n" {
		t.Fatalf("refused initialization changed the directory: %q, %v", contents, err)
	}
	t.Log("real EPERM and PostgreSQL process identity preserved unknown state and blocked forced initialization")
}

// Run as an ordinary user with passwordless sudo to postgres in a disposable
// Linux container. The PG18 fixture is running on 6543 with a 0700 PGDATA.
func TestCloneLinuxProtectedPGDATA(t *testing.T) {
	dir := os.Getenv("PIG_TEST_CLONE_PGDATA")
	if dir == "" || os.Geteuid() == 0 {
		t.Skip("requires ordinary Linux caller and PIG_TEST_CLONE_PGDATA")
	}
	withCurrentUserAsDBSU(t)
	if _, err := os.ReadDir(dir); !os.IsPermission(err) {
		t.Fatalf("caller must be unable to access PGDATA: %v", err)
	}
	if err := exec.Command("df", "-T", dir+"/base").Run(); err == nil {
		t.Fatal("ordinary caller unexpectedly traversed 0700 PGDATA")
	}
	oldInited, oldActive, oldPostgres := ext.Inited, ext.Active, ext.Postgres
	ext.Inited, ext.Active = true, &ext.PostgresInstall{BinPath: linuxTestPostgresBin(), MajorVersion: 18}
	t.Cleanup(func() { ext.Inited, ext.Active, ext.Postgres = oldInited, oldActive, oldPostgres })
	opts := &CloneOptions{DbSU: "postgres", SourceDB: "postgres", DestDB: "clone_acceptance", ConnDB: "template1", Port: 6543}
	if err := prepareClone(opts); err != nil {
		t.Fatal(err)
	}
	if opts.Preflight.DataDirectory != dir || opts.Preflight.FileSystem == "" || opts.Preflight.FileSystemError != "" {
		t.Fatalf("DBSU filesystem probe failed: %+v", opts.Preflight)
	}
	// Also prove the live-process positive path, rather than only rejection cases.
	if running, pid := CheckPostgresRunningAsDBSU("postgres", dir); !running || pid <= 0 {
		t.Fatalf("live target not recognized: running=%v pid=%d", running, pid)
	}
	t.Logf("caller=%s, PGDATA inaccessible, clone preflight as postgres: %+v", config.CurrentUser, opts.Preflight)
}

func TestReviewLinuxPrivateForkAndStaleSQLTarget(t *testing.T) {
	dir := os.Getenv("PIG_TEST_CLONE_PGDATA")
	if dir == "" || os.Geteuid() == 0 {
		t.Skip("requires ordinary Linux caller and live protected PGDATA")
	}
	withCurrentUserAsDBSU(t)
	oldInit, oldActive, oldPG, oldInstalls := ext.Inited, ext.Active, ext.Postgres, ext.Installs
	t.Cleanup(func() { ext.Inited, ext.Active, ext.Postgres, ext.Installs = oldInit, oldActive, oldPG, oldInstalls })
	ext.Inited, ext.Active = true, &ext.PostgresInstall{BinPath: linuxTestPostgresBin(), MajorVersion: 18}
	ext.Postgres, ext.Installs = ext.Active, map[int]*ext.PostgresInstall{18: ext.Active}
	role, err := DetectRole(&Config{PgData: dir, DbSU: "postgres"}, nil)
	if err != nil || role.Role != RolePrimary || !role.Alive || role.Source != "psql" {
		t.Fatalf("live primary role detection failed: %+v, %v", role, err)
	}
	dst := filepath.Join(filepath.Dir(dir), "review-fork")
	opts := &Options{Kind: KindInstance, DbSU: "postgres", Instance: InstanceOptions{SourceData: dir, DestData: dst, SourcePort: 6543, DestPort: 15432}}
	if _, err := precheckInstance(opts); err != nil {
		t.Fatalf("DBSU fork precheck under private parent failed: %v", err)
	}
	alias := filepath.Join(filepath.Dir(dir), "source-alias")
	if err := utils.DBSUCommand("postgres", []string{"ln", "-s", dir, alias}); err != nil {
		t.Fatal(err)
	}
	opts.Instance.DestData = alias
	if _, err := precheckInstance(opts); err == nil {
		t.Fatal("DBSU-resolved source alias accepted as destination")
	}
	stopped := filepath.Join(filepath.Dir(dir), "stopped")
	if err := utils.DBSUCommand("postgres", []string{"mkdir", stopped}); err != nil {
		t.Fatal(err)
	}
	dead := exec.Command("true")
	if err := dead.Run(); err != nil {
		t.Fatal(err)
	}
	pid := fmt.Sprintf("%d\n%s\n1738656000\n6543\n/var/run/postgresql\n", dead.Process.Pid, stopped)
	if err := utils.WriteFileAsDBSU(filepath.Join(stopped, "postmaster.pid"), pid, "postgres"); err != nil {
		t.Fatal(err)
	}
	if err := utils.WriteFileAsDBSU(filepath.Join(stopped, "PG_VERSION"), "18\n", "postgres"); err != nil {
		t.Fatal(err)
	}
	if err := Psql(&Config{PgData: stopped, DbSU: "postgres"}, "postgres", &PsqlOptions{Command: "CREATE TABLE pig_review_wrong_target(id int)"}); err == nil {
		t.Fatal("stale target reached live database")
	}
	out, err := utils.DBSUCommandStdout("postgres", []string{ext.Active.Psql(), "-p", "6543", "-h", "/var/run/postgresql", "-d", "postgres", "-Atc", "SELECT to_regclass('pig_review_wrong_target') IS NULL"})
	if err != nil || strings.TrimSpace(out) != "t" {
		t.Fatalf("other instance was changed: %q %v", out, err)
	}
	if _, err := os.Lstat(stopped); !os.IsPermission(err) {
		t.Fatalf("fork removal fixture must have an inaccessible parent: %v", err)
	}
	metadata := fmt.Sprintf(`{"kind":"pg_fork","target":{"data":%q,"port":15432}}`, stopped)
	if err := utils.WriteFileAsDBSU(filepath.Join(stopped, "fork.json"), metadata, "postgres"); err != nil {
		t.Fatal(err)
	}
	forkAlias := filepath.Join(filepath.Dir(dir), "fork-alias")
	if err := utils.DBSUCommand("postgres", []string{"ln", "-s", stopped, forkAlias}); err != nil {
		t.Fatal(err)
	}
	if _, err := RemoveFork(ForkTargetOptions{DbSU: "postgres", DestData: forkAlias, Yes: true}); err == nil {
		t.Fatal("private symlink fork was accepted for removal")
	}
	if _, err := RemoveFork(ForkTargetOptions{DbSU: "postgres", DestData: stopped, Yes: true}); err != nil {
		t.Fatalf("valid private unmanaged fork could not be removed as DBSU: %v", err)
	}
	if err := utils.DBSUCommand("postgres", []string{"test", "!", "-e", stopped}); err != nil {
		t.Fatalf("fork data remains after successful removal: %v", err)
	}
	t.Log("private PGDATA precheck/removal passed, symlinks rejected, stale target could not mutate the other live instance")
}
