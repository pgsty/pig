package inventory

import (
	"fmt"
	"net/netip"
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
	SortDiagnostics(diagnostics)
	return diagnostics
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

var validGroupName = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

func validateAnsible(doc *Document) []Diagnostic {
	if doc.root == nil || doc.root.Kind != yaml.MappingNode {
		return nil
	}
	var diagnostics []Diagnostic
	if len(doc.root.Content) < 2 || doc.root.Content[0].Value != "all" {
		diagnostics = append(diagnostics, doc.diagnostic("INV_ALL_FIRST", SeverityWarning, nil, doc.nodeSpan(doc.root), "top-level group all should be the first mapping entry"))
	}
	declarations := make(map[string]string)
	var validateGroup func(string, *yaml.Node, Path, []string)
	validateGroup = func(name string, node *yaml.Node, path Path, ancestors []string) {
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
		if isNullNode(node) {
			return
		}
		if node.Kind != yaml.MappingNode {
			diagnostics = append(diagnostics, doc.diagnostic("INV_GROUP_TYPE", SeverityError, path, doc.nodeSpan(node), fmt.Sprintf("group %q must be a mapping", name)))
			return
		}
		descendants := append(append([]string{}, ancestors...), name)
		for i := 0; i+1 < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valueNode := node.Content[i+1]
			childPath := appendPath(path, Key(keyNode.Value))
			switch keyNode.Value {
			case "vars":
				if !isMappingOrNull(valueNode) {
					diagnostics = append(diagnostics, doc.diagnostic("INV_VARS_TYPE", SeverityError, childPath, doc.nodeSpan(valueNode), "group vars must be a mapping"))
				}
			case "hosts":
				if !isMappingOrNull(valueNode) {
					diagnostics = append(diagnostics, doc.diagnostic("INV_HOSTS_TYPE", SeverityError, childPath, doc.nodeSpan(valueNode), "group hosts must be a mapping"))
					continue
				}
				if valueNode.Kind == yaml.MappingNode {
					for hostIndex := 0; hostIndex+1 < len(valueNode.Content); hostIndex += 2 {
						hostName := valueNode.Content[hostIndex].Value
						hostNode := valueNode.Content[hostIndex+1]
						hostPath := appendPath(childPath, Key(hostName))
						if !isMappingOrNull(hostNode) {
							diagnostics = append(diagnostics, doc.diagnostic("INV_HOST_VARS_TYPE", SeverityError, hostPath, doc.nodeSpan(hostNode), fmt.Sprintf("host %q vars must be a mapping", hostName)))
						}
					}
				}
			case "children":
				if !isMappingOrNull(valueNode) {
					diagnostics = append(diagnostics, doc.diagnostic("INV_CHILDREN_TYPE", SeverityError, childPath, doc.nodeSpan(valueNode), "group children must be a mapping"))
					continue
				}
				if valueNode.Kind == yaml.MappingNode {
					for childIndex := 0; childIndex+1 < len(valueNode.Content); childIndex += 2 {
						childName := valueNode.Content[childIndex].Value
						childNode := valueNode.Content[childIndex+1]
						validateGroup(childName, childNode, appendPath(childPath, Key(childName)), descendants)
					}
				}
			default:
				diagnostics = append(diagnostics, doc.diagnostic("INV_GROUP_KEY", SeverityWarning, childPath, doc.nodeSpan(keyNode), fmt.Sprintf("unsupported group field %q; expected hosts, vars, or children", keyNode.Value)))
			}
		}
	}
	for i := 0; i+1 < len(doc.root.Content); i += 2 {
		name := doc.root.Content[i].Value
		validateGroup(name, doc.root.Content[i+1], Keys(name), nil)
	}
	return diagnostics
}

func isNullNode(node *yaml.Node) bool {
	return node != nil && node.Kind == yaml.ScalarNode && (node.Tag == "!!null" || node.Value == "")
}

func isMappingOrNull(node *yaml.Node) bool {
	return node != nil && (node.Kind == yaml.MappingNode || isNullNode(node))
}

