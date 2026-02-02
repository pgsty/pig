package ancs

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestFromAnnotations_Complete tests parsing a complete set of annotations
func TestFromAnnotations_Complete(t *testing.T) {
	annotations := map[string]string{
		"name":       "pig ext add",
		"type":       "action",
		"volatility": "stable",
		"parallel":   "safe",
		"idempotent": "true",
		"risk":       "low",
		"confirm":    "none",
		"os_user":    "root",
		"cost":       "5000",
	}

	s := FromAnnotations(annotations)

	if s.Name != "pig ext add" {
		t.Errorf("Name = %q, want %q", s.Name, "pig ext add")
	}
	if s.Type != TYPE_ACTION {
		t.Errorf("Type = %q, want %q", s.Type, TYPE_ACTION)
	}
	if s.Volatility != VOLATILITY_STABLE {
		t.Errorf("Volatility = %q, want %q", s.Volatility, VOLATILITY_STABLE)
	}
	if s.Parallel != PARALLEL_SAFE {
		t.Errorf("Parallel = %q, want %q", s.Parallel, PARALLEL_SAFE)
	}
	if s.Idempotent != true {
		t.Errorf("Idempotent = %v, want %v", s.Idempotent, true)
	}
	if s.Risk != RISK_LOW {
		t.Errorf("Risk = %q, want %q", s.Risk, RISK_LOW)
	}
	if s.Confirm != CONFIRM_NONE {
		t.Errorf("Confirm = %q, want %q", s.Confirm, CONFIRM_NONE)
	}
	if s.OSUser != OS_USER_ROOT {
		t.Errorf("OSUser = %q, want %q", s.OSUser, OS_USER_ROOT)
	}
	if s.Cost != 5000 {
		t.Errorf("Cost = %d, want %d", s.Cost, 5000)
	}
}

// TestFromAnnotations_Defaults tests that default values are used for missing fields
func TestFromAnnotations_Defaults(t *testing.T) {
	annotations := map[string]string{}

	s := FromAnnotations(annotations)

	if s.Name != "" {
		t.Errorf("Name = %q, want empty", s.Name)
	}
	if s.Type != TYPE_QUERY {
		t.Errorf("Type = %q, want %q", s.Type, TYPE_QUERY)
	}
	if s.Volatility != VOLATILITY_STABLE {
		t.Errorf("Volatility = %q, want %q", s.Volatility, VOLATILITY_STABLE)
	}
	if s.Parallel != PARALLEL_SAFE {
		t.Errorf("Parallel = %q, want %q", s.Parallel, PARALLEL_SAFE)
	}
	if s.Idempotent != false {
		t.Errorf("Idempotent = %v, want %v", s.Idempotent, false)
	}
	if s.Risk != RISK_SAFE {
		t.Errorf("Risk = %q, want %q", s.Risk, RISK_SAFE)
	}
	if s.Confirm != CONFIRM_NONE {
		t.Errorf("Confirm = %q, want %q", s.Confirm, CONFIRM_NONE)
	}
	if s.OSUser != OS_USER_CURRENT {
		t.Errorf("OSUser = %q, want %q", s.OSUser, OS_USER_CURRENT)
	}
	if s.Cost != 0 {
		t.Errorf("Cost = %d, want %d", s.Cost, 0)
	}
}

// TestFromAnnotations_InvalidValues tests that invalid values fallback to defaults
func TestFromAnnotations_InvalidValues(t *testing.T) {
	annotations := map[string]string{
		"type":       "unknown",
		"volatility": "unknown",
		"parallel":   "unknown",
		"idempotent": "invalid",
		"risk":       "unknown",
		"confirm":    "unknown",
		"os_user":    "unknown",
		"cost":       "abc",
	}

	s := FromAnnotations(annotations)

	if s.Type != TYPE_QUERY {
		t.Errorf("Type = %q, want %q (fallback)", s.Type, TYPE_QUERY)
	}
	if s.Volatility != VOLATILITY_STABLE {
		t.Errorf("Volatility = %q, want %q (fallback)", s.Volatility, VOLATILITY_STABLE)
	}
	if s.Parallel != PARALLEL_SAFE {
		t.Errorf("Parallel = %q, want %q (fallback)", s.Parallel, PARALLEL_SAFE)
	}
	if s.Idempotent != false {
		t.Errorf("Idempotent = %v, want %v (fallback)", s.Idempotent, false)
	}
	if s.Risk != RISK_SAFE {
		t.Errorf("Risk = %q, want %q (fallback)", s.Risk, RISK_SAFE)
	}
	if s.Confirm != CONFIRM_NONE {
		t.Errorf("Confirm = %q, want %q (fallback)", s.Confirm, CONFIRM_NONE)
	}
	if s.OSUser != OS_USER_CURRENT {
		t.Errorf("OSUser = %q, want %q (fallback)", s.OSUser, OS_USER_CURRENT)
	}
	if s.Cost != 0 {
		t.Errorf("Cost = %d, want %d (fallback)", s.Cost, 0)
	}
}

