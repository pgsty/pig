// Package ancs provides Agent Native Command Schema (ANCS) metadata support.
// This file implements the HelpFunc for structured help output.
package ancs

import (
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"

	"github.com/spf13/cobra"

	"pig/internal/config"
)

// originalHelpFuncs stores the original help function for each command
var originalHelpFuncs = map[*cobra.Command]func(*cobra.Command, []string){}

// AgentHintText is the hint message shown at the bottom of text-mode help output.
// It informs human users about machine-readable output options.
const AgentHintText = "For agent/machine consumption: -o json | -o yaml"

// SetupHelp configures the command tree to use ANCS-aware help output.
// It saves the original help function and sets the custom one.
// This should be called after the root command is fully configured.
func SetupHelp(rootCmd *cobra.Command) {
	if rootCmd == nil {
		return
	}
	// First pass: capture original help funcs before any mutation.
	walkCommands(rootCmd, func(cmd *cobra.Command) {
		if _, ok := originalHelpFuncs[cmd]; !ok {
			originalHelpFuncs[cmd] = cmd.HelpFunc()
		}
	})
	// Second pass: set custom help func for all commands.
	walkCommands(rootCmd, func(cmd *cobra.Command) {
		cmd.SetHelpFunc(HelpFunc)
	})
}

// HelpFunc is the ANCS-aware help function that outputs structured data
// when format is yaml/json, otherwise falls back to the original help.
func HelpFunc(cmd *cobra.Command, args []string) {
	// Get format from flag directly since help runs before PersistentPreRunE
	format := getOutputFormat(cmd)

	switch format {
	case config.OUTPUT_YAML:
		if err := RenderHelp(cmd, format); err != nil {
			fmt.Fprintf(os.Stderr, "Error rendering help: %v\n", err)
			// Fall back to original help on error
			callOriginalHelp(cmd, args, false)
		}
	case config.OUTPUT_JSON, config.OUTPUT_JSON_PRETTY:
		if err := RenderHelp(cmd, format); err != nil {
			fmt.Fprintf(os.Stderr, "Error rendering help: %v\n", err)
			// Fall back to original help on error
			callOriginalHelp(cmd, args, false)
		}
	default:
		// For text format, use original help
		callOriginalHelp(cmd, args, true)
	}
}

// getOutputFormat retrieves the output format from the command's flags.
// It traverses up to the root command to find the flag.
func getOutputFormat(cmd *cobra.Command) string {
	// Otherwise, get directly from flag (help runs before PersistentPreRunE)
	root := cmd.Root()
	if root == nil {
		if valid := validateOutputFormat(config.OutputFormat); valid != "" {
			return valid
		}
		return config.OUTPUT_TEXT
	}

	flag := root.PersistentFlags().Lookup("output")
	if flag == nil {
		if valid := validateOutputFormat(config.OutputFormat); valid != "" {
			return valid
		}
		return config.OUTPUT_TEXT
	}

	format := strings.ToLower(strings.TrimSpace(flag.Value.String()))
	if valid := validateOutputFormat(format); valid != "" {
		return valid
	}

	if valid := validateOutputFormat(config.OutputFormat); valid != "" {
		return valid
	}
	return config.OUTPUT_TEXT
}

// callOriginalHelp invokes the original help function and appends the agent hint.
// If no original function was saved, it uses the default cobra help.
func callOriginalHelp(cmd *cobra.Command, args []string, withHint bool) {
	if originalHelpFunc, ok := originalHelpFuncs[cmd]; ok && originalHelpFunc != nil {
		// Guard against recursive help func (e.g., if originalHelpFunc == HelpFunc)
		if sameHelpFunc(originalHelpFunc, HelpFunc) {
			defaultHelp := (&cobra.Command{}).HelpFunc()
			defaultHelp(cmd, args)
		} else {
			originalHelpFunc(cmd, args)
		}
	} else {
		// Fallback to cobra's default help behavior (avoid recursion)
		defaultHelp := (&cobra.Command{}).HelpFunc()
		defaultHelp(cmd, args)
	}
	// Append agent hint after help output (text mode only)
	if withHint {
		printAgentHint(cmd.OutOrStdout())
	}
}

func sameHelpFunc(a, b func(*cobra.Command, []string)) bool {
	if a == nil || b == nil {
		return false
	}
	return reflect.ValueOf(a).Pointer() == reflect.ValueOf(b).Pointer()
}

// printAgentHint outputs the agent hint message with proper visual separation.
// This is only called in text mode to inform users about machine-readable options.
func printAgentHint(w io.Writer) {
	if w == nil {
		w = os.Stdout
	}
	fmt.Fprintln(w) // Blank line for visual separation
	fmt.Fprintln(w, AgentHintText)
}

func validateOutputFormat(format string) string {
	normalized := strings.ToLower(strings.TrimSpace(format))
	if normalized == "" {
		return ""
	}
	for _, valid := range config.ValidOutputFormats {
		if normalized == valid {
			return normalized
		}
	}
	return ""
}

func walkCommands(root *cobra.Command, fn func(*cobra.Command)) {
	if root == nil || fn == nil {
		return
	}
	fn(root)
	for _, cmd := range root.Commands() {
		walkCommands(cmd, fn)
	}
}

// RenderHelp outputs the command schema in the specified format.
// For the root command, it outputs the complete CapabilityMap.
// For subcommands, it outputs the CommandSchema.
// Supported formats: yaml, json, json-pretty
func RenderHelp(cmd *cobra.Command, format string) error {
	if cmd == nil {
		return fmt.Errorf("cannot render help for nil command")
	}

	var data []byte
	var err error

	// Check if this is the root command (no parent)
	if cmd.Parent() == nil {
		// Root command: output complete capability map
		capMap := BuildCapabilityMap(cmd)
		if capMap == nil {
			return fmt.Errorf("failed to build capability map")
		}

		switch format {
		case config.OUTPUT_YAML:
			data, err = capMap.YAML()
		case config.OUTPUT_JSON:
			data, err = capMap.JSON()
		case config.OUTPUT_JSON_PRETTY:
			data, err = capMap.JSONPretty()
		default:
			return fmt.Errorf("unsupported format: %s", format)
		}
	} else {
		// Subcommand: output command schema
		schema := FromCommand(cmd)
		if schema == nil {
			return fmt.Errorf("failed to extract command schema")
		}

		switch format {
		case config.OUTPUT_YAML:
			data, err = schema.YAML()
		case config.OUTPUT_JSON:
			data, err = schema.JSON()
		case config.OUTPUT_JSON_PRETTY:
			data, err = schema.JSONPretty()
		default:
			return fmt.Errorf("unsupported format: %s", format)
		}
	}

	if err != nil {
		return fmt.Errorf("failed to serialize: %w", err)
	}

	_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
	return nil
}
