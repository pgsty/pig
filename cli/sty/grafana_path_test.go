package sty

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestResolveGrafanaScopeShapes(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "files", "grafana")
	folder := filepath.Join(root, "pgsql")
	if err := os.MkdirAll(folder, 0755); err != nil {
		t.Fatal(err)
	}
	rootFile := filepath.Join(root, "pigsty.json")
	folderFile := filepath.Join(folder, "pgsql-overview.json")
	writeGrafanaTestDashboard(t, rootFile, "pigsty")
	writeGrafanaTestDashboard(t, folderFile, "pgsql-overview")

	for _, test := range []struct {
		name      string
		path      string
		kind      grafanaScopeKind
		folderUID string
		uid       string
	}{
		{name: "default root", kind: grafanaScopeRoot},
		{name: "explicit root", path: root, kind: grafanaScopeRoot},
		{name: "folder", path: folder, kind: grafanaScopeFolder, folderUID: "pgsql"},
		{name: "root file", path: rootFile, kind: grafanaScopeFile, uid: "pigsty"},
		{name: "folder file", path: folderFile, kind: grafanaScopeFile, folderUID: "pgsql", uid: "pgsql-overview"},
	} {
		t.Run(test.name, func(t *testing.T) {
			scope, err := resolveGrafanaScope(GrafanaOptions{Home: home, Path: test.path}, grafanaActionLoad)
			if err != nil {
				t.Fatal(err)
			}
			if scope.Root != root || scope.Kind != test.kind || scope.FolderUID != test.folderUID || scope.UID != test.uid {
				t.Fatalf("scope = %+v", scope)
			}
		})
	}
}

func TestResolveGrafanaScopeRequiresUnambiguousRoot(t *testing.T) {
	directory := t.TempDir()
	outside := filepath.Join(directory, "dashboards")
	if err := os.MkdirAll(outside, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveGrafanaScope(GrafanaOptions{Path: outside}, grafanaActionLoad); err == nil {
		t.Fatal("arbitrary directory was guessed as a Grafana root")
	}
	if _, err := resolveGrafanaScope(GrafanaOptions{}, grafanaActionLoad); err == nil {
		t.Fatal("missing PATH and Pigsty home succeeded")
	}

	root := filepath.Join(directory, "grafana")
	nested := filepath.Join(root, "pgsql", "nested")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveGrafanaScope(GrafanaOptions{Path: nested}, grafanaActionLoad); err == nil {
		t.Fatal("second-level Grafana directory succeeded")
	}
}

func TestResolveGrafanaScopeRejectsSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions differ on Windows")
	}
	directory := t.TempDir()
	root := filepath.Join(directory, "grafana")
	realFolder := filepath.Join(directory, "real")
	if err := os.MkdirAll(realFolder, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "pgsql")
	if err := os.Symlink(realFolder, link); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveGrafanaScope(GrafanaOptions{Path: link}, grafanaActionLoad); err == nil {
		t.Fatal("symlink scope succeeded")
	}
}

func TestResolveGrafanaDumpScopeAllowsArbitraryDirectory(t *testing.T) {
	parent := t.TempDir()
	destination := filepath.Join(parent, "dashboard-snapshot")
	scope, err := resolveGrafanaDumpScope(GrafanaOptions{DumpDir: destination})
	if err != nil {
		t.Fatal(err)
	}
	if scope.Kind != grafanaScopeRoot || scope.Root != destination || scope.Path != destination {
		t.Fatalf("scope = %+v", scope)
	}
	if _, err := resolveGrafanaDumpScope(GrafanaOptions{Path: filepath.Join(parent, "grafana"), DumpDir: destination}); err == nil {
		t.Fatal("--dir with PATH succeeded")
	}
}

func TestDashboardForGrafanaInitDoesNotMutateSource(t *testing.T) {
	source := map[string]interface{}{"id": json.Number("42"), "uid": "pigsty"}
	dashboard := dashboardForGrafanaInit(source)
	if dashboard["id"] != nil {
		t.Fatalf("dashboard id = %v, want nil", dashboard["id"])
	}
	if source["id"] != json.Number("42") {
		t.Fatalf("source id changed to %v", source["id"])
	}
}

func TestMarshalGrafanaDashboardIsDeterministicAndCompact(t *testing.T) {
	left := map[string]interface{}{}
	left["z"] = strings.Repeat("x", maxGrafanaInlineLength)
	left["a"] = map[string]interface{}{"b": json.Number("2"), "a": json.Number("1")}
	right := map[string]interface{}{}
	right["a"] = map[string]interface{}{"a": json.Number("1"), "b": json.Number("2")}
	right["z"] = strings.Repeat("x", maxGrafanaInlineLength)

	first, err := marshalGrafanaDashboard(left)
	if err != nil {
		t.Fatal(err)
	}
	second, err := marshalGrafanaDashboard(right)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("deterministic encodings differ:\n%s\n%s", first, second)
	}
	if !bytes.HasPrefix(first, []byte("{\n  \"a\":{\"a\":1,\"b\":2},\n  \"z\":")) || !bytes.HasSuffix(first, []byte("\n}\n")) {
		t.Fatalf("encoding is not compact hybrid JSON:\n%s", first)
	}
	if _, err := decodeGrafanaDashboard(first); err != nil {
		t.Fatalf("encoded dashboard is invalid JSON: %v", err)
	}
}

