package ext

import (
	"fmt"
	"reflect"
	"slices"
	"testing"

	"pig/internal/config"
)

func withCategoryAliasTestEnv(t *testing.T, osType, osCode, osArch string, exts []*Extension) func() {
	t.Helper()

	oldCatalog := Catalog
	oldOSType := config.OSType
	oldOSCode := config.OSCode
	oldOSArch := config.OSArch

	clearCategoryAliasCache()

	catalog := &ExtensionCatalog{
		Extensions: exts,
		ExtNameMap: map[string]*Extension{},
		ExtPkgMap:  map[string]*Extension{},
		Dependency: map[string][]string{},
		AliasMap:   map[string]string{},
	}
	for _, ext := range exts {
		catalog.ExtNameMap[ext.Name] = ext
		if ext.Pkg != "" && ext.Lead {
			catalog.ExtPkgMap[ext.Pkg] = ext
		}
	}

	Catalog = catalog
	config.OSType = osType
	config.OSCode = osCode
	config.OSArch = osArch

	return func() {
		clearCategoryAliasCache()
		Catalog = oldCatalog
		config.OSType = oldOSType
		config.OSCode = oldOSCode
		config.OSArch = oldOSArch
	}
}

func clearCategoryAliasCache() {
	categoryAliasCache.Range(func(key, value interface{}) bool {
		categoryAliasCache.Delete(key)
		return true
	})
}

func newTestCategoryExt(id int, name, pkg, category string, matrix []string) *Extension {
	matrixItems := make([]interface{}, 0, len(matrix))
	for _, m := range matrix {
		matrixItems = append(matrixItems, m)
	}
	return &Extension{
		ID:       id,
		Name:     name,
		Pkg:      pkg,
		Category: category,
		Lead:     true,
		Contrib:  false,
		Extra: map[string]interface{}{
			"matrix": matrixItems,
		},
	}
}

func TestResolveCategoryAliasVisibleOnly(t *testing.T) {
	extPGDG := newTestCategoryExt(100, "pg_cron", "pg_cron", "TIME", []string{"el9i:18:A:f:1:G:1.0"})
	extPGDG.RpmPkg, extPGDG.RpmRepo, extPGDG.RpmPg = "pg_cron_$v", "PGDG", []string{"18"}

	extPigsty := newTestCategoryExt(110, "pg_task", "pg_task", "TIME", []string{"el9i:18:A:f:1:P:1.0"})
	extPigsty.RpmPkg, extPigsty.RpmRepo, extPigsty.RpmPg = "pg_task_$v", "PIGSTY", []string{"18"}

	extHidden := newTestCategoryExt(120, "pg_hidden", "pg_hidden", "TIME", []string{"el9i:18:A:t:1:G:1.0"})
	extHidden.RpmPkg, extHidden.RpmRepo, extHidden.RpmPg = "pg_hidden_$v", "PGDG", []string{"18"}

	extBreak := newTestCategoryExt(130, "pg_break", "pg_break", "TIME", []string{"el9i:18:B:f:1:G:1.0"})
	extBreak.RpmPkg, extBreak.RpmRepo, extBreak.RpmPg = "pg_break_$v", "PGDG", []string{"18"}

	cleanup := withCategoryAliasTestEnv(t, config.DistroEL, "el9", "amd64", []*Extension{
		extPGDG, extPigsty, extHidden, extBreak,
	})
	defer cleanup()

	res := ResolveExtensionPackages(18, []string{"pg18-time"}, false)
	if len(res.NotFound) > 0 || len(res.NoPackage) > 0 {
		t.Fatalf("unexpected resolution errors: not_found=%v no_package=%v", res.NotFound, res.NoPackage)
	}
	want := []string{"pg_cron_18", "pg_task_18"}
	if !reflect.DeepEqual(res.Packages, want) {
		t.Fatalf("resolved packages mismatch\nwant: %v\ngot:  %v", want, res.Packages)
	}
}

