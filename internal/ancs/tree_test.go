package ancs

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// createTestCommandTree creates a mock command tree for testing
func createTestCommandTree() *cobra.Command {
	root := &cobra.Command{
		Use:   "pig",
		Short: "Postgres Install Guide",
	}

	// ext command with subcommands
	extCmd := &cobra.Command{
		Use:   "ext",
		Short: "Extension management commands",
		Annotations: map[string]string{
			"type":       "query",
			"volatility": "stable",
			"parallel":   "safe",
			"idempotent": "true",
			"risk":       "safe",
			"confirm":    "none",
			"os_user":    "current",
			"cost":       "0",
		},
	}

	extListCmd := &cobra.Command{
		Use:   "list [query]",
		Short: "List available extensions",
		Annotations: map[string]string{
			"type":       "query",
			"volatility": "immutable",
			"parallel":   "safe",
			"idempotent": "true",
			"risk":       "safe",
			"confirm":    "none",
			"os_user":    "current",
			"cost":       "100",
		},
	}

	extAddCmd := &cobra.Command{
		Use:   "add <name...>",
		Short: "Install PostgreSQL extensions",
		Annotations: map[string]string{
			"type":       "action",
			"volatility": "stable",
			"parallel":   "safe",
			"idempotent": "true",
			"risk":       "low",
			"confirm":    "none",
			"os_user":    "root",
			"cost":       "5000",
		},
	}

	extCmd.AddCommand(extListCmd, extAddCmd)

	// repo command with subcommands
	repoCmd := &cobra.Command{
		Use:   "repo",
		Short: "Repository management commands",
	}

	repoAddCmd := &cobra.Command{
		Use:   "add <name...>",
		Short: "Add repositories",
	}

	repoCmd.AddCommand(repoAddCmd)

	// Hidden command (should be skipped)
	hiddenCmd := &cobra.Command{
		Use:    "hidden",
		Short:  "Hidden command",
		Hidden: true,
	}

	// Status command (no subcommands)
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show environment status",
	}

	root.AddCommand(extCmd, repoCmd, hiddenCmd, statusCmd)
	return root
}

func TestBuildCapabilityMap_Nil(t *testing.T) {
	result := BuildCapabilityMap(nil)
	if result != nil {
		t.Errorf("BuildCapabilityMap(nil) = %v, want nil", result)
	}
}

func TestBuildCapabilityMap_Basic(t *testing.T) {
	root := createTestCommandTree()
	capMap := BuildCapabilityMap(root)

	if capMap == nil {
		t.Fatal("BuildCapabilityMap returned nil for valid command tree")
	}

	if capMap.Name != "pig" {
		t.Errorf("CapabilityMap.Name = %q, want %q", capMap.Name, "pig")
	}

	if capMap.Description != "Postgres Install Guide" {
		t.Errorf("CapabilityMap.Description = %q, want %q", capMap.Description, "Postgres Install Guide")
	}
	if capMap.Schema == nil {
		t.Fatal("CapabilityMap.Schema should not be nil")
	}
	if capMap.Schema.Name != "pig" {
		t.Errorf("CapabilityMap.Schema.Name = %q, want %q", capMap.Schema.Name, "pig")
	}

	if !strings.HasPrefix(capMap.Version, "v") {
		t.Errorf("CapabilityMap.Version = %q, should start with 'v'", capMap.Version)
	}
}

func TestBuildCapabilityMap_Commands(t *testing.T) {
	root := createTestCommandTree()
	capMap := BuildCapabilityMap(root)

	if capMap == nil {
		t.Fatal("BuildCapabilityMap returned nil")
	}

	// Should have 3 commands (ext, repo, status) - hidden is skipped
	// Note: help command is also auto-added by cobra but we skip it
	expectedCount := 3
	if len(capMap.Commands) < expectedCount {
		t.Errorf("CapabilityMap.Commands count = %d, want at least %d", len(capMap.Commands), expectedCount)
	}

	// Check that hidden command is not included
	for _, cmd := range capMap.Commands {
		if cmd.Name == "hidden" {
			t.Errorf("Hidden command should not be included in CapabilityMap")
		}
		if cmd.Name == "help" {
			t.Errorf("Help command should not be included in CapabilityMap")
		}
	}
}

