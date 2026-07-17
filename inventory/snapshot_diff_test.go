package inventory

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func mustSnapshot(t *testing.T, name, source string) *Snapshot {
	t.Helper()
	doc, err := Parse(name, []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := SnapshotFromDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestDiffSnapshotsTracksKeysOrderAndAtomicSequences(t *testing.T) {
	base := mustSnapshot(t, "base.yml", `all:
  children: {}
  vars:
    a: remove-secret
    b:
      x: old-secret
    c: 3
    list: [one, two]
`)
	current := mustSnapshot(t, "current.yml", `all:
  children: {}
  vars:
    c: 3
    d:
      nested: added-secret
    b:
      x: new-secret
    list: [two, one]
`)
	changes, err := DiffSnapshots(base, current)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		op   SnapshotOperation
		path string
	}{
		{SnapshotRemove, "/all/vars/a"},
		{SnapshotReorder, "/all/vars"},
		{SnapshotAdd, "/all/vars/d"},
		{SnapshotReplace, "/all/vars/b/x"},
		{SnapshotReplace, "/all/vars/list"},
	}
	if len(changes) != len(want) {
		t.Fatalf("changes = %+v", changes)
	}
	for i := range want {
		if changes[i].Operation != want[i].op || changes[i].Path.String() != want[i].path {
			t.Fatalf("change[%d] = %+v, want %s %s", i, changes[i], want[i].op, want[i].path)
		}
	}
	encoded, err := json.Marshal(changes)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"remove-secret", "old-secret", "new-secret", "added-secret"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("semantic diff exposed a value: %s", encoded)
		}
	}
}

func TestDiffSnapshotsIgnoresSourcePresentation(t *testing.T) {
	left := mustSnapshot(t, "left.yml", "# left\nall: {children: {}, vars: {region: 'china'}}\n")
	right := mustSnapshot(t, "right.yml", "all:\n  children: {}\n  vars:\n    region: \"china\" # right\n")
	changes, err := DiffSnapshots(left, right)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Fatalf("source-only changes became semantic changes: %+v", changes)
	}
}

func TestDiffSnapshotsMappingAndSequenceOrderAreSemantic(t *testing.T) {
	mappingBase := mustSnapshot(t, "mapping-base.yml", "all:\n  children: {}\n  vars: {a: 1, b: 2}\n")
	mappingCurrent := mustSnapshot(t, "mapping-current.yml", "all:\n  children: {}\n  vars: {b: 2, a: 1}\n")
	changes, err := DiffSnapshots(mappingBase, mappingCurrent)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Operation != SnapshotReorder || changes[0].Path.String() != "/all/vars" {
		t.Fatalf("mapping order changes = %+v", changes)
	}

	sequenceBase := mustSnapshot(t, "sequence-base.yml", "all:\n  children: {}\n  vars: {items: [a, b]}\n")
	sequenceCurrent := mustSnapshot(t, "sequence-current.yml", "all:\n  children: {}\n  vars: {items: [b, a]}\n")
	changes, err = DiffSnapshots(sequenceBase, sequenceCurrent)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Operation != SnapshotReplace || changes[0].Path.String() != "/all/vars/items" {
		t.Fatalf("sequence order changes = %+v", changes)
	}
}

func TestDiffSnapshotAdditionCarriesExactBaseGap(t *testing.T) {
	base := mustSnapshot(t, "base.yml", "all:\n  children: {}\n  vars: {a: 1, b: 2, c: 3}\n")
	current := mustSnapshot(t, "current.yml", "all:\n  children: {}\n  vars: {a: 1, x: 8, y: 9, b: 2, c: 3}\n")
	deltas, err := diffSnapshotValues(base, current)
	if err != nil {
		t.Fatal(err)
	}
	if len(deltas) != 2 {
		t.Fatalf("deltas = %+v", deltas)
	}
	for index, delta := range deltas {
		if delta.operation != SnapshotAdd || !delta.gap.hasPrevious || delta.gap.previous != "a" ||
			!delta.gap.hasNext || delta.gap.next != "b" || delta.gap.ordinal != index {
			t.Fatalf("delta[%d] gap = %+v", index, delta.gap)
		}
	}
}

func TestSnapshotRemovalsClassifiesExactPruneScope(t *testing.T) {
	tests := []struct {
		name      string
		base      string
		candidate string
		want      []SnapshotChange
	}{
		{
			name:      "mapping declaration removed",
			base:      "all:\n  vars:\n    keep: 1\n    password: secret\n",
			candidate: "all:\n  vars:\n    keep: 1\n",
			want:      []SnapshotChange{{Operation: SnapshotRemove, Path: Keys("all", "vars", "password")}},
		},
		{
			name:      "scalar replacement is not prune",
			base:      "all:\n  vars:\n    value: 1\n",
			candidate: "all:\n  vars:\n    value: 2\n",
		},
		{
			name:      "sequence append preserves every element",
			base:      "all:\n  vars:\n    values: [one]\n",
			candidate: "all:\n  vars:\n    values: [one, two]\n",
		},
		{
			name:      "sequence reorder preserves every element",
			base:      "all:\n  vars:\n    values: [one, two]\n",
			candidate: "all:\n  vars:\n    values: [two, one]\n",
		},
		{
			name:      "sequence element removal is prune",
			base:      "all:\n  vars:\n    values: [one, two]\n",
			candidate: "all:\n  vars:\n    values: [one]\n",
			want:      []SnapshotChange{{Operation: SnapshotReplace, Path: Keys("all", "vars", "values")}},
		},
		{
			name:      "sequence duplicate removal is prune",
			base:      "all:\n  vars:\n    values: [one, one]\n",
			candidate: "all:\n  vars:\n    values: [one]\n",
			want:      []SnapshotChange{{Operation: SnapshotReplace, Path: Keys("all", "vars", "values")}},
		},
		{
			name:      "sequence kind replacement can discard elements",
			base:      "all:\n  vars:\n    values: [one]\n",
			candidate: "all:\n  vars:\n    values: replacement\n",
			want:      []SnapshotChange{{Operation: SnapshotReplace, Path: Keys("all", "vars", "values")}},
		},
		{
			name:      "mapping replacement can discard declarations",
			base:      "all:\n  vars:\n    nested:\n      key: value\n",
			candidate: "all:\n  vars:\n    nested: replacement\n",
			want:      []SnapshotChange{{Operation: SnapshotReplace, Path: Keys("all", "vars", "nested")}},
		},
		{
			name:      "ordered mapping move is not prune",
			base:      "all:\n  vars:\n    one: 1\n    two: 2\n",
			candidate: "all:\n  vars:\n    two: 2\n    one: 1\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			base := mustSnapshot(t, "base.yml", test.base)
			candidate := mustSnapshot(t, "candidate.yml", test.candidate)
			got, err := SnapshotRemovals(base, candidate)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(test.want) {
				t.Fatalf("removals = %+v, want %+v", got, test.want)
			}
			for i := range got {
				if got[i].Operation != test.want[i].Operation || got[i].Path.String() != test.want[i].Path.String() {
					t.Fatalf("removals = %+v, want %+v", got, test.want)
				}
			}
			encoded, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(encoded, []byte("secret")) || bytes.Contains(encoded, []byte("replacement")) {
				t.Fatalf("prune plan leaked values: %s", encoded)
			}
		})
	}
}
