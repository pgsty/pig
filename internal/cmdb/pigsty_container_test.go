package cmdb

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pig/internal/config"
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
	canonicalRaw, err := CanonicalRawConfigJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	canonicalDeclarations, err := CanonicalDeclarationsJSON(declarations)
	if err != nil || string(canonicalRaw) != string(canonicalDeclarations) {
		t.Fatalf("canonical declaration parity = %s / %s, %v", canonicalRaw, canonicalDeclarations, err)
	}
	dynamic, err := store.DynamicInventory(t.Context())
	if err != nil || !json.Valid(dynamic) {
		t.Fatalf("dynamic Inventory = %s, %v", dynamic, err)
	}
	testContainerSourceSwitch(t, store.TargetFingerprint())
	checked, err := store.InspectPigstyCMDB(t.Context())
	if err != nil || checked.VerifiedObjects != len(requiredPigstyObjects) || checked.Globals != 2 || checked.Groups != 1 || checked.Hosts != 1 {
		t.Fatalf("CMDB inspection = %+v, %v", checked, err)
	}

	if _, err := store.pool.Exec(t.Context(), `INSERT INTO pigsty.global_var(key, value) VALUES ('concurrent_change', 'true')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReplaceDeclarationsIfCurrent(t.Context(), declarations, canonicalRaw); !IsCode(err, "CMDB_SOURCE_CHANGED") {
		t.Fatalf("stale declaration replacement error = %v", err)
	}
	var concurrentExists bool
	if err := store.pool.QueryRow(t.Context(), `SELECT EXISTS (SELECT 1 FROM pigsty.global_var WHERE key = 'concurrent_change')`).Scan(&concurrentExists); err != nil || !concurrentExists {
		t.Fatalf("stale replacement mutated rows: exists=%v err=%v", concurrentExists, err)
	}
	if _, err := store.pool.Exec(t.Context(), `DELETE FROM pigsty.global_var WHERE key = 'concurrent_change'`); err != nil {
		t.Fatal(err)
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

func testContainerSourceSwitch(t *testing.T, expectedTarget string) {
	t.Helper()
	parsed, err := url.Parse(cmdbContainer.dsn)
	if err != nil || parsed.User == nil {
		t.Fatalf("container connection parse = %v", err)
	}
	password, exists := parsed.User.Password()
	if !exists {
		t.Fatal("container connection has no test password")
	}
	home, err := os.MkdirTemp(".", ".cmdb-container-source-*")
	if err != nil {
		t.Fatal(err)
	}
	home, err = filepath.Abs(home)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(home)
	passfile := filepath.Join(home, "pgpass")
	line := strings.Join([]string{parsed.Hostname(), parsed.Port(), strings.TrimPrefix(parsed.Path, "/"), parsed.User.Username(), password}, ":") + "\n"
	if err := os.WriteFile(passfile, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	parsed.User = url.User(parsed.User.Username())
	query := parsed.Query()
	query.Set("passfile", passfile)
	parsed.RawQuery = query.Encode()
	safeStore, err := Open(t.Context(), ResolvedConnection{source: ConnectionArgSource, value: parsed.String()}, OpenOptions{})
	if err != nil {
		t.Fatalf("password-free passfile connection = %v", err)
	}
	defer safeStore.Close()
	if safeStore.TargetFingerprint() == "" || safeStore.TargetFingerprint() != expectedTarget {
		t.Fatal("safe connection did not pin the same value-free database target")
	}
	configPath := filepath.Join(home, "ansible.cfg")
	if err := os.WriteFile(configPath, []byte("[defaults]\ninventory = pigsty.yml\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "pigsty.yml"), []byte("all: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := safeStore.PlanEnablePigstyInventory(t.Context(), home)
	if err != nil || !plan.Changed || !plan.ScriptChanged || !plan.ConfigChanged {
		t.Fatalf("enable plan = %+v, %v", plan, err)
	}
	if _, err := os.Lstat(filepath.Join(home, "inventory.sh")); !os.IsNotExist(err) {
		t.Fatalf("enable plan wrote inventory.sh: %v", err)
	}
	result, err := plan.Apply()
	if err != nil || !result.Changed {
		t.Fatalf("enable apply = %+v, %v", result, err)
	}
	scriptPath := filepath.Join(home, "inventory.sh")
	script, err := os.ReadFile(scriptPath)
	info, statErr := os.Stat(scriptPath)
	if err != nil || statErr != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("managed script state = %v / %v / %v", err, statErr, info)
	}
	if kind, managed := config.ManagedInventoryScriptShape(script); !managed || kind != config.ManagedInventoryScriptPig || config.ManagedInventoryScriptHasPassword(script) {
		t.Fatal("enabled script is not the exact password-free Pig shape")
	}
	directive, err := config.ReadAnsibleInventoryDirective(configPath)
	if err != nil || directive.Value != "inventory.sh" || directive.Path != scriptPath {
		t.Fatalf("enabled directive = %+v, %v", directive, err)
	}
	plan, err = safeStore.PlanEnablePigstyInventory(t.Context(), home)
	if err != nil || plan.Changed {
		t.Fatalf("idempotent enable plan = %+v, %v", plan, err)
	}
	disabled, err := DisablePigstyInventory(home)
	if err != nil || !disabled.Changed || disabled.ScriptChanged {
		t.Fatalf("disable = %+v, %v", disabled, err)
	}
	directive, err = config.ReadAnsibleInventoryDirective(configPath)
	if err != nil || directive.Value != "pigsty.yml" {
		t.Fatalf("disabled directive = %+v, %v", directive, err)
	}
	if _, err := os.Stat(scriptPath); err != nil {
		t.Fatalf("disable removed inventory.sh: %v", err)
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
