package inventorycli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"pig/internal/cmdb"
	"pig/internal/output"
	"pig/inventory"
)

func preparePigstyDeclarations(path string) (cmdb.Declarations, *output.Result) {
	prepared, failure := preparePigstyInventory(path, false)
	if failure != nil {
		return cmdb.Declarations{}, failure
	}
	return prepared.Declarations, nil
}

func TestPreparePigstyDeclarationsUsesExistingShape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pigsty.yml")
	source := []byte(`all:
  vars:
    region: default
    enabled: true
    admin_ip: 10.10.10.10
  children:
    demo:
      hosts:
        10.10.10.10:
          node_seq: 1
          labels: [a, b]
      vars:
        cluster_name: demo
`)
	if err := os.WriteFile(path, source, 0o600); err != nil {
		t.Fatal(err)
	}
	got, failure := preparePigstyDeclarations(path)
	if failure != nil {
		t.Fatalf("prepare failure: %s (%s)", failure.Message, failure.Detail)
	}
	if len(got.Globals) != 3 || len(got.Groups) != 1 || len(got.Groups[0].Hosts) != 1 || len(got.Groups[0].Hosts[0].Vars) != 2 {
		t.Fatalf("declarations = %+v", got)
	}
	if got.Groups[0].Name != "demo" || got.Groups[0].Hosts[0].IP != "10.10.10.10" {
		t.Fatalf("ordered declarations = %+v", got.Groups[0])
	}
}

func TestPreparePigstyDeclarationsRejectsNestedGroups(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested.yml")
	source := []byte(`all:
  vars: {}
  children:
    parent:
      hosts: {}
      children:
        child:
          hosts: {}
`)
	if err := os.WriteFile(path, source, 0o600); err != nil {
		t.Fatal(err)
	}
	_, failure := preparePigstyDeclarations(path)
	if failure == nil {
		t.Fatal("nested groups were accepted")
	}
}

func TestPreparePigstyDeclarationsRejectsRootLevelGroups(t *testing.T) {
	source := []byte(`all:
  vars: {}
  children: {}
outside:
  hosts:
    192.0.2.1: {}
`)
	doc, err := inventory.Parse("root-group.yml", source)
	if err != nil {
		t.Fatal(err)
	}
	_, failure := pigstyDeclarationsFromDocument(doc, "root-group.yml")
	if failure == nil {
		t.Fatal("root-level group was silently omitted")
	}
	data, ok := failure.Data.(*CMDBFailureData)
	if !ok || data.CMDBCode != "CMDB_ROOT_GROUP_UNSUPPORTED" {
		t.Fatalf("root-level group failure = %+v", failure)
	}
}

func TestPigstyDeclarationsUseAnsibleScalarSemantics(t *testing.T) {
	doc, err := inventory.Parse("ansible-scalars.yml", []byte(`all:
  vars:
    enabled: yes
    duration: 1:20
    yaml12_octal: 0o755
  children: {}
`))
	if err != nil {
		t.Fatal(err)
	}
	declarations, failure := pigstyDeclarationsFromDocument(doc, "ansible-scalars.yml")
	if failure != nil {
		t.Fatalf("declaration failure: %+v", failure)
	}
	values := jsonFieldsObject(declarations.Globals)
	if values["enabled"] != true || values["duration"] != float64(80) || values["yaml12_octal"] != "0o755" {
		t.Fatalf("CMDB scalar projection = %#v", values)
	}
}

func TestPlainInventoryValueRejectsJSONKeyCoercionCollision(t *testing.T) {
	value := &inventory.Value{Kind: inventory.MappingValue, Mapping: []inventory.Entry{
		{Key: inventory.Scalar{Kind: inventory.BoolScalar, Value: true}, Value: inventory.Value{Kind: inventory.NullValue}},
		{Key: inventory.Scalar{Kind: inventory.StringScalar, Value: "true"}, Value: inventory.Value{Kind: inventory.NullValue}},
	}}
	if _, err := plainInventoryValue(value); err == nil || !strings.Contains(err.Error(), "collide") {
		t.Fatalf("coercion collision error = %v", err)
	}
}

