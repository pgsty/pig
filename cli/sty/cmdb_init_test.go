package sty

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"pig/internal/cmdb"
	"pig/internal/output"
)

type fakeCMDBInitializer struct {
	migration cmdb.InitResult
	state     cmdb.VerifiedState
	source    cmdb.ConnectionSource
	initErr   error
	verifyErr error
	version   string
}

func (f *fakeCMDBInitializer) Init(_ context.Context, version string) (cmdb.InitResult, error) {
	f.version = version
	return f.migration, f.initErr
}

func (f *fakeCMDBInitializer) Verify(context.Context) (cmdb.VerifiedState, error) {
	return f.state, f.verifyErr
}

func (f *fakeCMDBInitializer) Source() cmdb.ConnectionSource { return f.source }

func TestCMDBInitVerifiesStateThenPreparesRuntime(t *testing.T) {
	head, err := cmdb.ParseRevisionToken("cmdb-v1:2:sha256:" + strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	store := &fakeCMDBInitializer{
		migration: cmdb.InitResult{MigrationVersion: 1, Applied: 1},
		state:     cmdb.VerifiedState{MigrationVersion: 1, RevisionCount: 2, Head: &head},
		source:    cmdb.ConnectionFileSource,
	}
	var prepared cmdb.RuntimeOptions
	result := cmdbInit(t.Context(), store, CMDBInitOptions{
		PigstyHome: "/srv/pigsty", RuntimeRoot: "/runtime", PigBinary: "/bin/pig",
		ConnectionFile: "/private/connection", ToolVersion: "test-v1",
	}, func(options cmdb.RuntimeOptions) (cmdb.RuntimeRecord, error) {
		prepared = options
		return cmdb.RuntimeRecord{
			Kind: cmdb.RuntimeRecordKind, AdapterPath: "/runtime/key/inventory",
			AdapterSHA256: strings.Repeat("c", 64),
		}, nil
	})
	if result == nil || !result.Success || store.version != "test-v1" ||
		prepared.PigstyHome != "/srv/pigsty" || prepared.ConnectionSource != cmdb.ConnectionFileSource ||
		prepared.ConnectionFile != "/private/connection" {
		t.Fatalf("CMDB init = prepared %+v result %+v", prepared, result)
	}
	data, ok := result.Data.(*CMDBInitData)
	if !ok || data.MigrationVersion != 1 || data.Applied != 1 || data.RevisionCount != 2 ||
		data.Head == nil || !data.Head.Equal(head) || data.RuntimePath != filepath.Join("/runtime/key", "runtime.json") {
		t.Fatalf("CMDB init data = %+v", result.Data)
	}
}

func TestCMDBInitPartialRuntimeFailureIsSanitized(t *testing.T) {
	secret := "runtime-init-secret"
	store := &fakeCMDBInitializer{
		migration: cmdb.InitResult{MigrationVersion: 1},
		state:     cmdb.VerifiedState{MigrationVersion: 1},
		source:    cmdb.ConnectionEnvSource,
	}
	result := cmdbInit(t.Context(), store, CMDBInitOptions{PigstyHome: "/srv/pigsty"}, func(cmdb.RuntimeOptions) (cmdb.RuntimeRecord, error) {
		return cmdb.RuntimeRecord{}, &cmdb.Error{Code: "CMDB_RUNTIME_ADAPTER_INVALID", Message: "runtime adapter could not be prepared"}
	})
	if result == nil || result.Success || result.Code != output.CodeStyCMDBConfigInvalid {
		t.Fatalf("CMDB init runtime failure = %+v", result)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), secret) {
		t.Fatalf("CMDB init result leaked private value: %s", encoded)
	}
}
