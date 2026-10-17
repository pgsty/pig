package postgres

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"pig/cli/ext"
)

func TestMaintenanceRejectsConnectionStrings(t *testing.T) {
	oldRun, oldList := maintRunPsqlMaintenance, maintGetAllDatabases
	t.Cleanup(func() { maintRunPsqlMaintenance, maintGetAllDatabases = oldRun, oldList })
	maintRunPsqlMaintenance = func(*Config, string, string) error {
		t.Error("invalid database reached SQL execution")
		return nil
	}
	maintGetAllDatabases = func(*Config) ([]string, error) {
		t.Error("invalid database reached database enumeration")
		return nil, nil
	}
	for _, database := range []string{
		"host=/unused dbname=app password=SECRET",
		"postgresql://review:SECRET@localhost/app",
		"postgres://review:SECRET@localhost/app",
	} {
		for _, all := range []bool{false, true} {
			for name, run := range map[string]func() error{
				"vacuum":  func() error { return Vacuum(nil, database, &VacuumOptions{MaintOptions: MaintOptions{All: all}}) },
				"analyze": func() error { return Analyze(nil, database, &MaintOptions{All: all}) },
				"freeze":  func() error { return Freeze(nil, database, &FreezeOptions{All: all}) },
				"repack":  func() error { return Repack(nil, database, &RepackOptions{MaintOptions: MaintOptions{All: all}}) },
			} {
				if err := run(); err == nil || !strings.Contains(err.Error(), "connection string") || strings.Contains(err.Error(), "SECRET") {
					t.Errorf("%s all=%v: unsafe validation result: %v", name, all, err)
				}
			}
		}
	}
}

func TestMaintenanceAllValidatesDatabaseListBeforeChanges(t *testing.T) {
	oldRun, oldList := maintRunPsqlMaintenance, maintGetAllDatabases
	t.Cleanup(func() { maintRunPsqlMaintenance, maintGetAllDatabases = oldRun, oldList })
	maintGetAllDatabases = func(*Config) ([]string, error) {
		return []string{"app", "host=/unused password=SECRET"}, nil
	}
	maintRunPsqlMaintenance = func(*Config, string, string) error {
		t.Error("partially executed an ambiguous database list")
		return nil
	}
	if err := Vacuum(nil, "", &VacuumOptions{MaintOptions: MaintOptions{All: true}}); err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("unsafe all-database result: %v", err)
	}
}

// Exercise the real libpq parser against nonexistent local sockets. Only the
// postmaster identity is substituted; no server can receive the query.
func TestMaintenanceLibpqTargetBoundary(t *testing.T) {
	psql, err := exec.LookPath("psql")
	if err != nil {
		t.Skip("psql is required for the libpq acceptance test")
	}
	dbsu := withCurrentUserAsDBSU(t)
	mockPostmasterIdentity(t)
	// Keep socket paths below the platform's Unix-domain path length limit.
	dir, err := os.MkdirTemp("/tmp", "pig-maint-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	bin := t.TempDir()
	called, diagnostic := filepath.Join(dir, "called"), filepath.Join(dir, "diagnostic")
	script := "#!/bin/sh\nprintf called > \"$PIG_TEST_CLIENT_CALLED\"\nexec \"$PIG_TEST_REAL_PSQL\" \"$@\" 2>\"$PIG_TEST_CLIENT_DIAGNOSTIC\"\n"
	if err := os.WriteFile(filepath.Join(bin, "psql"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PIG_TEST_REAL_PSQL", psql)
	t.Setenv("PIG_TEST_CLIENT_CALLED", called)
	t.Setenv("PIG_TEST_CLIENT_DIAGNOSTIC", diagnostic)
	t.Setenv("PIG_NON_INTERACTIVE", "1")
	t.Setenv("PGCONNECT_TIMEOUT", "1")
	t.Setenv("LC_ALL", "C")
	oldInited, oldActive, oldPostgres := ext.Inited, ext.Active, ext.Postgres
	ext.Inited, ext.Active = true, &ext.PostgresInstall{BinPath: bin}
	t.Cleanup(func() { ext.Inited, ext.Active, ext.Postgres = oldInited, oldActive, oldPostgres })
	target := filepath.Join(dir, "target")
	writeTestPostmasterPid(t, dir, "1738656000", "6543", target)
	cfg := &Config{PgData: dir, DbSU: dbsu}
	for _, database := range []string{"app", "--all", "库 名'\\"} {
		if err := RunPsqlMaintenance(cfg, database, "SELECT 1"); err == nil {
			t.Fatal("unexpected connection to a nonexistent socket")
		}
		raw, err := os.ReadFile(diagnostic)
		if err != nil || !strings.Contains(string(raw), target+"/.s.PGSQL.6543") {
			t.Fatalf("libpq did not retain the selected target: %v / %s", err, raw)
		}
	}
	if err := os.Remove(called); err != nil {
		t.Fatal(err)
	}
	database := "host=" + filepath.Join(dir, "other") + " port=6544 dbname=app password=SECRET"
	if err := RunPsqlMaintenance(cfg, database, "SELECT 1"); err == nil || !strings.Contains(err.Error(), "connection string") || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("connection string escaped validation: %v", err)
	}
	if _, err := os.Stat(called); !os.IsNotExist(err) {
		t.Fatalf("connection string reached the real libpq client: %v", err)
	}
}
