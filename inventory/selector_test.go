package inventory

import (
	"bytes"
	"errors"
	"testing"
)

const selectorFixture = `all:
  children:
    pg-meta:                    # keep group comment
      hosts:
        10.10.10.10: {}
      vars:
        pg_cluster: pg-meta
        pg_databases:
          - name: meta
        infra_portal:
          i.pigsty: { port: 80 }
      children:
        pg-test:
          hosts: {}
  vars:                        # keep outer comment
    region: default            # keep inline comment
    infra_portal:
      i.pigsty: { port: 80 }
    slash/key:
      tilde~key: escaped
    note: |-
      first
      second
`

func TestParseSelectorShorthandAndCanonicalPointer(t *testing.T) {
	doc, err := Parse("selector.yml", []byte(selectorFixture))
	if err != nil {
		t.Fatal(err)
	}
	for selector, want := range map[string]string{
		"":                                      "/",
		"vars":                                  "/all/vars",
		"vars.region":                           "/all/vars/region",
		"vars.infra_portal":                     "/all/vars/infra_portal",
		"pg-meta":                               "/all/children/pg-meta",
		"pg-meta.vars.pg_databases":             "/all/children/pg-meta/vars/pg_databases",
		"pg-meta.vars.pg_databases[0].name":     "/all/children/pg-meta/vars/pg_databases/0/name",
		`pg-meta.hosts["10.10.10.10"]`:          "/all/children/pg-meta/hosts/10.10.10.10",
		`pg-meta.vars.infra_portal["i.pigsty"]`: "/all/children/pg-meta/vars/infra_portal/i.pigsty",
		"pg-meta.pg-test.hosts":                 "/all/children/pg-meta/children/pg-test/hosts",
		"pg-meta.children.pg-test.hosts":        "/all/children/pg-meta/children/pg-test/hosts",
		"/all/children/pg-meta/hosts/10.10.10.10": "/all/children/pg-meta/hosts/10.10.10.10",
		"/all/vars/slash~1key/tilde~0key":         "/all/vars/slash~1key/tilde~0key",
	} {
		path, err := ParseSelector(doc, selector)
		if err != nil {
			t.Fatalf("ParseSelector(%q): %v", selector, err)
		}
		if got := path.String(); got != want {
			t.Fatalf("ParseSelector(%q) = %s, want %s", selector, got, want)
		}
	}
	for _, test := range []struct{ selector, code string }{
		{"vars..region", SelectorSyntax},
		{"vars.missing", SelectorNotFound},
		{"pg-meta.vars.pg_databases[name]", SelectorSyntax},
		{"pg-meta.hosts.10.10.10.10", SelectorNotFound},
		{"vars.region.child", SelectorType},
		{"/all/vars/region/child", SelectorType},
		{"/all/vars/slash~2key", SelectorSyntax},
	} {
		_, err := ParseSelector(doc, test.selector)
		var selectorErr *SelectorError
		if !errors.As(err, &selectorErr) || selectorErr.Code != test.code {
			t.Fatalf("ParseSelector(%q) error = %v, want %s", test.selector, err, test.code)
		}
	}
}

