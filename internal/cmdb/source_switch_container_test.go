package cmdb

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestContainerSourceSwitchEnableDisableRoundTrip(t *testing.T) {
	store := openContainerStore(t)
	resetContainerSchema(t, store)
	if _, err := store.Init(t.Context(), "test-v1"); err != nil {
		t.Fatal(err)
	}
	fixture := newContainerSourceSwitchFixture(t)
	static := appendContainerSwitchHead(t, store, fixture.staticPath)

	enabled, err := store.SwitchSource(t.Context(), SourceSwitchRequest{
		Operation: SourceSwitchEnable, PigstyHome: fixture.home,
		RuntimePath: fixture.runtimePath, CurrentPig: fixture.pig,
		intentID: strings.Repeat("a", 32),
	})
	if err != nil || !enabled.Changed || enabled.NoOp || enabled.State != SourceSwitchApplied ||
		!enabled.Head.Equal(static.Token) || enabled.IntentPath == "" || enabled.BackupPath == "" {
		t.Fatalf("enable result = %+v, %v", enabled, err)
	}
	config, err := InspectAnsibleConfig(fixture.configPath)
	if err != nil || config.Locator != fixture.runtime.AdapterPath {
		t.Fatalf("enabled config = %+v, %v", config, err)
	}
	backup, err := os.ReadFile(enabled.BackupPath)
	if err != nil || !bytes.Equal(backup, fixture.originalConfig) {
		t.Fatalf("enable backup = %q, %v", backup, err)
	}

	noOp, err := store.SwitchSource(t.Context(), SourceSwitchRequest{
		Operation: SourceSwitchEnable, PigstyHome: fixture.home,
		RuntimePath: fixture.runtimePath, CurrentPig: fixture.pig,
	})
	if err != nil || !noOp.NoOp || noOp.Changed {
		t.Fatalf("already-enabled result = %+v, %v", noOp, err)
	}

	// Source-only changes remain semantically eligible as a disable fallback.
	staticBody, err := os.ReadFile(fixture.staticPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixture.staticPath, append([]byte("# source-only change\n"), staticBody...), 0o600); err != nil {
		t.Fatal(err)
	}
	disabled, err := store.SwitchSource(t.Context(), SourceSwitchRequest{
		Operation: SourceSwitchDisable, PigstyHome: fixture.home,
		RuntimePath: fixture.runtimePath, CurrentPig: fixture.pig,
		intentID: strings.Repeat("b", 32),
	})
	if err != nil || !disabled.Changed || disabled.NoOp || disabled.State != SourceSwitchApplied ||
		disabled.StaticLocator != fixture.staticPath {
		t.Fatalf("disable result = %+v, %v", disabled, err)
	}
	current, err := os.ReadFile(fixture.configPath)
	if err != nil || !bytes.Equal(current, fixture.originalConfig) {
		t.Fatalf("disable did not restore exact ansible.cfg: %v, %q", err, current)
	}
	if _, err := os.Lstat(enabled.IntentPath); err != nil {
		t.Fatalf("enable intent was not retained: %v", err)
	}
	noOp, err = store.SwitchSource(t.Context(), SourceSwitchRequest{
		Operation: SourceSwitchDisable, PigstyHome: fixture.home,
		RuntimePath: fixture.runtimePath, CurrentPig: fixture.pig,
	})
	if err != nil || !noOp.NoOp || noOp.Changed {
		t.Fatalf("already-disabled result = %+v, %v", noOp, err)
	}
}

