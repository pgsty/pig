package config

import "testing"

func TestOutputFormatConstants(t *testing.T) {
	// Test that constants have expected values
	if OUTPUT_TEXT != "text" {
		t.Errorf("OUTPUT_TEXT = %q, want %q", OUTPUT_TEXT, "text")
	}
	if OUTPUT_YAML != "yaml" {
		t.Errorf("OUTPUT_YAML = %q, want %q", OUTPUT_YAML, "yaml")
	}
	if OUTPUT_JSON != "json" {
		t.Errorf("OUTPUT_JSON = %q, want %q", OUTPUT_JSON, "json")
	}
	if OUTPUT_JSON_PRETTY != "json-pretty" {
		t.Errorf("OUTPUT_JSON_PRETTY = %q, want %q", OUTPUT_JSON_PRETTY, "json-pretty")
	}
}

func TestOutputFormatDefault(t *testing.T) {
	// Test that default value is text
	if OutputFormat != OUTPUT_TEXT {
		t.Errorf("OutputFormat default = %q, want %q", OutputFormat, OUTPUT_TEXT)
	}
}

func TestValidOutputFormats(t *testing.T) {
	// Test that ValidOutputFormats contains all valid formats
	validFormats := map[string]bool{
		"text":        false,
		"yaml":        false,
		"json":        false,
		"json-pretty": false,
	}

	for _, f := range ValidOutputFormats {
		if _, ok := validFormats[f]; ok {
			validFormats[f] = true
		} else {
			t.Errorf("ValidOutputFormats contains unexpected format: %q", f)
		}
	}

	for f, found := range validFormats {
		if !found {
			t.Errorf("ValidOutputFormats missing format: %q", f)
		}
	}
}
