/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

Tests for pb stanza structured output result and DTO.
*/
package pgbackrest

import (
	"encoding/json"
	"testing"

	"pig/internal/output"

	"gopkg.in/yaml.v3"
)

func TestPbStanzaResultDataJSONSerialization(t *testing.T) {
	data := &PbStanzaResultData{
		Stanza:    "pg-meta",
		Operation: "create",
		NoOnline:  true,
		Force:     false,
		Deleted:   false,
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	var decoded PbStanzaResultData
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if decoded.Stanza != data.Stanza {
		t.Errorf("Stanza mismatch: got %q, want %q", decoded.Stanza, data.Stanza)
	}
	if decoded.Operation != data.Operation {
		t.Errorf("Operation mismatch: got %q, want %q", decoded.Operation, data.Operation)
	}
	if decoded.NoOnline != data.NoOnline {
		t.Errorf("NoOnline mismatch: got %v, want %v", decoded.NoOnline, data.NoOnline)
	}
}

func TestPbStanzaResultDataYAMLSerialization(t *testing.T) {
	data := &PbStanzaResultData{
		Stanza:    "pg-meta",
		Operation: "delete",
		NoOnline:  false,
		Force:     true,
		Deleted:   true,
	}

	yamlBytes, err := yaml.Marshal(data)
	if err != nil {
		t.Fatalf("YAML marshal failed: %v", err)
	}

	var decoded PbStanzaResultData
	if err := yaml.Unmarshal(yamlBytes, &decoded); err != nil {
		t.Fatalf("YAML unmarshal failed: %v", err)
	}

	if decoded.Deleted != data.Deleted {
		t.Errorf("Deleted mismatch: got %v, want %v", decoded.Deleted, data.Deleted)
	}
}

func TestDeleteResultRequiresForce(t *testing.T) {
	result := DeleteResult(&Config{}, &DeleteOptions{Force: false})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Fatal("expected failure result")
	}
	if result.Code != output.CodePbStanzaDeleteRequiresForce {
		t.Fatalf("expected CodePbStanzaDeleteRequiresForce, got %d", result.Code)
	}
}

func TestDeleteResultNilOptions(t *testing.T) {
	result := DeleteResult(&Config{}, nil)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Fatal("expected failure result for nil options (no --force)")
	}
	if result.Code != output.CodePbStanzaDeleteRequiresForce {
		t.Fatalf("expected CodePbStanzaDeleteRequiresForce, got %d", result.Code)
	}
}

func TestPbStanzaResultDataOmitempty(t *testing.T) {
	tests := []struct {
		name     string
		data     *PbStanzaResultData
		wantKeys []string // keys that should be present
		noKeys   []string // keys that should be omitted
	}{
		{
			name: "create with no flags",
			data: &PbStanzaResultData{
				Stanza:    "test-stanza",
				Operation: "create",
			},
			wantKeys: []string{"stanza", "operation"},
			noKeys:   []string{"no_online", "force", "deleted"},
		},
		{
			name: "create with --no-online",
			data: &PbStanzaResultData{
				Stanza:    "test-stanza",
				Operation: "create",
				NoOnline:  true,
			},
			wantKeys: []string{"stanza", "operation", "no_online"},
			noKeys:   []string{"force", "deleted"},
		},
		{
			name: "delete with force",
			data: &PbStanzaResultData{
				Stanza:    "test-stanza",
				Operation: "delete",
				Force:     true,
				Deleted:   true,
			},
			wantKeys: []string{"stanza", "operation", "force", "deleted"},
			noKeys:   []string{"no_online"},
		},
		{
			name: "upgrade with no flags",
			data: &PbStanzaResultData{
				Stanza:    "test-stanza",
				Operation: "upgrade",
			},
			wantKeys: []string{"stanza", "operation"},
			noKeys:   []string{"no_online", "force", "deleted"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonBytes, err := json.Marshal(tt.data)
			if err != nil {
				t.Fatalf("JSON marshal failed: %v", err)
			}

			var jsonMap map[string]interface{}
			if err := json.Unmarshal(jsonBytes, &jsonMap); err != nil {
				t.Fatalf("JSON unmarshal to map failed: %v", err)
			}

			for _, key := range tt.wantKeys {
				if _, ok := jsonMap[key]; !ok {
					t.Errorf("expected key %q to be present in JSON, got: %s", key, string(jsonBytes))
				}
			}

			for _, key := range tt.noKeys {
				if _, ok := jsonMap[key]; ok {
					t.Errorf("expected key %q to be omitted in JSON, got: %s", key, string(jsonBytes))
				}
			}
		})
	}
}

func TestIsStanzaExistsMessage(t *testing.T) {
	tests := []struct {
		message string
		want    bool
	}{
		{"stanza 'pg-meta' already exists", true},
		{"stanza data already exists", true},
		{"Stanza Exists", true},
		{"stanza created successfully", false},
		{"something else entirely", false},
		{"", false},
	}

	for _, tt := range tests {
		got := isStanzaExistsMessage(tt.message)
		if got != tt.want {
			t.Errorf("isStanzaExistsMessage(%q) = %v, want %v", tt.message, got, tt.want)
		}
	}
}

func TestPbConfigErrorResult(t *testing.T) {
	tests := []struct {
		name         string
		errMsg       string
		fallbackCode int
		wantCode     int
	}{
		{
			name:         "config file not found",
			errMsg:       "config file not found: /etc/pgbackrest/pgbackrest.conf",
			fallbackCode: output.CodePbStanzaCreateFailed,
			wantCode:     output.CodePbConfigNotFound,
		},
		{
			name:         "config file not accessible",
			errMsg:       "config file not accessible: /etc/pgbackrest/pgbackrest.conf",
			fallbackCode: output.CodePbStanzaCreateFailed,
			wantCode:     output.CodePbConfigNotFound,
		},
		{
			name:         "no stanza found",
			errMsg:       "no stanza found in config file",
			fallbackCode: output.CodePbStanzaCreateFailed,
			wantCode:     output.CodePbStanzaNotFound,
		},
		{
			name:         "cannot detect stanza",
			errMsg:       "cannot detect stanza: use --stanza to specify",
			fallbackCode: output.CodePbStanzaCreateFailed,
			wantCode:     output.CodePbStanzaNotFound,
		},
		{
			name:         "fallback for unknown error",
			errMsg:       "some unknown error occurred",
			fallbackCode: output.CodePbStanzaCreateFailed,
			wantCode:     output.CodePbStanzaCreateFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &testError{msg: tt.errMsg}
			result := pbConfigErrorResult(err, tt.fallbackCode, "test message")
			if result == nil {
				t.Fatal("expected non-nil result")
			}
			if result.Code != tt.wantCode {
				t.Errorf("pbConfigErrorResult() code = %d, want %d", result.Code, tt.wantCode)
			}
		})
	}
}

// testError implements error interface for testing
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
