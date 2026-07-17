package inventory

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestCanonicalYAMLMatchesVersionedFixture(t *testing.T) {
	snapshot, err := SnapshotFromDocument(loadSnapshotFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	output, err := snapshot.CanonicalYAML()
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "snapshot", "fidelity.canonical.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(output, want) {
		t.Fatalf("canonical YAML mismatch\n got:\n%s\nwant:\n%s", output, want)
	}
	if bytes.HasPrefix(output, []byte("---")) || bytes.Contains(output, []byte("!!")) ||
		!bytes.HasSuffix(output, []byte("\n")) || bytes.HasSuffix(output, []byte("\n\n")) {
		t.Fatalf("canonical YAML framing mismatch: %q", output)
	}

	doc, err := Parse("round-trip.yml", output)
	if err != nil {
		t.Fatal(err)
	}
	roundTrip, err := SnapshotFromDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.Equal(roundTrip) {
		t.Fatalf("canonical YAML digest = %s, want %s", roundTrip.Digest(), snapshot.Digest())
	}
}

func TestCanonicalYAMLRendersNestedCollectionsAndAmbiguousScalars(t *testing.T) {
	source := []byte(`all:
  children: {}
  vars:
    empty_map: {}
    empty_seq: []
    nested:
      - name: "null"
        flags:
          - true
          - 1.0
      - -0.0
    exponent: 1.0e+20
`)
	doc, err := Parse("nested.yml", source)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := SnapshotFromDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	output, err := snapshot.CanonicalYAML()
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range [][]byte{
		[]byte(`"empty_map": {}`),
		[]byte(`"empty_seq": []`),
		[]byte(`"name": "null"`),
		[]byte(`- 1.0`),
		[]byte(`- -0.0`),
		[]byte(`"exponent": 1e+20`),
	} {
		if !bytes.Contains(output, fragment) {
			t.Fatalf("canonical YAML does not contain %q:\n%s", fragment, output)
		}
	}
}

func TestCanonicalYAMLEmptyRoot(t *testing.T) {
	snapshot, err := SnapshotFromDocument(&Document{
		source: []byte("{}\n"),
		root:   &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"},
	})
	if err != nil {
		t.Fatal(err)
	}
	output, err := snapshot.CanonicalYAML()
	if err != nil {
		t.Fatal(err)
	}
	if string(output) != "{}\n" {
		t.Fatalf("empty root = %q", output)
	}
}
