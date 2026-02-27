/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>
*/
package ext

import (
	"encoding/json"
	"pig/internal/config"
	"pig/internal/output"
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
	origPostgres := Postgres
	origOSCode := config.OSCode
	origOSArch := config.OSArch
	origOSType := config.OSType
	defer func() {
		Postgres = origPostgres
		config.OSCode = origOSCode
		config.OSArch = origOSArch
		config.OSType = origOSType
	}()

	Postgres = nil
	config.OSCode = "a23"
	config.OSArch = "amd64"
	config.OSType = config.DistroMAC

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
	if summary.Status != "available" {
		t.Fatalf("expected status=available on unsupported OS fallback, got %v", summary.Status)
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
	if result.Code != output.CodeExtensionInvalidArgs {
		t.Errorf("expected CodeExtensionInvalidArgs (%d), got %d", output.CodeExtensionInvalidArgs, result.Code)
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

func TestGetExtStatusTotalsIndependentOfContribFilter(t *testing.T) {
	// Save and restore
	origPostgres := Postgres
	origCatalog := Catalog
	origOSType := config.OSType
	defer func() {
		Postgres = origPostgres
		Catalog = origCatalog
		config.OSType = origOSType
	}()

	config.OSType = config.DistroEL

	extContrib := &Extension{
		Name:    "plpgsql",
		Pkg:     "plpgsql",
		Repo:    "CONTRIB",
		RpmRepo: "CONTRIB",
		PgVer:   []string{"17"},
	}
	extPgdg := &Extension{
		Name:    "postgis",
		Pkg:     "postgis",
		Repo:    "PGDG",
		RpmRepo: "PGDG",
		PgVer:   []string{"17"},
	}

	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{extContrib, extPgdg},
		ExtNameMap: map[string]*Extension{"plpgsql": extContrib, "postgis": extPgdg},
		ExtPkgMap:  map[string]*Extension{"plpgsql": extContrib, "postgis": extPgdg},
	}

	Postgres = &PostgresInstall{
		Version:      "PostgreSQL 17.0",
		MajorVersion: 17,
		Extensions: []*ExtensionInstall{
			{Extension: extContrib},
			{Extension: extPgdg},
		},
		ExtensionMap: map[string]*ExtensionInstall{
			"plpgsql": {Extension: extContrib},
			"postgis": {Extension: extPgdg},
		},
	}

	result := GetExtStatus(false) // hide contrib
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.Success {
		t.Fatalf("expected success=true, got false (code=%d msg=%q)", result.Code, result.Message)
	}

	data, ok := result.Data.(*ExtensionStatusData)
	if !ok {
		t.Fatal("expected data to be *ExtensionStatusData")
	}
	if data.Summary == nil {
		t.Fatal("expected Summary to be non-nil")
	}
	if data.Summary.TotalInstalled != 2 {
		t.Fatalf("expected TotalInstalled=2, got %d", data.Summary.TotalInstalled)
	}
	if data.Summary.ByRepo["CONTRIB"] != 1 {
		t.Fatalf("expected ByRepo[CONTRIB]=1, got %d", data.Summary.ByRepo["CONTRIB"])
	}
	if data.Summary.ByRepo["PGDG"] != 1 {
		t.Fatalf("expected ByRepo[PGDG]=1, got %d", data.Summary.ByRepo["PGDG"])
	}
	if len(data.Extensions) != 1 {
		t.Fatalf("expected 1 shown extension (contrib hidden), got %d", len(data.Extensions))
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
	if len(info.RequiredBy) > 0 {
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

func TestToSummaryStatusAvailable_MatrixPreferred(t *testing.T) {
	// Save and restore
	origCatalog := Catalog
	origPostgres := Postgres
	origOSCode := config.OSCode
	origOSArch := config.OSArch
	origOSType := config.OSType
	defer func() {
		Catalog = origCatalog
		Postgres = origPostgres
		config.OSCode = origOSCode
		config.OSArch = origOSArch
		config.OSType = origOSType
	}()

	config.OSCode = "el9"
	config.OSArch = "amd64"
	config.OSType = config.DistroEL
	Postgres = nil

	lead := &Extension{
		Name:   "lead_ext",
		Pkg:    "lead_pkg",
		Lead:   true,
		PgVer:  []string{"17"},
		Extra:  map[string]interface{}{"matrix": []interface{}{"el9i:17:A:f:1:P:1.0.0"}},
		RpmPg:  []string{}, // make Available() inconclusive; matrix should decide
		RpmPkg: "lead_pkg_$v",
	}
	child := &Extension{
		Name:    "child_ext",
		Pkg:     "lead_pkg",
		Lead:    false,
		LeadExt: "lead_ext",
		PgVer:   []string{"17"},
		RpmPkg:  "lead_pkg_$v",
	}

	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{lead, child},
		ExtNameMap: map[string]*Extension{"lead_ext": lead, "child_ext": child},
		ExtPkgMap:  map[string]*Extension{"lead_pkg": lead},
		AliasMap:   map[string]string{},
	}

	summary := child.ToSummary(17)
	if summary == nil {
		t.Fatal("expected non-nil summary")
	}
	if summary.Status != "available" {
		t.Fatalf("expected status=available (matrix), got %q", summary.Status)
	}
}

func TestToSummaryStatusNotAvail_WhenMatrixSaysMissing(t *testing.T) {
	// Save and restore
	origCatalog := Catalog
	origPostgres := Postgres
	origOSCode := config.OSCode
	origOSArch := config.OSArch
	origOSType := config.OSType
	defer func() {
		Catalog = origCatalog
		Postgres = origPostgres
		config.OSCode = origOSCode
		config.OSArch = origOSArch
		config.OSType = origOSType
	}()

	config.OSCode = "el9"
	config.OSArch = "amd64"
	config.OSType = config.DistroEL
	Postgres = nil

	ext := &Extension{
		Name:   "test_ext",
		Pkg:    "test_pkg",
		Lead:   true,
		PgVer:  []string{"17"},
		Extra:  map[string]interface{}{"matrix": []interface{}{"el9i:17:M:f"}},
		RpmPg:  []string{"17"}, // Available() would be true, but matrix should override
		RpmPkg: "test_pkg_$v",
	}

	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{ext},
		ExtNameMap: map[string]*Extension{"test_ext": ext},
		ExtPkgMap:  map[string]*Extension{"test_pkg": ext},
		AliasMap:   map[string]string{},
	}

	summary := ext.ToSummary(17)
	if summary == nil {
		t.Fatal("expected non-nil summary")
	}
	if summary.Status != "not_avail" {
		t.Fatalf("expected status=not_avail (matrix missing), got %q", summary.Status)
	}
}

func TestToSummaryStatusFallbackTheoreticalOnUnsupportedOS(t *testing.T) {
	// Save and restore
	origCatalog := Catalog
	origPostgres := Postgres
	origOSCode := config.OSCode
	origOSArch := config.OSArch
	origOSType := config.OSType
	defer func() {
		Catalog = origCatalog
		Postgres = origPostgres
		config.OSCode = origOSCode
		config.OSArch = origOSArch
		config.OSType = origOSType
	}()

	// Simulate an OS code not covered by the matrix (e.g. macOS).
	config.OSCode = "a23"
	config.OSArch = "amd64"
	config.OSType = config.DistroMAC
	Postgres = nil

	ext := &Extension{
		Name:  "test_ext",
		Pkg:   "test_pkg",
		Lead:  true,
		PgVer: []string{"17"},
		Extra: map[string]interface{}{"matrix": []interface{}{"el9i:17:M:f"}}, // should be ignored on unsupported OS
	}

	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{ext},
		ExtNameMap: map[string]*Extension{"test_ext": ext},
		ExtPkgMap:  map[string]*Extension{"test_pkg": ext},
		AliasMap:   map[string]string{},
	}

	summary := ext.ToSummary(17)
	if summary == nil {
		t.Fatal("expected non-nil summary")
	}
	if summary.Status != "available" {
		t.Fatalf("expected status=available (theoretical pg_ver), got %q", summary.Status)
	}

	summary2 := ext.ToSummary(18)
	if summary2 == nil {
		t.Fatal("expected non-nil summary")
	}
	if summary2.Status != "not_avail" {
		t.Fatalf("expected status=not_avail for unsupported PG version, got %q", summary2.Status)
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
	if result.Code != 100101 { // CodeExtensionInvalidArgs
		t.Errorf("expected CodeExtensionInvalidArgs (100101), got %d", result.Code)
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
	if result.Success {
		t.Error("expected success=false for partial failure")
	}
	if result.Code != output.CodeExtensionInstallFailed {
		t.Errorf("expected CodeExtensionInstallFailed, got %d", result.Code)
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
	if result.Code != 100101 { // CodeExtensionInvalidArgs
		t.Errorf("expected CodeExtensionInvalidArgs (100101), got %d", result.Code)
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
	if !result.Success {
		t.Error("expected success=true for empty names (no-op)")
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
		Name:    "postgis",
		Pkg:     "postgis",
		Version: "3.5.0",
		EnDesc:  "PostGIS geometry extension",
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

// ============================================================================
// Code Review #3 - Additional tests for improved coverage
// ============================================================================

func TestAddExtensionsNilCatalog(t *testing.T) {
	// Save and restore
	origCatalog := Catalog
	origOSType := config.OSType
	defer func() {
		Catalog = origCatalog
		config.OSType = origOSType
	}()

	// Force Linux OS type to test Catalog nil check path
	config.OSType = config.DistroEL
	Catalog = nil

	result := AddExtensions(17, []string{"postgis"}, true)
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

func TestRmExtensionsNilCatalog(t *testing.T) {
	// Save and restore
	origCatalog := Catalog
	origOSType := config.OSType
	defer func() {
		Catalog = origCatalog
		config.OSType = origOSType
	}()

	// Force Linux OS type to test Catalog nil check path
	config.OSType = config.DistroEL
	Catalog = nil

	result := RmExtensions(17, []string{"postgis"}, true)
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

func TestUpgradeExtensionsNilCatalog(t *testing.T) {
	// Save and restore
	origCatalog := Catalog
	origOSType := config.OSType
	defer func() {
		Catalog = origCatalog
		config.OSType = origOSType
	}()

	// Force Linux OS type to test Catalog nil check path
	config.OSType = config.DistroEL
	Catalog = nil

	result := UpgradeExtensions(17, []string{"postgis"}, true)
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

func TestImportExtensionsResultNoNames(t *testing.T) {
	result := ImportExtensionsResult(17, []string{}, "/tmp/test")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Error("expected success=false when no names provided")
	}
	if result.Code != 100101 { // CodeExtensionInvalidArgs
		t.Errorf("expected CodeExtensionInvalidArgs (100101), got %d", result.Code)
	}
}

func TestImportExtensionsResultNilCatalog(t *testing.T) {
	// Save and restore
	origCatalog := Catalog
	defer func() { Catalog = origCatalog }()

	Catalog = nil

	result := ImportExtensionsResult(17, []string{"postgis"}, "/tmp/test")
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

func TestLinkPostgresResultNoArgs(t *testing.T) {
	result := LinkPostgresResult()
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Error("expected success=false when no arguments provided")
	}
	if result.Code != output.CodeExtensionInvalidArgs {
		t.Errorf("expected CodeExtensionInvalidArgs (%d), got %d", output.CodeExtensionInvalidArgs, result.Code)
	}
}

func TestLinkPostgresResultTooManyArgs(t *testing.T) {
	result := LinkPostgresResult("17", "18")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Error("expected success=false when too many arguments")
	}
	if result.Code != output.CodeExtensionInvalidArgs {
		t.Errorf("expected CodeExtensionInvalidArgs (%d), got %d", output.CodeExtensionInvalidArgs, result.Code)
	}
}

func TestLinkPostgresResultInvalidVersion(t *testing.T) {
	// Version 5 is below minimum (10)
	result := LinkPostgresResult("5")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Error("expected success=false for invalid version")
	}
	if result.Code != output.CodeExtensionInvalidArgs {
		t.Errorf("expected CodeExtensionInvalidArgs (%d), got %d", output.CodeExtensionInvalidArgs, result.Code)
	}
}

func TestLinkResultDataUnlinkSerialization(t *testing.T) {
	data := &LinkResultData{
		Action:      "unlink",
		SymlinkPath: "/usr/pgsql",
		ProfilePath: "/etc/profile.d/pgsql.sh",
	}

	// Test JSON serialization
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}
	jsonStr := string(jsonBytes)
	if !strings.Contains(jsonStr, `"action":"unlink"`) {
		t.Errorf("JSON missing action field: %s", jsonStr)
	}
	if strings.Contains(jsonStr, `"pg_home"`) {
		t.Errorf("JSON should omit empty pg_home: %s", jsonStr)
	}

	// Test YAML serialization
	yamlBytes, err := yaml.Marshal(data)
	if err != nil {
		t.Fatalf("YAML marshal failed: %v", err)
	}
	yamlStr := string(yamlBytes)
	if !strings.Contains(yamlStr, "action: unlink") {
		t.Errorf("YAML missing action field: %s", yamlStr)
	}
}

/********************
 * Code Review Fix: Additional Tests for Coverage
 * Added during Epic 3 Code Review to address coverage gaps
 ********************/

// ReloadCatalogResult tests - was 0% coverage
func TestReloadCatalogResultDataSerialization(t *testing.T) {
	data := &ReloadResultData{
		SourceURL:      "https://pigsty.io/ext/data/extension.csv",
		ExtensionCount: 450,
		CatalogPath:    "/home/user/.pig/extension.csv",
		DownloadedAt:   "2026-02-02T10:00:00Z",
		DurationMs:     1500,
	}

	// Test JSON serialization
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}
	jsonStr := string(jsonBytes)
	if !strings.Contains(jsonStr, `"source_url"`) {
		t.Errorf("JSON missing source_url field: %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"extension_count":450`) {
		t.Errorf("JSON missing extension_count field: %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"duration_ms":1500`) {
		t.Errorf("JSON missing duration_ms field: %s", jsonStr)
	}

	// Test YAML serialization
	yamlBytes, err := yaml.Marshal(data)
	if err != nil {
		t.Fatalf("YAML marshal failed: %v", err)
	}
	yamlStr := string(yamlBytes)
	if !strings.Contains(yamlStr, "source_url:") {
		t.Errorf("YAML missing source_url field: %s", yamlStr)
	}
	if !strings.Contains(yamlStr, "extension_count: 450") {
		t.Errorf("YAML missing extension_count field: %s", yamlStr)
	}
}

// RmExtensions additional tests - was 41.7%, need more coverage
func TestRmExtensionsWithCatalog(t *testing.T) {
	// Save and restore original state
	origCatalog := Catalog
	origOSType := config.OSType
	defer func() {
		Catalog = origCatalog
		config.OSType = origOSType
	}()

	// Setup mock catalog
	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{
			{Name: "postgis", Pkg: "postgis", RpmPkg: "postgis36_$v", DebPkg: "postgresql-$v-postgis-3", RpmPg: []string{"17", "16"}, DebPg: []string{"17", "16"}},
		},
		ExtNameMap: map[string]*Extension{},
		ExtPkgMap:  map[string]*Extension{},
		AliasMap:   map[string]string{},
	}
	Catalog.ExtNameMap["postgis"] = Catalog.Extensions[0]
	Catalog.ExtPkgMap["postgis"] = Catalog.Extensions[0]

	// Test with DEB OS
	config.OSType = config.DistroDEB
	result := RmExtensions(17, []string{"postgis"}, true)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// Will fail due to no sudo, but should have correct data structure
	data, ok := result.Data.(*ExtensionRmData)
	if !ok {
		t.Fatal("expected ExtensionRmData type")
	}
	if data.PgVersion != 17 {
		t.Errorf("expected pg_version=17, got %d", data.PgVersion)
	}
	if len(data.Requested) != 1 || data.Requested[0] != "postgis" {
		t.Errorf("expected requested=[postgis], got %v", data.Requested)
	}
	if !data.AutoConfirm {
		t.Error("expected auto_confirm=true")
	}
}

func TestRmExtensionsAliasLookup(t *testing.T) {
	// Save and restore original state
	origCatalog := Catalog
	origOSType := config.OSType
	defer func() {
		Catalog = origCatalog
		config.OSType = origOSType
	}()

	// Setup mock catalog with alias
	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{},
		ExtNameMap: map[string]*Extension{},
		ExtPkgMap:  map[string]*Extension{},
		AliasMap:   map[string]string{"pg17": "postgresql-17"},
	}

	config.OSType = config.DistroDEB
	installFakePackageManager(t, "apt-get")
	result := RmExtensions(17, []string{"pg17"}, true)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.Success {
		t.Fatalf("expected success=true with fake apt-get, got code=%d message=%q", result.Code, result.Message)
	}
	data, ok := result.Data.(*ExtensionRmData)
	if !ok {
		t.Fatal("expected ExtensionRmData type")
	}
	expectedPackages := []string{
		"postgresql-17",
		"postgresql-client-17",
		"postgresql-plpython3-17",
		"postgresql-plperl-17",
		"postgresql-pltcl-17",
	}
	if len(data.Packages) != len(expectedPackages) {
		t.Fatalf("expected %d resolved packages, got %d (%v)", len(expectedPackages), len(data.Packages), data.Packages)
	}
	for i, expected := range expectedPackages {
		if data.Packages[i] != expected {
			t.Fatalf("unexpected package at index %d: want %q got %q (all: %v)", i, expected, data.Packages[i], data.Packages)
		}
	}
	if len(data.Removed) != len(expectedPackages) {
		t.Fatalf("expected %d removed packages, got %d (%v)", len(expectedPackages), len(data.Removed), data.Removed)
	}
	for i, expected := range expectedPackages {
		if data.Removed[i] != expected {
			t.Fatalf("unexpected removed package at index %d: want %q got %q (all: %v)", i, expected, data.Removed[i], data.Removed)
		}
	}
}

// UpgradeExtensions additional tests - was 41.7%, need more coverage
func TestUpgradeExtensionsWithCatalog(t *testing.T) {
	// Save and restore original state
	origCatalog := Catalog
	origOSType := config.OSType
	defer func() {
		Catalog = origCatalog
		config.OSType = origOSType
	}()

	// Setup mock catalog
	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{
			{Name: "pgvector", Pkg: "pgvector", RpmPkg: "pgvector_$v", DebPkg: "postgresql-$v-pgvector", RpmPg: []string{"17", "16"}, DebPg: []string{"17", "16"}},
		},
		ExtNameMap: map[string]*Extension{},
		ExtPkgMap:  map[string]*Extension{},
		AliasMap:   map[string]string{},
	}
	Catalog.ExtNameMap["pgvector"] = Catalog.Extensions[0]
	Catalog.ExtPkgMap["pgvector"] = Catalog.Extensions[0]

	// Test with EL OS
	config.OSType = config.DistroEL
	config.OSVersion = "9"
	installFakePackageManager(t, "dnf")
	result := UpgradeExtensions(17, []string{"pgvector"}, true)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.Success {
		t.Fatalf("expected success=true with fake dnf, got code=%d message=%q", result.Code, result.Message)
	}
	data, ok := result.Data.(*ExtensionUpdateData)
	if !ok {
		t.Fatal("expected ExtensionUpdateData type")
	}
	if data.PgVersion != 17 {
		t.Errorf("expected pg_version=17, got %d", data.PgVersion)
	}
	if len(data.Packages) != 1 || data.Packages[0] != "pgvector_17" {
		t.Fatalf("expected resolved packages [pgvector_17], got %v", data.Packages)
	}
	if len(data.Updated) != 1 || data.Updated[0] != "pgvector_17" {
		t.Fatalf("expected updated packages [pgvector_17], got %v", data.Updated)
	}
}

func TestUpgradeExtensionsAliasLookup(t *testing.T) {
	// Save and restore original state
	origCatalog := Catalog
	origOSType := config.OSType
	defer func() {
		Catalog = origCatalog
		config.OSType = origOSType
	}()

	// Setup mock catalog with alias
	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{},
		ExtNameMap: map[string]*Extension{},
		ExtPkgMap:  map[string]*Extension{},
		AliasMap:   map[string]string{"pg-core": "postgresql-17"},
	}

	config.OSType = config.DistroEL
	config.OSVersion = "8"
	result := UpgradeExtensions(17, []string{"pg-core"}, false)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	data, ok := result.Data.(*ExtensionUpdateData)
	if !ok {
		t.Fatal("expected ExtensionUpdateData type")
	}
	if data.AutoConfirm {
		t.Error("expected auto_confirm=false")
	}
}

// ImportExtensionsResult additional tests - was 17.2%, need more coverage
func TestImportExtensionsResultNoCatalog(t *testing.T) {
	// Save and restore original state
	origCatalog := Catalog
	defer func() {
		Catalog = origCatalog
	}()

	Catalog = nil
	result := ImportExtensionsResult(17, []string{"postgis"}, "/tmp/test")
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

func TestImportExtensionsResultExtensionNotFound(t *testing.T) {
	// Save and restore original state
	origCatalog := Catalog
	origOSType := config.OSType
	origOSCode := config.OSCode
	origOSArch := config.OSArch
	defer func() {
		Catalog = origCatalog
		config.OSType = origOSType
		config.OSCode = origOSCode
		config.OSArch = origOSArch
	}()

	// Setup empty catalog
	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{},
		ExtNameMap: map[string]*Extension{},
		ExtPkgMap:  map[string]*Extension{},
		AliasMap:   map[string]string{},
	}
	installFakePackageManager(t, "apt-get")
	config.OSType = config.DistroDEB
	config.OSCode = "u22"
	config.OSArch = "amd64"

	result := ImportExtensionsResult(17, []string{"nonexistent_ext"}, t.TempDir())
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Error("expected success=false when extension not found")
	}
	if result.Code != output.CodeExtensionNoPackage {
		t.Fatalf("expected CodeExtensionNoPackage, got code=%d message=%q", result.Code, result.Message)
	}
	data, ok := result.Data.(*ImportResultData)
	if !ok {
		t.Fatalf("expected *ImportResultData, got %T", result.Data)
	}
	if len(data.Requested) != 1 || data.Requested[0] != "nonexistent_ext" {
		t.Fatalf("unexpected requested list: %v", data.Requested)
	}
	if len(data.Failed) != 1 || data.Failed[0] != "nonexistent_ext" {
		t.Fatalf("unexpected failed list: %v", data.Failed)
	}
	if len(data.Packages) != 0 {
		t.Fatalf("expected no resolved packages, got %v", data.Packages)
	}
}

// ScanExtensionsResult additional tests - was 15.4%, need more coverage
func TestScanExtensionsResultNoCatalog(t *testing.T) {
	// Save and restore original state
	origCatalog := Catalog
	defer func() {
		Catalog = origCatalog
	}()

	Catalog = nil
	result := ScanExtensionsResult()
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

func TestScanExtensionsResultNoPostgres(t *testing.T) {
	// Save and restore original state
	origCatalog := Catalog
	origPostgres := Postgres
	defer func() {
		Catalog = origCatalog
		Postgres = origPostgres
	}()

	// Setup catalog but no Postgres
	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{},
		ExtNameMap: map[string]*Extension{},
		ExtPkgMap:  map[string]*Extension{},
	}
	Postgres = nil

	result := ScanExtensionsResult()
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Error("expected success=false when postgres is nil")
	}
	if result.Code != 100601 { // CodeExtensionNoPG
		t.Errorf("expected CodeExtensionNoPG, got %d", result.Code)
	}
}

// GetExtensionAvailability additional tests - was 28.6%, need more coverage
func TestGetExtensionAvailabilityGlobalMode(t *testing.T) {
	// Save and restore original state
	origCatalog := Catalog
	origOSCode := config.OSCode
	origOSArch := config.OSArch
	defer func() {
		Catalog = origCatalog
		config.OSCode = origOSCode
		config.OSArch = origOSArch
	}()

	config.OSCode = "el9"
	config.OSArch = "amd64"

	// Setup catalog with a lead extension
	ext := &Extension{
		Name:    "postgis",
		Pkg:     "postgis",
		Lead:    true,
		Contrib: false,
		Extra: map[string]interface{}{
			"matrix": []interface{}{"el9i:17:A:f:1:P:3.5.0"},
		},
	}
	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{ext},
		ExtNameMap: map[string]*Extension{"postgis": ext},
		ExtPkgMap:  map[string]*Extension{"postgis": ext},
	}

	// Test global availability (no arguments)
	result := GetExtensionAvailability([]string{})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.Success {
		t.Errorf("expected success=true, got message: %s", result.Message)
	}
	data, ok := result.Data.(*ExtensionAvailData)
	if !ok {
		t.Fatal("expected ExtensionAvailData type")
	}
	if data.PackageCount != 1 || len(data.Packages) != 1 {
		t.Fatalf("expected exactly 1 available package, got count=%d packages=%v", data.PackageCount, data.Packages)
	}
	if data.Packages[0].Pkg != "postgis" {
		t.Fatalf("expected package name postgis, got %q", data.Packages[0].Pkg)
	}
	if ver := data.Packages[0].Versions["17"]; ver != "3.5.0" {
		t.Fatalf("expected pg17 version=3.5.0, got %q", ver)
	}
}

func TestGetExtensionAvailabilityMultipleExtensions(t *testing.T) {
	// Save and restore original state
	origCatalog := Catalog
	defer func() {
		Catalog = origCatalog
	}()

	// Setup catalog with multiple extensions
	ext1 := &Extension{Name: "postgis", Pkg: "postgis", Lead: true}
	ext2 := &Extension{Name: "pgvector", Pkg: "pgvector", Lead: true}
	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{ext1, ext2},
		ExtNameMap: map[string]*Extension{"postgis": ext1, "pgvector": ext2},
		ExtPkgMap:  map[string]*Extension{"postgis": ext1, "pgvector": ext2},
	}

	// Test with multiple extensions
	result := GetExtensionAvailability([]string{"postgis", "pgvector"})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.Success {
		t.Errorf("expected success=true for existing extensions")
	}
	dataSlice, ok := result.Data.([]*ExtensionAvailData)
	if !ok {
		t.Fatalf("expected []*ExtensionAvailData, got %T", result.Data)
	}
	if len(dataSlice) != 2 {
		t.Fatalf("expected 2 results, got %d", len(dataSlice))
	}
	if dataSlice[0] == nil || dataSlice[1] == nil {
		t.Fatalf("expected non-nil availability entries, got %v", dataSlice)
	}
	if dataSlice[0].Extension != "postgis" || dataSlice[1].Extension != "pgvector" {
		t.Fatalf("unexpected extension order/content: [%s, %s]", dataSlice[0].Extension, dataSlice[1].Extension)
	}
}

