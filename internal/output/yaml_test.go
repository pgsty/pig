package output

import (
	"strings"
	"testing"
)

func TestResult_YAML_Success(t *testing.T) {
	r := OK("operation completed", nil)
	data, err := r.YAML()
	if err != nil {
		t.Fatalf("YAML() returned error: %v", err)
	}

	yaml := string(data)

	// Check required fields
	if !strings.Contains(yaml, "success: true") {
		t.Error("YAML should contain 'success: true'")
	}
	if !strings.Contains(yaml, "code: 0") {
		t.Error("YAML should contain 'code: 0'")
	}
	if !strings.Contains(yaml, "message: operation completed") {
		t.Error("YAML should contain the message")
	}
}

func TestResult_YAML_Failure(t *testing.T) {
	r := Fail(100101, "extension not found").WithDetail("extension 'nonexistent' is not available")
	data, err := r.YAML()
	if err != nil {
		t.Fatalf("YAML() returned error: %v", err)
	}

	yaml := string(data)

	if !strings.Contains(yaml, "success: false") {
		t.Error("YAML should contain 'success: false'")
	}
	if !strings.Contains(yaml, "code: 100101") {
		t.Error("YAML should contain the error code")
	}
	if !strings.Contains(yaml, "detail:") {
		t.Error("YAML should contain detail when present")
	}
}

func TestResult_YAML_WithData(t *testing.T) {
	data := map[string]interface{}{
		"installed": []string{"postgis", "pg_stat_statements"},
	}
	r := OK("extensions installed", data)
	yamlData, err := r.YAML()
	if err != nil {
		t.Fatalf("YAML() returned error: %v", err)
	}

	yaml := string(yamlData)

	if !strings.Contains(yaml, "data:") {
		t.Error("YAML should contain data field")
	}
	if !strings.Contains(yaml, "installed:") {
		t.Error("YAML should contain nested data")
	}
	if !strings.Contains(yaml, "postgis") {
		t.Error("YAML should contain data values")
	}
}

func TestResult_YAML_Omitempty_NoDetail(t *testing.T) {
	r := OK("success", nil)
	data, err := r.YAML()
	if err != nil {
		t.Fatalf("YAML() returned error: %v", err)
	}

	yaml := string(data)

	// Empty Detail should be omitted
	if strings.Contains(yaml, "detail:") {
		t.Error("YAML should omit empty detail field")
	}
}

func TestResult_YAML_Omitempty_NoData(t *testing.T) {
	r := OK("success", nil)
	data, err := r.YAML()
	if err != nil {
		t.Fatalf("YAML() returned error: %v", err)
	}

	yaml := string(data)

	// Nil Data should be omitted
	if strings.Contains(yaml, "data:") {
		t.Error("YAML should omit nil data field")
	}
}

func TestResult_YAML_SnakeCase(t *testing.T) {
	r := OK("test", nil).WithDetail("details here")
	data, err := r.YAML()
	if err != nil {
		t.Fatalf("YAML() returned error: %v", err)
	}

	yaml := string(data)

	// Verify snake_case field names exist (positive check)
	if !strings.Contains(yaml, "success:") {
		t.Error("YAML should contain 'success:' field")
	}
	if !strings.Contains(yaml, "code:") {
		t.Error("YAML should contain 'code:' field")
	}
	if !strings.Contains(yaml, "message:") {
		t.Error("YAML should contain 'message:' field")
	}
	if !strings.Contains(yaml, "detail:") {
		t.Error("YAML should contain 'detail:' field")
	}

	// Verify no PascalCase (negative check)
	if strings.Contains(yaml, "Success:") || strings.Contains(yaml, "Code:") || strings.Contains(yaml, "Message:") {
		t.Error("YAML should use snake_case field names, not PascalCase")
	}
}

func TestResult_YAML_NilReceiver(t *testing.T) {
	var r *Result = nil
	_, err := r.YAML()
	if err == nil {
		t.Error("YAML() on nil receiver should return error")
	}
	if !strings.Contains(err.Error(), "cannot render nil Result") {
		t.Errorf("Error message should be 'cannot render nil Result', got: %v", err)
	}
}