func TestExtractReplaceMappingFragmentIsLossless(t *testing.T) {
	source := []byte(selectorFixture)
	doc, err := Parse("fragment.yml", source)
	if err != nil {
		t.Fatal(err)
	}
	path, err := ParseSelector(doc, "pg-meta.vars")
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := doc.Extract(path)
	if err != nil {
		t.Fatal(err)
	}
	if fragment.Kind != MappingBodyFragment || bytes.HasPrefix(fragment.Bytes, []byte("      ")) {
		t.Fatalf("unexpected mapping fragment: kind=%s bytes=%q", fragment.Kind, fragment.Bytes)
	}

	noop := doc.Edit()
	if err := noop.ReplaceFragment(fragment, fragment.Bytes); err != nil {
		t.Fatal(err)
	}
	unchanged, changes, err := noop.Commit()
	if err != nil {
		t.Fatal(err)
	}
	if len(changes.Changes) != 0 || !bytes.Equal(unchanged.Bytes(), source) {
		t.Fatal("no-op fragment edit was not byte-exact")
	}

	edited := bytes.Replace(fragment.Bytes, []byte("pg_cluster: pg-meta"), []byte("pg_cluster: pg-new"), 1)
	editor := doc.Edit()
	if err := editor.ReplaceFragment(fragment, edited); err != nil {
		t.Fatal(err)
	}
	committed, _, err := editor.Commit()
	if err != nil {
		t.Fatal(err)
	}
	want := bytes.Replace(source, []byte("pg_cluster: pg-meta"), []byte("pg_cluster: pg-new"), 1)
	if !bytes.Equal(committed.Bytes(), want) {
		t.Fatalf("fragment edit changed bytes outside selection:\n%s", committed.Bytes())
	}
}

func TestGroupSequenceAndFlowFragments(t *testing.T) {
	source := []byte(selectorFixture)
	doc, err := Parse("fragments.yml", source)
	if err != nil {
		t.Fatal(err)
	}
	groupPath, _ := ParseSelector(doc, "pg-meta")
	group, err := doc.Extract(groupPath)
	if err != nil {
		t.Fatal(err)
	}
	if group.Kind != MappingBodyFragment || !bytes.HasPrefix(group.Bytes, []byte("hosts:")) {
		t.Fatalf("unexpected group fragment: kind=%s bytes=%q", group.Kind, group.Bytes)
	}
	if bytes.Contains(group.Bytes, []byte("pg-meta:")) || !bytes.Contains(source[group.span.start:group.span.end], []byte("children:")) {
		t.Fatal("group fragment did not expose exactly the group body")
	}

	sequencePath, _ := ParseSelector(doc, "pg-meta.vars.pg_databases")
	sequence, err := doc.Extract(sequencePath)
	if err != nil {
		t.Fatal(err)
	}
	if sequence.Kind != SequenceBodyFragment || !bytes.HasPrefix(sequence.Bytes, []byte("- name: meta")) {
		t.Fatalf("unexpected sequence fragment: kind=%s bytes=%q", sequence.Kind, sequence.Bytes)
	}
	edited := bytes.Replace(sequence.Bytes, []byte("- name: meta"), []byte("- name: meta\n- name: app"), 1)
	editor := doc.Edit()
	if err := editor.ReplaceFragment(sequence, edited); err != nil {
		t.Fatal(err)
	}
	committed, _, err := editor.Commit()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(committed.Bytes(), []byte("          - name: meta\n          - name: app\n")) {
		t.Fatalf("sequence fragment was not reindented correctly:\n%s", committed.Bytes())
	}

	flowPath, _ := ParseSelector(doc, `pg-meta.hosts["10.10.10.10"]`)
	flow, err := doc.Extract(flowPath)
	if err != nil {
		t.Fatal(err)
	}
	if flow.Kind != LiteralFragment || string(flow.Bytes) != "{}" {
		t.Fatalf("unexpected empty flow fragment: %+v", flow)
	}
}

