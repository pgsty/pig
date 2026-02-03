package ancs

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// =============================================================================
// Task 1: CommandSchema Structure Tests
// =============================================================================

func TestCommandSchemaStructure(t *testing.T) {
	// Test that CommandSchema can be created with all fields
	cs := &CommandSchema{
		Name:    "pig ext add",
		Use:     "add <name...>",
		Short:   "Install PostgreSQL extensions",
		Long:    "Install one or more PostgreSQL extensions using the system package manager",
		Example: "  pig ext add postgis          # Install postgis extension",
		Schema: &Schema{
			Name:       "pig ext add",
			Type:       TYPE_ACTION,
			Volatility: VOLATILITY_STABLE,
			Parallel:   PARALLEL_SAFE,
			Idempotent: true,
			Risk:       RISK_LOW,
			Confirm:    CONFIRM_NONE,
			OSUser:     OS_USER_ROOT,
			Cost:       5000,
		},
		Args: []Argument{
			{Name: "name", Required: true, Variadic: true},
		},
		Flags: []Flag{
			{Name: "version", Short: "v", Type: "int", Default: "", Desc: "specify PostgreSQL major version", Required: false},
			{Name: "yes", Short: "y", Type: "bool", Default: "false", Desc: "skip confirmation", Required: false},
		},
		SubCommands: []SubCmd{},
	}

	if cs.Name != "pig ext add" {
		t.Errorf("expected Name 'pig ext add', got %q", cs.Name)
	}
	if len(cs.Args) != 1 {
		t.Errorf("expected 1 argument, got %d", len(cs.Args))
	}
	if len(cs.Flags) != 2 {
		t.Errorf("expected 2 flags, got %d", len(cs.Flags))
	}
}

func TestArgumentStructure(t *testing.T) {
	tests := []struct {
		name     string
		arg      Argument
		required bool
		variadic bool
	}{
		{"required variadic", Argument{Name: "name", Required: true, Variadic: true}, true, true},
		{"required single", Argument{Name: "name", Required: true, Variadic: false}, true, false},
		{"optional single", Argument{Name: "query", Required: false, Variadic: false}, false, false},
		{"optional variadic", Argument{Name: "args", Required: false, Variadic: true}, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.arg.Required != tt.required {
				t.Errorf("expected Required=%v, got %v", tt.required, tt.arg.Required)
			}
			if tt.arg.Variadic != tt.variadic {
				t.Errorf("expected Variadic=%v, got %v", tt.variadic, tt.arg.Variadic)
			}
		})
	}
}

func TestFlagStructure(t *testing.T) {
	flag := Flag{
		Name:     "version",
		Short:    "v",
		Type:     "int",
		Default:  "17",
		Desc:     "PostgreSQL major version",
		Required: false,
	}

	if flag.Name != "version" {
		t.Errorf("expected Name 'version', got %q", flag.Name)
	}
	if flag.Short != "v" {
		t.Errorf("expected Short 'v', got %q", flag.Short)
	}
	if flag.Type != "int" {
		t.Errorf("expected Type 'int', got %q", flag.Type)
	}
}

func TestSubCmdStructure(t *testing.T) {
	subcmd := SubCmd{
		Name:  "add",
		Short: "Install extensions",
	}

	if subcmd.Name != "add" {
		t.Errorf("expected Name 'add', got %q", subcmd.Name)
	}
	if subcmd.Short != "Install extensions" {
		t.Errorf("expected Short 'Install extensions', got %q", subcmd.Short)
	}
}

// =============================================================================
// Task 1.5: JSON/YAML Tag Tests
// =============================================================================

func TestCommandSchemaJSONTags(t *testing.T) {
	cs := &CommandSchema{
		Name:        "test",
		Use:         "test",
		Short:       "Test command",
		SubCommands: []SubCmd{{Name: "sub", Short: "Sub command"}},
	}

	data, err := json.Marshal(cs)
	if err != nil {
		t.Fatalf("failed to marshal CommandSchema to JSON: %v", err)
	}

	jsonStr := string(data)
	// Check snake_case fields
	if !strings.Contains(jsonStr, `"sub_commands"`) {
		t.Errorf("expected 'sub_commands' in JSON output, got: %s", jsonStr)
	}
}

