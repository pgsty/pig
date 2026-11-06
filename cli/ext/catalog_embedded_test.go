package ext

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"pig/internal/config"
)

func loadEmbeddedCatalogForTest(t *testing.T) *ExtensionCatalog {
	t.Helper()

	ec := &ExtensionCatalog{}
	if err := ec.Load(nil); err != nil {
		t.Fatalf("failed to load embedded catalog: %v", err)
	}
	return ec
}

func TestEmbeddedCatalogCountsEveryRecord(t *testing.T) {
	ec := loadEmbeddedCatalogForTest(t)
	origCatalog, origShowPkg := Catalog, ShowPkg
	Catalog, ShowPkg = ec, false
	t.Cleanup(func() { Catalog, ShowPkg = origCatalog, origShowPkg })

	want := len(ec.Extensions)
	if ec.ExtensionCount() != want {
		t.Fatalf("ExtensionCount() = %d, want all %d catalog records", ec.ExtensionCount(), want)
	}
	result := ListExtensions("", 0)
	data, ok := result.Data.(*ExtensionListData)
	if !ok || data.Count != want || len(data.Extensions) != want {
		t.Fatalf("default list = %#v, want %d records", result.Data, want)
	}
	utilityFound := false
	for _, item := range data.Extensions {
		if item.Name == "pg_relation_sql" {
			utilityFound = true
		}
	}
	if !utilityFound {
		t.Fatal("default list must include pg_relation_sql")
	}
	if !strings.Contains(data.Text(), fmt.Sprintf("(%d Rows)", want)) {
		t.Fatal("text list must report the full catalog count")
	}
	availability := GetExtensionAvailability(nil).Data.(*ExtensionAvailData).Text()
	if !strings.Contains(availability, fmt.Sprintf("with %d extensions", want)) {
		t.Fatal("availability summary must report the full catalog count")
	}
}

func countAvailableEntriesByOS(ec *ExtensionCatalog) map[string]int {
	counts := make(map[string]int)
	if ec == nil {
		return counts
	}
	for _, ext := range ec.Extensions {
		if ext == nil {
			continue
		}
		for _, entry := range ext.GetPkgMatrix() {
			if entry != nil && entry.State == PkgAvail {
				counts[entry.OS]++
			}
		}
	}
	return counts
}

func TestEmbeddedCatalogRetainsSupportedAvailabilityCoverage(t *testing.T) {
	ec := loadEmbeddedCatalogForTest(t)
	counts := countAvailableEntriesByOS(ec)

	for _, osCode := range []string{"d12", "el9", "u22", "u24", "u26"} {
		if counts[osCode] == 0 {
			t.Fatalf("expected embedded catalog to retain non-zero available entries for %s, got counts=%v", osCode, counts)
		}
	}
}

func TestEmbeddedCatalogAvailabilityMatrixRendersU26Rows(t *testing.T) {
	ec := loadEmbeddedCatalogForTest(t)
	ext := ec.ExtNameMap["postgis"]
	if ext == nil {
		t.Fatal("expected embedded catalog to include postgis")
	}

	origCatalog := Catalog
	Catalog = ec
	defer func() { Catalog = origCatalog }()

	text := buildExtensionAvailData(ext).Text()
	if !strings.Contains(text, "u26.x86_64") {
		t.Fatalf("expected availability matrix to render u26.x86_64 row, got:\n%s", text)
	}
	if !strings.Contains(text, "u26.aarch64") {
		t.Fatalf("expected availability matrix to render u26.aarch64 row, got:\n%s", text)
	}
}

