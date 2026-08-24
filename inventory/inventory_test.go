package inventory

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/goccy/go-yaml/lexer"
	"github.com/goccy/go-yaml/token"
	"gopkg.in/yaml.v3"
)

func loadLosslessFixture(t *testing.T) []byte {
	t.Helper()
	source, err := os.ReadFile(filepath.Join("testdata", "lossless.yml"))
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func TestParseAndNoopCommitAreByteExact(t *testing.T) {
	source := loadLosslessFixture(t)
	doc, err := Parse("lossless.yml", source)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !bytes.Equal(doc.Bytes(), source) {
		t.Fatal("Parse changed source bytes")
	}
	assertByteExact(t, "parsed document serialization", source, writeDocument(t, doc))
	committed, changes, err := doc.Edit().Commit()
	if err != nil {
		t.Fatalf("no-op Commit: %v", err)
	}
	if len(changes.Changes) != 0 {
		t.Fatalf("no-op changes = %+v", changes)
	}
	if !bytes.Equal(committed.Bytes(), source) {
		t.Fatal("no-op Commit changed source bytes")
	}
	assertByteExact(t, "committed document serialization", source, writeDocument(t, committed))
}

func TestLoadRejectsExecutableDynamicSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inventory.yml")
	if err := os.WriteFile(path, []byte("all:\n  children: {}\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "dynamic") {
		t.Fatalf("Load executable source error = %v", err)
	}
}

func TestSemanticScalarsRetainRawLexemes(t *testing.T) {
	doc, err := Parse("lossless.yml", loadLosslessFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	mode, ok := doc.Lookup(AllVars("mode"))
	if !ok || mode.Scalar == nil || mode.Scalar.Raw != "0755" {
		t.Fatalf("mode scalar = %+v", mode)
	}
	for _, key := range []string{"flag_y"} {
		value, ok := doc.Lookup(AllVars(key))
		if !ok || value.Scalar == nil || value.Scalar.Kind != StringScalar {
			t.Fatalf("%s should remain a string scalar: %+v", key, value)
		}
	}
	off, ok := doc.Lookup(AllVars("flag_off"))
	if !ok || off.Scalar == nil || off.Scalar.Kind != BoolScalar || off.Scalar.Value != false || off.Scalar.Raw != "off" {
		t.Fatalf("flag_off should follow Ansible YAML 1.1 semantics: %+v", off)
	}
	folded, ok := doc.Lookup(AllVars("folded"))
	if !ok || folded.Scalar == nil || !strings.Contains(folded.Scalar.Raw, "line two") {
		t.Fatalf("folded scalar raw lexeme was truncated: %+v", folded)
	}
	inv := doc.Inventory()
	if inv == nil || inv.All == nil || inv.All.Vars == nil {
		t.Fatalf("semantic inventory not built: %+v", inv)
	}
}

func TestSemanticScalarsMatchAnsibleLegacyResolver(t *testing.T) {
	source := []byte(`all:
  vars:
    yes_value: yes
    off_value: OFF
    quoted: "off"
    sexagesimal_int: 1:20
    sexagesimal_float: 1:20.5
    legacy_octal: 0755
    yaml12_octal: 0o755
    yaml12_exponent: 1e3
    float_exponent: 1.0e+3
  children: {}
`)
	doc, err := Parse("ansible-scalars.yml", source)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		key   string
		kind  ScalarKind
		value any
	}{
		{"yes_value", BoolScalar, true},
		{"off_value", BoolScalar, false},
		{"quoted", StringScalar, "off"},
		{"sexagesimal_int", IntegerScalar, int64(80)},
		{"sexagesimal_float", FloatScalar, 80.5},
		{"legacy_octal", IntegerScalar, int64(493)},
		{"yaml12_octal", StringScalar, "0o755"},
		{"yaml12_exponent", StringScalar, "1e3"},
		{"float_exponent", FloatScalar, 1000.0},
	}
	for _, test := range tests {
		value, ok := doc.Lookup(AllVars(test.key))
		if !ok || value.Scalar == nil || value.Scalar.Kind != test.kind || fmt.Sprint(value.Scalar.Value) != fmt.Sprint(test.value) {
			t.Errorf("%s = %+v, want %s %v", test.key, value, test.kind, test.value)
		}
	}
	if !bytes.Equal(doc.Bytes(), source) {
		t.Fatal("semantic resolution changed source bytes")
	}
}

func TestDuplicateKeysFollowAnsibleNumericEquality(t *testing.T) {
	for _, source := range []string{
		"all:\n  vars: { 1: first, yes: second }\n",
		"all:\n  vars: { 1: first, 1.0: second }\n",
		"all:\n  vars: { 0: first, off: second }\n",
	} {
		if _, err := Parse("ansible-duplicate.yml", []byte(source)); err == nil || !strings.Contains(err.Error(), "duplicate mapping key") {
			t.Fatalf("Ansible-equivalent duplicate was accepted: source=%q err=%v", source, err)
		}
	}
	if _, err := Parse("typed-distinct.yml", []byte("all:\n  vars: { 1: numeric, '1': string }\n")); err != nil {
		t.Fatalf("numeric and quoted string keys should remain distinct: %v", err)
	}
}

func TestSemanticSnapshotsCannotMutateDocument(t *testing.T) {
	doc, err := Parse("snapshot.yml", []byte("all:\n  vars:\n    region: default\n  children: {}\n"))
	if err != nil {
		t.Fatal(err)
	}
	inv := doc.Inventory()
	inv.All.Name = "changed"
	inv.All.Vars.Mapping[0].Value.Scalar.Value = "changed"
	value, _ := doc.Lookup(AllVars("region"))
	value.Scalar.Value = "also-changed"

	fresh := doc.Inventory()
	region, _ := doc.Lookup(AllVars("region"))
	if fresh.All.Name != "all" || fresh.All.Vars.Mapping[0].Value.Scalar.Value != "default" || region.Scalar.Value != "default" {
		t.Fatalf("caller mutation escaped semantic snapshot: inventory=%+v region=%+v", fresh, region)
	}
}

func TestInventoryRetainsRootLevelGroups(t *testing.T) {
	doc, err := Parse("root-groups.yml", []byte(`all:
  vars: {}
foo:
  hosts:
    192.0.2.1: { role: root }
  children:
    nested:
      hosts: {}
`))
	if err != nil {
		t.Fatal(err)
	}
	inv := doc.Inventory()
	if inv == nil || inv.All == nil || len(inv.Groups) != 2 {
		t.Fatalf("root groups missing from semantic inventory: %+v", inv)
	}
	if inv.Groups[0].Name != "foo" || inv.Groups[0].Path.String() != "/foo" || inv.Groups[1].Path.String() != "/foo/children/nested" {
		t.Fatalf("root group paths = %+v", inv.Groups)
	}
}

func TestInventoryGroupOrderFollowsRootDeclarations(t *testing.T) {
	doc, err := Parse("ordered-root-groups.yml", []byte(`before:
  hosts: {}
all:
  children:
    nested:
      hosts: {}
after:
  hosts: {}
`))
	if err != nil {
		t.Fatal(err)
	}
	inv := doc.Inventory()
	if inv == nil || inv.All == nil || len(inv.Groups) != 3 {
		t.Fatalf("semantic inventory = %+v", inv)
	}
	want := []string{"before", "nested", "after"}
	for index, name := range want {
		if inv.Groups[index].Name != name {
			t.Fatalf("group order = %v, want %v", []string{inv.Groups[0].Name, inv.Groups[1].Name, inv.Groups[2].Name}, want)
		}
	}
	if diagnostics := doc.Validate(AnsibleRules); !HasErrors(diagnostics) || !hasDiagnostic(diagnostics, "INV_ALL_FIRST") {
		t.Fatalf("non-leading all must fail validation: %+v", diagnostics)
	}

	withoutAll, err := Parse("missing-all.yml", []byte("orphan:\n  hosts: {}\n"))
	if err != nil {
		t.Fatal(err)
	}
	missing := withoutAll.Inventory()
	if missing == nil || missing.All != nil || len(missing.Groups) != 1 || missing.Groups[0].Name != "orphan" {
		t.Fatalf("root declarations disappeared without all: %+v", missing)
	}
}

func TestInventoryNamesRetainSourceDeclarationText(t *testing.T) {
	doc, err := Parse("ambiguous-names.yml", []byte(`all:
  children:
    yes:
      hosts:
        off: {}
`))
	if err != nil {
		t.Fatal(err)
	}
	inv := doc.Inventory()
	if inv == nil || len(inv.Groups) != 1 || inv.Groups[0].Name != "yes" || len(inv.Groups[0].Hosts) != 1 || inv.Groups[0].Hosts[0].Name != "off" {
		t.Fatalf("source declaration names were semantically coerced: %+v", inv)
	}
	diagnostics := doc.Validate(AnsibleRules)
	if !hasDiagnostic(diagnostics, "INV_GROUP_NAME_TYPE") || !hasDiagnostic(diagnostics, "INV_HOST_NAME_TYPE") {
		t.Fatalf("ambiguous Ansible identity diagnostics = %+v", diagnostics)
	}
}

func TestSetOnlyChangesScalarLexeme(t *testing.T) {
	source := loadLosslessFixture(t)
	doc, err := Parse("lossless.yml", source)
	if err != nil {
		t.Fatal(err)
	}
	editor := doc.Edit()
	if err := editor.Set(AllVars("padded_key"), "changed"); err != nil {
		t.Fatal(err)
	}
	if err := editor.Set(AllVars("quoted"), "new # value"); err != nil {
		t.Fatal(err)
	}
	if err := editor.Set(AllVars("中文键"), "新值"); err != nil {
		t.Fatal(err)
	}
	committed, _, err := editor.Commit()
	if err != nil {
		t.Fatal(err)
	}
	want := bytes.Replace(source, []byte("padded_key   : value"), []byte("padded_key   : changed"), 1)
	want = bytes.Replace(want, []byte(`quoted       : "hello # world"`), []byte(`quoted       : "new # value"`), 1)
	want = bytes.Replace(want, []byte("中文键       : 原值"), []byte("中文键       : 新值"), 1)
	assertByteExact(t, "modified document bytes", want, committed.Bytes())
	assertByteExact(t, "modified document serialization", want, writeDocument(t, committed))
}

func TestProtectedNodesRefuseDirectSet(t *testing.T) {
	doc, err := Parse("lossless.yml", loadLosslessFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"folded", "literal", "anchor_value", "alias_value", "tagged"} {
		err := doc.Edit().Set(AllVars(key), "changed")
		if !errors.Is(err, ErrProtectedNode) {
			t.Fatalf("Set(%s) error = %v, want ErrProtectedNode", key, err)
		}
	}
}

func TestRenameKeyPreservesPreColonPadding(t *testing.T) {
	source := []byte("all:\n  vars:\n    short      : old  # keep padding\n")
	doc, err := Parse("rename-padding.yml", source)
	if err != nil {
		t.Fatal(err)
	}
	editor := doc.Edit()
	if err := editor.RenameKey(AllVars("short"), "renamed"); err != nil {
		t.Fatal(err)
	}
	committed, _, err := editor.Commit()
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("all:\n  vars:\n    renamed      : old  # keep padding\n")
	assertByteExact(t, "padded key rename", want, writeDocument(t, committed))
}

func TestFlowInsertDeleteAndBlockInsert(t *testing.T) {
	doc, err := Parse("lossless.yml", loadLosslessFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	editor := doc.Edit()
	if err := editor.Insert(AllVars("flow_map"), "c", 3, AfterKey("a")); err != nil {
		t.Fatal(err)
	}
	if err := editor.Delete(AllVars("flow_map", "b")); err != nil {
		t.Fatal(err)
	}
	if err := editor.Insert(AllVars(), "first_insert", "one", AfterKey("padded_key")); err != nil {
		t.Fatal(err)
	}
	if err := editor.Insert(AllVars(), "second_insert", "two", AfterKey("padded_key")); err != nil {
		t.Fatal(err)
	}
	committed, _, err := editor.Commit()
	if err != nil {
		t.Fatal(err)
	}
	got := string(committed.Bytes())
	if !strings.Contains(got, "flow_map     : {a: 1,c: 3}") {
		t.Fatalf("flow edit did not preserve compact delimiter style:\n%s", got)
	}
	first := strings.Index(got, "first_insert")
	second := strings.Index(got, "second_insert")
	if first < 0 || second < 0 || first > second {
		t.Fatalf("same-offset insert call order was not preserved:\n%s", got)
	}
}

func TestBlockInsertInheritsPaddingAndCommentColumn(t *testing.T) {
	source := []byte("all:\n  vars:\n    short     : old       # existing value\n    # keep this standalone comment with next\n    next      : keep      # next value\n")
	doc, err := Parse("aligned.yml", source)
	if err != nil {
		t.Fatal(err)
	}
	editor := doc.Edit()
	if err := editor.Insert(AllVars(), "medium", "new", AfterKey("short"), WithInlineComment("inserted value")); err != nil {
		t.Fatal(err)
	}
	if err := editor.Insert(AllVars(), "tiny", "added", AfterKey("short"), WithInlineComment("second value")); err != nil {
		t.Fatal(err)
	}
	committed, _, err := editor.Commit()
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("all:\n  vars:\n    short     : old       # existing value\n    medium    : new       # inserted value\n    tiny      : added     # second value\n    # keep this standalone comment with next\n    next      : keep      # next value\n")
	assertByteExact(t, "aligned block insertion", want, writeDocument(t, committed))

	remove := committed.Edit()
	if err := remove.Delete(AllVars("medium")); err != nil {
		t.Fatal(err)
	}
	if err := remove.Delete(AllVars("tiny")); err != nil {
		t.Fatal(err)
	}
	restored, _, err := remove.Commit()
	if err != nil {
		t.Fatal(err)
	}
	assertByteExact(t, "insert/delete restoration", source, writeDocument(t, restored))
}

func TestBlockInsertInfersAlignmentFromNearbySiblings(t *testing.T) {
	source := []byte("all:\n  vars:\n    longest_key: old\n    short      : keep        # sibling comment\n")
	doc, err := Parse("nearby-style.yml", source)
	if err != nil {
		t.Fatal(err)
	}
	editor := doc.Edit()
	if err := editor.Insert(AllVars(), "mid", "new", AfterKey("longest_key"), WithInlineComment("inserted comment")); err != nil {
		t.Fatal(err)
	}
	committed, _, err := editor.Commit()
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("all:\n  vars:\n    longest_key: old\n    mid        : new         # inserted comment\n    short      : keep        # sibling comment\n")
	assertByteExact(t, "mapping-wide alignment inference", want, writeDocument(t, committed))
}

func TestReplaceRenameUncommentAndConflictRollback(t *testing.T) {
	source := []byte("---\nall:\n  vars:\n    10.10.10.10: 10.10.10.10 # 10.10.10.10\n    region: default\n    #mirror_url: https://example.invalid\n...\n")
	doc, err := Parse("literal.yml", source)
	if err != nil {
		t.Fatal(err)
	}
	editor := doc.Edit()
	count, err := editor.ReplaceAll("10.10.10.10", "192.168.1.10")
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("replace count = %d, want 3", count)
	}
	if uncommented, err := editor.UncommentKey("mirror_url"); err != nil || uncommented.Changed != 1 {
		t.Fatalf("UncommentKey = %+v, %v", uncommented, err)
	}
	committed, changes, err := editor.Commit()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(committed.Bytes()), "10.10.10.10") || !strings.Contains(string(committed.Bytes()), "mirror_url: https://example.invalid") {
		t.Fatalf("literal/uncomment result:\n%s", committed.Bytes())
	}
	if len(changes.Changes) != 2 {
		t.Fatalf("changes = %+v", changes)
	}

	conflict := doc.Edit()
	if err := conflict.Set(AllVars("region"), "china"); err != nil {
		t.Fatal(err)
	}
	if _, err := conflict.ReplaceAll("default", "europe", WithLiteralTargets(LiteralScalars)); !errors.Is(err, ErrEditConflict) {
		t.Fatalf("overlap error = %v, want ErrEditConflict", err)
	}
	if candidate, _, err := conflict.Commit(); candidate != nil || !errors.Is(err, ErrEditConflict) {
		t.Fatalf("aborted transaction Commit = (%v, %v), want nil ErrEditConflict", candidate, err)
	}
	if !bytes.Equal(doc.Bytes(), source) {
		t.Fatal("failed transaction mutated base document")
	}

	renameDoc, err := Parse("rename.yml", []byte("all:\n  vars:\n    region: default\n"))
	if err != nil {
		t.Fatal(err)
	}
	renameEditor := renameDoc.Edit()
	if err := renameEditor.RenameKey(AllVars("region"), "mirror_region"); err != nil {
		t.Fatal(err)
	}
	renamed, _, err := renameEditor.Commit()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(renamed.Bytes()), "mirror_region: default") {
		t.Fatalf("RenameKey result:\n%s", renamed.Bytes())
	}
}

