package config

import (
	"bytes"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/spf13/viper"
)

const maxInventoryAnsibleConfigSize = 1 << 20

var ErrAnsibleInventoryDirectiveMissing = errors.New("ansible inventory directive is missing")

type InventorySourceTier string

const (
	InventorySourceFlag          InventorySourceTier = "flag"
	InventorySourceEnvironment   InventorySourceTier = "environment"
	InventorySourceConfig        InventorySourceTier = "config"
	InventorySourceAnsibleConfig InventorySourceTier = "ansible_config"
	InventorySourceDefault       InventorySourceTier = "default"
	InventorySourceUnconfigured  InventorySourceTier = "unconfigured"
)

type InventorySourceType string

const (
	InventorySourceStaticYAML        InventorySourceType = "static_yaml"
	InventorySourceDirectory         InventorySourceType = "directory"
	InventorySourceExecutableDynamic InventorySourceType = "executable_dynamic"
	InventorySourceManagedCMDB       InventorySourceType = "managed_cmdb"
	InventorySourceMissing           InventorySourceType = "missing"
	InventorySourceUnsupported       InventorySourceType = "unsupported"
)

type InventorySourceDescriptor struct {
	Home           string              `json:"home,omitempty" yaml:"home,omitempty"`
	Tier           InventorySourceTier `json:"tier" yaml:"tier"`
	Type           InventorySourceType `json:"type" yaml:"type"`
	Path           string              `json:"path,omitempty" yaml:"path,omitempty"`
	OriginPath     string              `json:"origin_path,omitempty" yaml:"origin_path,omitempty"`
	Exists         bool                `json:"exists" yaml:"exists"`
	Readable       bool                `json:"readable" yaml:"readable"`
	Executable     bool                `json:"executable" yaml:"executable"`
	Symlink        bool                `json:"symlink" yaml:"symlink"`
	Managed        bool                `json:"managed" yaml:"managed"`
	StaticCommands []string            `json:"static_commands,omitempty" yaml:"static_commands,omitempty"`
	Findings       []string            `json:"findings,omitempty" yaml:"findings,omitempty"`
}

type AnsibleInventoryDirective struct {
	Literal    string
	Value      string
	Path       string
	ValueStart int
	ValueEnd   int
}

// ParseAnsibleInventoryDirective is the shared strict parser used by source
// discovery and CMDB source switching. It deliberately supports one literal
// inventory path rather than general Ansible INI evaluation.
func ParseAnsibleInventoryDirective(configPath string, body []byte) (AnsibleInventoryDirective, error) {
	if len(body) > maxInventoryAnsibleConfigSize || !utf8.Valid(body) || bytes.IndexByte(body, 0) >= 0 {
		return AnsibleInventoryDirective{}, errors.New("invalid ansible config encoding")
	}
	parser := ansibleInventoryDirectiveParser{configPath: configPath, body: body}
	for offset := 0; offset < len(body); {
		lineStart := offset
		newline := bytes.IndexByte(body[offset:], '\n')
		lineEnd := len(body)
		if newline >= 0 {
			lineEnd = offset + newline
			offset = lineEnd + 1
		} else {
			offset = len(body)
		}
		if err := parser.consumeLine(lineStart, lineEnd); err != nil {
			return AnsibleInventoryDirective{}, err
		}
	}
	return parser.result()
}

type ansibleInventoryDirectiveParser struct {
	configPath       string
	body             []byte
	directive        AnsibleInventoryDirective
	defaultsSections int
	inDefaults       bool
	assignments      int
}