func TestGetExtensionAvailabilityPartialNotFound(t *testing.T) {
	// Save and restore original state
	origCatalog := Catalog
	defer func() {
		Catalog = origCatalog
	}()

	// Setup catalog with one extension
	ext := &Extension{Name: "postgis", Pkg: "postgis", Lead: true}
	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{ext},
		ExtNameMap: map[string]*Extension{"postgis": ext},
		ExtPkgMap:  map[string]*Extension{"postgis": ext},
	}

	// Test with mixed found/not-found
	result := GetExtensionAvailability([]string{"postgis", "nonexistent"})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.Success {
		t.Error("expected success=true for partial match")
	}
	if result.Detail == "" {
		t.Error("expected detail to mention not found extensions")
	}
}

// LinkPostgresResult additional tests - was 33.3%, need more coverage
func TestLinkPostgresResultUnlinkKeywords(t *testing.T) {
	keywords := []string{"null", "none", "nil", "nop", "no"}
	for _, keyword := range keywords {
		result := LinkPostgresResult(keyword)
		if result == nil {
			t.Fatalf("expected non-nil result for keyword: %s", keyword)
		}
		// Will fail on non-Linux but should recognize the keyword
		data, ok := result.Data.(*LinkResultData)
		if ok && data.Action != "unlink" {
			t.Errorf("expected action=unlink for keyword %s, got %s", keyword, data.Action)
		}
	}
}

