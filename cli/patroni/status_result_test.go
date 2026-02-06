/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>

Tests for pt status structured output result and DTO.
*/
package patroni

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// ============================================================================
// PtStatusResultData JSON Serialization Tests
// ============================================================================

func TestPtStatusResultData_JSONSerialization(t *testing.T) {
	lag := 0
	data := &PtStatusResultData{
		Cluster:        "pg-test",
		Leader:         "pg-test-1",
		Timeline:       3,
		MemberCount:    2,
		ServiceRunning: true,
		Members: []PtMemberSummary{
			{Member: "pg-test-1", Host: "10.0.0.1", Role: "leader", State: "running", TL: 3, Lag: nil},
			{Member: "pg-test-2", Host: "10.0.0.2", Role: "replica", State: "running", TL: 3, Lag: &lag},
		},
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("unexpected marshal error: %v", err)
	}

	var decoded PtStatusResultData
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}

	if decoded.Cluster != "pg-test" {
		t.Errorf("expected cluster 'pg-test', got '%s'", decoded.Cluster)
	}
	if decoded.Leader != "pg-test-1" {
		t.Errorf("expected leader 'pg-test-1', got '%s'", decoded.Leader)
	}
	if decoded.Timeline != 3 {
		t.Errorf("expected timeline 3, got %d", decoded.Timeline)
	}
	if decoded.MemberCount != 2 {
		t.Errorf("expected member_count 2, got %d", decoded.MemberCount)
	}
	if !decoded.ServiceRunning {
		t.Error("expected service_running true")
	}
	if len(decoded.Members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(decoded.Members))
	}
	if decoded.Members[0].Lag != nil {
		t.Errorf("expected nil lag for leader, got %v", *decoded.Members[0].Lag)
	}
	if decoded.Members[1].Lag == nil {
		t.Error("expected non-nil lag for replica")
	} else if *decoded.Members[1].Lag != 0 {
		t.Errorf("expected lag 0, got %d", *decoded.Members[1].Lag)
	}
}

func TestPtStatusResultData_YAMLSerialization(t *testing.T) {
	lag := 5
	data := &PtStatusResultData{
		Cluster:        "pg-ha",
		Leader:         "pg-ha-1",
		Timeline:       7,
		MemberCount:    3,
		ServiceRunning: true,
		Members: []PtMemberSummary{
			{Member: "pg-ha-1", Host: "10.0.0.1", Role: "leader", State: "running", TL: 7, Lag: nil},
			{Member: "pg-ha-2", Host: "10.0.0.2", Role: "replica", State: "running", TL: 7, Lag: &lag},
			{Member: "pg-ha-3", Host: "10.0.0.3", Role: "replica", State: "streaming", TL: 7, Lag: &lag},
		},
	}

	yamlBytes, err := yaml.Marshal(data)
	if err != nil {
		t.Fatalf("unexpected marshal error: %v", err)
	}

	var decoded PtStatusResultData
	if err := yaml.Unmarshal(yamlBytes, &decoded); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}

	if decoded.Cluster != "pg-ha" {
		t.Errorf("expected cluster 'pg-ha', got '%s'", decoded.Cluster)
	}
	if decoded.Leader != "pg-ha-1" {
		t.Errorf("expected leader 'pg-ha-1', got '%s'", decoded.Leader)
	}
	if decoded.MemberCount != 3 {
		t.Errorf("expected member_count 3, got %d", decoded.MemberCount)
	}
	if len(decoded.Members) != 3 {
		t.Fatalf("expected 3 members, got %d", len(decoded.Members))
	}
}

// ============================================================================
// Leader Identification Tests
// ============================================================================

func TestExtractLeaderAndTimeline_Normal(t *testing.T) {
	members := []PtMemberSummary{
		{Member: "pg-test-1", Role: "leader", TL: 5},
		{Member: "pg-test-2", Role: "replica", TL: 5},
	}
	leader, tl := extractLeaderAndTimeline(members)
	if leader != "pg-test-1" {
		t.Errorf("expected leader 'pg-test-1', got '%s'", leader)
	}
	if tl != 5 {
		t.Errorf("expected timeline 5, got %d", tl)
	}
}

func TestExtractLeaderAndTimeline_NoLeader(t *testing.T) {
	members := []PtMemberSummary{
		{Member: "pg-test-1", Role: "replica", TL: 3},
		{Member: "pg-test-2", Role: "replica", TL: 3},
	}
	leader, tl := extractLeaderAndTimeline(members)
	if leader != "" {
		t.Errorf("expected empty leader, got '%s'", leader)
	}
	if tl != 0 {
		t.Errorf("expected timeline 0, got %d", tl)
	}
}

