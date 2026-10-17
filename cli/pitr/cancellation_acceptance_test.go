package pitr

import (
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pig/cli/pgbackrest"
	"pig/cli/postgres"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"
)

func TestPITRConfirmationCancellationExecution(t *testing.T) {
	account, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	oldUser, oldFormat, oldStdin := config.CurrentUser, config.OutputFormat, os.Stdin
	oldInfo, oldResolve := pitrLoadPgBackRestInfo, pitrResolvePathAsDBSU
	oldStop, oldRestore, oldStart, oldCheck, oldSleep := pitrStopPostgres, pitrRestore, pitrStartPostgres, pitrCheckPostgresRunningAsDBSU, pitrSleep
	t.Cleanup(func() {
		config.CurrentUser, config.OutputFormat, os.Stdin = oldUser, oldFormat, oldStdin
		pitrLoadPgBackRestInfo, pitrResolvePathAsDBSU = oldInfo, oldResolve
		pitrStopPostgres, pitrRestore, pitrStartPostgres, pitrCheckPostgresRunningAsDBSU, pitrSleep = oldStop, oldRestore, oldStart, oldCheck, oldSleep
	})
	config.CurrentUser = account.Username
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "PG_VERSION"), []byte("18"), 0600); err != nil {
		t.Fatal(err)
	}
	opts := &Options{Default: true, DbSU: account.Username, Stanza: "pg-meta", ConfigPath: writePITRTestConfig(t, "[pg-meta]\npg1-path="+dir+"\n")}
	prechecks := 0
	pitrLoadPgBackRestInfo = func(*pgbackrest.Config, string) ([]pgbackrest.PgBackRestInfo, error) {
		prechecks++
		return []pgbackrest.PgBackRestInfo{{Name: "pg-meta", Status: pgbackrest.StatusInfo{Code: 0}, Backup: []pgbackrest.BackupInfo{{Label: "20261016-010101F"}}}}, nil
	}
	pitrResolvePathAsDBSU = func(_ string, path string) (string, error) { return filepath.Clean(path), nil }
	stop, restore, start := 0, 0, 0
	pitrCheckPostgresRunningAsDBSU = func(string, string) (bool, int) { return true, 12345 }
	pitrSleep = func(time.Duration) {}
	pitrStopPostgres = func(*postgres.Config, *postgres.StopOptions) error {
		stop++
		return errors.New("stop must not execute")
	}
	pitrRestore = func(*pgbackrest.Config, *pgbackrest.RestoreOptions) error {
		restore++
		return errors.New("restore must not execute")
	}
	pitrStartPostgres = func(*postgres.Config, *postgres.StartOptions) error {
		start++
		return errors.New("start must not execute")
	}
	bin, services := t.TempDir(), filepath.Join(t.TempDir(), "services")
	if err := os.WriteFile(filepath.Join(bin, "systemctl"), []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$PIG_PITR_SERVICES\"\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PIG_PITR_SERVICES", services)
	t.Setenv("PIG_NO_SUDO", "1")
	for _, mode := range []string{"text", "json", "yaml"} {
		for _, input := range []string{"n\n", ""} {
			t.Run(mode+"/"+strings.TrimSpace(input), func(t *testing.T) {
				config.OutputFormat = mode
				in, err := os.CreateTemp(t.TempDir(), "stdin")
				if err != nil {
					t.Fatal(err)
				}
				defer in.Close()
				_, _ = in.WriteString(input)
				_, _ = in.Seek(0, 0)
				os.Stdin = in
				before := prechecks
				if mode == "text" {
					err := Execute(opts)
					var pe *PITRError
					if !errors.As(err, &pe) || pe.Code != output.CodePITRConfirmationRequired || utils.ExitCode(err) != 7 {
						t.Fatalf("wrong cancellation error/exit: %v", err)
					}
				} else {
					result := ExecuteResult(opts)
					if result.Success || result.Code != output.CodePITRConfirmationRequired || output.ExitCode(result.Code) != 7 {
						t.Fatalf("wrong cancellation result/exit: %+v", result)
					}
				}
				if prechecks != before+1 {
					t.Fatal("did not traverse the execution precheck")
				}
				if stop != 0 || restore != 0 || start != 0 {
					t.Fatalf("cancelled workflow mutated state: stop=%d restore=%d start=%d", stop, restore, start)
				}
				calls, _ := os.ReadFile(services)
				if strings.Contains(string(calls), "stop") || strings.Contains(string(calls), "start") {
					t.Fatalf("cancelled workflow changed Patroni: %s", calls)
				}
			})
		}
	}
}
