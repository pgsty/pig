package ext

import (
	"os"
	"path/filepath"
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

func TestRunCommandInDirDoesNotChangeProcessWorkingDir(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}

	workDir := t.TempDir()
	if err := runCommandInDir([]string{"sh", "-c", "touch marker.file"}, workDir); err != nil {
		t.Fatalf("runCommandInDir failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(workDir, "marker.file")); err != nil {
		t.Fatalf("expected marker file in workDir, got error: %v", err)
	}

	cwdAfter, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd after run failed: %v", err)
	}
	if cwdAfter != cwd {
		t.Fatalf("process working directory changed: before=%s after=%s", cwd, cwdAfter)
	}
}