func validatePigsty(doc *Document) []Diagnostic {
	if doc.inventory == nil || doc.inventory.All == nil {
		return nil
	}
	var diagnostics []Diagnostic
	globalVars := mappingValues(doc.inventory.All.Vars)
	if admin, ok := globalVars["admin_ip"]; !ok {
		diagnostics = append(diagnostics, Diagnostic{Code: "PIG_ADMIN_IP_REQUIRED", Severity: SeverityError, Path: AllVars("admin_ip"), Message: "all.vars.admin_ip is required"})
	} else if !validIPv4(scalarString(admin)) {
		diagnostics = append(diagnostics, Diagnostic{Code: "PIG_ADMIN_IP", Severity: SeverityError, Path: admin.Path, Range: admin.Range, Message: "admin_ip must be a valid IPv4 address"})
	}

	seenSeq := make(map[string]map[int]string)
	memberships := make(map[string]map[string]string)
	clusterName := regexp.MustCompile(`^[A-Za-z0-9-]+$`)
	registerMember := func(module string, group *Group, host *Host) {
		if memberships[module] == nil {
			memberships[module] = make(map[string]string)
		}
		if previous, exists := memberships[module][host.Name]; exists && previous != group.Path.String() {
			diagnostics = append(diagnostics, Diagnostic{Code: "PIG_HOST_MULTI_GROUP", Severity: SeverityError, Path: host.Path, Range: host.Range, Message: fmt.Sprintf("host %q belongs to multiple %s groups: %s and %s", host.Name, module, previous, group.Path.String())})
		} else {
			memberships[module][host.Name] = group.Path.String()
		}
	}
	validateSeq := func(host *Host, value *Value, name, identity string) {
		seq, ok := nonNegativeInteger(value)
		if !ok {
			diagnostics = append(diagnostics, Diagnostic{Code: "PIG_SEQ_TYPE", Severity: SeverityError, Path: value.Path, Range: value.Range, Message: fmt.Sprintf("%s for host %q must be a non-negative integer", name, host.Name)})
			return
		}
		scope := name + "\x00" + identity
		if seenSeq[scope] == nil {
			seenSeq[scope] = make(map[int]string)
		}
		if previous, exists := seenSeq[scope][seq]; exists {
			diagnostics = append(diagnostics, Diagnostic{Code: "PIG_SEQ_DUPLICATE", Severity: SeverityError, Path: value.Path, Range: value.Range, Message: fmt.Sprintf("%s %d is duplicated by hosts %q and %q for identity %q", name, seq, previous, host.Name, identity)})
		} else {
			seenSeq[scope][seq] = host.Name
		}
	}
	var visit func(*Group)
	visit = func(group *Group) {
		if group == nil {
			return
		}
		groupVars := mappingValues(group.Vars)
		if group.Name != "all" && len(group.Hosts) == 0 && len(group.Children) == 0 {
			diagnostics = append(diagnostics, Diagnostic{Code: "PIG_GROUP_HOSTS_EMPTY", Severity: SeverityError, Path: group.Path.ChildKey("hosts"), Range: group.Range, Message: fmt.Sprintf("group %q has no hosts", group.Name)})
		}
		pgIdentity := scalarString(groupVars["pg_cluster"])
		if pgIdentity != "" && !clusterName.MatchString(pgIdentity) {
			value := groupVars["pg_cluster"]
			diagnostics = append(diagnostics, Diagnostic{Code: "PIG_PG_CLUSTER", Severity: SeverityError, Path: value.Path, Range: value.Range, Message: "pg_cluster must contain only letters, digits, and hyphens"})
		}
		if address, exists := groupVars["pg_vip_address"]; exists && !validIPv4CIDR(scalarString(address)) {
			diagnostics = append(diagnostics, Diagnostic{Code: "PIG_VIP_ADDRESS", Severity: SeverityError, Path: address.Path, Range: address.Range, Message: "pg_vip_address must be a valid IPv4 CIDR"})
		}
		vipEnabled := scalarBool(groupVars["pg_vip_enabled"])
		for _, host := range group.Hosts {
			hostVars := mappingValues(&host.Vars)
			if !validIPv4(host.Name) {
				diagnostics = append(diagnostics, Diagnostic{Code: "PIG_HOST_IP", Severity: SeverityError, Path: host.Path, Range: host.Range, Message: fmt.Sprintf("host %q must be a valid IPv4 address", host.Name)})
			}

			hostPG := scalarString(hostVars["pg_cluster"])
			if hostPG == "" {
				hostPG = pgIdentity
			}
			_, hasPGSeq := hostVars["pg_seq"]
			_, hasPGRole := hostVars["pg_role"]
			if hostPG != "" || hasPGSeq || hasPGRole {
				registerMember("pgsql", group, host)
				identity := hostPG
				if identity == "" {
					identity = group.Name
					diagnostics = append(diagnostics, Diagnostic{Code: "PIG_IDENTITY_MISSING", Severity: SeverityError, Path: host.Path, Range: host.Range, Message: fmt.Sprintf("host %q has no pg_cluster identity", host.Name)})
				}
				if value, ok := hostVars["pg_seq"]; ok {
					validateSeq(host, value, "pg_seq", identity)
				} else {
					diagnostics = append(diagnostics, Diagnostic{Code: "PIG_PG_SEQ_REQUIRED", Severity: SeverityError, Path: host.Path, Range: host.Range, Message: fmt.Sprintf("host %q requires pg_seq", host.Name)})
				}
				if role, ok := hostVars["pg_role"]; !ok {
					diagnostics = append(diagnostics, Diagnostic{Code: "PIG_PG_ROLE_REQUIRED", Severity: SeverityError, Path: host.Path, Range: host.Range, Message: fmt.Sprintf("host %q requires pg_role", host.Name)})
				} else if !validPGRole(scalarString(role)) {
					diagnostics = append(diagnostics, Diagnostic{Code: "PIG_PG_ROLE", Severity: SeverityError, Path: role.Path, Range: role.Range, Message: fmt.Sprintf("pg_role for host %q must be primary, replica, standby, offline, or delayed", host.Name)})
				}
				if upstream, ok := hostVars["pg_upstream"]; ok && !validIPv4(scalarString(upstream)) {
					diagnostics = append(diagnostics, Diagnostic{Code: "PIG_PG_UPSTREAM", Severity: SeverityError, Path: upstream.Path, Range: upstream.Range, Message: "pg_upstream must be a valid IPv4 address"})
				}
				vipAddress, hasVIP := hostVars["pg_vip_address"]
				if !hasVIP {
					vipAddress, hasVIP = groupVars["pg_vip_address"]
				}
				if hasVIP && !validIPv4CIDR(scalarString(vipAddress)) {
					diagnostics = append(diagnostics, Diagnostic{Code: "PIG_VIP_ADDRESS", Severity: SeverityError, Path: vipAddress.Path, Range: vipAddress.Range, Message: "pg_vip_address must be a valid IPv4 CIDR"})
				} else if vipEnabled && !hasVIP {
					enabled := groupVars["pg_vip_enabled"]
					diagnostics = append(diagnostics, Diagnostic{Code: "PIG_VIP_ADDRESS_REQUIRED", Severity: SeverityError, Path: enabled.Path, Range: enabled.Range, Message: fmt.Sprintf("host %q requires pg_vip_address when pg_vip_enabled is true", host.Name)})
				}
			}

			for _, spec := range []struct{ module, seq, cluster string }{
				{module: "etcd", seq: "etcd_seq", cluster: "etcd_cluster"},
				{module: "minio", seq: "minio_seq", cluster: "minio_cluster"},
				{module: "infra", seq: "infra_seq"},
			} {
				identity := group.Name
				cluster := scalarString(hostVars[spec.cluster])
				if cluster == "" {
					cluster = scalarString(groupVars[spec.cluster])
				}
				seqValue, hasSeq := hostVars[spec.seq]
				explicit := hasSeq || cluster != "" || spec.module == "infra" && group.Name == "infra"
				if !explicit {
					continue
				}
				registerMember(spec.module, group, host)
				if spec.cluster != "" {
					if cluster == "" {
						diagnostics = append(diagnostics, Diagnostic{Code: "PIG_IDENTITY_MISSING", Severity: SeverityError, Path: host.Path, Range: host.Range, Message: fmt.Sprintf("host %q has no %s identity", host.Name, spec.cluster)})
					} else {
						identity = cluster
					}
				}
				if !hasSeq {
					diagnostics = append(diagnostics, Diagnostic{Code: "PIG_SEQ_REQUIRED", Severity: SeverityError, Path: host.Path, Range: host.Range, Message: fmt.Sprintf("host %q requires %s", host.Name, spec.seq)})
				} else {
					validateSeq(host, seqValue, spec.seq, identity)
				}
			}
		}
		for _, child := range group.Children {
			visit(child)
		}
	}
	visit(doc.inventory.All)
	return diagnostics
}

