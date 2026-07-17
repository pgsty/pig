package sty

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"pig/internal/output"
	"pig/inventory"
)

func TestCMDBInventoryAdapterGrammarFailsBeforeRuntime(t *testing.T) {
	for _, arguments := range [][]string{
		nil,
		{"--list", "extra"},
		{"--host"},
		{"--host", ""},
		{"--host", "db-a", "extra"},
		{"--unknown"},
	} {
		body, result := CMDBInventoryAdapter(t.Context(), CMDBInventoryAdapterOptions{Arguments: arguments})
		if body != nil || result == nil || result.Success || result.Code != output.CodeStyCMDBInvalidArgs {
			t.Fatalf("arguments %q result = body %q, %+v", arguments, body, result)
		}
		encoded, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(encoded, []byte("db-a")) {
			t.Fatalf("invalid argument entered sanitized Result: %s", encoded)
		}
	}
}

func TestRenderCMDBInventoryRequestKeepsBodyPrivate(t *testing.T) {
	document, err := inventory.Parse("cmdb.yml", []byte(`all:
  children:
    database:
      hosts:
        db-a:
          role: primary
          password: projection-secret
      vars:
        cluster: alpha
`))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := inventory.SnapshotFromDocument(document)
	if err != nil {
		t.Fatal(err)
	}

	list, failure := renderCMDBInventoryRequest(snapshot, cmdbInventoryRequest{})
	if failure != nil || !bytes.Contains(list, []byte(`"password":"projection-secret"`)) ||
		!bytes.Contains(list, []byte(`"database"`)) {
		t.Fatalf("list projection = %s, %+v", list, failure)
	}
	host := "db-a"
	projected, failure := renderCMDBInventoryRequest(snapshot, cmdbInventoryRequest{host: &host})
	if failure != nil || string(projected) != `{"role":"primary","password":"projection-secret"}` {
		t.Fatalf("host projection = %s, %+v", projected, failure)
	}
	missing := "missing"
	projected, failure = renderCMDBInventoryRequest(snapshot, cmdbInventoryRequest{host: &missing})
	if failure != nil || string(projected) != `{}` {
		t.Fatalf("missing host projection = %s, %+v", projected, failure)
	}

	_, failure = renderCMDBInventoryRequest(nil, cmdbInventoryRequest{})
	if failure == nil || failure.Success {
		t.Fatalf("nil Snapshot failure = %+v", failure)
	}
	encoded, err := json.Marshal(failure)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "projection-secret") {
		t.Fatalf("private body entered Result: %s", encoded)
	}
}
