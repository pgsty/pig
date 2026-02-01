package output

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestNewResult(t *testing.T) {
	tests := []struct {
		name    string
		success bool
		code    int
		message string
	}{
		{"success result", true, 0, "operation completed"},
		{"failure result", false, 10101, "parameter error"},
		{"with custom code", true, 20000, "repo operation"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewResult(tt.success, tt.code, tt.message)
			if r.Success != tt.success {
				t.Errorf("Success = %v, want %v", r.Success, tt.success)
			}
			if r.Code != tt.code {
				t.Errorf("Code = %v, want %v", r.Code, tt.code)
			}
			if r.Message != tt.message {
				t.Errorf("Message = %v, want %v", r.Message, tt.message)
			}
		})
	}
}

func TestOK(t *testing.T) {
	data := map[string]string{"key": "value"}
	r := OK("success message", data)

	if !r.Success {
		t.Error("OK should set Success to true")
	}
	if r.Code != 0 {
		t.Errorf("OK should set Code to 0, got %d", r.Code)
	}
	if r.Message != "success message" {
		t.Errorf("Message = %v, want 'success message'", r.Message)
	}
	if r.Data == nil {
		t.Error("Data should not be nil")
	}
}

func TestOKWithNilData(t *testing.T) {
	r := OK("success without data", nil)

	if !r.Success {
		t.Error("OK should set Success to true")
	}
	if r.Data != nil {
		t.Error("Data should be nil when passed nil")
	}
}

func TestFail(t *testing.T) {
	r := Fail(10101, "parameter error")

	if r.Success {
		t.Error("Fail should set Success to false")
	}
	if r.Code != 10101 {
		t.Errorf("Code = %v, want 10101", r.Code)
	}
	if r.Message != "parameter error" {
		t.Errorf("Message = %v, want 'parameter error'", r.Message)
	}
}

func TestWithDetail(t *testing.T) {
	r := NewResult(false, 10101, "error").WithDetail("additional details")

	if r.Detail != "additional details" {
		t.Errorf("Detail = %v, want 'additional details'", r.Detail)
	}
}

func TestWithData(t *testing.T) {
	data := []string{"item1", "item2"}
	r := NewResult(true, 0, "success").WithData(data)

	if r.Data == nil {
		t.Error("Data should not be nil")
	}

	dataSlice, ok := r.Data.([]string)
	if !ok {
		t.Error("Data should be []string")
	}
	if len(dataSlice) != 2 {
		t.Errorf("Data length = %d, want 2", len(dataSlice))
	}
}

func TestChaining(t *testing.T) {
	r := NewResult(true, 0, "success").
		WithDetail("some detail").
		WithData(map[string]int{"count": 42})

	if r.Detail != "some detail" {
		t.Errorf("Detail = %v, want 'some detail'", r.Detail)
	}
	if r.Data == nil {
		t.Error("Data should not be nil")
	}
}

func TestResultExitCode(t *testing.T) {
	tests := []struct {
		name     string
		code     int
		expected int
	}{
		{"success", 0, 0},
		{"param error", MODULE_EXT + CAT_PARAM + 1, 2},
		{"permission error", MODULE_REPO + CAT_PERM + 1, 3},
		{"internal error", MODULE_SYSTEM + CAT_INTERNAL + 1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewResult(tt.code == 0, tt.code, "test")
			if got := r.ExitCode(); got != tt.expected {
				t.Errorf("ExitCode() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestResultJSONSerialization(t *testing.T) {
	r := &Result{
		Success: true,
		Code:    0,
		Message: "test message",
		Detail:  "test detail",
		Data:    map[string]string{"key": "value"},
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("Failed to marshal Result to JSON: %v", err)
	}

	var decoded Result
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal Result from JSON: %v", err)
	}

	if decoded.Success != r.Success {
		t.Errorf("Success = %v, want %v", decoded.Success, r.Success)
	}
	if decoded.Code != r.Code {
		t.Errorf("Code = %v, want %v", decoded.Code, r.Code)
	}
	if decoded.Message != r.Message {
		t.Errorf("Message = %v, want %v", decoded.Message, r.Message)
	}
	if decoded.Detail != r.Detail {
		t.Errorf("Detail = %v, want %v", decoded.Detail, r.Detail)
	}
}

func TestResultYAMLSerialization(t *testing.T) {
	r := &Result{
		Success: true,
		Code:    0,
		Message: "test message",
		Detail:  "test detail",
		Data:    map[string]string{"key": "value"},
	}

	data, err := yaml.Marshal(r)
	if err != nil {
		t.Fatalf("Failed to marshal Result to YAML: %v", err)
	}

	var decoded Result
	if err := yaml.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal Result from YAML: %v", err)
	}

	if decoded.Success != r.Success {
		t.Errorf("Success = %v, want %v", decoded.Success, r.Success)
	}
	if decoded.Code != r.Code {
		t.Errorf("Code = %v, want %v", decoded.Code, r.Code)
	}
	if decoded.Message != r.Message {
		t.Errorf("Message = %v, want %v", decoded.Message, r.Message)
	}
	if decoded.Detail != r.Detail {
		t.Errorf("Detail = %v, want %v", decoded.Detail, r.Detail)
	}
}

func TestResultOmitEmpty(t *testing.T) {
	r := &Result{
		Success: true,
		Code:    0,
		Message: "test",
		// Detail and Data are empty
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("Failed to marshal Result to JSON: %v", err)
	}

	jsonStr := string(data)
	if strings.Contains(jsonStr, "detail") {
		t.Error("Empty Detail should be omitted from JSON")
	}
	if strings.Contains(jsonStr, "data") {
		t.Error("Nil Data should be omitted from JSON")
	}
}
