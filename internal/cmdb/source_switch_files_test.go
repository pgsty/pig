package cmdb

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceSwitchIntentBacksUpBeforeAtomicNarrowCommit(t *testing.T) {
	fixture := newSourceSwitchFileFixture(t)
	document, err := readAnsibleConfig(fixture.configPath)
	if err != nil {
		t.Fatal(err)
	}
	patch, err := planAnsibleConfigPatch(document, fixture.runtime.AdapterPath)
	if err != nil {
		t.Fatal(err)
	}
	record, err := createSourceSwitchIntent(document, patch, sourceSwitchIntentOptions{
		operation: SourceSwitchEnable, home: fixture.home, runtimePath: fixture.runtimePath,
		head: fixture.head, adapter: fixture.runtime,
		priorLiteral: document.source.Literal, priorStaticLocator: document.source.Locator,
		intentID: strings.Repeat("1", 32),
	})
	if err != nil {
		t.Fatal(err)
	}

	if current, err := os.ReadFile(fixture.configPath); err != nil || !bytes.Equal(current, fixture.original) {
		t.Fatalf("intent creation changed config: %v, %q", err, current)
	}
	backup, err := os.ReadFile(record.BackupPath)
	if err != nil || !bytes.Equal(backup, fixture.original) {
		t.Fatalf("backup = %q, %v", backup, err)
	}
	assertSwitchFileMode(t, record.BackupPath, 0o640)
	assertSwitchFileMode(t, record.IntentPath, 0o600)
	assertSwitchFileMode(t, filepath.Join(filepath.Dir(fixture.runtimePath), "switch.json"), 0o600)
	inspection, err := InspectSourceSwitch(fixture.runtimePath, fixture.configPath)
	if err != nil || inspection.State != SourceSwitchPending || inspection.Record == nil || inspection.Record.IntentPath != record.IntentPath {
		t.Fatalf("pending inspection = %+v, %v", inspection, err)
	}

	ownershipChanged, err := commitAnsibleConfigPatch(patch, nil)
	if err != nil || ownershipChanged {
		t.Fatalf("config commit = ownership changed %t, %v", ownershipChanged, err)
	}
	want := bytes.Replace(fixture.original, []byte("'../static inventory.yml'"), []byte(fixture.runtime.AdapterPath), 1)
	current, err := os.ReadFile(fixture.configPath)
	if err != nil || !bytes.Equal(current, want) {
		t.Fatalf("narrow config commit = %q, %v; want %q", current, err, want)
	}
	assertSwitchFileMode(t, fixture.configPath, 0o640)
	inspection, err = InspectSourceSwitch(fixture.runtimePath, fixture.configPath)
	if err != nil || inspection.State != SourceSwitchApplied {
		t.Fatalf("applied inspection = %+v, %v", inspection, err)
	}
}