func TestScalarBlockAndStaleFragment(t *testing.T) {
	doc, err := Parse("fragment.yml", []byte(selectorFixture))
	if err != nil {
		t.Fatal(err)
	}
	regionPath, _ := ParseSelector(doc, "vars.region")
	region, err := doc.Extract(regionPath)
	if err != nil {
		t.Fatal(err)
	}
	editor := doc.Edit()
	if err := editor.ReplaceFragment(region, []byte("china\n")); err != nil {
		t.Fatal(err)
	}
	committed, _, err := editor.Commit()
	if err != nil {
		t.Fatal(err)
	}
	want := bytes.Replace([]byte(selectorFixture), []byte("region: default"), []byte("region: china"), 1)
	if !bytes.Equal(committed.Bytes(), want) {
		t.Fatalf("scalar fragment changed surrounding bytes:\n%s", committed.Bytes())
	}

	notePath, _ := ParseSelector(doc, "vars.note")
	note, err := doc.Extract(notePath)
	if err != nil {
		t.Fatal(err)
	}
	if note.Kind != BlockScalarFragment || !bytes.HasPrefix(note.Bytes, []byte("|-")) {
		t.Fatalf("unexpected block scalar fragment: %+v", note)
	}
	blockEditor := doc.Edit()
	blockEdit := bytes.Replace(note.Bytes, []byte("second"), []byte("changed"), 1)
	if err := blockEditor.ReplaceFragment(note, blockEdit); err != nil {
		t.Fatal(err)
	}
	blockCommitted, _, err := blockEditor.Commit()
	if err != nil {
		t.Fatal(err)
	}
	blockWant := bytes.Replace([]byte(selectorFixture), []byte("      second"), []byte("      changed"), 1)
	if !bytes.Equal(blockCommitted.Bytes(), blockWant) {
		t.Fatalf("block scalar edit changed surrounding bytes:\n%s", blockCommitted.Bytes())
	}

	stale := committed.Edit()
	if err := stale.ReplaceFragment(region, []byte("europe")); !errors.Is(err, ErrStaleFragment) {
		t.Fatalf("stale fragment error = %v", err)
	}
}

func TestFragmentCRLFAndProtectedNode(t *testing.T) {
	source := []byte("all:\r\n  vars:\r\n    region: default\r\n")
	doc, err := Parse("crlf.yml", source)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := doc.Extract(AllVars())
	if err != nil {
		t.Fatal(err)
	}
	editor := doc.Edit()
	if err := editor.ReplaceFragment(fragment, bytes.Replace(fragment.Bytes, []byte("default"), []byte("china"), 1)); err != nil {
		t.Fatal(err)
	}
	committed, _, err := editor.Commit()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(bytes.ReplaceAll(committed.Bytes(), []byte("\r\n"), nil), []byte("\n")) {
		t.Fatal("CRLF fragment edit introduced LF newlines")
	}

	protected, err := Parse("anchor.yml", []byte("all:\n  vars: &shared\n    region: default\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := protected.Extract(AllVars()); !errors.Is(err, ErrFragmentUnsupported) {
		t.Fatalf("protected fragment error = %v", err)
	}
}

func TestImplicitMappingFragmentPreservesLineStyle(t *testing.T) {
	tests := []struct {
		name, source, edited, want string
	}{
		{"lf", "all:\n  vars:\n", "region: china\n", "all:\n  vars:\n    region: china\n"},
		{
			"comment",
			"all:\n  vars: # keep this comment\n  children: {}\n",
			"region: china\nproxy_env:\n  http_proxy: http://proxy\n",
			"all:\n  vars: # keep this comment\n    region: china\n    proxy_env:\n      http_proxy: http://proxy\n  children: {}\n",
		},
		{"crlf", "all:\r\n  vars:\r\n", "region: china\n", "all:\r\n  vars:\r\n    region: china\r\n"},
		{"no-final-newline", "all:\n  vars:", "region: china\n", "all:\n  vars:\n    region: china"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc, err := Parse(test.name+".yml", []byte(test.source))
			if err != nil {
				t.Fatal(err)
			}
			fragment, err := doc.Extract(AllVars())
			if err != nil {
				t.Fatal(err)
			}
			if fragment.Kind != MappingBodyFragment || len(fragment.Bytes) != 0 {
				t.Fatalf("unexpected implicit mapping fragment: kind=%s bytes=%q", fragment.Kind, fragment.Bytes)
			}
			editor := doc.Edit()
			if err := editor.ReplaceFragment(fragment, []byte(test.edited)); err != nil {
				t.Fatal(err)
			}
			committed, _, err := editor.Commit()
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(committed.Bytes(), []byte(test.want)) {
				t.Fatalf("implicit mapping edit:\n got: %q\nwant: %q", committed.Bytes(), test.want)
			}
		})
	}
}
