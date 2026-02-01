package output

// Result represents a unified response structure for all CLI commands.
// It provides consistent structured output for both human and machine consumption.
type Result struct {
	Success bool        `json:"success" yaml:"success"`
	Code    int         `json:"code" yaml:"code"`
	Message string      `json:"message" yaml:"message"`
	Detail  string      `json:"detail,omitempty" yaml:"detail,omitempty"`
	Data    interface{} `json:"data,omitempty" yaml:"data,omitempty"`
}

// NewResult creates a new Result with the specified success status, code, and message.
func NewResult(success bool, code int, message string) *Result {
	return &Result{
		Success: success,
		Code:    code,
		Message: message,
	}
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
func (r *Result) WithDetail(detail string) *Result {
	r.Detail = detail
	return r
}

// WithData adds data to the Result and returns it for chaining.
func (r *Result) WithData(data interface{}) *Result {
	r.Data = data
	return r
}

// ExitCode returns the shell exit code based on the Result's status code.
// It extracts the category (CC) from the 222 structure and maps it to exit codes.
func (r *Result) ExitCode() int {
	return ExitCode(r.Code)
}