func TestCommentIndexAndUncomment(t *testing.T) {
	source := []byte("all:\n  children:\n    one:\n      vars:\n        note: keep # inline\n        #mirror: one\n        literal: |-\n          #mirror: not-a-comment\n    two:\n      vars:\n        #mirror: two\n  vars: {}\n")
	doc, err := Parse("comments.yml", source)
	if err != nil {
		t.Fatal(err)
	}
	if got := doc.CommentedKeyCount("mirror"); got != 2 {
		t.Fatalf("CommentedKeyCount = %d, want 2", got)
	}
	mutation := doc.Mutate()
	result, err := mutation.UncommentKey("mirror")
	if err != nil {
		t.Fatal(err)
	}
	if result.Matched != 2 || result.Changed != 2 || len(result.Paths) != 2 {
		t.Fatalf("uncomment result = %+v", result)
	}
	if got := mutation.Document().CommentedKeyCount("mirror"); got != 0 {
		t.Fatalf("remaining CommentedKeyCount = %d, want 0", got)
	}
	if !bytes.Contains(mutation.Document().Bytes(), []byte("#mirror: not-a-comment")) {
		t.Fatal("block scalar content was mistaken for a YAML comment")
	}
}

func TestBlockPlainScalarWithCommasIsReplacedCompletely(t *testing.T) {
	source := []byte("all:\n  vars:\n    modules: node,infra,pgsql # keep\n")
	doc, err := Parse("commas.yml", source)
	if err != nil {
		t.Fatal(err)
	}
	editor := doc.Edit()
	if err := editor.Set(AllVars("modules"), "node,infra,pgsql,beta"); err != nil {
		t.Fatal(err)
	}
	committed, _, err := editor.Commit()
	if err != nil {
		t.Fatal(err)
	}
	want := "all:\n  vars:\n    modules: node,infra,pgsql,beta # keep\n"
	if got := string(committed.Bytes()); got != want {
		t.Fatalf("comma scalar replacement mismatch:\nwant: %q\n got: %q", want, got)
	}
}

