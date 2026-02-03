// Package ancs provides Agent Native Command Schema (ANCS) metadata support.
// It defines structured metadata for CLI commands that AI agents can interpret
// to understand command capabilities, side effects, and execution requirements.
package ancs

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// CommandType constants define whether a command reads or modifies state
const (
	TYPE_QUERY  = "query"  // Read-only operation
	TYPE_ACTION = "action" // Modifies system state
)

// Volatility constants define output stability characteristics
const (
	VOLATILITY_IMMUTABLE = "immutable" // Same input always produces same output
	VOLATILITY_STABLE    = "stable"    // Output stable within same transaction/session
	VOLATILITY_VOLATILE  = "volatile"  // Output may change between calls
)

// ParallelSafety constants define safe parallel execution modes
const (
	PARALLEL_SAFE       = "safe"       // Can run in parallel with any command
	PARALLEL_RESTRICTED = "restricted" // Limited parallel execution allowed
	PARALLEL_UNSAFE     = "unsafe"     // Must run exclusively
)

// RiskLevel constants define potential impact severity
const (
	RISK_SAFE     = "safe"     // No risk, read-only or reversible
	RISK_LOW      = "low"      // Minor changes, easily reversible
	RISK_MEDIUM   = "medium"   // Moderate impact, requires attention
	RISK_HIGH     = "high"     // Significant impact, hard to reverse
	RISK_CRITICAL = "critical" // Critical impact, may cause data loss
)

// ConfirmLevel constants define confirmation requirements
const (
	CONFIRM_NONE        = "none"        // No confirmation needed
	CONFIRM_RECOMMENDED = "recommended" // Confirmation suggested but optional
	CONFIRM_REQUIRED    = "required"    // Must confirm before execution
)

// OSUser constants define required operating system user
const (
	OS_USER_CURRENT = "current" // Run as current user
	OS_USER_ROOT    = "root"    // Requires root/sudo
	OS_USER_DBSU    = "dbsu"    // Requires database superuser (e.g., postgres)
)

// Schema represents Agent Native Command Schema metadata.
// It describes command characteristics for AI agents to understand
// command capabilities, side effects, and execution requirements.
type Schema struct {
	Name       string `json:"name" yaml:"name"`             // Command full name (e.g., "pig ext add")
	Type       string `json:"type" yaml:"type"`             // query or action
	Volatility string `json:"volatility" yaml:"volatility"` // immutable, stable, volatile
	Parallel   string `json:"parallel" yaml:"parallel"`     // safe, restricted, unsafe
	Idempotent bool   `json:"idempotent" yaml:"idempotent"` // true if repeatable safely
	Risk       string `json:"risk" yaml:"risk"`             // safe, low, medium, high, critical
	Confirm    string `json:"confirm" yaml:"confirm"`       // none, recommended, required
	OSUser     string `json:"os_user" yaml:"os_user"`       // current, root, dbsu
	Cost       int    `json:"cost" yaml:"cost"`             // Expected duration in milliseconds
}

