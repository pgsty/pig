package inventory

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func loadSnapshotFixture(t *testing.T) *Document {
	t.Helper()
	path := filepath.Join("testdata", "snapshot", "fidelity.yml")
	doc, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestSnapshotCanonicalTypedEncodingAndDigest(t *testing.T) {
	snapshot, err := SnapshotFromDocument(loadSnapshotFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Kind() != SnapshotKind || snapshot.NodeCount() != 27 {
		t.Fatalf("kind/count = %q/%d", snapshot.Kind(), snapshot.NodeCount())
	}
	canonical := snapshot.CanonicalBytes()
	wantCanonical, err := os.ReadFile(filepath.Join("testdata", "snapshot", "fidelity.snapshot.json"))
	if err != nil {
		t.Fatal(err)
	}
	wantCanonical = bytes.TrimSuffix(wantCanonical, []byte("\n"))
	if !bytes.Equal(canonical, wantCanonical) {
		t.Fatalf("canonical fixture mismatch\n got: %s\nwant: %s", canonical, wantCanonical)
	}
	if bytes.HasSuffix(canonical, []byte("\n")) || bytes.Contains(canonical, []byte(" ")) {
		t.Fatalf("canonical JSON contains whitespace: %q", canonical)
	}
	for _, fragment := range []string{
		`{"t":"float","v":"1"}`,
		`{"t":"float","v":"-0"}`,
		`{"t":"int","v":"18446744073709551615"}`,
		`{"t":"str","v":"true"}`,
		`{"t":"null"}`,
	} {
		if !bytes.Contains(canonical, []byte(fragment)) {
			t.Fatalf("canonical JSON does not contain %s: %s", fragment, canonical)
		}
	}
	if snapshot.Digest() == "" || snapshot.SemanticSHA256() == [32]byte{} {
		t.Fatal("semantic digest was not populated")
	}

	decoded, err := ParseSnapshot(canonical)
	if err != nil {
		t.Fatalf("ParseSnapshot: %v", err)
	}
	if !snapshot.Equal(decoded) || snapshot.Digest() != decoded.Digest() {
		t.Fatalf("Snapshot round trip changed digest: %s != %s", snapshot.Digest(), decoded.Digest())
	}
	marshaled, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(marshaled, canonical) {
		t.Fatal("MarshalJSON did not preserve canonical bytes")
	}

	root := snapshot.Root()
	entries := root.Entries()
	entries[0] = SnapshotEntry{}
	if rootAgain := snapshot.Root(); len(rootAgain.Entries()) == 0 || rootAgain.Entries()[0].Key() != "all" {
		t.Fatal("Snapshot accessors exposed mutable storage")
	}
}

func TestSnapshotDeclarationOrderChangesDigest(t *testing.T) {
	left, err := Parse("left.yml", []byte("all:\n  children: {}\n  vars:\n    first: 1\n    second: 2\n"))
	if err != nil {
		t.Fatal(err)
	}
	right, err := Parse("right.yml", []byte("all:\n  children: {}\n  vars:\n    second: 2\n    first: 1\n"))
	if err != nil {
		t.Fatal(err)
	}
	leftSnapshot, err := SnapshotFromDocument(left)
	if err != nil {
		t.Fatal(err)
	}
	rightSnapshot, err := SnapshotFromDocument(right)
	if err != nil {
		t.Fatal(err)
	}
	if leftSnapshot.Equal(rightSnapshot) || leftSnapshot.Digest() == rightSnapshot.Digest() {
		t.Fatal("mapping reordering did not change the semantic digest")
	}
}

func TestSnapshotJSONEscapingDoesNotEscapeHTML(t *testing.T) {
	doc, err := Parse("escaping.yml", []byte("all:\n  children: {}\n  vars:\n    text: \"<tag>&\\u2028line\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := SnapshotFromDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	canonical := snapshot.CanonicalBytes()
	if bytes.Contains(canonical, []byte(`\u003c`)) || bytes.Contains(canonical, []byte(`\u003e`)) || bytes.Contains(canonical, []byte(`\u0026`)) {
		t.Fatalf("HTML characters were escaped: %s", canonical)
	}
	if !bytes.Contains(canonical, []byte(`<tag>&\u2028line`)) {
		t.Fatalf("canonical escaping mismatch: %s", canonical)
	}
}

func TestSnapshotRepresentabilityFailsClosedAndRedactsValues(t *testing.T) {
	tests := []struct {
		name   string
		source string
		reason SnapshotReason
	}{
		{"explicit-tag", "all:\n  children: {}\n  vars:\n    secret: !!str sentinel-value\n", SnapshotReasonExplicitTag},
		{"custom-tag", "all:\n  children: {}\n  vars:\n    secret: !vault sentinel-value\n", SnapshotReasonExplicitTag},
		{"anchor", "all:\n  children: {}\n  vars:\n    secret: &shared sentinel-value\n", SnapshotReasonAnchor},
		{"alias", "all:\n  children: {}\n  vars:\n    first: &shared sentinel-value\n    secret: *shared\n", SnapshotReasonAnchor},
		{"merge-key", "all:\n  children: {}\n  vars:\n    base: &base {secret: sentinel-value}\n    merged:\n      <<: *base\n", SnapshotReasonAnchor},
		{"non-string-key", "all:\n  children: {}\n  vars:\n    7: sentinel-value\n", SnapshotReasonMappingKeyType},
		{"non-finite", "all:\n  children: {}\n  vars:\n    secret: .inf\n", SnapshotReasonFloatNonFinite},
		{"integer-range", "all:\n  children: {}\n  vars:\n    secret: 18446744073709551616\n", SnapshotReasonIntegerRange},
		{"timestamp", "all:\n  children: {}\n  vars:\n    secret: 2026-07-17\n", SnapshotReasonUnknownScalar},
		{"duplicate-host", "all:\n  children:\n    blue:\n      hosts:\n        same:\n          secret: first\n    green:\n      hosts:\n        same:\n          secret: sentinel-value\n  vars: {}\n", SnapshotReasonDuplicateHostVars},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc, err := Parse(test.name+".yml", []byte(test.source))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			_, err = SnapshotFromDocument(doc)
			var snapshotErr *SnapshotError
			if !errors.As(err, &snapshotErr) {
				t.Fatalf("error = %v, want SnapshotError", err)
			}
			if snapshotErr.Code != "CMDB_SNAPSHOT_UNREPRESENTABLE" || snapshotErr.Reason != test.reason {
				t.Fatalf("SnapshotError = %+v, want reason %s", snapshotErr, test.reason)
			}
			if strings.Contains(err.Error(), "sentinel-value") {
				t.Fatalf("error exposed rejected value: %v", err)
			}
		})
	}
}

func TestSnapshotRejectsDuplicateMappingKeys(t *testing.T) {
	key := func() *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "secret"} }
	value := func(text string) *yaml.Node {
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: text}
	}
	doc := &Document{
		source: []byte("synthetic duplicate mapping"),
		root: &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
			key(), value("first"), key(), value("sentinel-value"),
		}},
	}
	_, err := SnapshotFromDocument(doc)
	var snapshotErr *SnapshotError
	if !errors.As(err, &snapshotErr) || snapshotErr.Reason != SnapshotReasonDuplicateKey {
		t.Fatalf("duplicate mapping error = %v", err)
	}
	if strings.Contains(err.Error(), "sentinel-value") || strings.Contains(err.Error(), "first") {
		t.Fatalf("duplicate mapping error exposed a value: %v", err)
	}
}