func TestChangeSetNeverContainsScalarValues(t *testing.T) {
	const oldSecret = "DBUser.Example"
	const newSecret = "SuperSecretValue123"
	doc, err := Parse("secret.yml", []byte("all:\n  vars:\n    password: "+oldSecret+"\n"))
	if err != nil {
		t.Fatal(err)
	}
	editor := doc.Edit()
	if err := editor.Set(AllVars("password"), newSecret); err != nil {
		t.Fatal(err)
	}
	_, changes, err := editor.Commit()
	if err != nil {
		t.Fatal(err)
	}
	if len(changes.Changes) != 1 {
		t.Fatalf("changes = %+v", changes)
	}
	dump := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(changes.Changes[0].Operation+" "+changes.Changes[0].Path.String()+" "+changes.Changes[0].Identifier), "\n", " "), "\t", " "))
	if strings.Contains(dump, oldSecret) || strings.Contains(dump, newSecret) {
		t.Fatalf("ChangeSet leaked scalar data: %q", dump)
	}
}

func TestCRLFAndNoTrailingNewline(t *testing.T) {
	source := []byte("---\r\nall:\r\n  vars:\r\n    region: default\r\n  children: {}\r\n...")
	doc, err := Parse("crlf.yml", source)
	if err != nil {
		t.Fatal(err)
	}
	editor := doc.Edit()
	if err := editor.Insert(AllVars(), "pg_version", 18, AfterKey("region")); err != nil {
		t.Fatal(err)
	}
	committed, _, err := editor.Commit()
	if err != nil {
		t.Fatal(err)
	}
	got := committed.Bytes()
	if bytes.Contains(bytes.ReplaceAll(got, []byte("\r\n"), nil), []byte("\n")) {
		t.Fatalf("insert introduced LF-only newline: %q", got)
	}
	if bytes.HasSuffix(got, []byte("\n")) {
		t.Fatalf("document unexpectedly gained a trailing newline: %q", got)
	}
}

