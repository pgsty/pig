package cmd

import (
	"encoding/json"
	"pig/internal/ancs"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// allCommands recursively collects all commands from a root command.
func allCommands(cmd *cobra.Command) []*cobra.Command {
	var result []*cobra.Command
	for _, c := range cmd.Commands() {
		if c.Name() == "help" || c.Name() == "completion" || c.Hidden {
			continue
		}
		result = append(result, c)
		result = append(result, allCommands(c)...)
	}
	return result
}

// leafCommands returns commands that have a Run or RunE function (i.e., executable commands).
func leafCommands(cmd *cobra.Command) []*cobra.Command {
	var result []*cobra.Command
	for _, c := range allCommands(cmd) {
		if c.Run != nil || c.RunE != nil {
			result = append(result, c)
		}
	}
	return result
}

// validTypes is the set of valid ANCS type values.
var validTypes = map[string]bool{
	"query": true, "action": true,
}

// validVolatilities is the set of valid ANCS volatility values.
var validVolatilities = map[string]bool{
	"immutable": true, "stable": true, "volatile": true,
}

// validParallels is the set of valid ANCS parallel values.
var validParallels = map[string]bool{
	"safe": true, "restricted": true, "unsafe": true,
}

// validRisks is the set of valid ANCS risk values.
var validRisks = map[string]bool{
	"safe": true, "low": true, "medium": true, "high": true, "critical": true,
}

// validConfirms is the set of valid ANCS confirm values.
var validConfirms = map[string]bool{
	"none": true, "recommended": true, "required": true,
}

// validOSUsers is the set of valid ANCS os_user values.
var validOSUsers = map[string]bool{
	"current": true, "root": true, "dbsu": true,
}

// requiredAnnotationKeys are the 9 ANCS fields that every annotated command must have.
var requiredAnnotationKeys = []string{
	"name", "type", "volatility", "parallel", "idempotent",
	"risk", "confirm", "os_user", "cost",
}

// TestAllLeafCommandsHaveAnnotations verifies that every command with a Run/RunE
// function has ANCS Annotations with all 9 required fields.
func TestAllLeafCommandsHaveAnnotations(t *testing.T) {
	cmds := leafCommands(rootCmd)
	if len(cmds) == 0 {
		t.Fatal("no leaf commands found; command tree may not be initialized")
	}

	for _, cmd := range cmds {
		path := cmd.CommandPath()
		t.Run(path, func(t *testing.T) {
			ann := cmd.Annotations
			if len(ann) == 0 {
				t.Errorf("command %q has Run/RunE but no Annotations", path)
				return
			}

			for _, key := range requiredAnnotationKeys {
				if _, ok := ann[key]; !ok {
					t.Errorf("command %q missing annotation key %q", path, key)
				}
			}
		})
	}
}

// TestAnnotationNameConsistency verifies that the "name" annotation matches
// the canonical command path (CommandPath), ensuring a stable schema identity.
func TestAnnotationNameConsistency(t *testing.T) {
	cmds := allCommands(rootCmd)

	for _, cmd := range cmds {
		ann := cmd.Annotations
		if ann == nil {
			continue
		}
		name, ok := ann["name"]
		if !ok || name == "" {
			continue
		}
		path := cmd.CommandPath()
		t.Run(path, func(t *testing.T) {
			if name != path {
				t.Errorf("annotation name=%q does not match command path %q", name, path)
			}
		})
	}
}

// TestAnnotationEnumValidity verifies that all ANCS enum fields contain valid values.
func TestAnnotationEnumValidity(t *testing.T) {
	cmds := leafCommands(rootCmd)

	for _, cmd := range cmds {
		path := cmd.CommandPath()
		ann := cmd.Annotations
		if len(ann) == 0 {
			continue
		}

		t.Run(path, func(t *testing.T) {
			validateAnnotationEnums(t, ann)
			validateAnnotationIdempotent(t, ann)
			validateAnnotationCost(t, ann)
		})
	}
}

// TestParentCommandsHaveAnnotations verifies that parent/group commands
// (commands with subcommands) also have Annotations.
func TestParentCommandsHaveAnnotations(t *testing.T) {
	cmds := allCommands(rootCmd)

	for _, cmd := range cmds {
		if !cmd.HasSubCommands() {
			continue
		}
		path := cmd.CommandPath()
		t.Run(path, func(t *testing.T) {
			ann := cmd.Annotations
			if len(ann) == 0 {
				t.Errorf("parent command %q has subcommands but no Annotations", path)
				return
			}
			if _, ok := ann["name"]; !ok {
				t.Errorf("parent command %q missing annotation key \"name\"", path)
			}
		})
	}
}

// TestFromAnnotationsRoundTrip verifies that FromAnnotations correctly parses
// every annotated command's annotations into a valid Schema.
func TestFromAnnotationsRoundTrip(t *testing.T) {
	cmds := allCommands(rootCmd)

	for _, cmd := range cmds {
		ann := cmd.Annotations
		if len(ann) == 0 {
			continue
		}
		path := cmd.CommandPath()
		t.Run(path, func(t *testing.T) {
			schema := ancs.FromAnnotations(ann)
			if schema == nil {
				t.Fatal("FromAnnotations returned nil")
			}
			if schema.Name == "" {
				t.Error("Schema.Name is empty")
			}
			if schema.Type == "" {
				t.Error("Schema.Type is empty")
			}
		})
	}
}

// TestCommandSchemaYAMLParseable verifies that CommandSchema YAML output is parseable.
func TestCommandSchemaYAMLParseable(t *testing.T) {
	// Test a sample of commands using their full Use names (CommandPath)
	testCmds := []string{"pig ext", "pig postgres", "pig pgbackrest", "pig patroni", "pig pitr"}
	cmds := allCommands(rootCmd)

	cmdMap := make(map[string]*cobra.Command)
	for _, c := range cmds {
		cmdMap[c.CommandPath()] = c
	}

	for _, name := range testCmds {
		cmd, ok := cmdMap[name]
		if !ok {
			t.Logf("skipping %q (not found in command tree)", name)
			continue
		}
		t.Run(name, func(t *testing.T) {
			cs := ancs.FromCommand(cmd)
			if cs == nil {
				t.Fatal("FromCommand returned nil")
			}
			data, err := cs.YAML()
			if err != nil {
				t.Fatalf("YAML() error: %v", err)
			}
			var parsed map[string]interface{}
			if err := yaml.Unmarshal(data, &parsed); err != nil {
				t.Fatalf("YAML output not parseable: %v", err)
			}
			if parsed["name"] != name {
				t.Errorf("YAML name=%v, want %q", parsed["name"], name)
			}
		})
	}
}

// TestCommandSchemaJSONParseable verifies that CommandSchema JSON output is parseable.
func TestCommandSchemaJSONParseable(t *testing.T) {
	// Test leaf commands with args and flags using full CommandPath
	testCmds := []string{"pig ext add", "pig postgres start", "pig pgbackrest backup"}
	cmds := allCommands(rootCmd)

	cmdMap := make(map[string]*cobra.Command)
	for _, c := range cmds {
		cmdMap[c.CommandPath()] = c
	}

	for _, name := range testCmds {
		cmd, ok := cmdMap[name]
		if !ok {
			t.Logf("skipping %q (not found in command tree)", name)
			continue
		}
		t.Run(name, func(t *testing.T) {
			cs := ancs.FromCommand(cmd)
			if cs == nil {
				t.Fatal("FromCommand returned nil")
			}
			data, err := cs.JSON()
			if err != nil {
				t.Fatalf("JSON() error: %v", err)
			}
			var parsed map[string]interface{}
			if err := json.Unmarshal(data, &parsed); err != nil {
				t.Fatalf("JSON output not parseable: %v", err)
			}
			if parsed["name"] != name {
				t.Errorf("JSON name=%v, want %q", parsed["name"], name)
			}
			// Verify schema is present
			if _, ok := parsed["schema"]; !ok {
				t.Error("JSON output missing 'schema' field")
			}
		})
	}
}

// TestCapabilityMapContainsAllTopLevelCommands verifies that BuildCapabilityMap
// includes all top-level command groups registered in rootCmd.
func TestCapabilityMapContainsAllTopLevelCommands(t *testing.T) {
	capMap := ancs.BuildCapabilityMap(rootCmd)
	if capMap == nil {
		t.Fatal("BuildCapabilityMap returned nil")
	}

	// Expected top-level commands using their Use field names (not aliases).
	// Cobra's Name() returns the Use field, so "postgres" not "pg".
	// Note: "license" is Hidden so excluded from CapabilityMap.
	expectedCmds := []string{
		"ext", "repo", "build", "install",
		"postgres", "patroni", "pgbackrest", "pitr", "pg_exporter", "do", "sty",
		"status", "version", "update", "context",
	}

	found := make(map[string]bool)
	for _, node := range capMap.Commands {
		found[node.Name] = true
	}

	for _, name := range expectedCmds {
		if !found[name] {
			t.Errorf("CapabilityMap missing top-level command %q", name)
		}
	}
}

// TestCapabilityMapYAMLParseable verifies CapabilityMap YAML is parseable.
func TestCapabilityMapYAMLParseable(t *testing.T) {
	capMap := ancs.BuildCapabilityMap(rootCmd)
	if capMap == nil {
		t.Fatal("BuildCapabilityMap returned nil")
	}

	data, err := capMap.YAML()
	if err != nil {
		t.Fatalf("YAML() error: %v", err)
	}

	var parsed map[string]interface{}
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("YAML output not parseable: %v", err)
	}

	if parsed["name"] != "pig" {
		t.Errorf("YAML name=%v, want %q", parsed["name"], "pig")
	}

	cmds, ok := parsed["commands"].([]interface{})
	if !ok {
		t.Fatal("YAML 'commands' field is not a list")
	}
	if len(cmds) == 0 {
		t.Error("YAML 'commands' is empty")
	}
}

