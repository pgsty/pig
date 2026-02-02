/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>
*/
package ext

import (
	"encoding/json"
	"pig/internal/config"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

/********************
 * DTO Structure Tests
 ********************/

func TestExtensionSummaryJSONSerialization(t *testing.T) {
	summary := &ExtensionSummary{
		Name:        "postgis",
		Pkg:         "postgis",
		Version:     "3.5.0",
		Category:    "GIS",
		License:     "GPL-2.0",
		Repo:        "PGDG",
		Status:      "available",
		PackageName: "postgis36_17",
		Description: "PostGIS geometry extension",
		PgVer:       []string{"17", "16", "15"},
	}

	// Test JSON marshaling
	data, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("failed to marshal ExtensionSummary: %v", err)
	}

	// Verify JSON contains expected fields
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if result["name"] != "postgis" {
		t.Errorf("expected name=postgis, got %v", result["name"])
	}
	if result["status"] != "available" {
		t.Errorf("expected status=available, got %v", result["status"])
	}
}

func TestExtensionSummaryYAMLSerialization(t *testing.T) {
	summary := &ExtensionSummary{
		Name:        "pg_vector",
		Pkg:         "pgvector",
		Version:     "0.6.0",
		Category:    "RAG",
		License:     "PostgreSQL",
		Repo:        "PGDG",
		Status:      "installed",
		PackageName: "pgvector_17",
		Description: "Vector data type and similarity search",
		PgVer:       []string{"17", "16", "15"},
	}

	// Test YAML marshaling
	data, err := yaml.Marshal(summary)
	if err != nil {
		t.Fatalf("failed to marshal ExtensionSummary to YAML: %v", err)
	}

	// Verify YAML contains expected fields
	var result map[string]interface{}
	if err := yaml.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to unmarshal YAML: %v", err)
	}

	if result["name"] != "pg_vector" {
		t.Errorf("expected name=pg_vector, got %v", result["name"])
	}
	if result["status"] != "installed" {
		t.Errorf("expected status=installed, got %v", result["status"])
	}
}

func TestExtensionInfoDataSerialization(t *testing.T) {
	info := &ExtensionInfoData{
		Name:        "timescaledb",
		Pkg:         "timescaledb",
		Category:    "TIME",
		License:     "Apache-2.0",
		Language:    "C",
		Version:     "2.15.0",
		URL:         "https://github.com/timescale/timescaledb",
		Description: "Scalable inserts and complex queries for time-series data",
		Properties: &ExtensionProperties{
			HasBin:      false,
			HasLib:      true,
			NeedLoad:    true,
			NeedDDL:     true,
			Relocatable: "f",
			Trusted:     "f",
		},
		Requires:   []string{},
		RequiredBy: []string{"promscale"},
		PgVer:      []string{"17", "16", "15", "14"},
		RpmPackage: &PackageInfo{
			Package:    "timescaledb-2-postgresql-$v-tsl",
			Repository: "TIMESCALE",
			Version:    "2.15.0",
			PgVer:      []string{"17", "16", "15", "14"},
		},
		Operations: &ExtensionOperations{
			Install: "pig ext add timescaledb",
			Config:  "shared_preload_libraries = 'timescaledb'",
			Create:  "CREATE EXTENSION timescaledb;",
			Build:   "pig build pkg timescaledb",
		},
	}

	// Test JSON round-trip
	jsonData, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("failed to marshal ExtensionInfoData: %v", err)
	}

	var jsonResult ExtensionInfoData
	if err := json.Unmarshal(jsonData, &jsonResult); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if jsonResult.Name != "timescaledb" {
		t.Errorf("expected name=timescaledb, got %v", jsonResult.Name)
	}
	if jsonResult.Properties == nil {
		t.Fatal("expected Properties to be non-nil")
	}
	if !jsonResult.Properties.NeedLoad {
		t.Error("expected NeedLoad=true")
	}

	// Test YAML round-trip
	yamlData, err := yaml.Marshal(info)
	if err != nil {
		t.Fatalf("failed to marshal ExtensionInfoData to YAML: %v", err)
	}

	var yamlResult ExtensionInfoData
	if err := yaml.Unmarshal(yamlData, &yamlResult); err != nil {
		t.Fatalf("failed to unmarshal YAML: %v", err)
	}

	if yamlResult.Operations == nil {
		t.Fatal("expected Operations to be non-nil")
	}
	if yamlResult.Operations.Config != "shared_preload_libraries = 'timescaledb'" {
		t.Errorf("unexpected config: %v", yamlResult.Operations.Config)
	}
}

func TestExtensionStatusDataSerialization(t *testing.T) {
	data := &ExtensionStatusData{
		PgInfo: &PostgresInfo{
			Version:      "PostgreSQL 17.0",
			MajorVersion: 17,
			BinDir:       "/usr/pgsql-17/bin",
			ExtensionDir: "/usr/pgsql-17/share/extension",
		},
		Summary: &ExtensionSummaryInfo{
			TotalInstalled: 50,
			ByRepo: map[string]int{
				"CONTRIB": 30,
				"PGDG":    15,
				"PIGSTY":  5,
			},
		},
		Extensions: []*ExtensionSummary{
			{Name: "plpgsql", Version: "1.0", Category: "LANG", Status: "installed"},
			{Name: "postgis", Version: "3.5.0", Category: "GIS", Status: "installed"},
		},
		NotFound: []string{"custom_ext"},
	}

	// Test JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("failed to marshal ExtensionStatusData: %v", err)
	}

	var jsonResult map[string]interface{}
	if err := json.Unmarshal(jsonData, &jsonResult); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	pgInfo := jsonResult["pg_info"].(map[string]interface{})
	if int(pgInfo["major_version"].(float64)) != 17 {
		t.Errorf("expected major_version=17, got %v", pgInfo["major_version"])
	}

	// Test YAML
	yamlData, err := yaml.Marshal(data)
	if err != nil {
		t.Fatalf("failed to marshal ExtensionStatusData to YAML: %v", err)
	}

	var yamlResult ExtensionStatusData
	if err := yaml.Unmarshal(yamlData, &yamlResult); err != nil {
		t.Fatalf("failed to unmarshal YAML: %v", err)
	}

	if yamlResult.Summary.TotalInstalled != 50 {
		t.Errorf("expected TotalInstalled=50, got %v", yamlResult.Summary.TotalInstalled)
	}
}

func TestExtensionAvailDataSerialization(t *testing.T) {
	// Test single extension mode
	singleData := &ExtensionAvailData{
		Extension: "postgis",
		Matrix: []*MatrixEntry{
			{OS: "el9", Arch: "amd64", PG: 17, State: "A", Version: "3.5.0", Org: "G"},
			{OS: "el9", Arch: "arm64", PG: 17, State: "A", Version: "3.5.0", Org: "G"},
			{OS: "u24", Arch: "amd64", PG: 17, State: "A", Version: "3.5.0", Org: "G"},
		},
		Summary:   "3/3 avail",
		LatestVer: "3.5.0",
	}

	jsonData, err := json.Marshal(singleData)
	if err != nil {
		t.Fatalf("failed to marshal ExtensionAvailData: %v", err)
	}

	var jsonResult map[string]interface{}
	if err := json.Unmarshal(jsonData, &jsonResult); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if jsonResult["extension"] != "postgis" {
		t.Errorf("expected extension=postgis, got %v", jsonResult["extension"])
	}
	if jsonResult["summary"] != "3/3 avail" {
		t.Errorf("expected summary='3/3 avail', got %v", jsonResult["summary"])
	}

	// Test global mode
	globalData := &ExtensionAvailData{
		OSCode:       "el9",
		Arch:         "amd64",
		PackageCount: 100,
		Packages: []*PackageAvailability{
			{Pkg: "postgis", Versions: map[string]string{"17": "3.5.0", "16": "3.5.0"}},
			{Pkg: "pgvector", Versions: map[string]string{"17": "0.6.0", "16": "0.6.0"}},
		},
	}

	yamlData, err := yaml.Marshal(globalData)
	if err != nil {
		t.Fatalf("failed to marshal global ExtensionAvailData: %v", err)
	}

	var yamlResult ExtensionAvailData
	if err := yaml.Unmarshal(yamlData, &yamlResult); err != nil {
		t.Fatalf("failed to unmarshal YAML: %v", err)
	}

	if yamlResult.PackageCount != 100 {
		t.Errorf("expected PackageCount=100, got %v", yamlResult.PackageCount)
	}
	if len(yamlResult.Packages) != 2 {
		t.Errorf("expected 2 packages, got %v", len(yamlResult.Packages))
	}
}

func TestExtensionListDataSerialization(t *testing.T) {
	data := &ExtensionListData{
		Query:     "gis",
		PgVersion: 17,
		OSCode:    "el9",
		Arch:      "amd64",
		Count:     3,
		Extensions: []*ExtensionSummary{
			{Name: "postgis", Category: "GIS", Status: "available"},
			{Name: "h3", Category: "GIS", Status: "available"},
			{Name: "pgrouting", Category: "GIS", Status: "available"},
		},
	}

	// JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("failed to marshal ExtensionListData: %v", err)
	}

	var jsonResult map[string]interface{}
	if err := json.Unmarshal(jsonData, &jsonResult); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if jsonResult["query"] != "gis" {
		t.Errorf("expected query=gis, got %v", jsonResult["query"])
	}
	if int(jsonResult["count"].(float64)) != 3 {
		t.Errorf("expected count=3, got %v", jsonResult["count"])
	}

	// YAML
	yamlData, err := yaml.Marshal(data)
	if err != nil {
		t.Fatalf("failed to marshal ExtensionListData to YAML: %v", err)
	}

	var yamlResult ExtensionListData
	if err := yaml.Unmarshal(yamlData, &yamlResult); err != nil {
		t.Fatalf("failed to unmarshal YAML: %v", err)
	}

	if yamlResult.Count != 3 {
		t.Errorf("expected Count=3, got %v", yamlResult.Count)
	}
	if len(yamlResult.Extensions) != 3 {
		t.Errorf("expected 3 extensions, got %v", len(yamlResult.Extensions))
	}
}

/********************
 * Conversion Method Tests
 ********************/

func TestExtensionToSummaryNil(t *testing.T) {
	var e *Extension
	summary := e.ToSummary(17)
	if summary != nil {
		t.Error("expected nil for nil Extension")
	}
}

