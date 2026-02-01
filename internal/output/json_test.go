package output

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestResult_JSON_Success(t *testing.T) {
	r := OK("operation completed", nil)
	data, err := r.JSON()
	if err != nil {
		t.Fatalf("JSON() returned error: %v", err)
	}

	jsonStr := string(data)

	if !strings.Contains(jsonStr, `"success":true`) {
		t.Error("JSON should contain 'success':true")
	}
	if !strings.Contains(jsonStr, `"code":0`) {
		t.Error("JSON should contain 'code':0")
	}
	if !strings.Contains(jsonStr, `"message":"operation completed"`) {
		t.Error("JSON should contain the message")
	}
}

func TestResult_JSON_Failure(t *testing.T) {
	r := Fail(100101, "extension not found").WithDetail("extension 'nonexistent' is not available")
	data, err := r.JSON()
	if err != nil {
		t.Fatalf("JSON() returned error: %v", err)
	}

	jsonStr := string(data)

	if !strings.Contains(jsonStr, `"success":false`) {
		t.Error("JSON should contain 'success':false")
	}
	if !strings.Contains(jsonStr, `"code":100101`) {
		t.Error("JSON should contain the error code")
	}
	if !strings.Contains(jsonStr, `"detail":`) {
		t.Error("JSON should contain detail when present")
	}
}

func TestResult_JSON_WithData(t *testing.T) {
	testData := map[string]interface{}{
		"installed": []string{"postgis", "pg_stat_statements"},
	}
	r := OK("extensions installed", testData)
	jsonData, err := r.JSON()
	if err != nil {
		t.Fatalf("JSON() returned error: %v", err)
	}

	jsonStr := string(jsonData)

	if !strings.Contains(jsonStr, `"data":`) {
		t.Error("JSON should contain data field")
	}
	if !strings.Contains(jsonStr, `"installed":`) {
		t.Error("JSON should contain nested data")
	}
	if !strings.Contains(jsonStr, `"postgis"`) {
		t.Error("JSON should contain data values")
	}
}

func TestResult_JSON_Omitempty_NoDetail(t *testing.T) {
	r := OK("success", nil)
	data, err := r.JSON()
	if err != nil {
		t.Fatalf("JSON() returned error: %v", err)
	}

	jsonStr := string(data)

	// Empty Detail should be omitted
	if strings.Contains(jsonStr, `"detail":`) {
		t.Error("JSON should omit empty detail field")
	}
}

func TestResult_JSON_Omitempty_NoData(t *testing.T) {
	r := OK("success", nil)
	data, err := r.JSON()
	if err != nil {
		t.Fatalf("JSON() returned error: %v", err)
	}

	jsonStr := string(data)

	// Nil Data should be omitted
	if strings.Contains(jsonStr, `"data":`) {
		t.Error("JSON should omit nil data field")
	}
}

func TestResult_JSON_SnakeCase(t *testing.T) {
	r := OK("test", nil).WithDetail("details here")
	data, err := r.JSON()
	if err != nil {
		t.Fatalf("JSON() returned error: %v", err)
	}

	jsonStr := string(data)

	// Verify snake_case field names exist (positive check)
	if !strings.Contains(jsonStr, `"success":`) {
		t.Error("JSON should contain 'success' field")
	}
	if !strings.Contains(jsonStr, `"code":`) {
		t.Error("JSON should contain 'code' field")
	}
	if !strings.Contains(jsonStr, `"message":`) {
		t.Error("JSON should contain 'message' field")
	}
	if !strings.Contains(jsonStr, `"detail":`) {
		t.Error("JSON should contain 'detail' field")
	}

	// Verify no PascalCase (negative check)
	if strings.Contains(jsonStr, `"Success":`) || strings.Contains(jsonStr, `"Code":`) || strings.Contains(jsonStr, `"Message":`) {
		t.Error("JSON should use snake_case field names, not PascalCase")
	}
}

func TestResult_JSON_NilReceiver(t *testing.T) {
	var r *Result = nil
	_, err := r.JSON()
	if err == nil {
		t.Error("JSON() on nil receiver should return error")
	}
	if !strings.Contains(err.Error(), "cannot render nil Result") {
		t.Errorf("Error message should be 'cannot render nil Result', got: %v", err)
	}
}

func TestResult_JSONPretty_NilReceiver(t *testing.T) {
	var r *Result = nil
	_, err := r.JSONPretty()
	if err == nil {
		t.Error("JSONPretty() on nil receiver should return error")
	}
	if !strings.Contains(err.Error(), "cannot render nil Result") {
		t.Errorf("Error message should be 'cannot render nil Result', got: %v", err)
	}
}

func TestResult_JSON_ValidJSON(t *testing.T) {
	r := OK("test", map[string]interface{}{"key": "value"}).WithDetail("details")
	data, err := r.JSON()
	if err != nil {
		t.Fatalf("JSON() returned error: %v", err)
	}

	// Verify it's valid JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Errorf("JSON() output is not valid JSON: %v", err)
	}
}

func TestResult_JSONPretty_Indented(t *testing.T) {
	r := OK("test", nil)
	data, err := r.JSONPretty()
	if err != nil {
		t.Fatalf("JSONPretty() returned error: %v", err)
	}

	jsonStr := string(data)

	// Pretty JSON should contain newlines and indentation
	if !strings.Contains(jsonStr, "\n") {
		t.Error("JSONPretty should contain newlines")
	}
	if !strings.Contains(jsonStr, "  ") {
		t.Error("JSONPretty should contain indentation")
	}
}

func TestResult_JSONPretty_ValidJSON(t *testing.T) {
	r := OK("test", map[string]interface{}{"key": "value"})
	data, err := r.JSONPretty()
	if err != nil {
		t.Fatalf("JSONPretty() returned error: %v", err)
	}

	// Verify it's valid JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Errorf("JSONPretty() output is not valid JSON: %v", err)
	}
}