func validPGRole(role string) bool {
	return role == "primary" || role == "replica" || role == "standby" || role == "offline" || role == "delayed"
}

func validIPv4(value string) bool {
	address, err := netip.ParseAddr(value)
	return err == nil && address.Is4()
}

func validIPv4CIDR(value string) bool {
	prefix, err := netip.ParsePrefix(value)
	return err == nil && prefix.Addr().Is4()
}

func scalarBool(value *Value) bool {
	return value != nil && value.Scalar != nil && value.Scalar.Kind == BoolScalar && value.Scalar.Value == true
}

func mappingValues(value *Value) map[string]*Value {
	result := make(map[string]*Value)
	if value == nil || value.Kind != MappingValue {
		return result
	}
	for i := range value.Mapping {
		entry := &value.Mapping[i]
		result[fmt.Sprint(entry.Key.Value)] = &entry.Value
	}
	return result
}

func nonNegativeInteger(value *Value) (int, bool) {
	if value == nil || value.Kind != ScalarValue || value.Scalar == nil || value.Scalar.Kind != IntegerScalar {
		return 0, false
	}
	switch number := value.Scalar.Value.(type) {
	case int:
		return number, number >= 0
	case int64:
		return int(number), number >= 0
	case uint64:
		return int(number), true
	default:
		return 0, false
	}
}

func scalarString(value *Value) string {
	if value == nil || value.Kind != ScalarValue || value.Scalar == nil || value.Scalar.Value == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(fmt.Sprint(value.Scalar.Value)))
}

func (d *Document) diagnostic(code string, severity Severity, path Path, span byteSpan, message string) Diagnostic {
	if !span.valid(len(d.source)) {
		span = byteSpan{}
	}
	return Diagnostic{Code: code, Severity: severity, Path: clonePath(path), Range: d.sourceRange(span), Message: message}
}
