/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

Tests for pt list structured output result and DTO.
*/
package patroni

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParsePatroniListJSON_Normal(t *testing.T) {
	input := `[
  {
    "Member": "pg-test-1",
    "Host": "10.0.0.1",
    "Role": "Leader",
    "State": "running",
    "TL": 1,
    "Lag in MB": null
  },
  {
    "Member": "pg-test-2",
    "Host": "10.0.0.2",
    "Role": "Replica",
    "State": "running",
    "TL": 1,
    "Lag in MB": 0
  }
]`
	data, err := parsePatroniListJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data == nil {
		t.Fatal("expected non-nil data")
	}
	if len(data.Members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(data.Members))
	}

	// Check leader
	m0 := data.Members[0]
	if m0.Member != "pg-test-1" {
		t.Errorf("expected member pg-test-1, got %s", m0.Member)
	}
	if m0.Host != "10.0.0.1" {
		t.Errorf("expected host 10.0.0.1, got %s", m0.Host)
	}
	if m0.Role != "leader" {
		t.Errorf("expected role 'leader', got '%s'", m0.Role)
	}
	if m0.State != "running" {
		t.Errorf("expected state 'running', got '%s'", m0.State)
	}
	if m0.TL != 1 {
		t.Errorf("expected TL 1, got %d", m0.TL)
	}
	if m0.Lag != nil {
		t.Errorf("expected nil lag for leader, got %v", *m0.Lag)
	}

	// Check replica
	m1 := data.Members[1]
	if m1.Member != "pg-test-2" {
		t.Errorf("expected member pg-test-2, got %s", m1.Member)
	}
	if m1.Role != "replica" {
		t.Errorf("expected role 'replica', got '%s'", m1.Role)
	}
	if m1.Lag == nil {
		t.Error("expected non-nil lag for replica")
	} else if *m1.Lag != 0 {
		t.Errorf("expected lag 0, got %d", *m1.Lag)
	}
}

func TestParsePatroniListJSON_Empty(t *testing.T) {
	input := `[]`
	data, err := parsePatroniListJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data == nil {
		t.Fatal("expected non-nil data")
	}
	if len(data.Members) != 0 {
		t.Errorf("expected 0 members, got %d", len(data.Members))
	}
}

func TestParsePatroniListJSON_InvalidJSON(t *testing.T) {
	input := `not valid json`
	data, err := parsePatroniListJSON(input)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if data != nil {
		t.Error("expected nil data for invalid JSON")
	}
}

func TestParsePatroniListJSON_SingleMember(t *testing.T) {
	input := `[
  {
    "Member": "pg-meta-1",
    "Host": "10.10.10.10",
    "Role": "Leader",
    "State": "running",
    "TL": 5,
    "Lag in MB": null
  }
]`
	data, err := parsePatroniListJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data.Members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(data.Members))
	}
	if data.Members[0].TL != 5 {
		t.Errorf("expected TL 5, got %d", data.Members[0].TL)
	}
}

func TestParsePatroniListJSON_StandbyLeader(t *testing.T) {
	input := `[
  {
    "Member": "pg-standby-1",
    "Host": "10.0.0.3",
    "Role": "Standby Leader",
    "State": "running",
    "TL": 2,
    "Lag in MB": 5
  }
]`
	data, err := parsePatroniListJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.Members[0].Role != "standby_leader" {
		t.Errorf("expected role 'standby_leader', got '%s'", data.Members[0].Role)
	}
	if data.Members[0].Lag == nil {
		t.Error("expected non-nil lag")
	} else if *data.Members[0].Lag != 5 {
		t.Errorf("expected lag 5, got %d", *data.Members[0].Lag)
	}
}

func TestPtListResultData_JSONSerialization(t *testing.T) {
	lag := 0
	data := &PtListResultData{
		Cluster: "pg-test",
		Members: []PtMemberSummary{
			{Member: "pg-test-1", Host: "10.0.0.1", Role: "leader", State: "running", TL: 1, Lag: nil},
			{Member: "pg-test-2", Host: "10.0.0.2", Role: "replica", State: "running", TL: 1, Lag: &lag},
		},
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("unexpected marshal error: %v", err)
	}

	var decoded PtListResultData
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}

	if decoded.Cluster != "pg-test" {
		t.Errorf("expected cluster 'pg-test', got '%s'", decoded.Cluster)
	}
	if len(decoded.Members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(decoded.Members))
	}
	if decoded.Members[0].Lag != nil {
		t.Errorf("expected nil lag for leader after roundtrip, got %v", *decoded.Members[0].Lag)
	}
	if decoded.Members[1].Lag == nil {
		t.Error("expected non-nil lag for replica after roundtrip")
	} else if *decoded.Members[1].Lag != 0 {
		t.Errorf("expected lag 0, got %d", *decoded.Members[1].Lag)
	}
}

