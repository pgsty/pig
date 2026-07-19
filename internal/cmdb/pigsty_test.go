package cmdb

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pig/internal/config"
)

func TestResolvePigstyConnectionPrecedence(t *testing.T) {
	resolved, err := ResolvePigstyConnection(PigstyConnectionOptions{
		Database:  "meta_test",
		LookupEnv: func(string) (string, bool) { return "postgres://ignored", true },
	})
	if err != nil || resolved.source != ConnectionArgSource || resolved.value != "dbname=meta_test" {
		t.Fatalf("explicit resolution = %+v, %v", resolved, err)
	}
	resolved, err = ResolvePigstyConnection(PigstyConnectionOptions{
		LookupEnv: func(name string) (string, bool) {
			if name != "METADB_URL" {
				t.Fatalf("lookup name = %q", name)
			}
			return "service=custom", true
		},
	})
	if err != nil || resolved.source != ConnectionURLSource || resolved.value != "service=custom" {
		t.Fatalf("environment resolution = %+v, %v", resolved, err)
	}
	resolved, err = ResolvePigstyConnection(PigstyConnectionOptions{LookupEnv: func(string) (string, bool) { return "", false }})
	if err != nil || resolved.source != ConnectionMetaSource || resolved.value != "service=meta" {
		t.Fatalf("fallback resolution = %+v, %v", resolved, err)
	}
	if _, err := ResolvePigstyConnection(PigstyConnectionOptions{LookupEnv: func(string) (string, bool) { return "", true }}); !IsCode(err, "CMDB_CONNECTION_INVALID") {
		t.Fatalf("empty METADB_URL error = %v", err)
	}
}

func TestValidateExistingDeclarations(t *testing.T) {
	valid := Declarations{
		Globals: []JSONField{{Key: "region", Value: json.RawMessage(`"default"`)}},
		Groups: []GroupDeclaration{{
			Name: "meta", Vars: []JSONField{{Key: "pg_cluster", Value: json.RawMessage(`"pg-meta"`)}},
			Hosts: []HostDeclaration{{IP: "10.10.10.10", Vars: []JSONField{{Key: "pg_seq", Value: json.RawMessage(`1`)}}}},
		}},
	}
	if err := validateDeclarations(valid); err != nil {
		t.Fatalf("valid declarations: %v", err)
	}
	invalid := valid
	invalid.Groups = append(invalid.Groups, valid.Groups[0])
	if err := validateDeclarations(invalid); !IsCode(err, "CMDB_DECLARATION_INVALID") {
		t.Fatalf("duplicate group error = %v", err)
	}
	invalid = valid
	invalid.Groups = append([]GroupDeclaration(nil), valid.Groups...)
	invalid.Groups[0].Hosts = []HostDeclaration{{IP: "db.example.com"}}
	if err := validateDeclarations(invalid); !IsCode(err, "CMDB_DECLARATION_INVALID") {
		t.Fatalf("hostname error = %v", err)
	}
	invalid = valid
	invalid.Globals = []JSONField{{Key: "bad", Value: json.RawMessage(`{`)}}
	if err := validateDeclarations(invalid); !IsCode(err, "CMDB_DECLARATION_INVALID") {
		t.Fatalf("invalid JSON error = %v", err)
	}
}

