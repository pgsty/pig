package sty

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pig/internal/output"
)

func TestDashboardListUsesUIDOwnershipInsteadOfFolderPlacement(t *testing.T) {
	manifest := dashboardTestManifest("managed")
	manifest.RemovedDashboards = []DashboardManifestTombstone{{UID: "retired", FolderUID: "legacy"}}
	home := writeDashboardTestRepository(t, manifest)
	fake := newFakeGrafana(t)
	fake.seedFolder("infra", "INFRA")
	fake.seedFolder("moved", "MOVED")
	fake.seedFolder("legacy", "LEGACY")
	fake.seedDashboard("managed", "Managed", "moved", false)
	fake.seedDashboard("copied", "Managed copy", "infra", false)
	fake.seedDashboard("retired", "Retired", "legacy", false)

	result := DashboardList(dashboardTestOptions(t, home, fake))
	if !result.Success || result.Code != 0 {
		t.Fatalf("dashboard list failed: %s", result)
	}
	data := result.Data.(*DashboardListData)
	managed := findDashboardObject(t, data.Objects, "dashboard", "managed")
	if !managed.Managed || !managed.Repository || !managed.Live || managed.Status != "moved" || managed.FolderUID != "moved" {
		t.Fatalf("moved managed dashboard = %+v", managed)
	}
	copy := findDashboardObject(t, data.Objects, "dashboard", "copied")
	if copy.Managed || copy.Repository || !copy.Live || copy.Status != "unmanaged" {
		t.Fatalf("copied dashboard inherited folder ownership: %+v", copy)
	}
	retired := findDashboardObject(t, data.Objects, "dashboard", "retired")
	if retired.Managed || !retired.Tombstoned || !retired.Repository || retired.Status != "removed_live" {
		t.Fatalf("tombstoned dashboard = %+v", retired)
	}
	placement := findDashboardObject(t, data.Objects, "folder", "infra")
	if placement.Managed || !placement.Repository || !placement.Live || placement.Status != "managed_placement" {
		t.Fatalf("manifest folder placement = %+v", placement)
	}
	movedFolder := findDashboardObject(t, data.Objects, "folder", "moved")
	if movedFolder.Managed || movedFolder.Repository || movedFolder.Status != "unmanaged" {
		t.Fatalf("moved folder inherited ownership: %+v", movedFolder)
	}
}

func TestDashboardCheckClassifiesPermissionAndRedactsCredentials(t *testing.T) {
	manifest := dashboardTestManifest("managed")
	home := writeDashboardTestRepository(t, manifest)
	fake := newFakeGrafana(t)
	options := dashboardTestOptions(t, home, fake)
	t.Setenv("GRAFANA_PASSWORD", "wrong-password")

	result := DashboardCheck(options)
	if result.Success || result.Code != output.CodeStyDashboardPermissionDenied {
		t.Fatalf("permission result = %s", result)
	}
	raw, err := result.JSON()
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"api-secret", "wrong-password", "server-secret"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("credential leaked in dashboard result: %s", raw)
		}
	}
}

