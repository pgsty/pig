package sty

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"pig/internal/cmdb"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"
	"pig/inventory"

	"gopkg.in/yaml.v3"
)

const (
	InventoryCMDBInitKind    = "pig.inventory.cmdb.init/v1"
	InventoryCMDBLoadKind    = "pig.inventory.cmdb.load/v1"
	InventoryCMDBDumpKind    = "pig.inventory.cmdb.dump/v1"
	InventoryCMDBEnableKind  = "pig.inventory.cmdb.enable/v1"
	InventoryCMDBDisableKind = "pig.inventory.cmdb.disable/v1"
	CMDBFailureKind          = "pig.inventory.cmdb.failure/v1"
	CMDBValidationKind       = "pig.inventory.cmdb.validation/v1"
)

// CMDBFailureData exposes one stable redacted finding code, never a driver
// error, DSN, SQL statement, Inventory value, or body.
type CMDBFailureData struct {
	Kind     string `json:"kind" yaml:"kind"`
	CMDBCode string `json:"cmdb_code" yaml:"cmdb_code"`
}

func (d *CMDBFailureData) Text() string {
	if d == nil || d.CMDBCode == "" {
		return ""
	}
	return "  finding: " + d.CMDBCode
}

type CMDBValidationData struct {
	Kind        string                 `json:"kind" yaml:"kind"`
	Path        string                 `json:"path" yaml:"path"`
	Diagnostics []inventory.Diagnostic `json:"diagnostics,omitempty" yaml:"diagnostics,omitempty"`
}

func (d *CMDBValidationData) Text() string {
	if d == nil || len(d.Diagnostics) == 0 {
		return ""
	}
	rows := make([][]string, 0, len(d.Diagnostics))
	for _, diagnostic := range d.Diagnostics {
		rows = append(rows, []string{diagnostic.Code, diagnostic.Path.String(), string(diagnostic.Severity)})
	}
	return strings.TrimSuffix(output.RenderTable([]string{"CODE", "PATH", "STATUS"}, rows), "\n")
}

// InventoryCMDBData is the shared value-free result metadata for the five
// thin operations. It never contains SQL, a DSN, or Inventory values.
type InventoryCMDBData struct {
	Kind                 string                `json:"kind" yaml:"kind"`
	Operation            string                `json:"operation" yaml:"operation"`
	ConnectionSource     cmdb.ConnectionSource `json:"connection_source,omitempty" yaml:"connection_source,omitempty"`
	SourcePath           string                `json:"source_path,omitempty" yaml:"source_path,omitempty"`
	OutputPath           string                `json:"output_path,omitempty" yaml:"output_path,omitempty"`
	ConfigPath           string                `json:"config_path,omitempty" yaml:"config_path,omitempty"`
	InventoryPath        string                `json:"inventory_path,omitempty" yaml:"inventory_path,omitempty"`
	Objects              int                   `json:"objects,omitempty" yaml:"objects,omitempty"`
	Statements           int                   `json:"statements,omitempty" yaml:"statements,omitempty"`
	Globals              int                   `json:"globals,omitempty" yaml:"globals,omitempty"`
	Groups               int                   `json:"groups,omitempty" yaml:"groups,omitempty"`
	Hosts                int                   `json:"hosts,omitempty" yaml:"hosts,omitempty"`
	GroupVars            int                   `json:"group_vars,omitempty" yaml:"group_vars,omitempty"`
	HostVars             int                   `json:"host_vars,omitempty" yaml:"host_vars,omitempty"`
	DefaultVars          int                   `json:"default_vars,omitempty" yaml:"default_vars,omitempty"`
	Packages             int                   `json:"packages,omitempty" yaml:"packages,omitempty"`
	Changed              bool                  `json:"changed,omitempty" yaml:"changed,omitempty"`
	RequiresConfirmation bool                  `json:"requires_confirmation,omitempty" yaml:"requires_confirmation,omitempty"`
	Warnings             []string              `json:"warnings,omitempty" yaml:"warnings,omitempty"`
}