func TestCanonicalDeclarationsAndRawConfigEquivalence(t *testing.T) {
	declarations := Declarations{
		Globals: []JSONField{{Key: "region", Value: json.RawMessage(`"default"`)}},
		Groups: []GroupDeclaration{{
			Name: "demo", Vars: []JSONField{{Key: "seq", Value: json.RawMessage(`1`)}},
			Hosts: []HostDeclaration{{IP: "10.10.10.10", Vars: []JSONField{{Key: "enabled", Value: json.RawMessage(`true`)}}}},
		}},
	}
	want, err := CanonicalDeclarationsJSON(declarations)
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"all":{"vars":{"region":"default"},"children":{"demo":{"vars":{"seq":1},"hosts":{"10.10.10.10":{"enabled":true}}}}}}`)
	got, err := CanonicalRawConfigJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("canonical raw = %s, declarations = %s", got, want)
	}
	empty, err := CanonicalRawConfigJSON([]byte(`{"all":{"children":null,"vars":null}}`))
	if err != nil || string(empty) != `{"all":{"children":{},"vars":{}}}` {
		t.Fatalf("empty canonical = %s, %v", empty, err)
	}
	if _, err := CanonicalRawConfigJSON([]byte(`{"all":{"children":{},"vars":{}},"extra":1}`)); !IsCode(err, "CMDB_RAW_CONFIG_INVALID") {
		t.Fatalf("extra raw_config field error = %v", err)
	}
}

func TestCanonicalDeclarationNumbersAreExactAndLexicallyStable(t *testing.T) {
	for input, want := range map[string]string{
		"1.0": "1", "100": "1e2", "0.0100": "1e-2", "-10.50e2": "-1.05e3", "0e999": "0",
	} {
		got, err := canonicalJSONNumber(json.Number(input))
		if err != nil || got.String() != want {
			t.Fatalf("canonical number %q = %q, %v; want %q", input, got, err, want)
		}
	}
	declarations := Declarations{Globals: []JSONField{
		{Key: "ratio", Value: json.RawMessage(`1`)},
		{Key: "exact", Value: json.RawMessage(`{"values":[9007199254740993,1.00]}`)},
	}}
	want, err := CanonicalDeclarationsJSON(declarations)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := CanonicalRawConfigJSON([]byte(`{"all":{"children":{},"vars":{"ratio":1.000,"exact":{"values":[9007199254740993,1]}}}}`))
	if err != nil || string(raw) != string(want) {
		t.Fatalf("numeric equivalence = %s / %s, %v", raw, want, err)
	}
	different, err := CanonicalRawConfigJSON([]byte(`{"all":{"children":{},"vars":{"ratio":1,"exact":{"values":[9007199254740992,1]}}}}`))
	if err != nil || string(different) == string(want) {
		t.Fatalf("large integer precision collapsed: %s / %s, %v", different, want, err)
	}
}

func TestInventoryScriptConnectionSafety(t *testing.T) {
	for _, value := range []string{
		"postgres://user:secret@localhost/meta",
		"postgres://user@localhost/meta?password=secret",
		"host=/tmp dbname=meta password=secret",
		"host=/tmp PASSWORD = 'secret' dbname=meta",
	} {
		if !literalConnectionHasPassword(value) {
			t.Fatalf("password not detected in %q", value)
		}
	}
	for _, value := range []string{"service=meta", "postgres://user@localhost/meta", "dbname=meta passfile=/private/pgpass"} {
		if literalConnectionHasPassword(value) {
			t.Fatalf("safe connection rejected: %q", value)
		}
	}
	if got := shellQuote("service=meta application_name='pig'"); got != `'service=meta application_name='\''pig'\'''` {
		t.Fatalf("shell quote = %q", got)
	}
	for _, value := range []string{"service=meta", "service=meta application_name='pig'"} {
		body, err := renderPigstyInventoryScript(value)
		kind, managed := config.ManagedInventoryScriptShape(body)
		if err != nil || !managed || kind != config.ManagedInventoryScriptPig {
			t.Fatalf("render %q = %q, %v", value, body, err)
		}
	}
	if _, err := renderPigstyInventoryScript("service=meta\necho unsafe"); !IsCode(err, "CMDB_CONNECTION_INVALID") {
		t.Fatalf("multiline connection error = %v", err)
	}
	if _, err := renderPigstyInventoryScript("host=db password=secret"); !IsCode(err, "CMDB_CONNECTION_SECRET") {
		t.Fatalf("password connection error = %v", err)
	}
}

