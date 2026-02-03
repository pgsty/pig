package ancs

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"pig/internal/config"
)

func TestRenderHelpNilCommand(t *testing.T) {
	err := RenderHelp(nil, config.OUTPUT_YAML)
	if err == nil {
		t.Error("expected error for nil command")
	}
}

func TestRenderHelpUnsupportedFormat(t *testing.T) {
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
	}

	err := RenderHelp(cmd, "invalid")
	if err == nil {
		t.Error("expected error for unsupported format")
	}
}

func TestRenderHelpYAML_RootCommand(t *testing.T) {
	// Test root command outputs CapabilityMap
	rootCmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
	}
	subCmd := &cobra.Command{
		Use:   "sub",
		Short: "Sub command",
	}
	rootCmd.AddCommand(subCmd)

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := RenderHelp(rootCmd, config.OUTPUT_YAML)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if err != nil {
		t.Fatalf("RenderHelp returned error: %v", err)
	}

	// Root command outputs CapabilityMap with version and commands
	if !strings.Contains(output, "name: test") {
		t.Errorf("expected 'name: test' in output, got: %s", output)
	}
	if !strings.Contains(output, "version:") {
		t.Errorf("expected 'version:' in output, got: %s", output)
	}
	if !strings.Contains(output, "commands:") {
		t.Errorf("expected 'commands:' in output, got: %s", output)
	}
}

func TestRenderHelpYAML_SubCommand(t *testing.T) {
	// Test subcommand outputs CommandSchema
	rootCmd := &cobra.Command{
		Use:   "root",
		Short: "Root command",
	}
	subCmd := &cobra.Command{
		Use:   "test <arg>",
		Short: "Test command",
		Annotations: map[string]string{
			"type": "action",
			"risk": "low",
		},
	}
	rootCmd.AddCommand(subCmd)

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := RenderHelp(subCmd, config.OUTPUT_YAML)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if err != nil {
		t.Fatalf("RenderHelp returned error: %v", err)
	}

	// Subcommand outputs CommandSchema with short description
	if !strings.Contains(output, "name: root test") {
		t.Errorf("expected 'name: root test' in output, got: %s", output)
	}
	if !strings.Contains(output, "short: Test command") {
		t.Errorf("expected 'short: Test command' in output, got: %s", output)
	}
}

func TestRenderHelpJSON(t *testing.T) {
	// Test subcommand to get CommandSchema output
	rootCmd := &cobra.Command{
		Use:   "root",
		Short: "Root command",
	}
	subCmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
	}
	rootCmd.AddCommand(subCmd)

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := RenderHelp(subCmd, config.OUTPUT_JSON)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if err != nil {
		t.Fatalf("RenderHelp returned error: %v", err)
	}

	if !strings.Contains(output, `"name":"root test"`) {
		t.Errorf("expected '\"name\":\"root test\"' in output, got: %s", output)
	}
}

func TestRenderHelpJSONPretty(t *testing.T) {
	// Test subcommand to get CommandSchema output
	rootCmd := &cobra.Command{
		Use:   "root",
		Short: "Root command",
	}
	subCmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
	}
	rootCmd.AddCommand(subCmd)

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := RenderHelp(subCmd, config.OUTPUT_JSON_PRETTY)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if err != nil {
		t.Fatalf("RenderHelp returned error: %v", err)
	}

	// Pretty JSON should have newlines and indentation
	if !strings.Contains(output, "\n") {
		t.Error("expected newlines in pretty JSON output")
	}
	if !strings.Contains(output, "  ") {
		t.Error("expected indentation in pretty JSON output")
	}
}

func TestSetupHelpNilCommand(t *testing.T) {
	// Should not panic
	SetupHelp(nil)
}

func TestHelpFuncTextFormat(t *testing.T) {
	// Save original format
	originalFormat := config.OutputFormat
	defer func() { config.OutputFormat = originalFormat }()

	config.OutputFormat = config.OUTPUT_TEXT

	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
		Long:  "This is a test command for help function testing",
	}

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Call HelpFunc
	HelpFunc(cmd, nil)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Text format should produce human-readable help
	// It should contain the long description
	if !strings.Contains(output, "test command for help function testing") {
		t.Errorf("expected long description in text help output, got: %s", output)
	}
}

