package ext

import (
	"testing"

	"pig/internal/config"
)

func withResolveTestEnv(t *testing.T, osType, osVersion string, catalog *ExtensionCatalog) func() {
	t.Helper()

	oldOSType := config.OSType
	oldOSVersion := config.OSVersion
	oldCatalog := Catalog

	config.OSType = osType
	config.OSVersion = osVersion
	Catalog = catalog

	return func() {
		config.OSType = oldOSType
		config.OSVersion = oldOSVersion
		Catalog = oldCatalog
	}
}

func newTestCatalog(ext *Extension) *ExtensionCatalog {
	return &ExtensionCatalog{
		Extensions: []*Extension{ext},
		ExtNameMap: map[string]*Extension{
			ext.Name: ext,
		},
		ExtPkgMap: map[string]*Extension{
			ext.Pkg: ext,
		},
		Dependency: map[string][]string{},
		AliasMap:   map[string]string{},
	}
}

func TestSplitNameVersion(t *testing.T) {
	name, version := splitNameVersion("postgis=3.5.0", true)
	if name != "postgis" || version != "3.5.0" {
		t.Fatalf("unexpected split for single '=': %q %q", name, version)
	}

	name, version = splitNameVersion("postgis=3.5.0=extra", true)
	if name != "postgis=3.5.0=extra" || version != "" {
		t.Fatalf("unexpected split for multiple '=': %q %q", name, version)
	}

	name, version = splitNameVersion("postgis=3.5.0", false)
	if name != "postgis=3.5.0" || version != "" {
		t.Fatalf("unexpected split when parse disabled: %q %q", name, version)
	}
}

func TestResolveExtensionPackagesWithVersionSpec(t *testing.T) {
	ext := &Extension{
		Name:   "test_ext",
		Pkg:    "test_pkg",
		Lead:   true,
		RpmPkg: "test_ext_$v",
	}
	cleanup := withResolveTestEnv(t, config.DistroEL, "9", newTestCatalog(ext))
	defer cleanup()

	res := ResolveExtensionPackages(17, []string{"test_ext=1.2.3"}, true)
	if len(res.NotFound) != 0 || len(res.NoPackage) != 0 {
		t.Fatalf("unexpected resolution errors: not_found=%v no_package=%v", res.NotFound, res.NoPackage)
	}
	if len(res.Packages) != 1 {
		t.Fatalf("expected 1 package, got %d (%v)", len(res.Packages), res.Packages)
	}
	if res.Packages[0] != "test_ext_17-1.2.3" {
		t.Fatalf("unexpected package: %s", res.Packages[0])
	}
	if res.PackageOwner[res.Packages[0]] != "test_ext" {
		t.Fatalf("unexpected owner for %s: %s", res.Packages[0], res.PackageOwner[res.Packages[0]])
	}
}

func TestResolveExtensionPackagesAlias(t *testing.T) {
	ext := &Extension{
		Name:   "dummy",
		Pkg:    "dummy",
		Lead:   true,
		RpmPkg: "dummy_$v",
	}
	cleanup := withResolveTestEnv(t, config.DistroEL, "9", newTestCatalog(ext))
	defer cleanup()

	res := ResolveExtensionPackages(17, []string{"pg17"}, false)
	if len(res.NotFound) != 0 || len(res.NoPackage) != 0 {
		t.Fatalf("unexpected resolution errors: not_found=%v no_package=%v", res.NotFound, res.NoPackage)
	}
	if len(res.Packages) == 0 {
		t.Fatal("expected alias pg17 to resolve to packages")
	}
	for _, pkg := range res.Packages {
		if res.PackageOwner[pkg] != "pg17" {
			t.Fatalf("unexpected owner for %s: %s", pkg, res.PackageOwner[pkg])
		}
	}
}

func TestResolveInstallPackagesTranslateAndVersion(t *testing.T) {
	ext := &Extension{
		Name:   "test_ext",
		Pkg:    "test_pkg",
		Lead:   true,
		DebPkg: "postgresql-$v-test-ext",
	}
	cleanup := withResolveTestEnv(t, config.DistroDEB, "12", newTestCatalog(ext))
	defer cleanup()

	res := ResolveInstallPackages(17, []string{"test_pkg=2.0.0"}, false)
	if len(res.Packages) != 1 {
		t.Fatalf("expected 1 package, got %d (%v)", len(res.Packages), res.Packages)
	}
	if res.Packages[0] != "postgresql-17-test-ext=2.0.0*" {
		t.Fatalf("unexpected package: %s", res.Packages[0])
	}
	if res.PackageOwner[res.Packages[0]] != "test_ext" {
		t.Fatalf("unexpected owner: %s", res.PackageOwner[res.Packages[0]])
	}
}

func TestResolveInstallPackagesNoTranslation(t *testing.T) {
	cleanup := withResolveTestEnv(t, config.DistroDEB, "12", newTestCatalog(&Extension{
		Name:   "test_ext",
		Pkg:    "test_pkg",
		Lead:   true,
		DebPkg: "postgresql-$v-test-ext",
	}))
	defer cleanup()

	res := ResolveInstallPackages(17, []string{"nginx=1.2.3"}, true)
	if len(res.Packages) != 1 {
		t.Fatalf("expected 1 package, got %d (%v)", len(res.Packages), res.Packages)
	}
	if res.Packages[0] != "nginx=1.2.3*" {
		t.Fatalf("unexpected package: %s", res.Packages[0])
	}
	if res.PackageOwner[res.Packages[0]] != "nginx" {
		t.Fatalf("unexpected owner: %s", res.PackageOwner[res.Packages[0]])
	}
}