func TestSnapshotConflictingDuplicateHostsUseTypedOrder(t *testing.T) {
	for _, vars := range []string{
		"port: 5432\n          enabled: true",
		"enabled: true\n          port: 5432",
	} {
		source := "all:\n  children:\n    blue:\n      hosts:\n        same:\n          port: 5432\n          enabled: true\n    green:\n      hosts:\n        same:\n          " + vars + "\n  vars: {}\n"
		doc, err := Parse("duplicates.yml", []byte(source))
		if err != nil {
			t.Fatal(err)
		}
		_, err = SnapshotFromDocument(doc)
		if strings.HasPrefix(vars, "port") {
			if err != nil {
				t.Fatalf("identical host declarations rejected: %v", err)
			}
			continue
		}
		var snapshotErr *SnapshotError
		if !errors.As(err, &snapshotErr) || snapshotErr.Reason != SnapshotReasonDuplicateHostVars {
			t.Fatalf("ordered duplicate host error = %v", err)
		}
	}
}

func TestSnapshotProjectionIsDeclarationOnlyAndDeterministic(t *testing.T) {
	snapshot, err := SnapshotFromDocument(loadSnapshotFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	projection, err := snapshot.AnsibleInventoryJSON()
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "snapshot", "fidelity.ansible.json"))
	if err != nil {
		t.Fatal(err)
	}
	want = bytes.TrimSuffix(want, []byte("\n"))
	if !bytes.Equal(projection, want) {
		t.Fatalf("projection mismatch\n got: %s\nwant: %s", projection, want)
	}
	if bytes.Count(projection, []byte(`"node.example":{`)) != 1 {
		t.Fatalf("duplicate hostvars entry was not collapsed: %s", projection)
	}
	if bytes.Contains(projection, []byte(`"region":"china","port"`)) {
		t.Fatalf("group vars were synthesized into hostvars: %s", projection)
	}
}