func TestExtensionToSummary(t *testing.T) {
	ext := &Extension{
		Name:     "test_ext",
		Pkg:      "test_pkg",
		Version:  "1.0.0",
		Category: "TEST",
		License:  "MIT",
		Repo:     "PIGSTY",
		EnDesc:   "A test extension",
		PgVer:    []string{"17", "16"},
	}

	summary := ext.ToSummary(17)
	if summary == nil {
		t.Fatal("expected non-nil summary")
	}
	if summary.Name != "test_ext" {
		t.Errorf("expected name=test_ext, got %v", summary.Name)
	}
	if summary.Status != "not_avail" {
		// Since catalog is not initialized, status should be not_avail
		t.Logf("status is %v (expected not_avail when catalog unavailable)", summary.Status)
	}
}

func TestExtensionToInfoDataNil(t *testing.T) {
	var e *Extension
	info := e.ToInfoData()
	if info != nil {
		t.Error("expected nil for nil Extension")
	}
}

func TestExtensionToInfoData(t *testing.T) {
	ext := &Extension{
		Name:        "test_ext",
		Pkg:         "test_pkg",
		LeadExt:     "test_ext",
		Category:    "TEST",
		License:     "MIT",
		Lang:        "C",
		Version:     "1.0.0",
		URL:         "https://example.com",
		EnDesc:      "A test extension",
		ZhDesc:      "测试扩展",
		HasBin:      false,
		HasLib:      true,
		NeedLoad:    true,
		NeedDDL:     true,
		Relocatable: "t",
		Trusted:     "f",
		Requires:    []string{"plpgsql"},
		SeeAlso:     []string{"other_ext"},
		PgVer:       []string{"17", "16"},
		Schemas:     []string{"public"},
		RpmRepo:     "PIGSTY",
		RpmPkg:      "test_ext_$v",
		RpmVer:      "1.0.0",
		RpmPg:       []string{"17", "16"},
		Extra:       map[string]interface{}{"lib": "test_lib"},
	}

	info := ext.ToInfoData()
	if info == nil {
		t.Fatal("expected non-nil info")
	}
	if info.Name != "test_ext" {
		t.Errorf("expected name=test_ext, got %v", info.Name)
	}
	if info.Properties == nil {
		t.Fatal("expected Properties to be non-nil")
	}
	if !info.Properties.NeedLoad {
		t.Error("expected NeedLoad=true")
	}
	if info.Properties.Relocatable != "t" {
		t.Errorf("expected Relocatable=t, got %v", info.Properties.Relocatable)
	}
	if info.Operations == nil {
		t.Fatal("expected Operations to be non-nil")
	}
	if info.Operations.Config != "shared_preload_libraries = 'test_lib'" {
		t.Errorf("unexpected config: %v", info.Operations.Config)
	}
	if info.Operations.Create != "CREATE EXTENSION test_ext CASCADE;" {
		t.Errorf("unexpected create: %v", info.Operations.Create)
	}
	if info.RpmPackage == nil {
		t.Fatal("expected RpmPackage to be non-nil")
	}
	if info.RpmPackage.Repository != "PIGSTY" {
		t.Errorf("expected repository=PIGSTY, got %v", info.RpmPackage.Repository)
	}
}

/********************
 * Result Constructor Tests (require catalog)
 ********************/

func TestListExtensionsNoCatalog(t *testing.T) {
	// Save and restore original catalog
	origCatalog := Catalog
	defer func() { Catalog = origCatalog }()

	// Set empty catalog
	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{},
		ExtNameMap: make(map[string]*Extension),
		ExtPkgMap:  make(map[string]*Extension),
	}

	result := ListExtensions("test", 17)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.Success {
		t.Error("expected success=true even with no results")
	}
	if result.Data == nil {
		t.Fatal("expected non-nil data")
	}

	data, ok := result.Data.(*ExtensionListData)
	if !ok {
		t.Fatal("expected data to be *ExtensionListData")
	}
	if data.Count != 0 {
		t.Errorf("expected count=0, got %v", data.Count)
	}
}

func TestGetExtensionInfoNoNames(t *testing.T) {
	result := GetExtensionInfo([]string{})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Error("expected success=false for empty names")
	}
}

func TestGetExtensionInfoNilCatalog(t *testing.T) {
	// Save and restore
	origCatalog := Catalog
	defer func() { Catalog = origCatalog }()

	Catalog = nil

	result := GetExtensionInfo([]string{"test_ext"})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Error("expected success=false when catalog is nil")
	}
	if result.Code != 100701 { // CodeExtensionCatalogError
		t.Errorf("expected CodeExtensionCatalogError, got %d", result.Code)
	}
}

func TestGetExtensionInfoNotFound(t *testing.T) {
	// Save and restore original catalog
	origCatalog := Catalog
	defer func() { Catalog = origCatalog }()

	// Set empty catalog
	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{},
		ExtNameMap: make(map[string]*Extension),
		ExtPkgMap:  make(map[string]*Extension),
	}

	result := GetExtensionInfo([]string{"nonexistent_extension"})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Error("expected success=false for not found extension")
	}
}

func TestGetExtStatusNoPG(t *testing.T) {
	// Save and restore
	origPostgres := Postgres
	origCatalog := Catalog
	defer func() {
		Postgres = origPostgres
		Catalog = origCatalog
	}()

	Postgres = nil
	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{},
		ExtNameMap: make(map[string]*Extension),
		ExtPkgMap:  make(map[string]*Extension),
	}

	result := GetExtStatus(false)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Error("expected success=false when no PostgreSQL found")
	}
}

func TestGetExtStatusNilCatalog(t *testing.T) {
	// Save and restore
	origCatalog := Catalog
	defer func() { Catalog = origCatalog }()

	Catalog = nil

	result := GetExtStatus(false)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Error("expected success=false when catalog is nil")
	}
	if result.Code != 100701 { // CodeExtensionCatalogError
		t.Errorf("expected CodeExtensionCatalogError, got %d", result.Code)
	}
}

func TestGetExtensionAvailabilityNoCatalog(t *testing.T) {
	// Save and restore
	origCatalog := Catalog
	defer func() { Catalog = origCatalog }()

	Catalog = nil

	result := GetExtensionAvailability([]string{})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Error("expected success=false when no catalog")
	}
}

func TestGetExtensionAvailabilityNotFound(t *testing.T) {
	// Save and restore
	origCatalog := Catalog
	defer func() { Catalog = origCatalog }()

	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{},
		ExtNameMap: make(map[string]*Extension),
		ExtPkgMap:  make(map[string]*Extension),
	}

	result := GetExtensionAvailability([]string{"nonexistent"})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Error("expected success=false for not found extension")
	}
}

/********************
 * Additional Tests for Improved Coverage
 ********************/

func TestListExtensionsNilCatalog(t *testing.T) {
	// Save and restore
	origCatalog := Catalog
	defer func() { Catalog = origCatalog }()

	Catalog = nil

	result := ListExtensions("test", 17)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Error("expected success=false when catalog is nil")
	}
}

func TestListExtensionsWithExtensions(t *testing.T) {
	// Save and restore
	origCatalog := Catalog
	defer func() { Catalog = origCatalog }()

	ext1 := &Extension{
		Name:     "test_ext1",
		Pkg:      "test_pkg1",
		Version:  "1.0.0",
		Category: "TEST",
		Lead:     true,
		EnDesc:   "Test extension 1",
		PgVer:    []string{"17", "16"},
	}
	ext2 := &Extension{
		Name:     "test_ext2",
		Pkg:      "test_pkg2",
		Version:  "2.0.0",
		Category: "TEST",
		Lead:     true,
		EnDesc:   "Test extension 2",
		PgVer:    []string{"17"},
	}

	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{ext1, ext2},
		ExtNameMap: map[string]*Extension{"test_ext1": ext1, "test_ext2": ext2},
		ExtPkgMap:  map[string]*Extension{"test_pkg1": ext1, "test_pkg2": ext2},
	}

	result := ListExtensions("", 17)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.Success {
		t.Error("expected success=true")
	}

	data, ok := result.Data.(*ExtensionListData)
	if !ok {
		t.Fatal("expected data to be *ExtensionListData")
	}
	if data.Count != 2 {
		t.Errorf("expected count=2, got %v", data.Count)
	}
}

func TestGetExtensionInfoSuccess(t *testing.T) {
	// Save and restore
	origCatalog := Catalog
	defer func() { Catalog = origCatalog }()

	ext := &Extension{
		Name:     "test_ext",
		Pkg:      "test_pkg",
		Category: "TEST",
		License:  "MIT",
		Lang:     "C",
		Version:  "1.0.0",
		EnDesc:   "A test extension",
		PgVer:    []string{"17", "16"},
	}

	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{ext},
		ExtNameMap: map[string]*Extension{"test_ext": ext},
		ExtPkgMap:  map[string]*Extension{"test_pkg": ext},
	}

	result := GetExtensionInfo([]string{"test_ext"})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.Success {
		t.Error("expected success=true")
	}

	info, ok := result.Data.(*ExtensionInfoData)
	if !ok {
		t.Fatal("expected data to be *ExtensionInfoData")
	}
	if info.Name != "test_ext" {
		t.Errorf("expected name=test_ext, got %v", info.Name)
	}
}

func TestGetExtensionInfoMultiple(t *testing.T) {
	// Save and restore
	origCatalog := Catalog
	defer func() { Catalog = origCatalog }()

	ext1 := &Extension{Name: "ext1", Pkg: "pkg1", PgVer: []string{"17"}}
	ext2 := &Extension{Name: "ext2", Pkg: "pkg2", PgVer: []string{"17"}}

	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{ext1, ext2},
		ExtNameMap: map[string]*Extension{"ext1": ext1, "ext2": ext2},
		ExtPkgMap:  map[string]*Extension{"pkg1": ext1, "pkg2": ext2},
	}

	result := GetExtensionInfo([]string{"ext1", "ext2"})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.Success {
		t.Error("expected success=true")
	}

	infos, ok := result.Data.([]*ExtensionInfoData)
	if !ok {
		t.Fatal("expected data to be []*ExtensionInfoData")
	}
	if len(infos) != 2 {
		t.Errorf("expected 2 infos, got %v", len(infos))
	}
}

func TestGetExtensionInfoPartialNotFound(t *testing.T) {
	// Save and restore
	origCatalog := Catalog
	defer func() { Catalog = origCatalog }()

	ext := &Extension{Name: "ext1", Pkg: "pkg1", PgVer: []string{"17"}}

	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{ext},
		ExtNameMap: map[string]*Extension{"ext1": ext},
		ExtPkgMap:  map[string]*Extension{"pkg1": ext},
	}

	result := GetExtensionInfo([]string{"ext1", "nonexistent"})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.Success {
		t.Error("expected success=true (partial results)")
	}
	if result.Detail == "" {
		t.Error("expected Detail to contain not found info")
	}
}

