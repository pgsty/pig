package sty

import (
	"encoding/json"
	"strings"
	"testing"

	"pig/inventory"

	"gopkg.in/yaml.v3"
)

func TestInventoryWirePathsAreCanonicalStrings(t *testing.T) {
	path := inventory.Keys("all", "children", "pg/meta", "hosts", "10.0.0.1", "key~name")
	diagnostics := inventoryDiagnostics([]inventory.Diagnostic{{
		Code: "TEST", Severity: inventory.SeverityError, Path: path,
		Message: "safe diagnostic",
	}})
	changes := inventoryChanges(inventory.ChangeSet{Changes: []inventory.Change{{Operation: "replace", Path: path}}})
	payload := struct {
		Diagnostics []InventoryDiagnostic `json:"diagnostics" yaml:"diagnostics"`
		Changes     InventoryChangeSet    `json:"changes" yaml:"changes"`
	}{diagnostics, changes}

	jsonBody, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	yamlBody, err := yaml.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	want := "/all/children/pg~1meta/hosts/10.0.0.1/key~0name"
	for format, body := range map[string]string{"json": string(jsonBody), "yaml": string(yamlBody)} {
		if !strings.Contains(body, want) {
			t.Fatalf("%s payload lacks canonical path %q: %s", format, want, body)
		}
		if strings.Contains(body, "kind: 0") || strings.Contains(body, `\"kind\":0`) {
			t.Fatalf("%s payload leaked numeric-only path segments: %s", format, body)
		}
	}
}