func TestLinkPostgresResultPgPrefix(t *testing.T) {
	origOSType := config.OSType
	defer func() {
		config.OSType = origOSType
	}()
	config.OSType = "unsupported-test-os"

	result := LinkPostgresResult("pg17")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Fatal("expected failure on unsupported OS type")
	}
	if result.Code != output.CodeExtensionUnsupportedOS {
		t.Fatalf("expected CodeExtensionUnsupportedOS, got code=%d message=%q", result.Code, result.Message)
	}
	if !strings.Contains(result.Message, "unsupported OS distribution") {
		t.Fatalf("expected unsupported OS message, got %q", result.Message)
	}
}

func TestLinkPostgresResultVersionRange(t *testing.T) {
	// Test version above max (30)
	result := LinkPostgresResult("35")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Error("expected success=false for version above max")
	}
}

func TestPackageManagerCmd(t *testing.T) {
	// Save and restore original state
	origOSType := config.OSType
	origOSVersion := config.OSVersion
	defer func() {
		config.OSType = origOSType
		config.OSVersion = origOSVersion
	}()

	tests := []struct {
		osType    string
		osVersion string
		expected  string
	}{
		{config.DistroEL, "7", "yum"},
		{config.DistroEL, "8", "dnf"},
		{config.DistroEL, "9", "dnf"},
		{config.DistroEL, "10", "dnf"},
		{config.DistroDEB, "12", "apt-get"},
		{config.DistroDEB, "22.04", "apt-get"},
		{config.DistroMAC, "", ""},
		{"unknown", "", ""},
	}

	for _, tt := range tests {
		config.OSType = tt.osType
		config.OSVersion = tt.osVersion
		result := PackageManagerCmd()
		if result != tt.expected {
			t.Errorf("PackageManagerCmd() for OSType=%s, OSVersion=%s: expected %s, got %s",
				tt.osType, tt.osVersion, tt.expected, result)
		}
	}
}