func TestResolvePgsqlCategoryAliasUsesTemplateFromLatestVersion(t *testing.T) {
	extAll := newTestCategoryExt(100, "pg_cron", "pg_cron", "TIME", []string{
		"el9i:18:A:f:1:G:1.0",
		"el9i:17:A:f:1:G:1.0",
	})
	extAll.RpmPkg, extAll.RpmRepo, extAll.RpmPg = "pg_cron_$v", "PGDG", []string{"18", "17"}

	extOnly18 := newTestCategoryExt(110, "pg_topn", "pg_topn", "TIME", []string{"el9i:18:A:f:1:G:1.0"})
	extOnly18.RpmPkg, extOnly18.RpmRepo, extOnly18.RpmPg = "pg_topn_$v", "PGDG", []string{"18"}

	cleanup := withCategoryAliasTestEnv(t, config.DistroEL, "el9", "amd64", []*Extension{extAll, extOnly18})
	defer cleanup()

	res := ResolveExtensionPackages(17, []string{"pgsql-time"}, false)
	if len(res.NotFound) > 0 || len(res.NoPackage) > 0 {
		t.Fatalf("unexpected resolution errors: not_found=%v no_package=%v", res.NotFound, res.NoPackage)
	}
	want := []string{"pg_cron_17", "pg_topn_17"}
	if !reflect.DeepEqual(res.Packages, want) {
		t.Fatalf("resolved packages mismatch\nwant: %v\ngot:  %v", want, res.Packages)
	}
}

func TestResolveCategoryAliasPGAuditELRename(t *testing.T) {
	ext := newTestCategoryExt(100, "pgaudit", "pgaudit", "SEC", []string{
		"el9i:15:A:f:1:G:1.0",
		"el9i:14:A:f:1:G:1.0",
		"el9i:13:A:f:1:G:1.0",
	})
	ext.RpmPkg, ext.RpmRepo, ext.RpmPg = "pgaudit_$v", "PGDG", []string{"15", "14", "13"}

	cleanup := withCategoryAliasTestEnv(t, config.DistroEL, "el9", "amd64", []*Extension{ext})
	defer cleanup()

	cases := []struct {
		pgVer    int
		alias    string
		expected string
	}{
		{15, "pg15-sec", "pgaudit17_15"},
		{14, "pg14-sec", "pgaudit16_14"},
		{13, "pg13-sec", "pgaudit15_13"},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("pg%d", tc.pgVer), func(t *testing.T) {
			res := ResolveExtensionPackages(tc.pgVer, []string{tc.alias}, false)
			if len(res.NotFound) > 0 || len(res.NoPackage) > 0 {
				t.Fatalf("unexpected resolution errors: not_found=%v no_package=%v", res.NotFound, res.NoPackage)
			}
			if !reflect.DeepEqual(res.Packages, []string{tc.expected}) {
				t.Fatalf("expected %q, got %v", tc.expected, res.Packages)
			}
		})
	}
}

func TestResolveCategoryAliasFallbackRPM(t *testing.T) {
	extMatrix := newTestCategoryExt(100, "matrix_ext", "matrix_ext", "FUNC", []string{"el10i:18:A:f:1:G:1.0"})
	extMatrix.RpmPkg, extMatrix.RpmRepo, extMatrix.RpmPg = "matrix_ext_$v", "PGDG", []string{"18"}

	extMatrixPigsty := newTestCategoryExt(105, "matrix_pigsty_ext", "matrix_pigsty_ext", "FUNC", []string{"el10i:18:A:f:1:P:1.0"})
	extMatrixPigsty.RpmPkg, extMatrixPigsty.RpmRepo, extMatrixPigsty.RpmPg = "matrix_pigsty_ext_$v", "PIGSTY", []string{"18"}

	extRepoPGDG := newTestCategoryExt(110, "repo_ext", "repo_ext", "FUNC", nil)
	extRepoPGDG.RpmPkg, extRepoPGDG.RpmRepo, extRepoPGDG.RpmPg = "repo_ext_$v", "PGDG", []string{"18"}

	extRepoPigsty := newTestCategoryExt(120, "pigsty_ext", "pigsty_ext", "FUNC", nil)
	extRepoPigsty.RpmPkg, extRepoPigsty.RpmRepo, extRepoPigsty.RpmPg = "pigsty_ext_$v", "PIGSTY", []string{"18"}

	cleanup := withCategoryAliasTestEnv(t, config.DistroEL, "el11", "amd64", []*Extension{
		extMatrix, extMatrixPigsty, extRepoPGDG, extRepoPigsty,
	})
	defer cleanup()

	res := ResolveExtensionPackages(18, []string{"pg18-func"}, false)
	if len(res.NotFound) > 0 || len(res.NoPackage) > 0 {
		t.Fatalf("unexpected resolution errors: not_found=%v no_package=%v", res.NotFound, res.NoPackage)
	}

	want := []string{"matrix_ext_18", "repo_ext_18"}
	if !reflect.DeepEqual(res.Packages, want) {
		t.Fatalf("resolved packages mismatch\nwant: %v\ngot:  %v", want, res.Packages)
	}
	if slices.Contains(res.Packages, "matrix_pigsty_ext_18") {
		t.Fatalf("matrix pigsty package should be filtered out in fallback mode: %v", res.Packages)
	}
}