func TestContainerSourceSwitchRejectsParityAndCompletesPendingIntent(t *testing.T) {
	store := openContainerStore(t)
	resetContainerSchema(t, store)
	if _, err := store.Init(t.Context(), "test-v1"); err != nil {
		t.Fatal(err)
	}
	fixture := newContainerSourceSwitchFixture(t)
	appendContainerSwitchHead(t, store, fixture.staticPath)
	original, err := os.ReadFile(fixture.configPath)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(fixture.staticPath, []byte("all:\n  children: {}\n  vars: { admin_ip: 127.0.0.1, generation: 2 }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SwitchSource(t.Context(), SourceSwitchRequest{
		Operation: SourceSwitchEnable, PigstyHome: fixture.home,
		RuntimePath: fixture.runtimePath, CurrentPig: fixture.pig,
	}); !IsCode(err, "CMDB_SOURCE_HEAD_MISMATCH") {
		t.Fatalf("semantic drift enable error = %v", err)
	}
	if current, err := os.ReadFile(fixture.configPath); err != nil || !bytes.Equal(current, original) {
		t.Fatalf("parity refusal changed config: %v, %q", err, current)
	}
	if err := os.WriteFile(fixture.staticPath, fixture.staticBody, 0o600); err != nil {
		t.Fatal(err)
	}

	request := SourceSwitchRequest{
		Operation: SourceSwitchEnable, PigstyHome: fixture.home,
		RuntimePath: fixture.runtimePath, CurrentPig: fixture.pig,
		intentID:           strings.Repeat("c", 32),
		beforeConfigCommit: func() error { return os.ErrDeadlineExceeded },
	}
	if _, err := store.SwitchSource(t.Context(), request); !IsCode(err, "CMDB_SOURCE_CHANGED") {
		t.Fatalf("interrupted enable error = %v", err)
	}
	inspection, err := InspectSourceSwitch(fixture.runtimePath, fixture.configPath)
	if err != nil || inspection.State != SourceSwitchPending {
		t.Fatalf("interrupted switch = %+v, %v", inspection, err)
	}
	if _, err := store.SwitchSource(t.Context(), SourceSwitchRequest{
		Operation: SourceSwitchDisable, PigstyHome: fixture.home,
		RuntimePath: fixture.runtimePath, CurrentPig: fixture.pig,
	}); !IsCode(err, "CMDB_SWITCH_CONFLICT") {
		t.Fatalf("opposite operation bypassed pending intent: %v", err)
	}
	request.beforeConfigCommit = nil
	completed, err := store.SwitchSource(t.Context(), request)
	if err != nil || !completed.Changed || completed.IntentPath != inspection.Record.IntentPath {
		t.Fatalf("pending completion = %+v, %v", completed, err)
	}
}

func TestContainerSourceSwitchLegacyMigrationNeverExecutesLegacyTarget(t *testing.T) {
	store := openContainerStore(t)
	resetContainerSchema(t, store)
	if _, err := store.Init(t.Context(), "test-v1"); err != nil {
		t.Fatal(err)
	}
	fixture := newContainerSourceSwitchFixture(t)
	appendContainerSwitchHead(t, store, fixture.staticPath)
	legacy := filepath.Join(fixture.home, "inventory.sh")
	legacyConfig := []byte("[defaults]\ninventory = " + legacy + "\n")
	if err := os.WriteFile(fixture.configPath, legacyConfig, 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy target unexpectedly exists: %v", err)
	}

	result, err := store.SwitchSource(t.Context(), SourceSwitchRequest{
		Operation: SourceSwitchDisable, PigstyHome: fixture.home,
		RuntimePath: fixture.runtimePath, CurrentPig: fixture.pig,
		StaticPath: fixture.staticPath, intentID: strings.Repeat("d", 32),
	})
	if err != nil || !result.Changed || result.StaticLocator != fixture.staticPath {
		t.Fatalf("legacy migration result = %+v, %v", result, err)
	}
	if _, err := os.Lstat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy target was inspected into existence or executed: %v", err)
	}
	config, err := InspectAnsibleConfig(fixture.configPath)
	if err != nil || config.Locator != fixture.staticPath {
		t.Fatalf("legacy migration config = %+v, %v", config, err)
	}
}

