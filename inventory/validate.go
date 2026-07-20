package inventory

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Severity is a stable diagnostic severity.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

// Diagnostic is safe to return from structured commands. Validation rules
// must not include secret values in Message.
type Diagnostic struct {
	Code     string      `json:"code" yaml:"code"`
	Severity Severity    `json:"severity" yaml:"severity"`
	Path     Path        `json:"path,omitempty" yaml:"path,omitempty"`
	Range    SourceRange `json:"range" yaml:"range"`
	Message  string      `json:"message" yaml:"message"`
}

// RuleSet is a composable validation layer.
type RuleSet interface {
	Name() string
	Validate(*Document) []Diagnostic
}

type ruleSet struct {
	name string
	fn   func(*Document) []Diagnostic
}

func (r ruleSet) Name() string { return r.name }

func (r ruleSet) Validate(doc *Document) []Diagnostic { return r.fn(doc) }

var (
	// YAMLRules validates source-level invariants not already rejected by Parse.
	YAMLRules RuleSet = ruleSet{name: "yaml", fn: validateYAML}
	// AnsibleRules validates the declaration shape of one static YAML inventory.
	AnsibleRules RuleSet = ruleSet{name: "ansible-inventory", fn: validateAnsible}
	// PigstyRules validates only critical, explicitly declared Pigsty identities.
	PigstyRules RuleSet = ruleSet{name: "pigsty", fn: validatePigsty}
	// CompleteInventoryRules validates requirements that apply to a complete
	// inventory file but not to partial documents used by lossless mutations.
	CompleteInventoryRules RuleSet = ruleSet{name: "pigsty-inventory-file", fn: validateCompleteInventory}
)

// Validate runs the supplied rules in order. With no arguments it runs all
// three built-in layers: YAML, Ansible inventory, and critical Pigsty rules.
func (d *Document) Validate(rules ...RuleSet) []Diagnostic {
	if d == nil {
		return []Diagnostic{{Code: "YAML_DOCUMENT_NIL", Severity: SeverityError, Message: "inventory document is nil"}}
	}
	if len(rules) == 0 {
		rules = []RuleSet{YAMLRules, AnsibleRules, PigstyRules}
	}
	var diagnostics []Diagnostic
	for _, ruleset := range rules {
		if ruleset == nil {
			continue
		}
		diagnostics = append(diagnostics, ruleset.Validate(d)...)
	}
	diagnostics = uniqueDiagnostics(diagnostics)
	SortDiagnostics(diagnostics)
	return diagnostics
}

// ValidateComplete validates one complete static Pigsty Inventory using the
// normative rule stack in contract order.
func (d *Document) ValidateComplete() []Diagnostic {
	return d.Validate(YAMLRules, AnsibleRules, PigstyRules, CompleteInventoryRules)
}

func uniqueDiagnostics(diagnostics []Diagnostic) []Diagnostic {
	seen := make(map[string]struct{}, len(diagnostics))
	result := diagnostics[:0]
	for _, diagnostic := range diagnostics {
		key := diagnostic.Code + "\x00" + diagnostic.Path.String()
		if diagnostic.Code != "UNVERIFIABLE" {
			key += "\x00" + diagnostic.Message
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, diagnostic)
	}
	return result
}

// SortDiagnostics orders diagnostics by source position, severity, and code.
func SortDiagnostics(diagnostics []Diagnostic) {
	sort.SliceStable(diagnostics, func(i, j int) bool {
		if diagnostics[i].Range.Start.Offset != diagnostics[j].Range.Start.Offset {
			return diagnostics[i].Range.Start.Offset < diagnostics[j].Range.Start.Offset
		}
		if severityRank(diagnostics[i].Severity) != severityRank(diagnostics[j].Severity) {
			return severityRank(diagnostics[i].Severity) < severityRank(diagnostics[j].Severity)
		}
		return diagnostics[i].Code < diagnostics[j].Code
	})
}

// HasErrors reports whether diagnostics contains an error.
func HasErrors(diagnostics []Diagnostic) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == SeverityError {
			return true
		}
	}
	return false
}

func severityRank(severity Severity) int {
	switch severity {
	case SeverityError:
		return 0
	case SeverityWarning:
		return 1
	default:
		return 2
	}
}

