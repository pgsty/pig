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

// Check represents a precondition or verification item in a plan/result.
type Check struct {
	Name   string `json:"name" yaml:"name"`
	Status string `json:"status,omitempty" yaml:"status,omitempty"`
	Detail string `json:"detail,omitempty" yaml:"detail,omitempty"`
}

// NextAction is a suggested follow-up command for users or agents.
type NextAction struct {
	Command  string `json:"command,omitempty" yaml:"command,omitempty"`
	Reason   string `json:"reason,omitempty" yaml:"reason,omitempty"`
	Required bool   `json:"required,omitempty" yaml:"required,omitempty"`
}

// OperationMeta describes a high-risk primitive operation without changing the Result envelope.
type OperationMeta struct {
	Module       string `json:"module,omitempty" yaml:"module,omitempty"`
	Command      string `json:"command,omitempty" yaml:"command,omitempty"`
	Boundary     string `json:"boundary,omitempty" yaml:"boundary,omitempty"`
	Risk         string `json:"risk,omitempty" yaml:"risk,omitempty"`
	Confirmation string `json:"confirmation,omitempty" yaml:"confirmation,omitempty"`
	Executed     bool   `json:"executed" yaml:"executed"`
	DryRun       bool   `json:"dry_run" yaml:"dry_run"`
}

// Plan represents an execution plan for a dangerous operation.
type Plan struct {
	API           int          `json:"api" yaml:"api"`
	Command       string       `json:"command" yaml:"command"`
	Boundary      string       `json:"boundary,omitempty" yaml:"boundary,omitempty"`
	Confirmation  string       `json:"confirmation,omitempty" yaml:"confirmation,omitempty"`
	Actions       []Action     `json:"actions" yaml:"actions"`
	Affects       []Resource   `json:"affects" yaml:"affects"`
	Expected      string       `json:"expected" yaml:"expected"`
	Risks         []string     `json:"risks,omitempty" yaml:"risks,omitempty"`
	Preconditions []Check      `json:"preconditions,omitempty" yaml:"preconditions,omitempty"`
	Verifications []Check      `json:"verifications,omitempty" yaml:"verifications,omitempty"`
	NextActions   []NextAction `json:"next_actions,omitempty" yaml:"next_actions,omitempty"`
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
	if p.Boundary != "" {
		sb.WriteString("Boundary: ")
		sb.WriteString(p.Boundary)
		sb.WriteString("\n")
	}
	if p.Confirmation != "" {
		sb.WriteString("Confirmation: ")
		sb.WriteString(p.Confirmation)
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

	writeChecks := func(title string, checks []Check) {
		if len(checks) == 0 {
			return
		}
		sb.WriteString("\n")
		sb.WriteString(title)
		sb.WriteString(":\n")
		headers := []string{"Name", "Status", "Detail"}
		rows := make([][]string, 0, len(checks))
		for _, check := range checks {
			rows = append(rows, []string{check.Name, check.Status, check.Detail})
		}
		sb.WriteString(RenderTable(headers, rows))
	}
	writeChecks("Preconditions", p.Preconditions)
	writeChecks("Verifications", p.Verifications)

	if len(p.NextActions) > 0 {
		sb.WriteString("\nNext Actions:\n")
		headers := []string{"Command", "Reason", "Required"}
		rows := make([][]string, 0, len(p.NextActions))
		for _, action := range p.NextActions {
			rows = append(rows, []string{action.Command, action.Reason, fmt.Sprintf("%t", action.Required)})
		}
		sb.WriteString(RenderTable(headers, rows))
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
	if p.API == 0 {
		// Plans are built as literals across cli/*; stamp the envelope schema
		// version at render time so every constructor gets it for free.
		p.API = APIVersion
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
