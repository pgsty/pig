package do

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"pig/internal/config"
	"reflect"
	"strings"
	"testing"
)

func TestBuildPgsqlAddCommandsAppendsReplicaAndRefreshesService(t *testing.T) {
	commands, err := BuildPgsqlAddCommands([]string{"pg-test", "10.10.10.12", "10.10.10.13"})
	if err != nil {
		t.Fatalf("BuildPgsqlAddCommands() error = %v", err)
	}

	want := [][]string{
		{"pgsql.yml", "-l", "10.10.10.13,10.10.10.12,&pg-test"},
		{"pgsql.yml", "-l", "pg-test,!10.10.10.12,!10.10.10.13", "-t", "pg_service"},
	}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("BuildPgsqlAddCommands() = %#v, want %#v", commands, want)
	}
}

func TestBuildPgsqlRmCommandsRemovesReplicaInsteadOfCluster(t *testing.T) {
	commands, err := BuildPgsqlRmCommands([]string{"pg-test", "10.10.10.13"}, true)
	if err != nil {
		t.Fatalf("BuildPgsqlRmCommands() error = %v", err)
	}

	want := [][]string{
		{"pgsql-rm.yml", "-l", "10.10.10.13,&pg-test", "-e", "pg_rm_pkg=true"},
	}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("BuildPgsqlRmCommands() = %#v, want %#v", commands, want)
	}
}

func TestBuildNodeCommandsAcceptMultipleSelectors(t *testing.T) {
	addCommands, err := BuildNodeAddCommands([]string{"10.10.10.10", "10.10.10.11"})
	if err != nil {
		t.Fatalf("BuildNodeAddCommands() error = %v", err)
	}
	if want := [][]string{{"node.yml", "-l", "10.10.10.10,10.10.10.11"}}; !reflect.DeepEqual(addCommands, want) {
		t.Fatalf("BuildNodeAddCommands() = %#v, want %#v", addCommands, want)
	}

	rmCommands, err := BuildNodeRmCommands([]string{"pg-test", "10.10.10.11"})
	if err != nil {
		t.Fatalf("BuildNodeRmCommands() error = %v", err)
	}
	if want := [][]string{{"node-rm.yml", "-l", "pg-test,10.10.10.11"}}; !reflect.DeepEqual(rmCommands, want) {
		t.Fatalf("BuildNodeRmCommands() = %#v, want %#v", rmCommands, want)
	}
}

func TestBuildNodeRepoCommandsRejectsTooManyArgs(t *testing.T) {
	if _, err := BuildNodeRepoCommands([]string{"pg-test", "node", "extra"}); err == nil {
		t.Fatal("BuildNodeRepoCommands() should reject more than two arguments")
	}
}

func TestBuildRedisCommandsValidatePortsAndUninstall(t *testing.T) {
	if _, err := BuildRedisAddCommands([]string{"redis-test", "6379"}); err == nil {
		t.Fatal("BuildRedisAddCommands() should require an IP selector when ports are specified")
	}
	if _, err := BuildRedisAddCommands([]string{"10.10.10.10", "80"}); err == nil {
		t.Fatal("BuildRedisAddCommands() should reject invalid redis port")
	}

	commands, err := BuildRedisRmCommands([]string{"10.10.10.10"}, true)
	if err != nil {
		t.Fatalf("BuildRedisRmCommands() error = %v", err)
	}
	want := [][]string{{"redis-rm.yml", "-l", "10.10.10.10", "-e", "redis_rm_pkg=true"}}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("BuildRedisRmCommands() = %#v, want %#v", commands, want)
	}

	if _, err := BuildRedisRmCommands([]string{"10.10.10.10", "6379"}, true); err == nil {
		t.Fatal("BuildRedisRmCommands() should reject --uninstall for per-port removal")
	}
}