func (d *InventoryCMDBData) Text() string {
	if d == nil {
		return ""
	}
	rows := [][]string{{"operation", d.Operation}}
	if d.ConnectionSource != "" {
		rows = append(rows, []string{"connection", string(d.ConnectionSource)})
	}
	for _, item := range []struct {
		name  string
		value string
	}{
		{"source", d.SourcePath}, {"output", d.OutputPath}, {"config", d.ConfigPath}, {"inventory", d.InventoryPath},
	} {
		if item.value != "" {
			rows = append(rows, []string{item.name, item.value})
		}
	}
	for _, item := range []struct {
		name  string
		value int
	}{
		{"objects", d.Objects}, {"statements", d.Statements}, {"globals", d.Globals}, {"groups", d.Groups},
		{"hosts", d.Hosts}, {"group vars", d.GroupVars}, {"host vars", d.HostVars},
		{"default vars", d.DefaultVars}, {"packages", d.Packages},
	} {
		if item.value != 0 {
			rows = append(rows, []string{item.name, fmt.Sprint(item.value)})
		}
	}
	for _, warning := range d.Warnings {
		rows = append(rows, []string{"warning", warning})
	}
	return strings.TrimSuffix(output.RenderTable([]string{"FIELD", "VALUE"}, rows), "\n")
}

type InventoryCMDBOptions struct {
	Home     string
	Database string
	Path     string
	Yes      bool
	OSCode   string
	OSArch   string
}

func openPigstyCMDB(ctx context.Context, database string) (*cmdb.Store, *output.Result) {
	resolved, err := cmdb.ResolvePigstyConnection(cmdb.PigstyConnectionOptions{Database: database})
	if err != nil {
		return nil, cmdbFailureResult("CMDB connection source is invalid", err)
	}
	store, err := cmdb.Open(ctx, resolved, cmdb.OpenOptions{})
	if err != nil {
		return nil, cmdbFailureResult("CMDB connection failed", err)
	}
	return store, nil
}

func InventoryCMDBInit(ctx context.Context, options InventoryCMDBOptions) *output.Result {
	path := filepath.Join(strings.TrimSpace(options.Home), "files", "cmdb.sql")
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return cmdbFailureResult("Pigsty cmdb.sql baseline is unavailable", &cmdb.Error{Code: "CMDB_BASELINE_INVALID"})
	}
	source, err := os.ReadFile(path)
	if err != nil || len(source) == 0 {
		return cmdbFailureResult("Pigsty cmdb.sql baseline cannot be read", &cmdb.Error{Code: "CMDB_BASELINE_INVALID"})
	}
	store, failure := openPigstyCMDB(ctx, options.Database)
	if failure != nil {
		return failure
	}
	defer store.Close()
	data := &InventoryCMDBData{Kind: InventoryCMDBInitKind, Operation: "init", ConnectionSource: store.Source(), SourcePath: path}
	inspection, err := store.InspectPigstyBaseline(ctx)
	if err != nil {
		return inventoryCMDBFailure("CMDB target could not be inspected", err, data)
	}
	if inspection.ExistingSchemas > 0 && !options.Yes {
		data.RequiresConfirmation = true
		return output.Fail(output.CodeStyCMDBConfirmationRequired, "CMDB init will replace existing pigsty/pglog schemas").
			WithData(data).
			WithNextActions(output.NextAction{Command: "pig inventory cmdb init --yes", Reason: "confirm replacement of existing Pigsty schemas", Required: true})
	}
	result, err := store.ApplyPigstyBaseline(ctx, source)
	if err != nil {
		return inventoryCMDBFailure("Pigsty cmdb.sql baseline could not be applied", err, data)
	}
	data.Objects, data.Statements = result.VerifiedObjects, result.StatementResults
	return output.OK("Pigsty CMDB initialized", data)
}

func InventoryCMDBLoad(ctx context.Context, options InventoryCMDBOptions) *output.Result {
	declarations, failure := preparePigstyDeclarations(options.Path)
	if failure != nil {
		return failure
	}
	store, failure := openPigstyCMDB(ctx, options.Database)
	if failure != nil {
		return failure
	}
	defer store.Close()
	data := &InventoryCMDBData{Kind: InventoryCMDBLoadKind, Operation: "load", ConnectionSource: store.Source(), SourcePath: options.Path}
	loaded, err := store.ReplaceDeclarations(ctx, declarations)
	if err != nil {
		return inventoryCMDBFailure("Pigsty Inventory could not be loaded", err, data)
	}
	data.Globals, data.Groups, data.Hosts = loaded.Globals, loaded.Groups, loaded.Hosts
	data.GroupVars, data.HostVars = loaded.GroupVar, loaded.HostVar

	defaults, warning := readPigstyOSDefaults(options.Home, options.OSCode, options.OSArch)
	if warning != "" {
		data.Warnings = append(data.Warnings, warning)
	} else if len(defaults.Values) > 0 || len(defaults.Packages) > 0 {
		updated, err := store.ApplyOSDefaults(ctx, defaults)
		if err != nil {
			data.Warnings = append(data.Warnings, "CMDB_DEFAULTS_FAILED")
		} else {
			data.DefaultVars, data.Packages = updated.Values, updated.Packages
		}
	}
	return output.OK("Pigsty Inventory loaded into CMDB", data)
}