func TestDashboardCredentialsUseExactAllVarsAndOwnerOnlyPasswordFile(t *testing.T) {
	manifest := dashboardTestManifest("managed")
	home := writeDashboardTestRepository(t, manifest)
	fake := newFakeGrafana(t)
	inventoryPath := filepath.Join(t.TempDir(), "pigsty.yml")
	inventoryBody := `all:
  vars:
    grafana_admin_username: inventory-user
    grafana_admin_password: inventory-password
  children:
    pg-meta:
      vars:
        grafana_admin_username: group-user
        grafana_admin_password: group-password
`
	if err := os.WriteFile(inventoryPath, []byte(inventoryBody), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GRAFANA_PASSWORD", "")
	fake.username, fake.password = "inventory-user", "inventory-password"
	options := DashboardOptions{Home: home, Inventory: inventoryPath, Endpoint: fake.server.URL, HTTPClient: fake.server.Client()}
	if result := DashboardCheck(options); !result.Success {
		t.Fatalf("exact all.vars credentials were not used: %s", result)
	}

	passwordFile := filepath.Join(t.TempDir(), "grafana-password")
	if err := os.WriteFile(passwordFile, []byte("file-password\n"), 0644); err != nil {
		t.Fatal(err)
	}
	options.PasswordFile = passwordFile
	result := DashboardCheck(options)
	if result.Success || result.Code != output.CodeStyDashboardConfigInvalid || strings.Contains(result.Detail, "file-password") {
		t.Fatalf("world-readable password file result = %s", result)
	}
	if err := os.Chmod(passwordFile, 0600); err != nil {
		t.Fatal(err)
	}
	fake.password = "file-password"
	if result := DashboardCheck(options); !result.Success {
		t.Fatalf("owner-only password file did not take precedence: %s", result)
	}
}

func TestDashboardDiffNormalizesVolatileFieldsAndIgnoresCopies(t *testing.T) {
	manifest := dashboardTestManifest("managed")
	home := writeDashboardTestRepository(t, manifest)
	fake := newFakeGrafana(t)
	fake.seedFolder("infra", "INFRA")
	fake.seedDashboard("managed", "Managed", "infra", false)
	fake.seedDashboard("copied", "Managed", "infra", false)
	options := dashboardTestOptions(t, home, fake)

	result := DashboardDiff(options)
	if !result.Success {
		t.Fatalf("volatile Grafana fields produced drift: %s", result)
	}
	fake.setDashboardField("managed", "title", "Changed")
	result = DashboardDiff(options)
	if result.Success || result.Code != output.CodeStyDashboardDrift {
		t.Fatalf("semantic drift result = %s", result)
	}
	data := result.Data.(*DashboardDiffData)
	if len(data.Differences) != 1 || data.Differences[0].UID != "managed" || len(data.Differences[0].Fields) != 1 || data.Differences[0].Fields[0] != "/title" {
		t.Fatalf("normalized differences = %+v", data.Differences)
	}
}

func TestDashboardInitIsIdempotentAndPreservesDatasourceSecrets(t *testing.T) {
	manifest := dashboardTestManifest("managed")
	manifest.Datasources = []DashboardManifestDatasource{{
		UID: "ds-test", Name: "Test", Type: "prometheus",
		Spec: map[string]interface{}{"access": "proxy", "url": "http://127.0.0.1:8428"},
		SecureFields: []DashboardManifestSecureField{{
			Name: "password", Env: "DS_TEST_PASSWORD", RequiredOnCreate: true,
		}},
	}}
	home := writeDashboardTestRepository(t, manifest)
	fake := newFakeGrafana(t)
	options := dashboardTestOptions(t, home, fake)
	t.Setenv("DS_TEST_PASSWORD", "datasource-secret")

	first := DashboardInit(options)
	if !first.Success {
		t.Fatalf("first dashboard init failed: %s", first)
	}
	if fake.mutationCount("folder:infra") != 1 || fake.mutationCount("dashboard:managed") != 1 || fake.mutationCount("datasource:ds-test") != 1 {
		t.Fatalf("first init mutation counts are not one")
	}
	if secret := fake.datasourceSecret("ds-test", "password"); secret != "datasource-secret" {
		t.Fatalf("created datasource secret = %q", secret)
	}

	second := DashboardInit(options)
	if !second.Success {
		t.Fatalf("idempotent dashboard init failed: %s", second)
	}
	if fake.mutationCount("folder:infra") != 1 || fake.mutationCount("dashboard:managed") != 1 || fake.mutationCount("datasource:ds-test") != 1 {
		t.Fatalf("second init performed a mutation")
	}

	fake.setDatasourceField("ds-test", "url", "http://127.0.0.1:9999")
	t.Setenv("DS_TEST_PASSWORD", "")
	third := DashboardInit(options)
	if !third.Success || fake.mutationCount("datasource:ds-test") != 2 {
		t.Fatalf("non-secret datasource repair failed: %s", third)
	}
	if secret := fake.datasourceSecret("ds-test", "password"); secret != "datasource-secret" {
		t.Fatalf("redacted input overwrote live datasource secret: %q", secret)
	}
	raw, err := third.JSON()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "datasource-secret") {
		t.Fatalf("datasource secret leaked in init result: %s", raw)
	}
}

func TestDashboardCleanRequiresConfirmationAndProtectsCopies(t *testing.T) {
	manifest := dashboardTestManifest("managed")
	manifest.RemovedDashboards = []DashboardManifestTombstone{{UID: "retired", FolderUID: "legacy"}}
	home := writeDashboardTestRepository(t, manifest)
	fake := newFakeGrafana(t)
	fake.seedFolder("infra", "INFRA")
	fake.seedFolder("moved", "MOVED")
	fake.seedFolder("legacy", "LEGACY")
	fake.seedDashboard("managed", "Managed", "moved", false)
	fake.seedDashboard("copied", "Managed copy", "infra", false)
	fake.seedDashboard("retired", "Retired", "legacy", false)
	options := dashboardTestOptions(t, home, fake)

	plan := DashboardClean(options)
	if plan.Success || plan.Code != output.CodeStyDashboardConfirmationRequired {
		t.Fatalf("clean confirmation result = %s", plan)
	}
	if fake.mutationCount("dashboard:managed") != 0 || !fake.hasDashboard("managed") {
		t.Fatal("clean plan mutated Grafana")
	}

	options.Yes = true
	result := DashboardClean(options)
	if !result.Success {
		t.Fatalf("dashboard clean failed: %s", result)
	}
	if fake.hasDashboard("managed") || !fake.hasDashboard("copied") || !fake.hasDashboard("retired") {
		t.Fatalf("clean crossed UID ownership boundary")
	}
	if !fake.hasFolder("infra") || !fake.hasFolder("moved") || !fake.hasFolder("legacy") {
		t.Fatalf("clean deleted a non-empty or non-placement folder")
	}
	if fake.mutationCount("dashboard:copied") != 0 || fake.mutationCount("dashboard:retired") != 0 {
		t.Fatalf("clean mutated copied or tombstoned dashboards")
	}
}