func (p *ansibleInventoryDirectiveParser) consumeLine(lineStart, lineEnd int) error {
	contentEnd := lineEnd
	if contentEnd > lineStart && p.body[contentEnd-1] == '\r' {
		contentEnd--
	}
	line := p.body[lineStart:contentEnd]
	if bytes.IndexByte(line, '\r') >= 0 {
		return errors.New("unsupported ansible config line ending")
	}
	trimmed, leading := trimInventorySpace(line)
	if len(trimmed) == 0 || trimmed[0] == '#' || trimmed[0] == ';' {
		return nil
	}
	if trimmed[0] == '[' {
		section, ok := parseInventorySection(trimmed)
		if !ok {
			return errors.New("malformed ansible config section")
		}
		p.inDefaults = strings.EqualFold(section, "defaults")
		if p.inDefaults {
			p.defaultsSections++
		}
		return nil
	}
	equal := bytes.IndexByte(trimmed, '=')
	if equal < 0 {
		if p.inDefaults && strings.EqualFold(strings.TrimSpace(string(trimmed)), "inventory") {
			return errors.New("malformed inventory directive")
		}
		return nil
	}
	key := strings.TrimSpace(string(trimmed[:equal]))
	if !p.inDefaults || !strings.EqualFold(key, "inventory") {
		return nil
	}
	literal, value, locator, relativeStart, relativeEnd, err := parseInventoryValue(p.configPath, trimmed, equal+1)
	if err != nil {
		return err
	}
	p.assignments++
	p.directive = AnsibleInventoryDirective{
		Literal: literal, Value: value, Path: locator,
		ValueStart: lineStart + leading + relativeStart,
		ValueEnd:   lineStart + leading + relativeEnd,
	}
	return nil
}

func (p *ansibleInventoryDirectiveParser) result() (AnsibleInventoryDirective, error) {
	if p.defaultsSections != 1 || p.assignments > 1 {
		return AnsibleInventoryDirective{}, errors.New("ambiguous ansible inventory directive")
	}
	if p.assignments == 0 {
		return AnsibleInventoryDirective{}, ErrAnsibleInventoryDirectiveMissing
	}
	if p.directive.ValueStart < 0 || p.directive.ValueStart >= p.directive.ValueEnd || p.directive.ValueEnd > len(p.body) {
		return AnsibleInventoryDirective{}, errors.New("invalid ansible inventory directive span")
	}
	return p.directive, nil
}

func ReadAnsibleInventoryDirective(path string) (AnsibleInventoryDirective, error) {
	file, err := os.Open(path)
	if err != nil {
		return AnsibleInventoryDirective{}, err
	}
	body, readErr := io.ReadAll(io.LimitReader(file, maxInventoryAnsibleConfigSize+1))
	closeErr := file.Close()
	if readErr != nil {
		return AnsibleInventoryDirective{}, readErr
	}
	if closeErr != nil {
		return AnsibleInventoryDirective{}, closeErr
	}
	if len(body) > maxInventoryAnsibleConfigSize {
		return AnsibleInventoryDirective{}, errors.New("ansible config is too large")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return AnsibleInventoryDirective{}, err
	}
	return ParseAnsibleInventoryDirective(filepath.Clean(absolute), body)
}

func parseInventorySection(line []byte) (string, bool) {
	close := bytes.IndexByte(line, ']')
	if close <= 1 {
		return "", false
	}
	name := strings.TrimSpace(string(line[1:close]))
	if name == "" || strings.ContainsAny(name, "[]\r\n\x00") {
		return "", false
	}
	rest := trimInventorySpaceOnly(line[close+1:])
	if len(rest) > 0 && rest[0] != '#' && rest[0] != ';' {
		return "", false
	}
	return name, true
}

func parseInventoryValue(configPath string, line []byte, valueOffset int) (string, string, string, int, int, error) {
	if valueOffset < 0 || valueOffset > len(line) {
		return "", "", "", 0, 0, errors.New("invalid inventory value offset")
	}
	leading := 0
	for valueOffset+leading < len(line) && inventorySpace(line[valueOffset+leading]) {
		leading++
	}
	start := valueOffset + leading
	if start >= len(line) || line[start] == '#' || line[start] == ';' {
		return "", "", "", 0, 0, errors.New("missing inventory literal")
	}
	end := start
	valueEnd := start
	var value []byte
	if line[start] == '\'' || line[start] == '"' {
		quote := line[start]
		end = start + 1
		for end < len(line) && line[end] != quote {
			end++
		}
		if end >= len(line) {
			return "", "", "", 0, 0, errors.New("unterminated inventory quote")
		}
		value = line[start+1 : end]
		valueEnd = end + 1
		end++
	} else {
		for end < len(line) && !inventorySpace(line[end]) {
			end++
		}
		value = line[start:end]
		valueEnd = end
		if bytes.ContainsAny(value, "'\"") {
			return "", "", "", 0, 0, errors.New("unsupported inventory quote")
		}
	}
	rest := trimInventorySpaceOnly(line[end:])
	if len(rest) > 0 && rest[0] != '#' && rest[0] != ';' {
		return "", "", "", 0, 0, errors.New("unsupported inventory suffix")
	}
	if !validInventoryLiteral(value) {
		return "", "", "", 0, 0, errors.New("unsupported inventory literal")
	}
	locator, err := resolveInventoryLiteral(configPath, string(value))
	if err != nil {
		return "", "", "", 0, 0, err
	}
	return string(line[start:valueEnd]), string(value), locator, start, valueEnd, nil
}