// InventoryCMDBDump returns secret-bearing YAML separately from safe Result
// metadata. With a destination, the returned body is nil.
func InventoryCMDBDump(ctx context.Context, options InventoryCMDBOptions) ([]byte, *output.Result) {
	if strings.TrimSpace(options.Path) == "" && config.IsStructuredOutput() {
		return nil, output.Fail(output.CodeStyCMDBInvalidArgs, "structured CMDB dump requires a destination file")
	}
	store, failure := openPigstyCMDB(ctx, options.Database)
	if failure != nil {
		return nil, failure
	}
	defer store.Close()
	raw, err := store.RawConfig(ctx)
	data := &InventoryCMDBData{Kind: InventoryCMDBDumpKind, Operation: "dump", ConnectionSource: store.Source()}
	if err != nil {
		return nil, inventoryCMDBFailure("Pigsty raw_config could not be exported", err, data)
	}
	body, err := canonicalCMDBYAML(raw)
	if err != nil {
		return nil, inventoryCMDBFailure("Pigsty raw_config could not be encoded as YAML", &cmdb.Error{Code: "CMDB_DUMP_FAILED"}, data)
	}
	if strings.TrimSpace(options.Path) == "" {
		return body, output.OK("Pigsty CMDB dumped", data)
	}
	path, err := filepath.Abs(options.Path)
	if err != nil {
		return nil, inventoryCMDBFailure("CMDB dump destination is invalid", &cmdb.Error{Code: "CMDB_OUTPUT_INVALID"}, data)
	}
	if err := utils.AtomicWriteFile(path, body, 0o600); err != nil {
		return nil, inventoryCMDBFailure("CMDB dump destination could not be written", &cmdb.Error{Code: "CMDB_OUTPUT_FAILED"}, data)
	}
	data.OutputPath = path
	return nil, output.OK("Pigsty CMDB dumped", data)
}

func InventoryCMDBEnable(ctx context.Context, options InventoryCMDBOptions) *output.Result {
	store, failure := openPigstyCMDB(ctx, options.Database)
	if failure != nil {
		return failure
	}
	defer store.Close()
	data := &InventoryCMDBData{Kind: InventoryCMDBEnableKind, Operation: "enable", ConnectionSource: store.Source()}
	result, err := store.EnablePigstyInventory(ctx, options.Home)
	if err != nil {
		return inventoryCMDBFailure("dynamic Pigsty Inventory could not be enabled", err, data)
	}
	data.ConfigPath, data.InventoryPath, data.Changed = result.ConfigPath, result.InventoryPath, result.Changed
	return output.OK("dynamic Pigsty Inventory enabled", data)
}

func InventoryCMDBDisable(options InventoryCMDBOptions) *output.Result {
	data := &InventoryCMDBData{Kind: InventoryCMDBDisableKind, Operation: "disable"}
	result, err := cmdb.DisablePigstyInventory(options.Home)
	if err != nil {
		return inventoryCMDBFailure("static Pigsty Inventory could not be enabled", err, data)
	}
	data.ConfigPath, data.InventoryPath, data.Changed = result.ConfigPath, result.InventoryPath, result.Changed
	return output.OK("static Pigsty Inventory enabled", data)
}

func inventoryCMDBFailure(message string, err error, data *InventoryCMDBData) *output.Result {
	result := cmdbFailureResult(message, err)
	result.Data = data
	return result
}

func cmdbFailureResult(message string, err error) *output.Result {
	code, status := cmdbFailureCode(err)
	return output.Fail(status, message).WithData(&CMDBFailureData{Kind: CMDBFailureKind, CMDBCode: code})
}