func TestBuildCapabilityMap_SubCommands(t *testing.T) {
	root := createTestCommandTree()
	capMap := BuildCapabilityMap(root)

	if capMap == nil {
		t.Fatal("BuildCapabilityMap returned nil")
	}

	// Find ext command
	var extNode *CommandNode
	for _, cmd := range capMap.Commands {
		if cmd.Name == "ext" {
			extNode = cmd
			break
		}
	}

	if extNode == nil {
		t.Fatal("ext command not found in CapabilityMap")
	}

	// ext should have 2 subcommands (list, add)
	if len(extNode.SubCommands) < 2 {
		t.Errorf("ext.SubCommands count = %d, want at least 2", len(extNode.SubCommands))
	}

	// Check full name
	if extNode.FullName != "pig ext" {
		t.Errorf("ext.FullName = %q, want %q", extNode.FullName, "pig ext")
	}

	// Check schema is populated
	if extNode.Schema == nil {
		t.Error("ext.Schema should not be nil")
	} else {
		if extNode.Schema.Type != TYPE_QUERY {
			t.Errorf("ext.Schema.Type = %q, want %q", extNode.Schema.Type, TYPE_QUERY)
		}
	}
}

func TestBuildCapabilityMap_Schema(t *testing.T) {
	root := createTestCommandTree()
	capMap := BuildCapabilityMap(root)

	if capMap == nil {
		t.Fatal("BuildCapabilityMap returned nil")
	}

	// Find ext list command
	var listNode *CommandNode
	for _, cmd := range capMap.Commands {
		if cmd.Name == "ext" {
			for _, sub := range cmd.SubCommands {
				if sub.Name == "list" {
					listNode = sub
					break
				}
			}
			break
		}
	}

	if listNode == nil {
		t.Fatal("ext list command not found")
	}

	if listNode.Schema == nil {
		t.Fatal("ext list Schema is nil")
	}

	// Verify schema values
	schema := listNode.Schema
	if schema.Type != TYPE_QUERY {
		t.Errorf("Schema.Type = %q, want %q", schema.Type, TYPE_QUERY)
	}
	if schema.Volatility != VOLATILITY_IMMUTABLE {
		t.Errorf("Schema.Volatility = %q, want %q", schema.Volatility, VOLATILITY_IMMUTABLE)
	}
	if !schema.Idempotent {
		t.Error("Schema.Idempotent should be true")
	}
	if schema.Cost != 100 {
		t.Errorf("Schema.Cost = %d, want %d", schema.Cost, 100)
	}
}

func TestBuildCommandNode_Nil(t *testing.T) {
	result := buildCommandNode(nil)
	if result != nil {
		t.Errorf("buildCommandNode(nil) = %v, want nil", result)
	}
}

func TestBuildCommandNode_Hidden(t *testing.T) {
	cmd := &cobra.Command{
		Use:    "hidden",
		Hidden: true,
	}
	result := buildCommandNode(cmd)
	if result != nil {
		t.Errorf("buildCommandNode(hidden) = %v, want nil", result)
	}
}

func TestBuildCommandNode_Help(t *testing.T) {
	cmd := &cobra.Command{
		Use: "help",
	}
	result := buildCommandNode(cmd)
	if result != nil {
		t.Errorf("buildCommandNode(help) = %v, want nil", result)
	}
}

func TestBuildCommandNode_Completion(t *testing.T) {
	cmd := &cobra.Command{
		Use: "completion",
	}
	result := buildCommandNode(cmd)
	if result != nil {
		t.Errorf("buildCommandNode(completion) = %v, want nil", result)
	}
}

func TestBuildCapabilityMap_SkipsCompletionSubCommand(t *testing.T) {
	// Test that completion command is skipped when it's a subcommand
	root := &cobra.Command{
		Use:   "test",
		Short: "Test command",
	}
	completionCmd := &cobra.Command{
		Use:   "completion",
		Short: "Generate shell completion scripts",
	}
	normalCmd := &cobra.Command{
		Use:   "normal",
		Short: "Normal command",
	}
	root.AddCommand(completionCmd, normalCmd)

	capMap := BuildCapabilityMap(root)
	if capMap == nil {
		t.Fatal("BuildCapabilityMap returned nil")
	}

	// Should only have 'normal' command, not 'completion'
	for _, cmd := range capMap.Commands {
		if cmd.Name == "completion" {
			t.Error("completion command should not be included in CapabilityMap")
		}
	}

	// Verify normal command is included
	found := false
	for _, cmd := range capMap.Commands {
		if cmd.Name == "normal" {
			found = true
			break
		}
	}
	if !found {
		t.Error("normal command should be included in CapabilityMap")
	}
}