func TestGetExtStatusWithPostgres(t *testing.T) {
	// Save and restore
	origPostgres := Postgres
	origCatalog := Catalog
	defer func() {
		Postgres = origPostgres
		Catalog = origCatalog
	}()

	ext := &Extension{
		Name:     "plpgsql",
		Pkg:      "plpgsql",
		Category: "LANG",
		Repo:     "CONTRIB",
		PgVer:    []string{"17"},
	}

	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{ext},
		ExtNameMap: map[string]*Extension{"plpgsql": ext},
		ExtPkgMap:  map[string]*Extension{"plpgsql": ext},
	}

	Postgres = &PostgresInstall{
		Version:      "PostgreSQL 17.0",
		MajorVersion: 17,
		BinPath:      "/usr/pgsql-17/bin",
		ExtPath:      "/usr/pgsql-17/share/extension",
		Extensions: []*ExtensionInstall{
			{Extension: ext},
		},
		ExtensionMap: map[string]*ExtensionInstall{
			"plpgsql": {Extension: ext},
		},
	}

	result := GetExtStatus(true) // include contrib
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.Success {
		t.Errorf("expected success=true, got %v with message: %v", result.Success, result.Message)
	}

	data, ok := result.Data.(*ExtensionStatusData)
	if !ok {
		t.Fatal("expected data to be *ExtensionStatusData")
	}
	if data.PgInfo == nil {
		t.Fatal("expected PgInfo to be non-nil")
	}
	if data.PgInfo.MajorVersion != 17 {
		t.Errorf("expected MajorVersion=17, got %v", data.PgInfo.MajorVersion)
	}
}

func TestBuildExtensionAvailDataNil(t *testing.T) {
	data := buildExtensionAvailData(nil)
	if data != nil {
		t.Error("expected nil for nil Extension")
	}
}

func TestBuildExtensionAvailData(t *testing.T) {
	// Save and restore
	origCatalog := Catalog
	defer func() { Catalog = origCatalog }()

	ext := &Extension{
		Name:    "test_ext",
		Pkg:     "test_pkg",
		Lead:    true,
		LeadExt: "test_ext",
		PgVer:   []string{"17", "16"},
	}

	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{ext},
		ExtNameMap: map[string]*Extension{"test_ext": ext},
		ExtPkgMap:  map[string]*Extension{"test_pkg": ext},
	}

	data := buildExtensionAvailData(ext)
	if data == nil {
		t.Fatal("expected non-nil data")
	}
	if data.Extension != "test_ext" {
		t.Errorf("expected extension=test_ext, got %v", data.Extension)
	}
}

func TestGetGlobalAvailabilityEmpty(t *testing.T) {
	// Save and restore
	origCatalog := Catalog
	defer func() { Catalog = origCatalog }()

	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{},
		ExtNameMap: make(map[string]*Extension),
		ExtPkgMap:  make(map[string]*Extension),
	}

	result := getGlobalAvailability("el9", "amd64")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.Success {
		t.Error("expected success=true")
	}

	data, ok := result.Data.(*ExtensionAvailData)
	if !ok {
		t.Fatal("expected data to be *ExtensionAvailData")
	}
	if data.PackageCount != 0 {
		t.Errorf("expected PackageCount=0, got %v", data.PackageCount)
	}
}

func TestGetExtensionAvailabilitiesMultiple(t *testing.T) {
	// Save and restore
	origCatalog := Catalog
	defer func() { Catalog = origCatalog }()

	ext1 := &Extension{Name: "ext1", Pkg: "pkg1", Lead: true, LeadExt: "ext1", PgVer: []string{"17"}}
	ext2 := &Extension{Name: "ext2", Pkg: "pkg2", Lead: true, LeadExt: "ext2", PgVer: []string{"17"}}

	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{ext1, ext2},
		ExtNameMap: map[string]*Extension{"ext1": ext1, "ext2": ext2},
		ExtPkgMap:  map[string]*Extension{"pkg1": ext1, "pkg2": ext2},
	}

	result := getExtensionAvailabilities([]string{"ext1", "ext2"}, "el9", "amd64")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.Success {
		t.Error("expected success=true")
	}

	data, ok := result.Data.([]*ExtensionAvailData)
	if !ok {
		t.Fatal("expected data to be []*ExtensionAvailData")
	}
	if len(data) != 2 {
		t.Errorf("expected 2 results, got %v", len(data))
	}
}

func TestGetSingleExtensionAvailability(t *testing.T) {
	// Save and restore
	origCatalog := Catalog
	defer func() { Catalog = origCatalog }()

	ext := &Extension{Name: "test_ext", Pkg: "test_pkg", Lead: true, LeadExt: "test_ext", PgVer: []string{"17"}}

	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{ext},
		ExtNameMap: map[string]*Extension{"test_ext": ext},
		ExtPkgMap:  map[string]*Extension{"test_pkg": ext},
	}

	result := getSingleExtensionAvailability("test_ext", "el9", "amd64")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.Success {
		t.Error("expected success=true")
	}

	data, ok := result.Data.(*ExtensionAvailData)
	if !ok {
		t.Fatal("expected data to be *ExtensionAvailData")
	}
	if data.Extension != "test_ext" {
		t.Errorf("expected extension=test_ext, got %v", data.Extension)
	}
}

func TestGetSingleExtensionAvailabilityByPkg(t *testing.T) {
	// Save and restore
	origCatalog := Catalog
	defer func() { Catalog = origCatalog }()

	ext := &Extension{Name: "test_ext", Pkg: "test_pkg", Lead: true, LeadExt: "test_ext", PgVer: []string{"17"}}

	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{ext},
		ExtNameMap: map[string]*Extension{"test_ext": ext},
		ExtPkgMap:  map[string]*Extension{"test_pkg": ext},
	}

	// Search by package name instead of extension name
	result := getSingleExtensionAvailability("test_pkg", "el9", "amd64")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.Success {
		t.Error("expected success=true")
	}
}

func TestToInfoDataWithNilCatalog(t *testing.T) {
	// Save and restore
	origCatalog := Catalog
	defer func() { Catalog = origCatalog }()

	Catalog = nil

	ext := &Extension{
		Name:     "test_ext",
		Pkg:      "test_pkg",
		PgVer:    []string{"17"},
		Requires: []string{"plpgsql"},
	}

	// This should not panic even with nil Catalog
	info := ext.ToInfoData()
	if info == nil {
		t.Fatal("expected non-nil info")
	}
	// RequiredBy should be nil/empty when Catalog is nil
	if info.RequiredBy != nil && len(info.RequiredBy) > 0 {
		t.Errorf("expected empty RequiredBy when Catalog is nil, got %v", info.RequiredBy)
	}
}

func TestToSummaryStatusInstalled(t *testing.T) {
	// Save and restore
	origPostgres := Postgres
	defer func() { Postgres = origPostgres }()

	ext := &Extension{
		Name:    "test_ext",
		Pkg:     "test_pkg",
		Version: "1.0.0",
		PgVer:   []string{"17"},
	}

	// Set up Postgres with the extension installed
	Postgres = &PostgresInstall{
		MajorVersion: 17,
		ExtensionMap: map[string]*ExtensionInstall{
			"test_ext": {Extension: ext},
		},
	}

	summary := ext.ToSummary(17)
	if summary == nil {
		t.Fatal("expected non-nil summary")
	}
	if summary.Status != "installed" {
		t.Errorf("expected status=installed, got %v", summary.Status)
	}
}

/********************
 * ExtensionAddData Tests (Story 3.2)
 ********************/

func TestExtensionAddDataJSONSerialization(t *testing.T) {
	data := &ExtensionAddData{
		PgVersion: 17,
		OSCode:    "el9",
		Arch:      "amd64",
		Requested: []string{"postgis", "pg_stat_statements"},
		Packages:  []string{"postgis36_17", "pg_stat_statements_17"},
		Installed: []*InstalledExtItem{
			{Name: "postgis", Package: "postgis36_17", Version: "3.5.0"},
			{Name: "pg_stat_statements", Package: "pg_stat_statements_17"},
		},
		Failed:      nil,
		DurationMs:  1500,
		AutoConfirm: true,
	}

	// Test JSON marshaling
	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("failed to marshal ExtensionAddData: %v", err)
	}

	// Verify JSON contains expected fields
	var result map[string]interface{}
	if err := json.Unmarshal(jsonData, &result); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if int(result["pg_version"].(float64)) != 17 {
		t.Errorf("expected pg_version=17, got %v", result["pg_version"])
	}
	if result["os_code"] != "el9" {
		t.Errorf("expected os_code=el9, got %v", result["os_code"])
	}
	if result["auto_confirm"] != true {
		t.Errorf("expected auto_confirm=true, got %v", result["auto_confirm"])
	}
	if int(result["duration_ms"].(float64)) != 1500 {
		t.Errorf("expected duration_ms=1500, got %v", result["duration_ms"])
	}

	// Check installed array
	installed := result["installed"].([]interface{})
	if len(installed) != 2 {
		t.Errorf("expected 2 installed items, got %v", len(installed))
	}
}

func TestExtensionAddDataYAMLSerialization(t *testing.T) {
	data := &ExtensionAddData{
		PgVersion: 16,
		OSCode:    "u24",
		Arch:      "arm64",
		Requested: []string{"pgvector"},
		Packages:  []string{"postgresql-16-pgvector"},
		Installed: []*InstalledExtItem{
			{Name: "pgvector", Package: "postgresql-16-pgvector", Version: "0.7.0"},
		},
		DurationMs:  2000,
		AutoConfirm: false,
	}

	// Test YAML marshaling
	yamlData, err := yaml.Marshal(data)
	if err != nil {
		t.Fatalf("failed to marshal ExtensionAddData to YAML: %v", err)
	}

	// Verify YAML round-trip
	var result ExtensionAddData
	if err := yaml.Unmarshal(yamlData, &result); err != nil {
		t.Fatalf("failed to unmarshal YAML: %v", err)
	}

	if result.PgVersion != 16 {
		t.Errorf("expected PgVersion=16, got %v", result.PgVersion)
	}
	if result.OSCode != "u24" {
		t.Errorf("expected OSCode=u24, got %v", result.OSCode)
	}
	if result.Arch != "arm64" {
		t.Errorf("expected Arch=arm64, got %v", result.Arch)
	}
	if len(result.Installed) != 1 {
		t.Errorf("expected 1 installed item, got %v", len(result.Installed))
	}
	if result.AutoConfirm != false {
		t.Errorf("expected AutoConfirm=false, got %v", result.AutoConfirm)
	}
}