// Test note: ReloadCatalogResult has 0% test coverage because it makes actual network calls
// to download the extension catalog. A proper unit test would require HTTP mocking.
// The DTO serialization is tested in TestReloadCatalogResultDataSerialization above.

/********************
 * Text() Method Tests
 * Verify output.Texter interface implementation and nil receiver safety
 ********************/

func TestExtensionListDataTextNil(t *testing.T) {
	var d *ExtensionListData
	if d.Text() != "" {
		t.Error("nil ExtensionListData.Text() should return empty string")
	}
}

func TestExtensionListDataTextVersionMode(t *testing.T) {
	d := &ExtensionListData{
		PgVersion: 17,
		Extensions: []*ExtensionSummary{
			{Name: "postgis", Version: "3.5.0", Category: "GIS", License: "GPLv2", Repo: "PGDG", PackageName: "postgis34_17", Description: "PostGIS geometry and geography spatial types and functions", PgVer: []string{"17", "16", "15"}},
			{Name: "pg_stat_statements", Version: "1.10", Category: "STAT", License: "PostgreSQL", Repo: "CONTRIB", PackageName: "", Description: "Track planning and execution statistics"},
		},
	}
	text := d.Text()
	if text == "" {
		t.Error("ExtensionListData.Text() returned empty string for valid data")
	}
	if !strings.Contains(text, "postgis") {
		t.Error("expected 'postgis' in output")
	}
	if !strings.Contains(text, "pg_stat_statements") {
		t.Error("expected 'pg_stat_statements' in output")
	}
	if !strings.Contains(text, "(2 Rows)") {
		t.Error("expected row count in output")
	}
}

