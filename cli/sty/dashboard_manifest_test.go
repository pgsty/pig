package sty

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestEmbeddedDashboardManifestIsValidAndVersioned(t *testing.T) {
	source, err := loadDashboardManifest(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	manifest := source.Manifest
	if source.Source != "embedded:"+dashboardManifestKind || manifest.Kind != dashboardManifestKind || manifest.Revision != 1 || manifest.Normalization != dashboardNormalizationKind {
		t.Fatalf("manifest source=%q value=%+v", source.Source, manifest)
	}
	if len(manifest.Dashboards) != 62 || len(manifest.Datasources) != 5 {
		t.Fatalf("embedded manifest has %d dashboards and %d datasources", len(manifest.Dashboards), len(manifest.Datasources))
	}
	if len(manifest.RemovedDashboards) != 0 || len(manifest.RemovedDatasources) != 0 {
		t.Fatalf("unexpected initial tombstones: %+v %+v", manifest.RemovedDashboards, manifest.RemovedDatasources)
	}
}

func TestDashboardManifestRejectsUIDReuseUnsafePathsAndSecrets(t *testing.T) {
	base := minimalDashboardManifest()
	tests := []struct {
		name   string
		mutate func(*DashboardManifest)
		want   string
	}{
		{name: "UID reuse", mutate: func(manifest *DashboardManifest) {
			manifest.RemovedDashboards = []DashboardManifestTombstone{{UID: "managed"}}
		}, want: "reused"},
		{name: "unsafe path", mutate: func(manifest *DashboardManifest) {
			manifest.Dashboards[0].Path = "../managed.json"
		}, want: "unsafe"},
		{name: "duplicate UID", mutate: func(manifest *DashboardManifest) {
			manifest.Dashboards = append(manifest.Dashboards, manifest.Dashboards[0])
		}, want: "duplicated"},
		{name: "secret in spec", mutate: func(manifest *DashboardManifest) {
			manifest.Datasources[0].Spec["password"] = "must-not-land"
		}, want: "secret-bearing"},
		{name: "secret literal nested", mutate: func(manifest *DashboardManifest) {
			manifest.Datasources[0].Spec["jsonData"] = map[string]interface{}{"apiToken": "must-not-land"}
		}, want: "secret-bearing"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := cloneDashboardManifest(t, base)
			test.mutate(manifest)
			err := validateDashboardManifest(manifest)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestLocalDashboardManifestOverridesEmbeddedDefault(t *testing.T) {
	home := t.TempDir()
	directory := filepath.Join(home, "files", "grafana")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	manifest := minimalDashboardManifest()
	manifest.Revision = 7
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(directory, dashboardManifestFilename)
	if err := os.WriteFile(filename, raw, 0600); err != nil {
		t.Fatal(err)
	}
	source, err := loadDashboardManifest(home, "")
	if err != nil {
		t.Fatal(err)
	}
	if source.Source != filename || source.Manifest.Revision != 7 || source.DashboardDir != directory {
		t.Fatalf("source = %+v", source)
	}
}

func TestEmbeddedManifestMatchesPigstyDashboardCorpus(t *testing.T) {
	conf := os.Getenv("PIGSTY_CONF_DIR")
	if conf == "" {
		t.Skip("set PIGSTY_CONF_DIR to validate the adjacent Pigsty dashboard corpus")
	}
	home := filepath.Dir(filepath.Clean(conf))
	source, err := loadDashboardManifest(home, "")
	if err != nil {
		t.Fatal(err)
	}
	assets, err := loadDashboardRepositoryAssets(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 62 {
		t.Fatalf("asset count = %d, want 62", len(assets))
	}
	manifestPaths := make(map[string]struct{}, len(assets))
	for _, asset := range assets {
		manifestPaths[asset.Manifest.Path] = struct{}{}
	}
	var repositoryPaths []string
	err = filepath.WalkDir(source.DashboardDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".json" || filepath.Base(path) == dashboardManifestFilename {
			return nil
		}
		relative, err := filepath.Rel(source.DashboardDir, path)
		if err != nil {
			return err
		}
		repositoryPaths = append(repositoryPaths, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(repositoryPaths) != len(manifestPaths) {
		t.Fatalf("repository has %d dashboard JSON files, manifest has %d", len(repositoryPaths), len(manifestPaths))
	}
	for _, path := range repositoryPaths {
		if _, exists := manifestPaths[path]; !exists {
			t.Errorf("dashboard is not in the manifest: %s", path)
		}
	}
}

func TestDashboardNormalizationIgnoresOnlyVersionedVolatileFields(t *testing.T) {
	desired := decodeDashboardObjectForTest(t, `{
  "id": null,
  "uid": "managed",
  "title": "Managed",
  "version": 1,
  "iteration": 100,
  "preload": false,
  "liveNow": false,
  "fiscalYearStartMonth": 0,
  "templating": {"list":[{"name":"cluster","current":{"text":"old","value":"old"}}]},
  "panels": [{"id":1,"title":"Panel"}]
}`)
	live := decodeDashboardObjectForTest(t, `{
  "id": 42,
  "uid": "managed",
  "title": "Managed",
  "version": 19,
  "iteration": 999,
  "templating": {"list":[{"name":"cluster","current":{"text":"live","value":"live"}}]},
  "panels": [{"id":1,"title":"Panel"}]
}`)
	normalizedDesired, err := normalizeDashboardJSON(desired)
	if err != nil {
		t.Fatal(err)
	}
	normalizedLive, err := normalizeDashboardJSON(live)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(normalizedDesired.Value, normalizedLive.Value) || string(normalizedDesired.Canonical) != string(normalizedLive.Canonical) {
		t.Fatalf("volatile fields produced drift:\ndesired=%s\nlive=%s", normalizedDesired.Canonical, normalizedLive.Canonical)
	}
	live["title"] = "Changed"
	normalizedLive, err = normalizeDashboardJSON(live)
	if err != nil {
		t.Fatal(err)
	}
	paths := diffJSONPointers(normalizedDesired.Value, normalizedLive.Value)
	if len(paths) != 1 || paths[0] != "/title" {
		t.Fatalf("semantic diff paths = %#v", paths)
	}
}

func TestDatasourceNormalizationExcludesSecureAndServerDefaultFields(t *testing.T) {
	desired := map[string]interface{}{
		"uid": "ds-meta", "name": "Meta", "type": "postgres", "url": "127.0.0.1:5432",
		"jsonData": map[string]interface{}{"sslmode": "require"},
	}
	live := map[string]interface{}{
		"id": json.Number("9"), "orgId": json.Number("1"), "uid": "ds-meta", "name": "Meta", "type": "postgres", "url": "127.0.0.1:5432",
		"jsonData":         map[string]interface{}{"sslmode": "require", "serverDefault": true},
		"secureJsonFields": map[string]interface{}{"password": true}, "readOnly": true,
	}
	_, _, fields, err := normalizeDatasourceAgainst(desired, live)
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) != 0 {
		t.Fatalf("server-only datasource fields produced drift: %#v", fields)
	}
	live["url"] = "127.0.0.1:6432"
	_, _, fields, err = normalizeDatasourceAgainst(desired, live)
	if err != nil || len(fields) != 1 || fields[0] != "/url" {
		t.Fatalf("managed datasource diff = %#v, err=%v", fields, err)
	}
}

func minimalDashboardManifest() *DashboardManifest {
	return &DashboardManifest{
		Kind: dashboardManifestKind, Revision: 1, Normalization: dashboardNormalizationKind,
		Dashboards:        []DashboardManifestDashboard{{UID: "managed", Path: "managed.json", FolderUID: "infra", FolderTitle: "INFRA", Title: "Managed"}},
		Datasources:       []DashboardManifestDatasource{{UID: "ds-test", Name: "Test", Type: "prometheus", Spec: map[string]interface{}{"url": "http://127.0.0.1:8428"}}},
		RemovedDashboards: []DashboardManifestTombstone{}, RemovedDatasources: []DashboardManifestTombstone{},
	}
}

func cloneDashboardManifest(t *testing.T, source *DashboardManifest) *DashboardManifest {
	t.Helper()
	raw, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	clone := &DashboardManifest{}
	if err := json.Unmarshal(raw, clone); err != nil {
		t.Fatal(err)
	}
	return clone
}

func decodeDashboardObjectForTest(t *testing.T, raw string) map[string]interface{} {
	t.Helper()
	value := make(map[string]interface{})
	if err := decodeStrictJSON([]byte(raw), maxDashboardDefinitionBytes, &value); err != nil {
		t.Fatal(err)
	}
	return value
}