func TestInspectManagedInventoryScriptRefusesThirdPartyWithoutMutation(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "inventory.sh")
	candidate := []byte("#!/bin/bash\nexec psql 'service=meta' -AXtwc 'SELECT text FROM pigsty.inventory;'\n")
	plan := &PigstySourcePlan{InventoryPath: path, scriptBody: candidate}
	if err := inspectManagedInventoryScript(plan); err != nil || !plan.ScriptChanged {
		t.Fatalf("missing script plan = %+v, %v", plan, err)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("planning created inventory.sh: %v", err)
	}

	thirdParty := []byte("#!/bin/bash\necho user-owned\n")
	if err := os.WriteFile(path, thirdParty, 0o700); err != nil {
		t.Fatal(err)
	}
	plan = &PigstySourcePlan{InventoryPath: path, scriptBody: candidate}
	if err := inspectManagedInventoryScript(plan); !IsCode(err, "CMDB_INVENTORY_SCRIPT_UNMANAGED") {
		t.Fatalf("third-party script error = %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(thirdParty) {
		t.Fatalf("third-party script changed: %q, %v", after, err)
	}

	legacy := []byte("#!/bin/bash\npsql postgresql://user:secret@db/meta -AXtwc 'SELECT text FROM pigsty.inventory;'\n")
	if err := os.WriteFile(path, legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	plan = &PigstySourcePlan{InventoryPath: path, scriptBody: candidate}
	if err := inspectManagedInventoryScript(plan); err != nil || !plan.ScriptChanged {
		t.Fatalf("legacy password replacement plan = %+v, %v", plan, err)
	}
	after, _ = os.ReadFile(path)
	if string(after) != string(legacy) {
		t.Fatal("planning replaced the legacy script")
	}

	spaceConninfo := []byte("#!/bin/bash\npsql host=db dbname=meta -AXtwc 'SELECT text FROM pigsty.inventory;'\n")
	if err := os.WriteFile(path, spaceConninfo, 0o755); err != nil {
		t.Fatal(err)
	}
	plan = &PigstySourcePlan{InventoryPath: path, scriptBody: candidate}
	if err := inspectManagedInventoryScript(plan); !IsCode(err, "CMDB_INVENTORY_SCRIPT_UNMANAGED") {
		t.Fatalf("space conninfo script error = %v", err)
	}
}

func TestDisablePlanIsReadOnlyAndRefusesThirdPartyDirective(t *testing.T) {
	home, err := os.MkdirTemp(".", ".cmdb-disable-plan-*")
	if err != nil {
		t.Fatal(err)
	}
	home, err = filepath.Abs(home)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(home)
	configPath := filepath.Join(home, "ansible.cfg")
	before := []byte("[defaults]\ninventory = inventory.sh\n")
	if err := os.WriteFile(configPath, before, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "pigsty.yml"), []byte("all: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := PlanDisablePigstyInventory(home)
	if err != nil || !plan.Changed || !plan.ConfigChanged || plan.ScriptChanged {
		t.Fatalf("disable plan = %+v, %v", plan, err)
	}
	after, _ := os.ReadFile(configPath)
	if string(after) != string(before) {
		t.Fatal("disable plan changed ansible.cfg")
	}
	if err := os.WriteFile(filepath.Join(home, "pigsty.yml"), []byte("all: {vars: {changed: true}}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := plan.Apply(); !IsCode(err, "CMDB_SOURCE_CHANGED") {
		t.Fatalf("stale static target error = %v", err)
	}
	after, _ = os.ReadFile(configPath)
	if string(after) != string(before) {
		t.Fatal("stale static target changed ansible.cfg")
	}
	thirdParty := []byte("[defaults]\ninventory = custom.py\n")
	if err := os.WriteFile(configPath, thirdParty, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := PlanDisablePigstyInventory(home); !IsCode(err, "CMDB_INVENTORY_DIRECTIVE_UNMANAGED") {
		t.Fatalf("third-party directive error = %v", err)
	}
	after, _ = os.ReadFile(configPath)
	if string(after) != string(thirdParty) {
		t.Fatal("third-party directive changed")
	}
}

func TestEnablePlanRechecksUnchangedManagedScriptBeforeSwitch(t *testing.T) {
	home, err := os.MkdirTemp(".", ".cmdb-enable-plan-*")
	if err != nil {
		t.Fatal(err)
	}
	home, err = filepath.Abs(home)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(home)
	configPath := filepath.Join(home, "ansible.cfg")
	configBefore := []byte("[defaults]\ninventory = pigsty.yml\n")
	if err := os.WriteFile(configPath, configBefore, 0o600); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(home, "inventory.sh")
	script := []byte("#!/bin/bash\nexec psql 'service=meta' -AXtwc 'SELECT text FROM pigsty.inventory;'\n")
	if err := os.WriteFile(scriptPath, script, 0o755); err != nil {
		t.Fatal(err)
	}
	document, err := readAnsibleConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	patch, changed, err := planGuardedInventoryLiteral(document, home, "inventory.sh")
	if err != nil || !changed {
		t.Fatalf("config plan = %+v, %v", patch, err)
	}
	plan := &PigstySourcePlan{
		ConfigPath: configPath, InventoryPath: scriptPath, scriptBody: script,
		ConfigChanged: true, Changed: true, configPatch: patch,
	}
	if err := inspectManagedInventoryScript(plan); err != nil || plan.ScriptChanged {
		t.Fatalf("managed script plan = %+v, %v", plan, err)
	}
	other := []byte("#!/bin/bash\nexec psql 'service=other' -AXtwc 'SELECT text FROM pigsty.inventory;'\n")
	if err := os.WriteFile(scriptPath, other, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := plan.Apply(); !IsCode(err, "CMDB_SOURCE_CHANGED") {
		t.Fatalf("stale managed script error = %v", err)
	}
	configAfter, _ := os.ReadFile(configPath)
	if string(configAfter) != string(configBefore) {
		t.Fatal("stale managed script changed ansible.cfg")
	}
}

func TestEnablePlanRollsBackScriptWhenConfigCommitFails(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "new", true: "existing"}[existing], func(t *testing.T) {
			home, err := os.MkdirTemp(".", ".cmdb-enable-rollback-*")
			if err != nil {
				t.Fatal(err)
			}
			home, err = filepath.Abs(home)
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(home)
			configPath := filepath.Join(home, "ansible.cfg")
			configBefore := []byte("[defaults]\ninventory = pigsty.yml\n")
			if err := os.WriteFile(configPath, configBefore, 0o600); err != nil {
				t.Fatal(err)
			}
			scriptPath := filepath.Join(home, "inventory.sh")
			oldScript := []byte("#!/bin/bash\nexec psql 'service=old' -AXtwc 'SELECT text FROM pigsty.inventory;'\n")
			if existing {
				if err := os.WriteFile(scriptPath, oldScript, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			candidate := []byte("#!/bin/bash\nexec psql 'service=meta' -AXtwc 'SELECT text FROM pigsty.inventory;'\n")
			document, err := readAnsibleConfig(configPath)
			if err != nil {
				t.Fatal(err)
			}
			patch, changed, err := planGuardedInventoryLiteral(document, home, "inventory.sh")
			if err != nil || !changed {
				t.Fatalf("config patch = %+v, %v", patch, err)
			}
			plan := &PigstySourcePlan{
				ConfigPath: configPath, InventoryPath: scriptPath, scriptBody: candidate,
				ConfigChanged: true, Changed: true, configPatch: patch,
				commitConfig: func(*ansibleConfigPatch) error {
					return failure("CMDB_SWITCH_FAILED", "injected ansible.cfg failure")
				},
			}
			if err := inspectManagedInventoryScript(plan); err != nil || !plan.ScriptChanged {
				t.Fatalf("script plan = %+v, %v", plan, err)
			}
			if _, err := plan.Apply(); !IsCode(err, "CMDB_SWITCH_FAILED") {
				t.Fatalf("apply error = %v", err)
			}
			configAfter, _ := os.ReadFile(configPath)
			if !bytes.Equal(configAfter, configBefore) {
				t.Fatalf("ansible.cfg changed: %q", configAfter)
			}
			if existing {
				after, readErr := os.ReadFile(scriptPath)
				info, statErr := os.Stat(scriptPath)
				if readErr != nil || statErr != nil {
					t.Fatalf("existing script cannot be read after rollback: read=%v stat=%v", readErr, statErr)
				}
				if !bytes.Equal(after, oldScript) || info.Mode().Perm() != 0o700 {
					t.Fatalf("existing script was not restored: %q mode=%v", after, info.Mode())
				}
			} else if _, err := os.Lstat(scriptPath); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("new script remains after rollback: %v", err)
			}
		})
	}
}

func TestDisablePigstyInventoryUsesNarrowPatch(t *testing.T) {
	home, err := os.MkdirTemp(".", ".cmdb-source-test-*")
	if err != nil {
		t.Fatal(err)
	}
	home, err = filepath.Abs(home)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(home)
	configPath := filepath.Join(home, "ansible.cfg")
	before := []byte("[defaults]\n# inventory note\ninventory   = inventory.sh  # active\nroles_path = roles\n\n[ssh_connection]\npipelining = True\n")
	if err := os.WriteFile(configPath, before, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "pigsty.yml"), []byte("all: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := DisablePigstyInventory(home)
	if err != nil || !result.Changed {
		t.Fatalf("disable = %+v, %v", result, err)
	}
	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(string(before), "inventory.sh", "pigsty.yml", 1)
	if string(after) != want {
		t.Fatalf("ansible.cfg changed outside value\n got: %s\nwant: %s", after, want)
	}
	info, err := os.Stat(configPath)
	if err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("ansible.cfg mode = %v, %v", info.Mode().Perm(), err)
	}
	result, err = DisablePigstyInventory(home)
	if err != nil || result.Changed {
		t.Fatalf("second disable = %+v, %v", result, err)
	}
}