func TestExtensionListDataTextCommonMode(t *testing.T) {
	d := &ExtensionListData{
		PgVersion: 0, // Common mode
		Extensions: []*ExtensionSummary{
			{Name: "postgis", Version: "3.5.0", Category: "GIS", License: "GPLv2", PgVer: []string{"17", "16"}},
		},
	}
	text := d.Text()
	if text == "" {
		t.Error("ExtensionListData.Text() returned empty string for valid data")
	}
	if !strings.Contains(text, "postgis") {
		t.Error("expected 'postgis' in output")
	}
	if !strings.Contains(text, "(1 Rows)") {
		t.Error("expected row count in output")
	}
}

func TestExtensionListDataTextEmpty(t *testing.T) {
	d := &ExtensionListData{
		PgVersion:  17,
		Extensions: []*ExtensionSummary{},
	}
	text := d.Text()
	if !strings.Contains(text, "(0 Rows)") {
		t.Error("expected (0 Rows) in output for empty extensions")
	}
}

func TestExtensionInfoDataTextNil(t *testing.T) {
	var d *ExtensionInfoData
	if d.Text() != "" {
		t.Error("nil ExtensionInfoData.Text() should return empty string")
	}
}

func TestExtensionInfoDataTextFallback(t *testing.T) {
	// No catalog loaded - uses fallback rendering
	origCatalog := Catalog
	Catalog = nil
	defer func() { Catalog = origCatalog }()

	d := &ExtensionInfoData{
		Name:        "postgis",
		Pkg:         "postgis",
		Category:    "GIS",
		License:     "GPLv2",
		Language:    "C",
		Version:     "3.5.0",
		PgVer:       []string{"17", "16", "15"},
		Description: "PostGIS geometry and geography spatial types and functions",
		URL:         "https://postgis.net",
		Operations: &ExtensionOperations{
			Install: "pig ext add postgis",
			Config:  "shared_preload_libraries = 'postgis'",
			Create:  "CREATE EXTENSION postgis;",
		},
	}
	text := d.Text()
	if text == "" {
		t.Error("ExtensionInfoData.Text() returned empty string for valid data")
	}
	if !strings.Contains(text, "postgis") {
		t.Error("expected 'postgis' in output")
	}
	if !strings.Contains(text, "GIS") {
		t.Error("expected 'GIS' in output")
	}
	if !strings.Contains(text, "https://postgis.net") {
		t.Error("expected URL in output")
	}
	if !strings.Contains(text, "pig ext add postgis") {
		t.Error("expected install command in output")
	}
}

