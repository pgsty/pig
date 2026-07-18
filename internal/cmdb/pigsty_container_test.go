package cmdb

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestContainerExistingPigstyBaselineLoadDumpAndTransactionRollback(t *testing.T) {
	store := openContainerStore(t)
	if cmdbContainer == nil || cmdbContainer.guard() != nil {
		t.Fatal("destructive test guard failed")
	}
	if _, err := store.pool.Exec(t.Context(), `DROP SCHEMA IF EXISTS pigsty_cmdb CASCADE; DROP SCHEMA IF EXISTS pigsty CASCADE; DROP SCHEMA IF EXISTS pglog CASCADE`); err != nil {
		t.Fatal(err)
	}
	home := os.Getenv("PIGSTY_HOME")
	if home == "" {
		home = filepath.Clean(filepath.Join("..", "..", "..", "pigsty"))
	}
	baseline, err := os.ReadFile(filepath.Join(home, "files", "cmdb.sql"))
	if err != nil {
		t.Skip("version-matched adjacent Pigsty cmdb.sql is unavailable")
	}
	inspection, err := store.InspectPigstyBaseline(t.Context())
	if err != nil || inspection.ExistingSchemas != 0 {
		t.Fatalf("initial inspection = %+v, %v", inspection, err)
	}
	initialized, err := store.ApplyPigstyBaseline(t.Context(), baseline)
	if err != nil || initialized.VerifiedObjects != len(requiredPigstyObjects) || initialized.StatementResults == 0 {
		t.Fatalf("baseline result = %+v, %v", initialized, err)
	}

	declarations := Declarations{
		Globals: []JSONField{
			{Key: "admin_ip", Value: json.RawMessage(`"10.10.10.10"`)},
			{Key: "region", Value: json.RawMessage(`"china"`)},
		},
		Groups: []GroupDeclaration{{
			Name: "pg-meta",
			Vars: []JSONField{{Key: "pg_cluster", Value: json.RawMessage(`"pg-meta"`)}},
			Hosts: []HostDeclaration{{
				IP: "10.10.10.10",
				Vars: []JSONField{
					{Key: "pg_seq", Value: json.RawMessage(`1`)},
					{Key: "pg_role", Value: json.RawMessage(`"primary"`)},
				},
			}},
		}},
	}
	loaded, err := store.ReplaceDeclarations(t.Context(), declarations)
	if err != nil || loaded.Globals != 2 || loaded.Groups != 1 || loaded.Hosts != 1 || loaded.GroupVar != 1 || loaded.HostVar != 2 {
		t.Fatalf("load result = %+v, %v", loaded, err)
	}
	raw, err := store.RawConfig(t.Context())
	if err != nil || !json.Valid(raw) {
		t.Fatalf("raw_config = %s, %v", raw, err)
	}
	dynamic, err := store.DynamicInventory(t.Context())
	if err != nil || !json.Valid(dynamic) {
		t.Fatalf("dynamic Inventory = %s, %v", dynamic, err)
	}

	before := declarationTableCounts(t, store)
	invalid := Declarations{
		Globals: []JSONField{{Key: "would_be_partial", Value: json.RawMessage(`true`)}},
		Groups:  []GroupDeclaration{{Name: "bad\x00group"}},
	}
	if _, err := store.ReplaceDeclarations(t.Context(), invalid); !IsCode(err, "CMDB_LOAD_FAILED") {
		t.Fatalf("injected failure = %v", err)
	}
	after := declarationTableCounts(t, store)
	if before != after {
		t.Fatalf("failed transaction changed declaration counts: before=%v after=%v", before, after)
	}

	defaults, err := store.ApplyOSDefaults(t.Context(), OSDefaults{
		Values:   []JSONField{{Key: "repo_packages", Value: json.RawMessage(`["node-bootstrap"]`)}},
		Packages: []TextField{{Key: "node-bootstrap", Value: "ansible python3"}},
	})
	if err != nil || defaults.Values != 1 || defaults.Packages != 1 {
		t.Fatalf("OS defaults = %+v, %v", defaults, err)
	}
	var abandonedExists bool
	if err := store.pool.QueryRow(t.Context(), `SELECT to_regnamespace('pigsty_cmdb') IS NOT NULL`).Scan(&abandonedExists); err != nil || abandonedExists {
		t.Fatalf("corrected adapter created pigsty_cmdb: exists=%v err=%v", abandonedExists, err)
	}
}

func declarationTableCounts(t *testing.T, store *Store) [5]int {
	t.Helper()
	var counts [5]int
	if err := store.pool.QueryRow(t.Context(), `
SELECT (SELECT count(*) FROM pigsty.global_var),
       (SELECT count(*) FROM pigsty.group),
       (SELECT count(*) FROM pigsty.group_var),
       (SELECT count(*) FROM pigsty.host),
       (SELECT count(*) FROM pigsty.host_var)`).Scan(&counts[0], &counts[1], &counts[2], &counts[3], &counts[4]); err != nil {
		t.Fatal(err)
	}
	return counts
}