func validInventoryLiteral(value []byte) bool {
	if len(value) == 0 || !utf8.Valid(value) || bytes.ContainsAny(value, "\x00\r\n\\,$") {
		return false
	}
	text := string(value)
	return !strings.Contains(text, "{{") && !strings.Contains(text, "}}") &&
		!strings.Contains(text, "{%") && !strings.Contains(text, "%}") &&
		!strings.Contains(text, "{#") && !strings.Contains(text, "#}") &&
		!strings.Contains(text, "%(")
}

func resolveInventoryLiteral(configPath, literal string) (string, error) {
	path := literal
	if !filepath.IsAbs(path) {
		path = filepath.Join(filepath.Dir(configPath), path)
	}
	absolute, err := filepath.Abs(path)
	if err != nil || strings.ContainsAny(absolute, "\x00\r\n") {
		return "", errors.New("invalid inventory path")
	}
	return filepath.Clean(absolute), nil
}

func trimInventorySpace(line []byte) ([]byte, int) {
	start := 0
	for start < len(line) && inventorySpace(line[start]) {
		start++
	}
	end := len(line)
	for end > start && inventorySpace(line[end-1]) {
		end--
	}
	return line[start:end], start
}

func trimInventorySpaceOnly(line []byte) []byte {
	trimmed, _ := trimInventorySpace(line)
	return trimmed
}

func inventorySpace(value byte) bool { return value == ' ' || value == '\t' }

type ManagedInventoryScriptKind string

const (
	ManagedInventoryScriptPig    ManagedInventoryScriptKind = "pig"
	ManagedInventoryScriptLegacy ManagedInventoryScriptKind = "legacy"
)

const managedInventoryQuerySuffix = " -AXtwc 'SELECT text FROM pigsty.inventory;'"

// ManagedInventoryScriptShape recognizes only the bounded forms owned by Pig
// and the legacy bin/inventory_cmdb script. It never evaluates or returns the
// connection token.
func ManagedInventoryScriptShape(body []byte) (ManagedInventoryScriptKind, bool) {
	kind, _, ok := managedInventoryScriptToken(body)
	return kind, ok
}

// ManagedInventoryScriptHasPassword inspects only a recognized managed shape
// and never returns the connection literal. Unmanaged input returns false and
// must still be rejected separately by ManagedInventoryScriptShape.
func ManagedInventoryScriptHasPassword(body []byte) bool {
	kind, token, ok := managedInventoryScriptToken(body)
	if !ok {
		return false
	}
	if kind == ManagedInventoryScriptPig {
		token = strings.ReplaceAll(token[1:len(token)-1], "'\\''", "'")
	}
	return InventoryConnectionHasPassword(token)
}

func managedInventoryScriptToken(body []byte) (ManagedInventoryScriptKind, string, bool) {
	if !utf8.Valid(body) || bytes.IndexByte(body, 0) >= 0 {
		return "", "", false
	}
	lines := strings.Split(string(body), "\n")
	if len(lines) != 3 || lines[0] != "#!/bin/bash" || lines[2] != "" {
		return "", "", false
	}
	command := lines[1]
	if strings.HasPrefix(command, "exec psql ") && strings.HasSuffix(command, managedInventoryQuerySuffix) {
		token := strings.TrimSuffix(strings.TrimPrefix(command, "exec psql "), managedInventoryQuerySuffix)
		if validPigShellToken(token) {
			return ManagedInventoryScriptPig, token, true
		}
	}
	if strings.HasPrefix(command, "psql ") && strings.HasSuffix(command, managedInventoryQuerySuffix) {
		token := strings.TrimSuffix(strings.TrimPrefix(command, "psql "), managedInventoryQuerySuffix)
		if validLegacyConnectionToken(token) {
			return ManagedInventoryScriptLegacy, token, true
		}
	}
	return "", "", false
}