func TestPtMemberSummary_LagNullHandling(t *testing.T) {
	// Test that nil Lag serializes to JSON null
	m := PtMemberSummary{Member: "a", Lag: nil}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	jsonStr := string(b)
	// Check that "lag":null is present
	var decoded map[string]interface{}
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decoded["lag"] != nil {
		t.Errorf("expected null lag in JSON, got %v, json=%s", decoded["lag"], jsonStr)
	}

	// Test that non-nil Lag serializes to number
	lag := 42
	m2 := PtMemberSummary{Member: "b", Lag: &lag}
	b2, err := json.Marshal(m2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var decoded2 map[string]interface{}
	if err := json.Unmarshal(b2, &decoded2); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decoded2["lag"] != float64(42) {
		t.Errorf("expected lag 42, got %v", decoded2["lag"])
	}
}

func TestPtListResultData_NilReceiver(t *testing.T) {
	var data *PtListResultData
	text := data.Text()
	if text != "" {
		t.Errorf("expected empty string for nil receiver, got %q", text)
	}
}

func TestPtListResultData_Text(t *testing.T) {
	lag := 0
	data := &PtListResultData{
		Cluster: "pg-test",
		Members: []PtMemberSummary{
			{Member: "pg-test-1", Host: "10.0.0.1", Role: "leader", State: "running", TL: 1, Lag: nil},
			{Member: "pg-test-2", Host: "10.0.0.2", Role: "replica", State: "running", TL: 1, Lag: &lag},
		},
	}
	text := data.Text()
	if text == "" {
		t.Error("expected non-empty text output")
	}
	if !strings.Contains(text, "Cluster: pg-test") {
		t.Error("expected cluster name in text output")
	}
	if !strings.Contains(text, "Members: 2") {
		t.Error("expected member count in text output")
	}
	if !strings.Contains(text, "pg-test-1") {
		t.Error("expected member pg-test-1 in text output")
	}
	if !strings.Contains(text, "null") {
		t.Error("expected 'null' for leader lag in text output")
	}
	if !strings.Contains(text, "0 MB") {
		t.Error("expected '0 MB' for replica lag in text output")
	}
}

func TestPtListResultData_TextEmpty(t *testing.T) {
	data := &PtListResultData{
		Cluster: "",
		Members: []PtMemberSummary{},
	}
	text := data.Text()
	if !strings.Contains(text, "Members: 0") {
		t.Error("expected 'Members: 0' in text output")
	}
	// No cluster line when cluster is empty
	if strings.Contains(text, "Cluster:") {
		t.Error("expected no cluster line when cluster is empty")
	}
}

func TestParsePatroniListJSON_LargeLag(t *testing.T) {
	input := `[
  {
    "Member": "pg-test-3",
    "Host": "10.0.0.3",
    "Role": "Replica",
    "State": "running",
    "TL": 1,
    "Lag in MB": 1024
  }
]`
	data, err := parsePatroniListJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.Members[0].Lag == nil {
		t.Fatal("expected non-nil lag")
	}
	if *data.Members[0].Lag != 1024 {
		t.Errorf("expected lag 1024, got %d", *data.Members[0].Lag)
	}
}

func TestParsePatroniListJSON_RoleLowerCase(t *testing.T) {
	// Patronictl outputs PascalCase roles, our DTO should lowercase them
	tests := []struct {
		input    string
		expected string
	}{
		{`"Leader"`, "leader"},
		{`"Replica"`, "replica"},
		{`"Standby Leader"`, "standby_leader"},
		{`"Sync Standby"`, "sync_standby"},
	}

	for _, tc := range tests {
		input := `[{"Member":"a","Host":"h","Role":` + tc.input + `,"State":"running","TL":1,"Lag in MB":null}]`
		data, err := parsePatroniListJSON(input)
		if err != nil {
			t.Fatalf("unexpected error for role %s: %v", tc.input, err)
		}
		if data.Members[0].Role != tc.expected {
			t.Errorf("for input role %s: expected '%s', got '%s'", tc.input, tc.expected, data.Members[0].Role)
		}
	}
}

func TestGetClusterName(t *testing.T) {
	// Test with valid YAML config content
	yamlContent := `scope: pg-test
namespace: /service/
name: pg-test-1
`
	name := parseClusterNameFromYAML(yamlContent)
	if name != "pg-test" {
		t.Errorf("expected 'pg-test', got '%s'", name)
	}

	// Test with empty content
	name = parseClusterNameFromYAML("")
	if name != "" {
		t.Errorf("expected empty string, got '%s'", name)
	}

	// Test with YAML without scope field
	yamlContent2 := `name: pg-test-1
namespace: /service/
`
	name = parseClusterNameFromYAML(yamlContent2)
	if name != "" {
		t.Errorf("expected empty string, got '%s'", name)
	}

	// Test with invalid YAML
	name = parseClusterNameFromYAML(":\n  :\n  invalid: [yaml")
	if name != "" {
		t.Errorf("expected empty string for invalid YAML, got '%s'", name)
	}
}

func TestIsConfigNotFound(t *testing.T) {
	tests := []struct {
		name   string
		errMsg string
		output string
		want   bool
	}{
		{
			name:   "no such file output",
			errMsg: "command failed: exit status 1",
			output: "Could not open config file /etc/patroni/patroni.yml: No such file or directory",
			want:   true,
		},
		{
			name:   "generic config not found",
			errMsg: "config not found",
			output: "",
			want:   true,
		},
		{
			name:   "permission denied",
			errMsg: "permission denied",
			output: "",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isConfigNotFound(testErr(tt.errMsg), tt.output)
			if got != tt.want {
				t.Fatalf("isConfigNotFound(%q,%q)=%v, want %v", tt.errMsg, tt.output, got, tt.want)
			}
		})
	}
}

type testErr string

func (e testErr) Error() string { return string(e) }
