package cmdb

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareRuntimeCreatesPrivateVerifiedAdapter(t *testing.T) {
	home, root, pig := runtimeFixture(t)
	record, err := PrepareRuntime(RuntimeOptions{
		PigstyHome: home, RuntimeRoot: root, PigBinary: pig,
		ConnectionSource: ConnectionMetaSource,
	})
	if err != nil {
		t.Fatal(err)
	}

	digest := sha256.Sum256([]byte(home))
	wantKey := hex.EncodeToString(digest[:])[:16]
	if record.Kind != RuntimeRecordKind || record.Home != home || record.HomeKey != wantKey ||
		record.ConnectionSource != ConnectionMetaSource || record.ConnectionFile != "" {
		t.Fatalf("unexpected runtime record: %+v", record)
	}
	runtimePath := filepath.Join(root, wantKey, "runtime.json")
	if got := filepath.Dir(record.AdapterPath); got != filepath.Dir(runtimePath) {
		t.Fatalf("adapter directory = %q, want %q", got, filepath.Dir(runtimePath))
	}
	assertRuntimeMode(t, root, 0o700, true)
	assertRuntimeMode(t, filepath.Dir(runtimePath), 0o700, true)
	assertRuntimeMode(t, filepath.Join(filepath.Dir(runtimePath), "backups"), 0o700, true)
	assertRuntimeMode(t, runtimePath, 0o600, false)
	assertRuntimeMode(t, record.AdapterPath, 0o700, false)

	verified, err := VerifyRuntimeFile(runtimePath, pig)
	if err != nil || verified != record {
		t.Fatalf("runtime verification = %+v, %v", verified, err)
	}
	wrapper, err := os.ReadFile(record.AdapterPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(wrapper), "# "+RuntimeMarker) ||
		!strings.Contains(string(wrapper), "sty cmdb _inventory \"$@\"") {
		t.Fatalf("adapter protocol missing: %s", wrapper)
	}

	capture := filepath.Join(filepath.Dir(root), "capture")
	runRuntimeAdapter(t, record.AdapterPath, capture, "--list")
	want := strings.Join([]string{runtimePath, "sty", "cmdb", "_inventory", "--list", ""}, "\n")
	if got, err := os.ReadFile(capture); err != nil || string(got) != want {
		t.Fatalf("list forwarding = %q, %v; want %q", got, err, want)
	}
	runRuntimeAdapter(t, record.AdapterPath, capture, "--host", "db-a")
	want = strings.Join([]string{runtimePath, "sty", "cmdb", "_inventory", "--host", "db-a", ""}, "\n")
	if got, err := os.ReadFile(capture); err != nil || string(got) != want {
		t.Fatalf("host forwarding = %q, %v; want %q", got, err, want)
	}
	if err := os.Remove(capture); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(record.AdapterPath, "--list", "extra")
	command.Env = append(os.Environ(), "PIG_RUNTIME_CAPTURE="+capture)
	if err := command.Run(); err == nil {
		t.Fatal("invalid adapter grammar executed the Pig binary")
	} else {
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) || exitError.ExitCode() != 2 {
			t.Fatalf("invalid grammar exit = %v", err)
		}
	}
	if _, err := os.Lstat(capture); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid grammar reached Pig binary: %v", err)
	}
}

