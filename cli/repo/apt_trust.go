package repo

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"pig/internal/config"
	"slices"
	"strings"
)

// APT merges binary/source entries and components by URI and suite. These
// release-level trust settings must agree even across different source files.
var aptTrustOptions = []string{"trusted", "signed-by", "allow-insecure", "allow-weak", "allow-downgrade-to-insecure"}

type aptSourceKey struct{ uri, suite string }

type aptSource struct {
	key   aptSourceKey
	trust map[string]string
	path  string
}

func (m *Manager) selectedAPTRepositories(modules []string) []*Repository {
	var repositories []*Repository
	if m.OsType != config.DistroDEB {
		return repositories
	}
	for _, module := range modules {
		for _, name := range m.Module[module] {
			r := m.Map[name]
			if r != nil && r.Available(config.OSCode, config.OSArch) && !slices.Contains(repositories, r) {
				repositories = append(repositories, r)
			}
		}
	}
	return repositories
}

// Capture explicit metadata before implicit key preparation adds signed-by.
func (m *Manager) explicitAPTPolicies(modules []string) map[*Repository]map[string]bool {
	policies := make(map[*Repository]map[string]bool)
	for _, r := range m.selectedAPTRepositories(modules) {
		policy := make(map[string]bool)
		for _, option := range aptTrustOptions {
			policy[option] = r.explicitAPTTrust[option] || (r.Meta[option] != "" && (option != "trusted" || r.Meta[option] != "yes"))
		}
		policies[r] = policy
	}
	return policies
}

// Reconcile only the generated definitions. Retained files are never rewritten
// or removed to implement an implicit key fallback or an additive operation.
func (m *Manager) reconcileAPTTrust(modules []string, explicit map[*Repository]map[string]bool, replace bool) (bool, error) {
	if m.OsType != config.DistroDEB {
		return false, nil
	}
	groups := make(map[aptSourceKey][]*Repository)
	var order []aptSourceKey
	for _, r := range m.selectedAPTRepositories(modules) {
		sources, err := parseAPTList(r.Content(m.Region))
		if err != nil || len(sources) != 1 {
			return false, fmt.Errorf("invalid APT definition for repository %q", r.Name)
		}
		key := sources[0].key
		if _, exists := groups[key]; !exists {
			order = append(order, key)
		}
		groups[key] = append(groups[key], r)
	}
	var existing []aptSource
	if !replace {
		var err error
		existing, err = m.readAPTSources()
		if err != nil {
			return false, err
		}
	}
	targets := make(map[string]bool)
	for _, module := range modules {
		if _, exists := m.Module[module]; exists {
			targets[m.getModulePath(module)] = true
		}
	}
	inherited := false
	for _, key := range order {
		prior, retained := priorAPTSources(key, existing, targets)
		policy, err := resolveAPTGroupTrust(groups[key], prior, explicit, retained)
		if err != nil {
			return false, err
		}
		inherited = inherited || len(prior) > 0
		for _, r := range groups[key] {
			for _, option := range aptTrustOptions {
				delete(r.Meta, option)
				if value := policy[option]; value != "" {
					r.Meta[option] = value
				}
			}
		}
	}
	return inherited, nil
}

func priorAPTSources(key aptSourceKey, existing []aptSource, targets map[string]bool) ([]aptSource, bool) {
	var retained, overwritten []aptSource
	for _, source := range existing {
		if source.key != key {
			continue
		}
		if targets[source.path] {
			overwritten = append(overwritten, source)
		} else {
			retained = append(retained, source)
		}
	}
	if len(retained) > 0 {
		return retained, true
	}
	return overwritten, false
}

func resolveAPTGroupTrust(repositories []*Repository, prior []aptSource, explicit map[*Repository]map[string]bool, retained bool) (map[string]string, error) {
	policy := make(map[string]string)
	for _, r := range repositories {
		for option, value := range aptTrustPolicy(r.Meta) {
			if policy[option] == "" {
				policy[option] = value
			}
		}
	}
	if len(prior) > 0 {
		policy = maps.Clone(prior[0].trust)
		for _, source := range prior[1:] {
			if !maps.Equal(policy, source.trust) {
				return nil, fmt.Errorf("conflicting APT trust options in %s and %s; reconcile the definitions or use pig repo set to replace existing repositories", prior[0].path, source.path)
			}
		}
	}
	if err := mergeExplicitAPTTrust(policy, repositories, explicit, retained); err != nil {
		return nil, err
	}
	// Inline Deb822 keys cannot be represented by our one-line .list writer.
	if strings.ContainsAny(policy["signed-by"], " \t\r\n") {
		return nil, fmt.Errorf("cannot inherit inline APT signing key; keep this source in its .sources file or use pig repo set with a keyring path")
	}
	return policy, nil
}

func mergeExplicitAPTTrust(policy map[string]string, repositories []*Repository, explicit map[*Repository]map[string]bool, retained bool) error {
	chosen := make(map[string]string)
	for _, r := range repositories {
		desired := aptTrustPolicy(r.Meta)
		for _, option := range aptTrustOptions {
			if !explicit[r][option] {
				continue
			}
			value := desired[option]
			previous, exists := chosen[option]
			if (retained && policy[option] != value) || (exists && previous != value) {
				return fmt.Errorf("conflicting APT %s policy for repository %q; reconcile the definitions or use pig repo set to replace existing repositories", option, r.Name)
			}
			chosen[option] = value
			delete(policy, option)
			if value != "" {
				policy[option] = value
			}
		}
	}
	return nil
}

