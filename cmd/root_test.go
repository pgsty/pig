package cmd

import (
	"pig/internal/config"
	"testing"
)

func TestOutputFlagExists(t *testing.T) {
	// Test that -o/--output flag exists on root command
	flag := rootCmd.PersistentFlags().Lookup("output")
	if flag == nil {
		t.Fatal("--output flag not found on root command")
	}

	// Test short flag
	shortFlag := rootCmd.PersistentFlags().ShorthandLookup("o")
	if shortFlag == nil {
		t.Fatal("-o short flag not found on root command")
	}

	// Test default value
	if flag.DefValue != "text" {
		t.Errorf("--output default value = %q, want %q", flag.DefValue, "text")
	}
}

func TestValidateOutputFormat(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		// Valid formats
		{"text", "text"},
		{"yaml", "yaml"},
		{"json", "json"},
		{"json-pretty", "json-pretty"},
		// Case insensitive
		{"TEXT", "text"},
		{"YAML", "yaml"},
		{"JSON", "json"},
		{"JSON-PRETTY", "json-pretty"},
		{"Text", "text"},
		{"Yaml", "yaml"},
		{"Json", "json"},
		{"Json-Pretty", "json-pretty"},
		// Invalid formats fallback to text
		{"xml", "text"},
		{"csv", "text"},
		{"", "text"},
		{"invalid", "text"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := validateOutputFormat(tt.input)
			if result != tt.expected {
				t.Errorf("validateOutputFormat(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestInitOutputFormat(t *testing.T) {
	// Save original values
	origFormat := outputFormat
	origConfigFormat := config.OutputFormat
	defer func() {
		outputFormat = origFormat
		config.OutputFormat = origConfigFormat
	}()

	tests := []struct {
		input    string
		expected string
	}{
		{"yaml", "yaml"},
		{"json", "json"},
		{"text", "text"},
		{"YAML", "yaml"},
		{"invalid", "text"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			outputFormat = tt.input
			initOutputFormat()
			if config.OutputFormat != tt.expected {
				t.Errorf("initOutputFormat() with %q: config.OutputFormat = %q, want %q",
					tt.input, config.OutputFormat, tt.expected)
			}
		})
	}
}

// TestOutputFlagYaml tests that -o yaml sets config.OutputFormat = "yaml" (AC: 5.1)
func TestOutputFlagYaml(t *testing.T) {
	origFormat := outputFormat
	origConfigFormat := config.OutputFormat
	defer func() {
		outputFormat = origFormat
		config.OutputFormat = origConfigFormat
	}()

	outputFormat = "yaml"
	initOutputFormat()

	if config.OutputFormat != "yaml" {
		t.Errorf("-o yaml: config.OutputFormat = %q, want %q", config.OutputFormat, "yaml")
	}
}

// TestOutputFlagJson tests that -o json sets config.OutputFormat = "json" (AC: 5.2)
func TestOutputFlagJson(t *testing.T) {
	origFormat := outputFormat
	origConfigFormat := config.OutputFormat
	defer func() {
		outputFormat = origFormat
		config.OutputFormat = origConfigFormat
	}()

	outputFormat = "json"
	initOutputFormat()

	if config.OutputFormat != "json" {
		t.Errorf("-o json: config.OutputFormat = %q, want %q", config.OutputFormat, "json")
	}
}

// TestOutputFlagText tests that -o text sets config.OutputFormat = "text" (AC: 5.3)
func TestOutputFlagText(t *testing.T) {
	origFormat := outputFormat
	origConfigFormat := config.OutputFormat
	defer func() {
		outputFormat = origFormat
		config.OutputFormat = origConfigFormat
	}()

	outputFormat = "text"
	initOutputFormat()

	if config.OutputFormat != "text" {
		t.Errorf("-o text: config.OutputFormat = %q, want %q", config.OutputFormat, "text")
	}
}

// TestOutputFlagDefault tests that default value is text (AC: 5.4)
func TestOutputFlagDefault(t *testing.T) {
	flag := rootCmd.PersistentFlags().Lookup("output")
	if flag == nil {
		t.Fatal("--output flag not found")
	}
	if flag.DefValue != "text" {
		t.Errorf("--output default = %q, want %q", flag.DefValue, "text")
	}
}

// TestOutputFlagInvalidFallback tests invalid format fallback to text (AC: 5.5)
func TestOutputFlagInvalidFallback(t *testing.T) {
	origFormat := outputFormat
	origConfigFormat := config.OutputFormat
	defer func() {
		outputFormat = origFormat
		config.OutputFormat = origConfigFormat
	}()

	invalidFormats := []string{"xml", "csv", "html", "markdown", ""}
	for _, format := range invalidFormats {
		outputFormat = format
		initOutputFormat()
		if config.OutputFormat != "text" {
			t.Errorf("invalid format %q: config.OutputFormat = %q, want %q",
				format, config.OutputFormat, "text")
		}
	}
}
