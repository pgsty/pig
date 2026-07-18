package cmdb

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func TestInventoryScriptConnectionSafety(t *testing.T) {
	for _, value := range []string{
		"postgres://user:secret@localhost/meta",
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