func TestDashboardCleanDeletesPlacementFolderOnlyWhenEmpty(t *testing.T) {
	manifest := dashboardTestManifest("managed")
	home := writeDashboardTestRepository(t, manifest)
	fake := newFakeGrafana(t)
	fake.seedFolder("infra", "INFRA")
	fake.seedDashboard("managed", "Managed", "infra", false)
	options := dashboardTestOptions(t, home, fake)
	options.Yes = true

	result := DashboardClean(options)
	if !result.Success {
		t.Fatalf("dashboard clean failed: %s", result)
	}
	if fake.hasDashboard("managed") || fake.hasFolder("infra") {
		t.Fatalf("empty manifest placement folder survived clean")
	}
}

func TestDashboardPruneDeletesOnlyTombstoneLineage(t *testing.T) {
	manifest := dashboardTestManifest("managed")
	manifest.RemovedDashboards = []DashboardManifestTombstone{{UID: "retired", FolderUID: "old"}}
	manifest.RemovedDatasources = []DashboardManifestTombstone{{UID: "ds-retired"}}
	home := writeDashboardTestRepository(t, manifest)
	fake := newFakeGrafana(t)
	fake.seedFolder("infra", "INFRA")
	fake.seedFolder("old", "OLD")
	fake.seedDashboard("managed", "Managed", "infra", false)
	fake.seedDashboard("retired", "Retired", "old", false)
	fake.seedDashboard("copied", "Copied", "old", false)
	fake.seedDatasource(map[string]interface{}{"uid": "ds-retired", "name": "Retired", "type": "prometheus"}, nil)
	fake.seedDatasource(map[string]interface{}{"uid": "ds-unmanaged", "name": "User", "type": "prometheus"}, nil)
	options := dashboardTestOptions(t, home, fake)
	options.Prune = true

	plan := DashboardInit(options)
	if plan.Success || plan.Code != output.CodeStyDashboardConfirmationRequired {
		t.Fatalf("prune confirmation result = %s", plan)
	}
	if !fake.hasDashboard("retired") || !fake.hasDatasource("ds-retired") {
		t.Fatal("prune plan mutated tombstone lineage")
	}

	options.Yes = true
	result := DashboardInit(options)
	if !result.Success {
		t.Fatalf("dashboard prune failed: %s", result)
	}
	if fake.hasDashboard("retired") || fake.hasDatasource("ds-retired") {
		t.Fatal("tombstoned resources survived prune")
	}
	if !fake.hasDashboard("managed") || !fake.hasDashboard("copied") || !fake.hasDatasource("ds-unmanaged") {
		t.Fatal("prune crossed tombstone lineage")
	}
	if !fake.hasFolder("old") {
		t.Fatal("prune deleted a tombstone folder containing an unmanaged copy")
	}
}

func TestDashboardInitReportsProvisionedObjectFailureAndContinues(t *testing.T) {
	manifest := dashboardTestManifest("locked", "mutable")
	home := writeDashboardTestRepository(t, manifest)
	fake := newFakeGrafana(t)
	fake.seedFolder("infra", "INFRA")
	fake.seedDashboard("locked", "Old locked", "infra", true)
	fake.seedDashboard("mutable", "Old mutable", "infra", false)
	options := dashboardTestOptions(t, home, fake)

	result := DashboardInit(options)
	if result.Success || result.Code != output.CodeStyDashboardPartialFailure {
		t.Fatalf("partial failure result = %s", result)
	}
	data := result.Data.(*DashboardActionData)
	locked := findDashboardResult(t, data.Results, "dashboard", "locked")
	mutable := findDashboardResult(t, data.Results, "dashboard", "mutable")
	if locked.Status != "failed" || !strings.Contains(locked.Message, "provisioned/read-only") {
		t.Fatalf("provisioned result = %+v", locked)
	}
	if mutable.Status != "applied" || fake.mutationCount("dashboard:mutable") != 1 {
		t.Fatalf("mutable result = %+v", mutable)
	}
}

