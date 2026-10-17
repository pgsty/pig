package postgres

import (
	"os"
	"path/filepath"
	"pig/cli/ext"
	"pig/internal/config"
	"strings"
	"testing"
)

func TestExplicitDataDirReachesEverySQLTool(t *testing.T) {
	dbsu := withCurrentUserAsDBSU(t)
	mockPostmasterIdentity(t)
	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "argv")
	script := `#!/bin/sh
printf '%s\000' "$0" "$@" >> "$PIG_TEST_PG_ARGV"
printf '\000' >> "$PIG_TEST_PG_ARGV"
case "$*" in *'SELECT datname'*) printf 'app\nreport\n';; esac
`
	for _, name := range []string{"psql", "pg_repack"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PIG_TEST_PG_ARGV", log)
	// A conflicting ambient connection must never win over -D.
	t.Setenv("PGHOST", "wrong-default")
	t.Setenv("PGPORT", "5432")
	oldInited, oldActive, oldPostgres := ext.Inited, ext.Active, ext.Postgres
	ext.Inited = true
	ext.Active = &ext.PostgresInstall{BinPath: bin}
	t.Cleanup(func() { ext.Inited, ext.Active, ext.Postgres = oldInited, oldActive, oldPostgres })
	for _, state := range []string{"valid", "missing", "bad-port", "large-port", "no-socket", "copied-pid"} {
		t.Run(state, func(t *testing.T) {
			dir := t.TempDir()
			cfg := &Config{PgData: dir, DbSU: dbsu}
			port, socket := "6543", "/tmp/pig-target-socket"
			switch state {
			case "bad-port":
				port = "oops"
			case "large-port":
				port = "70000"
			case "no-socket":
				socket = ""
			}
			if state != "missing" {
				writeTestPostmasterPid(t, dir, "1738656000", port, socket)
			}
			if state == "copied-pid" {
				if err := os.WriteFile(filepath.Join(dir, "postmaster.pid"), []byte(testPostmasterPid("/unrelated/source", "1738656000", port, socket)), 0600); err != nil {
					t.Fatal(err)
				}
			}
			actions := []struct {
				name string
				run  func() error
			}{
				{"ps", func() error { return Ps(cfg, nil) }},
				{"kill-preview", func() error { return Kill(cfg, nil) }},
				{"kill", func() error { return Kill(cfg, &KillOptions{Execute: true}) }},
				{"vacuum", func() error { return Vacuum(cfg, "app", nil) }},
				{"analyze", func() error { return Analyze(cfg, "app", nil) }},
				{"freeze", func() error { return Freeze(cfg, "app", nil) }},
				{"repack", func() error { return Repack(cfg, "app", nil) }},
				{"vacuum-all", func() error { return Vacuum(cfg, "", &VacuumOptions{MaintOptions: MaintOptions{All: true}}) }},
				{"analyze-all", func() error { return Analyze(cfg, "", &MaintOptions{All: true}) }},
				{"freeze-all", func() error { return Freeze(cfg, "", &FreezeOptions{All: true}) }},
				{"repack-all", func() error { return Repack(cfg, "", &RepackOptions{MaintOptions: MaintOptions{All: true}}) }},
			}
			for _, action := range actions {
				t.Run(action.name, func(t *testing.T) {
					_ = os.Remove(log)
					err := action.run()
					data, _ := os.ReadFile(log)
					if state != "valid" {
						if err == nil || len(data) != 0 {
							t.Fatalf("ambiguous target executed: err=%v argv=%q", err, data)
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					calls := strings.Split(strings.TrimSuffix(string(data), "\x00\x00"), "\x00\x00")
					if len(data) == 0 {
						t.Fatal("no child command executed")
					}
					for _, call := range calls {
						args := strings.Split(call, "\x00")
						assertArgPair(t, args, "-p", port)
						assertArgPair(t, args, "-h", socket)
					}
					if strings.HasSuffix(action.name, "-all") && action.name != "repack-all" && len(calls) != 3 {
						t.Fatalf("enumeration + two maintenance calls expected, got %d", len(calls))
					}
				})
			}
		})
	}
}

func TestPsqlExplicitStdinInNonInteractiveModes(t *testing.T) {
	dbsu := withCurrentUserAsDBSU(t)
	oldInited, oldActive, oldFormat, oldInput := ext.Inited, ext.Active, config.OutputFormat, os.Stdin
	t.Cleanup(func() {
		ext.Inited, ext.Active, config.OutputFormat, os.Stdin = oldInited, oldActive, oldFormat, oldInput
	})
	bin := t.TempDir()
	output := filepath.Join(t.TempDir(), "sql")
	if err := os.WriteFile(filepath.Join(bin, "psql"), []byte("#!/bin/sh\ncat > \"$PIG_TEST_SQL_INPUT\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	ext.Inited, ext.Active = true, &ext.PostgresInstall{BinPath: bin}
	t.Setenv("PIG_TEST_SQL_INPUT", output)
	for _, mode := range []string{"env", "json", "yaml"} {
		t.Run(mode, func(t *testing.T) {
			config.OutputFormat = config.OUTPUT_TEXT
			t.Setenv("PIG_NON_INTERACTIVE", "")
			if mode == "env" {
				t.Setenv("PIG_NON_INTERACTIVE", "1")
			} else {
				config.OutputFormat = mode
			}
			input, err := os.CreateTemp(t.TempDir(), "stdin")
			if err != nil {
				t.Fatal(err)
			}
			defer input.Close()
			const sql = "select 'data stdin sentinel';\n"
			_, _ = input.WriteString(sql)
			_, _ = input.Seek(0, 0)
			os.Stdin = input
			if err := Psql(&Config{DbSU: dbsu}, "postgres", &PsqlOptions{File: "-"}); err != nil {
				t.Fatal(err)
			}
			if got, err := os.ReadFile(output); err != nil || string(got) != sql {
				t.Fatalf("explicit psql input lost: %q, %v", got, err)
			}
		})
	}
}