// TestCapabilityMapJSONParseable verifies CapabilityMap JSON is parseable.
func TestCapabilityMapJSONParseable(t *testing.T) {
	capMap := ancs.BuildCapabilityMap(rootCmd)
	if capMap == nil {
		t.Fatal("BuildCapabilityMap returned nil")
	}

	data, err := capMap.JSON()
	if err != nil {
		t.Fatalf("JSON() error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("JSON output not parseable: %v", err)
	}

	if parsed["name"] != "pig" {
		t.Errorf("JSON name=%v, want %q", parsed["name"], "pig")
	}
}

// TestSubcommandSchemaHasArgsAndFlags verifies that CommandSchema correctly
// extracts args and flags from specific commands known to have them.
func TestSubcommandSchemaHasArgsAndFlags(t *testing.T) {
	cmds := allCommands(rootCmd)
	cmdMap := make(map[string]*cobra.Command)
	for _, c := range cmds {
		cmdMap[c.CommandPath()] = c
	}

	tests := []struct {
		path     string
		hasArgs  bool
		hasFlags bool
	}{
		{"pig ext add", false, true},  // Use: "add" (no args in Use string)
		{"pig ext info", false, true}, // Use: "info" (no args in Use string)
		{"pig postgres start", false, true},
		{"pig pgbackrest backup", false, true},
		{"pig pitr", false, true},
	}

	for _, tt := range tests {
		cmd, ok := cmdMap[tt.path]
		if !ok {
			t.Logf("skipping %q (not found)", tt.path)
			continue
		}
		t.Run(tt.path, func(t *testing.T) {
			cs := ancs.FromCommand(cmd)
			if cs == nil {
				t.Fatal("FromCommand returned nil")
			}
			if tt.hasArgs && len(cs.Args) == 0 {
				t.Errorf("expected args for %q, got none", tt.path)
			}
			if tt.hasFlags && len(cs.Flags) == 0 {
				t.Errorf("expected flags for %q, got none", tt.path)
			}
		})
	}
}

// TestAnnotationCountSanity verifies the total number of annotated commands
// meets a minimum threshold to catch accidentally dropped annotations.
func TestAnnotationCountSanity(t *testing.T) {
	cmds := allCommands(rootCmd)

	annotatedCount := 0
	for _, cmd := range cmds {
		if len(cmd.Annotations) > 0 {
			annotatedCount++
		}
	}

	// We expect at least 100 annotated commands across all groups.
	// This prevents silent regression where annotations are accidentally removed.
	const minExpected = 100
	if annotatedCount < minExpected {
		t.Errorf("only %d annotated commands found, expected at least %d", annotatedCount, minExpected)
	}
	t.Logf("found %d annotated commands total", annotatedCount)
}

// TestFlagChoicesAnnotationsEndToEnd verifies that commands with flag choices
// annotations produce correct schema output with choices populated.
func TestFlagChoicesAnnotationsEndToEnd(t *testing.T) {
	cmds := allCommands(rootCmd)
	cmdMap := make(map[string]*cobra.Command)
	for _, c := range cmds {
		cmdMap[c.CommandPath()] = c
	}

	tests := []struct {
		path     string
		flagName string
		choices  []string
	}{
		{"pig postgres stop", "mode", []string{"smart", "fast", "immediate"}},
		{"pig postgres restart", "mode", []string{"smart", "fast", "immediate"}},
	}

	for _, tt := range tests {
		cmd, ok := cmdMap[tt.path]
		if !ok {
			t.Logf("skipping %q (not found)", tt.path)
			continue
		}
		t.Run(tt.path+"/"+tt.flagName, func(t *testing.T) {
			cs := ancs.FromCommand(cmd)
			if cs == nil {
				t.Fatal("FromCommand returned nil")
			}

			var found *ancs.Flag
			for i := range cs.Flags {
				if cs.Flags[i].Name == tt.flagName {
					found = &cs.Flags[i]
					break
				}
			}
			if found == nil {
				t.Fatalf("flag %q not found in schema", tt.flagName)
			}
			if len(found.Choices) != len(tt.choices) {
				t.Fatalf("expected %d choices, got %d: %v", len(tt.choices), len(found.Choices), found.Choices)
			}
			for i, c := range found.Choices {
				if c != tt.choices[i] {
					t.Errorf("choice[%d]: expected %q, got %q", i, tt.choices[i], c)
				}
			}
		})
	}
}

// TestOutputFlagChoicesFromRoot verifies that the global --output flag choices
// are propagated to child commands via inherited flags.
func TestOutputFlagChoicesFromRoot(t *testing.T) {
	// rootCmd has flags.output.choices annotation
	ann := rootCmd.Annotations
	if ann == nil {
		t.Fatal("rootCmd has no Annotations")
	}
	choices, ok := ann["flags.output.choices"]
	if !ok {
		t.Fatal("rootCmd missing flags.output.choices annotation")
	}
	if choices != "text,yaml,json,json-pretty" {
		t.Errorf("expected 'text,yaml,json,json-pretty', got %q", choices)
	}

	// Verify a child command sees inherited output flag choices in schema
	cmds := allCommands(rootCmd)
	cmdMap := make(map[string]*cobra.Command)
	for _, c := range cmds {
		cmdMap[c.CommandPath()] = c
	}
	cmd, ok := cmdMap["pig postgres status"]
	if !ok {
		t.Skip("command 'pig postgres status' not found; skip inherited output choices check")
	}
	cs := ancs.FromCommand(cmd)
	if cs == nil {
		t.Fatal("FromCommand returned nil")
	}
	var outputFlag *ancs.Flag
	for i := range cs.Flags {
		if cs.Flags[i].Name == "output" {
			outputFlag = &cs.Flags[i]
			break
		}
	}
	if outputFlag == nil {
		t.Fatalf("output flag not found in schema for %q", cmd.CommandPath())
	}
	expected := []string{"text", "yaml", "json", "json-pretty"}
	if len(outputFlag.Choices) != len(expected) {
		t.Fatalf("expected %d output choices, got %d: %v", len(expected), len(outputFlag.Choices), outputFlag.Choices)
	}
	for i, c := range expected {
		if outputFlag.Choices[i] != c {
			t.Errorf("output choice[%d]: expected %q, got %q", i, c, outputFlag.Choices[i])
		}
	}
}

// TestAnnotationNamePrefix verifies all annotation names start with "pig ".
func TestAnnotationNamePrefix(t *testing.T) {
	cmds := allCommands(rootCmd)

	for _, cmd := range cmds {
		ann := cmd.Annotations
		if ann == nil {
			continue
		}
		name, ok := ann["name"]
		if !ok || name == "" {
			continue
		}
		if !strings.HasPrefix(name, "pig ") && name != "pig" {
			t.Errorf("command %q has annotation name=%q which doesn't start with \"pig \"",
				cmd.CommandPath(), name)
		}
	}
}

// TestAnnotationsCoverage traverses all registered commands and verifies that
// every command with RunE/Run has Annotations with at least the 3 critical
// fields (type, risk, os_user). It reports any missing commands for tracking.
// (AC#5: 能力声明覆盖率验证测试)
func TestAnnotationsCoverage(t *testing.T) {
	cmds := leafCommands(rootCmd)
	if len(cmds) == 0 {
		t.Fatal("no leaf commands found; command tree may not be initialized")
	}

	criticalFields := []string{"type", "risk", "os_user"}
	var missing []string

	for _, cmd := range cmds {
		path := cmd.CommandPath()
		ann := cmd.Annotations
		if len(ann) == 0 {
			missing = append(missing, path)
			continue
		}
		// Verify critical fields exist and have valid values
		for _, key := range criticalFields {
			v, ok := ann[key]
			if !ok || v == "" {
				t.Errorf("command %q missing critical annotation %q", path, key)
			}
		}
		// Validate critical field values
		if v, ok := ann["type"]; ok && !validTypes[v] {
			t.Errorf("command %q has invalid type=%q", path, v)
		}
		if v, ok := ann["risk"]; ok && !validRisks[v] {
			t.Errorf("command %q has invalid risk=%q", path, v)
		}
		if v, ok := ann["os_user"]; ok && !validOSUsers[v] {
			t.Errorf("command %q has invalid os_user=%q", path, v)
		}
	}

	if len(missing) > 0 {
		t.Errorf("commands missing Annotations (%d):\n%s", len(missing), strings.Join(missing, "\n"))
	}
	t.Logf("coverage: %d/%d leaf commands have Annotations", len(cmds)-len(missing), len(cmds))
}

// riskLevel returns a numeric level for risk values for comparison.
func riskLevel(risk string) int {
	switch risk {
	case "safe":
		return 0
	case "low":
		return 1
	case "medium":
		return 2
	case "high":
		return 3
	case "critical":
		return 4
	default:
		return -1
	}
}

// confirmLevel returns a numeric level for confirm values for comparison.
func confirmLevel(confirm string) int {
	switch confirm {
	case "none":
		return 0
	case "recommended":
		return 1
	case "required":
		return 2
	default:
		return -1
	}
}

// TestAnnotationsSemanticConsistency verifies that Annotations values are
// semantically consistent according to the ANCS rules:
// 1. query commands have risk <= low
// 2. high/critical risk commands have confirm >= recommended
// 3. critical risk commands have confirm = required
// 4. all enum values are valid
// 5. cost is a non-negative integer
// 6. name field is consistent with command path
// (AC#6: 语义一致性验证测试)
func TestAnnotationsSemanticConsistency(t *testing.T) {
	cmds := allCommands(rootCmd)

	for _, cmd := range cmds {
		ann := cmd.Annotations
		if len(ann) == 0 {
			continue
		}
		path := cmd.CommandPath()
		schema := ancs.FromAnnotations(ann)
		if schema == nil {
			continue
		}

		t.Run(path, func(t *testing.T) {
			// Rule 1: query commands should have risk <= low
			if schema.Type == "query" && riskLevel(schema.Risk) > riskLevel("low") {
				t.Errorf("query command has risk=%q (expected safe or low)", schema.Risk)
			}

			// Rule 2: high/critical risk commands must have confirm >= recommended
			if riskLevel(schema.Risk) >= riskLevel("high") && confirmLevel(schema.Confirm) < confirmLevel("recommended") {
				t.Errorf("risk=%q but confirm=%q (expected recommended or required)", schema.Risk, schema.Confirm)
			}

			// Rule 3: critical risk commands must have confirm = required
			if schema.Risk == "critical" && schema.Confirm != "required" {
				t.Errorf("risk=critical but confirm=%q (expected required)", schema.Confirm)
			}

			// Rule 4: all enum values valid (via raw annotations)
			validateAnnotationEnums(t, ann)
			validateAnnotationIdempotent(t, ann)

			// Rule 5: cost is non-negative integer
			validateAnnotationCost(t, ann)
		})
	}
}

type enumFieldCheck struct {
	key   string
	valid map[string]bool
}

var annotationEnumChecks = []enumFieldCheck{
	{key: "type", valid: validTypes},
	{key: "volatility", valid: validVolatilities},
	{key: "parallel", valid: validParallels},
	{key: "risk", valid: validRisks},
	{key: "confirm", valid: validConfirms},
	{key: "os_user", valid: validOSUsers},
}

func validateAnnotationEnums(t *testing.T, ann map[string]string) {
	t.Helper()
	for _, check := range annotationEnumChecks {
		if v, ok := ann[check.key]; ok && !check.valid[v] {
			t.Errorf("invalid %s=%q", check.key, v)
		}
	}
}

func validateAnnotationIdempotent(t *testing.T, ann map[string]string) {
	t.Helper()
	if v, ok := ann["idempotent"]; ok {
		if v != "true" && v != "false" {
			t.Errorf("invalid idempotent=%q (must be \"true\" or \"false\")", v)
		}
	}
}

func validateAnnotationCost(t *testing.T, ann map[string]string) {
	t.Helper()
	if v, ok := ann["cost"]; ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			t.Errorf("invalid cost=%q (must be integer)", v)
		} else if n < 0 {
			t.Errorf("invalid cost=%d (must be non-negative)", n)
		}
	}
}