func TestSourceSwitchRetainsSupersededIntentAndRestoresExactLiteral(t *testing.T) {
	fixture := newSourceSwitchFileFixture(t)
	original, err := readAnsibleConfig(fixture.configPath)
	if err != nil {
		t.Fatal(err)
	}
	enablePatch, err := planAnsibleConfigPatch(original, fixture.runtime.AdapterPath)
	if err != nil {
		t.Fatal(err)
	}
	enable, err := createSourceSwitchIntent(original, enablePatch, sourceSwitchIntentOptions{
		operation: SourceSwitchEnable, home: fixture.home, runtimePath: fixture.runtimePath,
		head: fixture.head, adapter: fixture.runtime,
		priorLiteral: original.source.Literal, priorStaticLocator: original.source.Locator,
		intentID: strings.Repeat("2", 32),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := commitAnsibleConfigPatch(enablePatch, nil); err != nil {
		t.Fatal(err)
	}

	enabled, err := readAnsibleConfig(fixture.configPath)
	if err != nil {
		t.Fatal(err)
	}
	disablePatch, err := planAnsibleConfigLiteralPatch(enabled, enable.PriorLiteral, enable.PriorStaticLocator)
	if err != nil {
		t.Fatal(err)
	}
	disable, err := createSourceSwitchIntent(enabled, disablePatch, sourceSwitchIntentOptions{
		operation: SourceSwitchDisable, home: fixture.home, runtimePath: fixture.runtimePath,
		head: fixture.head, adapter: fixture.runtime,
		priorLiteral: enable.PriorLiteral, priorStaticLocator: enable.PriorStaticLocator,
		previous: &enable, intentID: strings.Repeat("3", 32),
	})
	if err != nil {
		t.Fatal(err)
	}
	if disable.PreviousIntent != enable.IntentPath {
		t.Fatalf("previous intent = %q, want %q", disable.PreviousIntent, enable.IntentPath)
	}
	if _, err := commitAnsibleConfigPatch(disablePatch, nil); err != nil {
		t.Fatal(err)
	}
	current, err := os.ReadFile(fixture.configPath)
	if err != nil || !bytes.Equal(current, fixture.original) {
		t.Fatalf("disable did not restore exact config: %v, %q", err, current)
	}
	for _, retained := range []string{enable.IntentPath, enable.BackupPath, disable.IntentPath, disable.BackupPath} {
		if _, err := os.Lstat(retained); err != nil {
			t.Fatalf("superseded artifact %s missing: %v", retained, err)
		}
	}
	inspection, err := InspectSourceSwitch(fixture.runtimePath, fixture.configPath)
	if err != nil || inspection.State != SourceSwitchApplied || inspection.Record.Operation != SourceSwitchDisable {
		t.Fatalf("disable inspection = %+v, %v", inspection, err)
	}
}

func TestSourceSwitchConfigCASNeverOverwritesConcurrentBytes(t *testing.T) {
	fixture := newSourceSwitchFileFixture(t)
	document, err := readAnsibleConfig(fixture.configPath)
	if err != nil {
		t.Fatal(err)
	}
	patch, err := planAnsibleConfigPatch(document, fixture.runtime.AdapterPath)
	if err != nil {
		t.Fatal(err)
	}
	record, err := createSourceSwitchIntent(document, patch, sourceSwitchIntentOptions{
		operation: SourceSwitchEnable, home: fixture.home, runtimePath: fixture.runtimePath,
		head: fixture.head, adapter: fixture.runtime,
		priorLiteral: document.source.Literal, priorStaticLocator: document.source.Locator,
		intentID: strings.Repeat("4", 32),
	})
	if err != nil {
		t.Fatal(err)
	}
	concurrent := bytes.Replace(fixture.original, []byte("forks = 5"), []byte("forks = 9"), 1)
	_, err = commitAnsibleConfigPatch(patch, func() error {
		return os.WriteFile(fixture.configPath, concurrent, 0o640)
	})
	if !IsCode(err, "CMDB_SOURCE_CHANGED") {
		t.Fatalf("config CAS error = %v", err)
	}
	current, readErr := os.ReadFile(fixture.configPath)
	if readErr != nil || !bytes.Equal(current, concurrent) {
		t.Fatalf("CAS overwrote concurrent bytes: %v, %q", readErr, current)
	}
	if backup, readErr := os.ReadFile(record.BackupPath); readErr != nil || !bytes.Equal(backup, fixture.original) {
		t.Fatalf("pre-CAS backup changed: %v, %q", readErr, backup)
	}
	inspection, inspectErr := InspectSourceSwitch(fixture.runtimePath, fixture.configPath)
	if inspectErr != nil || inspection.State != SourceSwitchDrift {
		t.Fatalf("concurrent state = %+v, %v", inspection, inspectErr)
	}
}

func TestSourceSwitchRecordAndBackupTamperFailClosed(t *testing.T) {
	for _, target := range []string{"record", "backup"} {
		t.Run(target, func(t *testing.T) {
			fixture := newSourceSwitchFileFixture(t)
			document, err := readAnsibleConfig(fixture.configPath)
			if err != nil {
				t.Fatal(err)
			}
			patch, err := planAnsibleConfigPatch(document, fixture.runtime.AdapterPath)
			if err != nil {
				t.Fatal(err)
			}
			record, err := createSourceSwitchIntent(document, patch, sourceSwitchIntentOptions{
				operation: SourceSwitchEnable, home: fixture.home, runtimePath: fixture.runtimePath,
				head: fixture.head, adapter: fixture.runtime,
				priorLiteral: document.source.Literal, priorStaticLocator: document.source.Locator,
				intentID: strings.Repeat("5", 32),
			})
			if err != nil {
				t.Fatal(err)
			}
			switch target {
			case "record":
				latest := filepath.Join(filepath.Dir(fixture.runtimePath), "switch.json")
				body, err := os.ReadFile(latest)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(latest, append(body, ' '), 0o600); err != nil {
					t.Fatal(err)
				}
			case "backup":
				if err := os.WriteFile(record.BackupPath, []byte("changed\n"), 0o640); err != nil {
					t.Fatal(err)
				}
			}
			before, err := os.ReadFile(fixture.configPath)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := InspectSourceSwitch(fixture.runtimePath, fixture.configPath); !IsCode(err, "CMDB_SWITCH_INVALID") {
				t.Fatalf("tampered %s error = %v", target, err)
			}
			after, err := os.ReadFile(fixture.configPath)
			if err != nil || !bytes.Equal(after, before) {
				t.Fatalf("tamper inspection mutated config: %v, %q", err, after)
			}
		})
	}
}

type sourceSwitchFileFixture struct {
	home        string
	configPath  string
	runtimePath string
	runtime     RuntimeRecord
	head        RevisionToken
	original    []byte
}

func newSourceSwitchFileFixture(t *testing.T) sourceSwitchFileFixture {
	t.Helper()
	directory := symlinkFreeTempDir(t)
	home := filepath.Join(directory, "pigsty")
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatal(err)
	}
	pig := filepath.Join(directory, "pig")
	writeRuntimeExecutable(t, pig, "#!/bin/sh\nexit 0\n")
	runtimeRoot := filepath.Join(directory, "runtime")
	runtime, err := PrepareRuntime(RuntimeOptions{
		PigstyHome: home, RuntimeRoot: runtimeRoot, PigBinary: pig,
		ConnectionSource: ConnectionMetaSource,
	})
	if err != nil {
		t.Fatal(err)
	}
	runtimePath := filepath.Join(filepath.Dir(runtime.AdapterPath), "runtime.json")
	original := []byte("# exact\r\n[defaults]\r\n  inventory = '../static inventory.yml' # fallback\r\nforks = 5\r\n")
	configPath := filepath.Join(home, "ansible.cfg")
	if err := os.WriteFile(configPath, original, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(configPath, 0o640); err != nil {
		t.Fatal(err)
	}
	head, err := ParseRevisionToken("cmdb-v1:1:sha256:" + strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	return sourceSwitchFileFixture{
		home: home, configPath: configPath, runtimePath: runtimePath,
		runtime: runtime, head: head, original: original,
	}
}

func assertSwitchFileMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != want {
		t.Fatalf("%s mode = %v, want %v", path, info.Mode().Perm(), want)
	}
}

func TestSourceSwitchHookFailureLeavesPendingIntent(t *testing.T) {
	fixture := newSourceSwitchFileFixture(t)
	document, err := readAnsibleConfig(fixture.configPath)
	if err != nil {
		t.Fatal(err)
	}
	patch, err := planAnsibleConfigPatch(document, fixture.runtime.AdapterPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = createSourceSwitchIntent(document, patch, sourceSwitchIntentOptions{
		operation: SourceSwitchEnable, home: fixture.home, runtimePath: fixture.runtimePath,
		head: fixture.head, adapter: fixture.runtime,
		priorLiteral: document.source.Literal, priorStaticLocator: document.source.Locator,
		intentID: strings.Repeat("6", 32),
	})
	if err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("stop")
	if _, err := commitAnsibleConfigPatch(patch, func() error { return sentinel }); !IsCode(err, "CMDB_SOURCE_CHANGED") {
		t.Fatalf("hook failure = %v", err)
	}
	inspection, err := InspectSourceSwitch(fixture.runtimePath, fixture.configPath)
	if err != nil || inspection.State != SourceSwitchPending {
		t.Fatalf("interrupted state = %+v, %v", inspection, err)
	}
	if _, err := commitAnsibleConfigPatch(patch, nil); err != nil {
		t.Fatalf("pending intent could not be completed: %v", err)
	}
}

func TestReadSwitchStaticSourceRejectsUnsupportedShapes(t *testing.T) {
	directory := symlinkFreeTempDir(t)
	validBody := "all:\n  children: {}\n  vars: { admin_ip: 127.0.0.1 }\n"
	valid := filepath.Join(directory, "valid.yml")
	if err := os.WriteFile(valid, []byte(validBody), 0o600); err != nil {
		t.Fatal(err)
	}
	if source, err := readSwitchStaticSource(valid); err != nil || source.snapshot == nil || source.path != valid {
		t.Fatalf("valid static source = %+v, %v", source, err)
	}
	executable := filepath.Join(directory, "dynamic")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(executable, 0o700); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(directory, "linked.yml")
	if err := os.Symlink(valid, symlink); err != nil {
		t.Fatal(err)
	}
	invalid := filepath.Join(directory, "invalid.yml")
	if err := os.WriteFile(invalid, []byte("plugin: example.dynamic\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	unrepresentable := filepath.Join(directory, "vault.yml")
	if err := os.WriteFile(unrepresentable, []byte("all:\n  children: {}\n  vars: { admin_ip: 127.0.0.1, password: !vault private-value }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, test := range map[string]struct {
		path string
		code string
	}{
		"directory":       {directory, "CMDB_SOURCE_INVALID"},
		"executable":      {executable, "CMDB_SOURCE_INVALID"},
		"symlink":         {symlink, "CMDB_SOURCE_INVALID"},
		"plugin":          {invalid, "CMDB_SOURCE_INVALID"},
		"unrepresentable": {unrepresentable, "CMDB_SNAPSHOT_UNREPRESENTABLE"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := readSwitchStaticSource(test.path)
			if !IsCode(err, test.code) || strings.Contains(err.Error(), "private-value") {
				t.Fatalf("%s error = %v", name, err)
			}
		})
	}
}
