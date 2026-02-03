// Package ancs provides Agent Native Command Schema (ANCS) metadata support.
// This file defines CommandSchema for structured command metadata representation.
package ancs

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"
)

// CommandSchema represents complete command metadata including ANCS schema,
// arguments, flags, and subcommands for AI agent consumption.
type CommandSchema struct {
	Name        string     `json:"name" yaml:"name"`                                     // Full command path (e.g., "pig ext add")
	Use         string     `json:"use" yaml:"use"`                                       // Usage pattern (e.g., "add <name...>")
	Short       string     `json:"short" yaml:"short"`                                   // Brief description
	Long        string     `json:"long,omitempty" yaml:"long,omitempty"`                 // Detailed description
	Example     string     `json:"example,omitempty" yaml:"example,omitempty"`           // Usage examples
	Schema      *Schema    `json:"schema,omitempty" yaml:"schema,omitempty"`             // ANCS metadata
	Args        []Argument `json:"args,omitempty" yaml:"args,omitempty"`                 // Positional arguments
	Flags       []Flag     `json:"flags,omitempty" yaml:"flags,omitempty"`               // Command flags
	SubCommands []SubCmd   `json:"sub_commands,omitempty" yaml:"sub_commands,omitempty"` // Child commands
}

// Argument represents a positional command argument parsed from cmd.Use.
type Argument struct {
	Name     string `json:"name" yaml:"name"`         // Argument name
	Required bool   `json:"required" yaml:"required"` // true if <arg>, false if [arg]
	Variadic bool   `json:"variadic" yaml:"variadic"` // true if arg... (accepts multiple values)
}

// Flag represents a command flag with its metadata.
type Flag struct {
	Name     string `json:"name" yaml:"name"`                           // Flag name (long form)
	Short    string `json:"short,omitempty" yaml:"short,omitempty"`     // Short form (single char)
	Type     string `json:"type" yaml:"type"`                           // Value type (string, int, bool, etc.)
	Default  string `json:"default,omitempty" yaml:"default,omitempty"` // Default value
	Desc     string `json:"desc" yaml:"desc"`                           // Description
	Required bool   `json:"required" yaml:"required"`                   // Whether flag is required
	Hidden   bool   `json:"hidden,omitempty" yaml:"hidden,omitempty"`   // Whether flag is hidden from help
}

// SubCmd represents a subcommand with minimal info for listing.
// It provides just the name and short description for command tree display.
type SubCmd struct {
	Name  string `json:"name" yaml:"name"`   // Subcommand name
	Short string `json:"short" yaml:"short"` // Brief description
}

// FromCommand creates a CommandSchema from a Cobra command.
// Returns nil if cmd is nil.
func FromCommand(cmd *cobra.Command) *CommandSchema {
	if cmd == nil {
		return nil
	}

	// Extract schema and set name from command path if not provided
	schema := FromAnnotations(cmd.Annotations)
	if schema != nil && schema.Name == "" {
		schema.Name = cmd.CommandPath()
	}

	cs := &CommandSchema{
		Name:    cmd.CommandPath(),
		Use:     cmd.Use,
		Short:   cmd.Short,
		Long:    cmd.Long,
		Example: strings.TrimSpace(cmd.Example),
		Schema:  schema,
		Args:    parseArgsFromUse(cmd.Use),
		Flags:   extractFlags(cmd),
	}
	if len(cs.Args) == 0 {
		cs.Args = inferArgsFromValidator(cmd)
	}

	// Extract subcommands
	for _, sub := range cmd.Commands() {
		// Skip help command
		if sub.Name() == "help" || sub.Hidden {
			continue
		}
		cs.SubCommands = append(cs.SubCommands, SubCmd{
			Name:  sub.Name(),
			Short: sub.Short,
		})
	}

	return cs
}