func TestHelpFuncYAMLFormat(t *testing.T) {
	// Save original format
	originalFormat := config.OutputFormat
	defer func() { config.OutputFormat = originalFormat }()

	config.OutputFormat = config.OUTPUT_YAML

	// Use subcommand to test CommandSchema output
	rootCmd := &cobra.Command{
		Use:   "root",
		Short: "Root command",
	}
	subCmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
	}
	rootCmd.AddCommand(subCmd)

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Call HelpFunc on subcommand
	HelpFunc(subCmd, nil)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// YAML format should produce structured output
	if !strings.Contains(output, "name: root test") {
		t.Errorf("expected YAML output with 'name: root test', got: %s", output)
	}
}

func TestHelpFuncJSONFormat(t *testing.T) {
	// Save original format
	originalFormat := config.OutputFormat
	defer func() { config.OutputFormat = originalFormat }()

	config.OutputFormat = config.OUTPUT_JSON

	// Use subcommand to test CommandSchema output
	rootCmd := &cobra.Command{
		Use:   "root",
		Short: "Root command",
	}
	subCmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
	}
	rootCmd.AddCommand(subCmd)

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Call HelpFunc on subcommand
	HelpFunc(subCmd, nil)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// JSON format should produce structured output
	if !strings.Contains(output, `"name":"root test"`) {
		t.Errorf("expected JSON output with '\"name\":\"root test\"', got: %s", output)
	}
}

// Tests for Agent Hint functionality (Story 2.4)

func TestAgentHintConstant(t *testing.T) {
	// Test that the constant is defined correctly
	expected := "For agent/machine consumption: -o json | -o yaml"
	if AgentHintText != expected {
		t.Errorf("AgentHintText = %q, want %q", AgentHintText, expected)
	}
}

func TestAgentHintInTextModeHelp(t *testing.T) {
	// Save original format
	originalFormat := config.OutputFormat
	defer func() { config.OutputFormat = originalFormat }()

	config.OutputFormat = config.OUTPUT_TEXT

	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
		Long:  "This is a test command",
	}

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Call HelpFunc
	HelpFunc(cmd, nil)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Text mode should include the agent hint
	if !strings.Contains(output, AgentHintText) {
		t.Errorf("expected agent hint in text mode help, got: %s", output)
	}
	if !strings.Contains(output, "-o json") {
		t.Errorf("expected '-o json' in hint, got: %s", output)
	}
	if !strings.Contains(output, "-o yaml") {
		t.Errorf("expected '-o yaml' in hint, got: %s", output)
	}
}

func TestNoAgentHintInYAMLMode(t *testing.T) {
	// Save original format
	originalFormat := config.OutputFormat
	defer func() { config.OutputFormat = originalFormat }()

	config.OutputFormat = config.OUTPUT_YAML

	rootCmd := &cobra.Command{
		Use:   "root",
		Short: "Root command",
	}
	subCmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
	}
	rootCmd.AddCommand(subCmd)

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	HelpFunc(subCmd, nil)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// YAML mode should NOT include the agent hint
	if strings.Contains(output, AgentHintText) {
		t.Errorf("agent hint should not appear in YAML mode, got: %s", output)
	}
}

func TestNoAgentHintInJSONMode(t *testing.T) {
	// Save original format
	originalFormat := config.OutputFormat
	defer func() { config.OutputFormat = originalFormat }()

	config.OutputFormat = config.OUTPUT_JSON

	rootCmd := &cobra.Command{
		Use:   "root",
		Short: "Root command",
	}
	subCmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
	}
	rootCmd.AddCommand(subCmd)

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	HelpFunc(subCmd, nil)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// JSON mode should NOT include the agent hint
	if strings.Contains(output, AgentHintText) {
		t.Errorf("agent hint should not appear in JSON mode, got: %s", output)
	}
}

func TestAgentHintHasBlankLineSeparator(t *testing.T) {
	// Save original format
	originalFormat := config.OutputFormat
	defer func() { config.OutputFormat = originalFormat }()

	config.OutputFormat = config.OUTPUT_TEXT

	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
		Long:  "This is a test command",
	}

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	HelpFunc(cmd, nil)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// The hint should be preceded by a blank line (for visual separation)
	// Check that there's a double newline before the hint
	if !strings.Contains(output, "\n\n"+AgentHintText) {
		t.Errorf("expected blank line before agent hint, got: %s", output)
	}
}

func TestAgentHintUsesCommandOutputWriter(t *testing.T) {
	originalFormat := config.OutputFormat
	defer func() { config.OutputFormat = originalFormat }()
	config.OutputFormat = config.OUTPUT_TEXT

	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
	}

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	HelpFunc(cmd, nil)

	output := buf.String()
	if !strings.Contains(output, AgentHintText) {
		t.Errorf("expected agent hint in command output writer, got: %s", output)
	}
}
