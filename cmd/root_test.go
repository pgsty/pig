package cmd

import (
	"fmt"
	"io"
	"os"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
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

// TestRunnableCommandsDeclareArgs keeps the command-line contract explicit.
// A nil Args validator makes Cobra silently accept misspelled positional
// arguments, which is especially dangerous for destructive commands.
func TestRunnableCommandsDeclareArgs(t *testing.T) {
	// Keep this allowlist narrow and document why an exception cannot declare
	// cobra.ArbitraryArgs instead. There are currently no such exceptions.
	allowed := map[string]string{}

	var missing []string
	var walk func(*cobra.Command)
	walk = func(parent *cobra.Command) {
		for _, cmd := range parent.Commands() {
			if (cmd.Run != nil || cmd.RunE != nil) && cmd.Args == nil {
				if _, ok := allowed[cmd.CommandPath()]; !ok {
					missing = append(missing, cmd.CommandPath())
				}
			}
			walk(cmd)
		}
	}
	walk(rootCmd)
	sort.Strings(missing)
	if len(missing) != 0 {
		t.Fatalf("runnable commands without explicit Args validators:\n%s", strings.Join(missing, "\n"))
	}
}

func TestRiskyCommandsRejectUndeclaredPositionals(t *testing.T) {
	// These commands intentionally consume a positional operand. Raw restore
	// and initdb operands still have their own "--" placement validation.
	allowed := map[string]string{
		"pig do node-rm":          "node selector(s)",
		"pig do pgsql-rm":         "cluster and instance selector(s)",
		"pig do redis-rm":         "redis selector and port(s)",
		"pig patroni failover":    "optional failover candidate",
		"pig patroni reinit":      "required member name",
		"pig patroni restart":     "optional member name",
		"pig pgbackrest restore":  "raw pgBackRest args after --",
		"pig pitr":                "raw pgBackRest args after --",
		"pig inventory cmdb load": "optional static Inventory file",
		"pig postgres clone":      "source and destination database names",
		"pig postgres fork":       "legacy fork target or action",
		"pig postgres fork init":  "fork name",
		"pig postgres fork rm":    "fork name",
		"pig postgres fork stop":  "fork name",
		"pig sty grafana clean":   "optional local dashboard path",
	}

	var unsafe []string
	var walk func(*cobra.Command)
	walk = func(parent *cobra.Command) {
		for _, cmd := range parent.Commands() {
			risk := cmd.Annotations["risk"]
			if (risk == "high" || risk == "critical") && cmd.Args != nil {
				if err := cmd.Args(cmd, []string{"unexpected"}); err == nil {
					if _, ok := allowed[cmd.CommandPath()]; !ok {
						unsafe = append(unsafe, cmd.CommandPath())
					}
				}
			}
			walk(cmd)
		}
	}
	walk(rootCmd)
	sort.Strings(unsafe)
	if len(unsafe) != 0 {
		t.Fatalf("risky commands accepting undocumented positionals:\n%s", strings.Join(unsafe, "\n"))
	}
}

func TestInventoryEnvironmentDetectionIsScopedToCMDBLoad(t *testing.T) {
	for _, command := range []*cobra.Command{
		inventoryStatusCmd, inventoryListCmd, inventoryShowCmd, inventoryEditCmd,
		inventoryValidateCmd, inventoryCheckCmd, inventoryDiffCmd, inventoryCMDBCheckCmd,
		inventoryCMDBInitCmd, inventoryCMDBDumpCmd, inventoryCMDBEnableCmd, inventoryCMDBDisableCmd,
	} {
		if !commandSkipsEnvironment(command) {
			t.Fatalf("%s unexpectedly enables external environment probes", command.CommandPath())
		}
		if commandInitOptions(command).CreateConfig {
			t.Fatalf("%s unexpectedly creates persistent Pig config", command.CommandPath())
		}
	}
	if commandSkipsEnvironment(inventoryCMDBLoadCmd) {
		t.Fatal("CMDB load skipped OS detection required by OS-default parity")
	}
	if commandSkipsEnvironment(installCmd) {
		t.Fatal("non-Inventory command inherited Inventory initialization policy")
	}
}

func TestStyInitializationIsSideEffectFreeAndProbeScoped(t *testing.T) {
	originalRaw := pigstyConfRaw
	defer func() { pigstyConfRaw = originalRaw }()

	for _, command := range []*cobra.Command{
		styCmd, pigstyInitCmd, pigstyBootCmd, pigstyDeployCmd, pigstyGrafanaCmd,
		pigstyGrafanaListCmd, pigstyListcmd, pigstyGetcmd,
	} {
		options := commandInitOptions(command)
		if options.CreateConfig {
			t.Fatalf("%s unexpectedly creates persistent Pig config", command.CommandPath())
		}
		if !options.SkipEnvironment {
			t.Fatalf("%s unexpectedly enables external environment probes", command.CommandPath())
		}
	}

	pigstyConfRaw = false
	if options := commandInitOptions(pigstyConfCmd); options.CreateConfig || options.SkipEnvironment {
		t.Fatalf("native configure init options = %+v, want no config creation with environment probes", options)
	}
	pigstyConfRaw = true
	if options := commandInitOptions(pigstyConfCmd); options.CreateConfig || !options.SkipEnvironment {
		t.Fatalf("raw configure init options = %+v, want no config creation or duplicate probes", options)
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

func TestReorderOutputBeforeHelp(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "root help output after help",
			args: []string{"--help", "-o", "yaml"},
			want: []string{"-o", "yaml", "--help"},
		},
		{
			name: "subcommand help output after help",
			args: []string{"patroni", "status", "--help", "-o", "json"},
			want: []string{"patroni", "status", "-o", "json", "--help"},
		},
		{
			name: "already ordered",
			args: []string{"-o", "yaml", "--help"},
			want: []string{"-o", "yaml", "--help"},
		},
		{
			name: "no help no change",
			args: []string{"context", "-o", "json"},
			want: []string{"context", "-o", "json"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reorderOutputBeforeHelp(tt.args)
			if len(got) != len(tt.want) {
				t.Fatalf("len(got)=%d, len(want)=%d, got=%v want=%v", len(got), len(tt.want), got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("got[%d]=%q, want %q (got=%v want=%v)", i, got[i], tt.want[i], got, tt.want)
				}
			}
		})
	}
}

func TestIsStructuredOutputRequested(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "json", args: []string{"context", "-o", "json"}, want: true},
		{name: "yaml", args: []string{"--help", "-o", "yaml"}, want: true},
		{name: "json-pretty", args: []string{"--output=json-pretty"}, want: true},
		{name: "text", args: []string{"status", "-o", "text"}, want: false},
		{name: "missing", args: []string{"status"}, want: false},
		{name: "invalid fallback text", args: []string{"status", "--output", "xml"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isStructuredOutputRequested(tt.args)
			if got != tt.want {
				t.Fatalf("isStructuredOutputRequested(%v)=%v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestApplyStructuredOutputSilence(t *testing.T) {
	origFormat := config.OutputFormat
	origSilenceUsage := rootCmd.SilenceUsage
	origSilenceErrors := rootCmd.SilenceErrors
	defer func() {
		config.OutputFormat = origFormat
		rootCmd.SilenceUsage = origSilenceUsage
		rootCmd.SilenceErrors = origSilenceErrors
	}()

	config.OutputFormat = config.OUTPUT_JSON
	applyStructuredOutputSilence(rootCmd)
	if !rootCmd.SilenceUsage || !rootCmd.SilenceErrors {
		t.Fatalf("structured output should enable silence flags, got usage=%v errors=%v",
			rootCmd.SilenceUsage, rootCmd.SilenceErrors)
	}

	config.OutputFormat = config.OUTPUT_TEXT
	applyStructuredOutputSilence(rootCmd)
	if rootCmd.SilenceUsage || rootCmd.SilenceErrors {
		t.Fatalf("text output should disable silence flags, got usage=%v errors=%v",
			rootCmd.SilenceUsage, rootCmd.SilenceErrors)
	}
}

func TestShouldLogExecutionError(t *testing.T) {
	origFormat := config.OutputFormat
	defer func() {
		config.OutputFormat = origFormat
	}()

	config.OutputFormat = config.OUTPUT_TEXT
	if !shouldLogExecutionError(fmt.Errorf("boom")) {
		t.Fatal("text mode should log normal errors")
	}

	config.OutputFormat = config.OUTPUT_JSON
	if !shouldLogExecutionError(fmt.Errorf("boom")) {
		t.Fatal("structured mode should still log non-ExitCodeError failures")
	}

	if shouldLogExecutionError(&utils.ExitCodeError{Code: 1, Err: fmt.Errorf("boom")}) {
		t.Fatal("structured mode should not log ExitCodeError failures")
	}

	config.OutputFormat = config.OUTPUT_TEXT
	if shouldLogExecutionError(&utils.ExitCodeError{Code: 1, Silent: true}) {
		t.Fatal("silent exit code should not be logged")
	}

	if shouldLogExecutionError(nil) {
		t.Fatal("nil error should not be logged")
	}
}

func TestIsUsageExecutionError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "unknown command", err: fmt.Errorf(`unknown command "x" for "pig"`), want: true},
		{name: "unknown long flag", err: fmt.Errorf("unknown flag: --bad"), want: true},
		{name: "unknown shorthand flag", err: fmt.Errorf("unknown shorthand flag: 'Z' in -Z"), want: true},
		{name: "arg count", err: fmt.Errorf("accepts 1 arg(s), received 2"), want: true},
		{name: "invalid argument", err: fmt.Errorf(`invalid argument "bad" for "--output" flag`), want: true},
		{name: "runtime error", err: fmt.Errorf("dial tcp timeout"), want: false},
		{name: "nil", err: nil, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isUsageExecutionError(tt.err); got != tt.want {
				t.Fatalf("isUsageExecutionError(%v)=%v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestEmitStructuredExecutionErrorUsage(t *testing.T) {
	origFormat := config.OutputFormat
	defer func() {
		config.OutputFormat = origFormat
	}()

	tests := []struct {
		name string
		err  error
	}{
		{name: "unknown long flag", err: fmt.Errorf("unknown flag: --bad")},
		{name: "unknown shorthand flag", err: fmt.Errorf("unknown shorthand flag: 'Z' in -Z")},
		{name: "arg count", err: fmt.Errorf("accepts 1 arg(s), received 2")},
		{name: "invalid argument", err: fmt.Errorf(`invalid argument "bad" for "--output" flag`)},
		{name: "unknown command", err: fmt.Errorf(`unknown command "unknown" for "pig"`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := []string{"unknown", "-o", "json"}
			prepareEarlyOutputSettings(args)
			out := captureStdout(t, func() {
				exitCode, handled := emitStructuredExecutionError(tt.err, args)
				if !handled {
					t.Fatal("expected structured execution error to be handled")
				}
				if exitCode != 2 {
					t.Fatalf("exitCode=%d, want 2", exitCode)
				}
			})
			if !strings.Contains(out, `"success":false`) {
				t.Fatalf("expected JSON output, got: %q", out)
			}
			if !strings.Contains(out, `"code":990101`) {
				t.Fatalf("expected system invalid-args code in output, got: %q", out)
			}
		})
	}
}

func TestEmitStructuredExecutionErrorText(t *testing.T) {
	out := captureStdout(t, func() {
		exitCode, handled := emitStructuredExecutionError(
			fmt.Errorf("unknown shorthand flag: 'Z' in -Z"),
			[]string{"-Z"},
		)
		if handled || exitCode != 0 {
			t.Fatalf("text error should not be handled, got handled=%v exitCode=%d", handled, exitCode)
		}
	})
	if out != "" {
		t.Fatalf("text error should not print structured output, got %q", out)
	}
}

func TestEmitStructuredExecutionErrorNonUsage(t *testing.T) {
	origFormat := config.OutputFormat
	defer func() {
		config.OutputFormat = origFormat
	}()

	args := []string{"status", "--output=json"}
	prepareEarlyOutputSettings(args)
	out := captureStdout(t, func() {
		exitCode, handled := emitStructuredExecutionError(fmt.Errorf("runtime failure"), args)
		if !handled {
			t.Fatal("expected structured execution error to be handled")
		}
		wantExit := output.ExitCode(output.CodeSystemCommandFailed)
		if exitCode != wantExit {
			t.Fatalf("exitCode=%d, want %d", exitCode, wantExit)
		}
	})
	if !strings.Contains(out, `"code":990801`) {
		t.Fatalf("expected system command-failed code in output, got: %q", out)
	}
}

func TestEmitStructuredExecutionErrorSkipExitCodeError(t *testing.T) {
	origFormat := config.OutputFormat
	defer func() {
		config.OutputFormat = origFormat
	}()

	args := []string{"status", "-o", "json"}
	prepareEarlyOutputSettings(args)
	exitCode, handled := emitStructuredExecutionError(&utils.ExitCodeError{Code: 2, Err: fmt.Errorf("boom")}, args)
	if handled || exitCode != 0 {
		t.Fatalf("expected ExitCodeError to be skipped, got handled=%v exitCode=%d", handled, exitCode)
	}
}

func TestEmitStructuredExecutionErrorSilentExitCode(t *testing.T) {
	origFormat := config.OutputFormat
	defer func() {
		config.OutputFormat = origFormat
	}()

	args := []string{"status", "-o", "json"}
	prepareEarlyOutputSettings(args)
	out := captureStdout(t, func() {
		exitCode, handled := emitStructuredExecutionError(&utils.ExitCodeError{Code: 1, Silent: true}, args)
		if !handled || exitCode != 1 {
			t.Fatalf("expected silent exit code to be handled as exit 1, got handled=%v exitCode=%d", handled, exitCode)
		}
	})
	if out != "" {
		t.Fatalf("silent exit should not print structured output, got %q", out)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() failed: %v", err)
	}
	defer r.Close()
	os.Stdout = w
	defer func() {
		os.Stdout = origStdout
	}()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("close writer failed: %v", err)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read captured stdout failed: %v", err)
	}
	return string(data)
}
