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
