package postgres

import (
	"fmt"
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

func TestUnmanagedInitializedForkReplacementRequiresMetadata(t *testing.T) {
	dbsu := withCurrentUserAsDBSU(t)
	for _, kind := range []string{"missing", "corrupt", "wrong-kind", "wrong-target"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			src, dst := filepath.Join(root, "source"), filepath.Join(root, "target")
			for _, dir := range []string{src, dst} {
				if err := os.Mkdir(dir, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "PG_VERSION"), []byte("18"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			metadata := map[string]string{"corrupt": "{", "wrong-kind": `{"kind":"other"}`, "wrong-target": `{"kind":"pg_fork","target":{"data":"/elsewhere"}}`}[kind]
			if metadata != "" {
				if err := os.WriteFile(filepath.Join(dst, "fork.json"), []byte(metadata), 0600); err != nil {
					t.Fatal(err)
				}
			}
			_, err := precheckInstance(&Options{Kind: KindInstance, DbSU: dbsu, Replace: true, Instance: InstanceOptions{SourceData: src, DestData: dst, SourcePort: 5432, DestPort: 15432}})
			if err == nil || !strings.Contains(err.Error(), "fork") {
				t.Fatalf("metadata %s accepted: %v", kind, err)
			}
			if got, _ := os.ReadFile(filepath.Join(dst, "PG_VERSION")); string(got) != "18" {
				t.Fatal("target changed")
			}
		})
	}
}

func TestForkDefaultDataPathIsNotReserved(t *testing.T) {
	if err := validateForkRemovalPath("/pg/data"); err != nil {
		t.Errorf("conventional data path rejected before ownership checks: %v", err)
	}
	t.Run("destination", func(t *testing.T) {
		// Only resolve the existing path; never create or remove /pg/data.
		if _, err := normalizeDataPath("", "/pg/data"); err != nil {
			t.Skipf("host data path cannot be resolved: %v", err)
		}
		if _, _, err := validateForkDataPathsAs("", t.TempDir(), "/pg/data"); err != nil {
			t.Fatalf("conventional data path rejected as an unrelated destination: %v", err)
		}
	})
}

func TestForkRejectsDanglingDestinationAlias(t *testing.T) {
	root := t.TempDir()
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(filepath.Join(root, "missing"), alias); err != nil {
		t.Fatal(err)
	}
	if _, _, err := validateForkDataPathsAs("", t.TempDir(), alias); err == nil {
		t.Fatal("accepted unresolved destination alias")
	}
}

func TestForkMetadataRequiresRecordedTarget(t *testing.T) {
	dbsu := withCurrentUserAsDBSU(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "PG_VERSION"), []byte("18"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, metadata := range []string{`{"kind":"pg_fork"}`, `{"kind":"pg_fork","target":null}`, `{"kind":"pg_fork","target":{"data":""}}`} {
		if err := os.WriteFile(filepath.Join(dir, "fork.json"), []byte(metadata), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := requireForkInfo(dbsu, dir); err == nil {
			t.Errorf("incomplete metadata authorized target: %s", metadata)
		}
	}
	// An actual recorded matching target remains valid.
	metadata := fmt.Sprintf(`{"kind":"pg_fork","target":{"data":%q}}`, dir)
	if err := os.WriteFile(filepath.Join(dir, "fork.json"), []byte(metadata), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := requireForkInfo(dbsu, dir); err != nil {
		t.Fatal(err)
	}
}

func TestForkCannotIgnoreInaccessiblePathAlias(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission fixture requires an ordinary uid")
	}
	root := t.TempDir()
	src := filepath.Join(root, "source")
	protected := filepath.Join(root, "protected")
	for _, dir := range []string{src, protected} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	alias := filepath.Join(protected, "source-alias")
	if err := os.Symlink(src, alias); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(protected, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(protected, 0700) })
	if _, _, err := validateForkDataPathsAs("", src, alias); err == nil {
		t.Fatal("unresolved alias of source accepted as unrelated destination")
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