func TestBuildCapabilityMap_SchemaNamePopulated(t *testing.T) {
	// Test that Schema.Name is automatically populated from CommandPath
	root := &cobra.Command{
		Use:   "pig",
		Short: "Test root",
	}
	extCmd := &cobra.Command{
		Use:   "ext",
		Short: "Extension command",
		// No "name" in Annotations
		Annotations: map[string]string{
			"type": "query",
		},
	}
	root.AddCommand(extCmd)

	capMap := BuildCapabilityMap(root)
	if capMap == nil {
		t.Fatal("BuildCapabilityMap returned nil")
	}

	// Find ext command
	var extNode *CommandNode
	for _, cmd := range capMap.Commands {
		if cmd.Name == "ext" {
			extNode = cmd
			break
		}
	}

	if extNode == nil {
		t.Fatal("ext command not found")
	}

	// Schema.Name should be populated with command path
	if extNode.Schema == nil {
		t.Fatal("ext.Schema is nil")
	}
	if extNode.Schema.Name != "pig ext" {
		t.Errorf("Schema.Name = %q, want %q", extNode.Schema.Name, "pig ext")
	}
}

func TestCapabilityMap_YAML_Nil(t *testing.T) {
	var c *CapabilityMap
	_, err := c.YAML()
	if err == nil {
		t.Error("CapabilityMap.YAML() on nil should return error")
	}
}

func TestCapabilityMap_JSON_Nil(t *testing.T) {
	var c *CapabilityMap
	_, err := c.JSON()
	if err == nil {
		t.Error("CapabilityMap.JSON() on nil should return error")
	}
}

func TestCapabilityMap_JSONPretty_Nil(t *testing.T) {
	var c *CapabilityMap
	_, err := c.JSONPretty()
	if err == nil {
		t.Error("CapabilityMap.JSONPretty() on nil should return error")
	}
}

func TestCommandNode_YAML_Nil(t *testing.T) {
	var n *CommandNode
	_, err := n.YAML()
	if err == nil {
		t.Error("CommandNode.YAML() on nil should return error")
	}
}

func TestCommandNode_JSON_Nil(t *testing.T) {
	var n *CommandNode
	_, err := n.JSON()
	if err == nil {
		t.Error("CommandNode.JSON() on nil should return error")
	}
}

func TestCommandNode_JSONPretty_Nil(t *testing.T) {
	var n *CommandNode
	_, err := n.JSONPretty()
	if err == nil {
		t.Error("CommandNode.JSONPretty() on nil should return error")
	}
}

func TestCommandNode_JSONPretty(t *testing.T) {
	node := &CommandNode{
		Name:     "test",
		FullName: "pig test",
		Short:    "Test command",
	}

	data, err := node.JSONPretty()
	if err != nil {
		t.Fatalf("JSONPretty() error: %v", err)
	}

	output := string(data)
	if !strings.Contains(output, "\n") {
		t.Error("JSONPretty should contain newlines")
	}
	if !strings.Contains(output, "  ") {
		t.Error("JSONPretty should contain indentation")
	}
}

func TestCapabilityMap_YAML(t *testing.T) {
	root := createTestCommandTree()
	capMap := BuildCapabilityMap(root)

	data, err := capMap.YAML()
	if err != nil {
		t.Fatalf("CapabilityMap.YAML() error: %v", err)
	}

	// Verify it's valid YAML
	var parsed map[string]interface{}
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Invalid YAML output: %v", err)
	}

	// Check key fields
	if parsed["name"] != "pig" {
		t.Errorf("YAML name = %v, want 'pig'", parsed["name"])
	}

	// Check commands exist
	if _, ok := parsed["commands"]; !ok {
		t.Error("YAML output missing 'commands' field")
	}
}

func TestCapabilityMap_JSON(t *testing.T) {
	root := createTestCommandTree()
	capMap := BuildCapabilityMap(root)

	data, err := capMap.JSON()
	if err != nil {
		t.Fatalf("CapabilityMap.JSON() error: %v", err)
	}

	// Verify it's valid JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Invalid JSON output: %v", err)
	}

	// Check key fields
	if parsed["name"] != "pig" {
		t.Errorf("JSON name = %v, want 'pig'", parsed["name"])
	}
}