func TestLoadGrafanaScopeAssetsIsDeterministicAndStrict(t *testing.T) {
	directory := t.TempDir()
	root := filepath.Join(directory, "grafana")
	if err := os.MkdirAll(filepath.Join(root, "pgsql"), 0755); err != nil {
		t.Fatal(err)
	}
	writeGrafanaTestDashboard(t, filepath.Join(root, "z.json"), "z")
	writeGrafanaTestDashboard(t, filepath.Join(root, "a.json"), "a")
	writeGrafanaTestDashboard(t, filepath.Join(root, "pgsql", "b.json"), "b")
	scope, err := resolveGrafanaScope(GrafanaOptions{Path: root}, grafanaActionLoad)
	if err != nil {
		t.Fatal(err)
	}
	if err := loadGrafanaScopeAssets(scope); err != nil {
		t.Fatal(err)
	}
	if len(scope.Assets) != 3 || scope.Assets[0].UID != "a" || scope.Assets[1].UID != "z" || scope.Assets[2].UID != "b" {
		t.Fatalf("assets = %+v", scope.Assets)
	}
	if len(scope.Folders) != 1 || scope.Folders[0] != "pgsql" {
		t.Fatalf("folders = %v", scope.Folders)
	}

	bad := filepath.Join(root, "bad-name.json")
	writeGrafanaTestDashboard(t, bad, "different-uid")
	if err := loadGrafanaScopeAssets(scope); err == nil {
		t.Fatal("filename/UID mismatch succeeded")
	}
}

func TestLoadGrafanaScopeAcceptsDashboardV2Resource(t *testing.T) {
	root := filepath.Join(t.TempDir(), "grafana")
	filename := filepath.Join(root, "minio", "minio-instance.json")
	if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{
		"apiVersion":"dashboard.grafana.app/v2",
		"kind":"Dashboard",
		"metadata":{"name":"minio-instance","annotations":{"grafana.app/message":"tabs","grafana.app/createdBy":"server"},"generation":9},
		"spec":{"title":"MinIO Instance","layout":{"kind":"TabsLayout"}},
		"status":{"operatorStates":{"conversion":{"state":"success"}}}
	}`)
	if err := os.WriteFile(filename, raw, 0644); err != nil {
		t.Fatal(err)
	}
	scope, err := resolveGrafanaScope(GrafanaOptions{Path: root}, grafanaActionLoad)
	if err != nil {
		t.Fatal(err)
	}
	if err := loadGrafanaScopeAssets(scope); err != nil {
		t.Fatal(err)
	}
	if len(scope.Assets) != 1 {
		t.Fatalf("assets = %+v", scope.Assets)
	}
	asset := scope.Assets[0]
	if asset.UID != "minio-instance" || asset.FolderUID != "minio" || asset.Namespace != "default" || asset.Format != grafanaDashboardV2 {
		t.Fatalf("asset = %+v", asset)
	}
	resource, err := dashboardResourceForGrafanaInit(asset.Dashboard, asset.Namespace, asset.UID, asset.FolderUID)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := resource["status"]; exists {
		t.Fatalf("server-managed status was retained: %#v", resource)
	}
	metadata := resource["metadata"].(map[string]interface{})
	annotations := metadata["annotations"].(map[string]interface{})
	if metadata["name"] != "minio-instance" || metadata["namespace"] != "default" ||
		annotations["grafana.app/folder"] != "minio" || annotations["grafana.app/message"] != "tabs" {
		t.Fatalf("normalized metadata = %#v", metadata)
	}
	if _, exists := annotations["grafana.app/createdBy"]; exists {
		t.Fatalf("server-managed annotation was retained: %#v", annotations)
	}
}

func TestLoadGrafanaScopeRejectsUnsupportedDashboardResourceVersion(t *testing.T) {
	root := filepath.Join(t.TempDir(), "grafana")
	filename := filepath.Join(root, "minio-instance.json")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(`{"apiVersion":"dashboard.grafana.app/v1beta1","kind":"Dashboard","metadata":{"name":"minio-instance"},"spec":{}}`), 0644); err != nil {
		t.Fatal(err)
	}
	scope, err := resolveGrafanaScope(GrafanaOptions{Path: root}, grafanaActionLoad)
	if err != nil {
		t.Fatal(err)
	}
	if err := loadGrafanaScopeAssets(scope); err == nil || !strings.Contains(err.Error(), "unsupported Grafana dashboard resource") {
		t.Fatalf("unsupported resource error = %v", err)
	}
}

func writeGrafanaTestDashboard(t *testing.T, filename, uid string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"id":null,"uid":"` + uid + `","title":"` + uid + `","version":1}`)
	if err := os.WriteFile(filename, raw, 0644); err != nil {
		t.Fatal(err)
	}
}