func cmdbFailureCode(err error) (string, int) {
	var failure *cmdb.Error
	if !errors.As(err, &failure) || failure == nil || failure.Code == "" {
		return "CMDB_INTERNAL", output.CodeStyCMDBOperationFailed
	}
	switch failure.Code {
	case "CMDB_PARAMETER_INVALID", "CMDB_DECLARATION_INVALID":
		return failure.Code, output.CodeStyCMDBInvalidArgs
	case "CMDB_SWITCH_PERMISSION":
		return failure.Code, output.CodeStyCMDBPermissionDenied
	case "CMDB_CONNECTION_FAILED":
		return failure.Code, output.CodeStyCMDBConnectionFailed
	case "CMDB_CONNECTION_INVALID", "CMDB_SOURCE_INVALID", "CMDB_CONNECTION_SECRET", "CMDB_ANSIBLE_CONFIG_INVALID", "CMDB_SWITCH_INVALID":
		return failure.Code, output.CodeStyCMDBConfigInvalid
	case "CMDB_NESTED_GROUP_UNSUPPORTED", "CMDB_HOSTS_REQUIRED", "CMDB_HOST_VARS_REQUIRED":
		return failure.Code, output.CodeStyCMDBUnrepresentable
	case "CMDB_BASELINE_INVALID", "CMDB_BASELINE_VERIFY_FAILED":
		return failure.Code, output.CodeStyCMDBSchemaInvalid
	case "CMDB_OUTPUT_INVALID", "CMDB_OUTPUT_FAILED", "CMDB_SWITCH_PARTIAL":
		return failure.Code, output.CodeStyCMDBOutputFailed
	case "CMDB_SOURCE_CHANGED":
		return failure.Code, output.CodeStyCMDBSourceChanged
	case "CMDB_CONFIRMATION_REQUIRED":
		return failure.Code, output.CodeStyCMDBConfirmationRequired
	default:
		return failure.Code, output.CodeStyCMDBOperationFailed
	}
}

func preparePigstyDeclarations(path string) (cmdb.Declarations, *output.Result) {
	doc, failure := loadStaticInventory(path, output.CodeStyCMDBInvalidArgs)
	if failure != nil {
		return cmdb.Declarations{}, failure
	}
	// The existing loader imports declarations; product-level completeness is
	// owned by `pig inventory validate`, not by this transport operation.
	diagnostics := doc.Validate(inventory.YAMLRules, inventory.AnsibleRules)
	if inventory.HasErrors(diagnostics) {
		return cmdb.Declarations{}, output.Fail(output.CodeStyValidationFailed, "Inventory validation failed before CMDB load").
			WithData(&CMDBValidationData{Kind: CMDBValidationKind, Path: path, Diagnostics: diagnostics})
	}
	semantic := doc.Inventory()
	if semantic == nil || semantic.All == nil || len(semantic.All.Hosts) > 0 {
		return cmdb.Declarations{}, cmdbRepresentabilityFailure(path, "CMDB_DECLARATION_INVALID")
	}
	globals, err := valueFields(semantic.All.Vars)
	if err != nil {
		return cmdb.Declarations{}, cmdbRepresentabilityFailure(path, "CMDB_DECLARATION_INVALID")
	}
	declarations := cmdb.Declarations{Globals: globals, Groups: make([]cmdb.GroupDeclaration, 0, len(semantic.All.Children))}
	for _, group := range semantic.All.Children {
		if group == nil || len(group.Children) > 0 {
			return cmdb.Declarations{}, cmdbRepresentabilityFailure(path, "CMDB_NESTED_GROUP_UNSUPPORTED")
		}
		groupValue, ok := doc.Lookup(group.Path)
		if !ok || groupValue.Kind != inventory.MappingValue {
			return cmdb.Declarations{}, cmdbRepresentabilityFailure(path, "CMDB_DECLARATION_INVALID")
		}
		hostsValue, ok := groupValue.LookupKey("hosts")
		if !ok || hostsValue.Kind != inventory.MappingValue {
			return cmdb.Declarations{}, cmdbRepresentabilityFailure(path, "CMDB_HOSTS_REQUIRED")
		}
		vars, err := valueFields(group.Vars)
		if err != nil {
			return cmdb.Declarations{}, cmdbRepresentabilityFailure(path, "CMDB_DECLARATION_INVALID")
		}
		declaration := cmdb.GroupDeclaration{Name: group.Name, Vars: vars, Hosts: make([]cmdb.HostDeclaration, 0, len(group.Hosts))}
		for _, host := range group.Hosts {
			if host == nil || host.Vars.Kind != inventory.MappingValue {
				return cmdb.Declarations{}, cmdbRepresentabilityFailure(path, "CMDB_HOST_VARS_REQUIRED")
			}
			vars, err := valueFields(&host.Vars)
			if err != nil {
				return cmdb.Declarations{}, cmdbRepresentabilityFailure(path, "CMDB_DECLARATION_INVALID")
			}
			declaration.Hosts = append(declaration.Hosts, cmdb.HostDeclaration{IP: host.Name, Vars: vars})
		}
		declarations.Groups = append(declarations.Groups, declaration)
	}
	return declarations, nil
}