func TestExtensionAddDataWithFailed(t *testing.T) {
	data := &ExtensionAddData{
		PgVersion: 17,
		OSCode:    "el9",
		Arch:      "amd64",
		Requested: []string{"postgis", "nonexistent_ext"},
		Packages:  []string{"postgis36_17"},
		Installed: []*InstalledExtItem{
			{Name: "postgis", Package: "postgis36_17"},
		},
		Failed: []*FailedExtItem{
			{Name: "nonexistent_ext", Error: "extension not found in catalog", Code: 100501},
		},
		DurationMs:  500,
		AutoConfirm: true,
	}

	// Test JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("failed to marshal ExtensionAddData: %v", err)
	}

	var jsonResult map[string]interface{}
	if err := json.Unmarshal(jsonData, &jsonResult); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	failed := jsonResult["failed"].([]interface{})
	if len(failed) != 1 {
		t.Errorf("expected 1 failed item, got %v", len(failed))
	}
	failedItem := failed[0].(map[string]interface{})
	if failedItem["name"] != "nonexistent_ext" {
		t.Errorf("expected name=nonexistent_ext, got %v", failedItem["name"])
	}
	if int(failedItem["code"].(float64)) != 100501 {
		t.Errorf("expected code=100501, got %v", failedItem["code"])
	}
}

func TestInstalledExtItemSerialization(t *testing.T) {
	item := &InstalledExtItem{
		Name:    "timescaledb",
		Package: "timescaledb-2-postgresql-17-tsl",
		Version: "2.17.0",
	}

	// JSON
	jsonData, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("failed to marshal InstalledExtItem: %v", err)
	}

	var jsonResult map[string]interface{}
	if err := json.Unmarshal(jsonData, &jsonResult); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if jsonResult["name"] != "timescaledb" {
		t.Errorf("expected name=timescaledb, got %v", jsonResult["name"])
	}
	if jsonResult["package"] != "timescaledb-2-postgresql-17-tsl" {
		t.Errorf("expected package=timescaledb-2-postgresql-17-tsl, got %v", jsonResult["package"])
	}
	if jsonResult["version"] != "2.17.0" {
		t.Errorf("expected version=2.17.0, got %v", jsonResult["version"])
	}
}

func TestFailedExtItemSerialization(t *testing.T) {
	item := &FailedExtItem{
		Name:    "broken_ext",
		Package: "broken-pkg-17",
		Error:   "package not found in repository",
		Code:    100801,
	}

	// YAML
	yamlData, err := yaml.Marshal(item)
	if err != nil {
		t.Fatalf("failed to marshal FailedExtItem: %v", err)
	}

	var yamlResult FailedExtItem
	if err := yaml.Unmarshal(yamlData, &yamlResult); err != nil {
		t.Fatalf("failed to unmarshal YAML: %v", err)
	}

	if yamlResult.Name != "broken_ext" {
		t.Errorf("expected Name=broken_ext, got %v", yamlResult.Name)
	}
	if yamlResult.Code != 100801 {
		t.Errorf("expected Code=100801, got %v", yamlResult.Code)
	}
	if yamlResult.Error != "package not found in repository" {
		t.Errorf("expected Error='package not found in repository', got %v", yamlResult.Error)
	}
}

func TestExtensionAddDataOmitEmpty(t *testing.T) {
	// Test that failed field is omitted when empty
	data := &ExtensionAddData{
		PgVersion: 17,
		OSCode:    "el9",
		Arch:      "amd64",
		Requested: []string{"postgis"},
		Packages:  []string{"postgis36_17"},
		Installed: []*InstalledExtItem{
			{Name: "postgis", Package: "postgis36_17"},
		},
		Failed:      nil, // should be omitted
		DurationMs:  100,
		AutoConfirm: true,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("failed to marshal ExtensionAddData: %v", err)
	}

	// Check that "failed" is not in the JSON output
	jsonStr := string(jsonData)
	if strings.Contains(jsonStr, "failed") {
		t.Errorf("expected 'failed' to be omitted when nil, got: %v", jsonStr)
	}
}

func TestInstalledExtItemVersionOmitEmpty(t *testing.T) {
	// Test that version field is omitted when empty
	item := &InstalledExtItem{
		Name:    "test",
		Package: "test-pkg",
		Version: "", // should be omitted
	}

	jsonData, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("failed to marshal InstalledExtItem: %v", err)
	}

	jsonStr := string(jsonData)
	if strings.Contains(jsonStr, "version") {
		t.Errorf("expected 'version' to be omitted when empty, got: %v", jsonStr)
	}
}

func TestFailedExtItemPackageOmitEmpty(t *testing.T) {
	// Test that package field is omitted when empty
	item := &FailedExtItem{
		Name:    "test",
		Package: "", // should be omitted
		Error:   "some error",
		Code:    100501,
	}

	jsonData, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("failed to marshal FailedExtItem: %v", err)
	}

	jsonStr := string(jsonData)
	if strings.Contains(jsonStr, "package") {
		t.Errorf("expected 'package' to be omitted when empty, got: %v", jsonStr)
	}
}

/********************
 * AddExtensions Function Tests (Story 3.2)
 ********************/

func TestAddExtensionsNoNames(t *testing.T) {
	result := AddExtensions(17, []string{}, true)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Error("expected success=false for empty names")
	}
	if result.Code != 100501 { // CodeExtensionNotFound
		t.Errorf("expected CodeExtensionNotFound (100501), got %d", result.Code)
	}
}

func TestAddExtensionsUnsupportedOS(t *testing.T) {
	// Save and restore OS type
	origOSType := config.OSType
	defer func() { config.OSType = origOSType }()

	config.OSType = config.DistroMAC

	result := AddExtensions(17, []string{"postgis"}, true)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Error("expected success=false for unsupported OS")
	}
	if result.Code != 100602 { // CodeExtensionUnsupportedOS
		t.Errorf("expected CodeExtensionUnsupportedOS (100602), got %d", result.Code)
	}
}

func TestAddExtensionsExtensionNotFound(t *testing.T) {
	// Save and restore
	origCatalog := Catalog
	origOSType := config.OSType
	defer func() {
		Catalog = origCatalog
		config.OSType = origOSType
	}()

	// Set to a supported OS type for testing
	config.OSType = config.DistroEL

	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{},
		ExtNameMap: make(map[string]*Extension),
		ExtPkgMap:  make(map[string]*Extension),
		AliasMap:   make(map[string]string),
	}

	result := AddExtensions(17, []string{"nonexistent_ext"}, true)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Error("expected success=false for nonexistent extension")
	}

	data, ok := result.Data.(*ExtensionAddData)
	if !ok {
		t.Fatal("expected data to be *ExtensionAddData")
	}
	if len(data.Failed) != 1 {
		t.Errorf("expected 1 failed item, got %d", len(data.Failed))
	}
	if data.Failed[0].Name != "nonexistent_ext" {
		t.Errorf("expected failed name=nonexistent_ext, got %v", data.Failed[0].Name)
	}
}

func TestAddExtensionsDefaultPgVersion(t *testing.T) {
	// Save and restore
	origCatalog := Catalog
	origOSType := config.OSType
	defer func() {
		Catalog = origCatalog
		config.OSType = origOSType
	}()

	config.OSType = config.DistroEL

	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{},
		ExtNameMap: make(map[string]*Extension),
		ExtPkgMap:  make(map[string]*Extension),
		AliasMap:   make(map[string]string),
	}

	// Pass pgVer=0 to test default version logic
	result := AddExtensions(0, []string{"test"}, true)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Should use PostgresLatestMajorVersion by default
	data, ok := result.Data.(*ExtensionAddData)
	if !ok {
		t.Fatal("expected data to be *ExtensionAddData")
	}
	if data.PgVersion != PostgresLatestMajorVersion {
		t.Errorf("expected PgVersion=%d, got %d", PostgresLatestMajorVersion, data.PgVersion)
	}
}

func TestAddExtensionsMixedResults(t *testing.T) {
	// Save and restore
	origCatalog := Catalog
	origOSType := config.OSType
	defer func() {
		Catalog = origCatalog
		config.OSType = origOSType
	}()

	config.OSType = config.DistroEL

	// Create a test extension with package
	ext := &Extension{
		Name:   "test_ext",
		Pkg:    "test_pkg",
		PgVer:  []string{"17"},
		RpmPkg: "test-pkg-$v",
		DebPkg: "postgresql-$v-test",
	}

	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{ext},
		ExtNameMap: map[string]*Extension{"test_ext": ext},
		ExtPkgMap:  map[string]*Extension{"test_pkg": ext},
		AliasMap:   make(map[string]string),
	}

	// One valid, one invalid extension
	result := AddExtensions(17, []string{"test_ext", "nonexistent"}, true)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	data, ok := result.Data.(*ExtensionAddData)
	if !ok {
		t.Fatal("expected data to be *ExtensionAddData")
	}

	// Should have recorded the not found extension in failed
	foundNotExistent := false
	for _, f := range data.Failed {
		if f.Name == "nonexistent" {
			foundNotExistent = true
			break
		}
	}
	if !foundNotExistent {
		t.Error("expected 'nonexistent' in failed list")
	}
}

func TestAddExtensionsNoPackageAvailable(t *testing.T) {
	// Save and restore
	origCatalog := Catalog
	origOSType := config.OSType
	defer func() {
		Catalog = origCatalog
		config.OSType = origOSType
	}()

	config.OSType = config.DistroEL

	// Create an extension with no package
	ext := &Extension{
		Name:   "no_pkg_ext",
		Pkg:    "no_pkg",
		PgVer:  []string{"17"},
		RpmPkg: "", // No RPM package
		DebPkg: "", // No DEB package
	}

	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{ext},
		ExtNameMap: map[string]*Extension{"no_pkg_ext": ext},
		ExtPkgMap:  map[string]*Extension{"no_pkg": ext},
		AliasMap:   make(map[string]string),
	}

	result := AddExtensions(17, []string{"no_pkg_ext"}, true)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Error("expected success=false when no package available")
	}

	data, ok := result.Data.(*ExtensionAddData)
	if !ok {
		t.Fatal("expected data to be *ExtensionAddData")
	}
	if len(data.Failed) != 1 {
		t.Errorf("expected 1 failed item, got %d", len(data.Failed))
	}
	if data.Failed[0].Code != 100502 { // CodeExtensionNoPackage
		t.Errorf("expected CodeExtensionNoPackage (100502), got %d", data.Failed[0].Code)
	}
}