// InventoryConnectionHasPassword recognizes inline URI and libpq conninfo
// password parameters without returning or logging the literal.
func InventoryConnectionHasPassword(value string) bool {
	trimmed := strings.TrimSpace(value)
	if parsed, err := url.Parse(trimmed); err == nil && parsed.Scheme != "" {
		if parsed.User != nil {
			if _, exists := parsed.User.Password(); exists {
				return true
			}
		}
		for key := range parsed.Query() {
			if strings.EqualFold(key, "password") {
				return true
			}
		}
	}
	lower := strings.ToLower(trimmed)
	for offset := 0; ; {
		index := strings.Index(lower[offset:], "password")
		if index < 0 {
			return false
		}
		index += offset
		beforeOK := index == 0 || unicode.IsSpace(rune(lower[index-1]))
		after := strings.TrimLeft(lower[index+len("password"):], " \t")
		if beforeOK && strings.HasPrefix(after, "=") {
			return true
		}
		offset = index + len("password")
	}
}

func validPigShellToken(token string) bool {
	if len(token) <= 2 || token[0] != '\'' || token[len(token)-1] != '\'' || strings.ContainsAny(token, "\x00\r\n") {
		return false
	}
	for index := 1; index < len(token)-1; {
		if token[index] != '\'' {
			index++
			continue
		}
		if index+3 >= len(token) || token[index:index+4] != "'\\''" {
			return false
		}
		index += 4
	}
	return true
}

func validLegacyConnectionToken(token string) bool {
	if token == "" || strings.ContainsAny(token, "\x00\r\n;|&<>`'\"\\$") {
		return false
	}
	for _, value := range token {
		if unicode.IsSpace(value) || unicode.IsControl(value) {
			return false
		}
	}
	return true
}

func resolveInventorySource(explicit, pigstyHome string) InventorySourceDescriptor {
	descriptor := InventorySourceDescriptor{Home: pigstyHome, Tier: InventorySourceUnconfigured, Type: InventorySourceMissing}
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	if value := strings.TrimSpace(explicit); value != "" {
		return inspectInventoryCandidate(descriptor, value, InventorySourceFlag, cwd, "")
	}
	if value := strings.TrimSpace(os.Getenv("PIGSTY_INVENTORY")); value != "" {
		return inspectInventoryCandidate(descriptor, value, InventorySourceEnvironment, cwd, "")
	}
	if value := strings.TrimSpace(viperString("inventory")); value != "" {
		expanded := expandHomePath(value)
		if !filepath.IsAbs(expanded) && pigstyHome == "" {
			descriptor.Tier = InventorySourceConfig
			descriptor.Type = InventorySourceUnsupported
			descriptor.OriginPath = ConfigFile
			descriptor.Findings = []string{"SOURCE_CONFIG_HOME_REQUIRED"}
			return descriptor
		}
		return inspectInventoryCandidate(descriptor, expanded, InventorySourceConfig, pigstyHome, ConfigFile)
	}
	if pigstyHome == "" {
		descriptor.Findings = []string{"SOURCE_NOT_CONFIGURED"}
		return descriptor
	}
	configPath := filepath.Join(pigstyHome, "ansible.cfg")
	directive, err := ReadAnsibleInventoryDirective(configPath)
	if err == nil {
		return inspectResolvedInventoryPath(descriptor, directive.Path, InventorySourceAnsibleConfig, configPath)
	}
	if err != nil && !errors.Is(err, ErrAnsibleInventoryDirectiveMissing) && !errors.Is(err, os.ErrNotExist) {
		descriptor.Tier = InventorySourceAnsibleConfig
		descriptor.Type = InventorySourceUnsupported
		descriptor.OriginPath = configPath
		descriptor.Findings = []string{"SOURCE_ANSIBLE_CONFIG_INVALID"}
		return descriptor
	}
	return inspectInventoryCandidate(descriptor, "pigsty.yml", InventorySourceDefault, pigstyHome, configPath)
}