func TestCommandSchemaYAMLTags(t *testing.T) {
	cs := &CommandSchema{
		Name:        "test",
		Use:         "test",
		Short:       "Test command",
		SubCommands: []SubCmd{{Name: "sub", Short: "Sub command"}},
	}

	data, err := yaml.Marshal(cs)
	if err != nil {
		t.Fatalf("failed to marshal CommandSchema to YAML: %v", err)
	}

	yamlStr := string(data)
	// Check snake_case fields
	if !strings.Contains(yamlStr, "sub_commands:") {
		t.Errorf("expected 'sub_commands:' in YAML output, got: %s", yamlStr)
	}
}

// =============================================================================
// Task 2: FromCommand Tests
// =============================================================================

func TestFromCommandNil(t *testing.T) {
	cs := FromCommand(nil)
	if cs != nil {
		t.Errorf("expected nil for nil cmd, got %+v", cs)
	}
}

func TestFromCommandBasic(t *testing.T) {
	cmd := &cobra.Command{
		Use:     "add <name...>",
		Short:   "Install extensions",
		Long:    "Install one or more PostgreSQL extensions",
		Example: "  pig ext add postgis",
		Annotations: map[string]string{
			"name":       "pig ext add",
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
	cmd.Flags().IntP("version", "v", 0, "PostgreSQL major version")
	cmd.Flags().BoolP("yes", "y", false, "skip confirmation")

	cs := FromCommand(cmd)
	if cs == nil {
		t.Fatal("expected non-nil CommandSchema")
	}

	if cs.Use != "add <name...>" {
		t.Errorf("expected Use 'add <name...>', got %q", cs.Use)
	}
	if cs.Short != "Install extensions" {
		t.Errorf("expected Short 'Install extensions', got %q", cs.Short)
	}
	if cs.Schema == nil {
		t.Fatal("expected non-nil Schema")
	}
	if cs.Schema.Type != TYPE_ACTION {
		t.Errorf("expected Schema.Type 'action', got %q", cs.Schema.Type)
	}
}

func TestFromCommandNoAnnotations(t *testing.T) {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show status",
	}

	cs := FromCommand(cmd)
	if cs == nil {
		t.Fatal("expected non-nil CommandSchema")
	}
	if cs.Schema == nil {
		t.Fatal("expected non-nil Schema with defaults")
	}
	// Should have default values
	if cs.Schema.Type != TYPE_QUERY {
		t.Errorf("expected default Type 'query', got %q", cs.Schema.Type)
	}
}

func TestFromCommandWithSubCommands(t *testing.T) {
	parent := &cobra.Command{
		Use:   "ext",
		Short: "Extension management",
	}
	child1 := &cobra.Command{
		Use:   "add",
		Short: "Install extensions",
	}
	child2 := &cobra.Command{
		Use:   "rm",
		Short: "Remove extensions",
	}
	hidden := &cobra.Command{
		Use:    "hidden",
		Short:  "Hidden command",
		Hidden: true,
	}
	parent.AddCommand(child1, child2, hidden)

	cs := FromCommand(parent)
	if cs == nil {
		t.Fatal("expected non-nil CommandSchema")
	}
	if len(cs.SubCommands) != 2 {
		t.Errorf("expected 2 sub commands, got %d", len(cs.SubCommands))
	}
}

func TestFromCommand_InheritedFlags(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	root.PersistentFlags().String("output", "text", "output format")

	child := &cobra.Command{Use: "child"}
	root.AddCommand(child)

	cs := FromCommand(child)
	if cs == nil {
		t.Fatal("expected non-nil CommandSchema")
	}

	found := false
	for _, f := range cs.Flags {
		if f.Name == "output" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected inherited persistent flag 'output' to be included")
	}
}

func TestFromCommand_ArgsFallbackWhenUseMissing(t *testing.T) {
	cmd := &cobra.Command{
		Use:  "test",
		Args: cobra.ExactArgs(1),
	}

	cs := FromCommand(cmd)
	if cs == nil {
		t.Fatal("expected non-nil CommandSchema")
	}
	if len(cs.Args) != 1 {
		t.Fatalf("expected 1 inferred arg, got %d", len(cs.Args))
	}
	if cs.Args[0].Name != "args" || !cs.Args[0].Required || !cs.Args[0].Variadic {
		t.Errorf("unexpected inferred arg: %+v", cs.Args[0])
	}
}

// =============================================================================
// Task 2.4: Argument Parsing Tests
// =============================================================================

func TestParseArgsFromUse(t *testing.T) {
	tests := []struct {
		use      string
		expected []Argument
	}{
		{"add <name...>", []Argument{{Name: "name", Required: true, Variadic: true}}},
		{"info <name>", []Argument{{Name: "name", Required: true, Variadic: false}}},
		{"list [query]", []Argument{{Name: "query", Required: false, Variadic: false}}},
		{"status", []Argument{}},
		{"get [name...]", []Argument{{Name: "name", Required: false, Variadic: true}}},
		{"cmd <arg1> <arg2>", []Argument{
			{Name: "arg1", Required: true, Variadic: false},
			{Name: "arg2", Required: true, Variadic: false},
		}},
		{"cmd <arg1> [arg2]", []Argument{
			{Name: "arg1", Required: true, Variadic: false},
			{Name: "arg2", Required: false, Variadic: false},
		}},
		// Mixed order tests - verify original order is preserved
		{"cmd <arg1> [arg2] <arg3>", []Argument{
			{Name: "arg1", Required: true, Variadic: false},
			{Name: "arg2", Required: false, Variadic: false},
			{Name: "arg3", Required: true, Variadic: false},
		}},
		{"cmd [opt1] <req1>", []Argument{
			{Name: "opt1", Required: false, Variadic: false},
			{Name: "req1", Required: true, Variadic: false},
		}},
		{"cmd [opt1] <req1> [opt2] <req2>", []Argument{
			{Name: "opt1", Required: false, Variadic: false},
			{Name: "req1", Required: true, Variadic: false},
			{Name: "opt2", Required: false, Variadic: false},
			{Name: "req2", Required: true, Variadic: false},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.use, func(t *testing.T) {
			args := parseArgsFromUse(tt.use)
			if len(args) != len(tt.expected) {
				t.Errorf("expected %d args, got %d for use=%q", len(tt.expected), len(args), tt.use)
				return
			}
			for i, arg := range args {
				if arg.Name != tt.expected[i].Name {
					t.Errorf("arg[%d]: expected Name=%q, got %q", i, tt.expected[i].Name, arg.Name)
				}
				if arg.Required != tt.expected[i].Required {
					t.Errorf("arg[%d]: expected Required=%v, got %v", i, tt.expected[i].Required, arg.Required)
				}
				if arg.Variadic != tt.expected[i].Variadic {
					t.Errorf("arg[%d]: expected Variadic=%v, got %v", i, tt.expected[i].Variadic, arg.Variadic)
				}
			}
		})
	}
}

// =============================================================================
// Task 3: Serialization Tests
// =============================================================================

func TestCommandSchemaYAMLMethod(t *testing.T) {
	cs := &CommandSchema{
		Name:  "test",
		Use:   "test",
		Short: "Test command",
	}

	data, err := cs.YAML()
	if err != nil {
		t.Fatalf("YAML() returned error: %v", err)
	}
	if len(data) == 0 {
		t.Error("YAML() returned empty data")
	}
	if !strings.Contains(string(data), "name: test") {
		t.Errorf("expected 'name: test' in YAML output, got: %s", string(data))
	}
}

func TestCommandSchemaJSONMethod(t *testing.T) {
	cs := &CommandSchema{
		Name:  "test",
		Use:   "test",
		Short: "Test command",
	}

	data, err := cs.JSON()
	if err != nil {
		t.Fatalf("JSON() returned error: %v", err)
	}
	if len(data) == 0 {
		t.Error("JSON() returned empty data")
	}
	if !strings.Contains(string(data), `"name":"test"`) {
		t.Errorf("expected '\"name\":\"test\"' in JSON output, got: %s", string(data))
	}
}

func TestCommandSchemaJSONPrettyMethod(t *testing.T) {
	cs := &CommandSchema{
		Name:  "test",
		Use:   "test",
		Short: "Test command",
	}

	data, err := cs.JSONPretty()
	if err != nil {
		t.Fatalf("JSONPretty() returned error: %v", err)
	}
	if len(data) == 0 {
		t.Error("JSONPretty() returned empty data")
	}
	// Pretty JSON should have newlines
	if !strings.Contains(string(data), "\n") {
		t.Error("JSONPretty() should produce formatted output with newlines")
	}
}

// =============================================================================
// Task 3.4: Nil Receiver Tests
// =============================================================================

func TestCommandSchemaNilReceiverYAML(t *testing.T) {
	var cs *CommandSchema = nil
	data, err := cs.YAML()
	if err == nil {
		t.Error("expected error for nil receiver YAML()")
	}
	if data != nil {
		t.Errorf("expected nil data for nil receiver, got %v", data)
	}
}

func TestCommandSchemaNilReceiverJSON(t *testing.T) {
	var cs *CommandSchema = nil
	data, err := cs.JSON()
	if err == nil {
		t.Error("expected error for nil receiver JSON()")
	}
	if data != nil {
		t.Errorf("expected nil data for nil receiver, got %v", data)
	}
}

func TestCommandSchemaNilReceiverJSONPretty(t *testing.T) {
	var cs *CommandSchema = nil
	data, err := cs.JSONPretty()
	if err == nil {
		t.Error("expected error for nil receiver JSONPretty()")
	}
	if data != nil {
		t.Errorf("expected nil data for nil receiver, got %v", data)
	}
}

// =============================================================================
// Task 6: Flag Extraction Tests
// =============================================================================

func TestFromCommandFlagExtraction(t *testing.T) {
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
	}
	cmd.Flags().StringP("name", "n", "default", "Name flag")
	cmd.Flags().IntP("count", "c", 10, "Count flag")
	cmd.Flags().BoolP("verbose", "v", false, "Verbose flag")

	cs := FromCommand(cmd)
	if cs == nil {
		t.Fatal("expected non-nil CommandSchema")
	}

	// Should have 3 flags (excluding help which is auto-added)
	foundFlags := make(map[string]bool)
	for _, f := range cs.Flags {
		foundFlags[f.Name] = true
	}

	if !foundFlags["name"] {
		t.Error("expected 'name' flag to be extracted")
	}
	if !foundFlags["count"] {
		t.Error("expected 'count' flag to be extracted")
	}
	if !foundFlags["verbose"] {
		t.Error("expected 'verbose' flag to be extracted")
	}
}

func TestFromCommandExampleTrimmed(t *testing.T) {
	cmd := &cobra.Command{
		Use:     "test",
		Short:   "Test command",
		Example: "  \n  pig test example\n  pig test another\n  ",
	}

	cs := FromCommand(cmd)
	if cs == nil {
		t.Fatal("expected non-nil CommandSchema")
	}

	// Example should be trimmed of leading/trailing whitespace
	if cs.Example != "pig test example\n  pig test another" {
		t.Errorf("expected trimmed Example, got %q", cs.Example)
	}
	// Should not start or end with whitespace
	if len(cs.Example) > 0 && (cs.Example[0] == ' ' || cs.Example[0] == '\n') {
		t.Error("Example should not start with whitespace")
	}
}

func TestFromCommandHiddenFlagExtraction(t *testing.T) {
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
	}
	cmd.Flags().StringP("visible", "v", "", "Visible flag")
	cmd.Flags().StringP("hidden", "h", "", "Hidden flag")
	cmd.Flags().MarkHidden("hidden")

	cs := FromCommand(cmd)
	if cs == nil {
		t.Fatal("expected non-nil CommandSchema")
	}

	var visibleFlag, hiddenFlag *Flag
	for i := range cs.Flags {
		if cs.Flags[i].Name == "visible" {
			visibleFlag = &cs.Flags[i]
		}
		if cs.Flags[i].Name == "hidden" {
			hiddenFlag = &cs.Flags[i]
		}
	}

	if visibleFlag == nil {
		t.Fatal("expected 'visible' flag to be extracted")
	}
	if visibleFlag.Hidden {
		t.Error("expected 'visible' flag to have Hidden=false")
	}

	if hiddenFlag == nil {
		t.Fatal("expected 'hidden' flag to be extracted")
	}
	if !hiddenFlag.Hidden {
		t.Error("expected 'hidden' flag to have Hidden=true")
	}
}