func validateYAML(doc *Document) []Diagnostic {
	if doc.root == nil {
		return []Diagnostic{doc.diagnostic("YAML_ROOT_MISSING", SeverityError, nil, byteSpan{}, "YAML document has no root value")}
	}
	var diagnostics []Diagnostic
	if doc.root.Kind != yaml.MappingNode {
		diagnostics = append(diagnostics, doc.diagnostic("YAML_ROOT_TYPE", SeverityError, nil, doc.nodeSpan(doc.root), "inventory root must be a mapping"))
	}
	var visit func(*yaml.Node, Path)
	visit = func(node *yaml.Node, path Path) {
		if node == nil {
			return
		}
		if node.Kind == yaml.MappingNode {
			for i := 0; i+1 < len(node.Content); i += 2 {
				key := node.Content[i]
				value := node.Content[i+1]
				keyName := key.Value
				childPath := appendPath(path, Key(keyName))
				if key.Kind != yaml.ScalarNode {
					diagnostics = append(diagnostics, doc.diagnostic("YAML_KEY_TYPE", SeverityError, childPath, doc.nodeSpan(key), "mapping keys must be scalars"))
				}
				visit(value, childPath)
			}
			return
		}
		if node.Kind == yaml.SequenceNode {
			for i, child := range node.Content {
				visit(child, appendPath(path, Index(i)))
			}
		}
	}
	visit(doc.root, nil)
	return diagnostics
}