func TestAddExtensionsDataFields(t *testing.T) {
	// Save and restore
	origCatalog := Catalog
	origOSType := config.OSType
	defer func() {
		Catalog = origCatalog
		config.OSType = origOSType
	}()

	config.OSType = config.DistroEL

	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{},
		ExtNameMap: make(map[string]*Extension),
		ExtPkgMap:  make(map[string]*Extension),
		AliasMap:   make(map[string]string),
	}

	result := AddExtensions(17, []string{"test"}, true)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	data, ok := result.Data.(*ExtensionAddData)
	if !ok {
		t.Fatal("expected data to be *ExtensionAddData")
	}

	// Verify data fields are populated
	if data.PgVersion != 17 {
		t.Errorf("expected PgVersion=17, got %d", data.PgVersion)
	}
	if data.AutoConfirm != true {
		t.Errorf("expected AutoConfirm=true, got %v", data.AutoConfirm)
	}
	if len(data.Requested) != 1 || data.Requested[0] != "test" {
		t.Errorf("expected Requested=[test], got %v", data.Requested)
	}
	if data.DurationMs < 0 {
		t.Errorf("expected DurationMs >= 0, got %d", data.DurationMs)
	}
}

func TestAddExtensionsWithVersion(t *testing.T) {
	// Test version specification in extension name (name=version format)
	// Save and restore
	origCatalog := Catalog
	origOSType := config.OSType
	defer func() {
		Catalog = origCatalog
		config.OSType = origOSType
	}()

	config.OSType = config.DistroEL

	ext := &Extension{
		Name:   "test_ext",
		Pkg:    "test_pkg",
		PgVer:  []string{"17"},
		RpmPkg: "test-pkg-$v",
	}

	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{ext},
		ExtNameMap: map[string]*Extension{"test_ext": ext},
		ExtPkgMap:  map[string]*Extension{"test_pkg": ext},
		AliasMap:   make(map[string]string),
	}

	// Test with version specification
	result := AddExtensions(17, []string{"test_ext=1.2.3"}, true)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	data, ok := result.Data.(*ExtensionAddData)
	if !ok {
		t.Fatal("expected data to be *ExtensionAddData")
	}

	// Package should have version appended (EL format: pkg-version)
	if len(data.Packages) != 1 || data.Packages[0] != "test-pkg-17-1.2.3" {
		t.Errorf("expected Packages=[test-pkg-17-1.2.3], got %v", data.Packages)
	}
}

func TestAddExtensionsDebPackageFormat(t *testing.T) {
	// Test Debian package name format
	// Save and restore
	origCatalog := Catalog
	origOSType := config.OSType
	defer func() {
		Catalog = origCatalog
		config.OSType = origOSType
	}()

	config.OSType = config.DistroDEB

	ext := &Extension{
		Name:   "test_ext",
		Pkg:    "test_pkg",
		PgVer:  []string{"17"},
		DebPkg: "postgresql-$v-test",
	}

	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{ext},
		ExtNameMap: map[string]*Extension{"test_ext": ext},
		ExtPkgMap:  map[string]*Extension{"test_pkg": ext},
		AliasMap:   make(map[string]string),
	}

	result := AddExtensions(17, []string{"test_ext"}, true)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	data, ok := result.Data.(*ExtensionAddData)
	if !ok {
		t.Fatal("expected data to be *ExtensionAddData")
	}

	// Package should have $v replaced with version
	if len(data.Packages) != 1 || data.Packages[0] != "postgresql-17-test" {
		t.Errorf("expected Packages=[postgresql-17-test], got %v", data.Packages)
	}
}

/********************
 * ExtensionRmData Tests (Story 3.3)
 ********************/

func TestExtensionRmDataJSONSerialization(t *testing.T) {
	data := &ExtensionRmData{
		PgVersion:   17,
		OSCode:      "el9",
		Arch:        "amd64",
		Requested:   []string{"postgis", "pg_stat_statements"},
		Packages:    []string{"postgis36_17", "pg_stat_statements_17"},
		Removed:     []string{"postgis", "pg_stat_statements"},
		Failed:      nil,
		DurationMs:  1200,
		AutoConfirm: true,
	}

	// Test JSON marshaling
	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("failed to marshal ExtensionRmData: %v", err)
	}

	// Verify JSON contains expected fields
	var result map[string]interface{}
	if err := json.Unmarshal(jsonData, &result); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if int(result["pg_version"].(float64)) != 17 {
		t.Errorf("expected pg_version=17, got %v", result["pg_version"])
	}
	if result["os_code"] != "el9" {
		t.Errorf("expected os_code=el9, got %v", result["os_code"])
	}
	if result["auto_confirm"] != true {
		t.Errorf("expected auto_confirm=true, got %v", result["auto_confirm"])
	}
	if int(result["duration_ms"].(float64)) != 1200 {
		t.Errorf("expected duration_ms=1200, got %v", result["duration_ms"])
	}

	// Check removed array
	removed := result["removed"].([]interface{})
	if len(removed) != 2 {
		t.Errorf("expected 2 removed items, got %v", len(removed))
	}
}

func TestExtensionRmDataYAMLSerialization(t *testing.T) {
	data := &ExtensionRmData{
		PgVersion:   16,
		OSCode:      "u24",
		Arch:        "arm64",
		Requested:   []string{"pgvector"},
		Packages:    []string{"postgresql-16-pgvector"},
		Removed:     []string{"pgvector"},
		DurationMs:  1800,
		AutoConfirm: false,
	}

	// Test YAML marshaling
	yamlData, err := yaml.Marshal(data)
	if err != nil {
		t.Fatalf("failed to marshal ExtensionRmData to YAML: %v", err)
	}

	// Verify YAML round-trip
	var result ExtensionRmData
	if err := yaml.Unmarshal(yamlData, &result); err != nil {
		t.Fatalf("failed to unmarshal YAML: %v", err)
	}

	if result.PgVersion != 16 {
		t.Errorf("expected PgVersion=16, got %v", result.PgVersion)
	}
	if result.OSCode != "u24" {
		t.Errorf("expected OSCode=u24, got %v", result.OSCode)
	}
	if result.Arch != "arm64" {
		t.Errorf("expected Arch=arm64, got %v", result.Arch)
	}
	if len(result.Removed) != 1 {
		t.Errorf("expected 1 removed item, got %v", len(result.Removed))
	}
	if result.AutoConfirm != false {
		t.Errorf("expected AutoConfirm=false, got %v", result.AutoConfirm)
	}
}

func TestExtensionRmDataWithFailed(t *testing.T) {
	data := &ExtensionRmData{
		PgVersion: 17,
		OSCode:    "el9",
		Arch:      "amd64",
		Requested: []string{"postgis", "nonexistent_ext"},
		Packages:  []string{"postgis36_17"},
		Removed:   []string{"postgis"},
		Failed: []*FailedExtItem{
			{Name: "nonexistent_ext", Error: "extension not found in catalog", Code: 100501},
		},
		DurationMs:  500,
		AutoConfirm: true,
	}

	// Test JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("failed to marshal ExtensionRmData: %v", err)
	}

	var jsonResult map[string]interface{}
	if err := json.Unmarshal(jsonData, &jsonResult); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	failed := jsonResult["failed"].([]interface{})
	if len(failed) != 1 {
		t.Errorf("expected 1 failed item, got %v", len(failed))
	}
	failedItem := failed[0].(map[string]interface{})
	if failedItem["name"] != "nonexistent_ext" {
		t.Errorf("expected name=nonexistent_ext, got %v", failedItem["name"])
	}
}

func TestExtensionRmDataOmitEmpty(t *testing.T) {
	// Test that failed field is omitted when empty
	data := &ExtensionRmData{
		PgVersion:   17,
		OSCode:      "el9",
		Arch:        "amd64",
		Requested:   []string{"postgis"},
		Packages:    []string{"postgis36_17"},
		Removed:     []string{"postgis"},
		Failed:      nil, // should be omitted
		DurationMs:  100,
		AutoConfirm: true,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("failed to marshal ExtensionRmData: %v", err)
	}

	// Check that "failed" is not in the JSON output
	jsonStr := string(jsonData)
	if strings.Contains(jsonStr, "failed") {
		t.Errorf("expected 'failed' to be omitted when nil, got: %v", jsonStr)
	}
}

/********************
 * ExtensionUpdateData Tests (Story 3.3)
 ********************/

func TestExtensionUpdateDataJSONSerialization(t *testing.T) {
	data := &ExtensionUpdateData{
		PgVersion:   17,
		OSCode:      "el9",
		Arch:        "amd64",
		Requested:   []string{"postgis", "pg_stat_statements"},
		Packages:    []string{"postgis36_17", "pg_stat_statements_17"},
		Updated:     []string{"postgis", "pg_stat_statements"},
		Failed:      nil,
		DurationMs:  2500,
		AutoConfirm: true,
	}

	// Test JSON marshaling
	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("failed to marshal ExtensionUpdateData: %v", err)
	}

	// Verify JSON contains expected fields
	var result map[string]interface{}
	if err := json.Unmarshal(jsonData, &result); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if int(result["pg_version"].(float64)) != 17 {
		t.Errorf("expected pg_version=17, got %v", result["pg_version"])
	}
	if result["os_code"] != "el9" {
		t.Errorf("expected os_code=el9, got %v", result["os_code"])
	}
	if result["auto_confirm"] != true {
		t.Errorf("expected auto_confirm=true, got %v", result["auto_confirm"])
	}
	if int(result["duration_ms"].(float64)) != 2500 {
		t.Errorf("expected duration_ms=2500, got %v", result["duration_ms"])
	}

	// Check updated array
	updated := result["updated"].([]interface{})
	if len(updated) != 2 {
		t.Errorf("expected 2 updated items, got %v", len(updated))
	}
}