func TestResolveCategoryAliasNoFallbackOnKnownOS(t *testing.T) {
	extRepoPGDG := newTestCategoryExt(100, "repo_ext", "repo_ext", "FUNC", nil)
	extRepoPGDG.RpmPkg, extRepoPGDG.RpmRepo, extRepoPGDG.RpmPg = "repo_ext_$v", "PGDG", []string{"18"}

	cleanup := withCategoryAliasTestEnv(t, config.DistroEL, "el9", "amd64", []*Extension{extRepoPGDG})
	defer cleanup()

	res := ResolveExtensionPackages(18, []string{"pg18-func"}, false)
	if len(res.NotFound) != 0 {
		t.Fatalf("expected no not_found entries, got %v", res.NotFound)
	}
	if !reflect.DeepEqual(res.NoPackage, []string{"pg18-func"}) {
		t.Fatalf("expected no_package=[pg18-func], got %v", res.NoPackage)
	}
	if len(res.Packages) != 0 {
		t.Fatalf("expected no packages, got %v", res.Packages)
	}
}

func TestResolveCategoryAliasFallbackDEB(t *testing.T) {
	extMatrix := newTestCategoryExt(100, "matrix_ext", "matrix_ext", "LANG", []string{"d13i:18:A:f:1:G:1.0"})
	extMatrix.DebPkg, extMatrix.DebRepo, extMatrix.DebPg = "postgresql-$v-matrix-ext", "PGDG", []string{"18"}

	extMatrixPigsty := newTestCategoryExt(105, "matrix_pigsty_ext", "matrix_pigsty_ext", "LANG", []string{"d13i:18:A:f:1:P:1.0"})
	extMatrixPigsty.DebPkg, extMatrixPigsty.DebRepo, extMatrixPigsty.DebPg = "postgresql-$v-matrix-pigsty-ext", "PIGSTY", []string{"18"}

	extRepoPGDG := newTestCategoryExt(110, "repo_ext", "repo_ext", "LANG", nil)
	extRepoPGDG.DebPkg, extRepoPGDG.DebRepo, extRepoPGDG.DebPg = "postgresql-$v-repo-ext", "PGDG", []string{"18"}

	extRepoPigsty := newTestCategoryExt(120, "pigsty_ext", "pigsty_ext", "LANG", nil)
	extRepoPigsty.DebPkg, extRepoPigsty.DebRepo, extRepoPigsty.DebPg = "postgresql-$v-pigsty-ext", "PIGSTY", []string{"18"}

	cleanup := withCategoryAliasTestEnv(t, config.DistroDEB, "u26", "amd64", []*Extension{
		extMatrix, extMatrixPigsty, extRepoPGDG, extRepoPigsty,
	})
	defer cleanup()

	res := ResolveExtensionPackages(18, []string{"pg18-lang"}, false)
	if len(res.NotFound) > 0 || len(res.NoPackage) > 0 {
		t.Fatalf("unexpected resolution errors: not_found=%v no_package=%v", res.NotFound, res.NoPackage)
	}

	if !slices.Contains(res.Packages, "postgresql-18-matrix-ext") {
		t.Fatalf("expected matrix package in %v", res.Packages)
	}
	if !slices.Contains(res.Packages, "postgresql-18-repo-ext") {
		t.Fatalf("expected repo fallback package in %v", res.Packages)
	}
	if slices.Contains(res.Packages, "postgresql-18-matrix-pigsty-ext") {
		t.Fatalf("matrix pigsty package should be filtered out in fallback mode: %v", res.Packages)
	}
	if slices.Contains(res.Packages, "postgresql-18-pigsty-ext") {
		t.Fatalf("pigsty package should be filtered out: %v", res.Packages)
	}
}

func TestResolveCategoryAliasNoPackage(t *testing.T) {
	extHidden := newTestCategoryExt(100, "hidden_etl", "hidden_etl", "ETL", []string{"el9i:18:A:t:1:G:1.0"})
	extHidden.RpmPkg, extHidden.RpmRepo, extHidden.RpmPg = "hidden_etl_$v", "PGDG", []string{"18"}

	cleanup := withCategoryAliasTestEnv(t, config.DistroEL, "el9", "amd64", []*Extension{extHidden})
	defer cleanup()

	res := ResolveExtensionPackages(18, []string{"pg18-etl"}, false)
	if len(res.NotFound) != 0 {
		t.Fatalf("expected no not_found entries, got %v", res.NotFound)
	}
	if !reflect.DeepEqual(res.NoPackage, []string{"pg18-etl"}) {
		t.Fatalf("expected no_package=[pg18-etl], got %v", res.NoPackage)
	}
}