func aptTrustPolicy(options map[string]string) map[string]string {
	policy := make(map[string]string)
	for _, option := range aptTrustOptions {
		value := strings.TrimSpace(options[option])
		if value == "" {
			continue
		}
		if option == "signed-by" && !strings.Contains(value, "-----BEGIN PGP PUBLIC KEY BLOCK-----") {
			values := strings.Fields(strings.ReplaceAll(value, ",", " "))
			slices.Sort(values)
			value = strings.Join(values, ",")
		} else if option != "signed-by" {
			switch strings.ToLower(value) {
			case "yes", "true", "1", "on":
				value = "yes"
			case "no", "false", "0", "off":
				value = "no"
			}
		}
		policy[option] = value
	}
	return policy
}

func (m *Manager) readAPTSources() ([]aptSource, error) {
	paths := []string{filepath.Join(filepath.Dir(m.RepoDir), "sources.list")}
	entries, err := os.ReadDir(m.RepoDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read APT sources directory: %w", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || strings.HasPrefix(name, ".") || strings.Trim(name, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_.-") != "" {
			continue
		}
		if strings.HasSuffix(name, ".list") || strings.HasSuffix(name, ".sources") {
			paths = append(paths, filepath.Join(m.RepoDir, name))
		}
	}
	var sources []aptSource
	for _, path := range paths {
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("inspect APT source %s: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("APT source %s is not a regular file", path)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read APT source %s: %w", path, err)
		}
		var parsed []aptSource
		if strings.HasSuffix(path, ".sources") {
			parsed, err = parseAPTStanzas(string(content))
		} else {
			parsed, err = parseAPTList(string(content))
		}
		if err != nil {
			return nil, fmt.Errorf("parse APT source %s: %w", path, err)
		}
		for _, source := range parsed {
			source.path = path
			sources = append(sources, source)
		}
	}
	return sources, nil
}

func aptKey(uri, suite string) aptSourceKey {
	if !strings.HasSuffix(uri, "/") {
		uri += "/"
	}
	return aptSourceKey{uri: uri, suite: suite}
}

func parseAPTList(content string) ([]aptSource, error) {
	var sources []aptSource
	for i, line := range strings.Split(content, "\n") {
		line, _, _ = strings.Cut(line, "#")
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if fields[0] != "deb" && fields[0] != "deb-src" {
			return nil, fmt.Errorf("unsupported source type at line %d", i+1)
		}
		rest := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), fields[0]))
		options := make(map[string]string)
		if strings.HasPrefix(rest, "[") {
			end := strings.IndexByte(rest, ']')
			if end < 0 {
				return nil, fmt.Errorf("unclosed source options at line %d", i+1)
			}
			for _, field := range strings.Fields(rest[1:end]) {
				key, value, ok := strings.Cut(field, "=")
				if !ok {
					return nil, fmt.Errorf("invalid source option at line %d", i+1)
				}
				options[strings.ToLower(key)] = value
			}
			rest = rest[end+1:]
		}
		fields = strings.Fields(rest)
		if len(fields) < 2 {
			return nil, fmt.Errorf("missing source URI or suite at line %d", i+1)
		}
		sources = append(sources, aptSource{key: aptKey(fields[0], fields[1]), trust: aptTrustPolicy(options)})
	}
	return sources, nil
}

func parseAPTStanzas(content string) ([]aptSource, error) {
	var sources []aptSource
	fields := make(map[string]string)
	last := ""
	for i, line := range strings.Split(content+"\n\n", "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if strings.TrimSpace(line) == "" {
			if len(fields) != 0 {
				stanza, err := aptStanzaSources(fields)
				if err != nil {
					return nil, fmt.Errorf("stanza ending at line %d: %w", i+1, err)
				}
				sources = append(sources, stanza...)
			}
			fields = make(map[string]string)
			last = ""
			continue
		}
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			if last == "" {
				return nil, fmt.Errorf("unexpected continuation at line %d", i+1)
			}
			fields[last] += "\n" + strings.TrimSpace(line)
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf("invalid field at line %d", i+1)
		}
		last = strings.ToLower(strings.TrimSpace(key))
		fields[last] = strings.TrimSpace(value)
	}
	return sources, nil
}

func aptStanzaSources(fields map[string]string) ([]aptSource, error) {
	if strings.EqualFold(fields["enabled"], "no") {
		return nil, nil
	}
	types, uris, suites := strings.Fields(fields["types"]), strings.Fields(fields["uris"]), strings.Fields(fields["suites"])
	if len(types) == 0 || len(uris) == 0 || len(suites) == 0 {
		return nil, fmt.Errorf("missing Types, URIs, or Suites")
	}
	for _, kind := range types {
		if kind != "deb" && kind != "deb-src" {
			return nil, fmt.Errorf("unsupported source type")
		}
	}
	var sources []aptSource
	for _, uri := range uris {
		for _, suite := range suites {
			sources = append(sources, aptSource{key: aptKey(uri, suite), trust: aptTrustPolicy(fields)})
		}
	}
	return sources, nil
}
