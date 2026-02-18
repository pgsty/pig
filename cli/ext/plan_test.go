package ext

import (
	"strings"
	"testing"

	"pig/internal/config"
	"pig/internal/output"
)

// ============================================================================
// Test Helpers
// ============================================================================

// setupTestCatalog creates a minimal catalog for plan testing
func setupTestCatalog() func() {
	oldCatalog := Catalog
	oldPostgres := Postgres
	oldOSType := config.OSType

	config.OSType = config.DistroEL

	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{
			{
				Name:     "postgis",
				Pkg:      "postgis",
				Lead:     true,
				EnDesc:   "PostGIS geometry and geography spatial types",
				NeedLoad: false,
				RpmPkg:   "postgis35_$v*",
				DebPkg:   "postgresql-$v-postgis-3",
				RpmPg:    []string{"17", "16", "15", "14", "13"},
				DebPg:    []string{"17", "16", "15", "14", "13"},
				Requires: []string{},
			},
			{
				Name:      "timescaledb",
				Pkg:       "timescaledb",
				Lead:      true,
				EnDesc:    "Enables scalable inserts and complex queries for time-series data",
				NeedLoad:  true,
				RpmPkg:    "timescaledb-2-postgresql-$v*",
				DebPkg:    "timescaledb-2-postgresql-$v",
				RpmPg:     []string{"17", "16", "15", "14", "13"},
				DebPg:     []string{"17", "16", "15", "14", "13"},
				Requires:  []string{},
				RequireBy: []string{},
			},
			{
				Name:      "postgis_topology",
				Pkg:       "postgis",
				Lead:      false,
				EnDesc:    "PostGIS topology spatial types and functions",
				NeedLoad:  false,
				RpmPkg:    "postgis35_$v*",
				DebPkg:    "postgresql-$v-postgis-3",
				RpmPg:     []string{"17", "16", "15", "14", "13"},
				DebPg:     []string{"17", "16", "15", "14", "13"},
				Requires:  []string{"postgis"},
				RequireBy: []string{},
			},
		},
		ExtNameMap: make(map[string]*Extension),
		ExtPkgMap:  make(map[string]*Extension),
		Dependency: make(map[string][]string),
		AliasMap:   make(map[string]string),
	}

	for i, ext := range Catalog.Extensions {
		Catalog.ExtNameMap[ext.Name] = Catalog.Extensions[i]
		if ext.Pkg != "" && ext.Lead {
			Catalog.ExtPkgMap[ext.Pkg] = Catalog.Extensions[i]
		}
		for _, req := range ext.Requires {
			Catalog.Dependency[req] = append(Catalog.Dependency[req], ext.Name)
		}
	}

	return func() {
		Catalog = oldCatalog
		Postgres = oldPostgres
		config.OSType = oldOSType
	}
}

// ============================================================================
// BuildAddPlan Tests
// ============================================================================

func TestBuildAddPlan_Normal(t *testing.T) {
	cleanup := setupTestCatalog()
	defer cleanup()

	plan := BuildAddPlan(17, []string{"postgis"}, false)
	if plan == nil {
		t.Fatal("BuildAddPlan returned nil")
	}

	// Check command
	if !strings.Contains(plan.Command, "pig ext add postgis") {
		t.Errorf("Plan.Command should contain 'pig ext add postgis', got %q", plan.Command)
	}

	// Check actions
	if len(plan.Actions) < 2 {
		t.Errorf("Expected at least 2 actions (resolve + execute), got %d", len(plan.Actions))
	}
	if len(plan.Actions) > 0 && !strings.Contains(plan.Actions[0].Description, "Resolve") {
		t.Errorf("First action should be resolve, got %q", plan.Actions[0].Description)
	}

	// Check affects
	if len(plan.Affects) == 0 {
		t.Error("Affects should not be empty for normal add")
	}
	foundPkg := false
	for _, a := range plan.Affects {
		if a.Type == "package" {
			foundPkg = true
			break
		}
	}
	if !foundPkg {
		t.Error("Affects should include package resource")
	}

	// Check expected
	if plan.Expected == "" {
		t.Error("Expected should not be empty")
	}
	if !strings.Contains(plan.Expected, "postgis") {
		t.Errorf("Expected should mention postgis, got %q", plan.Expected)
	}
}