func TestValidationDetectsStructureAndCriticalIdentity(t *testing.T) {
	source := []byte(`all:
  children:
    bad.group:
      hosts:
        db1: { pg_seq: one, pg_role: leader }
      vars: {}
  vars:
    admin_ip: 10.0.0.1
`)
	doc, err := Parse("invalid-inventory.yml", source)
	if err != nil {
		t.Fatal(err)
	}
	diagnostics := doc.Validate()
	want := map[string]bool{"INV_GROUP_NAME": false, "PIG_SEQ_TYPE": false, "PIG_IDENTITY_MISSING": false, "PIG_PG_ROLE": false}
	for _, diagnostic := range diagnostics {
		if _, exists := want[diagnostic.Code]; exists {
			want[diagnostic.Code] = true
		}
	}
	for code, found := range want {
		if !found {
			t.Fatalf("missing diagnostic %s in %+v", code, diagnostics)
		}
	}
}

func TestMutationScopedVariablesAndContinuousVisibility(t *testing.T) {
	source := []byte("all:\n  children:\n    pg-test:\n      hosts:\n        10.0.0.1:\n  vars:\n    region: default # existing comment\n")
	doc, err := Parse("scopes.yml", source)
	if err != nil {
		t.Fatal(err)
	}
	mutation := doc.Mutate()
	if count, err := mutation.ReplaceAll("10.0.0.1", "10.0.0.2"); err != nil || count != 1 {
		t.Fatalf("ReplaceAll = %d, %v", count, err)
	}
	globals := mutation.AllVars()
	if err := globals.Set("region", "china", WithInlineComment("must not replace existing comment")); err != nil {
		t.Fatal(err)
	}
	if err := globals.Set("pg_version", 18, AfterKey("region"), WithInlineComment("postgres major")); err != nil {
		t.Fatal(err)
	}
	proxy := globals.Mapping("proxy_env", AfterKey("region"))
	if err := proxy.Set("http_proxy", "http://proxy.example", WithInlineComment("download proxy")); err != nil {
		t.Fatal(err)
	}
	if err := proxy.Set("no_proxy", "localhost"); err != nil {
		t.Fatal(err)
	}

	inv := mutation.Document().Inventory()
	if inv == nil || len(inv.Groups) != 1 || len(inv.Groups[0].Hosts) != 1 {
		t.Fatalf("inventory after replacement = %+v", inv)
	}
	group := inv.Groups[0]
	host := group.Hosts[0]
	if host.Name != "10.0.0.2" {
		t.Fatalf("renamed host = %q", host.Name)
	}
	if err := mutation.GroupVars(group).Set("pg_cluster", "pg-test", WithInlineComment("cluster identity")); err != nil {
		t.Fatal(err)
	}
	if err := mutation.HostVars(host).Set("pg_seq", 1, WithInlineComment("instance sequence")); err != nil {
		t.Fatal(err)
	}
	if err := mutation.HostVars(host).Set("pg_role", "primary", WithInlineComment("instance role")); err != nil {
		t.Fatal(err)
	}

	committed, changes, err := mutation.Commit()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(doc.Bytes(), source) {
		t.Fatal("Mutation changed its base Document")
	}
	if len(changes.Changes) < 8 {
		t.Fatalf("changes = %+v", changes)
	}
	got := string(committed.Bytes())
	for _, fragment := range []string{
		"10.0.0.2:",
		"region: china # existing comment",
		"pg_version: 18",
		"# postgres major",
		"proxy_env:",
		`http_proxy: "http://proxy.example"`,
		"# download proxy",
		"pg_cluster: pg-test",
		"# cluster identity",
		"pg_seq: 1",
		"# instance sequence",
		"pg_role: primary",
		"# instance role",
	} {
		if !strings.Contains(got, fragment) {
			t.Fatalf("mutated inventory is missing %q:\n%s", fragment, got)
		}
	}
	if reparsed, err := Parse("scopes-roundtrip.yml", committed.Bytes()); err != nil {
		t.Fatal(err)
	} else if !bytes.Equal(reparsed.Bytes(), committed.Bytes()) {
		t.Fatal("mutated inventory was not byte-exact after reparse")
	}
}