func TestDashboardDumpIsAtomicAndExcludesDatasourceSecrets(t *testing.T) {
	manifest := dashboardTestManifest("managed")
	manifest.Datasources = []DashboardManifestDatasource{{
		UID: "ds-test", Name: "Test", Type: "prometheus", Spec: map[string]interface{}{"url": "http://127.0.0.1:8428"},
		SecureFields: []DashboardManifestSecureField{{Name: "password", Env: "DS_TEST_PASSWORD"}},
	}}
	home := writeDashboardTestRepository(t, manifest)
	fake := newFakeGrafana(t)
	fake.seedDashboard("managed", "Managed", "infra", false)
	fake.seedDatasource(map[string]interface{}{"uid": "ds-test", "name": "Test", "type": "prometheus", "url": "http://127.0.0.1:8428"}, map[string]string{"password": "never-dump-this"})
	options := dashboardTestOptions(t, home, fake)
	destination := filepath.Join(t.TempDir(), "export")
	options.DumpDir = destination

	result := DashboardDump(options)
	if !result.Success {
		t.Fatalf("dashboard dump failed: %s", result)
	}
	info, err := os.Stat(destination)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		t.Fatalf("dump directory mode = %v, err=%v", info, err)
	}
	var combined strings.Builder
	err = filepath.WalkDir(destination, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		fileInfo, statErr := entry.Info()
		if statErr != nil {
			return statErr
		}
		if fileInfo.Mode().Perm() != 0600 {
			t.Fatalf("dump file %s mode = %o", path, fileInfo.Mode().Perm())
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		combined.Write(raw)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(combined.String(), "never-dump-this") || strings.Contains(combined.String(), "secureJsonData") || strings.Contains(combined.String(), "secureJsonFields") {
		t.Fatalf("datasource credential material appeared in dump: %s", combined.String())
	}

	missing := dashboardTestManifest("managed", "missing")
	missingHome := writeDashboardTestRepository(t, missing)
	failedDestination := filepath.Join(t.TempDir(), "failed-export")
	failedOptions := dashboardTestOptions(t, missingHome, fake)
	failedOptions.DumpDir = failedDestination
	failed := DashboardDump(failedOptions)
	if failed.Success || failed.Code != output.CodeStyDashboardPartialFailure {
		t.Fatalf("partial dump result = %s", failed)
	}
	if _, err := os.Lstat(failedDestination); !os.IsNotExist(err) {
		t.Fatalf("failed dump left a destination: %v", err)
	}
}

func dashboardTestManifest(uids ...string) *DashboardManifest {
	manifest := &DashboardManifest{
		Kind: dashboardManifestKind, Revision: 1, Normalization: dashboardNormalizationKind,
		Dashboards: []DashboardManifestDashboard{}, Datasources: []DashboardManifestDatasource{},
		RemovedDashboards: []DashboardManifestTombstone{}, RemovedDatasources: []DashboardManifestTombstone{},
	}
	for _, uid := range uids {
		title := strings.ToUpper(uid[:1]) + uid[1:]
		manifest.Dashboards = append(manifest.Dashboards, DashboardManifestDashboard{
			UID: uid, Path: uid + ".json", FolderUID: "infra", FolderTitle: "INFRA", Title: title,
		})
	}
	return manifest
}

func writeDashboardTestRepository(t *testing.T, manifest *DashboardManifest) string {
	t.Helper()
	home := t.TempDir()
	directory := filepath.Join(home, "files", "grafana")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	for _, item := range manifest.Dashboards {
		dashboard := map[string]interface{}{
			"uid": item.UID, "title": item.Title, "id": nil, "version": 0, "panels": []interface{}{},
		}
		raw, err := json.MarshalIndent(dashboard, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		filename := filepath.Join(directory, filepath.FromSlash(item.Path))
		if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, append(raw, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, dashboardManifestFilename), append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	return home
}

func dashboardTestOptions(t *testing.T, home string, fake *fakeGrafana) DashboardOptions {
	t.Helper()
	t.Setenv("GRAFANA_PASSWORD", fake.password)
	return DashboardOptions{Home: home, Endpoint: fake.server.URL, Username: fake.username, HTTPClient: fake.server.Client()}
}

func findDashboardObject(t *testing.T, objects []DashboardObject, kind, uid string) DashboardObject {
	t.Helper()
	for _, object := range objects {
		if object.ObjectKind == kind && object.UID == uid {
			return object
		}
	}
	t.Fatalf("%s %s not found in %+v", kind, uid, objects)
	return DashboardObject{}
}

func findDashboardResult(t *testing.T, results []DashboardObjectResult, kind, uid string) DashboardObjectResult {
	t.Helper()
	for _, result := range results {
		if result.ObjectKind == kind && result.UID == uid {
			return result
		}
	}
	t.Fatalf("%s %s not found in %+v", kind, uid, results)
	return DashboardObjectResult{}
}
