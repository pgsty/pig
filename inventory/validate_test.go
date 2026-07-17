package inventory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

type parityManifest struct {
	Schema       string       `json:"schema"`
	LegacySHA256 string       `json:"legacy_sha256"`
	Cases        []parityCase `json:"cases"`
}

type parityCase struct {
	Fixture       string   `json:"fixture"`
	LegacySuccess bool     `json:"legacy_success"`
	NativeSuccess bool     `json:"native_success"`
	NativeCodes   []string `json:"native_codes"`
	Divergences   []string `json:"divergences"`
}

type divergenceRegistry struct {
	Schema  string            `json:"schema"`
	Entries []divergenceEntry `json:"entries"`
}

type divergenceEntry struct {
	RuleID       string `json:"rule_id"`
	Fixture      string `json:"fixture"`
	LegacyResult string `json:"legacy_result"`
	NativeResult string `json:"native_result"`
	Rationale    string `json:"rationale"`
}

func TestPinnedLegacyParityAndDivergenceRegistry(t *testing.T) {
	root := filepath.Join("testdata", "validate")
	manifest := readJSONFixture[parityManifest](t, filepath.Join(root, "parity.json"))
	registry := readJSONFixture[divergenceRegistry](t, filepath.Join(root, "divergences.json"))
	if manifest.Schema != "pig.sty.validate.parity/v1" || registry.Schema != "pig.sty.validate.divergences/v1" {
		t.Fatalf("unexpected fixture schemas: %q, %q", manifest.Schema, registry.Schema)
	}

	legacyPath := filepath.Join(root, "legacy_validate.py")
	legacySource, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(legacySource)
	if got := hex.EncodeToString(digest[:]); got != manifest.LegacySHA256 {
		t.Fatalf("pinned legacy SHA-256 = %s, want %s", got, manifest.LegacySHA256)
	}
	if lines := strings.Count(string(legacySource), "\n"); lines != 550 {
		t.Fatalf("pinned legacy source lines = %d, want 550", lines)
	}

	registered := make(map[string]divergenceEntry, len(registry.Entries))
	for _, entry := range registry.Entries {
		if entry.RuleID == "" || entry.Fixture == "" || entry.LegacyResult == "" || entry.NativeResult == "" || entry.Rationale == "" {
			t.Fatalf("incomplete divergence entry: %+v", entry)
		}
		if _, duplicate := registered[entry.RuleID]; duplicate {
			t.Fatalf("duplicate divergence rule ID %q", entry.RuleID)
		}
		registered[entry.RuleID] = entry
	}

	python, legacyAvailable := legacyPython(t)
	referenced := make(map[string]bool)
	for _, test := range manifest.Cases {
		t.Run(test.Fixture, func(t *testing.T) {
			path := filepath.Join(root, test.Fixture)
			doc, err := Load(path)
			if err != nil {
				t.Fatal(err)
			}
			diagnostics := doc.Validate(YAMLRules, AnsibleRules, PigstyRules, CompleteInventoryRules)
			nativeSuccess := !HasErrors(diagnostics)
			if nativeSuccess != test.NativeSuccess {
				t.Fatalf("native success = %v, want %v; diagnostics=%+v", nativeSuccess, test.NativeSuccess, diagnostics)
			}
			if got := diagnosticCodes(diagnostics); !reflect.DeepEqual(got, test.NativeCodes) {
				t.Fatalf("native codes = %v, want %v", got, test.NativeCodes)
			}

			outcomeMismatch := test.LegacySuccess != test.NativeSuccess
			if outcomeMismatch != (len(test.Divergences) > 0) {
				t.Fatalf("legacy/native mismatch=%v but divergences=%v", outcomeMismatch, test.Divergences)
			}
			for _, ruleID := range test.Divergences {
				entry, exists := registered[ruleID]
				if !exists {
					t.Fatalf("unregistered divergence %q", ruleID)
				}
				if entry.Fixture != test.Fixture {
					t.Fatalf("divergence %q fixture = %q, want %q", ruleID, entry.Fixture, test.Fixture)
				}
				referenced[ruleID] = true
			}

			if legacyAvailable {
				command := exec.Command(python, legacyPath, path)
				command.Stdout = nil
				command.Stderr = nil
				err := command.Run()
				legacySuccess := err == nil
				var exitError *exec.ExitError
				if err != nil && !errors.As(err, &exitError) {
					t.Fatalf("execute pinned legacy validator: %v", err)
				}
				if legacySuccess != test.LegacySuccess {
					t.Fatalf("legacy success = %v, want %v", legacySuccess, test.LegacySuccess)
				}
			}
		})
	}
	for ruleID := range registered {
		if !referenced[ruleID] {
			t.Errorf("divergence %q is not exercised by the parity manifest", ruleID)
		}
	}
}