func TestBuildAddPlan_VersionPinAndYesFlag(t *testing.T) {
	// Save and restore
	oldCatalog := Catalog
	oldPostgres := Postgres
	oldOSType := config.OSType
	defer func() {
		Catalog = oldCatalog
		Postgres = oldPostgres
		config.OSType = oldOSType
	}()

	config.OSType = config.DistroEL
	Postgres = nil

	ext := &Extension{
		Name:   "test_ext",
		Pkg:    "test_ext",
		Lead:   true,
		RpmPkg: "test_ext_$v",
		PgVer:  []string{"17"},
	}
	Catalog = &ExtensionCatalog{
		Extensions: []*Extension{ext},
		ExtNameMap: map[string]*Extension{"test_ext": ext},
		ExtPkgMap:  map[string]*Extension{"test_ext": ext},
		Dependency: map[string][]string{},
		AliasMap:   map[string]string{},
	}

	plan := BuildAddPlan(17, []string{"test_ext=1.2.3"}, true)
	if plan == nil {
		t.Fatal("BuildAddPlan returned nil")
	}
	if len(plan.Actions) < 2 {
		t.Fatalf("expected at least 2 actions, got %d", len(plan.Actions))
	}
	exec := plan.Actions[1].Description
	if !strings.Contains(exec, "test_ext_17-1.2.3") {
		t.Fatalf("expected version pin in execute action, got %q", exec)
	}
	if !strings.Contains(exec, "install -y") {
		t.Fatalf("expected -y in execute action when yes=true, got %q", exec)
	}

	plan2 := BuildAddPlan(17, []string{"test_ext=1.2.3"}, false)
	if plan2 == nil || len(plan2.Actions) < 2 {
		t.Fatal("expected non-nil plan with actions")
	}
	exec2 := plan2.Actions[1].Description
	if strings.Contains(exec2, "install -y") {
		t.Fatalf("did not expect -y in execute action when yes=false, got %q", exec2)
	}
}

func TestBuildAddPlan_AlreadyInstalled(t *testing.T) {
	cleanup := setupTestCatalog()
	defer cleanup()

	// Set up Postgres with postgis already installed
	Postgres = &PostgresInstall{
		MajorVersion: 17,
		ExtensionMap: map[string]*ExtensionInstall{
			"postgis": {Extension: &Extension{Name: "postgis"}},
		},
	}

	plan := BuildAddPlan(17, []string{"postgis"}, false)
	if plan == nil {
		t.Fatal("BuildAddPlan returned nil")
	}

	// Actions should be empty
	if len(plan.Actions) != 0 {
		t.Errorf("Expected 0 actions for already installed, got %d", len(plan.Actions))
	}

	// Expected should indicate already installed
	if !strings.Contains(plan.Expected, "already installed") {
		t.Errorf("Expected should mention 'already installed', got %q", plan.Expected)
	}
}

func TestBuildAddPlan_NeedLoad(t *testing.T) {
	cleanup := setupTestCatalog()
	defer cleanup()

	plan := BuildAddPlan(17, []string{"timescaledb"}, false)
	if plan == nil {
		t.Fatal("BuildAddPlan returned nil")
	}

	// Check risks mention shared_preload_libraries
	foundLoadRisk := false
	for _, risk := range plan.Risks {
		if strings.Contains(risk, "shared_preload_libraries") {
			foundLoadRisk = true
			break
		}
	}
	if !foundLoadRisk {
		t.Error("Risks should mention shared_preload_libraries for NeedLoad extension")
	}

	// Check affects include service
	foundService := false
	for _, a := range plan.Affects {
		if a.Type == "service" {
			foundService = true
			break
		}
	}
	if !foundService {
		t.Error("Affects should include service resource for NeedLoad extension")
	}
}

func TestBuildAddPlan_MultipleExtensions(t *testing.T) {
	cleanup := setupTestCatalog()
	defer cleanup()

	plan := BuildAddPlan(17, []string{"postgis", "timescaledb"}, false)
	if plan == nil {
		t.Fatal("BuildAddPlan returned nil")
	}

	// Check command
	if !strings.Contains(plan.Command, "postgis") || !strings.Contains(plan.Command, "timescaledb") {
		t.Errorf("Plan.Command should contain both extension names, got %q", plan.Command)
	}

	// Check affects has multiple packages
	pkgCount := 0
	for _, a := range plan.Affects {
		if a.Type == "package" {
			pkgCount++
		}
	}
	if pkgCount < 2 {
		t.Errorf("Expected at least 2 package resources, got %d", pkgCount)
	}
}

