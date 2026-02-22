package install

import (
	"strings"
	"testing"

	"pig/cli/ext"
	"pig/internal/config"
	"pig/internal/output"
)

func withInstallPlanTestEnv(t *testing.T, osType, osVersion string, catalog *ext.ExtensionCatalog) func() {
	t.Helper()

	oldOSType := config.OSType
	oldOSVersion := config.OSVersion
	oldCatalog := ext.Catalog

	config.OSType = osType
	config.OSVersion = osVersion
	ext.Catalog = catalog

	return func() {
		config.OSType = oldOSType
		config.OSVersion = oldOSVersion
		ext.Catalog = oldCatalog
	}
}

func newInstallPlanCatalog(e *ext.Extension) *ext.ExtensionCatalog {
	return &ext.ExtensionCatalog{
		Extensions: []*ext.Extension{e},
		ExtNameMap: map[string]*ext.Extension{
			e.Name: e,
		},
		ExtPkgMap: map[string]*ext.Extension{
			e.Pkg: e,
		},
		Dependency: map[string][]string{},
		AliasMap:   map[string]string{},
	}
}

func containsInstallAction(actions []output.Action, expected string) bool {
	for _, action := range actions {
		if strings.Contains(action.Description, expected) {
			return true
		}
	}
	return false
}

func containsInstallResource(affects []output.Resource, name string) bool {
	for _, a := range affects {
		if a.Name == name {
			return true
		}
	}
	return false
}

func containsInstallRisk(risks []string, expected string) bool {
	for _, risk := range risks {
		if strings.Contains(strings.ToLower(risk), strings.ToLower(expected)) {
			return true
		}
	}
	return false
}

func TestBuildInstallPlanEmptyNames(t *testing.T) {
	plan := BuildInstallPlan(17, nil, false, false)
	if plan == nil {
		t.Fatal("BuildInstallPlan returned nil")
	}
	if !strings.Contains(plan.Expected, "no package names provided") {
		t.Fatalf("unexpected Expected: %q", plan.Expected)
	}
	if len(plan.Actions) != 0 {
		t.Fatalf("expected no actions, got %d", len(plan.Actions))
	}
}

func TestBuildInstallPlanResolveAndExecute(t *testing.T) {
	cleanup := withInstallPlanTestEnv(t, config.DistroDEB, "12", newInstallPlanCatalog(&ext.Extension{
		Name:   "postgis",
		Pkg:    "postgis",
		Lead:   true,
		DebPkg: "postgresql-$v-postgis-3",
	}))
	defer cleanup()

	plan := BuildInstallPlan(17, []string{"postgis", "nginx"}, true, false)
	if plan == nil {
		t.Fatal("BuildInstallPlan returned nil")
	}
	if !strings.Contains(plan.Command, "pig install -y postgis nginx") {
		t.Fatalf("unexpected command: %q", plan.Command)
	}
	if len(plan.Actions) < 2 {
		t.Fatalf("expected at least 2 actions, got %d", len(plan.Actions))
	}
	if !containsInstallAction(plan.Actions, "Resolve package names") {
		t.Fatalf("missing resolve action: %#v", plan.Actions)
	}
	if !containsInstallAction(plan.Actions, "sudo apt-get install -y") {
		t.Fatalf("missing execute action: %#v", plan.Actions)
	}
	if !containsInstallResource(plan.Affects, "postgresql-17-postgis-3") {
		t.Fatalf("missing translated package in affects: %#v", plan.Affects)
	}
	if !containsInstallResource(plan.Affects, "nginx") {
		t.Fatalf("missing raw package in affects: %#v", plan.Affects)
	}
	if !strings.Contains(plan.Expected, "Packages installed:") {
		t.Fatalf("unexpected Expected: %q", plan.Expected)
	}
}

func TestBuildInstallPlanNoTranslationRisk(t *testing.T) {
	cleanup := withInstallPlanTestEnv(t, config.DistroDEB, "12", newInstallPlanCatalog(&ext.Extension{
		Name:   "postgis",
		Pkg:    "postgis",
		Lead:   true,
		DebPkg: "postgresql-$v-postgis-3",
	}))
	defer cleanup()

	plan := BuildInstallPlan(17, []string{"nginx"}, false, true)
	if plan == nil {
		t.Fatal("BuildInstallPlan returned nil")
	}
	if !strings.Contains(plan.Command, "pig install -n nginx") {
		t.Fatalf("unexpected command: %q", plan.Command)
	}
	if !containsInstallRisk(plan.Risks, "translation is disabled") {
		t.Fatalf("expected no-translation risk, got: %v", plan.Risks)
	}
}

func TestBuildInstallPlanNoPackageAvailable(t *testing.T) {
	cleanup := withInstallPlanTestEnv(t, config.DistroDEB, "12", newInstallPlanCatalog(&ext.Extension{
		Name: "no_pkg",
		Pkg:  "no_pkg",
		Lead: true,
	}))
	defer cleanup()

	plan := BuildInstallPlan(17, []string{"no_pkg"}, false, false)
	if plan == nil {
		t.Fatal("BuildInstallPlan returned nil")
	}
	if !containsInstallAction(plan.Actions, "Skip (no package available)") {
		t.Fatalf("expected skip action, got: %#v", plan.Actions)
	}
	if !strings.Contains(plan.Expected, "Skipped (no package available): no_pkg") {
		t.Fatalf("unexpected Expected: %q", plan.Expected)
	}
	if !containsInstallRisk(plan.Risks, "No package available for: no_pkg") {
		t.Fatalf("expected no-package risk, got: %v", plan.Risks)
	}
}

func TestBuildInstallPlanUnsupportedOS(t *testing.T) {
	cleanup := withInstallPlanTestEnv(t, config.DistroMAC, "14", nil)
	defer cleanup()

	plan := BuildInstallPlan(17, []string{"nginx"}, false, false)
	if plan == nil {
		t.Fatal("BuildInstallPlan returned nil")
	}
	if !strings.Contains(plan.Expected, "macOS brew installation is not supported yet") {
		t.Fatalf("unexpected Expected: %q", plan.Expected)
	}
	if len(plan.Actions) != 0 {
		t.Fatalf("expected no actions on unsupported platform, got %d", len(plan.Actions))
	}
}