func TestExtensionStatusDataTextNil(t *testing.T) {
	var d *ExtensionStatusData
	if d.Text() != "" {
		t.Error("nil ExtensionStatusData.Text() should return empty string")
	}
}

func TestExtensionStatusDataTextWithData(t *testing.T) {
	d := &ExtensionStatusData{
		PgInfo: &PostgresInfo{
			MajorVersion: 17,
			Version:      "17.2",
			BinDir:       "/usr/pgsql-17/bin",
			ExtensionDir: "/usr/pgsql-17/share/extension",
		},
		Summary: &ExtensionSummaryInfo{
			TotalInstalled: 75,
			ByRepo:         map[string]int{"PIGSTY": 10, "PGDG": 15, "CONTRIB": 50},
		},
		Extensions: []*ExtensionSummary{
			{Name: "postgis", Version: "3.5.0", Category: "GIS", License: "GPLv2", Repo: "PIGSTY", PackageName: "postgis34_17", Description: "PostGIS"},
		},
	}
	text := d.Text()
	if !strings.Contains(text, "PostgreSQL 17") {
		t.Error("expected PostgreSQL version in output")
	}
	if !strings.Contains(text, "/usr/pgsql-17/bin") {
		t.Error("expected bin dir in output")
	}
	if !strings.Contains(text, "Extension Stat") {
		t.Error("expected extension stat in output")
	}
	if !strings.Contains(text, "postgis") {
		t.Error("expected postgis in output")
	}
	if !strings.Contains(text, "(1 Rows)") {
		t.Error("expected row count in output")
	}
}

func TestExtensionStatusDataTextNotFound(t *testing.T) {
	d := &ExtensionStatusData{
		NotFound: []string{"foo", "bar"},
	}
	text := d.Text()
	if !strings.Contains(text, "Not found in catalog: foo, bar") {
		t.Error("expected not found list in output")
	}
}

func TestScanResultDataTextNil(t *testing.T) {
	var d *ScanResultData
	if d.Text() != "" {
		t.Error("nil ScanResultData.Text() should return empty string")
	}
}

