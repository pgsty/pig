package build

import (
	"os"
	"path/filepath"
	"pig/internal/config"
	"reflect"
	"testing"
)

func TestParseDebBuildDependsMultiline(t *testing.T) {
	control := `Source: pgedge-18
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

func TestNormalizeDebDependencyEntrySelectsAvailableAlternative(t *testing.T) {
	original := debDependencyAvailable
	debDependencyAvailable = func(pkg string) bool {
		return pkg == "libselinux-dev"
	}
	defer func() {
		debDependencyAvailable = original
	}()

	got := normalizeDebDependencyEntry("libselinux1-dev [linux-any] | libselinux-dev [linux-any]", "")
	want := "libselinux-dev"
	if got != want {
		t.Fatalf("normalizeDebDependencyEntry(alternative) = %q, want %q", got, want)
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
%global pgmajorversion 18
Name: %{sname}_%{pgmajorversion}
`

	dir := t.TempDir()
	specFile := filepath.Join(dir, "test.spec")
	if err := os.WriteFile(specFile, []byte(spec), 0644); err != nil {
		t.Fatalf("write spec failed: %v", err)
	}

	got := inferRPMPGMajorFromSpec(specFile)
	if got != "18" {
		t.Fatalf("inferRPMPGMajorFromSpec = %q, want %q", got, "18")
	}
}

func TestResolveRPMBuildSpecAndPG(t *testing.T) {
	tests := []struct {
		pkg      string
		wantSpec string
		wantPG   string
	}{
		{pkg: "openhalo", wantSpec: "openhalodb", wantPG: "14"},
		{pkg: "ivorysql", wantSpec: "ivorysql", wantPG: "18"},
		{pkg: "babelfish-18", wantSpec: "babelfish", wantPG: "18"},
		{pkg: "pgedge-17", wantSpec: "pgedge", wantPG: "17"},
		{pkg: "orioledb-16", wantSpec: "orioledb", wantPG: "16"},
		{pkg: "polarstore", wantSpec: "polarstore", wantPG: ""},
		{pkg: "pg_duckdb", wantSpec: "pg_duckdb", wantPG: "18"},
	}

	for _, tt := range tests {
		t.Run(tt.pkg, func(t *testing.T) {
			gotSpec, gotPG, _ := resolveRPMBuildSpecAndPG(tt.pkg, "")
			if gotSpec != tt.wantSpec || gotPG != tt.wantPG {
				t.Fatalf("resolveRPMBuildSpecAndPG(%q) = (%q, %q), want (%q, %q)", tt.pkg, gotSpec, gotPG, tt.wantSpec, tt.wantPG)
			}
		})
	}
}

func TestResolveDebBuildRecipe(t *testing.T) {
	tests := []struct {
		pkg        string
		wantRecipe string
	}{
		{pkg: "openhalo", wantRecipe: "openhalodb"},
		{pkg: "openhalodb", wantRecipe: "openhalodb"},
		{pkg: "ivorysql", wantRecipe: "ivorysql"},
		{pkg: "babelfish", wantRecipe: "babelfish"},
		{pkg: "babelfish-17", wantRecipe: "babelfish"},
		{pkg: "babelfish-18", wantRecipe: "babelfish"},
		{pkg: "polarstore", wantRecipe: "polarstore"},
		{pkg: "pg_duckdb", wantRecipe: "pg_duckdb"},
	}

	for _, tt := range tests {
		t.Run(tt.pkg, func(t *testing.T) {
			got := resolveDebBuildRecipe(tt.pkg)
			if got != tt.wantRecipe {
				t.Fatalf("resolveDebBuildRecipe(%q) = %q, want %q", tt.pkg, got, tt.wantRecipe)
			}
		})
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

func TestInstallDepsListReturnsNilOnDependencyFailures(t *testing.T) {
	oldOSType := config.OSType
	t.Cleanup(func() {
		config.OSType = oldOSType
	})

	// Force InstallDeps to fail fast without invoking package managers.
	config.OSType = "unsupported-test-os"

	if err := InstallDepsList([]string{"nonexistent-pkg-for-warning-path"}, ""); err != nil {
		t.Fatalf("InstallDepsList() should keep warning-only behavior, got error: %v", err)
	}
}