func TestSnapshotAnsibleHostProjection(t *testing.T) {
	doc := mustDocument(t, "host.yml", "all:\n  hosts:\n    db-a:\n      role: primary\n      password: secret\n")
	snapshot, err := SnapshotFromDocument(doc)
	if err != nil {
		t.Fatal(err)
	}

	host, err := snapshot.AnsibleHostJSON("db-a")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(host), `{"role":"primary","password":"secret"}`; got != want {
		t.Fatalf("host projection = %s, want %s", got, want)
	}

	missing, err := snapshot.AnsibleHostJSON("missing")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(missing), `{}`; got != want {
		t.Fatalf("missing host projection = %s, want %s", got, want)
	}
}

func TestParseSnapshotRejectsNonCanonicalTypedShapes(t *testing.T) {
	valid, err := SnapshotFromDocument(loadSnapshotFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	reordered := bytes.Replace(valid.CanonicalBytes(), []byte(`{"kind":"pig.inventory.snapshot/v1","root":`), []byte(`{"root":`), 1)
	reordered = append(reordered[:len(reordered)-1], []byte(`,"kind":"pig.inventory.snapshot/v1"}`)...)
	parsed, err := ParseSnapshot(reordered)
	if err != nil || !parsed.Equal(valid) {
		t.Fatalf("field-order-independent parse failed: %v", err)
	}

	invalid := []string{
		`{"kind":"pig.inventory.snapshot/v2","root":{"t":"map","v":[]}}`,
		`{"kind":"pig.inventory.snapshot/v1","root":{"t":"int","v":"1"}}`,
		`{"kind":"pig.inventory.snapshot/v1","root":{"t":"map","v":[]},"extra":true}`,
		`{"kind":"pig.inventory.snapshot/v1","root":{"t":"map","v":[{"k":"x","v":{"t":"int","v":"01"}}]}}`,
		`{"kind":"pig.inventory.snapshot/v1","root":{"t":"map","v":[{"k":"x","v":{"t":"float","v":"1.0"}}]}}`,
		`{"kind":"pig.inventory.snapshot/v1","root":{"t":"map","v":[{"k":"x","v":{"t":"null","v":null}}]}}`,
		`{"kind":"pig.inventory.snapshot/v1","root":{"t":"map","v":[{"k":"x","v":{"t":"null"}},{"k":"x","v":{"t":"null"}}]}}`,
	}
	for _, source := range invalid {
		if _, err := ParseSnapshot([]byte(source)); err == nil {
			t.Fatalf("invalid Snapshot was accepted: %s", source)
		}
	}
}

func TestSnapshotSourceLimit(t *testing.T) {
	doc := &Document{source: make([]byte, maxSnapshotSourceBytes+1), root: &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}}
	_, err := SnapshotFromDocument(doc)
	var snapshotErr *SnapshotError
	if !errors.As(err, &snapshotErr) || snapshotErr.Reason != SnapshotReasonSourceTooLarge {
		t.Fatalf("source limit error = %v", err)
	}
}

func TestSnapshotFixtureIsReadable(t *testing.T) {
	if _, err := os.Stat(filepath.Join("testdata", "snapshot", "fidelity.yml")); err != nil {
		t.Fatal(err)
	}
}
