package ext

import (
	"reflect"
	"testing"
)

func TestParseAptDependsOutput(t *testing.T) {
	input := `
Depends: libc6
Depends: libssl3
 |Depends: libfoo1
PreDepends: dpkg
Suggests: foo-doc
<none>
`
	got := parseAptDependsOutput(input)
	want := []string{"libc6", "libssl3", "libfoo1", "dpkg", "foo-doc"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseAptDependsOutput() = %#v, want %#v", got, want)
	}
}

func TestMergeDEBCandidates(t *testing.T) {
	pkgs := []string{"pkgA", "pkgB", "pkgA"}
	deps := map[string][]string{
		"pkgA": {"dep1", "dep2", "dep1"},
		"pkgB": {"dep2", "dep3"},
	}

	got := mergeDEBCandidates(pkgs, deps)
	want := []string{"pkgA", "pkgB", "dep1", "dep2", "dep3"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("mergeDEBCandidates() = %#v, want %#v", got, want)
	}
}
