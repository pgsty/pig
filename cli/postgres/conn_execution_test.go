package postgres

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"pig/cli/ext"
	"pig/internal/config"
	"pig/internal/utils"
)

// Keep every PostgreSQL binary and privilege switch inside the fixture, even if
// the host has a real installation. Shell utilities only operate on temp files.
func installFakePostgresCommands(t *testing.T, scripts map[string]string) string {
	t.Helper()
	bin := t.TempDir()
	for _, name := range []string{"psql", "pg_ctl", "pg_controldata", "ps", "systemctl", "sudo", "su"} {
		script, ok := scripts[name]
		if !ok {
			script = "exit 99\n"
		}
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	oldInit, oldActive, oldPG, oldInstalls := ext.Inited, ext.Active, ext.Postgres, ext.Installs
	t.Cleanup(func() { ext.Inited, ext.Active, ext.Postgres, ext.Installs = oldInit, oldActive, oldPG, oldInstalls })
	ext.Inited, ext.Active = true, &ext.PostgresInstall{BinPath: bin, MajorVersion: 18}
	ext.Postgres, ext.Installs = ext.Active, map[int]*ext.PostgresInstall{18: ext.Active}
	return bin
}

func TestPsqlTextPreservesNativeFileDescriptors(t *testing.T) {
	dbsu := withCurrentUserAsDBSU(t)
	installFakePostgresCommands(t, map[string]string{"psql": `exec "$PIG_TEST_EXECUTABLE" -test.run '^TestPsqlNativeFileDescriptorsHelper$'`})
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PIG_TEST_EXECUTABLE", executable)
	t.Setenv("PIG_TEST_NATIVE_FDS", "1")
	oldIn, oldOut, oldErr, oldFormat := os.Stdin, os.Stdout, os.Stderr, config.OutputFormat
	t.Cleanup(func() { os.Stdin, os.Stdout, os.Stderr, config.OutputFormat = oldIn, oldOut, oldErr, oldFormat })
	config.OutputFormat = config.OUTPUT_TEXT
	t.Setenv("PIG_NON_INTERACTIVE", "")
	for _, fixture := range []struct {
		name string
		dest **os.File
	}{{"STDIN", &os.Stdin}, {"STDOUT", &os.Stdout}, {"STDERR", &os.Stderr}} {
		file, err := os.CreateTemp(t.TempDir(), fixture.name)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = file.Close() })
		*fixture.dest = file
		t.Setenv("PIG_TEST_"+fixture.name, file.Name())
	}
	if _, err := os.Stdin.WriteString("native input\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stdin.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	err = Psql(&Config{DbSU: dbsu, PgVersion: 18}, "postgres", nil)
	if utils.ExitCode(err) != 7 || !utils.IsSilentExit(err) {
		t.Fatalf("native descriptors/exit lost: %v", err)
	}
	for _, fixture := range []struct {
		file *os.File
		want string
	}{{os.Stdout, "out:native input"}, {os.Stderr, "err:native input"}} {
		data, err := os.ReadFile(fixture.file.Name())
		// PIG prints the command hint to stderr before the native program.
		if err != nil || !strings.HasSuffix(string(data), fixture.want) {
			t.Fatalf("stream = %q, %v; want %q", data, err, fixture.want)
		}
	}
}

func TestPsqlNativeFileDescriptorsHelper(t *testing.T) {
	if os.Getenv("PIG_TEST_NATIVE_FDS") != "1" {
		return
	}
	for i, file := range []*os.File{os.Stdin, os.Stdout, os.Stderr} {
		name := []string{"STDIN", "STDOUT", "STDERR"}[i]
		actual, err := file.Stat()
		if err != nil {
			os.Exit(90 + i)
		}
		original, err := os.Stat(os.Getenv("PIG_TEST_" + name))
		if err != nil || !os.SameFile(actual, original) {
			os.Exit(90 + i)
		}
	}
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(93)
	}
	fmt.Fprint(os.Stdout, "out:"+strings.TrimSpace(string(input)))
	fmt.Fprint(os.Stderr, "err:"+strings.TrimSpace(string(input)))
	os.Exit(7)
}

func TestPsqlFileStopsOnErrorInEveryOutputMode(t *testing.T) {
	dbsu := withCurrentUserAsDBSU(t)
	installFakePostgresCommands(t, map[string]string{"psql": `
for arg do
  if [ "$arg" = 'ON_ERROR_STOP=1' ]; then
    printf 'script failed\n' >&2
    exit 3
  fi
done
exit 0
`})
	oldFormat := config.OutputFormat
	t.Cleanup(func() { config.OutputFormat = oldFormat })
	for _, mode := range []string{config.OUTPUT_TEXT, config.OUTPUT_JSON, config.OUTPUT_YAML} {
		t.Run(mode, func(t *testing.T) {
			config.OutputFormat = mode
			for _, file := range []string{"script.sql", "-"} {
				err := Psql(&Config{DbSU: dbsu, PgVersion: 18}, "postgres", &PsqlOptions{File: file})
				if utils.ExitCode(err) != 3 {
					t.Fatalf("file %q returned %v, want SQL script failure exit 3", file, err)
				}
			}
		})
	}
	for _, opts := range []*PsqlOptions{nil, {Command: "SELECT 1"}} {
		args, err := buildPsqlArgs("fake-psql", nil, "", opts)
		if err != nil || strings.Contains(strings.Join(args, " "), "ON_ERROR_STOP") {
			t.Fatalf("non-file psql options changed: %q, %v", args, err)
		}
	}
}

func TestPsqlRelativeFileSurvivesSuLoginDirectory(t *testing.T) {
	withCurrentUserAsDBSU(t)
	config.CurrentUser = "root"
	installFakePostgresCommands(t, map[string]string{
		"su": `
test "$1" = '-' && test "$2" = 'fixture-dbsu' && test "$3" = '-c' || exit 90
cd "$PIG_TEST_LOGIN_HOME" || exit 91
exec /bin/sh -c "$4"
`,
		"psql": `
while [ "$#" -gt 0 ]; do
  if [ "$1" = '-f' ]; then cat "$2" > "$PIG_TEST_FILE_CONTENT"; exit $?; fi
  shift
done
exit 92
`,
	})
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(t.TempDir(), "script '$.sql")
	if err := os.WriteFile(input, []byte("caller script\n"), 0600); err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(cwd, input)
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "received")
	t.Setenv("PIG_TEST_LOGIN_HOME", t.TempDir())
	t.Setenv("PIG_TEST_FILE_CONTENT", output)
	if err := Psql(&Config{DbSU: "fixture-dbsu", PgVersion: 18}, "", &PsqlOptions{File: relative}); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(output); err != nil || string(data) != "caller script\n" {
		t.Fatalf("relative file resolved from wrong directory: %q, %v", data, err)
	}
}

func TestGetAllDatabasesIgnoresPsqlrcAndStderr(t *testing.T) {
	dbsu := withCurrentUserAsDBSU(t)
	installFakePostgresCommands(t, map[string]string{"psql": `
skiprc=false
for arg do
  if [ "$arg" = '-X' ]; then skiprc=true; fi
done
if [ "$skiprc" = false ]; then printf 'psqlrc banner\n'; fi
printf 'app\nreport\n'
printf 'diagnostic\n' >&2
`})
	dbs, err := GetAllDatabases(&Config{DbSU: dbsu, PgVersion: 18})
	if err != nil || !reflect.DeepEqual(dbs, []string{"app", "report"}) {
		t.Fatalf("database list polluted: %q, %v", dbs, err)
	}
}