// TestAnnotationsSpecificCommands verifies that key commands have the expected
// semantic values per the story reference table. (AC#2: metadata 值语义正确)
func TestAnnotationsSpecificCommands(t *testing.T) {
	cmds := allCommands(rootCmd)
	cmdMap := make(map[string]*cobra.Command)
	for _, c := range cmds {
		cmdMap[c.CommandPath()] = c
	}

	tests := []struct {
		path       string
		typ        string
		risk       string
		confirm    string
		osUser     string
		idempotent string
	}{
		{"pig postgres status", "query", "safe", "none", "dbsu", "true"},
		{"pig postgres start", "action", "medium", "none", "dbsu", "true"},
		{"pig postgres stop", "action", "high", "recommended", "dbsu", "true"},
		{"pig postgres restart", "action", "high", "recommended", "dbsu", "false"},
		{"pig postgres reload", "action", "low", "none", "dbsu", "true"},
		{"pig postgres promote", "action", "critical", "required", "dbsu", "false"},
		{"pig postgres init", "action", "high", "recommended", "dbsu", "false"},
		{"pig pgbackrest info", "query", "safe", "none", "dbsu", "true"},
		{"pig pgbackrest backup", "action", "low", "none", "dbsu", "true"},
		{"pig pgbackrest restore", "action", "critical", "required", "dbsu", "false"},
		{"pig patroni list", "query", "safe", "none", "dbsu", "true"},
		{"pig patroni failover", "action", "critical", "required", "dbsu", "false"},
		{"pig ext list", "query", "safe", "none", "current", "true"},
		{"pig ext add", "action", "low", "none", "root", "true"},
		{"pig ext rm", "action", "medium", "recommended", "root", "false"},
		{"pig repo add", "action", "low", "none", "root", "true"},
	}

	for _, tt := range tests {
		cmd, ok := cmdMap[tt.path]
		if !ok {
			t.Logf("skipping %q (not found in command tree)", tt.path)
			continue
		}
		t.Run(tt.path, func(t *testing.T) {
			ann := cmd.Annotations
			if ann == nil {
				t.Fatalf("command %q has no Annotations", tt.path)
			}
			if v := ann["type"]; v != tt.typ {
				t.Errorf("type=%q, want %q", v, tt.typ)
			}
			if v := ann["risk"]; v != tt.risk {
				t.Errorf("risk=%q, want %q", v, tt.risk)
			}
			if v := ann["confirm"]; v != tt.confirm {
				t.Errorf("confirm=%q, want %q", v, tt.confirm)
			}
			if v := ann["os_user"]; v != tt.osUser {
				t.Errorf("os_user=%q, want %q", v, tt.osUser)
			}
			if v := ann["idempotent"]; v != tt.idempotent {
				t.Errorf("idempotent=%q, want %q", v, tt.idempotent)
			}
		})
	}
}

