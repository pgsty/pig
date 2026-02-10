package output

import (
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestEmbeddedJSON_MarshalJSON_EmbedsObjectNotString(t *testing.T) {
	r := OK("ok", NewEmbeddedJSON([]byte(`[{"a":1,"b":null}]`)))
	out, err := r.JSON()
	if err != nil {
		t.Fatalf("JSON() failed: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("unmarshal result json failed: %v", err)
	}

	data, ok := decoded["data"]
	if !ok {
		t.Fatalf("missing data field")
	}
	arr, ok := data.([]any)
	if !ok {
		t.Fatalf("data should be array, got %T", data)
	}
	if len(arr) != 1 {
		t.Fatalf("data length = %d, want 1", len(arr))
	}
	obj, ok := arr[0].(map[string]any)
	if !ok {
		t.Fatalf("data[0] should be object, got %T", arr[0])
	}
	if obj["a"] != float64(1) {
		t.Fatalf("data[0].a = %v, want 1", obj["a"])
	}
	if _, exists := obj["b"]; !exists {
		t.Fatalf("data[0].b missing")
	}
	if obj["b"] != nil {
		t.Fatalf("data[0].b = %v, want null", obj["b"])
	}
}

func TestEmbeddedJSON_MarshalYAML_PreservesNull(t *testing.T) {
	r := OK("ok", NewEmbeddedJSON([]byte(`[{"reference":null,"system-id":7451234567890123456}]`)))
	out, err := r.YAML()
	if err != nil {
		t.Fatalf("YAML() failed: %v", err)
	}

	var decoded map[string]any
	if err := yaml.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("unmarshal result yaml failed: %v", err)
	}

	data, ok := decoded["data"]
	if !ok {
		t.Fatalf("missing data field")
	}
	arr, ok := data.([]any)
	if !ok {
		t.Fatalf("data should be array, got %T", data)
	}
	obj, ok := arr[0].(map[string]any)
	if !ok {
		t.Fatalf("data[0] should be object, got %T", arr[0])
	}
	if obj["reference"] != nil {
		t.Fatalf("reference = %v, want null", obj["reference"])
	}
	if _, ok := obj["system-id"]; !ok {
		t.Fatalf("system-id missing")
	}
}

func TestEmbeddedJSON_InvalidJSON_ReturnsError(t *testing.T) {
	r := OK("ok", NewEmbeddedJSON([]byte(`not json`)))
	if _, err := r.JSON(); err == nil {
		t.Fatalf("expected error for invalid embedded json, got nil")
	}
}

func TestEmbeddedJSON_InvalidJSON_YAML_ReturnsError(t *testing.T) {
	r := OK("ok", NewEmbeddedJSON([]byte(`not json`)))
	if _, err := r.YAML(); err == nil {
		t.Fatalf("expected error for invalid embedded json in YAML, got nil")
	}
}