func TestScanResultDataTextWithData(t *testing.T) {
	d := &ScanResultData{
		PgInfo: &PostgresInfo{
			MajorVersion: 17,
			Version:      "17.2",
			BinDir:       "/usr/pgsql-17/bin",
			ExtensionDir: "/usr/pgsql-17/share/extension",
		},
		Extensions: []*ScanExtEntry{
			{Name: "plpgsql", Version: "1.0", Description: "PL/pgSQL procedural language", ControlMeta: map[string]string{"module_pathname": "$libdir/plpgsql"}, Libraries: []string{"plpgsql"}},
		},
		EncodingLibs:  []string{"utf8_and_euc_cn"},
		BuiltInLibs:   []string{"libpq"},
		UnmatchedLibs: []string{"unknown_lib"},
	}
	text := d.Text()
	if !strings.Contains(text, "PostgreSQL 17") {
		t.Error("expected PostgreSQL version in output")
	}
	if !strings.Contains(text, "plpgsql") {
		t.Error("expected plpgsql in output")
	}
	if !strings.Contains(text, "Encoding Libs") {
		t.Error("expected encoding libs in output")
	}
	if !strings.Contains(text, "Built-in Libs") {
		t.Error("expected built-in libs in output")
	}
	if !strings.Contains(text, "Unmatched Shared Libraries") {
		t.Error("expected unmatched libs in output")
	}
}

func TestExtensionAvailDataTextNil(t *testing.T) {
	var d *ExtensionAvailData
	if d.Text() != "" {
		t.Error("nil ExtensionAvailData.Text() should return empty string")
	}
}

func TestExtensionAvailDataTextSingleFallback(t *testing.T) {
	// No catalog loaded - uses fallback rendering
	origCatalog := Catalog
	Catalog = nil
	defer func() { Catalog = origCatalog }()

	d := &ExtensionAvailData{
		Extension: "postgis",
		LatestVer: "3.5.0",
		Summary:   "84/84 avail",
		Matrix: []*MatrixEntry{
			{OS: "el9", Arch: "amd64", PG: 17, State: "A", Version: "3.5.0", Org: "P"},
		},
	}
	text := d.Text()
	if !strings.Contains(text, "postgis") {
		t.Error("expected extension name in output")
	}
	if !strings.Contains(text, "3.5.0") {
		t.Error("expected version in output")
	}
}

func TestExtensionAvailDataTextGlobalFallback(t *testing.T) {
	// No catalog loaded - uses fallback rendering
	origCatalog := Catalog
	Catalog = nil
	defer func() { Catalog = origCatalog }()

	d := &ExtensionAvailData{
		OSCode:       "el9",
		Arch:         "amd64",
		PackageCount: 200,
		Packages: []*PackageAvailability{
			{Pkg: "postgis", Versions: map[string]string{"17": "3.5.0", "16": "3.4.0"}},
		},
	}
	text := d.Text()
	if !strings.Contains(text, "el9") {
		t.Error("expected OS code in output")
	}
	if !strings.Contains(text, "200") {
		t.Error("expected package count in output")
	}
}

func TestExtensionAvailDataTextGlobalUnsupportedOSShowsFallback(t *testing.T) {
	origCatalog := Catalog
	defer func() { Catalog = origCatalog }()

	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{{Name: "postgis", Pkg: "postgis", Lead: true}},
		ExtNameMap: map[string]*Extension{},
		ExtPkgMap:  map[string]*Extension{},
	}

	d := &ExtensionAvailData{
		OSCode:       "a25",
		Arch:         "arm64",
		PackageCount: 0,
		Packages:     []*PackageAvailability{},
	}
	text := d.Text()
	if !strings.Contains(text, "Current OS 'a25' is not a supported Linux distribution") {
		t.Fatalf("expected unsupported OS note, got: %s", text)
	}
	if !strings.Contains(text, "Showing matrix for el9.x86_64 as example") {
		t.Fatalf("expected fallback to el9.x86_64 matrix for unsupported OS, got: %s", text)
	}
}

func TestExtensionAvailDataTextGlobalNoANSIInPlainTextMode(t *testing.T) {
	origCatalog := Catalog
	origFormat := config.OutputFormat
	defer func() {
		Catalog = origCatalog
		config.OutputFormat = origFormat
	}()

	config.OutputFormat = config.OUTPUT_TEXT
	ext := &Extension{
		Name:  "postgis",
		Pkg:   "postgis",
		Lead:  true,
		Extra: map[string]interface{}{"matrix": []interface{}{"el9i:17:A:f:1:P:3.5.0"}},
	}
	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{ext},
		ExtNameMap: map[string]*Extension{"postgis": ext},
		ExtPkgMap:  map[string]*Extension{"postgis": ext},
	}

	d := &ExtensionAvailData{
		OSCode:       "el9",
		Arch:         "amd64",
		PackageCount: 1,
		Packages: []*PackageAvailability{
			{Pkg: "postgis", Versions: map[string]string{"17": "3.5.0"}},
		},
	}
	text := d.Text()
	if strings.Contains(text, "\x1b[") {
		t.Fatalf("plain text output should not contain ANSI escape codes, got: %q", text)
	}
}

func TestExtensionAddDataTextNil(t *testing.T) {
	var d *ExtensionAddData
	if d.Text() != "" {
		t.Error("nil ExtensionAddData.Text() should return empty string")
	}
}

func TestExtensionAddDataTextWithData(t *testing.T) {
	d := &ExtensionAddData{
		PgVersion:  17,
		DurationMs: 1234,
		Installed: []*InstalledExtItem{
			{Name: "postgis", Package: "postgis34_17"},
		},
		Failed: []*FailedExtItem{
			{Name: "foo", Error: "not found"},
		},
	}
	text := d.Text()
	if !strings.Contains(text, "Installed 1 package(s)") {
		t.Error("expected installed count in output")
	}
	if !strings.Contains(text, "postgis") {
		t.Error("expected package name in output")
	}
	if !strings.Contains(text, "Failed 1 package(s)") {
		t.Error("expected failed count in output")
	}
	if !strings.Contains(text, "1234ms") {
		t.Error("expected duration in output")
	}
}

func TestExtensionRmDataTextNil(t *testing.T) {
	var d *ExtensionRmData
	if d.Text() != "" {
		t.Error("nil ExtensionRmData.Text() should return empty string")
	}
}