// TestAnnotationsDefaultValues verifies that FromAnnotations returns sensible
// defaults when called with nil or empty annotations, ensuring unannotated
// commands won't crash. (AC#4: 未声明 Annotations 的命令有合理默认值)
func TestAnnotationsDefaultValues(t *testing.T) {
	// nil annotations
	schema := ancs.FromAnnotations(nil)
	if schema == nil {
		t.Fatal("FromAnnotations(nil) returned nil")
	}
	if schema.Type != "query" {
		t.Errorf("default Type=%q, want %q", schema.Type, "query")
	}
	if schema.Risk != "safe" {
		t.Errorf("default Risk=%q, want %q", schema.Risk, "safe")
	}
	if schema.Volatility != "stable" {
		t.Errorf("default Volatility=%q, want %q", schema.Volatility, "stable")
	}
	if schema.Parallel != "safe" {
		t.Errorf("default Parallel=%q, want %q", schema.Parallel, "safe")
	}
	if schema.Confirm != "none" {
		t.Errorf("default Confirm=%q, want %q", schema.Confirm, "none")
	}
	if schema.OSUser != "current" {
		t.Errorf("default OSUser=%q, want %q", schema.OSUser, "current")
	}
	if schema.Idempotent != false {
		t.Errorf("default Idempotent=%v, want false", schema.Idempotent)
	}
	if schema.Cost != 0 {
		t.Errorf("default Cost=%d, want 0", schema.Cost)
	}

	// empty annotations
	schema2 := ancs.FromAnnotations(map[string]string{})
	if schema2 == nil {
		t.Fatal("FromAnnotations(empty) returned nil")
	}
	if schema2.Type != "query" {
		t.Errorf("empty annotations default Type=%q, want %q", schema2.Type, "query")
	}
}