func TestMutationFailureRollsBackWholeSession(t *testing.T) {
	source := []byte("all:\n  vars:\n    region: default\n")
	doc, err := Parse("rollback.yml", source)
	if err != nil {
		t.Fatal(err)
	}
	mutation := doc.Mutate()
	if err := mutation.AllVars().Set("region", "china"); err != nil {
		t.Fatal(err)
	}
	if err := mutation.AllVars().Mapping("region").Set("nested", true); err == nil {
		t.Fatal("expected non-mapping scope error")
	}
	if committed, changes, err := mutation.Commit(); err == nil || committed != nil || len(changes.Changes) != 0 {
		t.Fatalf("failed Commit = doc=%v changes=%+v err=%v", committed, changes, err)
	}
	if !bytes.Equal(mutation.Document().Bytes(), source) {
		t.Fatal("failed Mutation exposed a partially edited snapshot")
	}
	if !bytes.Equal(doc.Bytes(), source) {
		t.Fatal("failed Mutation changed its base Document")
	}
}

func TestMutationPopulatesExplicitNullVars(t *testing.T) {
	doc, err := Parse("null-vars.yml", []byte("all:\n  vars: null\n"))
	if err != nil {
		t.Fatal(err)
	}
	mutation := doc.Mutate()
	if err := mutation.AllVars().Set("region", "china", WithInlineComment("region selector")); err != nil {
		t.Fatal(err)
	}
	committed, _, err := mutation.Commit()
	if err != nil {
		t.Fatal(err)
	}
	output := string(committed.Bytes())
	if !strings.Contains(output, "vars:\n    region: china # region selector") {
		t.Fatalf("explicit null mapping was not populated as a block mapping:\n%s", output)
	}
}

func TestMutationHostVarsKeepDuplicateDeclarationsSeparate(t *testing.T) {
	source := []byte("all:\n  children:\n    blue:\n      hosts:\n        10.0.0.1: {}\n    green:\n      hosts:\n        10.0.0.1: {}\n  vars: {}\n")
	doc, err := Parse("duplicate-host.yml", source)
	if err != nil {
		t.Fatal(err)
	}
	groups := doc.Inventory().Groups
	if len(groups) != 2 || len(groups[0].Hosts) != 1 || len(groups[1].Hosts) != 1 {
		t.Fatalf("groups = %+v", groups)
	}
	mutation := doc.Mutate()
	if err := mutation.HostVars(groups[0].Hosts[0]).Set("color", "blue", WithInlineComment("blue declaration")); err != nil {
		t.Fatal(err)
	}
	if err := mutation.HostVars(groups[1].Hosts[0]).Set("color", "green", WithInlineComment("green declaration")); err != nil {
		t.Fatal(err)
	}
	committed, _, err := mutation.Commit()
	if err != nil {
		t.Fatal(err)
	}
	blue, ok := committed.Lookup(groups[0].Hosts[0].Path.ChildKey("color"))
	if !ok || blue.Scalar == nil || blue.Scalar.Value != "blue" {
		t.Fatalf("blue host value = %+v", blue)
	}
	green, ok := committed.Lookup(groups[1].Hosts[0].Path.ChildKey("color"))
	if !ok || green.Scalar == nil || green.Scalar.Value != "green" {
		t.Fatalf("green host value = %+v", green)
	}
	output := string(committed.Bytes())
	if !strings.Contains(output, `{ color: blue } # blue declaration`) || !strings.Contains(output, `{ color: green } # green declaration`) {
		t.Fatalf("flow host comments were not preserved:\n%s", output)
	}
}

