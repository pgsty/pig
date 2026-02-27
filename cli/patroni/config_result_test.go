/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

Tests for pt config show structured output result and DTO.
*/
package patroni

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// ============================================================================
// parseShowConfigOutput Tests
// ============================================================================

func TestParseShowConfigOutput_FullConfig(t *testing.T) {
	yamlStr := `loop_wait: 10
ttl: 30
retry_timeout: 10
maximum_lag_on_failover: 1048576
maximum_lag_on_syncnode: -1
postgresql:
  parameters:
    max_connections: 100
    shared_buffers: 256MB
    work_mem: 4MB
    maintenance_work_mem: 64MB
    max_wal_senders: 10
    max_replication_slots: 10
    wal_level: replica
  use_pg_rewind: true
  use_slots: true
`

	data, err := parseShowConfigOutput(yamlStr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data == nil {
		t.Fatal("expected non-nil data")
	}

	// Check typed integer fields
	if data.LoopWait == nil || *data.LoopWait != 10 {
		t.Errorf("expected loop_wait=10, got %v", data.LoopWait)
	}
	if data.TTL == nil || *data.TTL != 30 {
		t.Errorf("expected ttl=30, got %v", data.TTL)
	}
	if data.RetryTimeout == nil || *data.RetryTimeout != 10 {
		t.Errorf("expected retry_timeout=10, got %v", data.RetryTimeout)
	}
	if data.MaximumLagOnFailover == nil || *data.MaximumLagOnFailover != 1048576 {
		t.Errorf("expected maximum_lag_on_failover=1048576, got %v", data.MaximumLagOnFailover)
	}
	if data.MaximumLagOnSyncnode == nil || *data.MaximumLagOnSyncnode != -1 {
		t.Errorf("expected maximum_lag_on_syncnode=-1, got %v", data.MaximumLagOnSyncnode)
	}

	// Check postgresql map
	if data.PostgreSQL == nil {
		t.Fatal("expected non-nil postgresql")
	}
	params, ok := data.PostgreSQL["parameters"].(map[string]interface{})
	if !ok {
		t.Fatal("expected postgresql.parameters to be a map")
	}
	if params["max_connections"] != 100 {
		t.Errorf("expected max_connections=100, got %v", params["max_connections"])
	}

	// Check Raw contains all keys
	if data.Raw == nil {
		t.Fatal("expected non-nil Raw")
	}
	if _, ok := data.Raw["loop_wait"]; !ok {
		t.Error("expected Raw to contain loop_wait")
	}
	if _, ok := data.Raw["postgresql"]; !ok {
		t.Error("expected Raw to contain postgresql")
	}
}

func TestParseShowConfigOutput_MinimalConfig(t *testing.T) {
	yamlStr := `loop_wait: 10
ttl: 30
`

	data, err := parseShowConfigOutput(yamlStr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if data.LoopWait == nil || *data.LoopWait != 10 {
		t.Errorf("expected loop_wait=10, got %v", data.LoopWait)
	}
	if data.TTL == nil || *data.TTL != 30 {
		t.Errorf("expected ttl=30, got %v", data.TTL)
	}

	// Other fields should be nil
	if data.RetryTimeout != nil {
		t.Errorf("expected nil retry_timeout, got %v", data.RetryTimeout)
	}
	if data.MaximumLagOnFailover != nil {
		t.Errorf("expected nil maximum_lag_on_failover, got %v", data.MaximumLagOnFailover)
	}
	if data.PostgreSQL != nil {
		t.Errorf("expected nil postgresql, got %v", data.PostgreSQL)
	}
	if data.Standby != nil {
		t.Errorf("expected nil standby_cluster, got %v", data.Standby)
	}
	if data.Slots != nil {
		t.Errorf("expected nil slots, got %v", data.Slots)
	}
}

func TestParseShowConfigOutput_EmptyString(t *testing.T) {
	_, err := parseShowConfigOutput("")
	if err == nil {
		t.Error("expected error for empty string")
	}
}

func TestParseShowConfigOutput_WhitespaceOnly(t *testing.T) {
	_, err := parseShowConfigOutput("   \n  \t  \n")
	if err == nil {
		t.Error("expected error for whitespace-only string")
	}
}

func TestParseShowConfigOutput_InvalidYAML(t *testing.T) {
	_, err := parseShowConfigOutput("{{invalid yaml content}}")
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestParseShowConfigOutput_UnknownKeysInRaw(t *testing.T) {
	yamlStr := `loop_wait: 10
ttl: 30
custom_key: custom_value
another_unknown:
  nested: true
`

	data, err := parseShowConfigOutput(yamlStr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Known fields extracted
	if data.LoopWait == nil || *data.LoopWait != 10 {
		t.Errorf("expected loop_wait=10, got %v", data.LoopWait)
	}

	// Unknown keys preserved in Raw
	if data.Raw["custom_key"] != "custom_value" {
		t.Errorf("expected custom_key in Raw, got %v", data.Raw["custom_key"])
	}
	nested, ok := data.Raw["another_unknown"].(map[string]interface{})
	if !ok {
		t.Fatal("expected another_unknown to be a map in Raw")
	}
	if nested["nested"] != true {
		t.Errorf("expected another_unknown.nested=true, got %v", nested["nested"])
	}
}

func TestParseShowConfigOutput_StandbyCluster(t *testing.T) {
	yamlStr := `loop_wait: 10
ttl: 30
standby_cluster:
  host: 10.0.0.1
  port: 5432
  create_replica_methods:
  - basebackup
`

	data, err := parseShowConfigOutput(yamlStr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if data.Standby == nil {
		t.Fatal("expected non-nil standby_cluster")
	}
	if data.Standby["host"] != "10.0.0.1" {
		t.Errorf("expected standby host=10.0.0.1, got %v", data.Standby["host"])
	}
}

func TestParseShowConfigOutput_Slots(t *testing.T) {
	yamlStr := `loop_wait: 10
ttl: 30
slots:
  my_slot:
    type: physical
`

	data, err := parseShowConfigOutput(yamlStr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if data.Slots == nil {
		t.Fatal("expected non-nil slots")
	}
	mySlot, ok := data.Slots["my_slot"].(map[string]interface{})
	if !ok {
		t.Fatal("expected my_slot to be a map")
	}
	if mySlot["type"] != "physical" {
		t.Errorf("expected slot type=physical, got %v", mySlot["type"])
	}
}

// ============================================================================
// toInt Helper Tests
// ============================================================================

func TestToInt_IntValue(t *testing.T) {
	n, ok := toInt(42)
	if !ok || n != 42 {
		t.Errorf("expected (42, true), got (%d, %v)", n, ok)
	}
}

func TestToInt_Float64Value(t *testing.T) {
	n, ok := toInt(float64(42))
	if !ok || n != 42 {
		t.Errorf("expected (42, true), got (%d, %v)", n, ok)
	}
}

func TestToInt_Int64Value(t *testing.T) {
	n, ok := toInt(int64(42))
	if !ok || n != 42 {
		t.Errorf("expected (42, true), got (%d, %v)", n, ok)
	}
}

func TestToInt_StringValue(t *testing.T) {
	_, ok := toInt("42")
	if ok {
		t.Error("expected false for string input")
	}
}

func TestToInt_NilValue(t *testing.T) {
	_, ok := toInt(nil)
	if ok {
		t.Error("expected false for nil input")
	}
}

func TestToInt_NegativeInt(t *testing.T) {
	n, ok := toInt(-1)
	if !ok || n != -1 {
		t.Errorf("expected (-1, true), got (%d, %v)", n, ok)
	}
}

// ============================================================================
// PtConfigResultData JSON/YAML Serialization Tests
// ============================================================================

func TestPtConfigResultData_JSONSerialization(t *testing.T) {
	lw := 10
	ttl := 30
	data := &PtConfigResultData{
		LoopWait: &lw,
		TTL:      &ttl,
		PostgreSQL: map[string]interface{}{
			"parameters": map[string]interface{}{
				"max_connections": 100,
			},
			"use_pg_rewind": true,
		},
		Raw: map[string]interface{}{
			"loop_wait":  10,
			"ttl":        30,
			"postgresql": map[string]interface{}{},
		},
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("unexpected marshal error: %v", err)
	}

	var decoded PtConfigResultData
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}

	if decoded.LoopWait == nil || *decoded.LoopWait != 10 {
		t.Errorf("expected loop_wait=10 after roundtrip, got %v", decoded.LoopWait)
	}
	if decoded.TTL == nil || *decoded.TTL != 30 {
		t.Errorf("expected ttl=30 after roundtrip, got %v", decoded.TTL)
	}
	if decoded.PostgreSQL == nil {
		t.Error("expected non-nil postgresql after roundtrip")
	}
}

func TestPtConfigResultData_YAMLSerialization(t *testing.T) {
	lw := 10
	ttl := 30
	data := &PtConfigResultData{
		LoopWait: &lw,
		TTL:      &ttl,
		Raw: map[string]interface{}{
			"loop_wait": 10,
			"ttl":       30,
		},
	}

	yamlBytes, err := yaml.Marshal(data)
	if err != nil {
		t.Fatalf("unexpected marshal error: %v", err)
	}

	var decoded PtConfigResultData
	if err := yaml.Unmarshal(yamlBytes, &decoded); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}

	if decoded.LoopWait == nil || *decoded.LoopWait != 10 {
		t.Errorf("expected loop_wait=10 after roundtrip, got %v", decoded.LoopWait)
	}
	if decoded.TTL == nil || *decoded.TTL != 30 {
		t.Errorf("expected ttl=30 after roundtrip, got %v", decoded.TTL)
	}
}

func TestPtConfigResultData_JSONOmitempty(t *testing.T) {
	// All nil fields should be omitted
	data := &PtConfigResultData{
		Raw: map[string]interface{}{"only_raw": true},
	}

	b, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	jsonStr := string(b)

	// These fields should NOT appear when nil
	omittedFields := []string{"loop_wait", "ttl", "retry_timeout", "maximum_lag_on_failover",
		"maximum_lag_on_syncnode", "postgresql", "standby_cluster", "slots", "ignore_slots"}
	for _, field := range omittedFields {
		if strings.Contains(jsonStr, `"`+field+`"`) {
			t.Errorf("expected field %s to be omitted in JSON, got: %s", field, jsonStr)
		}
	}

	// Raw should be present
	if !strings.Contains(jsonStr, `"raw"`) {
		t.Errorf("expected raw field in JSON, got: %s", jsonStr)
	}
}

func TestPtConfigResultData_JSONFieldNames(t *testing.T) {
	lw := 10
	ttl := 30
	rt := 10
	mlf := 1048576
	mls := -1
	data := &PtConfigResultData{
		LoopWait:             &lw,
		TTL:                  &ttl,
		RetryTimeout:         &rt,
		MaximumLagOnFailover: &mlf,
		MaximumLagOnSyncnode: &mls,
		PostgreSQL:           map[string]interface{}{"use_slots": true},
		Standby:              map[string]interface{}{"host": "10.0.0.1"},
		Slots:                map[string]interface{}{"slot1": "physical"},
		IgnoreSlots:          []interface{}{"ignored"},
		Raw:                  map[string]interface{}{"loop_wait": 10},
	}

	b, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	jsonStr := string(b)

	expectedFields := []string{
		`"loop_wait"`, `"ttl"`, `"retry_timeout"`,
		`"maximum_lag_on_failover"`, `"maximum_lag_on_syncnode"`,
		`"postgresql"`, `"standby_cluster"`, `"slots"`, `"ignore_slots"`, `"raw"`,
	}
	for _, field := range expectedFields {
		if !strings.Contains(jsonStr, field) {
			t.Errorf("expected JSON to contain field %s, got: %s", field, jsonStr)
		}
	}
}

// ============================================================================
// Nil Receiver Safety Tests
// ============================================================================

func TestPtConfigResultData_NilReceiver(t *testing.T) {
	var data *PtConfigResultData
	text := data.Text()
	if text != "" {
		t.Errorf("expected empty string for nil receiver, got %q", text)
	}
}

// ============================================================================
// Text Output Tests
// ============================================================================

func TestPtConfigResultData_Text(t *testing.T) {
	lw := 10
	ttl := 30
	data := &PtConfigResultData{
		LoopWait: &lw,
		TTL:      &ttl,
		PostgreSQL: map[string]interface{}{
			"parameters": map[string]interface{}{
				"max_connections": 100,
			},
		},
		Raw: map[string]interface{}{
			"loop_wait":  10,
			"ttl":        30,
			"postgresql": map[string]interface{}{},
		},
	}
	text := data.Text()
	if text == "" {
		t.Error("expected non-empty text output")
	}
	if !strings.Contains(text, "loop_wait: 10") {
		t.Error("expected loop_wait in text output")
	}
	if !strings.Contains(text, "ttl: 30") {
		t.Error("expected ttl in text output")
	}
}

func TestPtConfigResultData_TextEmpty(t *testing.T) {
	data := &PtConfigResultData{
		Raw: map[string]interface{}{},
	}
	text := data.Text()
	// Should not panic, may produce empty or minimal output
	_ = text
}

func TestPtConfigResultData_TextWithAllFields(t *testing.T) {
	lw := 10
	ttl := 30
	rt := 10
	mlf := 1048576
	data := &PtConfigResultData{
		LoopWait:             &lw,
		TTL:                  &ttl,
		RetryTimeout:         &rt,
		MaximumLagOnFailover: &mlf,
		Raw: map[string]interface{}{
			"loop_wait":              10,
			"ttl":                    30,
			"retry_timeout":          10,
			"maximum_lag_on_failover": 1048576,
		},
	}
	text := data.Text()
	if !strings.Contains(text, "loop_wait") {
		t.Error("expected loop_wait in text")
	}
	if !strings.Contains(text, "ttl") {
		t.Error("expected ttl in text")
	}
}

// ============================================================================
// Known Field Extraction Tests
// ============================================================================

func TestParseShowConfigOutput_ExtractsKnownFields(t *testing.T) {
	yamlStr := `loop_wait: 5
ttl: 15
retry_timeout: 7
maximum_lag_on_failover: 2097152
maximum_lag_on_syncnode: 0
`

	data, err := parseShowConfigOutput(yamlStr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tests := []struct {
		name     string
		got      *int
		expected int
	}{
		{"loop_wait", data.LoopWait, 5},
		{"ttl", data.TTL, 15},
		{"retry_timeout", data.RetryTimeout, 7},
		{"maximum_lag_on_failover", data.MaximumLagOnFailover, 2097152},
		{"maximum_lag_on_syncnode", data.MaximumLagOnSyncnode, 0},
	}

	for _, tc := range tests {
		if tc.got == nil {
			t.Errorf("expected %s to be non-nil", tc.name)
			continue
		}
		if *tc.got != tc.expected {
			t.Errorf("expected %s=%d, got %d", tc.name, tc.expected, *tc.got)
		}
	}
}

// ============================================================================
// IgnoreSlots Field Tests
// ============================================================================

func TestParseShowConfigOutput_IgnoreSlots(t *testing.T) {
	yamlStr := `loop_wait: 10
ttl: 30
ignore_slots:
  - name: my_slot
    type: physical
  - name: other_slot
`

	data, err := parseShowConfigOutput(yamlStr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if data.IgnoreSlots == nil {
		t.Fatal("expected non-nil ignore_slots")
	}
	if len(data.IgnoreSlots) != 2 {
		t.Errorf("expected 2 ignore_slots entries, got %d", len(data.IgnoreSlots))
	}
}

// ============================================================================
// YAML integer type handling (yaml.v3 uses int, not float64)
// ============================================================================

func TestParseShowConfigOutput_YAMLIntegerTypes(t *testing.T) {
	// yaml.v3 parses integers as int, but we should handle edge cases
	yamlStr := `loop_wait: 0
ttl: 2147483647
`
	data, err := parseShowConfigOutput(yamlStr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.LoopWait == nil || *data.LoopWait != 0 {
		t.Errorf("expected loop_wait=0, got %v", data.LoopWait)
	}
	if data.TTL == nil || *data.TTL != 2147483647 {
		t.Errorf("expected ttl=2147483647, got %v", data.TTL)
	}
}
