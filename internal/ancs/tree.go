// Package ancs provides Agent Native Command Schema (ANCS) metadata support.
// This file defines CapabilityMap for complete command tree representation.
package ancs

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"pig/internal/config"
)

// CapabilityMap represents the complete command tree with ANCS metadata.
// It provides a comprehensive view of all commands and their capabilities
// for AI agents to discover and understand the CLI's functionality.
type CapabilityMap struct {
	Name        string         `json:"name" yaml:"name"`               // Root command name (e.g., "pig")
	Version     string         `json:"version" yaml:"version"`         // CLI version (e.g., "v0.9.1")
	Description string         `json:"description" yaml:"description"` // CLI description
	Commands    []*CommandNode `json:"commands" yaml:"commands"`       // Top-level commands
}

// CommandNode represents a node in the command tree.
// It contains command metadata and optionally nested subcommands.
type CommandNode struct {
	Name        string         `json:"name" yaml:"name"`                                     // Command name (e.g., "ext")
	FullName    string         `json:"full_name" yaml:"full_name"`                           // Full command path (e.g., "pig ext")
	Short       string         `json:"short" yaml:"short"`                                   // Brief description
	Schema      *Schema        `json:"schema,omitempty" yaml:"schema,omitempty"`             // ANCS metadata
	SubCommands []*CommandNode `json:"sub_commands,omitempty" yaml:"sub_commands,omitempty"` // Nested commands
}

// BuildCapabilityMap creates a CapabilityMap from a root Cobra command.
// It recursively traverses all commands and extracts ANCS metadata.
// Returns nil if rootCmd is nil.
func BuildCapabilityMap(rootCmd *cobra.Command) *CapabilityMap {
	if rootCmd == nil {
		return nil
	}

	capMap := &CapabilityMap{
		Name:        rootCmd.Name(),
		Version:     getVersion(),
		Description: rootCmd.Short,
	}

	// Build command nodes for each top-level command
	for _, cmd := range rootCmd.Commands() {
		if node := buildCommandNode(cmd); node != nil {
			capMap.Commands = append(capMap.Commands, node)
		}
	}

	return capMap
}

// buildCommandNode recursively builds a CommandNode from a Cobra command.
// Returns nil for nil commands or hidden commands.
func buildCommandNode(cmd *cobra.Command) *CommandNode {
	if cmd == nil || cmd.Hidden {
		return nil
	}

	// Skip built-in Cobra commands
	if cmd.Name() == "help" || cmd.Name() == "completion" {
		return nil
	}

	// Extract schema and set name from command path if not provided
	schema := FromAnnotations(cmd.Annotations)
	if schema != nil && schema.Name == "" {
		schema.Name = cmd.CommandPath()
	}

	node := &CommandNode{
		Name:     cmd.Name(),
		FullName: cmd.CommandPath(),
		Short:    cmd.Short,
		Schema:   schema,
	}

	// Recursively build subcommands
	for _, subCmd := range cmd.Commands() {
		if child := buildCommandNode(subCmd); child != nil {
			node.SubCommands = append(node.SubCommands, child)
		}
	}

	return node
}

// getVersion returns the CLI version string from config.
// Returns "unknown" if PigVersion is empty.
func getVersion() string {
	if config.PigVersion == "" {
		return "unknown"
	}
	return "v" + config.PigVersion
}

// YAML serializes the CapabilityMap to YAML format.
// Returns an error if the receiver is nil.
func (c *CapabilityMap) YAML() ([]byte, error) {
	if c == nil {
		return nil, fmt.Errorf("cannot serialize nil CapabilityMap to YAML")
	}
	return yaml.Marshal(c)
}

// JSON serializes the CapabilityMap to JSON format.
// Returns an error if the receiver is nil.
func (c *CapabilityMap) JSON() ([]byte, error) {
	if c == nil {
		return nil, fmt.Errorf("cannot serialize nil CapabilityMap to JSON")
	}
	return json.Marshal(c)
}

// JSONPretty serializes the CapabilityMap to pretty-printed JSON format.
// Returns an error if the receiver is nil.
func (c *CapabilityMap) JSONPretty() ([]byte, error) {
	if c == nil {
		return nil, fmt.Errorf("cannot serialize nil CapabilityMap to JSON")
	}
	return json.MarshalIndent(c, "", "  ")
}

// YAML serializes the CommandNode to YAML format.
// Returns an error if the receiver is nil.
func (n *CommandNode) YAML() ([]byte, error) {
	if n == nil {
		return nil, fmt.Errorf("cannot serialize nil CommandNode to YAML")
	}
	return yaml.Marshal(n)
}

// JSON serializes the CommandNode to JSON format.
// Returns an error if the receiver is nil.
func (n *CommandNode) JSON() ([]byte, error) {
	if n == nil {
		return nil, fmt.Errorf("cannot serialize nil CommandNode to JSON")
	}
	return json.Marshal(n)
}

// JSONPretty serializes the CommandNode to pretty-printed JSON format.
// Returns an error if the receiver is nil.
func (n *CommandNode) JSONPretty() ([]byte, error) {
	if n == nil {
		return nil, fmt.Errorf("cannot serialize nil CommandNode to JSON")
	}
	return json.MarshalIndent(n, "", "  ")
}