func TestExtensionUpdateDataYAMLSerialization(t *testing.T) {
	data := &ExtensionUpdateData{
		PgVersion:   16,
		OSCode:      "u24",
		Arch:        "arm64",
		Requested:   []string{"pgvector"},
		Packages:    []string{"postgresql-16-pgvector"},
		Updated:     []string{"pgvector"},
		DurationMs:  3000,
		AutoConfirm: false,
	}

	// Test YAML marshaling
	yamlData, err := yaml.Marshal(data)
	if err != nil {
		t.Fatalf("failed to marshal ExtensionUpdateData to YAML: %v", err)
	}

	// Verify YAML round-trip
	var result ExtensionUpdateData
	if err := yaml.Unmarshal(yamlData, &result); err != nil {
		t.Fatalf("failed to unmarshal YAML: %v", err)
	}

	if result.PgVersion != 16 {
		t.Errorf("expected PgVersion=16, got %v", result.PgVersion)
	}
	if result.OSCode != "u24" {
		t.Errorf("expected OSCode=u24, got %v", result.OSCode)
	}
	if result.Arch != "arm64" {
		t.Errorf("expected Arch=arm64, got %v", result.Arch)
	}
	if len(result.Updated) != 1 {
		t.Errorf("expected 1 updated item, got %v", len(result.Updated))
	}
	if result.AutoConfirm != false {
		t.Errorf("expected AutoConfirm=false, got %v", result.AutoConfirm)
	}
}

func TestExtensionUpdateDataWithFailed(t *testing.T) {
	data := &ExtensionUpdateData{
		PgVersion: 17,
		OSCode:    "el9",
		Arch:      "amd64",
		Requested: []string{"postgis", "nonexistent_ext"},
		Packages:  []string{"postgis36_17"},
		Updated:   []string{"postgis"},
		Failed: []*FailedExtItem{
			{Name: "nonexistent_ext", Error: "extension not found in catalog", Code: 100501},
		},
		DurationMs:  600,
		AutoConfirm: true,
	}

	// Test JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("failed to marshal ExtensionUpdateData: %v", err)
	}

	var jsonResult map[string]interface{}
	if err := json.Unmarshal(jsonData, &jsonResult); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	failed := jsonResult["failed"].([]interface{})
	if len(failed) != 1 {
		t.Errorf("expected 1 failed item, got %v", len(failed))
	}
	failedItem := failed[0].(map[string]interface{})
	if failedItem["name"] != "nonexistent_ext" {
		t.Errorf("expected name=nonexistent_ext, got %v", failedItem["name"])
	}
}

func TestExtensionUpdateDataOmitEmpty(t *testing.T) {
	// Test that failed field is omitted when empty
	data := &ExtensionUpdateData{
		PgVersion:   17,
		OSCode:      "el9",
		Arch:        "amd64",
		Requested:   []string{"postgis"},
		Packages:    []string{"postgis36_17"},
		Updated:     []string{"postgis"},
		Failed:      nil, // should be omitted
		DurationMs:  100,
		AutoConfirm: true,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("failed to marshal ExtensionUpdateData: %v", err)
	}

	// Check that "failed" is not in the JSON output
	jsonStr := string(jsonData)
	if strings.Contains(jsonStr, "failed") {
		t.Errorf("expected 'failed' to be omitted when nil, got: %v", jsonStr)
	}
}

/********************
 * RmExtensions Function Tests (Story 3.3)
 ********************/

func TestRmExtensionsNoNames(t *testing.T) {
	result := RmExtensions(17, []string{}, true)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Error("expected success=false for empty names")
	}
	if result.Code != 100501 { // CodeExtensionNotFound
		t.Errorf("expected CodeExtensionNotFound (100501), got %d", result.Code)
	}
}

func TestRmExtensionsUnsupportedOS(t *testing.T) {
	// Save and restore OS type
	origOSType := config.OSType
	defer func() { config.OSType = origOSType }()

	config.OSType = config.DistroMAC

	result := RmExtensions(17, []string{"postgis"}, true)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Error("expected success=false for unsupported OS")
	}
	if result.Code != 100602 { // CodeExtensionUnsupportedOS
		t.Errorf("expected CodeExtensionUnsupportedOS (100602), got %d", result.Code)
	}
}

func TestRmExtensionsExtensionNotFound(t *testing.T) {
	// Save and restore
	origCatalog := Catalog
	origOSType := config.OSType
	defer func() {
		Catalog = origCatalog
		config.OSType = origOSType
	}()

	// Set to a supported OS type for testing
	config.OSType = config.DistroEL

	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{},
		ExtNameMap: make(map[string]*Extension),
		ExtPkgMap:  make(map[string]*Extension),
		AliasMap:   make(map[string]string),
	}

	result := RmExtensions(17, []string{"nonexistent_ext"}, true)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Error("expected success=false for nonexistent extension")
	}

	data, ok := result.Data.(*ExtensionRmData)
	if !ok {
		t.Fatal("expected data to be *ExtensionRmData")
	}
	if len(data.Failed) != 1 {
		t.Errorf("expected 1 failed item, got %d", len(data.Failed))
	}
	if data.Failed[0].Name != "nonexistent_ext" {
		t.Errorf("expected failed name=nonexistent_ext, got %v", data.Failed[0].Name)
	}
}

func TestRmExtensionsDefaultPgVersion(t *testing.T) {
	// Save and restore
	origCatalog := Catalog
	origOSType := config.OSType
	defer func() {
		Catalog = origCatalog
		config.OSType = origOSType
	}()

	config.OSType = config.DistroEL

	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{},
		ExtNameMap: make(map[string]*Extension),
		ExtPkgMap:  make(map[string]*Extension),
		AliasMap:   make(map[string]string),
	}

	// Pass pgVer=0 to test default version logic
	result := RmExtensions(0, []string{"test"}, true)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Should use PostgresLatestMajorVersion by default
	data, ok := result.Data.(*ExtensionRmData)
	if !ok {
		t.Fatal("expected data to be *ExtensionRmData")
	}
	if data.PgVersion != PostgresLatestMajorVersion {
		t.Errorf("expected PgVersion=%d, got %d", PostgresLatestMajorVersion, data.PgVersion)
	}
}

func TestRmExtensionsNoPackageAvailable(t *testing.T) {
	// Save and restore
	origCatalog := Catalog
	origOSType := config.OSType
	defer func() {
		Catalog = origCatalog
		config.OSType = origOSType
	}()

	config.OSType = config.DistroEL

	// Create an extension with no package
	ext := &Extension{
		Name:   "no_pkg_ext",
		Pkg:    "no_pkg",
		PgVer:  []string{"17"},
		RpmPkg: "", // No RPM package
		DebPkg: "", // No DEB package
	}

	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{ext},
		ExtNameMap: map[string]*Extension{"no_pkg_ext": ext},
		ExtPkgMap:  map[string]*Extension{"no_pkg": ext},
		AliasMap:   make(map[string]string),
	}

	result := RmExtensions(17, []string{"no_pkg_ext"}, true)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Error("expected success=false when no package available")
	}

	data, ok := result.Data.(*ExtensionRmData)
	if !ok {
		t.Fatal("expected data to be *ExtensionRmData")
	}
	if len(data.Failed) != 1 {
		t.Errorf("expected 1 failed item, got %d", len(data.Failed))
	}
	if data.Failed[0].Code != 100502 { // CodeExtensionNoPackage
		t.Errorf("expected CodeExtensionNoPackage (100502), got %d", data.Failed[0].Code)
	}
}

func TestRmExtensionsDataFields(t *testing.T) {
	// Save and restore
	origCatalog := Catalog
	origOSType := config.OSType
	defer func() {
		Catalog = origCatalog
		config.OSType = origOSType
	}()

	config.OSType = config.DistroEL

	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{},
		ExtNameMap: make(map[string]*Extension),
		ExtPkgMap:  make(map[string]*Extension),
		AliasMap:   make(map[string]string),
	}

	result := RmExtensions(17, []string{"test"}, true)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	data, ok := result.Data.(*ExtensionRmData)
	if !ok {
		t.Fatal("expected data to be *ExtensionRmData")
	}

	// Verify data fields are populated
	if data.PgVersion != 17 {
		t.Errorf("expected PgVersion=17, got %d", data.PgVersion)
	}
	if data.AutoConfirm != true {
		t.Errorf("expected AutoConfirm=true, got %v", data.AutoConfirm)
	}
	if len(data.Requested) != 1 || data.Requested[0] != "test" {
		t.Errorf("expected Requested=[test], got %v", data.Requested)
	}
	if data.DurationMs < 0 {
		t.Errorf("expected DurationMs >= 0, got %d", data.DurationMs)
	}
}

/********************
 * UpgradeExtensions Function Tests (Story 3.3)
 ********************/

func TestUpgradeExtensionsNoNames(t *testing.T) {
	result := UpgradeExtensions(17, []string{}, true)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Error("expected success=false for empty names")
	}
	if result.Code != 100501 { // CodeExtensionNotFound
		t.Errorf("expected CodeExtensionNotFound (100501), got %d", result.Code)
	}
}

func TestUpgradeExtensionsUnsupportedOS(t *testing.T) {
	// Save and restore OS type
	origOSType := config.OSType
	defer func() { config.OSType = origOSType }()

	config.OSType = config.DistroMAC

	result := UpgradeExtensions(17, []string{"postgis"}, true)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Error("expected success=false for unsupported OS")
	}
	if result.Code != 100602 { // CodeExtensionUnsupportedOS
		t.Errorf("expected CodeExtensionUnsupportedOS (100602), got %d", result.Code)
	}
}

func TestUpgradeExtensionsExtensionNotFound(t *testing.T) {
	// Save and restore
	origCatalog := Catalog
	origOSType := config.OSType
	defer func() {
		Catalog = origCatalog
		config.OSType = origOSType
	}()

	// Set to a supported OS type for testing
	config.OSType = config.DistroEL

	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{},
		ExtNameMap: make(map[string]*Extension),
		ExtPkgMap:  make(map[string]*Extension),
		AliasMap:   make(map[string]string),
	}

	result := UpgradeExtensions(17, []string{"nonexistent_ext"}, true)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Error("expected success=false for nonexistent extension")
	}

	data, ok := result.Data.(*ExtensionUpdateData)
	if !ok {
		t.Fatal("expected data to be *ExtensionUpdateData")
	}
	if len(data.Failed) != 1 {
		t.Errorf("expected 1 failed item, got %d", len(data.Failed))
	}
	if data.Failed[0].Name != "nonexistent_ext" {
		t.Errorf("expected failed name=nonexistent_ext, got %v", data.Failed[0].Name)
	}
}