func TestBuildAddPlan_ExtensionNotFound(t *testing.T) {
	cleanup := setupTestCatalog()
	defer cleanup()

	plan := BuildAddPlan(17, []string{"nonexistent_ext"}, false)
	if plan == nil {
		t.Fatal("BuildAddPlan returned nil")
	}

	// Should have risk about not found
	foundNotFoundRisk := false
	for _, risk := range plan.Risks {
		if strings.Contains(risk, "not found") {
			foundNotFoundRisk = true
			break
		}
	}
	if !foundNotFoundRisk {
		t.Error("Risks should mention 'not found' for missing extension")
	}
}

func TestBuildAddPlan_CatalogNil(t *testing.T) {
	oldCatalog := Catalog
	Catalog = nil
	defer func() { Catalog = oldCatalog }()

	plan := BuildAddPlan(17, []string{"postgis"}, false)
	if plan == nil {
		t.Fatal("BuildAddPlan returned nil even with nil Catalog")
	}

	// Should indicate error
	if !strings.Contains(plan.Expected, "catalog not initialized") {
		t.Errorf("Expected should mention catalog error, got %q", plan.Expected)
	}
}

func TestBuildAddPlan_EmptyNames(t *testing.T) {
	cleanup := setupTestCatalog()
	defer cleanup()

	plan := BuildAddPlan(17, []string{}, false)
	if plan == nil {
		t.Fatal("BuildAddPlan returned nil")
	}
	if !strings.Contains(plan.Expected, "no extensions specified") {
		t.Fatalf("expected empty-name error in Expected, got %q", plan.Expected)
	}
	if len(plan.Actions) != 0 {
		t.Fatalf("expected no actions for empty names, got %d", len(plan.Actions))
	}
}

// ============================================================================
// BuildRmPlan Tests
// ============================================================================

func TestBuildRmPlan_Normal(t *testing.T) {
	cleanup := setupTestCatalog()
	defer cleanup()

	plan := BuildRmPlan(17, []string{"postgis"}, false)
	if plan == nil {
		t.Fatal("BuildRmPlan returned nil")
	}

	// Check command
	if !strings.Contains(plan.Command, "pig ext rm postgis") {
		t.Errorf("Plan.Command should contain 'pig ext rm postgis', got %q", plan.Command)
	}

	// Check actions
	if len(plan.Actions) < 2 {
		t.Errorf("Expected at least 2 actions (resolve + execute), got %d", len(plan.Actions))
	}

	// Check affects
	if len(plan.Affects) == 0 {
		t.Error("Affects should not be empty for normal rm")
	}

	// Check expected
	if plan.Expected == "" {
		t.Error("Expected should not be empty")
	}

	// Check risks include general removal risks
	if len(plan.Risks) == 0 {
		t.Error("Risks should not be empty for removal")
	}
}

func TestBuildRmPlan_WithDependents(t *testing.T) {
	cleanup := setupTestCatalog()
	defer cleanup()

	plan := BuildRmPlan(17, []string{"postgis"}, false)
	if plan == nil {
		t.Fatal("BuildRmPlan returned nil")
	}

	// postgis_topology depends on postgis, so risks should warn
	foundDependentRisk := false
	for _, risk := range plan.Risks {
		if strings.Contains(risk, "Dependent") || strings.Contains(risk, "postgis_topology") {
			foundDependentRisk = true
			break
		}
	}
	if !foundDependentRisk {
		t.Error("Risks should mention dependent extensions (postgis_topology depends on postgis)")
	}
}

func TestBuildRmPlan_NotInstalled(t *testing.T) {
	cleanup := setupTestCatalog()
	defer cleanup()

	plan := BuildRmPlan(17, []string{"nonexistent_ext"}, false)
	if plan == nil {
		t.Fatal("BuildRmPlan returned nil")
	}

	// Actions should be empty since nothing to remove
	if len(plan.Actions) != 0 {
		t.Errorf("Expected 0 actions for not-found extension, got %d", len(plan.Actions))
	}
}