// TestFromAnnotations_Nil tests handling of nil input
func TestFromAnnotations_Nil(t *testing.T) {
	s := FromAnnotations(nil)

	if s == nil {
		t.Fatal("FromAnnotations(nil) returned nil, want non-nil Schema with defaults")
	}
	if s.Type != TYPE_QUERY {
		t.Errorf("Type = %q, want %q", s.Type, TYPE_QUERY)
	}
	if s.Volatility != VOLATILITY_STABLE {
		t.Errorf("Volatility = %q, want %q", s.Volatility, VOLATILITY_STABLE)
	}
}

// TestFromAnnotations_AllEnumValues tests all valid enum values
func TestFromAnnotations_AllEnumValues(t *testing.T) {
	tests := []struct {
		key    string
		values []string
	}{
		{"type", []string{TYPE_QUERY, TYPE_ACTION}},
		{"volatility", []string{VOLATILITY_IMMUTABLE, VOLATILITY_STABLE, VOLATILITY_VOLATILE}},
		{"parallel", []string{PARALLEL_SAFE, PARALLEL_RESTRICTED, PARALLEL_UNSAFE}},
		{"risk", []string{RISK_SAFE, RISK_LOW, RISK_MEDIUM, RISK_HIGH, RISK_CRITICAL}},
		{"confirm", []string{CONFIRM_NONE, CONFIRM_RECOMMENDED, CONFIRM_REQUIRED}},
		{"os_user", []string{OS_USER_CURRENT, OS_USER_ROOT, OS_USER_DBSU}},
	}

	for _, tt := range tests {
		for _, val := range tt.values {
			annotations := map[string]string{tt.key: val}
			s := FromAnnotations(annotations)

			var got string
			switch tt.key {
			case "type":
				got = s.Type
			case "volatility":
				got = s.Volatility
			case "parallel":
				got = s.Parallel
			case "risk":
				got = s.Risk
			case "confirm":
				got = s.Confirm
			case "os_user":
				got = s.OSUser
			}

			if got != val {
				t.Errorf("FromAnnotations(%s=%s): got %q, want %q", tt.key, val, got, val)
			}
		}
	}
}

// TestFromAnnotations_Idempotent tests idempotent field parsing
func TestFromAnnotations_Idempotent(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"true", true},
		{"True", true},
		{"TRUE", true},
		{"1", true},
		{"false", false},
		{"False", false},
		{"FALSE", false},
		{"0", false},
		{"", false},
		{"invalid", false},
	}

	for _, tt := range tests {
		annotations := map[string]string{"idempotent": tt.input}
		s := FromAnnotations(annotations)
		if s.Idempotent != tt.want {
			t.Errorf("idempotent=%q: got %v, want %v", tt.input, s.Idempotent, tt.want)
		}
	}
}

// TestFromAnnotations_Cost tests cost field parsing
func TestFromAnnotations_Cost(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"0", 0},
		{"100", 100},
		{"5000", 5000},
		{"999999", 999999},
		{"-1", 0},   // negative values fallback to 0
		{"-100", 0}, // negative values fallback to 0
		{"", 0},
		{"abc", 0},
		{"12.34", 0},
	}

	for _, tt := range tests {
		annotations := map[string]string{"cost": tt.input}
		s := FromAnnotations(annotations)
		if s.Cost != tt.want {
			t.Errorf("cost=%q: got %d, want %d", tt.input, s.Cost, tt.want)
		}
	}
}