var validGroupName = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func validateAnsible(doc *Document) []Diagnostic {
	if doc.root == nil || doc.root.Kind != yaml.MappingNode {
		return nil
	}
	var diagnostics []Diagnostic
	_, hasAll := mappingNodeValue(doc.root, "all")
	if !hasAll {
		diagnostics = append(diagnostics, doc.diagnostic("INV_ALL_REQUIRED", SeverityError, Keys("all"), doc.nodeSpan(doc.root), "top-level group all is required"))
	} else if len(doc.root.Content) < 2 || doc.root.Content[0].Value != "all" {
		diagnostics = append(diagnostics, doc.diagnostic("INV_ALL_FIRST", SeverityError, nil, doc.nodeSpan(doc.root), "top-level group all must be the first mapping entry"))
	}
	declarations := make(map[string]string)
	var validateGroup func(string, *yaml.Node, *yaml.Node, Path, []string)
	validateGroup = func(name string, keyNode, node *yaml.Node, path Path, ancestors []string) {
		if keyNode != nil && doc.semanticScalar(keyNode, doc.scalarSpan(keyNode)).Kind != StringScalar {
			diagnostics = append(diagnostics, doc.diagnostic("INV_GROUP_NAME_TYPE", SeverityError, path, doc.nodeSpan(keyNode), fmt.Sprintf("group name %q must resolve to a string", name)))
		}
		if name != "all" && !validGroupName.MatchString(name) {
			diagnostics = append(diagnostics, doc.diagnostic("INV_GROUP_NAME", SeverityWarning, path, doc.nodeSpan(node), fmt.Sprintf("group %q contains characters Ansible may transform or reject", name)))
		}
		for _, ancestor := range ancestors {
			if ancestor == name {
				diagnostics = append(diagnostics, doc.diagnostic("INV_GROUP_CYCLE", SeverityError, path, doc.nodeSpan(node), fmt.Sprintf("group cycle detected at %q", name)))
				return
			}
		}
		pathID := pathKey(path)
		if previous, exists := declarations[name]; exists && previous != pathID {
			diagnostics = append(diagnostics, doc.diagnostic("INV_GROUP_REDECLARED", SeverityWarning, path, doc.nodeSpan(node), fmt.Sprintf("group %q is declared more than once", name)))
		} else {
			declarations[name] = pathID
		}
		if isOpaqueYAMLNode(node) {
			diagnostics = append(diagnostics, doc.diagnostic("UNVERIFIABLE", SeverityInfo, path, doc.nodeSpan(node), "plaintext value is unavailable for static Inventory structure validation"))
			return
		}
		if isNullNode(node) {
			if name == "all" {
				diagnostics = append(diagnostics, doc.diagnostic("INV_GROUP_TYPE", SeverityError, path, doc.nodeSpan(node), "group all must be a mapping"))
			}
			return
		}
		if node.Kind != yaml.MappingNode {
			diagnostics = append(diagnostics, doc.diagnostic("INV_GROUP_TYPE", SeverityError, path, doc.nodeSpan(node), fmt.Sprintf("group %q must be a mapping", name)))
			return
		}
		descendants := append(append([]string{}, ancestors...), name)
		hasVars := false
		for i := 0; i+1 < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valueNode := node.Content[i+1]
			childPath := appendPath(path, Key(keyNode.Value))
			switch keyNode.Value {
			case "vars":
				hasVars = true
				if isOpaqueYAMLNode(valueNode) {
					diagnostics = append(diagnostics, doc.diagnostic("UNVERIFIABLE", SeverityInfo, childPath, doc.nodeSpan(valueNode), "plaintext value is unavailable for group vars validation"))
				} else if !isMappingOrNull(valueNode) {
					diagnostics = append(diagnostics, doc.diagnostic("INV_VARS_TYPE", SeverityError, childPath, doc.nodeSpan(valueNode), "group vars must be a mapping"))
				}
			case "hosts":
				if isOpaqueYAMLNode(valueNode) {
					diagnostics = append(diagnostics, doc.diagnostic("UNVERIFIABLE", SeverityInfo, childPath, doc.nodeSpan(valueNode), "plaintext value is unavailable for group hosts validation"))
					continue
				}
				if !isMappingOrNull(valueNode) {
					diagnostics = append(diagnostics, doc.diagnostic("INV_HOSTS_TYPE", SeverityError, childPath, doc.nodeSpan(valueNode), "group hosts must be a mapping"))
					continue
				}
				if valueNode.Kind == yaml.MappingNode {
					for hostIndex := 0; hostIndex+1 < len(valueNode.Content); hostIndex += 2 {
						hostKey := valueNode.Content[hostIndex]
						hostName := hostKey.Value
						hostNode := valueNode.Content[hostIndex+1]
						hostPath := appendPath(childPath, Key(hostName))
						if doc.semanticScalar(hostKey, doc.scalarSpan(hostKey)).Kind != StringScalar {
							diagnostics = append(diagnostics, doc.diagnostic("INV_HOST_NAME_TYPE", SeverityError, hostPath, doc.nodeSpan(hostKey), fmt.Sprintf("host name %q must resolve to a string", hostName)))
						}
						if isOpaqueYAMLNode(hostNode) {
							diagnostics = append(diagnostics, doc.diagnostic("UNVERIFIABLE", SeverityInfo, hostPath, doc.nodeSpan(hostNode), "plaintext value is unavailable for host vars validation"))
						} else if !isMappingOrNull(hostNode) {
							diagnostics = append(diagnostics, doc.diagnostic("INV_HOST_VARS_TYPE", SeverityError, hostPath, doc.nodeSpan(hostNode), fmt.Sprintf("host %q vars must be a mapping", hostName)))
						}
					}
				}
			case "children":
				if isOpaqueYAMLNode(valueNode) {
					diagnostics = append(diagnostics, doc.diagnostic("UNVERIFIABLE", SeverityInfo, childPath, doc.nodeSpan(valueNode), "plaintext value is unavailable for child group validation"))
					continue
				}
				if !isMappingOrNull(valueNode) {
					diagnostics = append(diagnostics, doc.diagnostic("INV_CHILDREN_TYPE", SeverityError, childPath, doc.nodeSpan(valueNode), "group children must be a mapping"))
					continue
				}
				if valueNode.Kind == yaml.MappingNode {
					for childIndex := 0; childIndex+1 < len(valueNode.Content); childIndex += 2 {
						childKey := valueNode.Content[childIndex]
						childName := childKey.Value
						childNode := valueNode.Content[childIndex+1]
						validateGroup(childName, childKey, childNode, appendPath(childPath, Key(childName)), descendants)
					}
				}
			default:
				diagnostics = append(diagnostics, doc.diagnostic("INV_GROUP_KEY", SeverityWarning, childPath, doc.nodeSpan(keyNode), fmt.Sprintf("unsupported group field %q; expected hosts, vars, or children", keyNode.Value)))
			}
		}
		if name == "all" {
			if !hasVars {
				diagnostics = append(diagnostics, doc.diagnostic("INV_VARS_REQUIRED", SeverityWarning, path.ChildKey("vars"), doc.nodeSpan(node), "all.vars is not declared"))
			}
		}
	}
	for i := 0; i+1 < len(doc.root.Content); i += 2 {
		keyNode := doc.root.Content[i]
		name := keyNode.Value
		validateGroup(name, keyNode, doc.root.Content[i+1], Keys(name), nil)
	}
	return diagnostics
}

func validateCompleteInventory(doc *Document) []Diagnostic {
	if doc == nil || doc.root == nil || doc.root.Kind != yaml.MappingNode {
		return nil
	}
	all, exists := mappingNodeValue(doc.root, "all")
	if !exists || all == nil || all.Kind != yaml.MappingNode || isOpaqueYAMLNode(all) {
		return nil
	}
	diagnostics := validatePigstyCompleteness(doc)
	diagnostics = append(diagnostics, validateCompleteNullVars(doc, all, Keys("all"))...)
	diagnostics = append(diagnostics, validateCompleteGroupHosts(doc, all, Keys("all"))...)
	children, exists := mappingNodeValue(all, "children")
	if !exists {
		return append(diagnostics, doc.diagnostic("INV_CHILDREN_REQUIRED", SeverityError, Keys("all", "children"), doc.nodeSpan(all), "all.children is required"))
	}
	if isOpaqueYAMLNode(children) {
		return diagnostics
	}
	if children.Kind != yaml.MappingNode {
		return append(diagnostics, doc.diagnostic("INV_CHILDREN_TYPE", SeverityError, Keys("all", "children"), doc.nodeSpan(children), "all.children must be a mapping"))
	}
	return diagnostics
}

