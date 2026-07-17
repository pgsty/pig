package inventory

import (
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
