package inventory

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func mustDocument(t *testing.T, name, source string) *Document {
	t.Helper()
	doc, err := Parse(name, []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func mergeFixture(t *testing.T, baseSource, workingSource, currentSource string) *SnapshotMergeResult {
	t.Helper()
	baseDoc := mustDocument(t, "base.yml", baseSource)
	base, err := SnapshotFromDocument(baseDoc)
	if err != nil {
		t.Fatal(err)
	}
	currentDoc := mustDocument(t, "current.yml", currentSource)
	current, err := SnapshotFromDocument(currentDoc)
	if err != nil {
		t.Fatal(err)
	}
	result, err := MergeSnapshots(base, mustDocument(t, "working.yml", workingSource), current)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestMergeSnapshotsAppliesDisjointChangeToLosslessWorkingSource(t *testing.T) {
	base := `all:
  children: {}
  vars:
    region      : default  # region
    port        : 5432     # port
`
	working := `# keep leading comment
all:
  children: {} # keep children
  vars:
    region      : china    # working edit
    port        : 5432     # port
`
	current := `all:
  children: {}
  vars:
    region: default
    port: 6432
`
	result := mergeFixture(t, base, working, current)
	if !result.Clean() || len(result.Conflicts) != 0 {
		t.Fatalf("merge result = %+v", result)
	}
	want := `# keep leading comment
all:
  children: {} # keep children
  vars:
    region      : china    # working edit
    port        : 6432     # port
`
	if got := string(result.Document().Bytes()); got != want {
		t.Fatalf("lossless merge mismatch\n got:\n%s\nwant:\n%s", got, want)
	}
	if len(result.Changes) != 1 || result.Changes[0].Operation != SnapshotReplace ||
		result.Changes[0].Path.String() != "/all/vars/port" {
		t.Fatalf("merge changes = %+v", result.Changes)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"china", "6432", "working edit"} {
		if bytes.Contains(encoded, []byte(value)) {
			t.Fatalf("clean merge metadata exposed source values: %s", encoded)
		}
	}
}

func TestMergeSnapshotsUsesDistinctAdditionGapsAndCanonicalFragments(t *testing.T) {
	base := "all:\n  children: {}\n  vars:\n    a: 1\n    b: 2\n    c: 3\n"
	working := "all:\n  children: {}\n  vars:\n    a: 1\n    x: working\n    b: 2\n    c: 3\n"
	current := "all:\n  children: {}\n  vars:\n    a: 1\n    b: 2\n    y:\n      nested: [one, two]\n    c: 3\n"
	result := mergeFixture(t, base, working, current)
	if !result.Clean() {
		t.Fatalf("distinct-gap merge conflicts = %+v", result.Conflicts)
	}
	want := "all:\n  children: {}\n  vars:\n    a: 1\n    x: working\n    b: 2\n    \"y\": {\"nested\":[\"one\",\"two\"]}\n    c: 3\n"
	if got := string(result.Document().Bytes()); got != want {
		t.Fatalf("addition merge mismatch\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestMergeSnapshotsAddsAtBeginningAndIntoEmptyFlowMapping(t *testing.T) {
	base := "all:\n  children: {}\n  vars:\n    b: 2\n"
	current := "all:\n  children: {}\n  vars:\n    a: 1\n    b: 2\n"
	result := mergeFixture(t, base, base, current)
	if !result.Clean() || string(result.Document().Bytes()) != "all:\n  children: {}\n  vars:\n    a: 1\n    b: 2\n" {
		t.Fatalf("beginning insertion = %q, conflicts=%+v", result.Document().Bytes(), result.Conflicts)
	}

	emptyBase := "all:\n  children: {}\n  vars: {}\n"
	emptyCurrent := "all:\n  children: {}\n  vars: {\"\": value}\n"
	result = mergeFixture(t, emptyBase, emptyBase, emptyCurrent)
	if !result.Clean() || string(result.Document().Bytes()) != "all:\n  children: {}\n  vars: { \"\": \"value\" }\n" {
		t.Fatalf("empty flow insertion = %q, conflicts=%+v", result.Document().Bytes(), result.Conflicts)
	}

	emptyAnchorBase := "all:\n  children: {}\n  vars: {\"\": 0, b: 2}\n"
	emptyAnchorCurrent := "all:\n  children: {}\n  vars: {a: 1, \"\": 0, b: 2}\n"
	result = mergeFixture(t, emptyAnchorBase, emptyAnchorBase, emptyAnchorCurrent)
	if !result.Clean() || string(result.Document().Bytes()) != "all:\n  children: {}\n  vars: {a: 1, \"\": 0, b: 2}\n" {
		t.Fatalf("empty-key anchor insertion = %q, conflicts=%+v", result.Document().Bytes(), result.Conflicts)
	}
}

func TestMergeSnapshotsPreservesOrderForMultipleBeginningAdditions(t *testing.T) {
	base := "all:\n  children: {}\n  vars:\n    c: 3\n"
	current := "all:\n  children: {}\n  vars:\n    a: 1\n    b: 2\n    c: 3\n"
	result := mergeFixture(t, base, base, current)
	if !result.Clean() {
		t.Fatalf("multiple beginning additions conflicts = %+v", result.Conflicts)
	}
	if got := string(result.Document().Bytes()); got != current {
		t.Fatalf("multiple beginning additions = %q, want %q", got, current)
	}
}

func TestMergeSnapshotsPreservesOrderForMultipleFlowAdditions(t *testing.T) {
	base := "all:\n  children: {}\n  vars: {a: 1}\n"
	current := "all:\n  children: {}\n  vars: {a: 1, x: 8, y: 9}\n"
	result := mergeFixture(t, base, base, current)
	if !result.Clean() {
		t.Fatalf("multiple flow additions conflicts = %+v", result.Conflicts)
	}
	want := "all:\n  children: {}\n  vars: {a: 1, x: 8, \"y\": 9}\n"
	if got := string(result.Document().Bytes()); got != want {
		t.Fatalf("multiple flow additions = %q, want %q", got, want)
	}
}

func TestMergeSnapshotsSameGapAdditionConflictsWithoutValues(t *testing.T) {
	base := "all:\n  children: {}\n  vars: {a: 1, b: 2}\n"
	working := "all:\n  children: {}\n  vars: {a: 1, working_key: working-secret, b: 2}\n"
	current := "all:\n  children: {}\n  vars: {a: 1, current_key: current-secret, b: 2}\n"
	result := mergeFixture(t, base, working, current)
	if result.Clean() || result.Document() != nil || len(result.Conflicts) == 0 {
		t.Fatalf("same-gap additions did not conflict: %+v", result)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"working-secret", "current-secret"} {
		if bytes.Contains(encoded, []byte(secret)) {
			t.Fatalf("conflict output exposed a value: %s", encoded)
		}
	}
}

func TestMergeSnapshotsIdenticalSameGapAdditionsAreIdempotent(t *testing.T) {
	base := "all:\n  children: {}\n  vars: {a: 1, b: 2}\n"
	added := "# working presentation\nall:\n  children: {}\n  vars: {a: 1, x: same, y: 9, b: 2}\n"
	current := "all:\n  children: {}\n  vars: {a: 1, x: same, y: 9, b: 2}\n"
	result := mergeFixture(t, base, added, current)
	if !result.Clean() || len(result.Changes) != 0 || string(result.Document().Bytes()) != added {
		t.Fatalf("identical additions changed working source: clean=%v changes=%+v body=%q", result.Clean(), result.Changes, result.Document().Bytes())
	}
}

func TestMergeSnapshotsAnchorRemovalAndRelatedChangesConflict(t *testing.T) {
	tests := []struct {
		name    string
		base    string
		working string
		current string
	}{
		{
			name:    "removed-gap-anchor",
			base:    "all:\n  children: {}\n  vars: {a: 1, b: 2, c: 3}\n",
			working: "all:\n  children: {}\n  vars: {a: 1, c: 3}\n",
			current: "all:\n  children: {}\n  vars: {a: 1, b: 2, x: 9, c: 3}\n",
		},
		{
			name:    "ancestor-descendant",
			base:    "all:\n  children: {}\n  vars: {node: {x: 1, y: 2}}\n",
			working: "all:\n  children: {}\n  vars: {node: replaced}\n",
			current: "all:\n  children: {}\n  vars: {node: {x: 9, y: 2}}\n",
		},
		{
			name:    "remove-vs-change",
			base:    "all:\n  children: {}\n  vars: {node: {x: 1}}\n",
			working: "all:\n  children: {}\n  vars: {}\n",
			current: "all:\n  children: {}\n  vars: {node: {x: 2}}\n",
		},
		{
			name:    "same-path-different-value",
			base:    "all:\n  children: {}\n  vars: {x: 1}\n",
			working: "all:\n  children: {}\n  vars: {x: 2}\n",
			current: "all:\n  children: {}\n  vars: {x: 3}\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := mergeFixture(t, test.base, test.working, test.current)
			if result.Clean() || result.Document() != nil || len(result.Conflicts) == 0 {
				t.Fatalf("expected conflict: %+v", result)
			}
		})
	}
}

func TestMergeSnapshotsEqualConcurrentChangeIsClean(t *testing.T) {
	base := "all:\n  children: {}\n  vars: {x: 1}\n"
	working := "# keep\nall:\n  children: {}\n  vars: {x: 2}\n"
	current := "all:\n  children: {}\n  vars: {x: 2}\n"
	result := mergeFixture(t, base, working, current)
	if !result.Clean() || len(result.Changes) != 0 || string(result.Document().Bytes()) != working {
		t.Fatalf("equal concurrent edit was not idempotent: %+v", result)
	}
}

func TestMergeSnapshotsReordersExactBlockEntrySpans(t *testing.T) {
	base := `all:
  children: {}
  vars:
    a    : 1  # alpha

    b    : 2  # beta
    c    : 3  # gamma
`
	current := `all:
  children: {}
  vars:
    c: 3
    a: 1
    b: 2
`
	result := mergeFixture(t, base, base, current)
	if !result.Clean() {
		t.Fatalf("block reorder conflicts = %+v", result.Conflicts)
	}
	want := `all:
  children: {}
  vars:
    c    : 3  # gamma
    a    : 1  # alpha

    b    : 2  # beta
`
	if got := string(result.Document().Bytes()); got != want {
		t.Fatalf("block entry spans were not moved exactly\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestMergeSnapshotsRemovesThenReordersSurvivingEntries(t *testing.T) {
	base := "all:\n  children: {}\n  vars:\n    a: 1\n    b: 2\n    c: 3\n"
	current := "all:\n  children: {}\n  vars:\n    c: 3\n    a: 1\n"
	result := mergeFixture(t, base, base, current)
	if !result.Clean() || string(result.Document().Bytes()) != current {
		t.Fatalf("remove/reorder merge = %q, conflicts=%+v", result.Document().Bytes(), result.Conflicts)
	}
	if len(result.Changes) != 2 || result.Changes[0].Operation != SnapshotRemove || result.Changes[1].Operation != SnapshotReorder {
		t.Fatalf("remove/reorder changes = %+v", result.Changes)
	}
}

func TestMergeSnapshotsReordersFlowMappingWithPositionalSeparators(t *testing.T) {
	base := "all:\n  children: {}\n  vars: { a: 1,b: 2 , c: 3 }\n"
	current := "all:\n  children: {}\n  vars: {c: 3, a: 1, b: 2}\n"
	result := mergeFixture(t, base, base, current)
	if !result.Clean() {
		t.Fatalf("flow reorder conflicts = %+v", result.Conflicts)
	}
	want := "all:\n  children: {}\n  vars: { c: 3,a: 1 , b: 2 }\n"
	if got := string(result.Document().Bytes()); got != want {
		t.Fatalf("flow reorder = %q, want %q", got, want)
	}
}

func TestMergeSnapshotsReorderConflictsWithDescendantChange(t *testing.T) {
	base := "all:\n  children: {}\n  vars: {a: 1, b: 2}\n"
	working := "all:\n  children: {}\n  vars: {a: 9, b: 2}\n"
	current := "all:\n  children: {}\n  vars: {b: 2, a: 1}\n"
	result := mergeFixture(t, base, working, current)
	if result.Clean() || len(result.Conflicts) == 0 {
		t.Fatalf("reorder/descendant change did not conflict: %+v", result)
	}
}

func TestMergeSnapshotsUnsafeCommentAttachmentFailsClosed(t *testing.T) {
	base := "all:\n  children: {}\n  vars:\n    a: 1\n    b: 2\n"
	working := "all:\n  children: {}\n  vars:\n    a: 1\n    # belongs with b\n    b: 2\n"
	current := "all:\n  children: {}\n  vars:\n    b: 2\n    a: 1\n"
	baseSnapshot, err := SnapshotFromDocument(mustDocument(t, "base.yml", base))
	if err != nil {
		t.Fatal(err)
	}
	currentSnapshot, err := SnapshotFromDocument(mustDocument(t, "current.yml", current))
	if err != nil {
		t.Fatal(err)
	}
	result, err := MergeSnapshots(baseSnapshot, mustDocument(t, "working.yml", working), currentSnapshot)
	if !errors.Is(err, ErrSnapshotMergeUnsafe) || result != nil {
		t.Fatalf("unsafe comment merge = result %+v, error %v", result, err)
	}
	if strings.Contains(err.Error(), "belongs with b") {
		t.Fatalf("unsafe merge error exposed comment content: %v", err)
	}
}

func TestMergeSnapshotsAtomicallyReplacesSequenceWithCanonicalFragment(t *testing.T) {
	base := "# keep\nall:\n  children: {}\n  vars:\n    items:\n      - old\n      - values\n    untouched: yes-string\n"
	current := "all:\n  children: {}\n  vars:\n    items:\n      - new\n      - 2\n    untouched: yes-string\n"
	result := mergeFixture(t, base, base, current)
	if !result.Clean() {
		t.Fatalf("sequence replacement conflicts = %+v", result.Conflicts)
	}
	want := "# keep\nall:\n  children: {}\n  vars:\n    items:\n      [\"new\",2]\n    untouched: yes-string\n"
	if got := string(result.Document().Bytes()); got != want {
		t.Fatalf("sequence replacement\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestMergeSnapshotsPreservesFloatTypeAndReplacesImplicitNull(t *testing.T) {
	base := "all:\n  children: {}\n  vars:\n    number: 1\n    nested:\n"
	current := "all:\n  children: {}\n  vars:\n    number: 1.0\n    nested:\n      key: value\n"
	result := mergeFixture(t, base, base, current)
	if !result.Clean() {
		t.Fatalf("typed replacement conflicts = %+v", result.Conflicts)
	}
	want := "all:\n  children: {}\n  vars:\n    number: 1.0\n    nested: {\"key\":\"value\"}\n"
	if got := string(result.Document().Bytes()); got != want {
		t.Fatalf("typed replacement\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestMergeSnapshotsReplacesBlockScalarWithoutTouchingSiblingBytes(t *testing.T) {
	base := "all:\n  children: {}\n  vars:\n    note: |\n      old secret\n      second line\n    sibling : keep  # exact\n"
	current := "all:\n  children: {}\n  vars:\n    note: new value\n    sibling: keep\n"
	result := mergeFixture(t, base, base, current)
	if !result.Clean() {
		t.Fatalf("block scalar replacement conflicts = %+v", result.Conflicts)
	}
	want := "all:\n  children: {}\n  vars:\n    note: \"new value\"\n    sibling : keep  # exact\n"
	if got := string(result.Document().Bytes()); got != want {
		t.Fatalf("block scalar replacement\n got:\n%s\nwant:\n%s", got, want)
	}
}