func TestBuildRmPlan_CatalogNil(t *testing.T) {
	oldCatalog := Catalog
	Catalog = nil
	defer func() { Catalog = oldCatalog }()

	plan := BuildRmPlan(17, []string{"postgis"}, false)
	if plan == nil {
		t.Fatal("BuildRmPlan returned nil even with nil Catalog")
	}

	if !strings.Contains(plan.Expected, "catalog not initialized") {
		t.Errorf("Expected should mention catalog error, got %q", plan.Expected)
	}
}

func TestBuildRmPlan_EmptyNames(t *testing.T) {
	cleanup := setupTestCatalog()
	defer cleanup()

	plan := BuildRmPlan(17, []string{}, false)
	if plan == nil {
		t.Fatal("BuildRmPlan returned nil")
	}
	if !strings.Contains(plan.Expected, "no extensions specified") {
		t.Fatalf("expected empty-name error in Expected, got %q", plan.Expected)
	}
	if len(plan.Actions) != 0 {
		t.Fatalf("expected no actions for empty names, got %d", len(plan.Actions))
	}
}

// ============================================================================
// Plan Structure Completeness Tests
// ============================================================================

func TestPlanFieldsCompleteness_Add(t *testing.T) {
	cleanup := setupTestCatalog()
	defer cleanup()

	plan := BuildAddPlan(17, []string{"postgis"}, false)
	validateExtPlanFields(t, plan, "add")
}

func TestPlanFieldsCompleteness_Rm(t *testing.T) {
	cleanup := setupTestCatalog()
	defer cleanup()

	plan := BuildRmPlan(17, []string{"postgis"}, false)
	validateExtPlanFields(t, plan, "rm")
}

func validateExtPlanFields(t *testing.T, plan *output.Plan, planType string) {
	t.Helper()
	if plan == nil {
		t.Fatalf("%s plan: plan is nil", planType)
	}
	if plan.Command == "" {
		t.Errorf("%s plan: Command should not be empty", planType)
	}
	if len(plan.Actions) == 0 {
		t.Errorf("%s plan: Actions should not be empty for normal %s", planType, planType)
	}
	if len(plan.Affects) == 0 {
		t.Errorf("%s plan: Affects should not be empty for normal %s", planType, planType)
	}
	if plan.Expected == "" {
		t.Errorf("%s plan: Expected should not be empty", planType)
	}
	// Risks can be empty for some states
}

// ============================================================================
// buildAddActions / buildRmActions Tests
// ============================================================================

func TestBuildAddActions_WithResolved(t *testing.T) {
	resolved := []resolvedExt{
		{name: "postgis", packages: []string{"postgis35_17*"}},
	}
	actions := buildAddActions(resolved, nil, 17, false)
	if len(actions) != 2 {
		t.Errorf("Expected 2 actions, got %d", len(actions))
	}
	if len(actions) > 0 && !strings.Contains(actions[0].Description, "Resolve") {
		t.Errorf("First action should be resolve, got %q", actions[0].Description)
	}
}

func TestBuildAddActions_WithNotFound(t *testing.T) {
	resolved := []resolvedExt{
		{name: "postgis", packages: []string{"postgis35_17*"}},
	}
	notFound := []string{"nonexistent"}
	actions := buildAddActions(resolved, notFound, 17, false)
	// Should have resolve, execute, skip
	if len(actions) != 3 {
		t.Errorf("Expected 3 actions, got %d", len(actions))
	}
}

func TestBuildRmActions_WithResolved(t *testing.T) {
	resolved := []resolvedExt{
		{name: "postgis", packages: []string{"postgis35_17*"}},
	}
	actions := buildRmActions(resolved, nil, 17, false)
	if len(actions) != 2 {
		t.Errorf("Expected 2 actions, got %d", len(actions))
	}
}

// ============================================================================
// buildAddAffects / buildRmAffects Tests
// ============================================================================