func TestCapabilityMap_JSONPretty(t *testing.T) {
	root := createTestCommandTree()
	capMap := BuildCapabilityMap(root)

	data, err := capMap.JSONPretty()
	if err != nil {
		t.Fatalf("CapabilityMap.JSONPretty() error: %v", err)
	}

	// Pretty JSON should contain newlines and indentation
	output := string(data)
	if !strings.Contains(output, "\n") {
		t.Error("JSONPretty should contain newlines")
	}
	if !strings.Contains(output, "  ") {
		t.Error("JSONPretty should contain indentation")
	}

	// Verify it's valid JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Invalid JSON output: %v", err)
	}
}

func TestCapabilityMap_YAMLFieldNames(t *testing.T) {
	root := createTestCommandTree()
	capMap := BuildCapabilityMap(root)

	data, err := capMap.YAML()
	if err != nil {
		t.Fatalf("YAML() error: %v", err)
	}

	output := string(data)

	// Check snake_case field names
	expectedFields := []string{"name:", "version:", "description:", "schema:", "commands:", "full_name:", "short:"}
	for _, field := range expectedFields {
		if !strings.Contains(output, field) {
			t.Errorf("YAML output missing field %q", field)
		}
	}

	// Check sub_commands field
	if !strings.Contains(output, "sub_commands:") {
		t.Error("YAML output missing 'sub_commands' field")
	}
}

func TestCapabilityMap_JSONFieldNames(t *testing.T) {
	root := createTestCommandTree()
	capMap := BuildCapabilityMap(root)

	data, err := capMap.JSONPretty()
	if err != nil {
		t.Fatalf("JSONPretty() error: %v", err)
	}

	output := string(data)

	// Check snake_case field names in JSON
	expectedFields := []string{`"name"`, `"version"`, `"description"`, `"schema"`, `"commands"`, `"full_name"`, `"short"`}
	for _, field := range expectedFields {
		if !strings.Contains(output, field) {
			t.Errorf("JSON output missing field %s", field)
		}
	}

	// Check sub_commands field
	if !strings.Contains(output, `"sub_commands"`) {
		t.Error("JSON output missing 'sub_commands' field")
	}
}

func TestCapabilityMap_NoEmptySubCommands(t *testing.T) {
	root := createTestCommandTree()
	capMap := BuildCapabilityMap(root)

	// Find status command (no subcommands)
	var statusNode *CommandNode
	for _, cmd := range capMap.Commands {
		if cmd.Name == "status" {
			statusNode = cmd
			break
		}
	}

	if statusNode == nil {
		t.Fatal("status command not found")
	}

	// SubCommands should be nil or empty
	if len(statusNode.SubCommands) != 0 {
		t.Errorf("status.SubCommands should be empty, got %d", len(statusNode.SubCommands))
	}

	// Check YAML output doesn't include sub_commands for status
	data, err := capMap.YAML()
	if err != nil {
		t.Fatalf("YAML() error: %v", err)
	}

	// Parse and check structure
	var parsed CapabilityMap
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to parse YAML: %v", err)
	}

	for _, cmd := range parsed.Commands {
		if cmd.Name == "status" {
			// With omitempty, SubCommands should be nil when empty
			if len(cmd.SubCommands) != 0 {
				t.Errorf("Parsed status.SubCommands should be empty")
			}
		}
	}
}

func TestBuildCapabilityMap_Performance(t *testing.T) {
	root := createLargeCommandTree(100)

	start := time.Now()
	capMap := BuildCapabilityMap(root)
	elapsed := time.Since(start)

	if capMap == nil {
		t.Fatal("BuildCapabilityMap returned nil")
	}
	if elapsed > 200*time.Millisecond {
		t.Fatalf("BuildCapabilityMap took %s, expected < 200ms", elapsed)
	}
}

// BenchmarkBuildCapabilityMap measures the performance of building the capability map
func BenchmarkBuildCapabilityMap(b *testing.B) {
	root := createTestCommandTree()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		BuildCapabilityMap(root)
	}
}

func createLargeCommandTree(count int) *cobra.Command {
	root := &cobra.Command{
		Use:   "pig",
		Short: "Postgres Install Guide",
	}
	for i := 0; i < count; i++ {
		cmd := &cobra.Command{
			Use:   fmt.Sprintf("cmd-%d", i),
			Short: "Test command",
		}
		// Add a couple of subcommands to simulate depth
		for j := 0; j < 3; j++ {
			sub := &cobra.Command{
				Use:   fmt.Sprintf("sub-%d-%d", i, j),
				Short: "Sub command",
			}
			cmd.AddCommand(sub)
		}
		root.AddCommand(cmd)
	}
	return root
}