func cmdbRepresentabilityFailure(path, code string) *output.Result {
	return output.Fail(output.CodeStyCMDBUnrepresentable, "Inventory cannot be represented by the existing Pigsty CMDB").
		WithData(&CMDBFailureData{Kind: CMDBFailureKind, CMDBCode: code}).
		WithDetail("source: " + path)
}

func valueFields(value *inventory.Value) ([]cmdb.JSONField, error) {
	if value == nil {
		return nil, nil
	}
	if value.Kind != inventory.MappingValue {
		return nil, errors.New("declaration vars must be a mapping")
	}
	fields := make([]cmdb.JSONField, 0, len(value.Mapping))
	for _, entry := range value.Mapping {
		key, ok := entry.Key.Value.(string)
		if !ok || key == "" {
			return nil, errors.New("declaration key must be a string")
		}
		plain, err := plainInventoryValue(&entry.Value)
		if err != nil {
			return nil, err
		}
		encoded, err := json.Marshal(plain)
		if err != nil {
			return nil, err
		}
		fields = append(fields, cmdb.JSONField{Key: key, Value: encoded})
	}
	return fields, nil
}

func plainInventoryValue(value *inventory.Value) (any, error) {
	if value == nil || value.Kind == inventory.NullValue {
		return nil, nil
	}
	if value.Anchor != "" || value.Alias != "" || strings.HasPrefix(value.Tag, "!") && !strings.HasPrefix(value.Tag, "!!") {
		return nil, errors.New("tagged or aliased value")
	}
	switch value.Kind {
	case inventory.MappingValue:
		mapping := make(map[string]any, len(value.Mapping))
		for _, entry := range value.Mapping {
			key, ok := jsonMappingKey(entry.Key)
			if !ok {
				return nil, errors.New("mapping key is not a string")
			}
			child, err := plainInventoryValue(&entry.Value)
			if err != nil {
				return nil, err
			}
			mapping[key] = child
		}
		return mapping, nil
	case inventory.SequenceValue:
		sequence := make([]any, len(value.Sequence))
		for index := range value.Sequence {
			child, err := plainInventoryValue(&value.Sequence[index])
			if err != nil {
				return nil, err
			}
			sequence[index] = child
		}
		return sequence, nil
	case inventory.ScalarValue:
		if value.Scalar == nil || value.Scalar.Style == "tagged" {
			return nil, errors.New("unsupported scalar")
		}
		switch value.Scalar.Kind {
		case inventory.StringScalar, inventory.BoolScalar, inventory.IntegerScalar, inventory.FloatScalar:
			return value.Scalar.Value, nil
		case inventory.NullScalar:
			return nil, nil
		default:
			return nil, errors.New("unknown scalar")
		}
	default:
		return nil, errors.New("unsupported value")
	}
}

func jsonMappingKey(key inventory.Scalar) (string, bool) {
	switch key.Kind {
	case inventory.StringScalar:
		value, ok := key.Value.(string)
		return value, ok
	case inventory.BoolScalar, inventory.IntegerScalar, inventory.FloatScalar:
		return fmt.Sprint(key.Value), true
	case inventory.NullScalar:
		return "null", true
	default:
		return "", false
	}
}

var osDefaultKeys = map[string]string{
	"repo_packages_default":       "repo_packages",
	"repo_extra_packages_default": "repo_extra_packages",
	"node_packages_default":       "node_packages",
	"infra_packages_default":      "infra_packages",
	"repo_upstream_default":       "repo_upstream",
}