func TestContainerSourceSwitchHoldsSharedLockThroughConfigCommit(t *testing.T) {
	store := openContainerStore(t)
	resetContainerSchema(t, store)
	if _, err := store.Init(t.Context(), "test-v1"); err != nil {
		t.Fatal(err)
	}
	fixture := newContainerSourceSwitchFixture(t)
	initial := appendContainerSwitchHead(t, store, fixture.staticPath)
	writer := openContainerStore(t)
	next := staticLoadFixture(t, "switch-next.yml", "all: { children: {}, vars: { admin_ip: 127.0.0.1, generation: 2 } }\n")
	done := make(chan error, 1)
	hook := func() error {
		expected := initial.Token
		go func() {
			_, err := writer.AppendStatic(t.Context(), &expected, next)
			done <- err
		}()
		select {
		case err := <-done:
			if err == nil {
				return errors.New("native writer bypassed source-switch shared lock")
			}
			return err
		case <-time.After(150 * time.Millisecond):
			return nil
		}
	}
	result, err := store.SwitchSource(t.Context(), SourceSwitchRequest{
		Operation: SourceSwitchEnable, PigstyHome: fixture.home,
		RuntimePath: fixture.runtimePath, CurrentPig: fixture.pig,
		beforeConfigCommit: hook, intentID: strings.Repeat("e", 32),
	})
	if err != nil || !result.Changed || result.Head == nil || !result.Head.Equal(initial.Token) {
		t.Fatalf("locked switch result = %+v, %v", result, err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("writer failed after source-switch lock release: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("writer remained blocked after source-switch commit")
	}
	state, err := store.Verify(t.Context())
	if err != nil || state.Head == nil || state.Head.Equal(initial.Token) {
		t.Fatalf("writer did not advance head after switch: %+v, %v", state, err)
	}
}

type containerSourceSwitchFixture struct {
	home           string
	configPath     string
	staticPath     string
	staticBody     []byte
	originalConfig []byte
	runtimePath    string
	runtime        RuntimeRecord
	pig            string
}

func newContainerSourceSwitchFixture(t *testing.T) containerSourceSwitchFixture {
	t.Helper()
	directory := symlinkFreeTempDir(t)
	home := filepath.Join(directory, "pigsty")
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatal(err)
	}
	pig := filepath.Join(directory, "pig")
	writeRuntimeExecutable(t, pig, "#!/bin/sh\nexit 0\n")
	runtime, err := PrepareRuntime(RuntimeOptions{
		PigstyHome: home, RuntimeRoot: filepath.Join(directory, "runtime"), PigBinary: pig,
		ConnectionSource: ConnectionMetaSource,
	})
	if err != nil {
		t.Fatal(err)
	}
	staticBody := []byte("all:\n  children: {}\n  vars:\n    admin_ip: 127.0.0.1\n    version: v4.4.0\n    region: default\n    generation: 1\n    password: source-switch-secret\n")
	staticPath := filepath.Join(directory, "static inventory.yml")
	if err := os.WriteFile(staticPath, staticBody, 0o600); err != nil {
		t.Fatal(err)
	}
	originalConfig := []byte("# keep\r\n[defaults]\r\ninventory = '../static inventory.yml' # exact\r\nforks = 5\r\n")
	configPath := filepath.Join(home, "ansible.cfg")
	if err := os.WriteFile(configPath, originalConfig, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(configPath, 0o640); err != nil {
		t.Fatal(err)
	}
	return containerSourceSwitchFixture{
		home: home, configPath: configPath, staticPath: staticPath, staticBody: staticBody,
		originalConfig: originalConfig,
		runtimePath:    filepath.Join(filepath.Dir(runtime.AdapterPath), "runtime.json"),
		runtime:        runtime, pig: pig,
	}
}

func appendContainerSwitchHead(t *testing.T, store *Store, path string) AppendResult {
	t.Helper()
	static, err := readSwitchStaticSource(path)
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.AppendStatic(t.Context(), nil, StaticLoad{
		Snapshot: static.snapshot, SourceLocator: path,
		SourceBytesSHA256: sha256.Sum256(mustReadContainerSwitchFile(t, path)), ToolVersion: "test-v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func mustReadContainerSwitchFile(t *testing.T, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return body
}