func TestExtractLeaderAndTimeline_Empty(t *testing.T) {
	leader, tl := extractLeaderAndTimeline(nil)
	if leader != "" {
		t.Errorf("expected empty leader, got '%s'", leader)
	}
	if tl != 0 {
		t.Errorf("expected timeline 0, got %d", tl)
	}
}

func TestExtractLeaderAndTimeline_MultipleLeaders(t *testing.T) {
	// Edge case: if multiple leaders exist, return the first one
	members := []PtMemberSummary{
		{Member: "pg-test-1", Role: "leader", TL: 5},
		{Member: "pg-test-2", Role: "leader", TL: 6},
	}
	leader, tl := extractLeaderAndTimeline(members)
	if leader != "pg-test-1" {
		t.Errorf("expected first leader 'pg-test-1', got '%s'", leader)
	}
	if tl != 5 {
		t.Errorf("expected timeline 5, got %d", tl)
	}
}

func TestExtractLeaderAndTimeline_StandbyLeader(t *testing.T) {
	members := []PtMemberSummary{
		{Member: "pg-standby-1", Role: "standby_leader", TL: 7},
		{Member: "pg-standby-2", Role: "replica", TL: 7},
	}
	leader, tl := extractLeaderAndTimeline(members)
	if leader != "pg-standby-1" {
		t.Errorf("expected standby leader 'pg-standby-1', got '%s'", leader)
	}
	if tl != 7 {
		t.Errorf("expected timeline 7, got %d", tl)
	}
}

// ============================================================================
// Timeline Extraction Tests
// ============================================================================

func TestExtractLeaderAndTimeline_HighTimeline(t *testing.T) {
	members := []PtMemberSummary{
		{Member: "pg-prod-1", Role: "leader", TL: 42},
	}
	_, tl := extractLeaderAndTimeline(members)
	if tl != 42 {
		t.Errorf("expected timeline 42, got %d", tl)
	}
}

// ============================================================================
// Lag Field null Handling Tests
// ============================================================================

func TestPtStatusResultData_LagNullHandling(t *testing.T) {
	lag := 10
	data := &PtStatusResultData{
		Members: []PtMemberSummary{
			{Member: "leader-1", Role: "leader", Lag: nil},
			{Member: "replica-1", Role: "replica", Lag: &lag},
		},
		MemberCount: 2,
	}

	b, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var decoded PtStatusResultData
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if decoded.Members[0].Lag != nil {
		t.Errorf("expected nil lag for leader, got %v", *decoded.Members[0].Lag)
	}
	if decoded.Members[1].Lag == nil {
		t.Error("expected non-nil lag for replica")
	} else if *decoded.Members[1].Lag != 10 {
		t.Errorf("expected lag 10, got %d", *decoded.Members[1].Lag)
	}
}

// ============================================================================
// Empty Member List Tests
// ============================================================================

func TestPtStatusResultData_EmptyMembers(t *testing.T) {
	data := &PtStatusResultData{
		Cluster:        "pg-empty",
		Members:        []PtMemberSummary{},
		MemberCount:    0,
		ServiceRunning: true,
	}

	b, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var decoded PtStatusResultData
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if decoded.MemberCount != 0 {
		t.Errorf("expected member_count 0, got %d", decoded.MemberCount)
	}
	if len(decoded.Members) != 0 {
		t.Errorf("expected 0 members, got %d", len(decoded.Members))
	}
}

func TestPtStatusResultData_NilMembers(t *testing.T) {
	data := &PtStatusResultData{
		Cluster:        "pg-nil",
		Members:        nil,
		MemberCount:    0,
		ServiceRunning: false,
	}

	b, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var decoded PtStatusResultData
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if decoded.MemberCount != 0 {
		t.Errorf("expected member_count 0, got %d", decoded.MemberCount)
	}
}

// ============================================================================
// Invalid JSON Parse Tests
// ============================================================================

func TestStatusResult_InvalidJSONParsing(t *testing.T) {
	// Test that parsePatroniListJSON handles invalid JSON (already tested in list_result_test.go)
	// but verify the behavior expected by StatusResult
	_, err := parsePatroniListJSON("not valid json")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}

	_, err = parsePatroniListJSON("")
	if err == nil {
		t.Error("expected error for empty string")
	}

	_, err = parsePatroniListJSON("{}")
	if err == nil {
		t.Error("expected error for non-array JSON")
	}
}

// ============================================================================
// Nil Receiver Safety Tests
// ============================================================================

func TestPtStatusResultData_NilReceiver(t *testing.T) {
	var data *PtStatusResultData
	text := data.Text()
	if text != "" {
		t.Errorf("expected empty string for nil receiver, got %q", text)
	}
}

