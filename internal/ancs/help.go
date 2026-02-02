// Package ancs provides Agent Native Command Schema (ANCS) metadata support.
// This file implements the HelpFunc for structured help output.
package ancs

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"pig/internal/config"
)

// originalHelpFunc stores the original help function for fallback
var originalHelpFunc func(*cobra.Command, []string)

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
	originalHelpFunc = rootCmd.HelpFunc()
	rootCmd.SetHelpFunc(HelpFunc)
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
			callOriginalHelp(cmd, args)
		}
	case config.OUTPUT_JSON, config.OUTPUT_JSON_PRETTY:
		if err := RenderHelp(cmd, format); err != nil {
			fmt.Fprintf(os.Stderr, "Error rendering help: %v\n", err)
			// Fall back to original help on error
			callOriginalHelp(cmd, args)
		}
	default:
		// For text format, use original help
		callOriginalHelp(cmd, args)
	}
}

// getOutputFormat retrieves the output format from the command's flags.
// It traverses up to the root command to find the flag.
func getOutputFormat(cmd *cobra.Command) string {
	// First check if config.OutputFormat is already set
	if config.OutputFormat != "" && config.OutputFormat != config.OUTPUT_TEXT {
		return config.OutputFormat
	}

	// Otherwise, get directly from flag (help runs before PersistentPreRunE)
	root := cmd.Root()
	if root == nil {
		return config.OUTPUT_TEXT
	}

	flag := root.PersistentFlags().Lookup("output")
	if flag == nil {
		return config.OUTPUT_TEXT
	}

	format := strings.ToLower(strings.TrimSpace(flag.Value.String()))
	for _, valid := range config.ValidOutputFormats {
		if format == valid {
			return format
		}
	}

	return config.OUTPUT_TEXT
}

// callOriginalHelp invokes the original help function and appends the agent hint.
// If no original function was saved, it uses the default cobra help.
func callOriginalHelp(cmd *cobra.Command, args []string) {
	if originalHelpFunc != nil {
		originalHelpFunc(cmd, args)
	} else {
		// Fallback to cobra's built-in help behavior
		// Temporarily set HelpFunc to nil to avoid recursion
		// Use defer to ensure restoration even if Help() panics
		cmd.SetHelpFunc(nil)
		defer cmd.SetHelpFunc(HelpFunc)
		cmd.Help()
	}
	// Append agent hint after help output (only called in text mode)
	printAgentHint()
}

// printAgentHint outputs the agent hint message with proper visual separation.
// This is only called in text mode to inform users about machine-readable options.
func printAgentHint() {
	fmt.Println() // Blank line for visual separation
	fmt.Println(AgentHintText)
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

	fmt.Println(string(data))
	return nil
}