// parseArgsFromUse extracts arguments from a Cobra Use string.
// Patterns:
//   - <arg>    → required, not variadic
//   - <arg...> → required, variadic
//   - [arg]    → optional, not variadic
//   - [arg...] → optional, variadic
//
// Arguments are returned in their original order as they appear in the Use string.
func parseArgsFromUse(use string) []Argument {
	var args []Argument

	// Remove command name (first word)
	parts := strings.Fields(use)
	if len(parts) <= 1 {
		return args
	}

	// Combined regex to match both <arg> and [arg] patterns, preserving order
	// Group 1: required arg content (from <...>)
	// Group 2: optional arg content (from [...])
	argRe := regexp.MustCompile(`<([^>]+)>|\[([^\]]+)\]`)

	argPart := strings.Join(parts[1:], " ")

	// Find all arguments in order
	for _, match := range argRe.FindAllStringSubmatch(argPart, -1) {
		var name string
		var required bool

		if match[1] != "" {
			// Required argument: <arg>
			name = match[1]
			required = true
		} else if match[2] != "" {
			// Optional argument: [arg]
			name = match[2]
			required = false
		} else {
			continue
		}

		variadic := strings.HasSuffix(name, "...")
		if variadic {
			name = strings.TrimSuffix(name, "...")
		}

		args = append(args, Argument{
			Name:     name,
			Required: required,
			Variadic: variadic,
		})
	}

	return args
}

// inferArgsFromValidator provides a minimal placeholder when a command defines
// argument validation but does not include args in the Use string.
// This avoids returning an empty args list in structured output.
func inferArgsFromValidator(cmd *cobra.Command) []Argument {
	if cmd == nil || cmd.Args == nil || isNoArgsValidator(cmd.Args) {
		return nil
	}
	return []Argument{
		{
			Name:     "args",
			Required: true,
			Variadic: true,
		},
	}
}

func isNoArgsValidator(fn cobra.PositionalArgs) bool {
	if fn == nil {
		return true
	}
	fnPtr := reflect.ValueOf(fn).Pointer()
	noArgsPtr := reflect.ValueOf(cobra.NoArgs).Pointer()
	if fnPtr == noArgsPtr {
		return true
	}
	fnName := runtime.FuncForPC(fnPtr)
	noArgsName := runtime.FuncForPC(noArgsPtr)
	if fnName == nil || noArgsName == nil {
		return false
	}
	return fnName.Name() == noArgsName.Name()
}

// extractFlags extracts flags from a Cobra command.
// Hidden flags are included but marked with Hidden: true.
func extractFlags(cmd *cobra.Command) []Flag {
	var flags []Flag
	seen := make(map[string]struct{})

	collectFlags := func(fs *pflag.FlagSet) {
		if fs == nil {
			return
		}
		fs.VisitAll(func(f *pflag.Flag) {
			// Skip help flag as it's auto-added by Cobra
			if f.Name == "help" {
				return
			}
			if _, ok := seen[f.Name]; ok {
				return
			}
			seen[f.Name] = struct{}{}

			flag := Flag{
				Name:     f.Name,
				Short:    f.Shorthand,
				Type:     f.Value.Type(),
				Default:  f.DefValue,
				Desc:     f.Usage,
				Required: false, // Cobra doesn't have built-in required flag support
				Hidden:   f.Hidden,
			}

			// Check for required annotation
			if ann := f.Annotations; ann != nil {
				if _, ok := ann["required"]; ok {
					flag.Required = true
				}
			}

			flags = append(flags, flag)
		})
	}

	// Non-inherited flags include local + persistent on this command
	collectFlags(cmd.NonInheritedFlags())
	// Inherited flags include persistent flags from parent commands
	collectFlags(cmd.InheritedFlags())

	return flags
}

// YAML serializes the CommandSchema to YAML format.
// Returns an error if the receiver is nil.
func (c *CommandSchema) YAML() ([]byte, error) {
	if c == nil {
		return nil, fmt.Errorf("cannot serialize nil CommandSchema to YAML")
	}
	return yaml.Marshal(c)
}

// JSON serializes the CommandSchema to JSON format.
// Returns an error if the receiver is nil.
func (c *CommandSchema) JSON() ([]byte, error) {
	if c == nil {
		return nil, fmt.Errorf("cannot serialize nil CommandSchema to JSON")
	}
	return json.Marshal(c)
}

// JSONPretty serializes the CommandSchema to pretty-printed JSON format.
// Returns an error if the receiver is nil.
func (c *CommandSchema) JSONPretty() ([]byte, error) {
	if c == nil {
		return nil, fmt.Errorf("cannot serialize nil CommandSchema to JSON")
	}
	return json.MarshalIndent(c, "", "  ")
}
