package output

import (
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Action represents a single step in a plan.
type Action struct {
	Step        int    `json:"step" yaml:"step"`
	Description string `json:"description" yaml:"description"`
}

// Resource represents a resource affected by a plan.
type Resource struct {
	Type   string `json:"type" yaml:"type"`
	Name   string `json:"name" yaml:"name"`
	Impact string `json:"impact,omitempty" yaml:"impact,omitempty"`
	Detail string `json:"detail,omitempty" yaml:"detail,omitempty"`
}

// Plan represents an execution plan for a dangerous operation.
type Plan struct {
	Command  string     `json:"command" yaml:"command"`
	Actions  []Action   `json:"actions" yaml:"actions"`
	Affects  []Resource `json:"affects" yaml:"affects"`
	Expected string     `json:"expected" yaml:"expected"`
	Risks    []string   `json:"risks,omitempty" yaml:"risks,omitempty"`
}

// Text returns a human-readable text representation of the Plan.
// It lists actions and affected resources for clear preview.
// Returns an empty string if the receiver is nil.
func (p *Plan) Text() string {
	if p == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("Execution Plan\n")

	if p.Command != "" {
		sb.WriteString("Command: ")
		sb.WriteString(p.Command)
		sb.WriteString("\n")
	}

	if len(p.Actions) > 0 {
		sb.WriteString("\nActions:\n")
		for i, action := range p.Actions {
			step := action.Step
			if step <= 0 {
				step = i + 1
			}
			sb.WriteString(fmt.Sprintf("  [%d] %s\n", step, action.Description))
		}
	}

	if len(p.Affects) > 0 {
		sb.WriteString("\nAffects:\n")
		headers := []string{"Type", "Name", "Impact", "Detail"}
		rows := make([][]string, 0, len(p.Affects))
		for _, res := range p.Affects {
			rows = append(rows, []string{res.Type, res.Name, res.Impact, res.Detail})
		}
		sb.WriteString(RenderTable(headers, rows))
	}

	if p.Expected != "" {
		sb.WriteString("\nExpected:\n")
		sb.WriteString("  ")
		sb.WriteString(p.Expected)
		sb.WriteString("\n")
	}

	if len(p.Risks) > 0 {
		sb.WriteString("\nRisks:\n")
		for _, risk := range p.Risks {
			sb.WriteString("  - ")
			sb.WriteString(risk)
			sb.WriteString("\n")
		}
	}

	return strings.TrimRight(sb.String(), "\n")
}

// YAML serializes the Plan to YAML format.
// Returns an error if the receiver is nil.
func (p *Plan) YAML() ([]byte, error) {
	if p == nil {
		return nil, fmt.Errorf("cannot render nil Plan")
	}
	return yaml.Marshal(p)
}

// JSON serializes the Plan to compact JSON format.
// Returns an error if the receiver is nil.
func (p *Plan) JSON() ([]byte, error) {
	if p == nil {
		return nil, fmt.Errorf("cannot render nil Plan")
	}
	return json.Marshal(p)
}

// JSONPretty serializes the Plan to indented JSON format.
// Returns an error if the receiver is nil.
func (p *Plan) JSONPretty() ([]byte, error) {
	if p == nil {
		return nil, fmt.Errorf("cannot render nil Plan")
	}
	return json.MarshalIndent(p, "", "  ")
}

// Render serializes the Plan to the specified format.
// Supported formats: "yaml", "json", "json-pretty", "text", "text-color"
// For "text-color" we currently return plain text for consistency.
func (p *Plan) Render(format string) ([]byte, error) {
	if p == nil {
		return nil, fmt.Errorf("cannot render nil Plan")
	}
	switch format {
	case "yaml":
		return p.YAML()
	case "json":
		return p.JSON()
	case "json-pretty":
		return p.JSONPretty()
	case "text", "text-color":
		return []byte(p.Text()), nil
	default:
		return nil, fmt.Errorf("unknown output format: %s", format)
	}
}
