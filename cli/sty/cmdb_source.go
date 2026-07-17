package sty

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"

	"pig/internal/cmdb"
	"pig/internal/output"
)

const maxCMDBUserConfigBytes = 1 << 20

// CMDBSourceResolveOptions preserves each parent-contract precedence tier as a
// separate input. No selected tier is silently replaced by a later fallback.
type CMDBSourceResolveOptions struct {
	PigstyHome          string
	InvocationInventory string
	WorkingDirectory    string
	UserHome            string
	UserConfigPath      string
	RuntimeRoot         string
	RuntimePath         string
	PigBinary           string
	LookupEnv           func(string) (string, bool)
}

// CMDBSourceResolution keeps the invocation source separate from the
// authoritative persistent ansible.cfg source used for split-brain reporting.
type CMDBSourceResolution struct {
	Effective         CMDBSourceDescriptor `json:"effective" yaml:"effective"`
	Persistent        CMDBSourceDescriptor `json:"persistent" yaml:"persistent"`
	PersistentFinding string               `json:"persistent_finding,omitempty" yaml:"persistent_finding,omitempty"`
}

type cmdbRuntimeSourceContext struct {
	recordPath      string
	expectedAdapter string
	pigBinary       string
}

// ResolveCMDBSource implements CLI, environment, user config, ansible.cfg,
// home-default precedence. It never probes PostgreSQL and never falls through
// after a non-empty selected tier proves unsupported.
func ResolveCMDBSource(options CMDBSourceResolveOptions) (CMDBSourceResolution, *output.Result) {
	var resolution CMDBSourceResolution
	home, failure := canonicalCMDBSourceHome(options.PigstyHome)
	if failure != nil {
		return resolution, failure
	}
	workingDirectory, failure := cmdbSourceWorkingDirectory(options.WorkingDirectory)
	if failure != nil {
		return resolution, failure
	}
	userHome := options.UserHome
	if userHome == "" {
		var err error
		userHome, err = os.UserHomeDir()
		if err != nil {
			return resolution, cmdbSourceFailure("CMDB_SOURCE_INVALID", "CMDB source user home is unavailable")
		}
	}
	userHome, failure = canonicalCMDBSourceDirectory(userHome)
	if failure != nil {
		return resolution, failure
	}
	runtimeContext := newCMDBRuntimeSourceContext(home, options.RuntimeRoot, options.RuntimePath, options.PigBinary)

	persistent, persistentPresent, persistentCode := resolvePersistentCMDBSource(home, runtimeContext)
	resolution.Persistent = persistent
	resolution.PersistentFinding = persistentCode

	if strings.TrimSpace(options.InvocationInventory) != "" {
		descriptor, code := resolveCMDBSourceValue(
			options.InvocationInventory, workingDirectory, userHome,
			CMDBSourceCLI, false, true, runtimeContext,
		)
		resolution.Effective = descriptor
		if code != "" {
			return resolution, cmdbSourceFailure(code, "invocation Inventory source is unsupported")
		}
		return resolution, nil
	}

	lookup := options.LookupEnv
	if lookup == nil {
		lookup = os.LookupEnv
	}
	if value, present := lookup("PIGSTY_INVENTORY"); present && strings.TrimSpace(value) != "" {
		descriptor, code := resolveCMDBSourceValue(
			value, workingDirectory, userHome,
			CMDBSourceEnvironment, false, false, runtimeContext,
		)
		resolution.Effective = descriptor
		if code != "" {
			return resolution, cmdbSourceFailure(code, "environment Inventory source is unsupported")
		}
		return resolution, nil
	}

	configPath := options.UserConfigPath
	if configPath == "" {
		configPath = filepath.Join(userHome, ".pig", "config.yml")
	}
	configured, present, err := readCMDBUserInventory(configPath)
	if err != nil {
		resolution.Effective = CMDBSourceDescriptor{
			Kind: CMDBSourceUnsupported, Origin: CMDBSourceUserConfig,
			Locator: canonicalBestEffortPath(configPath, workingDirectory),
		}
		return resolution, cmdbSourceFailure("CMDB_SOURCE_INVALID", "user Inventory configuration is invalid")
	}
	if present && strings.TrimSpace(configured) != "" {
		descriptor, code := resolveCMDBSourceValue(
			configured, workingDirectory, userHome,
			CMDBSourceUserConfig, false, false, runtimeContext,
		)
		resolution.Effective = descriptor
		if code != "" {
			return resolution, cmdbSourceFailure(code, "user-config Inventory source is unsupported")
		}
		return resolution, nil
	}

	if persistentPresent {
		resolution.Effective = persistent
		if persistentCode != "" {
			return resolution, cmdbSourceFailure(persistentCode, "persistent Inventory source is unsupported")
		}
		return resolution, nil
	}

	descriptor, code := classifyCMDBSourcePath(
		filepath.Join(home, "pigsty.yml"), CMDBSourceHomeDefault, false, false, runtimeContext,
	)
	resolution.Effective = descriptor
	if code != "" {
		return resolution, cmdbSourceFailure(code, "home-default Inventory source is unsupported")
	}
	return resolution, nil
}