func TestSimplePlaybookBuilders(t *testing.T) {
	tests := []struct {
		name  string
		build PlaybookBuilder
		args  []string
		want  [][]string
	}{
		{"pgsql-user", BuildPgsqlUserCommands, []string{"pg-meta", "dbuser_meta", "--check"}, [][]string{{"pgsql-user.yml", "-l", "pg-meta", "-e", `{"username":"dbuser_meta"}`, "--check"}}},
		{"pgsql-db", BuildPgsqlDBCommands, []string{"pg-meta", "meta"}, [][]string{{"pgsql-db.yml", "-l", "pg-meta", "-e", `{"dbname":"meta"}`}}},
		{"pgsql-ext", BuildPgsqlExtCommands, []string{"pg-meta", "postgis", "vector"}, [][]string{{"pgsql.yml", "-l", "pg-meta", "-t", "pg_extension", "-e", `{"pg_extensions":["postgis,vector"]}`}}},
		{"pgsql-hba", BuildPgsqlHBACommands, []string{"pg-meta"}, [][]string{{"pgsql.yml", "-l", "pg-meta", "-t", "pg_hba,pg_reload,pgbouncer_hba,pgbouncer_reload"}}},
		{"pgsql-svc", BuildPgsqlServiceCommands, []string{"pg-meta"}, [][]string{{"pgsql.yml", "-l", "pg-meta", "-t", "pg_service"}}},
		{"pgmon-add", BuildPgmonAddCommands, []string{"pg-meta"}, [][]string{{"pgsql-monitor.yml", "-e", `{"clsname":"pg-meta"}`}}},
		{"node-pkg", BuildNodePackageCommands, []string{"all", "duckdb", "restic"}, [][]string{{"node.yml", "-l", "all", "-t", "node_pkg_extra", "-e", `{"node_packages":["duckdb,restic"]}`}}},
		{"repo-build", BuildRepoBuildCommands, nil, [][]string{{"infra.yml", "-l", "infra", "-t", "repo_build"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.build(tt.args)
			if err != nil {
				t.Fatalf("builder returned error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("builder result = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestScalarExtraVarsAndClusterNamesRejectInjection(t *testing.T) {
	for name, build := range map[string]PlaybookBuilder{
		"user":  BuildPgsqlUserCommands,
		"db":    BuildPgsqlDBCommands,
		"pgmon": BuildPgmonAddCommands,
	} {
		t.Run(name, func(t *testing.T) {
			args := []string{"pg-meta", "value is_admin=true"}
			if name == "pgmon" {
				args = []string{"pg-meta state=absent"}
			}
			if _, err := build(args); err == nil {
				t.Fatal("injected scalar or cluster name was accepted")
			}
		})
	}
	if err := RemovePgmon("", []string{"../../etc/passwd"}); err == nil {
		t.Fatal("RemovePgmon accepted an unsafe cluster path")
	}
}

func TestNodeRepoModuleUsesJSONScalar(t *testing.T) {
	commands, err := BuildNodeRepoCommands([]string{"all", "node is_admin=true"})
	if err != nil {
		t.Fatal(err)
	}
	encoded := commands[0][len(commands[0])-1]
	var decoded map[string]string
	if err := json.Unmarshal([]byte(encoded), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["node_repo_modules"] != "node is_admin=true" || len(decoded) != 1 {
		t.Fatalf("decoded scalar = %#v", decoded)
	}
}

func TestEncodeCommaListExtraVarPreservesOrdinaryOutput(t *testing.T) {
	got, err := encodeCommaListExtraVar("pg_extensions", []string{"postgis", "vector&search<>"})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"pg_extensions":["postgis,vector&search<>"]}`
	if got != want {
		t.Fatalf("encoded extra var = %q, want byte-identical %q", got, want)
	}
}

func TestListExtraVarBuildersEscapeJSONInjection(t *testing.T) {
	injected := `x"],"become":true,"z":["y`
	tests := []struct {
		name  string
		build PlaybookBuilder
		key   string
		args  []string
	}{
		{name: "pgsql extensions", build: BuildPgsqlExtCommands, key: "pg_extensions", args: []string{"pg-meta", `quote"value`, `path\pkg`, injected}},
		{name: "node packages", build: BuildNodePackageCommands, key: "node_packages", args: []string{"all", `quote"value`, `path\pkg`, injected}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			commands, err := tt.build(tt.args)
			if err != nil {
				t.Fatal(err)
			}
			command := commands[0]
			encoded := command[len(command)-1]
			if !json.Valid([]byte(encoded)) {
				t.Fatalf("builder emitted invalid JSON: %q", encoded)
			}

			var decoded map[string][]string
			if err := json.Unmarshal([]byte(encoded), &decoded); err != nil {
				t.Fatal(err)
			}
			if len(decoded) != 1 {
				t.Fatalf("decoded extra vars = %#v, want exactly one key", decoded)
			}
			values, ok := decoded[tt.key]
			if !ok || len(values) != 1 {
				t.Fatalf("decoded %s = %#v, want one string value", tt.key, values)
			}
			if want := strings.Join(tt.args[1:], ","); values[0] != want {
				t.Fatalf("decoded value = %q, want %q", values[0], want)
			}
		})
	}
}

func TestSingleSelectorBuildersRejectExtraArgs(t *testing.T) {
	for name, build := range map[string]PlaybookBuilder{
		"pgsql-hba": BuildPgsqlHBACommands,
		"pgsql-svc": BuildPgsqlServiceCommands,
		"pgmon-add": BuildPgmonAddCommands,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := build([]string{"pg-meta", "ignored"}); err == nil {
				t.Fatal("builder should reject an ignored positional argument")
			}
		})
	}
	if err := RemovePgmon("", []string{"pg-meta", "ignored"}); err == nil {
		t.Fatal("RemovePgmon should reject an ignored positional argument")
	}
}

func TestRunAnsibleUsesAnsibleFromPigstyHome(t *testing.T) {
	home, _, caller, capture := setupAnsibleRunnerTest(t, "ansible")
	before, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if before != caller {
		t.Fatalf("test cwd = %q, want %q", before, caller)
	}

	args := []string{"all", "-m", "ping"}
	if err := RunAnsible("inventory.yml", args); err != nil {
		t.Fatalf("RunAnsible() error = %v", err)
	}
	after, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("process cwd changed from %q to %q", before, after)
	}

	assertAnsibleCapture(t, capture, home, append([]string{"-i", "inventory.yml"}, args...))
}

func TestRunPlaybookUsesAnsiblePlaybookWithoutChangingCWD(t *testing.T) {
	home, inventory, caller, capture := setupAnsibleRunnerTest(t, "ansible-playbook")
	before, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if before != caller {
		t.Fatalf("test cwd = %q, want %q", before, caller)
	}

	args := []string{"site.yml", "--check"}
	if err := RunPlaybook(inventory, args); err != nil {
		t.Fatalf("RunPlaybook() error = %v", err)
	}
	after, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("process cwd changed from %q to %q", before, after)
	}

	assertAnsibleCapture(t, capture, home, append([]string{"-i", inventory}, args...))
}

func TestAnsibleRunnersCheckActualBinary(t *testing.T) {
	tests := []struct {
		name    string
		run     func(string, []string) error
		wantErr string
	}{
		{name: "ansible", run: RunAnsible, wantErr: "ansible command not found"},
		{name: "ansible-playbook", run: RunPlaybook, wantErr: "ansible-playbook command not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, inventory, _, _ := setupAnsibleRunnerTest(t)
			err := tt.run(inventory, []string{"--version"})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("runner error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestAnsibleRunnerPrecheckErrors(t *testing.T) {
	t.Run("empty home", func(t *testing.T) {
		_, inventory, _, _ := setupAnsibleRunnerTest(t, "ansible", "ansible-playbook")
		config.PigstyHome = ""
		err := RunAnsible(inventory, nil)
		if err == nil || !strings.Contains(err.Error(), "pigsty home not found") {
			t.Fatalf("RunAnsible() error = %v, want missing home", err)
		}
	})

	t.Run("missing home", func(t *testing.T) {
		home, inventory, _, _ := setupAnsibleRunnerTest(t, "ansible", "ansible-playbook")
		config.PigstyHome = filepath.Join(home, "missing")
		err := RunAnsible(inventory, nil)
		if err == nil || !strings.Contains(err.Error(), "pigsty home") || !strings.Contains(err.Error(), "not found") {
			t.Fatalf("RunAnsible() error = %v, want missing home path", err)
		}
	})

	t.Run("relative inventory", func(t *testing.T) {
		setupAnsibleRunnerTest(t, "ansible", "ansible-playbook")
		err := RunAnsible("missing.yml", nil)
		if err == nil || !strings.Contains(err.Error(), "inventory") || !strings.Contains(err.Error(), "missing.yml") {
			t.Fatalf("RunAnsible() error = %v, want missing relative inventory", err)
		}
	})

	t.Run("default inventory", func(t *testing.T) {
		setupAnsibleRunnerTest(t, "ansible", "ansible-playbook")
		config.PigstyConfig = ""
		err := RunAnsible("", nil)
		if err == nil || !strings.Contains(err.Error(), "pigsty config inventory not found") {
			t.Fatalf("RunAnsible() error = %v, want missing default inventory", err)
		}
	})
}

func TestRunAnsiblePreservesStructuredOutputStreams(t *testing.T) {
	_, inventory, _, capture := setupAnsibleRunnerTest(t, "ansible", "ansible-playbook")
	binDir := filepath.Join(filepath.Dir(capture), "bin")
	writeFakeAnsibleTool(t, binDir, "ansible", `#!/bin/sh
printf 'stdout-marker\n'
printf 'stderr-marker\n' >&2
`)
	config.OutputFormat = config.OUTPUT_JSON

	stdout, stderr, runErr := captureAnsibleOutput(t, func() error {
		return RunAnsible(inventory, []string{"all", "-m", "ping"})
	})
	if runErr != nil {
		t.Fatalf("RunAnsible() error = %v", runErr)
	}
	if stdout != "" {
		t.Fatalf("structured command stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "stdout-marker\n") || !strings.Contains(stderr, "stderr-marker\n") {
		t.Fatalf("structured command stderr = %q, want both command streams", stderr)
	}
}

func setupAnsibleRunnerTest(t *testing.T, binaries ...string) (home, inventory, caller, capture string) {
	t.Helper()
	originalHome := config.PigstyHome
	originalConfig := config.PigstyConfig
	originalOutput := config.OutputFormat
	t.Cleanup(func() {
		config.PigstyHome = originalHome
		config.PigstyConfig = originalConfig
		config.OutputFormat = originalOutput
	})

	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	home = filepath.Join(root, "pigsty")
	caller = filepath.Join(root, "caller")
	for _, dir := range []string{binDir, home, caller} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	inventory = filepath.Join(home, "inventory.yml")
	if err := os.WriteFile(inventory, []byte("all: {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	capture = filepath.Join(root, "capture")
	t.Setenv("PIG_TEST_CAPTURE", capture)
	t.Setenv("PATH", binDir)
	for _, binary := range binaries {
		writeFakeAnsibleTool(t, binDir, binary, `#!/bin/sh
printf '%s\n' "$PWD" "$@" > "$PIG_TEST_CAPTURE"
`)
	}
	config.PigstyHome = home
	config.PigstyConfig = inventory
	t.Chdir(caller)
	return home, inventory, caller, capture
}

func writeFakeAnsibleTool(t *testing.T, binDir, name, script string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(binDir, name), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
}

func assertAnsibleCapture(t *testing.T, capture, workDir string, args []string) {
	t.Helper()
	raw, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	if len(lines) == 0 {
		t.Fatal("fake ansible capture is empty")
	}
	gotDir, err := os.Stat(lines[0])
	if err != nil {
		t.Fatal(err)
	}
	wantDir, err := os.Stat(workDir)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(gotDir, wantDir) {
		t.Fatalf("command cwd = %q, want %q", lines[0], workDir)
	}
	if !reflect.DeepEqual(lines[1:], args) {
		t.Fatalf("command args = %#v, want %#v", lines[1:], args)
	}
}

func captureAnsibleOutput(t *testing.T, run func() error) (stdout, stderr string, runErr error) {
	t.Helper()
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		_ = stdoutR.Close()
		_ = stdoutW.Close()
		t.Fatal(err)
	}
	originalStdout := os.Stdout
	originalStderr := os.Stderr
	os.Stdout = stdoutW
	os.Stderr = stderrW
	runErr = run()
	_ = stdoutW.Close()
	_ = stderrW.Close()
	os.Stdout = originalStdout
	os.Stderr = originalStderr
	stdoutBytes, stdoutErr := io.ReadAll(stdoutR)
	stderrBytes, stderrErr := io.ReadAll(stderrR)
	_ = stdoutR.Close()
	_ = stderrR.Close()
	if stdoutErr != nil {
		t.Fatal(stdoutErr)
	}
	if stderrErr != nil {
		t.Fatal(stderrErr)
	}
	return string(stdoutBytes), string(stderrBytes), runErr
}