func TestDiagnosticsHaveStableIDsPathsAndRanges(t *testing.T) {
	path := filepath.Join("testdata", "validate", "invalid-declarations.yml")
	first, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	left := first.Validate(YAMLRules, AnsibleRules, PigstyRules, CompleteInventoryRules)
	right := second.Validate(YAMLRules, AnsibleRules, PigstyRules, CompleteInventoryRules)
	if !reflect.DeepEqual(left, right) {
		t.Fatalf("diagnostics changed across identical parses:\nleft=%+v\nright=%+v", left, right)
	}
	for _, diagnostic := range left {
		if diagnostic.Code == "" || diagnostic.Path.String() == "/" {
			t.Fatalf("diagnostic has no stable ID/path: %+v", diagnostic)
		}
		if diagnostic.Range.Start.Line < 1 || diagnostic.Range.End.Line < diagnostic.Range.Start.Line {
			t.Fatalf("diagnostic has invalid range: %+v", diagnostic)
		}
	}
	want := struct {
		path                   string
		startLine, startColumn int
		endLine, endColumn     int
	}{path: "/all/children/pgsql/vars/pg_users/0/name", startLine: 16, startColumn: 21, endLine: 16, endColumn: 23}
	for _, diagnostic := range left {
		if diagnostic.Code != "PIG_USER_FIELD_TYPE" {
			continue
		}
		if diagnostic.Path.String() != want.path || diagnostic.Range.Start.Line != want.startLine ||
			diagnostic.Range.Start.Column != want.startColumn || diagnostic.Range.End.Line != want.endLine ||
			diagnostic.Range.End.Column != want.endColumn {
			t.Fatalf("stable diagnostic changed: %+v", diagnostic)
		}
		return
	}
	t.Fatal("PIG_USER_FIELD_TYPE diagnostic not found")
}

func TestOpaqueDeclarationsAreInformational(t *testing.T) {
	doc, err := Load(filepath.Join("testdata", "validate", "opaque.yml"))
	if err != nil {
		t.Fatal(err)
	}
	diagnostics := doc.Validate(YAMLRules, AnsibleRules, PigstyRules, CompleteInventoryRules)
	if HasErrors(diagnostics) || len(diagnostics) != 3 {
		t.Fatalf("opaque diagnostics = %+v", diagnostics)
	}
	wantPaths := []string{
		"/all/children/pgsql/hosts/10.0.0.1/pg_seq",
		"/all/children/pgsql/vars/pg_users",
		"/all/vars/admin_ip",
	}
	for index, diagnostic := range diagnostics {
		if diagnostic.Code != "UNVERIFIABLE" || diagnostic.Severity != SeverityInfo || diagnostic.Path.String() != wantPaths[index] {
			t.Fatalf("opaque diagnostic %d = %+v", index, diagnostic)
		}
	}
}

func TestOpaqueHostVarsDoNotProduceFalseShapeOrRequiredErrors(t *testing.T) {
	source := []byte(`all:
  children:
    infra:
      hosts:
        10.0.0.1: { infra_seq: 1 }
    etcd:
      hosts:
        10.0.0.1: { etcd_seq: 1 }
      vars: { etcd_cluster: etcd }
    pgsql:
      hosts:
        10.0.0.1: !vault |
          $ANSIBLE_VAULT;1.1;AES256
          6630
      vars: { pg_cluster: pgsql }
  vars:
    version: v4.4.0
    admin_ip: 10.0.0.1
    region: default
`)
	doc, err := Parse("opaque-host.yml", source)
	if err != nil {
		t.Fatal(err)
	}
	diagnostics := doc.Validate(YAMLRules, AnsibleRules, PigstyRules, CompleteInventoryRules)
	if HasErrors(diagnostics) || len(diagnostics) != 1 || diagnostics[0].Code != "UNVERIFIABLE" || diagnostics[0].Path.String() != "/all/children/pgsql/hosts/10.0.0.1" {
		t.Fatalf("opaque host diagnostics = %+v", diagnostics)
	}
}