func resolvePersistentCMDBSource(home string, runtimeContext cmdbRuntimeSourceContext) (CMDBSourceDescriptor, bool, string) {
	configPath := filepath.Join(home, "ansible.cfg")
	descriptor := CMDBSourceDescriptor{
		Kind: CMDBSourceUnsupported, Origin: CMDBSourceAnsibleConfig,
		Locator: configPath, Persistent: true,
	}
	if _, err := os.Lstat(configPath); errors.Is(err, os.ErrNotExist) {
		return descriptor, false, "CMDB_ANSIBLE_CONFIG_MISSING"
	}
	source, err := cmdb.InspectAnsibleConfig(configPath)
	if err != nil {
		return descriptor, true, "CMDB_ANSIBLE_CONFIG_INVALID"
	}
	classified, code := classifyCMDBSourcePath(source.Locator, CMDBSourceAnsibleConfig, true, false, runtimeContext)
	return classified, true, code
}

func resolveCMDBSourceValue(value, base, userHome string, origin CMDBSourceOrigin, persistent, forceStatic bool, runtimeContext cmdbRuntimeSourceContext) (CMDBSourceDescriptor, string) {
	path, ok := canonicalCMDBSourceValue(value, base, userHome)
	if !ok {
		return CMDBSourceDescriptor{
			Kind: CMDBSourceUnsupported, Origin: origin,
			Persistent: persistent,
		}, "CMDB_SOURCE_UNSUPPORTED"
	}
	return classifyCMDBSourcePath(path, origin, persistent, forceStatic, runtimeContext)
}

func classifyCMDBSourcePath(path string, origin CMDBSourceOrigin, persistent, forceStatic bool, runtimeContext cmdbRuntimeSourceContext) (CMDBSourceDescriptor, string) {
	descriptor := CMDBSourceDescriptor{
		Kind: CMDBSourceUnsupported, Origin: origin, Locator: path, Persistent: persistent,
	}
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return descriptor, "CMDB_SOURCE_UNSUPPORTED"
	}
	if path == runtimeContext.expectedAdapter && !forceStatic {
		record, err := cmdb.VerifyRuntimeFile(runtimeContext.recordPath, runtimeContext.pigBinary)
		if err != nil || record.AdapterPath != path {
			return descriptor, "CMDB_RUNTIME_ADAPTER_INVALID"
		}
		descriptor.Kind = CMDBSourceNative
		descriptor.Locator = cmdb.RuntimeAdapterIdentity(record)
		return descriptor, ""
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && cmdbSourceParentIsCanonical(path) {
			descriptor.Kind = CMDBSourceStatic
			return descriptor, ""
		}
		return descriptor, "CMDB_SOURCE_UNSUPPORTED"
	}
	if resolved != path {
		return descriptor, "CMDB_SOURCE_UNSUPPORTED"
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 != 0 {
		return descriptor, "CMDB_SOURCE_UNSUPPORTED"
	}
	if looksLikeCMDBInventoryPlugin(path, info) {
		return descriptor, "CMDB_SOURCE_UNSUPPORTED"
	}
	descriptor.Kind = CMDBSourceStatic
	return descriptor, ""
}

func newCMDBRuntimeSourceContext(home, runtimeRoot, runtimePath, pigBinary string) cmdbRuntimeSourceContext {
	context := cmdbRuntimeSourceContext{pigBinary: pigBinary}
	recordPath := runtimePath
	if recordPath == "" {
		var err error
		recordPath, err = cmdb.RuntimeRecordPath(home, runtimeRoot)
		if err != nil {
			return context
		}
	} else {
		absolute, err := filepath.Abs(recordPath)
		if err != nil {
			return context
		}
		recordPath = filepath.Clean(absolute)
	}
	context.recordPath = recordPath
	context.expectedAdapter = filepath.Join(filepath.Dir(recordPath), "inventory")
	if context.pigBinary == "" {
		context.pigBinary, _ = os.Executable()
	}
	return context
}

func canonicalCMDBSourceValue(value, base, userHome string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" || !utf8.ValidString(value) || strings.ContainsAny(value, "\x00\r\n,$") ||
		strings.Contains(value, "{{") || strings.Contains(value, "{%") || strings.Contains(value, "%(") {
		return "", false
	}
	if value == "~" {
		value = userHome
	} else if strings.HasPrefix(value, "~/") {
		value = filepath.Join(userHome, strings.TrimPrefix(value, "~/"))
	}
	if !filepath.IsAbs(value) {
		value = filepath.Join(base, value)
	}
	absolute, err := filepath.Abs(value)
	if err != nil || !utf8.ValidString(absolute) {
		return "", false
	}
	return filepath.Clean(absolute), true
}