func TestExtensionRmDataTextWithData(t *testing.T) {
	d := &ExtensionRmData{
		PgVersion:  17,
		DurationMs: 567,
		Removed:    []string{"postgis", "pg_trgm"},
		Failed: []*FailedExtItem{
			{Name: "bar", Error: "permission denied"},
		},
	}
	text := d.Text()
	if !strings.Contains(text, "Removed 2 package(s)") {
		t.Error("expected removed count in output")
	}
	if !strings.Contains(text, "postgis") {
		t.Error("expected package name in output")
	}
	if !strings.Contains(text, "Failed 1 package(s)") {
		t.Error("expected failed count in output")
	}
	if !strings.Contains(text, "567ms") {
		t.Error("expected duration in output")
	}
}

func TestExtensionUpdateDataTextNil(t *testing.T) {
	var d *ExtensionUpdateData
	if d.Text() != "" {
		t.Error("nil ExtensionUpdateData.Text() should return empty string")
	}
}

func TestExtensionUpdateDataTextWithData(t *testing.T) {
	d := &ExtensionUpdateData{
		PgVersion:  17,
		DurationMs: 890,
		Updated:    []string{"postgis"},
	}
	text := d.Text()
	if !strings.Contains(text, "Updated 1 package(s)") {
		t.Error("expected updated count in output")
	}
	if !strings.Contains(text, "postgis") {
		t.Error("expected package name in output")
	}
	if !strings.Contains(text, "890ms") {
		t.Error("expected duration in output")
	}
}

func TestImportResultDataTextNil(t *testing.T) {
	var d *ImportResultData
	if d.Text() != "" {
		t.Error("nil ImportResultData.Text() should return empty string")
	}
}

func TestImportResultDataTextWithData(t *testing.T) {
	d := &ImportResultData{
		PgVersion:  17,
		RepoDir:    "/www/pigsty",
		DurationMs: 5000,
		Downloaded: []string{"postgis34_17", "pg_trgm_17"},
		Failed:     []string{"bad_pkg"},
	}
	text := d.Text()
	if !strings.Contains(text, "/www/pigsty") {
		t.Error("expected repo dir in output")
	}
	if !strings.Contains(text, "Downloaded 2 package(s)") {
		t.Error("expected downloaded count in output")
	}
	if !strings.Contains(text, "Failed 1 package(s)") {
		t.Error("expected failed count in output")
	}
	if !strings.Contains(text, "5000ms") {
		t.Error("expected duration in output")
	}
}

func TestLinkResultDataTextNil(t *testing.T) {
	var d *LinkResultData
	if d.Text() != "" {
		t.Error("nil LinkResultData.Text() should return empty string")
	}
}

func TestLinkResultDataTextLink(t *testing.T) {
	d := &LinkResultData{
		Action:       "link",
		PgHome:       "/usr/pgsql-17",
		SymlinkPath:  "/usr/pgsql",
		ProfilePath:  "/etc/profile.d/pgsql.sh",
		ActivatedCmd: ". /etc/profile.d/pgsql.sh",
	}
	text := d.Text()
	if !strings.Contains(text, "Linked /usr/pgsql -> /usr/pgsql-17") {
		t.Error("expected link message in output")
	}
	if !strings.Contains(text, "Activate: . /etc/profile.d/pgsql.sh") {
		t.Error("expected activate command in output")
	}
}

func TestLinkResultDataTextUnlink(t *testing.T) {
	d := &LinkResultData{
		Action:      "unlink",
		SymlinkPath: "/usr/pgsql",
		ProfilePath: "/etc/profile.d/pgsql.sh",
	}
	text := d.Text()
	if !strings.Contains(text, "Unlinked PostgreSQL from /usr/pgsql") {
		t.Error("expected unlink message in output")
	}
}

func TestReloadResultDataTextNil(t *testing.T) {
	var d *ReloadResultData
	if d.Text() != "" {
		t.Error("nil ReloadResultData.Text() should return empty string")
	}
}

func TestReloadResultDataTextWithData(t *testing.T) {
	d := &ReloadResultData{
		SourceURL:      "https://pigsty.io/ext/data/extension.csv",
		ExtensionCount: 451,
		CatalogPath:    "/root/.pig/extension.csv",
		DurationMs:     350,
	}
	text := d.Text()
	if !strings.Contains(text, "pigsty.io") {
		t.Error("expected source URL in output")
	}
	if !strings.Contains(text, "451") {
		t.Error("expected extension count in output")
	}
	if !strings.Contains(text, "/root/.pig/extension.csv") {
		t.Error("expected catalog path in output")
	}
	if !strings.Contains(text, "350ms") {
		t.Error("expected duration in output")
	}
}

func TestFlagsFromSummaryNil(t *testing.T) {
	if flagsFromSummary(nil) != "" {
		t.Error("flagsFromSummary(nil) should return empty string")
	}
}

func TestFlagsFromSummaryNoCatalog(t *testing.T) {
	origCatalog := Catalog
	Catalog = nil
	defer func() { Catalog = origCatalog }()

	s := &ExtensionSummary{Name: "postgis"}
	if flagsFromSummary(s) != "" {
		t.Error("flagsFromSummary with nil catalog should return empty string")
	}
}

// TestTextMethodImplementsTexter verifies the Texter interface is satisfied at compile time.
// The output.Texter interface is: type Texter interface { Text() string }
func TestTextMethodImplementsTexter(t *testing.T) {
	type Texter interface {
		Text() string
	}

	// Each of these should compile, proving they implement Texter
	var _ Texter = (*ExtensionListData)(nil)
	var _ Texter = (*ExtensionInfoData)(nil)
	var _ Texter = (*ExtensionStatusData)(nil)
	var _ Texter = (*ScanResultData)(nil)
	var _ Texter = (*ExtensionAvailData)(nil)
	var _ Texter = (*ExtensionAddData)(nil)
	var _ Texter = (*ExtensionRmData)(nil)
	var _ Texter = (*ExtensionUpdateData)(nil)
	var _ Texter = (*ImportResultData)(nil)
	var _ Texter = (*LinkResultData)(nil)
	var _ Texter = (*ReloadResultData)(nil)
}