func TestCanonicalCMDBYAMLIsBlockAndSemanticallyStable(t *testing.T) {
	raw := []byte(`{"all":{"children":{"demo":{"hosts":{"10.10.10.10":{"note":"yes","seq":1}},"vars":{}},"empty":{"hosts":null,"vars":null}},"vars":{"region":"china","enabled":true}}}`)
	canonical, err := cmdb.CanonicalRawConfigJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	body, err := canonicalCMDBYAML(canonical)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.HasPrefix(body, []byte("{")) || !bytes.Contains(body, []byte("all:\n")) || !bytes.Contains(body, []byte(`note: "yes"`)) {
		t.Fatalf("unexpected canonical YAML:\n%s", body)
	}
	if !bytes.Contains(body, []byte("empty:\n      hosts: {}\n      vars: {}")) {
		t.Fatalf("null declaration containers were not normalized:\n%s", body)
	}
	doc, err := inventory.Parse("dump.yml", body)
	if err != nil || doc.Inventory() == nil || doc.Inventory().All == nil {
		t.Fatalf("dump did not parse: %v", err)
	}
}

func TestReadPigstyOSDefaultsUsesFallbackAndArchAlias(t *testing.T) {
	home := t.TempDir()
	directory := filepath.Join(home, "roles", "node_id", "vars")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	source := []byte(`repo_packages_default: [node-bootstrap]
node_packages_default: [bash, curl]
package_map:
  node-bootstrap: "ansible python3"
  pgsql: "postgresql$v"
`)
	if err := os.WriteFile(filepath.Join(directory, "el9.x86_64.yml"), source, 0o600); err != nil {
		t.Fatal(err)
	}
	defaults, warning := readPigstyOSDefaults(home, "el10", "amd64")
	if warning != "" || len(defaults.Values) != 2 || len(defaults.Packages) != 2 {
		t.Fatalf("defaults = %+v, warning=%q", defaults, warning)
	}
	keys := map[string]bool{}
	for _, field := range defaults.Values {
		keys[field.Key] = true
	}
	if !keys["repo_packages"] || !keys["node_packages"] {
		t.Fatalf("mapped keys = %v", keys)
	}
}

func TestReadPigstyOSDefaultsUsesAnsibleScalarSemantics(t *testing.T) {
	home := t.TempDir()
	directory := filepath.Join(home, "roles", "node_id", "vars")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	source := []byte("repo_packages_default: yes\npackage_map:\n  duration: 1:20\n")
	if err := os.WriteFile(filepath.Join(directory, "el9.x86_64.yml"), source, 0o600); err != nil {
		t.Fatal(err)
	}
	defaults, warning := readPigstyOSDefaults(home, "el9", "x86_64")
	if warning != "" || len(defaults.Values) != 1 || string(defaults.Values[0].Value) != "true" ||
		len(defaults.Packages) != 1 || defaults.Packages[0].Value != "80" {
		t.Fatalf("defaults = %+v, warning=%q", defaults, warning)
	}
}

func TestCMDBLoadDefaultsPreflightWarnsOnlyForChangedPlan(t *testing.T) {
	data := &InventoryCMDBData{Changed: true}
	preflightPigstyOSDefaults(data, t.TempDir(), "el9", "amd64")
	preflightPigstyOSDefaults(data, t.TempDir(), "el9", "amd64")
	if !reflect.DeepEqual(data.Warnings, []string{"CMDB_DEFAULTS_UNAVAILABLE"}) {
		t.Fatalf("warnings = %v", data.Warnings)
	}

	noop := &InventoryCMDBData{Noop: true}
	preflightPigstyOSDefaults(noop, t.TempDir(), "el9", "amd64")
	if len(noop.Warnings) != 0 {
		t.Fatalf("no-op warnings = %v", noop.Warnings)
	}
}

func TestCMDBResultDoesNotCarryValues(t *testing.T) {
	data := &InventoryCMDBData{
		Kind: InventoryCMDBLoadKind, Operation: "load", ConnectionSource: cmdb.ConnectionURLSource,
		Globals: 10, Groups: 2, Hosts: 3, Warnings: []string{"CMDB_DEFAULTS_FAILED"},
	}
	text := data.Text()
	for _, forbidden := range []string{"password", "postgres://", "10.10.10.10", "region"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("safe result contains %q: %s", forbidden, text)
		}
	}
}