// FromAnnotations creates a Schema from Cobra command Annotations.
// It parses the annotation map and applies default values for missing fields.
// Invalid enum values fallback to their respective defaults.
func FromAnnotations(annotations map[string]string) *Schema {
	s := &Schema{
		Type:       TYPE_QUERY,
		Volatility: VOLATILITY_STABLE,
		Parallel:   PARALLEL_SAFE,
		Idempotent: false,
		Risk:       RISK_SAFE,
		Confirm:    CONFIRM_NONE,
		OSUser:     OS_USER_CURRENT,
		Cost:       0,
	}

	if annotations == nil {
		return s
	}

	// Name - string, no validation needed
	if v, ok := annotations["name"]; ok {
		s.Name = strings.TrimSpace(v)
	}

	// Type - enum with fallback
	if v, ok := annotations["type"]; ok {
		normalized := normalizeEnum(v)
		if isValidType(normalized) {
			s.Type = normalized
		}
	}

	// Volatility - enum with fallback
	if v, ok := annotations["volatility"]; ok {
		normalized := normalizeEnum(v)
		if isValidVolatility(normalized) {
			s.Volatility = normalized
		}
	}

	// Parallel - enum with fallback
	if v, ok := annotations["parallel"]; ok {
		normalized := normalizeEnum(v)
		if isValidParallel(normalized) {
			s.Parallel = normalized
		}
	}

	// Idempotent - bool parsing
	if v, ok := annotations["idempotent"]; ok {
		s.Idempotent = parseBool(v)
	}

	// Risk - enum with fallback
	if v, ok := annotations["risk"]; ok {
		normalized := normalizeEnum(v)
		if isValidRisk(normalized) {
			s.Risk = normalized
		}
	}

	// Confirm - enum with fallback
	if v, ok := annotations["confirm"]; ok {
		normalized := normalizeEnum(v)
		if isValidConfirm(normalized) {
			s.Confirm = normalized
		}
	}

	// OSUser - enum with fallback
	if v, ok := annotations["os_user"]; ok {
		normalized := normalizeEnum(v)
		if isValidOSUser(normalized) {
			s.OSUser = normalized
		}
	}

	// Cost - int parsing with fallback to 0, negative values ignored
	if v, ok := annotations["cost"]; ok {
		if parsed, err := strconv.Atoi(v); err == nil && parsed >= 0 {
			s.Cost = parsed
		}
	}

	return s
}

// IsEmpty returns true if the Schema is nil or has no meaningful data.
// A Schema with default values (from FromAnnotations with empty input)
// is NOT considered empty.
func (s *Schema) IsEmpty() bool {
	if s == nil {
		return true
	}
	// Empty schema has all zero values
	return s.Name == "" &&
		s.Type == "" &&
		s.Volatility == "" &&
		s.Parallel == "" &&
		s.Idempotent == false &&
		s.Risk == "" &&
		s.Confirm == "" &&
		s.OSUser == "" &&
		s.Cost == 0
}

// String returns a human-readable representation of the Schema for debugging.
func (s *Schema) String() string {
	if s == nil {
		return "Schema{nil}"
	}
	return fmt.Sprintf("Schema{name=%q, type=%q, volatility=%q, parallel=%q, idempotent=%v, risk=%q, confirm=%q, os_user=%q, cost=%d}",
		s.Name, s.Type, s.Volatility, s.Parallel, s.Idempotent, s.Risk, s.Confirm, s.OSUser, s.Cost)
}

// YAML serializes the Schema to YAML format.
// Returns an error if the receiver is nil.
func (s *Schema) YAML() ([]byte, error) {
	if s == nil {
		return nil, fmt.Errorf("cannot serialize nil Schema to YAML")
	}
	return yaml.Marshal(s)
}

// JSON serializes the Schema to JSON format.
// Returns an error if the receiver is nil.
func (s *Schema) JSON() ([]byte, error) {
	if s == nil {
		return nil, fmt.Errorf("cannot serialize nil Schema to JSON")
	}
	return json.Marshal(s)
}

// Validation helpers

func isValidType(v string) bool {
	return v == TYPE_QUERY || v == TYPE_ACTION
}

func isValidVolatility(v string) bool {
	return v == VOLATILITY_IMMUTABLE || v == VOLATILITY_STABLE || v == VOLATILITY_VOLATILE
}

func isValidParallel(v string) bool {
	return v == PARALLEL_SAFE || v == PARALLEL_RESTRICTED || v == PARALLEL_UNSAFE
}

func isValidRisk(v string) bool {
	return v == RISK_SAFE || v == RISK_LOW || v == RISK_MEDIUM || v == RISK_HIGH || v == RISK_CRITICAL
}

func isValidConfirm(v string) bool {
	return v == CONFIRM_NONE || v == CONFIRM_RECOMMENDED || v == CONFIRM_REQUIRED
}

func isValidOSUser(v string) bool {
	return v == OS_USER_CURRENT || v == OS_USER_ROOT || v == OS_USER_DBSU
}

func parseBool(v string) bool {
	lower := strings.ToLower(strings.TrimSpace(v))
	return lower == "true" || lower == "1"
}

func normalizeEnum(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}
