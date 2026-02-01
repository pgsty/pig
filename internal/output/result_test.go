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

func TestResult_Render_YAML(t *testing.T) {
	r := OK("test", nil)
	data, err := r.Render("yaml")
	if err != nil {
		t.Fatalf("Render(yaml) returned error: %v", err)
	}

	if !strings.Contains(string(data), "success: true") {
		t.Error("Render(yaml) should return YAML format")
	}
}

func TestResult_Render_JSON(t *testing.T) {
	r := OK("test", nil)
	data, err := r.Render("json")
	if err != nil {
		t.Fatalf("Render(json) returned error: %v", err)
	}

	if !strings.Contains(string(data), `"success":true`) {
		t.Error("Render(json) should return JSON format")
	}
}

func TestResult_Render_Text(t *testing.T) {
	r := OK("test message", nil)
	data, err := r.Render("text")
	if err != nil {
		t.Fatalf("Render(text) returned error: %v", err)
	}

	if string(data) != "test message" {
		t.Errorf("Render(text) = %v, want 'test message'", string(data))
	}
}

func TestResult_Render_UnknownFormat(t *testing.T) {
	r := OK("test", nil)
	_, err := r.Render("unknown")
	if err == nil {
		t.Error("Render(unknown) should return error")
	}

	if !strings.Contains(err.Error(), "unknown output format") {
		t.Errorf("Error message = %v, should contain 'unknown output format'", err.Error())
	}
}

func TestResult_Render_JSONPretty(t *testing.T) {
	r := OK("test", nil)
	data, err := r.Render("json-pretty")
	if err != nil {
		t.Fatalf("Render(json-pretty) returned error: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, "\n") {
		t.Error("Render(json-pretty) should return indented JSON with newlines")
	}
	if !strings.Contains(jsonStr, `"success": true`) {
		t.Error("Render(json-pretty) should return valid JSON")
	}
}

func TestResult_Render_TextWithDetail(t *testing.T) {
	r := Fail(10101, "operation failed").WithDetail("additional info")
	data, err := r.Render("text")
	if err != nil {
		t.Fatalf("Render(text) returned error: %v", err)
	}

	text := string(data)
	if !strings.Contains(text, "operation failed") {
		t.Error("Render(text) should contain message")
	}
	if !strings.Contains(text, "additional info") {
		t.Error("Render(text) should contain detail when present")
	}
}

func TestResult_Render_NilReceiver(t *testing.T) {
	var r *Result = nil
	_, err := r.Render("json")
	if err == nil {
		t.Error("Render() on nil receiver should return error")
	}
	if !strings.Contains(err.Error(), "nil") {
		t.Errorf("Error message should mention nil, got: %v", err)
	}
}

func TestWithDetail_NilReceiver(t *testing.T) {
	var r *Result = nil
	result := r.WithDetail("test")
	if result != nil {
		t.Error("WithDetail() on nil receiver should return nil")
	}
}

func TestWithData_NilReceiver(t *testing.T) {
	var r *Result = nil
	result := r.WithData("test")
	if result != nil {
		t.Error("WithData() on nil receiver should return nil")
	}
}

func TestExitCode_NilReceiver(t *testing.T) {
	var r *Result = nil
	exitCode := r.ExitCode()
	if exitCode != 1 {
		t.Errorf("ExitCode() on nil receiver should return 1, got %d", exitCode)
	}
}

func TestResult_String(t *testing.T) {
	tests := []struct {
		name     string
		result   *Result
		contains []string
	}{
		{
			name:     "nil result",
			result:   nil,
			contains: []string{"nil"},
		},
		{
			name:     "success result",
			result:   OK("test message", nil),
			contains: []string{"success=true", "code=0", `message="test message"`},
		},
		{
			name:     "failure with detail",
			result:   Fail(10101, "error").WithDetail("details here"),
			contains: []string{"success=false", "code=10101", `detail="details here"`},
		},
		{
			name:     "with data",
			result:   OK("test", map[string]int{"count": 42}),
			contains: []string{"data="},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			str := tt.result.String()
			for _, substr := range tt.contains {
				if !strings.Contains(str, substr) {
					t.Errorf("String() = %q, should contain %q", str, substr)
				}
			}
		})
	}
}
