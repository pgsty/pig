package sty

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pig/internal/cmdb"
	"pig/internal/output"
	"pig/inventory"
)

func TestPrepareCMDBStaticLoadCanonicalizesAndValidates(t *testing.T) {
	directory := resolvedTempDir(t)
	name := " inventory file .yml "
	path := filepath.Join(directory, name)
	source := []byte("all:\n  children: {}\n  vars:\n    admin_ip: 10.0.0.1\n    password: private-static-value\n")
	if err := os.WriteFile(path, source, 0o640); err != nil {
		t.Fatal(err)
	}
	load, failure := prepareCMDBStaticLoad(name, directory, "test-v1")
	if failure != nil {
		t.Fatalf("valid static source failed: %s", failure)
	}
	if load.SourceLocator != path || load.SourceBytesSHA256 != sha256.Sum256(source) || load.ToolVersion != "test-v1" || load.Snapshot == nil {
		t.Fatalf("static load provenance mismatch: path=%q digest=%x", load.SourceLocator, load.SourceBytesSHA256)
	}
	document, err := inventory.Parse(path, source)
	if err != nil {
		t.Fatal(err)
	}
	want, err := inventory.SnapshotFromDocument(document)
	if err != nil {
		t.Fatal(err)
	}
	if !load.Snapshot.Equal(want) {
		t.Fatalf("Snapshot digest = %s, want %s", load.Snapshot.Digest(), want.Digest())
	}
}