func TestTypedMappingMutationRenameAndDeleteAreLossless(t *testing.T) {
	source := []byte("all:\n  vars:\n    region: default\n")
	doc, err := Parse("typed-mutation.yml", source)
	if err != nil {
		t.Fatal(err)
	}

	mutation := doc.Mutate()
	vars := mutation.AllVars()
	if err := vars.Set("region", "china"); err != nil {
		t.Fatal(err)
	}
	if err := vars.Set(
		"generated_region",
		"enabled",
		AfterKey("region"),
		WithInlineComment("controller-only"),
	); err != nil {
		t.Fatal(err)
	}
	if err := vars.Rename("generated_region", "region_mode"); err != nil {
		t.Fatal(err)
	}
	if err := vars.Set("temporary", true); err != nil {
		t.Fatal(err)
	}
	if err := vars.Delete("temporary"); err != nil {
		t.Fatal(err)
	}

	committed, changes, err := mutation.Commit()
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("all:\n  vars:\n    region: china\n    region_mode: enabled # controller-only\n")
	if !bytes.Equal(committed.Bytes(), want) {
		t.Fatalf("typed mutation changed unexpected bytes:\n%s", committed.Bytes())
	}
	wantOperations := []string{"set", "insert", "rename-key", "insert", "delete"}
	if len(changes.Changes) != len(wantOperations) {
		t.Fatalf("changes = %+v", changes.Changes)
	}
	for index, operation := range wantOperations {
		if changes.Changes[index].Operation != operation {
			t.Fatalf("change %d operation = %q, want %q", index, changes.Changes[index].Operation, operation)
		}
	}
}

func TestParseRejectsMultipleDocumentsAndDuplicateKeys(t *testing.T) {
	for name, source := range map[string]string{
		"multiple":  "all: {}\n---\nall: {}\n",
		"duplicate": "all:\n  vars:\n    region: default\n    region: china\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(name+".yml", []byte(source)); err == nil {
				t.Fatal("expected parse error")
			}
		})
	}
}

func TestPigstyConfCorpusLosslessSerialization(t *testing.T) {
	root := pigstyConfCorpus(t)
	files := yamlCorpusFiles(t, root)
	var totalBytes int
	var genericYAMLExact int
	for _, file := range files {
		file := file
		relative, err := filepath.Rel(root, file)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(strings.TrimSuffix(relative, filepath.Ext(relative)), func(t *testing.T) {
			source, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			totalBytes += len(source)

			// Load must complete private syntax/comment indexing and the public
			// declaration-oriented semantic model before lossless serialization.
			doc, err := Load(file)
			if err != nil {
				t.Fatal(err)
			}
			if doc.Inventory() == nil || doc.Inventory().All == nil {
				t.Fatal("semantic inventory was not built")
			}
			if diagnostics := doc.Validate(); HasErrors(diagnostics) {
				t.Fatalf("validation errors: %+v", diagnostics)
			}
			firstGeneration := writeDocument(t, doc)
			assertByteExact(t, "load/write", source, firstGeneration)

			// A second parse/write generation catches serializers that happen to
			// retain the first input but normalize their own output afterward.
			reparsed, err := Parse(file+"#roundtrip", firstGeneration)
			if err != nil {
				t.Fatal(err)
			}
			secondGeneration := writeDocument(t, reparsed)
			assertByteExact(t, "second-generation parse/write", source, secondGeneration)

			committed, changes, err := reparsed.Edit().Commit()
			if err != nil {
				t.Fatal(err)
			}
			if len(changes.Changes) != 0 {
				t.Fatalf("no-op commit reported changes: %+v", changes)
			}
			assertByteExact(t, "no-op commit/write", source, writeDocument(t, committed))

			// This is a control, not the supported write-back path. A generic
			// semantic YAML round trip is expected to normalize Pigsty's compact
			// source and therefore demonstrates why Document retains source bytes.
			var semantic yaml.Node
			if err := yaml.Unmarshal(source, &semantic); err != nil {
				t.Fatalf("generic yaml.Unmarshal control: %v", err)
			}
			normalized, err := yaml.Marshal(&semantic)
			if err != nil {
				t.Fatalf("generic yaml.Marshal control: %v", err)
			}
			if bytes.Equal(normalized, source) {
				genericYAMLExact++
			}
		})
	}
	if genericYAMLExact == len(files) {
		t.Fatal("generic YAML control unexpectedly preserved the entire corpus byte-for-byte")
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		realRoot = root
	}
	t.Logf("lossless round trip: %d files, %d bytes, root=%s; generic yaml.Unmarshal/Marshal exact: %d/%d", len(files), totalBytes, realRoot, genericYAMLExact, len(files))
}

func TestPigstyConfCorpusCommentIndexComplete(t *testing.T) {
	root := pigstyConfCorpus(t)
	for _, file := range yamlCorpusFiles(t, root) {
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		doc, err := Parse(file, source)
		if err != nil {
			t.Fatal(err)
		}
		want := 0
		for _, item := range lexer.Tokenize(string(source)) {
			if item != nil && item.Type == token.CommentType {
				want++
			}
		}
		if len(doc.comments) != want {
			t.Fatalf("%s: indexed comments=%d, lexer comments=%d", file, len(doc.comments), want)
		}
		for _, comment := range doc.comments {
			if !comment.valid(len(source)) || source[comment.start] != '#' {
				t.Fatalf("%s: invalid comment span %+v", file, comment)
			}
		}
	}
}

