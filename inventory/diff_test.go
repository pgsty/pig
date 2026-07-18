package inventory

import (
	"reflect"
	"testing"
)

func TestDiffDocumentsIgnoresMappingOrderAndFormatting(t *testing.T) {
	left, err := Parse("left.yml", []byte("all:\n  vars: { region: default, version: v4.4.0 }\n  children: {}\n"))
	if err != nil {
		t.Fatal(err)
	}
	right, err := Parse("right.yml", []byte("all:\n  children: { }\n  vars:\n    version: v4.4.0\n    region: default\n"))
	if err != nil {
		t.Fatal(err)
	}
	changes, err := DiffDocuments(left, right)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Fatalf("presentation-only diff = %+v", changes)
	}
}

func TestDiffDocumentsIsValueFreeAndSequenceAtomic(t *testing.T) {
	left, err := Parse("left.yml", []byte(`all:
  vars:
    password: old-secret
    remove_me: true
    ports: [1, 2]
  children: {}
`))
	if err != nil {
		t.Fatal(err)
	}
	right, err := Parse("right.yml", []byte(`all:
  vars:
    password: new-secret
    add_me: false
    ports: [2, 1]
  children: {}
`))
	if err != nil {
		t.Fatal(err)
	}
	changes, err := DiffDocuments(left, right)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"replace /all/vars/password scalar",
		"remove /all/vars/remove_me scalar",
		"replace /all/vars/ports sequence",
		"add /all/vars/add_me scalar",
	}
	got := make([]string, 0, len(changes))
	for _, change := range changes {
		got = append(got, change.Operation+" "+change.Path.String()+" "+string(change.ValueKind))
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("diff = %v, want %v", got, want)
	}
}