func TestBuildAddAffects_Package(t *testing.T) {
	resolved := []resolvedExt{
		{name: "postgis", ext: &Extension{Name: "postgis", EnDesc: "PostGIS"}, packages: []string{"postgis35_17*"}},
	}
	affects := buildAddAffects(resolved, 17)
	if len(affects) == 0 {
		t.Error("Affects should not be empty")
	}
	if affects[0].Type != "package" {
		t.Errorf("First affect type should be 'package', got %q", affects[0].Type)
	}
}

func TestBuildAddAffects_NeedLoadService(t *testing.T) {
	resolved := []resolvedExt{
		{name: "timescaledb", ext: &Extension{Name: "timescaledb", NeedLoad: true}, packages: []string{"timescaledb-2-postgresql-17*"}},
	}
	affects := buildAddAffects(resolved, 17)
	foundService := false
	for _, a := range affects {
		if a.Type == "service" {
			foundService = true
			if !strings.Contains(a.Impact, "restart") {
				t.Errorf("Service impact should mention restart, got %q", a.Impact)
			}
			break
		}
	}
	if !foundService {
		t.Error("Affects should include service for NeedLoad extension")
	}
}

func TestBuildRmAffects(t *testing.T) {
	resolved := []resolvedExt{
		{name: "postgis", ext: &Extension{Name: "postgis", EnDesc: "PostGIS"}, packages: []string{"postgis35_17*"}},
	}
	affects := buildRmAffects(resolved)
	if len(affects) == 0 {
		t.Error("Affects should not be empty")
	}
	if affects[0].Impact != "remove" {
		t.Errorf("Impact should be 'remove', got %q", affects[0].Impact)
	}
}

// ============================================================================
// buildAddRisks / buildRmRisks Tests
// ============================================================================

func TestBuildAddRisks_NeedLoad(t *testing.T) {
	resolved := []resolvedExt{
		{name: "timescaledb", ext: &Extension{Name: "timescaledb", NeedLoad: true}},
	}
	risks := buildAddRisks(resolved, nil, nil)
	foundLoadRisk := false
	for _, risk := range risks {
		if strings.Contains(risk, "shared_preload_libraries") {
			foundLoadRisk = true
			break
		}
	}
	if !foundLoadRisk {
		t.Error("Risks should mention shared_preload_libraries for NeedLoad extension")
	}
}

func TestBuildRmRisks_WithDependents(t *testing.T) {
	cleanup := setupTestCatalog()
	defer cleanup()

	// postgis has postgis_topology depending on it (set up in catalog.Dependency)
	postgisExt := Catalog.ExtNameMap["postgis"]
	resolved := []resolvedExt{
		{name: "postgis", ext: postgisExt, packages: []string{"postgis35_17*"}},
	}
	risks := buildRmRisks(resolved, nil)
	foundDependentRisk := false
	for _, risk := range risks {
		if strings.Contains(risk, "Dependent") || strings.Contains(risk, "postgis_topology") {
			foundDependentRisk = true
			break
		}
	}
	if !foundDependentRisk {
		t.Error("Risks should mention dependent extensions")
	}
}

func TestBuildRmRisks_GeneralRisks(t *testing.T) {
	resolved := []resolvedExt{
		{name: "postgis", ext: &Extension{Name: "postgis"}, packages: []string{"postgis35_17*"}},
	}
	risks := buildRmRisks(resolved, nil)
	foundDBObjectRisk := false
	foundAppRisk := false
	for _, risk := range risks {
		if strings.Contains(risk, "Database objects") {
			foundDBObjectRisk = true
		}
		if strings.Contains(risk, "Applications") {
			foundAppRisk = true
		}
	}
	if !foundDBObjectRisk {
		t.Error("Risks should warn about database objects")
	}
	if !foundAppRisk {
		t.Error("Risks should warn about applications")
	}
}

// ============================================================================
// Command String Tests
// ============================================================================

func TestBuildAddCommand(t *testing.T) {
	cmd := buildAddCommand([]string{"postgis", "timescaledb"})
	if cmd != "pig ext add postgis timescaledb" {
		t.Errorf("Expected 'pig ext add postgis timescaledb', got %q", cmd)
	}
}

func TestBuildRmCommand(t *testing.T) {
	cmd := buildRmCommand([]string{"postgis"})
	if cmd != "pig ext rm postgis" {
		t.Errorf("Expected 'pig ext rm postgis', got %q", cmd)
	}
}