func TestPigstyConfCorpusReversibleStyledInsertion(t *testing.T) {
	root := pigstyConfCorpus(t)
	files := yamlCorpusFiles(t, root)
	for _, file := range files {
		file := file
		relative, err := filepath.Rel(root, file)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(strings.TrimSuffix(relative, filepath.Ext(relative)), func(t *testing.T) {
			source, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			doc, err := Load(file)
			if err != nil {
				t.Fatal(err)
			}
			parent, ok := doc.Lookup(AllVars())
			if !ok || parent.Kind != MappingValue || len(parent.Mapping) == 0 {
				t.Fatal("all.vars is not a non-empty block mapping")
			}

			anchor, anchorRef := styledInsertionAnchor(doc, parent)
			if anchorRef == nil {
				t.Fatal("all.vars has no usable insertion anchor")
			}
			const probeKey = "z"
			if _, exists := doc.Lookup(AllVars(probeKey)); exists {
				t.Fatalf("test probe key %q already exists", probeKey)
			}
			editor := doc.Edit()
			if err := editor.Insert(AllVars(), probeKey, "style-probe", AfterKey(anchor), WithInlineComment("round-trip style probe")); err != nil {
				t.Fatal(err)
			}
			committed, _, err := editor.Commit()
			if err != nil {
				t.Fatal(err)
			}
			mutated := writeDocument(t, committed)
			inserted, ok := onlyInsertedBytes(source, mutated)
			if !ok {
				t.Fatal("insertion changed bytes outside one contiguous added fragment")
			}
			if !bytes.Contains(inserted, []byte(probeKey)) || !bytes.Contains(inserted, []byte("# round-trip style probe")) {
				t.Fatalf("inserted fragment lost key or comment: %q", inserted)
			}
			insertedRef := committed.refs[pathKey(AllVars(probeKey))]
			if insertedRef == nil || insertedRef.key == nil {
				t.Fatal("committed document did not index inserted key")
			}
			assertInsertedLineStyle(t, doc, anchorRef, committed, insertedRef, mutated)

			reparsed, err := Parse(file+"#mutated", mutated)
			if err != nil {
				t.Fatal(err)
			}
			assertByteExact(t, "mutated parse/write", mutated, writeDocument(t, reparsed))
			remove := reparsed.Edit()
			if err := remove.Delete(AllVars(probeKey)); err != nil {
				t.Fatal(err)
			}
			restored, _, err := remove.Commit()
			if err != nil {
				t.Fatal(err)
			}
			assertByteExact(t, "corpus insert/delete restoration", source, writeDocument(t, restored))
		})
	}
}

func TestPigstyConfCorpusReversibleFragmentEdit(t *testing.T) {
	root := pigstyConfCorpus(t)
	for _, file := range yamlCorpusFiles(t, root) {
		file := file
		relative, err := filepath.Rel(root, file)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(strings.TrimSuffix(relative, filepath.Ext(relative)), func(t *testing.T) {
			source, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			doc, err := Parse(file, source)
			if err != nil {
				t.Fatal(err)
			}
			path := AllVars("region")
			if _, ok := doc.Lookup(path); !ok {
				t.Skip("template has no all.vars.region")
			}
			fragment, err := doc.Extract(path)
			if err != nil {
				t.Fatal(err)
			}
			replacement := []byte("china")
			if string(fragment.Bytes) == "china" {
				replacement = []byte("europe")
			}
			editor := doc.Edit()
			if err := editor.ReplaceFragment(fragment, replacement); err != nil {
				t.Fatal(err)
			}
			changed, _, err := editor.Commit()
			if err != nil {
				t.Fatal(err)
			}
			mutated := changed.Bytes()
			if !bytes.Equal(source[:fragment.span.start], mutated[:fragment.span.start]) {
				t.Fatal("fragment edit changed bytes before selected range")
			}
			suffixStart := fragment.span.start + len(replacement)
			if !bytes.Equal(source[fragment.span.end:], mutated[suffixStart:]) {
				t.Fatal("fragment edit changed bytes after selected range")
			}

			reverse, err := changed.Extract(path)
			if err != nil {
				t.Fatal(err)
			}
			revert := changed.Edit()
			if err := revert.ReplaceFragment(reverse, fragment.Bytes); err != nil {
				t.Fatal(err)
			}
			restored, _, err := revert.Commit()
			if err != nil {
				t.Fatal(err)
			}
			assertByteExact(t, "corpus fragment restoration", source, restored.Bytes())
		})
	}
}

func styledInsertionAnchor(doc *Document, parent *Value) (string, *nodeRef) {
	var fallbackName string
	var fallback *nodeRef
	for i := range parent.Mapping {
		name := strings.TrimSpace(parent.Mapping[i].Key.Value.(string))
		ref := doc.refs[pathKey(AllVars(name))]
		if ref == nil || ref.key == nil || ref.node == nil || ref.node.Kind != yaml.ScalarNode {
			continue
		}
		if fallback == nil {
			fallbackName, fallback = name, ref
		}
		lineStart := doc.lineStarts[ref.key.Line-1]
		lineEnd := doc.lineEnd(lineStart, false)
		if inlineCommentOffset(doc, ref.keySpan.end, lineEnd) >= 0 {
			return name, ref
		}
	}
	return fallbackName, fallback
}