// TestCapabilityMapAllNodesHaveSchema verifies that every node in the
// CapabilityMap built from the real command tree has a non-nil Schema.
// (AC#7: CapabilityMap 包含能力元数据)
func TestCapabilityMapAllNodesHaveSchema(t *testing.T) {
	capMap := ancs.BuildCapabilityMap(rootCmd)
	if capMap == nil {
		t.Fatal("BuildCapabilityMap returned nil")
	}
	if capMap.Schema == nil {
		t.Error("CapabilityMap root Schema is nil")
	}

	var walkNodes func(nodes []*ancs.CommandNode)
	walkNodes = func(nodes []*ancs.CommandNode) {
		for _, node := range nodes {
			t.Run(node.FullName, func(t *testing.T) {
				if node.Schema == nil {
					t.Errorf("node %q has nil Schema", node.FullName)
				}
			})
			if len(node.SubCommands) > 0 {
				walkNodes(node.SubCommands)
			}
		}
	}
	walkNodes(capMap.Commands)
}

// TestCapabilityMapSchemaFieldsPopulated verifies that Schema fields in the
// CapabilityMap are populated (non-empty type and risk for leaf nodes).
// (AC#7: CapabilityMap 包含能力元数据)
func TestCapabilityMapSchemaFieldsPopulated(t *testing.T) {
	capMap := ancs.BuildCapabilityMap(rootCmd)
	if capMap == nil {
		t.Fatal("BuildCapabilityMap returned nil")
	}

	var walkLeaves func(nodes []*ancs.CommandNode)
	walkLeaves = func(nodes []*ancs.CommandNode) {
		for _, node := range nodes {
			if len(node.SubCommands) > 0 {
				walkLeaves(node.SubCommands)
			} else {
				// Leaf node — schema fields should be populated
				t.Run(node.FullName, func(t *testing.T) {
					if node.Schema == nil {
						t.Fatalf("leaf node %q has nil Schema", node.FullName)
					}
					if node.Schema.Type == "" {
						t.Errorf("leaf node %q has empty Schema.Type", node.FullName)
					}
					if node.Schema.Risk == "" {
						t.Errorf("leaf node %q has empty Schema.Risk", node.FullName)
					}
					if node.Schema.OSUser == "" {
						t.Errorf("leaf node %q has empty Schema.OSUser", node.FullName)
					}
				})
			}
		}
	}
	walkLeaves(capMap.Commands)
}

