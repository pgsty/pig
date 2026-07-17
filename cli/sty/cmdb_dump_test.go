package sty

import (
	"bytes"
	"encoding/json"
	"testing"

	"pig/internal/cmdb"
	"pig/internal/output"
	"pig/inventory"
)

func TestCMDBDumpRejectsAmbiguousParametersBeforeStoreAccess(t *testing.T) {
	tests := []CMDBDumpOptions{
		{},
		{OutputPath: "out.yml", Merge: true, Revision: "invalid"},
		{OutputPath: "out.yml", Merge: true, InvocationSource: "override.yml"},
		{OutputPath: "out.yml", ExpectBase: "invalid"},
		{OutputPath: "out.yml", Revision: "not-an-exact-token"},
	}
	for _, options := range tests {
		result := CMDBDump(t.Context(), nil, options)
		if result.Success || result.Code != output.CodeStyCMDBInvalidArgs {
			t.Fatalf("invalid dump options %+v produced %v", options, result)
		}
	}
}

func TestCMDBDumpDataContainsConflictsButNoBodies(t *testing.T) {
	data := &CMDBDumpData{
		Kind: cmdb.DumpKind,
		Conflicts: []inventory.SnapshotConflict{{
			Working: inventory.SnapshotChange{Operation: inventory.SnapshotReplace, Path: inventory.Keys("all", "vars", "password")},
			Current: inventory.SnapshotChange{Operation: inventory.SnapshotRemove, Path: inventory.Keys("all", "vars", "password")},
		}},
		FindingCode: "CMDB_MERGE_CONFLICT",
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range [][]byte{[]byte("body"), []byte("old_value"), []byte("new_value"), []byte("source_locator")} {
		if bytes.Contains(encoded, forbidden) {
			t.Fatalf("dump DTO contains forbidden field %q: %s", forbidden, encoded)
		}
	}
	if !bytes.Contains(encoded, []byte("CMDB_MERGE_CONFLICT")) || !bytes.Contains(encoded, []byte(`"password"`)) {
		t.Fatalf("dump conflict metadata is incomplete: %s", encoded)
	}
}