func TestPrepareCMDBStaticLoadRejectsUnsafeSources(t *testing.T) {
	directory := resolvedTempDir(t)
	valid := filepath.Join(directory, "valid.yml")
	if err := os.WriteFile(valid, []byte("all: {children: {}, vars: {admin_ip: 10.0.0.1}}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(directory, "symlink.yml")
	if err := os.Symlink(valid, symlink); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(directory, "executable.yml")
	if err := os.WriteFile(executable, []byte("all: {}\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	oversized := filepath.Join(directory, "oversized.yml")
	file, err := os.OpenFile(oversized, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxCMDBStaticSourceBytes + 1); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		path string
		code int
	}{
		{name: "missing", path: filepath.Join(directory, "missing.yml"), code: output.CodeStyCMDBResourceMissing},
		{name: "directory", path: directory, code: output.CodeStyCMDBInvalidArgs},
		{name: "symlink", path: symlink, code: output.CodeStyCMDBInvalidArgs},
		{name: "executable", path: executable, code: output.CodeStyCMDBInvalidArgs},
		{name: "oversized", path: oversized, code: output.CodeStyCMDBSnapshotUnrepresentable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, failure := prepareCMDBStaticLoad(test.path, "", "test-v1")
			if failure == nil || failure.Success || failure.Code != test.code {
				t.Fatalf("unsafe source result = %v, want code %d", failure, test.code)
			}
		})
	}
}

func TestPrepareCMDBStaticLoadDiagnosticsNeverExposeValues(t *testing.T) {
	directory := resolvedTempDir(t)
	tests := []struct {
		name   string
		source string
		code   int
		secret string
	}{
		{
			name: "native validation", code: output.CodeStyCMDBConfigInvalid, secret: "native-secret-value",
			source: "all:\n  children: {}\n  vars:\n    admin_ip: native-secret-value\n",
		},
		{
			name: "representability", code: output.CodeStyCMDBSnapshotUnrepresentable, secret: "vault-secret-value",
			source: "all:\n  children: {}\n  vars:\n    admin_ip: 10.0.0.1\n    password: !vault vault-secret-value\n",
		},
		{
			name: "parse", code: output.CodeStyCMDBConfigInvalid, secret: "parse-secret-value",
			source: "all:\n  children: {}\n  vars: [parse-secret-value\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(directory, test.name+".yml")
			if err := os.WriteFile(path, []byte(test.source), 0o600); err != nil {
				t.Fatal(err)
			}
			_, failure := prepareCMDBStaticLoad(path, "", "test-v1")
			if failure == nil || failure.Code != test.code {
				t.Fatalf("validation result = %v, want code %d", failure, test.code)
			}
			encoded, err := json.Marshal(failure)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(encoded, []byte(test.secret)) || bytes.Contains(encoded, []byte(test.source)) {
				t.Fatalf("validation result leaked source values: %s", encoded)
			}
		})
	}
}

func TestPlanCMDBLoadRejectsInvalidStaticSourceBeforeStoreAccess(t *testing.T) {
	directory := resolvedTempDir(t)
	path := filepath.Join(directory, "invalid.yml")
	secret := "pre-database-private-value"
	if err := os.WriteFile(path, []byte("all:\n  children: {}\n  vars:\n    admin_ip: "+secret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	operation, failure := PlanCMDBLoad(t.Context(), nil, CMDBLoadOptions{StaticFile: path, ToolVersion: "test-v1"})
	if operation != nil || failure == nil || failure.Code != output.CodeStyCMDBConfigInvalid {
		t.Fatalf("invalid pre-database source result = operation=%v failure=%v", operation, failure)
	}
	encoded, err := json.Marshal(failure)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte(secret)) {
		t.Fatalf("pre-database validation leaked value: %s", encoded)
	}
}

func TestPlanCMDBLoadRejectsAmbiguityBeforeStoreAccess(t *testing.T) {
	resultCases := []CMDBLoadOptions{
		{Revision: "unsafe", StaticFile: "inventory.yml"},
		{Revision: "unsafe", InvocationSource: "override.yml"},
	}
	for _, options := range resultCases {
		operation, failure := PlanCMDBLoad(t.Context(), nil, options)
		if operation != nil || failure == nil || failure.Code != output.CodeStyCMDBInvalidArgs {
			t.Fatalf("ambiguous load result = operation=%v failure=%v", operation, failure)
		}
	}
	operation, failure := PlanCMDBLoad(t.Context(), nil, CMDBLoadOptions{Revision: "not-an-exact-token"})
	if operation != nil || failure == nil || failure.Code != output.CodeStyCMDBInvalidArgs {
		t.Fatalf("invalid revision result = operation=%v failure=%v", operation, failure)
	}
}

func TestApplyCMDBLoadRejectsStaticSourceRevisionChange(t *testing.T) {
	directory := resolvedTempDir(t)
	path := filepath.Join(directory, "inventory.yml")
	before := []byte("all: {children: {}, vars: {admin_ip: 10.0.0.1, generation: 1}}\n")
	if err := os.WriteFile(path, before, 0o600); err != nil {
		t.Fatal(err)
	}
	operation := &CMDBLoadOperation{
		store: &cmdb.Store{}, staticPath: path, staticDigest: sha256.Sum256(before),
		data: CMDBLoadData{Kind: cmdb.LoadKind, SourceKind: cmdb.SourceStatic},
	}
	after := []byte("all: {children: {}, vars: {admin_ip: 10.0.0.1, generation: 2}}\n")
	if err := os.WriteFile(path, after, 0o600); err != nil {
		t.Fatal(err)
	}
	result := ApplyCMDBLoad(t.Context(), operation, CMDBLoadOptions{})
	if result.Success || result.Code != output.CodeStyCMDBSourceChanged {
		t.Fatalf("changed source result = %v", result)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, before) || bytes.Contains(encoded, after) {
		t.Fatalf("source-change result leaked Inventory bytes: %s", encoded)
	}
}

func TestCMDBLoadDataAndFailuresAreValueFree(t *testing.T) {
	secret := "driver-or-inventory-secret"
	result := cmdbFailureResult("CMDB operation failed", errors.New(secret))
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte(secret)) || !bytes.Contains(encoded, []byte("CMDB_INTERNAL")) {
		t.Fatalf("unknown error was not sanitized: %s", encoded)
	}

	plan := cmdb.LoadPlan{
		Kind: cmdb.LoadKind, SourceKind: cmdb.SourceStatic,
		CandidateSHA256: strings.Repeat("a", 64),
		Changes:         []inventory.SnapshotChange{{Operation: inventory.SnapshotReplace, Path: inventory.Keys("all", "vars", "password")}},
	}
	operation := newCMDBLoadOperation(&cmdb.Store{}, plan, "/private/source.yml", sha256.Sum256([]byte(secret)))
	encoded, err = json.Marshal(operation.Data())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte(secret)) || bytes.Contains(encoded, []byte("/private/source.yml")) {
		t.Fatalf("load plan leaked private provenance: %s", encoded)
	}
}

func resolvedTempDir(t *testing.T) string {
	t.Helper()
	directory, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return directory
}