func canonicalCMDBSourceHome(home string) (string, *output.Result) {
	if strings.TrimSpace(home) == "" {
		return "", cmdbSourceFailure("CMDB_SOURCE_INVALID", "Pigsty home is required for source resolution")
	}
	absolute, err := filepath.Abs(home)
	if err != nil {
		return "", cmdbSourceFailure("CMDB_SOURCE_INVALID", "Pigsty home path is invalid")
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", cmdbSourceFailure("CMDB_SOURCE_INVALID", "Pigsty home is unavailable")
	}
	info, err := os.Lstat(resolved)
	if err != nil || !info.IsDir() {
		return "", cmdbSourceFailure("CMDB_SOURCE_INVALID", "Pigsty home is unavailable")
	}
	return filepath.Clean(resolved), nil
}

func canonicalCMDBSourceDirectory(path string) (string, *output.Result) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", cmdbSourceFailure("CMDB_SOURCE_INVALID", "source base directory is invalid")
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", cmdbSourceFailure("CMDB_SOURCE_INVALID", "source base directory is unavailable")
	}
	info, err := os.Lstat(resolved)
	if err != nil || !info.IsDir() {
		return "", cmdbSourceFailure("CMDB_SOURCE_INVALID", "source base directory is unavailable")
	}
	return filepath.Clean(resolved), nil
}

func cmdbSourceWorkingDirectory(configured string) (string, *output.Result) {
	if configured == "" {
		var err error
		configured, err = os.Getwd()
		if err != nil {
			return "", cmdbSourceFailure("CMDB_SOURCE_INVALID", "working directory is unavailable")
		}
	}
	return canonicalCMDBSourceDirectory(configured)
}

func readCMDBUserInventory(name string) (string, bool, error) {
	absolute, err := filepath.Abs(name)
	if err != nil {
		return "", false, errors.New("invalid user config path")
	}
	absolute = filepath.Clean(absolute)
	resolved, err := filepath.EvalSymlinks(absolute)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil || resolved != absolute {
		return "", false, errors.New("unsafe user config path")
	}
	info, err := os.Lstat(absolute)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxCMDBUserConfigBytes {
		return "", false, errors.New("invalid user config file")
	}
	file, err := os.Open(absolute)
	if err != nil {
		return "", false, errors.New("unreadable user config file")
	}
	body, readErr := io.ReadAll(io.LimitReader(file, maxCMDBUserConfigBytes+1))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil || len(body) > maxCMDBUserConfigBytes {
		return "", false, errors.New("unreadable user config file")
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return "", false, nil
	}
	var document yaml.Node
	if err := yaml.Unmarshal(body, &document); err != nil || len(document.Content) != 1 {
		return "", false, errors.New("invalid user config YAML")
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return "", false, errors.New("invalid user config root")
	}
	found := false
	value := ""
	for index := 0; index < len(root.Content); index += 2 {
		key, item := root.Content[index], root.Content[index+1]
		if key.Kind != yaml.ScalarNode || key.Value != "inventory" {
			continue
		}
		if found || item.Kind != yaml.ScalarNode {
			return "", false, errors.New("invalid user inventory value")
		}
		found = true
		switch item.Tag {
		case "!!str":
			value = item.Value
		case "!!null":
			value = ""
		default:
			return "", false, errors.New("invalid user inventory value")
		}
	}
	return value, found, nil
}

func cmdbSourceParentIsCanonical(path string) bool {
	parent := filepath.Dir(path)
	resolved, err := filepath.EvalSymlinks(parent)
	return err == nil && resolved == parent
}

func looksLikeCMDBInventoryPlugin(path string, info os.FileInfo) bool {
	if info == nil || info.Size() < 0 || info.Size() > maxCMDBUserConfigBytes {
		return false
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var document yaml.Node
	if yaml.Unmarshal(body, &document) != nil || len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return false
	}
	root := document.Content[0]
	for index := 0; index < len(root.Content); index += 2 {
		if root.Content[index].Kind == yaml.ScalarNode && root.Content[index].Value == "plugin" {
			return true
		}
	}
	return false
}

func canonicalBestEffortPath(path, base string) string {
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, path)
	}
	if absolute, err := filepath.Abs(path); err == nil {
		return filepath.Clean(absolute)
	}
	return strings.TrimSpace(path)
}

func cmdbSourceFailure(code, message string) *output.Result {
	status := output.CodeStyCMDBConfigInvalid
	if code == "CMDB_RUNTIME_ADAPTER_INVALID" {
		status = output.CodeStyCMDBOperationFailed
	}
	return output.Fail(status, message).WithData(&CMDBFailureData{Kind: CMDBFailureKind, CMDBCode: code})
}
