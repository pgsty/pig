package output

import (
	"fmt"
	"strings"
)

// Result represents a unified response structure for all CLI commands.
// It provides consistent structured output for both human and machine consumption.
type Result struct {
	Success bool        `json:"success" yaml:"success"`
	Code    int         `json:"code" yaml:"code"`
	Message string      `json:"message" yaml:"message"`
	Detail  string      `json:"detail,omitempty" yaml:"detail,omitempty"`
	Data    interface{} `json:"data,omitempty" yaml:"data,omitempty"`
}

// OK creates a successful Result with the given message and optional data.
func OK(message string, data interface{}) *Result {
	return &Result{
		Success: true,
		Code:    0,
		Message: message,
		Data:    data,
	}
}

// Fail creates a failed Result with the given code and message.
func Fail(code int, message string) *Result {
	return &Result{
		Success: false,
		Code:    code,
		Message: message,
	}
}

// WithDetail adds detail information to the Result and returns it for chaining.
// Returns nil if the receiver is nil.
func (r *Result) WithDetail(detail string) *Result {
	if r == nil {
		return nil
	}
	r.Detail = detail
	return r
}

// WithData adds data to the Result and returns it for chaining.
// Returns nil if the receiver is nil.
func (r *Result) WithData(data interface{}) *Result {
	if r == nil {
		return nil
	}
	r.Data = data
	return r
}

// ExitCode returns the shell exit code based on the Result's status code.
// It extracts the category (CC) from the 222 structure and maps it to exit codes.
// Returns 1 if the receiver is nil.
func (r *Result) ExitCode() int {
	if r == nil {
		return 1
	}
	exit := ExitCode(r.Code)
	if !r.Success && exit == 0 {
		return 1
	}
	return exit
}

// String returns a human-readable representation of the Result for debugging.
func (r *Result) String() string {
	if r == nil {
		return "Result{nil}"
	}
	var parts []string
	parts = append(parts, fmt.Sprintf("success=%v", r.Success))
	parts = append(parts, fmt.Sprintf("code=%d", r.Code))
	parts = append(parts, fmt.Sprintf("message=%q", r.Message))
	if r.Detail != "" {
		parts = append(parts, fmt.Sprintf("detail=%q", r.Detail))
	}
	if r.Data != nil {
		parts = append(parts, fmt.Sprintf("data=%v", r.Data))
	}
	return "Result{" + strings.Join(parts, ", ") + "}"
}

// Render serializes the Result to the specified format.
// Supported formats: "yaml", "json", "json-pretty", "text", "text-color"
// For "text" format, returns human-readable output with ✓/✗ indicators.
// For "text-color" format, adds ANSI color codes (respects NO_COLOR and TTY detection).
// Returns an error for unknown formats or nil receiver.
func (r *Result) Render(format string) ([]byte, error) {
	if r == nil {
		return nil, fmt.Errorf("cannot render nil Result")
	}
	switch format {
	case "yaml":
		return r.YAML()
	case "json":
		return r.JSON()
	case "json-pretty":
		return r.JSONPretty()
	case "text":
		return []byte(r.Text()), nil
	case "text-color":
		return []byte(r.ColorText()), nil
	default:
		return nil, fmt.Errorf("unknown output format: %s", format)
	}
}