func TestUpgradeExtensionsDefaultPgVersion(t *testing.T) {
	// Save and restore
	origCatalog := Catalog
	origOSType := config.OSType
	defer func() {
		Catalog = origCatalog
		config.OSType = origOSType
	}()

	config.OSType = config.DistroEL

	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{},
		ExtNameMap: make(map[string]*Extension),
		ExtPkgMap:  make(map[string]*Extension),
		AliasMap:   make(map[string]string),
	}

	// Pass pgVer=0 to test default version logic
	result := UpgradeExtensions(0, []string{"test"}, true)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Should use PostgresLatestMajorVersion by default
	data, ok := result.Data.(*ExtensionUpdateData)
	if !ok {
		t.Fatal("expected data to be *ExtensionUpdateData")
	}
	if data.PgVersion != PostgresLatestMajorVersion {
		t.Errorf("expected PgVersion=%d, got %d", PostgresLatestMajorVersion, data.PgVersion)
	}
}

func TestUpgradeExtensionsNoPackageAvailable(t *testing.T) {
	// Save and restore
	origCatalog := Catalog
	origOSType := config.OSType
	defer func() {
		Catalog = origCatalog
		config.OSType = origOSType
	}()

	config.OSType = config.DistroEL

	// Create an extension with no package
	ext := &Extension{
		Name:   "no_pkg_ext",
		Pkg:    "no_pkg",
		PgVer:  []string{"17"},
		RpmPkg: "", // No RPM package
		DebPkg: "", // No DEB package
	}

	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{ext},
		ExtNameMap: map[string]*Extension{"no_pkg_ext": ext},
		ExtPkgMap:  map[string]*Extension{"no_pkg": ext},
		AliasMap:   make(map[string]string),
	}

	result := UpgradeExtensions(17, []string{"no_pkg_ext"}, true)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Error("expected success=false when no package available")
	}

	data, ok := result.Data.(*ExtensionUpdateData)
	if !ok {
		t.Fatal("expected data to be *ExtensionUpdateData")
	}
	if len(data.Failed) != 1 {
		t.Errorf("expected 1 failed item, got %d", len(data.Failed))
	}
	if data.Failed[0].Code != 100502 { // CodeExtensionNoPackage
		t.Errorf("expected CodeExtensionNoPackage (100502), got %d", data.Failed[0].Code)
	}
}

func TestUpgradeExtensionsDataFields(t *testing.T) {
	// Save and restore
	origCatalog := Catalog
	origOSType := config.OSType
	defer func() {
		Catalog = origCatalog
		config.OSType = origOSType
	}()

	config.OSType = config.DistroEL

	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{},
		ExtNameMap: make(map[string]*Extension),
		ExtPkgMap:  make(map[string]*Extension),
		AliasMap:   make(map[string]string),
	}

	result := UpgradeExtensions(17, []string{"test"}, true)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	data, ok := result.Data.(*ExtensionUpdateData)
	if !ok {
		t.Fatal("expected data to be *ExtensionUpdateData")
	}

	// Verify data fields are populated
	if data.PgVersion != 17 {
		t.Errorf("expected PgVersion=17, got %d", data.PgVersion)
	}
	if data.AutoConfirm != true {
		t.Errorf("expected AutoConfirm=true, got %v", data.AutoConfirm)
	}
	if len(data.Requested) != 1 || data.Requested[0] != "test" {
		t.Errorf("expected Requested=[test], got %v", data.Requested)
	}
	if data.DurationMs < 0 {
		t.Errorf("expected DurationMs >= 0, got %d", data.DurationMs)
	}
}

/********************
 * ImportResultData Tests (Story 3.4)
 ********************/

func TestImportResultDataJSONSerialization(t *testing.T) {
	data := &ImportResultData{
		PgVersion:  17,
		OSCode:     "el9",
		Arch:       "amd64",
		RepoDir:    "/www/pigsty",
		Requested:  []string{"postgis", "pgvector"},
		Packages:   []string{"postgis36_17", "pgvector_17"},
		PkgCount:   2,
		Downloaded: []string{"postgis36_17", "pgvector_17"},
		Failed:     nil,
		DurationMs: 5000,
	}

	// Test JSON marshaling
	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("failed to marshal ImportResultData: %v", err)
	}

	// Verify JSON contains expected fields
	var result map[string]interface{}
	if err := json.Unmarshal(jsonData, &result); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if int(result["pg_version"].(float64)) != 17 {
		t.Errorf("expected pg_version=17, got %v", result["pg_version"])
	}
	if result["repo_dir"] != "/www/pigsty" {
		t.Errorf("expected repo_dir=/www/pigsty, got %v", result["repo_dir"])
	}
	if int(result["pkg_count"].(float64)) != 2 {
		t.Errorf("expected pkg_count=2, got %v", result["pkg_count"])
	}
	if int(result["duration_ms"].(float64)) != 5000 {
		t.Errorf("expected duration_ms=5000, got %v", result["duration_ms"])
	}
}

func TestImportResultDataYAMLSerialization(t *testing.T) {
	data := &ImportResultData{
		PgVersion:  16,
		OSCode:     "u24",
		Arch:       "arm64",
		RepoDir:    "/www/pigsty",
		Requested:  []string{"timescaledb"},
		Packages:   []string{"postgresql-16-timescaledb"},
		PkgCount:   1,
		Downloaded: []string{"postgresql-16-timescaledb"},
		DurationMs: 3000,
	}

	// Test YAML marshaling
	yamlData, err := yaml.Marshal(data)
	if err != nil {
		t.Fatalf("failed to marshal ImportResultData to YAML: %v", err)
	}

	// Verify YAML round-trip
	var result ImportResultData
	if err := yaml.Unmarshal(yamlData, &result); err != nil {
		t.Fatalf("failed to unmarshal YAML: %v", err)
	}

	if result.PgVersion != 16 {
		t.Errorf("expected PgVersion=16, got %v", result.PgVersion)
	}
	if result.RepoDir != "/www/pigsty" {
		t.Errorf("expected RepoDir=/www/pigsty, got %v", result.RepoDir)
	}
	if result.PkgCount != 1 {
		t.Errorf("expected PkgCount=1, got %v", result.PkgCount)
	}
}

func TestImportResultDataWithFailed(t *testing.T) {
	data := &ImportResultData{
		PgVersion:  17,
		OSCode:     "el9",
		Arch:       "amd64",
		RepoDir:    "/www/pigsty",
		Requested:  []string{"postgis", "nonexistent"},
		Packages:   []string{"postgis36_17"},
		PkgCount:   1,
		Downloaded: []string{"postgis36_17"},
		Failed:     []string{"nonexistent"},
		DurationMs: 2000,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("failed to marshal ImportResultData: %v", err)
	}

	var jsonResult map[string]interface{}
	if err := json.Unmarshal(jsonData, &jsonResult); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	failed := jsonResult["failed"].([]interface{})
	if len(failed) != 1 {
		t.Errorf("expected 1 failed item, got %v", len(failed))
	}
	if failed[0] != "nonexistent" {
		t.Errorf("expected failed[0]=nonexistent, got %v", failed[0])
	}
}

func TestImportResultDataOmitEmpty(t *testing.T) {
	data := &ImportResultData{
		PgVersion:  17,
		OSCode:     "el9",
		Arch:       "amd64",
		RepoDir:    "/www/pigsty",
		Requested:  []string{"postgis"},
		Packages:   []string{"postgis36_17"},
		PkgCount:   1,
		Downloaded: nil, // should be omitted
		Failed:     nil, // should be omitted
		DurationMs: 1000,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("failed to marshal ImportResultData: %v", err)
	}

	jsonStr := string(jsonData)
	if strings.Contains(jsonStr, "\"downloaded\"") {
		t.Errorf("expected 'downloaded' to be omitted when nil, got: %v", jsonStr)
	}
	if strings.Contains(jsonStr, "\"failed\"") {
		t.Errorf("expected 'failed' to be omitted when nil, got: %v", jsonStr)
	}
}

/********************
 * ScanResultData Tests (Story 3.4)
 ********************/

func TestScanResultDataJSONSerialization(t *testing.T) {
	data := &ScanResultData{
		PgInfo: &PostgresInfo{
			Version:      "PostgreSQL 17.2",
			MajorVersion: 17,
			BinDir:       "/usr/pgsql-17/bin",
			ExtensionDir: "/usr/pgsql-17/share/extension",
		},
		ExtCount: 2,
		Extensions: []*ScanExtEntry{
			{
				Name:        "plpgsql",
				ControlName: "plpgsql",
				Version:     "1.0",
				Description: "PL/pgSQL procedural language",
				InCatalog:   true,
			},
			{
				Name:        "postgis",
				ControlName: "postgis",
				Version:     "3.5.0",
				Description: "PostGIS geometry extension",
				Libraries:   []string{"postgis-3.so"},
				InCatalog:   true,
			},
		},
		UnmatchedLibs: []string{"custom_lib"},
		EncodingLibs:  []string{"utf8_and_latin1"},
		BuiltInLibs:   []string{"llvmjit"},
	}

	// Test JSON marshaling
	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("failed to marshal ScanResultData: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(jsonData, &result); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if int(result["extension_count"].(float64)) != 2 {
		t.Errorf("expected extension_count=2, got %v", result["extension_count"])
	}

	pgInfo := result["pg_info"].(map[string]interface{})
	if int(pgInfo["major_version"].(float64)) != 17 {
		t.Errorf("expected major_version=17, got %v", pgInfo["major_version"])
	}

	extensions := result["extensions"].([]interface{})
	if len(extensions) != 2 {
		t.Errorf("expected 2 extensions, got %v", len(extensions))
	}
}

func TestScanResultDataYAMLSerialization(t *testing.T) {
	data := &ScanResultData{
		PgInfo: &PostgresInfo{
			Version:      "PostgreSQL 16.0",
			MajorVersion: 16,
			BinDir:       "/usr/lib/postgresql/16/bin",
			ExtensionDir: "/usr/share/postgresql/16/extension",
		},
		ExtCount: 1,
		Extensions: []*ScanExtEntry{
			{Name: "pgvector", Version: "0.7.0", InCatalog: true},
		},
	}

	yamlData, err := yaml.Marshal(data)
	if err != nil {
		t.Fatalf("failed to marshal ScanResultData to YAML: %v", err)
	}

	var result ScanResultData
	if err := yaml.Unmarshal(yamlData, &result); err != nil {
		t.Fatalf("failed to unmarshal YAML: %v", err)
	}

	if result.ExtCount != 1 {
		t.Errorf("expected ExtCount=1, got %v", result.ExtCount)
	}
	if result.PgInfo.MajorVersion != 16 {
		t.Errorf("expected MajorVersion=16, got %v", result.PgInfo.MajorVersion)
	}
}