func TestExistingPigstyCMDBCorpusRepresentability(t *testing.T) {
	home := os.Getenv("PIGSTY_HOME")
	if home == "" {
		home = filepath.Clean(filepath.Join("..", "..", "..", "pigsty"))
	}
	root := filepath.Join(home, "conf")
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		t.Skip("adjacent Pigsty conf corpus is unavailable")
	}
	var files []string
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && (strings.HasSuffix(path, ".yml") || strings.HasSuffix(path, ".yaml")) {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	if len(files) < 40 {
		t.Fatalf("unexpectedly small Pigsty conf corpus: %d", len(files))
	}
	python, pythonErr := exec.LookPath("python3")
	if pythonErr == nil {
		pythonErr = exec.Command(python, "-c", "import yaml").Run()
	}
	loader := filepath.Join(home, "bin", "inventory_load")
	for _, path := range files {
		t.Run(strings.TrimPrefix(path, root+string(filepath.Separator)), func(t *testing.T) {
			declarations, failure := preparePigstyDeclarations(path)
			if failure != nil {
				t.Fatalf("%s: %s (%s)", failure.Message, failure.Detail, failure.String())
			}
			if pythonErr != nil {
				return
			}
			want, err := pythonLoaderProjection(python, loader, path)
			if err != nil {
				t.Fatal(err)
			}
			got := declarationProjection(declarations)
			if !reflect.DeepEqual(got, want) {
				t.Fatal("Go declaration projection differs from inventory_load parse semantics")
			}
		})
	}
}

const pythonProjection = `
import json, runpy, sys
m = runpy.run_path(sys.argv[1])
c = m['parse_conf'](sys.argv[2])
a = c.get('all') or {}
out = {'all': {'children': {}, 'vars': a.get('vars') or {}}}
for name, group in (a.get('children') or {}).items():
    out['all']['children'][name] = {
        'hosts': group.get('hosts') or {},
        'vars': group.get('vars') or {},
    }
print(json.dumps(out, separators=(',', ':'), sort_keys=True))
`

func pythonLoaderProjection(python, loader, path string) (any, error) {
	body, err := exec.Command(python, "-c", pythonProjection, loader, path).Output()
	if err != nil {
		return nil, err
	}
	var value any
	err = json.Unmarshal(body, &value)
	return value, err
}

func declarationProjection(declarations cmdb.Declarations) any {
	children := make(map[string]any, len(declarations.Groups))
	for _, group := range declarations.Groups {
		hosts := make(map[string]any, len(group.Hosts))
		for _, host := range group.Hosts {
			hosts[host.IP] = jsonFieldsObject(host.Vars)
		}
		children[group.Name] = map[string]any{"hosts": hosts, "vars": jsonFieldsObject(group.Vars)}
	}
	return map[string]any{"all": map[string]any{"children": children, "vars": jsonFieldsObject(declarations.Globals)}}
}

func jsonFieldsObject(fields []cmdb.JSONField) map[string]any {
	result := make(map[string]any, len(fields))
	for _, field := range fields {
		var value any
		if err := json.Unmarshal(field.Value, &value); err != nil {
			panic(err)
		}
		result[field.Key] = value
	}
	return result
}

func TestCMDBExportAmbiguousStringsSemanticRoundtrip(t *testing.T) {
	values := map[string]any{"duration": "1:30", "octal": "010", "boolean": "yes", "date": "2001-12-15", "timestamp": "2001-1-5 2:59:43 -5", "nested": []any{"1:30", map[string]any{"010": "1:30"}}}
	raw, err := json.Marshal(map[string]any{"all": map[string]any{"vars": values, "children": map[string]any{}}})
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := cmdb.CanonicalRawConfigJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	body, err := canonicalCMDBYAML(canonical)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := yaml.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	if got := decoded["all"].(map[string]any)["vars"]; !reflect.DeepEqual(got, values) {
		t.Fatalf("CMDB YAML changed string types: %#v", got)
	}
	doc, err := inventory.Parse("cmdb-export.yml", body)
	if err != nil {
		t.Fatal(err)
	}
	declarations, failure := pigstyDeclarationsFromDocument(doc, "cmdb-export.yml")
	if failure != nil {
		t.Fatal(failure)
	}
	if got := jsonFieldsObject(declarations.Globals); !reflect.DeepEqual(got, values) {
		t.Fatalf("CMDB semantic roundtrip: got=%#v want=%#v YAML=%s", got, values, body)
	}
}