func assertInsertedLineStyle(t *testing.T, original *Document, anchor *nodeRef, committed *Document, inserted *nodeRef, mutated []byte) {
	t.Helper()
	anchorLineStart := original.lineStarts[anchor.key.Line-1]
	anchorLineEnd := original.lineEnd(anchorLineStart, false)
	anchorLine := original.source[anchorLineStart:anchorLineEnd]
	insertOffset := inserted.keySpan.start
	insertLineStart := committed.lineStarts[inserted.key.Line-1]
	insertLineEnd := len(mutated)
	if end := bytes.IndexByte(mutated[insertOffset:], '\n'); end >= 0 {
		insertLineEnd = insertOffset + end
	}
	insertLine := mutated[insertLineStart:insertLineEnd]
	if leadingWhitespace(anchorLine) != leadingWhitespace(insertLine) {
		t.Fatalf("insert indentation %q does not match anchor %q", insertLine, anchorLine)
	}

	anchorColon := bytes.IndexByte(original.source[anchor.keySpan.end:anchorLineEnd], ':')
	if anchorColon < 0 {
		t.Fatalf("anchor colon not found in %q", anchorLine)
	}
	anchorColon += anchor.keySpan.end
	insertKeyEnd := inserted.keySpan.end
	insertColonRelative := bytes.IndexByte(mutated[insertKeyEnd:insertLineEnd], ':')
	if insertColonRelative < 0 {
		t.Fatalf("inserted colon not found in %q", insertLine)
	}
	insertColon := insertKeyEnd + insertColonRelative
	anchorPadding := original.source[anchor.keySpan.end:anchorColon]
	insertPadding := mutated[insertKeyEnd:insertColon]
	if len(anchorPadding) == 0 {
		if len(insertPadding) != 0 {
			t.Fatalf("inserted key gained pre-colon padding: anchor=%q inserted=%q", anchorLine, insertLine)
		}
	} else if original.positionAt(anchorColon).Column != committed.positionAt(insertColon).Column {
		t.Fatalf("inserted colon column does not match anchor: anchor=%q inserted=%q", anchorLine, insertLine)
	}

	anchorComment := inlineCommentOffset(original, anchorColon+1, anchorLineEnd)
	insertCommentRelative := bytes.IndexByte(mutated[insertColon+1:insertLineEnd], '#')
	if anchorComment >= 0 && insertCommentRelative >= 0 {
		insertComment := insertColon + 1 + insertCommentRelative
		if original.positionAt(anchorComment).Column != committed.positionAt(insertComment).Column {
			t.Fatalf("inserted comment column does not match anchor: anchor=%q inserted=%q", anchorLine, insertLine)
		}
	}
}

func inlineCommentOffset(doc *Document, start, end int) int {
	for offset := start; offset < end; {
		relative := bytes.IndexByte(doc.source[offset:end], '#')
		if relative < 0 {
			return -1
		}
		candidate := offset + relative
		for _, comment := range doc.comments {
			if candidate >= comment.start && candidate < comment.end {
				return candidate
			}
		}
		offset = candidate + 1
	}
	return -1
}

func leadingWhitespace(line []byte) string {
	end := 0
	for end < len(line) && (line[end] == ' ' || line[end] == '\t') {
		end++
	}
	return string(line[:end])
}

func onlyInsertedBytes(original, mutated []byte) ([]byte, bool) {
	if len(mutated) <= len(original) {
		return nil, false
	}
	prefix := 0
	for prefix < len(original) && original[prefix] == mutated[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(original)-prefix && original[len(original)-1-suffix] == mutated[len(mutated)-1-suffix] {
		suffix++
	}
	if prefix+suffix != len(original) {
		return nil, false
	}
	return mutated[prefix : len(mutated)-suffix], true
}

func writeDocument(t *testing.T, doc *Document) []byte {
	t.Helper()
	var output bytes.Buffer
	written, err := doc.WriteTo(&output)
	if err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	if written != int64(output.Len()) {
		t.Fatalf("WriteTo reported %d bytes but wrote %d", written, output.Len())
	}
	return output.Bytes()
}

func assertByteExact(t *testing.T, stage string, want, got []byte) {
	t.Helper()
	if bytes.Equal(got, want) {
		return
	}
	wantHash := sha256.Sum256(want)
	gotHash := sha256.Sum256(got)
	t.Fatalf("%s changed bytes: len %d -> %d, first difference at byte %d, sha256 %x -> %x", stage, len(want), len(got), firstDifferentByte(want, got), wantHash, gotHash)
}

func firstDifferentByte(left, right []byte) int {
	limit := min(len(left), len(right))
	for i := 0; i < limit; i++ {
		if left[i] != right[i] {
			return i
		}
	}
	return limit
}

func pigstyConfCorpus(t *testing.T) string {
	t.Helper()
	if configured := os.Getenv("PIGSTY_CONF_DIR"); configured != "" {
		return requireCorpusDirectory(t, configured)
	}
	candidates := make([]string, 0, 2)
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, "pigsty", "conf"))
	}
	candidates = append(candidates, filepath.Join("..", "..", "pigsty", "conf"))
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return filepath.Clean(candidate)
		}
	}
	t.Skip("Pigsty conf corpus is not available; set PIGSTY_CONF_DIR to enable it")
	return ""
}

func requireCorpusDirectory(t *testing.T, root string) string {
	t.Helper()
	root = filepath.Clean(root)
	info, err := os.Stat(root)
	if err != nil {
		t.Fatalf("PIGSTY_CONF_DIR %q: %v", root, err)
	}
	if !info.IsDir() {
		t.Fatalf("PIGSTY_CONF_DIR %q is not a directory", root)
	}
	return root
}

func yamlCorpusFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		extension := strings.ToLower(filepath.Ext(path))
		if extension == ".yml" || extension == ".yaml" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatalf("Pigsty conf corpus %q contains no YAML files", root)
	}
	sort.Strings(files)
	return files
}