func TestRuntimeDetectsAndInitRefreshesBinaryAndAdapter(t *testing.T) {
	home, root, pig := runtimeFixture(t)
	record, err := PrepareRuntime(RuntimeOptions{
		PigstyHome: home, RuntimeRoot: root, PigBinary: pig,
		ConnectionSource: ConnectionMetaSource,
	})
	if err != nil {
		t.Fatal(err)
	}
	runtimePath := filepath.Join(filepath.Dir(record.AdapterPath), "runtime.json")

	other := filepath.Join(filepath.Dir(pig), "other-pig")
	writeRuntimeExecutable(t, other, runtimePigScript)
	if _, err := VerifyRuntimeFile(runtimePath, other); !IsCode(err, "CMDB_RUNTIME_PIG_INVALID") {
		t.Fatalf("moved current binary error = %v", err)
	}

	writeRuntimeExecutable(t, pig, runtimePigScript+"# replaced\n")
	if _, err := VerifyRuntimeFile(runtimePath, pig); !IsCode(err, "CMDB_RUNTIME_PIG_INVALID") {
		t.Fatalf("replaced binary error = %v", err)
	}
	refreshed, err := PrepareRuntime(RuntimeOptions{
		PigstyHome: home, RuntimeRoot: root, PigBinary: pig,
		ConnectionSource: ConnectionMetaSource,
	})
	if err != nil || refreshed.PigSHA256 == record.PigSHA256 {
		t.Fatalf("binary refresh = %+v, %v", refreshed, err)
	}

	if err := os.WriteFile(refreshed.AdapterPath, []byte("#!/bin/sh\n# altered\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyRuntimeFile(runtimePath, pig); !IsCode(err, "CMDB_RUNTIME_ADAPTER_INVALID") {
		t.Fatalf("altered adapter error = %v", err)
	}
	refreshed, err = PrepareRuntime(RuntimeOptions{
		PigstyHome: home, RuntimeRoot: root, PigBinary: pig,
		ConnectionSource: ConnectionMetaSource,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyRuntimeFile(runtimePath, pig); err != nil {
		t.Fatalf("refreshed adapter did not verify: %v", err)
	}
}

func TestPrepareRuntimeFailsClosedOnUnclaimedDirectoryAndUnsafeRoot(t *testing.T) {
	home, root, pig := runtimeFixture(t)
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(home))
	key := hex.EncodeToString(digest[:])[:16]
	if err := os.Mkdir(filepath.Join(root, key), 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := PrepareRuntime(RuntimeOptions{
		PigstyHome: home, RuntimeRoot: root, PigBinary: pig,
		ConnectionSource: ConnectionMetaSource,
	})
	if !IsCode(err, "CMDB_RUNTIME_COLLISION") {
		t.Fatalf("unclaimed key directory error = %v", err)
	}

	unsafeRoot := filepath.Join(filepath.Dir(root), "unsafe")
	if err := os.Mkdir(unsafeRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err = PrepareRuntime(RuntimeOptions{
		PigstyHome: home, RuntimeRoot: unsafeRoot, PigBinary: pig,
		ConnectionSource: ConnectionMetaSource,
	})
	if !IsCode(err, "CMDB_RUNTIME_PERMISSION") {
		t.Fatalf("public runtime root error = %v", err)
	}

	target := filepath.Join(filepath.Dir(root), "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(filepath.Dir(root), "linked")
	if err := os.Symlink(target, linked); err != nil {
		t.Fatal(err)
	}
	_, err = PrepareRuntime(RuntimeOptions{
		PigstyHome: home, RuntimeRoot: filepath.Join(linked, "cmdb"), PigBinary: pig,
		ConnectionSource: ConnectionMetaSource,
	})
	if !IsCode(err, "CMDB_RUNTIME_INVALID") {
		t.Fatalf("symlinked runtime ancestor error = %v", err)
	}
	if _, statErr := os.Lstat(filepath.Join(target, "cmdb")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("unsafe root was mutated before rejection: %v", statErr)
	}
}

func TestRuntimeConnectionResolutionIsExactAndSecretSafe(t *testing.T) {
	secret := "runtime-secret-value"
	t.Setenv(connectionEnv, secret)
	meta, err := ResolveRuntimeConnection(RuntimeRecord{ConnectionSource: ConnectionMetaSource})
	if err != nil || meta.Source() != ConnectionMetaSource || meta.value != "service=meta" {
		t.Fatalf("service source fell through to environment: source=%q err=%v", meta.Source(), err)
	}
	environment, err := ResolveRuntimeConnection(RuntimeRecord{ConnectionSource: ConnectionEnvSource})
	if err != nil || environment.Source() != ConnectionEnvSource || environment.value != secret {
		t.Fatalf("environment source = %q, %v", environment.Source(), err)
	}
	t.Setenv(connectionEnv, "")
	if _, err := ResolveRuntimeConnection(RuntimeRecord{ConnectionSource: ConnectionEnvSource}); !IsCode(err, "CMDB_RUNTIME_CONNECTION_INVALID") || strings.Contains(err.Error(), secret) {
		t.Fatalf("missing environment error = %v", err)
	}

	directory := symlinkFreeTempDir(t)
	connectionFile := filepath.Join(directory, "connection")
	if err := os.WriteFile(connectionFile, []byte(secret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := ResolveRuntimeConnection(RuntimeRecord{
		ConnectionSource: ConnectionFileSource, ConnectionFile: connectionFile,
	})
	if err != nil || file.Source() != ConnectionFileSource || file.value != secret {
		t.Fatalf("file source = %q, %v", file.Source(), err)
	}
	if err := os.Chmod(connectionFile, 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveRuntimeConnection(RuntimeRecord{
		ConnectionSource: ConnectionFileSource, ConnectionFile: connectionFile,
	}); !IsCode(err, "CMDB_RUNTIME_CONNECTION_INVALID") || strings.Contains(err.Error(), secret) {
		t.Fatalf("unsafe connection error = %v", err)
	}
}

func TestRuntimeRecordIsCanonicalAndContainsNoConnectionValue(t *testing.T) {
	home, root, pig := runtimeFixture(t)
	secret := "postgres://owner:runtime-password@example/cmdb"
	t.Setenv(connectionEnv, secret)
	record, err := PrepareRuntime(RuntimeOptions{
		PigstyHome: home, RuntimeRoot: root, PigBinary: pig,
		ConnectionSource: ConnectionEnvSource,
	})
	if err != nil {
		t.Fatal(err)
	}
	runtimePath := filepath.Join(filepath.Dir(record.AdapterPath), "runtime.json")
	runtimeBody, err := os.ReadFile(runtimePath)
	if err != nil {
		t.Fatal(err)
	}
	wrapperBody, err := os.ReadFile(record.AdapterPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(runtimeBody), secret) || strings.Contains(string(wrapperBody), secret) {
		t.Fatal("runtime artifacts contain a connection value")
	}
	if err := os.WriteFile(runtimePath, append(runtimeBody, ' '), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyRuntimeFile(runtimePath, pig); !IsCode(err, "CMDB_RUNTIME_INVALID") {
		t.Fatalf("non-canonical runtime record error = %v", err)
	}
}

const runtimePigScript = `#!/bin/sh
: "${PIG_RUNTIME_CAPTURE:?}"
{
  printf '%s\n' "$PIGSTY_CMDB_RUNTIME"
  for argument in "$@"; do
    printf '%s\n' "$argument"
  done
} > "$PIG_RUNTIME_CAPTURE"
`

func runtimeFixture(t *testing.T) (string, string, string) {
	t.Helper()
	directory := symlinkFreeTempDir(t)
	home := filepath.Join(directory, "pigsty-home")
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatal(err)
	}
	pig := filepath.Join(directory, "pig'binary")
	writeRuntimeExecutable(t, pig, runtimePigScript)
	return home, filepath.Join(directory, "runtime"), pig
}

func writeRuntimeExecutable(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		t.Fatal(err)
	}
}

func runRuntimeAdapter(t *testing.T, adapter, capture string, arguments ...string) {
	t.Helper()
	command := exec.Command(adapter, arguments...)
	command.Env = append(os.Environ(), "PIG_RUNTIME_CAPTURE="+capture)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("adapter execution failed: %v: %s", err, output)
	}
}

func assertRuntimeMode(t *testing.T, path string, mode os.FileMode, directory bool) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != mode || info.IsDir() != directory {
		t.Fatalf("%s mode/type = %v/%t, want %v/%t", path, info.Mode().Perm(), info.IsDir(), mode, directory)
	}
}