// ============================================================================
// Service Not Running + Partial Data Tests
// ============================================================================

func TestPtStatusResultData_ServiceNotRunning(t *testing.T) {
	data := &PtStatusResultData{
		Cluster:        "pg-down",
		ServiceRunning: false,
		MemberCount:    0,
		Members:        nil,
	}

	b, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var decoded PtStatusResultData
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if decoded.ServiceRunning {
		t.Error("expected service_running false")
	}
	if decoded.Cluster != "pg-down" {
		t.Errorf("expected cluster 'pg-down', got '%s'", decoded.Cluster)
	}
}

// ============================================================================
// Text Output Tests
// ============================================================================

func TestPtStatusResultData_Text(t *testing.T) {
	lag := 0
	data := &PtStatusResultData{
		Cluster:        "pg-test",
		Leader:         "pg-test-1",
		Timeline:       3,
		MemberCount:    2,
		ServiceRunning: true,
		Members: []PtMemberSummary{
			{Member: "pg-test-1", Host: "10.0.0.1", Role: "leader", State: "running", TL: 3, Lag: nil},
			{Member: "pg-test-2", Host: "10.0.0.2", Role: "replica", State: "running", TL: 3, Lag: &lag},
		},
	}
	text := data.Text()
	if text == "" {
		t.Error("expected non-empty text output")
	}
	if !strings.Contains(text, "Cluster: pg-test") {
		t.Error("expected cluster name in text output")
	}
	if !strings.Contains(text, "Leader: pg-test-1") {
		t.Error("expected leader name in text output")
	}
	if !strings.Contains(text, "Timeline: 3") {
		t.Error("expected timeline in text output")
	}
	if !strings.Contains(text, "Service Running: true") {
		t.Error("expected service running status in text output")
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

func TestPtStatusResultData_TextEmpty(t *testing.T) {
	data := &PtStatusResultData{
		MemberCount:    0,
		ServiceRunning: false,
		Members:        []PtMemberSummary{},
	}
	text := data.Text()
	if !strings.Contains(text, "Members: 0") {
		t.Error("expected 'Members: 0' in text output")
	}
	if !strings.Contains(text, "Service Running: false") {
		t.Error("expected 'Service Running: false' in text output")
	}
	// No cluster line when cluster is empty
	if strings.Contains(text, "Cluster:") {
		t.Error("expected no cluster line when cluster is empty")
	}
	// No leader line when leader is empty
	if strings.Contains(text, "Leader:") {
		t.Error("expected no leader line when leader is empty")
	}
	// No timeline line when timeline is 0
	if strings.Contains(text, "Timeline:") {
		t.Error("expected no timeline line when timeline is 0")
	}
}

func TestPtStatusResultData_TextPartialData(t *testing.T) {
	// Simulate partial data when service is not running
	data := &PtStatusResultData{
		Cluster:        "pg-partial",
		ServiceRunning: false,
		MemberCount:    0,
	}
	text := data.Text()
	if !strings.Contains(text, "Cluster: pg-partial") {
		t.Error("expected cluster name in partial data text output")
	}
	if !strings.Contains(text, "Service Running: false") {
		t.Error("expected service not running in partial data text output")
	}
}

// ============================================================================
// JSON Field Name Tests
// ============================================================================

func TestPtStatusResultData_JSONFieldNames(t *testing.T) {
	data := &PtStatusResultData{
		Cluster:        "test",
		Leader:         "n1",
		Timeline:       1,
		MemberCount:    1,
		ServiceRunning: true,
		Members: []PtMemberSummary{
			{Member: "n1", Host: "h1", Role: "leader", State: "running", TL: 1},
		},
	}

	b, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	jsonStr := string(b)

	// Verify JSON field names match spec
	expectedFields := []string{
		`"cluster"`,
		`"leader"`,
		`"timeline"`,
		`"member_count"`,
		`"service_running"`,
		`"members"`,
	}
	for _, field := range expectedFields {
		if !strings.Contains(jsonStr, field) {
			t.Errorf("expected JSON to contain field %s, got: %s", field, jsonStr)
		}
	}
}

// ============================================================================
// ServiceRunning always present in JSON (not omitempty)
// ============================================================================

func TestPtStatusResultData_ServiceRunningAlwaysPresent(t *testing.T) {
	data := &PtStatusResultData{
		ServiceRunning: false,
		MemberCount:    0,
	}

	b, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	jsonStr := string(b)
	if !strings.Contains(jsonStr, `"service_running":false`) {
		t.Errorf("expected service_running:false in JSON, got: %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"member_count":0`) {
		t.Errorf("expected member_count:0 in JSON, got: %s", jsonStr)
	}
}
