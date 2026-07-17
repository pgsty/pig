package sty

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pig/internal/cmdb"
	"pig/internal/output"
)

type fakeCMDBSourceSwitcher struct {
	request cmdb.SourceSwitchRequest
	result  cmdb.SourceSwitchResult
	err     error
}

func (f *fakeCMDBSourceSwitcher) SwitchSource(_ context.Context, request cmdb.SourceSwitchRequest) (cmdb.SourceSwitchResult, error) {
	f.request = request
	return f.result, f.err
}

func TestCMDBSourceSwitchMapsOptionsAndSafeResult(t *testing.T) {
	directory := symlinkFreeStyTempDir(t)
	home := filepath.Join(directory, "pigsty")
	if err := osMkdirForCMDBTest(home); err != nil {
		t.Fatal(err)
	}
	runtimePath := filepath.Join(directory, "runtime", "runtime.json")
	staticPath := filepath.Join(directory, "static.yml")
	head, err := cmdb.ParseRevisionToken("cmdb-v1:4:sha256:" + strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	fake := &fakeCMDBSourceSwitcher{result: cmdb.SourceSwitchResult{
		Kind: cmdb.SourceSwitchKind, Operation: cmdb.SourceSwitchDisable,
		State: cmdb.SourceSwitchApplied, Changed: true, Head: &head,
		ConfigPath: filepath.Join(home, "ansible.cfg"), StaticLocator: staticPath,
		AdapterIdentity: "adapter-identity", IntentPath: "/runtime/intent", BackupPath: "/runtime/backup",
	}}
	result := cmdbSourceSwitch(t.Context(), fake, cmdb.SourceSwitchDisable, CMDBSourceSwitchOptions{
		PigstyHome: home, RuntimePath: runtimePath, PigBinary: "/test/pig",
		StaticPath: "static.yml", WorkingDirectory: directory,
	})
	if result == nil || !result.Success || fake.request.Operation != cmdb.SourceSwitchDisable ||
		fake.request.RuntimePath != runtimePath || fake.request.StaticPath != staticPath || fake.request.CurrentPig != "/test/pig" {
		t.Fatalf("source-switch mapping = request %+v result %+v", fake.request, result)
	}
	data, ok := result.Data.(*CMDBSourceSwitchData)
	if !ok || !data.Changed || data.Head == nil || data.Head.String() != head.String() || data.StaticLocator != staticPath {
		t.Fatalf("source-switch data = %+v", result.Data)
	}
}

func TestCMDBSourceSwitchFailureKeepsValuesOutOfResult(t *testing.T) {
	secret := "switch-private-value"
	fake := &fakeCMDBSourceSwitcher{err: &cmdb.Error{
		Code: "CMDB_SOURCE_HEAD_MISMATCH", Message: "static declarations do not match head",
	}}
	result := cmdbSourceSwitch(t.Context(), fake, cmdb.SourceSwitchEnable, CMDBSourceSwitchOptions{
		PigstyHome: "/unused", RuntimePath: "/runtime/runtime.json",
	})
	if result == nil || result.Success || result.Code != output.CodeStyCMDBDrift {
		t.Fatalf("source-switch failure = %+v", result)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), secret) || strings.Contains(string(encoded), "Inventory body") {
		t.Fatalf("source-switch result leaked private data: %s", encoded)
	}

	if _, ok := resolveCMDBSwitchStaticPath("bad\npath", ""); ok {
		t.Fatal("newline-bearing explicit static path was accepted")
	}
}

func osMkdirForCMDBTest(path string) error {
	return os.Mkdir(path, 0o700)
}