func readPigstyOSDefaults(home, osCode, arch string) (cmdb.OSDefaults, string) {
	osCode, arch = strings.TrimSpace(strings.ToLower(osCode)), normalizeNodeVarsArch(arch)
	fallback := ""
	switch {
	case strings.HasPrefix(osCode, "el"):
		fallback = "el9"
	case strings.HasPrefix(osCode, "d"):
		fallback = "d12"
	case strings.HasPrefix(osCode, "u"):
		fallback = "u22"
	default:
		return cmdb.OSDefaults{}, ""
	}
	directory := filepath.Join(home, "roles", "node_id", "vars")
	path := filepath.Join(directory, osCode+"."+arch+".yml")
	if _, err := os.Stat(path); err != nil {
		path = filepath.Join(directory, fallback+"."+arch+".yml")
	}
	source, err := os.ReadFile(path)
	if err != nil {
		return cmdb.OSDefaults{}, ""
	}
	var values map[string]any
	if err := yaml.Unmarshal(source, &values); err != nil {
		return cmdb.OSDefaults{}, "CMDB_DEFAULTS_PARSE_FAILED"
	}
	defaults := cmdb.OSDefaults{}
	for sourceKey, targetKey := range osDefaultKeys {
		value, exists := values[sourceKey]
		if !exists {
			continue
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return cmdb.OSDefaults{}, "CMDB_DEFAULTS_PARSE_FAILED"
		}
		defaults.Values = append(defaults.Values, cmdb.JSONField{Key: targetKey, Value: encoded})
	}
	if packages, ok := values["package_map"].(map[string]any); ok {
		for key, value := range packages {
			defaults.Packages = append(defaults.Packages, cmdb.TextField{Key: key, Value: fmt.Sprint(value)})
		}
	}
	return defaults, ""
}

func normalizeNodeVarsArch(arch string) string {
	switch strings.TrimSpace(strings.ToLower(arch)) {
	case "amd64", "x86_64":
		return "x86_64"
	case "arm64", "aarch64":
		return "aarch64"
	default:
		return strings.TrimSpace(strings.ToLower(arch))
	}
}

func canonicalCMDBYAML(source []byte) ([]byte, error) {
	if !json.Valid(source) {
		return nil, errors.New("invalid raw_config JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(source))
	decoder.UseNumber()
	var generated map[string]any
	if err := decoder.Decode(&generated); err != nil {
		return nil, err
	}
	normalizeRawConfig(generated)
	source, err := json.Marshal(generated)
	if err != nil {
		return nil, err
	}
	var document yaml.Node
	if err := yaml.Unmarshal(source, &document); err != nil {
		return nil, err
	}
	clearGeneratedYAMLStyle(&document)
	var buffer bytes.Buffer
	encoder := yaml.NewEncoder(&buffer)
	encoder.SetIndent(2)
	if err := encoder.Encode(&document); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	body := buffer.Bytes()
	if _, err := inventory.Parse("CMDB dump", body); err != nil {
		return nil, err
	}
	return body, nil
}

func normalizeRawConfig(root map[string]any) {
	all, _ := root["all"].(map[string]any)
	if all == nil {
		return
	}
	if all["vars"] == nil {
		all["vars"] = map[string]any{}
	}
	children, _ := all["children"].(map[string]any)
	if children == nil {
		children = map[string]any{}
		all["children"] = children
	}
	for _, raw := range children {
		group, _ := raw.(map[string]any)
		if group == nil {
			continue
		}
		if group["hosts"] == nil {
			group["hosts"] = map[string]any{}
		}
		if group["vars"] == nil {
			group["vars"] = map[string]any{}
		}
	}
}

func clearGeneratedYAMLStyle(node *yaml.Node) {
	if node == nil {
		return
	}
	node.Style = 0
	if node.Kind == yaml.ScalarNode && node.Tag == "!!str" && yaml11AmbiguousString(node.Value) {
		node.Style = yaml.DoubleQuotedStyle
	}
	for _, child := range node.Content {
		clearGeneratedYAMLStyle(child)
	}
}

func yaml11AmbiguousString(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "y", "yes", "n", "no", "true", "false", "on", "off", "null", "~", ".nan", ".inf", "+.inf", "-.inf":
		return true
	default:
		return false
	}
}
