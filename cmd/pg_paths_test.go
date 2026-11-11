package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pig/cli/postgres"
)

func TestPgPathOptionsKeepCallerTargetAcrossDirectoryChange(t *testing.T) {
	originalConfig, originalLog := *pgConfig, pgStartLog
	t.Cleanup(func() { *pgConfig, pgStartLog = originalConfig, originalLog })
	caller := t.TempDir()
	t.Chdir(caller)
	*pgConfig = postgres.Config{PgData: "cluster's data", LogDir: "logs", DbSU: "fixture-dbsu", PgVersion: 18}
	pgStartLog = "logs/start.log"
	if err := normalizePgPathOptions(pgStartCmd); err != nil {
		t.Fatal(err)
	}

	// Model the cwd change made by su -, and a second call from an adapter.
	t.Chdir(t.TempDir())
	if err := normalizePgPathOptions(pgStartCmd); err != nil {
		t.Fatal(err)
	}
	for _, path := range []struct{ got, relative string }{
		{postgres.GetPgData(pgConfig), "cluster's data"},
		{postgres.GetLogDir(pgConfig), "logs"},
		{pgStartLog, "logs/start.log"},
	} {
		if want := filepath.Join(caller, path.relative); path.got != want {
			t.Errorf("path = %q, want caller target %q", path.got, want)
		}
	}
	plan := postgres.BuildKillPlan(pgConfig, &postgres.KillOptions{All: true})
	if !strings.Contains(plan.Command, caller) || !strings.Contains(plan.NextActions[0].Command, caller) {
		t.Fatalf("plan lost normalized target: %+v", plan)
	}
}

func TestPgPathOptionsPreserveOmittedAndAbsolutePaths(t *testing.T) {
	originalConfig, originalLog := *pgConfig, pgStartLog
	t.Cleanup(func() { *pgConfig, pgStartLog = originalConfig, originalLog })
	*pgConfig = postgres.Config{}
	pgStartLog = "start-only.log"
	if err := normalizePgPathOptions(pgPsqlCmd); err != nil {
		t.Fatal(err)
	}
	if pgConfig.PgData != "" || pgConfig.LogDir != "" || pgStartLog != "start-only.log" {
		t.Fatal("omitted SQL target or another command's log path changed")
	}
	pgConfig.PgData = "/data/pg18"
	if err := normalizePgPathOptions(pgPsqlCmd); err != nil || pgConfig.PgData != "/data/pg18" {
		t.Fatalf("absolute target changed: %+v, %v", pgConfig, err)
	}
}

func TestPgRelativePathRejectsUnavailableCallerDirectory(t *testing.T) {
	originalConfig := *pgConfig
	t.Cleanup(func() { *pgConfig = originalConfig })
	dir := filepath.Join(t.TempDir(), "removed-cwd")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Getwd(); err == nil {
		t.Skip("this platform still resolves a removed current directory")
	}
	*pgConfig = postgres.Config{PgData: "data"}
	if err := normalizePgPathOptions(pgPsqlCmd); err == nil {
		t.Fatal("unresolvable relative target must not reach a DBSU command")
	}
}