// TestCapabilityMapCommandSchemaYAML verifies that the CapabilityMap YAML output
// includes schema fields for commands with Annotations. (AC#7, end-to-end)
func TestCapabilityMapCommandSchemaYAML(t *testing.T) {
	capMap := ancs.BuildCapabilityMap(rootCmd)
	if capMap == nil {
		t.Fatal("BuildCapabilityMap returned nil")
	}

	data, err := capMap.YAML()
	if err != nil {
		t.Fatalf("YAML() error: %v", err)
	}

	output := string(data)
	// Verify schema-related fields appear in YAML output
	for _, field := range []string{"type:", "risk:", "os_user:", "volatility:", "parallel:", "idempotent:", "confirm:", "cost:"} {
		if !strings.Contains(output, field) {
			t.Errorf("CapabilityMap YAML missing schema field %q", field)
		}
	}
}

// TestCapabilityMapCommandSchemaJSON verifies that the CapabilityMap JSON output
// includes schema fields for commands with Annotations. (AC#7, end-to-end)
func TestCapabilityMapCommandSchemaJSON(t *testing.T) {
	capMap := ancs.BuildCapabilityMap(rootCmd)
	if capMap == nil {
		t.Fatal("BuildCapabilityMap returned nil")
	}

	data, err := capMap.JSON()
	if err != nil {
		t.Fatalf("JSON() error: %v", err)
	}

	output := string(data)
	for _, field := range []string{`"type"`, `"risk"`, `"os_user"`, `"volatility"`, `"parallel"`, `"idempotent"`, `"confirm"`, `"cost"`} {
		if !strings.Contains(output, field) {
			t.Errorf("CapabilityMap JSON missing schema field %s", field)
		}
	}
}
