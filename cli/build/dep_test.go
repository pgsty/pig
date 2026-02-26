package build

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseDebBuildDependsMultiline(t *testing.T) {
	control := `Source: pgedge-17
Section: database
Build-Depends:
 autoconf,
 bison,
 debhelper-compat (= 13),
 libselinux1-dev [linux-any],
 libfoo-dev | libbar-dev,
 perl (>= 5.8),
 postgresql-all (>= 217~),
 pkgconf,
 zlib1g-dev
Standards-Version: 4.6.2
`

	got := parseDebBuildDepends(control, "")
	want := []string{
		"autoconf",
		"bison",
		"libselinux1-dev",
		"libfoo-dev",
		"perl",
		"pkgconf",
		"zlib1g-dev",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseDebBuildDepends(multiline) = %v, want %v", got, want)
	}
}

func TestParseDebBuildDependsSingleLineWithPGVersion(t *testing.T) {
	control := `Source: pg_duckdb
Build-Depends: debhelper-compat (= 13), postgresql-server-dev-PGVERSION, libcurl4-openssl-dev (>= 7), foo:any, ${misc:Depends}
Standards-Version: 4.6.2
`

	got := parseDebBuildDepends(control, "17")
	want := []string{
		"postgresql-server-dev-17",
		"libcurl4-openssl-dev",
		"foo:any",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseDebBuildDepends(single-line) = %v, want %v", got, want)
	}
}

func TestParseDebBuildDependsWithArchAndIndep(t *testing.T) {
	control := `Source: test
Build-Depends: debhelper-compat (= 13), postgresql-all (>= 217~)
Build-Depends-Arch:
 libxml2-dev,
 libfoo-dev | libbar-dev
Build-Depends-Indep: pandoc (>= 2.0)
Standards-Version: 4.6.2
`

	got := parseDebBuildDepends(control, "")
	want := []string{
		"libxml2-dev",
		"libfoo-dev",
		"pandoc",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseDebBuildDepends(arch+indep) = %v, want %v", got, want)
	}
}

func TestExpandDebPGVersionDeps(t *testing.T) {
	deps := []string{
		"postgresql-server-dev-PGVERSION",
		"libcurl4-openssl-dev",
		"postgresql-server-dev-PGVERSION",
	}

	got := expandDebPGVersionDeps(deps, []string{"17", "16"})
	want := []string{
		"postgresql-server-dev-17",
		"postgresql-server-dev-16",
		"libcurl4-openssl-dev",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expandDebPGVersionDeps = %v, want %v", got, want)
	}
}

func TestInferRPMPGMajorFromSpec(t *testing.T) {
	spec := `%global sname pgedge
%global pgmajorversion 17
Name: %{sname}_%{pgmajorversion}
`

	dir := t.TempDir()
	specFile := filepath.Join(dir, "test.spec")
	if err := os.WriteFile(specFile, []byte(spec), 0644); err != nil {
		t.Fatalf("write spec failed: %v", err)
	}

	got := inferRPMPGMajorFromSpec(specFile)
	if got != "17" {
		t.Fatalf("inferRPMPGMajorFromSpec = %q, want %q", got, "17")
	}
}

func TestIntersectStringSets(t *testing.T) {
	got := intersectStringSets(
		[]string{"18", "17", "17", "16"},
		[]string{"17", "15"},
	)
	want := []string{"17"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("intersectStringSets = %v, want %v", got, want)
	}
}