func TestEmbeddedCatalogPGAuditPackageResolution(t *testing.T) {
	ec := loadEmbeddedCatalogForTest(t)
	for _, osCode := range []string{"el8", "el9", "el10"} {
		for _, arch := range []string{"amd64", "arm64"} {
			t.Run(osCode+"/"+arch, func(t *testing.T) {
				t.Cleanup(withCategoryAliasTestEnv(t, config.DistroEL, osCode, arch, ec.Extensions))
				for _, pgVer := range []int{14, 15} {
					want := fmt.Sprintf("pgaudit_%d", pgVer)
					for _, name := range []string{"pgaudit", fmt.Sprintf("pg%d-sec", pgVer), "pgsql-sec"} {
						t.Run(fmt.Sprintf("pg%d/%s", pgVer, name), func(t *testing.T) {
							for _, res := range []*PackageResolveResult{
								ResolveExtensionPackages(pgVer, []string{name}, false),
								ResolveInstallPackages(pgVer, []string{name}, false),
							} {
								if len(res.NotFound) != 0 || len(res.NoPackage) != 0 {
									t.Fatalf("resolution failed: not_found=%v no_package=%v", res.NotFound, res.NoPackage)
								}
								if !slices.Contains(res.Packages, want) {
									t.Fatalf("packages = %v, want normalized package %s", res.Packages, want)
								}
								for _, legacy := range []string{"pgaudit16_14", "pgaudit17_15"} {
									if slices.Contains(res.Packages, legacy) {
										t.Fatalf("packages still contain legacy name %s", legacy)
									}
								}
								if name == "pgaudit" && len(res.Packages) != 1 {
									t.Fatalf("direct pgaudit request = %v, want only %s", res.Packages, want)
								}
							}
						})
					}
				}
			})
		}
	}
}

func TestEmbeddedCatalogPostGISPackageResolution(t *testing.T) {
	ec := loadEmbeddedCatalogForTest(t)
	platforms := []struct {
		osType, osCode, postgis, standardizer, geocoder string
	}{
		{config.DistroEL, "el8", "postgis37_%d", "address_standardizer_%d", "postgis_tiger_geocoder_%d"},
		{config.DistroEL, "el9", "postgis37_%d", "address_standardizer_%d", "postgis_tiger_geocoder_%d"},
		{config.DistroDEB, "u24", "postgresql-%d-postgis-3", "postgresql-%d-address-standardizer", "postgresql-%d-postgis-tiger-geocoder"},
	}
	for _, platform := range platforms {
		t.Run(platform.osCode, func(t *testing.T) {
			t.Cleanup(withCategoryAliasTestEnv(t, platform.osType, platform.osCode, "amd64", ec.Extensions))
			for _, pgVer := range []int{14, 15, 16, 17, 18} {
				t.Run(fmt.Sprintf("pg%d", pgVer), func(t *testing.T) {
					names := []string{"postgis", "postgis_topology", "postgis_raster", "postgis_sfcgal", "address_standardizer", "address_standardizer_data_us"}
					want := []string{fmt.Sprintf(platform.postgis, pgVer), fmt.Sprintf(platform.standardizer, pgVer)}
					geocoder := fmt.Sprintf(platform.geocoder, pgVer)
					if pgVer >= 16 {
						names = append(names, "postgis_tiger_geocoder")
						want = append(want, geocoder)
					}
					for _, res := range []*PackageResolveResult{
						ResolveExtensionPackages(pgVer, names, false),
						ResolveInstallPackages(pgVer, names, false),
					} {
						if len(res.NotFound) != 0 || len(res.NoPackage) != 0 || !slices.Equal(res.Packages, want) {
							t.Fatalf("direct packages = %v (not_found=%v no_package=%v), want %v", res.Packages, res.NotFound, res.NoPackage, want)
						}
					}
					for _, alias := range []string{fmt.Sprintf("pg%d-gis", pgVer), "pgsql-gis"} {
						res := ResolveExtensionPackages(pgVer, []string{alias}, false)
						if len(res.NotFound) != 0 || len(res.NoPackage) != 0 {
							t.Fatalf("%s failed: not_found=%v no_package=%v", alias, res.NotFound, res.NoPackage)
						}
						for _, pkg := range want {
							if !slices.Contains(res.Packages, pkg) {
								t.Fatalf("%s packages = %v, missing %s", alias, res.Packages, pkg)
							}
						}
						if pgVer < 16 && slices.Contains(res.Packages, geocoder) {
							t.Fatalf("%s includes unsupported geocoder package %s", alias, geocoder)
						}
					}
				})
			}
		})
	}
}