func TestScanExtEntrySerialization(t *testing.T) {
	entry := &ScanExtEntry{
		Name:        "timescaledb",
		ControlName: "timescaledb",
		Version:     "2.17.0",
		Description: "Scalable inserts for time-series data",
		Libraries:   []string{"timescaledb-2.17.so"},
		InCatalog:   true,
		ControlMeta: map[string]string{
			"superuser":   "true",
			"relocatable": "false",
		},
	}

	// JSON
	jsonData, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("failed to marshal ScanExtEntry: %v", err)
	}

	var jsonResult map[string]interface{}
	if err := json.Unmarshal(jsonData, &jsonResult); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if jsonResult["name"] != "timescaledb" {
		t.Errorf("expected name=timescaledb, got %v", jsonResult["name"])
	}
	if jsonResult["in_catalog"] != true {
		t.Errorf("expected in_catalog=true, got %v", jsonResult["in_catalog"])
	}

	libs := jsonResult["libraries"].([]interface{})
	if len(libs) != 1 {
		t.Errorf("expected 1 library, got %v", len(libs))
	}
}

func TestScanExtEntryOmitEmpty(t *testing.T) {
	entry := &ScanExtEntry{
		Name:      "simple_ext",
		Version:   "1.0",
		InCatalog: false,
		// Other fields nil/empty - should be omitted
	}

	jsonData, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("failed to marshal ScanExtEntry: %v", err)
	}

	jsonStr := string(jsonData)
	if strings.Contains(jsonStr, "\"control_name\"") {
		t.Errorf("expected 'control_name' to be omitted when empty, got: %v", jsonStr)
	}
	if strings.Contains(jsonStr, "\"description\"") {
		t.Errorf("expected 'description' to be omitted when empty, got: %v", jsonStr)
	}
	if strings.Contains(jsonStr, "\"libraries\"") {
		t.Errorf("expected 'libraries' to be omitted when nil, got: %v", jsonStr)
	}
	if strings.Contains(jsonStr, "\"control_meta\"") {
		t.Errorf("expected 'control_meta' to be omitted when nil, got: %v", jsonStr)
	}
}

/********************
 * LinkResultData Tests (Story 3.4)
 ********************/

func TestLinkResultDataJSONSerialization(t *testing.T) {
	data := &LinkResultData{
		Action:       "link",
		PgHome:       "/usr/pgsql-17",
		SymlinkPath:  "/usr/pgsql",
		ProfilePath:  "/etc/profile.d/pgsql.sh",
		ActivatedCmd: ". /etc/profile.d/pgsql.sh",
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("failed to marshal LinkResultData: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(jsonData, &result); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if result["action"] != "link" {
		t.Errorf("expected action=link, got %v", result["action"])
	}
	if result["pg_home"] != "/usr/pgsql-17" {
		t.Errorf("expected pg_home=/usr/pgsql-17, got %v", result["pg_home"])
	}
	if result["symlink_path"] != "/usr/pgsql" {
		t.Errorf("expected symlink_path=/usr/pgsql, got %v", result["symlink_path"])
	}
	if result["activated_cmd"] != ". /etc/profile.d/pgsql.sh" {
		t.Errorf("expected activated_cmd='. /etc/profile.d/pgsql.sh', got %v", result["activated_cmd"])
	}
}

func TestLinkResultDataYAMLSerialization(t *testing.T) {
	data := &LinkResultData{
		Action:      "unlink",
		SymlinkPath: "/usr/pgsql",
		ProfilePath: "/etc/profile.d/pgsql.sh",
	}

	yamlData, err := yaml.Marshal(data)
	if err != nil {
		t.Fatalf("failed to marshal LinkResultData to YAML: %v", err)
	}

	var result LinkResultData
	if err := yaml.Unmarshal(yamlData, &result); err != nil {
		t.Fatalf("failed to unmarshal YAML: %v", err)
	}

	if result.Action != "unlink" {
		t.Errorf("expected Action=unlink, got %v", result.Action)
	}
	if result.PgHome != "" {
		t.Errorf("expected PgHome to be empty for unlink, got %v", result.PgHome)
	}
}

func TestLinkResultDataOmitEmpty(t *testing.T) {
	data := &LinkResultData{
		Action:      "unlink",
		SymlinkPath: "/usr/pgsql",
		ProfilePath: "/etc/profile.d/pgsql.sh",
		// PgHome and ActivatedCmd should be omitted
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("failed to marshal LinkResultData: %v", err)
	}

	jsonStr := string(jsonData)
	if strings.Contains(jsonStr, "\"pg_home\"") {
		t.Errorf("expected 'pg_home' to be omitted when empty, got: %v", jsonStr)
	}
	if strings.Contains(jsonStr, "\"activated_cmd\"") {
		t.Errorf("expected 'activated_cmd' to be omitted when empty, got: %v", jsonStr)
	}
}

/********************
 * ReloadResultData Tests (Story 3.4)
 ********************/

func TestReloadResultDataJSONSerialization(t *testing.T) {
	data := &ReloadResultData{
		SourceURL:      "https://pigsty.io/ext/data/extension.csv",
		ExtensionCount: 445,
		CatalogPath:    "/home/user/.pig/extension.csv",
		DownloadedAt:   "2026-02-02T15:04:05Z",
		DurationMs:     1500,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("failed to marshal ReloadResultData: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(jsonData, &result); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if result["source_url"] != "https://pigsty.io/ext/data/extension.csv" {
		t.Errorf("expected source_url='https://pigsty.io/ext/data/extension.csv', got %v", result["source_url"])
	}
	if int(result["extension_count"].(float64)) != 445 {
		t.Errorf("expected extension_count=445, got %v", result["extension_count"])
	}
	if result["catalog_path"] != "/home/user/.pig/extension.csv" {
		t.Errorf("expected catalog_path='/home/user/.pig/extension.csv', got %v", result["catalog_path"])
	}
	if int(result["duration_ms"].(float64)) != 1500 {
		t.Errorf("expected duration_ms=1500, got %v", result["duration_ms"])
	}
}

func TestReloadResultDataYAMLSerialization(t *testing.T) {
	data := &ReloadResultData{
		SourceURL:      "https://pigsty.cc/ext/data/extension.csv",
		ExtensionCount: 440,
		CatalogPath:    "/root/.pig/extension.csv",
		DownloadedAt:   "2026-02-02T10:00:00Z",
		DurationMs:     2000,
	}

	yamlData, err := yaml.Marshal(data)
	if err != nil {
		t.Fatalf("failed to marshal ReloadResultData to YAML: %v", err)
	}

	var result ReloadResultData
	if err := yaml.Unmarshal(yamlData, &result); err != nil {
		t.Fatalf("failed to unmarshal YAML: %v", err)
	}

	if result.ExtensionCount != 440 {
		t.Errorf("expected ExtensionCount=440, got %v", result.ExtensionCount)
	}
	if result.DurationMs != 2000 {
		t.Errorf("expected DurationMs=2000, got %v", result.DurationMs)
	}
}

/********************
 * ToScanEntry Tests (Story 3.4)
 ********************/

func TestExtensionInstallToScanEntryNil(t *testing.T) {
	var ei *ExtensionInstall
	entry := ei.ToScanEntry()
	if entry != nil {
		t.Error("expected nil for nil ExtensionInstall")
	}
}

func TestExtensionInstallToScanEntry(t *testing.T) {
	ext := &Extension{
		Name:     "postgis",
		Pkg:      "postgis",
		Version:  "3.5.0",
		EnDesc:   "PostGIS geometry extension",
	}

	ei := &ExtensionInstall{
		Extension:      ext,
		ControlName:    "postgis",
		InstallVersion: "3.5.0",
		ControlDesc:    "PostGIS geometry extension",
		Libraries:      map[string]bool{"postgis-3": true},
		ControlMeta:    map[string]string{"superuser": "false"},
	}

	entry := ei.ToScanEntry()
	if entry == nil {
		t.Fatal("expected non-nil entry")
	}
	if entry.Name != "postgis" {
		t.Errorf("expected Name=postgis, got %v", entry.Name)
	}
	if entry.ControlName != "postgis" {
		t.Errorf("expected ControlName=postgis, got %v", entry.ControlName)
	}
	if entry.Version != "3.5.0" {
		t.Errorf("expected Version=3.5.0, got %v", entry.Version)
	}
	if !entry.InCatalog {
		t.Error("expected InCatalog=true when Extension is set")
	}
	if len(entry.Libraries) != 1 {
		t.Errorf("expected 1 library, got %v", len(entry.Libraries))
	}
}

func TestExtensionInstallToScanEntryNoCatalog(t *testing.T) {
	ei := &ExtensionInstall{
		Extension:      nil, // Not in catalog
		ControlName:    "custom_ext",
		InstallVersion: "1.0.0",
		ControlDesc:    "A custom extension",
		Libraries:      map[string]bool{},
	}

	entry := ei.ToScanEntry()
	if entry == nil {
		t.Fatal("expected non-nil entry")
	}
	if entry.InCatalog {
		t.Error("expected InCatalog=false when Extension is nil")
	}
	if entry.Name != "custom_ext" {
		t.Errorf("expected Name=custom_ext, got %v", entry.Name)
	}
}

/********************
 * ScanExtensionsResult Tests (Story 3.4)
 ********************/

func TestScanExtensionsResultNilCatalog(t *testing.T) {
	// Save and restore
	origCatalog := Catalog
	defer func() { Catalog = origCatalog }()

	Catalog = nil

	result := ScanExtensionsResult()
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Error("expected success=false when catalog is nil")
	}
	if result.Code != 100701 { // CodeExtensionCatalogError
		t.Errorf("expected CodeExtensionCatalogError (100701), got %d", result.Code)
	}
}

func TestScanExtensionsResultNoPG(t *testing.T) {
	// Save and restore
	origCatalog := Catalog
	origPostgres := Postgres
	defer func() {
		Catalog = origCatalog
		Postgres = origPostgres
	}()

	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{},
		ExtNameMap: make(map[string]*Extension),
		ExtPkgMap:  make(map[string]*Extension),
	}
	Postgres = nil

	result := ScanExtensionsResult()
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Error("expected success=false when no PostgreSQL found")
	}
	if result.Code != 100601 { // CodeExtensionNoPG
		t.Errorf("expected CodeExtensionNoPG (100601), got %d", result.Code)
	}
}
