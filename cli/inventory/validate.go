package inventorycli

import (
	"context"
	"fmt"
	"pig/internal/output"
	"pig/inventory"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const InventoryValidateKind = "pig.inventory.validate/v1"

// ValidateOptions controls static and optional external inventory validation.
type ValidateOptions struct {
	Path    string
	Strict  bool
	Ansible bool
	Timeout time.Duration
}

// ValidateData is the stable structured validation payload.
type ValidateData struct {
	Kind        string                `json:"kind" yaml:"kind"`
	Path        string                `json:"path" yaml:"path"`
	Valid       bool                  `json:"valid" yaml:"valid"`
	Strict      bool                  `json:"strict" yaml:"strict"`
	Diagnostics []InventoryDiagnostic `json:"diagnostics,omitempty" yaml:"diagnostics,omitempty"`
}

func (d *ValidateData) Text() string {
	if d == nil {
		return ""
	}
	var lines []string
	for _, diagnostic := range d.Diagnostics {
		location := diagnostic.Path
		if diagnostic.Range.Start.Line > 0 {
			location = fmt.Sprintf("%s:%d:%d", d.Path, diagnostic.Range.Start.Line, diagnostic.Range.Start.Column)
		}
		lines = append(lines, fmt.Sprintf("  [%s] %s %s: %s", strings.ToUpper(string(diagnostic.Severity)), diagnostic.Code, location, diagnostic.Message))
	}
	return strings.Join(lines, "\n")
}

// ValidateInventory parses and validates one static YAML inventory.
func ValidateInventory(opts ValidateOptions) *output.Result {
	if opts.Timeout < 0 {
		return output.Fail(output.CodeInventoryValidateInvalidArgs, "ansible validation timeout must be positive")
	}
	if !opts.Ansible && opts.Timeout > 0 {
		return output.Fail(output.CodeInventoryValidateInvalidArgs, "ansible validation timeout requires --ansible")
	}
	path, source, failure := readStaticInventorySource(opts.Path, output.CodeInventoryValidateInvalidArgs)
	if failure != nil {
		return failure
	}
	doc, err := inventory.Parse(path, source)
	if err != nil {
		diagnostic := parseDiagnostic(err)
		data := &ValidateData{Kind: InventoryValidateKind, Path: path, Strict: opts.Strict, Diagnostics: inventoryDiagnostics([]inventory.Diagnostic{diagnostic})}
		return output.Fail(output.CodeInventoryValidationFailed, "inventory is invalid").WithData(data)
	}
	diagnostics := doc.ValidateComplete()
	if opts.Ansible {
		timeout := opts.Timeout
		if timeout == 0 {
			timeout = 10 * time.Second
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		diagnostics = append(diagnostics, inventory.LookPathAndValidateWithAnsible(ctx, doc)...)
		cancel()
	}
	inventory.SortDiagnostics(diagnostics)
	failed := inventory.HasErrors(diagnostics) || opts.Strict && hasWarnings(diagnostics)
	data := &ValidateData{Kind: InventoryValidateKind, Path: path, Valid: !failed, Strict: opts.Strict, Diagnostics: inventoryDiagnostics(diagnostics)}
	if failed {
		return output.Fail(output.CodeInventoryValidationFailed, "inventory validation failed").WithData(data)
	}
	return output.OK("inventory is valid", data)
}

func hasWarnings(diagnostics []inventory.Diagnostic) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == inventory.SeverityWarning {
			return true
		}
	}
	return false
}

var parseLinePattern = regexp.MustCompile(`(?i)line[ :]+([0-9]+)(?::([0-9]+))?`)

func parseDiagnostic(err error) inventory.Diagnostic {
	position := inventory.Position{}
	if match := parseLinePattern.FindStringSubmatch(fmt.Sprint(err)); len(match) > 1 {
		position.Line, _ = strconv.Atoi(match[1])
		if len(match) > 2 {
			position.Column, _ = strconv.Atoi(match[2])
		}
	}
	return inventory.Diagnostic{
		Code: "YAML_PARSE", Severity: inventory.SeverityError,
		Range:   inventory.SourceRange{Start: position, End: position},
		Message: "inventory YAML could not be parsed",
	}
}

func parseErrorSummary(err error) string {
	diagnostic := parseDiagnostic(err)
	if diagnostic.Range.Start.Line > 0 {
		return fmt.Sprintf("%s at line %d column %d", diagnostic.Message, diagnostic.Range.Start.Line, diagnostic.Range.Start.Column)
	}
	return diagnostic.Message
}