// viperString is a small seam for source-resolution tests and keeps all Viper
// access inside the config package.
var viperString = func(key string) string { return viper.GetString(key) }

func inspectInventoryCandidate(base InventorySourceDescriptor, value string, tier InventorySourceTier, pathBase, origin string) InventorySourceDescriptor {
	path := resolveInventoryCandidate(value, pathBase)
	return inspectResolvedInventoryPath(base, path, tier, origin)
}

func inspectResolvedInventoryPath(base InventorySourceDescriptor, path string, tier InventorySourceTier, origin string) InventorySourceDescriptor {
	base.Tier = tier
	base.Path = path
	base.OriginPath = origin
	if path == "" {
		base.Type = InventorySourceUnsupported
		base.Findings = []string{"SOURCE_PATH_INVALID"}
		return base
	}
	before, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		base.Type = InventorySourceMissing
		base.Findings = []string{"SOURCE_NOT_FOUND"}
		return base
	}
	if err != nil {
		base.Type = InventorySourceUnsupported
		base.Findings = []string{"SOURCE_INSPECTION_FAILED"}
		return base
	}
	base.Exists = true
	base.Symlink = before.Mode()&os.ModeSymlink != 0
	info := before
	if base.Symlink {
		info, err = os.Stat(path)
		if err != nil {
			base.Type = InventorySourceUnsupported
			base.Findings = []string{"SOURCE_SYMLINK_INVALID"}
			return base
		}
	}
	base.Executable = info.Mode().Perm()&0111 != 0
	if file, openErr := os.Open(path); openErr == nil {
		base.Readable = true
		_ = file.Close()
	}
	classifyInventorySource(&base, info, path, tier)
	if !base.Readable {
		base.Findings = append(base.Findings, "SOURCE_UNREADABLE")
	}
	return base
}

func classifyInventorySource(base *InventorySourceDescriptor, info os.FileInfo, path string, tier InventorySourceTier) {
	switch {
	case info.IsDir():
		base.Type = InventorySourceDirectory
		base.Findings = append(base.Findings, "SOURCE_DIRECTORY_UNSUPPORTED")
	case !info.Mode().IsRegular():
		base.Type = InventorySourceUnsupported
		base.Findings = append(base.Findings, "SOURCE_TYPE_UNSUPPORTED")
	case base.Executable:
		base.Type = InventorySourceExecutableDynamic
		inspectManagedInventoryScript(base, path, tier)
		if !base.Managed {
			base.Findings = append(base.Findings, "SOURCE_DYNAMIC_UNSUPPORTED")
		}
	default:
		base.Type = InventorySourceStaticYAML
		if base.Readable {
			base.StaticCommands = []string{"list", "show", "validate", "diff"}
			if !base.Symlink {
				base.StaticCommands = append(base.StaticCommands, "edit")
			} else {
				base.Findings = append(base.Findings, "SOURCE_SYMLINK_EDIT_UNSUPPORTED")
			}
		}
	}
}

func inspectManagedInventoryScript(base *InventorySourceDescriptor, path string, tier InventorySourceTier) {
	managedPath := tier == InventorySourceAnsibleConfig && base.Home != "" && path == filepath.Join(base.Home, "inventory.sh")
	if base.Symlink || !managedPath {
		return
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return
	}
	if _, managed := ManagedInventoryScriptShape(body); !managed {
		return
	}
	base.Type = InventorySourceManagedCMDB
	base.Managed = true
	base.Findings = append(base.Findings, "SOURCE_MANAGED_CMDB_ACTIVE")
	if ManagedInventoryScriptHasPassword(body) {
		base.Findings = append(base.Findings, "SOURCE_MANAGED_PASSWORD_LITERAL")
	}
}