func validateCompleteGroupHosts(doc *Document, group *yaml.Node, path Path) []Diagnostic {
	if group == nil || group.Kind != yaml.MappingNode || isOpaqueYAMLNode(group) {
		return nil
	}
	children, exists := mappingNodeValue(group, "children")
	if !exists || children == nil || children.Kind != yaml.MappingNode || isOpaqueYAMLNode(children) {
		return nil
	}
	var diagnostics []Diagnostic
	for index := 0; index+1 < len(children.Content); index += 2 {
		name := children.Content[index].Value
		child := children.Content[index+1]
		childPath := path.ChildKey("children").ChildKey(name)
		if child == nil || child.Kind != yaml.MappingNode || isOpaqueYAMLNode(child) {
			continue
		}
		hosts, declared := mappingNodeValue(child, "hosts")
		switch {
		case !declared:
			diagnostics = append(diagnostics, doc.diagnostic("PIG_GROUP_HOSTS_REQUIRED", SeverityError, childPath.ChildKey("hosts"), doc.nodeSpan(child), fmt.Sprintf("group %q must declare hosts", name)))
		case isOpaqueYAMLNode(hosts):
		case isNullNode(hosts), hosts.Kind == yaml.MappingNode && len(hosts.Content) == 0:
			diagnostics = append(diagnostics, doc.diagnostic("PIG_GROUP_HOSTS_EMPTY", SeverityError, childPath.ChildKey("hosts"), doc.nodeSpan(hosts), fmt.Sprintf("group %q has no hosts", name)))
		}
		diagnostics = append(diagnostics, validateCompleteGroupHosts(doc, child, childPath)...)
	}
	return diagnostics
}

func validateCompleteNullVars(doc *Document, group *yaml.Node, path Path) []Diagnostic {
	if group == nil || group.Kind != yaml.MappingNode || isOpaqueYAMLNode(group) {
		return nil
	}
	var diagnostics []Diagnostic
	if vars, exists := mappingNodeValue(group, "vars"); exists && !isOpaqueYAMLNode(vars) && isNullNode(vars) {
		diagnostics = append(diagnostics, doc.diagnostic("INV_VARS_TYPE", SeverityError, path.ChildKey("vars"), doc.nodeSpan(vars), "group vars must be a mapping"))
	}
	children, exists := mappingNodeValue(group, "children")
	if !exists || children == nil || children.Kind != yaml.MappingNode || isOpaqueYAMLNode(children) {
		return diagnostics
	}
	for index := 0; index+1 < len(children.Content); index += 2 {
		name := children.Content[index].Value
		diagnostics = append(diagnostics, validateCompleteNullVars(doc, children.Content[index+1], path.ChildKey("children").ChildKey(name))...)
	}
	return diagnostics
}

func mappingNodeValue(mapping *yaml.Node, key string) (*yaml.Node, bool) {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil, false
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1], true
		}
	}
	return nil, false
}

func isOpaqueYAMLNode(node *yaml.Node) bool {
	if node == nil {
		return false
	}
	if node.Kind == yaml.AliasNode {
		return true
	}
	return isCustomYAMLTag(node.Tag)
}

func isCustomYAMLTag(tag string) bool {
	if tag == "" || strings.HasPrefix(tag, "!!") || strings.HasPrefix(tag, "tag:yaml.org,2002:") {
		return false
	}
	return true
}

func isNullNode(node *yaml.Node) bool {
	return node != nil && node.Kind == yaml.ScalarNode && (node.Tag == "!!null" || node.Value == "")
}

func isMappingOrNull(node *yaml.Node) bool {
	return node != nil && (node.Kind == yaml.MappingNode || isNullNode(node))
}

func mappingValues(value *Value) map[string]*Value {
	result := make(map[string]*Value)
	if value == nil || value.Kind != MappingValue || isOpaqueValue(value) {
		return result
	}
	for i := range value.Mapping {
		entry := &value.Mapping[i]
		result[fmt.Sprint(entry.Key.Value)] = &entry.Value
	}
	return result
}

func (d *Document) diagnostic(code string, severity Severity, path Path, span byteSpan, message string) Diagnostic {
	if !span.valid(len(d.source)) {
		span = byteSpan{}
	}
	return Diagnostic{Code: code, Severity: severity, Path: clonePath(path), Range: d.sourceRange(span), Message: message}
}