func TestTaggedGroupIsOpaqueAsAWhole(t *testing.T) {
	source := []byte(`all: !opaque
  children:
    infra:
      hosts:
        10.0.0.1: { infra_seq: invalid }
  vars:
    admin_ip: invalid
`)
	doc, err := Parse("opaque-all.yml", source)
	if err != nil {
		t.Fatal(err)
	}
	diagnostics := doc.Validate(YAMLRules, AnsibleRules, PigstyRules, CompleteInventoryRules)
	if HasErrors(diagnostics) || len(diagnostics) != 1 || diagnostics[0].Code != "UNVERIFIABLE" || diagnostics[0].Path.String() != "/all" {
		t.Fatalf("tagged group diagnostics = %+v", diagnostics)
	}
}

func TestTaggedGroupVarsDoNotCreateFalseIdentityErrors(t *testing.T) {
	source := []byte(`all:
  children:
    infra:
      hosts:
        10.0.0.1: { infra_seq: 1 }
    etcd:
      hosts:
        10.0.0.1: { etcd_seq: 1 }
      vars: { etcd_cluster: etcd }
    pgsql:
      hosts:
        10.0.0.1: { pg_seq: 1, pg_role: primary }
      vars: !opaque { pg_cluster: bad_name }
  vars:
    version: v4.4.0
    admin_ip: 10.0.0.1
    region: default
`)
	doc, err := Parse("opaque-group-vars.yml", source)
	if err != nil {
		t.Fatal(err)
	}
	diagnostics := doc.Validate(YAMLRules, AnsibleRules, PigstyRules, CompleteInventoryRules)
	if HasErrors(diagnostics) || len(diagnostics) != 1 || diagnostics[0].Code != "UNVERIFIABLE" || diagnostics[0].Path.String() != "/all/children/pgsql/vars" {
		t.Fatalf("tagged group vars diagnostics = %+v", diagnostics)
	}
}

func TestValidationDiagnosticsDoNotContainArbitraryValues(t *testing.T) {
	const adminValue = "arbitrary-admin-value"
	const passwordValue = "arbitrary-password-value"
	source := []byte(`all:
  children:
    infra:
      hosts:
        10.0.0.1: { infra_seq: 1 }
    etcd:
      hosts:
        10.0.0.1: { etcd_seq: 1 }
      vars: { etcd_cluster: etcd }
    pgsql:
      hosts:
        10.0.0.1: { pg_seq: 1, pg_role: primary }
      vars:
        pg_cluster: pgsql
        pg_users:
          - { name: app, password: [ arbitrary-password-value ] }
  vars:
    version: v4.4.0
    admin_ip: arbitrary-admin-value
    region: default
`)
	doc, err := Parse("redacted.yml", source)
	if err != nil {
		t.Fatal(err)
	}
	diagnostics := doc.Validate(YAMLRules, AnsibleRules, PigstyRules, CompleteInventoryRules)
	if !HasErrors(diagnostics) {
		t.Fatalf("redaction fixture unexpectedly valid: %+v", diagnostics)
	}
	for _, diagnostic := range diagnostics {
		if strings.Contains(diagnostic.Message, adminValue) || strings.Contains(diagnostic.Message, passwordValue) {
			t.Fatalf("diagnostic leaked an Inventory value: %+v", diagnostic)
		}
	}
}

func TestCompleteInventoryRuleDoesNotBreakPartialMutationDocuments(t *testing.T) {
	doc, err := Parse("fragment.yml", []byte("all:\n  vars:\n    version: v4.4.0\n    admin_ip: 10.0.0.1\n    region: default\n"))
	if err != nil {
		t.Fatal(err)
	}
	if diagnostics := doc.Validate(); HasErrors(diagnostics) {
		t.Fatalf("partial mutation document failed default rules: %+v", diagnostics)
	}
	diagnostics := doc.Validate(YAMLRules, AnsibleRules, PigstyRules, CompleteInventoryRules)
	if !hasDiagnostic(diagnostics, "INV_CHILDREN_REQUIRED") {
		t.Fatalf("complete file rules did not require all.children: %+v", diagnostics)
	}
}

func TestCompleteInventoryRejectsNullChildrenAndVars(t *testing.T) {
	doc, err := Parse("null-shape.yml", []byte("all:\n  children:\n  vars:\n"))
	if err != nil {
		t.Fatal(err)
	}
	diagnostics := doc.Validate(YAMLRules, AnsibleRules, PigstyRules, CompleteInventoryRules)
	if !hasDiagnostic(diagnostics, "INV_CHILDREN_TYPE") || !hasDiagnostic(diagnostics, "INV_VARS_TYPE") {
		t.Fatalf("null complete-file shape diagnostics = %+v", diagnostics)
	}
}

