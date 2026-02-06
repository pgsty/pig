package output

import (
	"bytes"
	"os"
	"pig/internal/config"
	"strings"
	"testing"
)

func TestPrint(t *testing.T) {
	// Save original values
	origFormat := config.OutputFormat
	origStdout := os.Stdout
	defer func() {
		config.OutputFormat = origFormat
		os.Stdout = origStdout
	}()

	tests := []struct {
		name     string
		format   string
		result   *Result
		contains []string
	}{
		{
			name:     "text format",
			format:   "text",
			result:   OK("test message", nil),
			contains: []string{"✓", "test message"},
		},
		{
			name:     "yaml format",
			format:   "yaml",
			result:   OK("test message", nil),
			contains: []string{"success: true", "message: test message"},
		},
		{
			name:     "json format",
			format:   "json",
			result:   OK("test message", nil),
			contains: []string{`"success":true`, `"message":"test message"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.OutputFormat = tt.format

			// Capture stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			err := Print(tt.result)
			w.Close()

			var buf bytes.Buffer
			buf.ReadFrom(r)
			output := buf.String()

			os.Stdout = origStdout

			if err != nil {
				t.Errorf("Print() error = %v", err)
			}

			for _, want := range tt.contains {
				if !strings.Contains(output, want) {
					t.Errorf("Print() output = %q, want to contain %q", output, want)
				}
			}
		})
	}
}

func TestPrintNil(t *testing.T) {
	err := Print(nil)
	if err == nil {
		t.Error("Print(nil) should return error")
	}
}

func TestPrintWithData(t *testing.T) {
	// Save original values
	origFormat := config.OutputFormat
	origStdout := os.Stdout
	defer func() {
		config.OutputFormat = origFormat
		os.Stdout = origStdout
	}()

	tests := []struct {
		name     string
		format   string
		data     interface{}
		message  string
		contains []string
	}{
		{
			name:     "yaml with data",
			format:   "yaml",
			data:     map[string]string{"key": "value"},
			message:  "data message",
			contains: []string{"success: true", "message: data message", "key: value"},
		},
		{
			name:     "json with data",
			format:   "json",
			data:     map[string]string{"key": "value"},
			message:  "data message",
			contains: []string{`"success":true`, `"key":"value"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.OutputFormat = tt.format

			// Capture stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			err := Print(OK(tt.message, tt.data))
			w.Close()

			var buf bytes.Buffer
			buf.ReadFrom(r)
			output := buf.String()

			os.Stdout = origStdout

			if err != nil {
				t.Errorf("Print(OK(...)) error = %v", err)
			}

			for _, want := range tt.contains {
				if !strings.Contains(output, want) {
					t.Errorf("Print(OK(...)) output = %q, want to contain %q", output, want)
				}
			}
		})
	}
}

// TestPrintFormatsCorrectly tests that Print() function outputs correctly based on format (AC: 5.6)
func TestPrintFormatsCorrectly(t *testing.T) {
	// Save original values
	origFormat := config.OutputFormat
	origStdout := os.Stdout
	defer func() {
		config.OutputFormat = origFormat
		os.Stdout = origStdout
	}()

	result := OK("format test", map[string]int{"count": 42})

	tests := []struct {
		format      string
		mustContain []string
		mustNotHave []string
	}{
		{
			format:      "text",
			mustContain: []string{"✓", "format test"},
			mustNotHave: []string{`"success"`, "success:"},
		},
		{
			format:      "yaml",
			mustContain: []string{"success: true", "message: format test", "count: 42"},
			mustNotHave: []string{`"success"`},
		},
		{
			format:      "json",
			mustContain: []string{`"success":true`, `"message":"format test"`, `"count":42`},
			mustNotHave: []string{"success: true"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			config.OutputFormat = tt.format

			// Capture stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			err := Print(result)
			w.Close()

			var buf bytes.Buffer
			buf.ReadFrom(r)
			output := buf.String()

			os.Stdout = origStdout

			if err != nil {
				t.Errorf("Print() error = %v", err)
			}

			for _, want := range tt.mustContain {
				if !strings.Contains(output, want) {
					t.Errorf("Print(%s) output missing %q in %q", tt.format, want, output)
				}
			}

			for _, notWant := range tt.mustNotHave {
				if strings.Contains(output, notWant) {
					t.Errorf("Print(%s) output should not contain %q in %q", tt.format, notWant, output)
				}
			}
		})
	}
}

func TestPrintFailResult(t *testing.T) {
	// Save original values
	origFormat := config.OutputFormat
	origStdout := os.Stdout
	defer func() {
		config.OutputFormat = origFormat
		os.Stdout = origStdout
	}()

	config.OutputFormat = "json"

	// Capture stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := Print(Fail(100, "test error"))
	w.Close()

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	os.Stdout = origStdout

	if err != nil {
		t.Errorf("Print(Fail(...)) error = %v", err)
	}

	if !strings.Contains(output, `"success":false`) {
		t.Errorf("Print(Fail(...)) output should contain success:false, got %q", output)
	}
	if !strings.Contains(output, `"code":100`) {
		t.Errorf("Print(Fail(...)) output should contain code:100, got %q", output)
	}
	if !strings.Contains(output, `"message":"test error"`) {
		t.Errorf("Print(Fail(...)) output should contain message, got %q", output)
	}
}

func TestPrintTo(t *testing.T) {
	// Save original format
	origFormat := config.OutputFormat
	defer func() {
		config.OutputFormat = origFormat
	}()

	tests := []struct {
		name     string
		format   string
		result   *Result
		contains []string
	}{
		{
			name:     "yaml to buffer",
			format:   "yaml",
			result:   OK("test", nil),
			contains: []string{"success: true", "message: test"},
		},
		{
			name:     "json to buffer",
			format:   "json",
			result:   Fail(100, "error"),
			contains: []string{`"success":false`, `"code":100`},
		},
		{
			name:     "json-pretty to buffer",
			format:   "json-pretty",
			result:   OK("pretty", nil),
			contains: []string{`"success": true`, "\n"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.OutputFormat = tt.format

			var buf bytes.Buffer
			err := PrintTo(&buf, tt.result)
			if err != nil {
				t.Errorf("PrintTo() error = %v", err)
			}

			output := buf.String()
			for _, want := range tt.contains {
				if !strings.Contains(output, want) {
					t.Errorf("PrintTo() output = %q, want to contain %q", output, want)
				}
			}
		})
	}
}

func TestPrintToNil(t *testing.T) {
	var buf bytes.Buffer
	err := PrintTo(&buf, nil)
	if err == nil {
		t.Error("PrintTo(nil) should return error")
	}
}