// TestSchema_IsEmpty tests the IsEmpty method
func TestSchema_IsEmpty(t *testing.T) {
	tests := []struct {
		name   string
		schema *Schema
		want   bool
	}{
		{"nil schema", nil, true},
		{"empty schema", &Schema{}, true},
		{"schema with defaults", FromAnnotations(nil), false},
		{"schema with name", &Schema{Name: "test"}, false},
		{"schema with type", &Schema{Type: TYPE_ACTION}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.schema.IsEmpty()
			if got != tt.want {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestSchema_String tests the String method
func TestSchema_String(t *testing.T) {
	tests := []struct {
		name   string
		schema *Schema
		want   string
	}{
		{"nil schema", nil, "Schema{nil}"},
		{"empty schema", &Schema{}, "Schema{name=\"\", type=\"\", volatility=\"\", parallel=\"\", idempotent=false, risk=\"\", confirm=\"\", os_user=\"\", cost=0}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.schema.String()
			if got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestSchema_String_WithValues tests String with actual values
func TestSchema_String_WithValues(t *testing.T) {
	s := &Schema{
		Name:       "pig ext add",
		Type:       TYPE_ACTION,
		Volatility: VOLATILITY_STABLE,
		Parallel:   PARALLEL_SAFE,
		Idempotent: true,
		Risk:       RISK_LOW,
		Confirm:    CONFIRM_NONE,
		OSUser:     OS_USER_ROOT,
		Cost:       5000,
	}
	got := s.String()
	// Just verify it contains key parts
	if got == "" || got == "Schema{nil}" {
		t.Errorf("String() returned empty or nil string for non-nil schema")
	}
	if !strings.Contains(got, "pig ext add") {
		t.Errorf("String() should contain name")
	}
	if !strings.Contains(got, "action") {
		t.Errorf("String() should contain type")
	}
}

// TestSchema_YAML tests YAML serialization
func TestSchema_YAML(t *testing.T) {
	s := &Schema{
		Name:       "pig ext add",
		Type:       TYPE_ACTION,
		Volatility: VOLATILITY_STABLE,
		Parallel:   PARALLEL_SAFE,
		Idempotent: true,
		Risk:       RISK_LOW,
		Confirm:    CONFIRM_NONE,
		OSUser:     OS_USER_ROOT,
		Cost:       5000,
	}

	data, err := s.YAML()
	if err != nil {
		t.Fatalf("YAML() error = %v", err)
	}

	// Verify it's valid YAML by parsing it back
	var parsed map[string]interface{}
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("YAML() output is not valid YAML: %v", err)
	}

	if parsed["name"] != "pig ext add" {
		t.Errorf("YAML name = %v, want %q", parsed["name"], "pig ext add")
	}
	if parsed["os_user"] != "root" {
		t.Errorf("YAML os_user = %v, want %q", parsed["os_user"], "root")
	}
}

// TestSchema_YAML_Nil tests YAML with nil receiver
func TestSchema_YAML_Nil(t *testing.T) {
	var s *Schema
	_, err := s.YAML()
	if err == nil {
		t.Error("YAML() on nil should return error")
	}
}

// TestSchema_JSON tests JSON serialization
func TestSchema_JSON(t *testing.T) {
	s := &Schema{
		Name:       "pig ext add",
		Type:       TYPE_ACTION,
		Volatility: VOLATILITY_STABLE,
		Parallel:   PARALLEL_SAFE,
		Idempotent: true,
		Risk:       RISK_LOW,
		Confirm:    CONFIRM_NONE,
		OSUser:     OS_USER_ROOT,
		Cost:       5000,
	}

	data, err := s.JSON()
	if err != nil {
		t.Fatalf("JSON() error = %v", err)
	}

	// Verify it's valid JSON by parsing it back
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("JSON() output is not valid JSON: %v", err)
	}

	if parsed["name"] != "pig ext add" {
		t.Errorf("JSON name = %v, want %q", parsed["name"], "pig ext add")
	}
	if parsed["os_user"] != "root" {
		t.Errorf("JSON os_user = %v, want %q", parsed["os_user"], "root")
	}
}

// TestSchema_JSON_Nil tests JSON with nil receiver
func TestSchema_JSON_Nil(t *testing.T) {
	var s *Schema
	_, err := s.JSON()
	if err == nil {
		t.Error("JSON() on nil should return error")
	}
}

// TestNilReceiverSafety tests all methods with nil receiver
func TestNilReceiverSafety(t *testing.T) {
	var s *Schema

	// IsEmpty should not panic
	if !s.IsEmpty() {
		t.Error("nil.IsEmpty() should return true")
	}

	// String should not panic
	str := s.String()
	if str != "Schema{nil}" {
		t.Errorf("nil.String() = %q, want %q", str, "Schema{nil}")
	}

	// YAML should return error, not panic
	_, err := s.YAML()
	if err == nil {
		t.Error("nil.YAML() should return error")
	}

	// JSON should return error, not panic
	_, err = s.JSON()
	if err == nil {
		t.Error("nil.JSON() should return error")
	}
}