func TestPigstyRulesDoNotResolveEffectiveVars(t *testing.T) {
	source := []byte(`all:
  children:
    pgsql:
      hosts:
        10.0.0.1: {}
      vars:
        pg_cluster: pgsql
        pg_seq: 1
        pg_role: primary
  vars:
    version: v4.4.0
    admin_ip: 10.0.0.1
    region: default
`)
	doc, err := Parse("declarations-only.yml", source)
	if err != nil {
		t.Fatal(err)
	}
	diagnostics := doc.Validate(YAMLRules, AnsibleRules, PigstyRules, CompleteInventoryRules)
	if !hasDiagnostic(diagnostics, "PIG_PG_SEQ_REQUIRED") || !hasDiagnostic(diagnostics, "PIG_PG_ROLE_REQUIRED") {
		t.Fatalf("group vars were incorrectly treated as effective host vars: %+v", diagnostics)
	}
}

type corpusBaseline struct {
	Schema                  string         `json:"schema"`
	InventoryPaths          int            `json:"inventory_paths"`
	RegularYAMLFiles        int            `json:"regular_yaml_files"`
	SymlinkedInventoryPaths int            `json:"symlinked_inventory_paths"`
	Errors                  int            `json:"errors"`
	Warnings                map[string]int `json:"warnings"`
	Info                    map[string]int `json:"info"`
}

func TestPigstyConfCorpusValidationBaseline(t *testing.T) {
	root := os.Getenv("PIGSTY_CONF_DIR")
	if root == "" {
		t.Skip("set PIGSTY_CONF_DIR to verify the documented validation baseline")
	}
	root = requireCorpusDirectory(t, root)
	baseline := readJSONFixture[corpusBaseline](t, filepath.Join("testdata", "validate", "corpus-baseline.json"))
	if baseline.Schema != "pig.sty.validate.corpus-baseline/v1" {
		t.Fatalf("unexpected baseline schema %q", baseline.Schema)
	}
	files := yamlCorpusFiles(t, root)
	regular, symlinks := 0, 0
	warnings := make(map[string]int)
	infoCounts := make(map[string]int)
	errorsCount := 0
	for _, path := range files {
		fileInfo, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		if fileInfo.Mode()&os.ModeSymlink != 0 {
			symlinks++
		} else {
			regular++
		}
		doc, err := Load(path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		for _, diagnostic := range doc.Validate(YAMLRules, AnsibleRules, PigstyRules, CompleteInventoryRules) {
			switch diagnostic.Severity {
			case SeverityError:
				errorsCount++
			case SeverityWarning:
				warnings[diagnostic.Code]++
			case SeverityInfo:
				infoCounts[diagnostic.Code]++
			}
		}
	}
	if len(files) != baseline.InventoryPaths || regular != baseline.RegularYAMLFiles || symlinks != baseline.SymlinkedInventoryPaths {
		t.Fatalf("corpus paths=(%d regular=%d symlink=%d), want (%d,%d,%d)", len(files), regular, symlinks, baseline.InventoryPaths, baseline.RegularYAMLFiles, baseline.SymlinkedInventoryPaths)
	}
	if errorsCount != baseline.Errors || !reflect.DeepEqual(warnings, baseline.Warnings) || !reflect.DeepEqual(infoCounts, baseline.Info) {
		t.Fatalf("corpus diagnostics errors=%d warnings=%v info=%v; baseline errors=%d warnings=%v info=%v", errorsCount, warnings, infoCounts, baseline.Errors, baseline.Warnings, baseline.Info)
	}
}

func legacyPython(t *testing.T) (string, bool) {
	t.Helper()
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Log("python3 unavailable; pinned source and native parity remain verified")
		return "", false
	}
	if err := exec.Command(python, "-c", "import yaml").Run(); err != nil {
		t.Log("python3 has no PyYAML; pinned source and native parity remain verified")
		return "", false
	}
	return python, true
}

func diagnosticCodes(diagnostics []Diagnostic) []string {
	set := make(map[string]struct{}, len(diagnostics))
	for _, diagnostic := range diagnostics {
		set[diagnostic.Code] = struct{}{}
	}
	result := make([]string, 0, len(set))
	for code := range set {
		result = append(result, code)
	}
	sort.Strings(result)
	return result
}

func hasDiagnostic(diagnostics []Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}

func readJSONFixture[T any](t *testing.T, path string) T {
	t.Helper()
	var result T
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(content, &result); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return result
}
