package sty

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"pig/internal/output"
)

type fakeGrafanaDashboard struct {
	value  map[string]interface{}
	folder string
}

type fakeGrafanaServer struct {
	server                  *httptest.Server
	organization            string
	dashboards              map[string]fakeGrafanaDashboard
	resources               map[string]map[string]interface{}
	folders                 map[string]string
	nextID                  int64
	starCalls               int
	organizationPrefs       int
	userPrefs               int
	organizationPreferences map[string]interface{}
	userPreferences         map[string]interface{}
	requests                []string
	upsertFailures          map[string]int
	healthDatabase          string
}

func newFakeGrafanaServer(t *testing.T) *fakeGrafanaServer {
	t.Helper()
	fake := &fakeGrafanaServer{
		organization: "Main Org.", dashboards: make(map[string]fakeGrafanaDashboard),
		resources: make(map[string]map[string]interface{}), folders: make(map[string]string), nextID: 10,
		organizationPreferences: map[string]interface{}{"theme": "dark", "language": "zh-Hans", "timezone": "utc"},
		userPreferences:         map[string]interface{}{"theme": "light", "language": "en-US", "weekStart": "monday"},
		upsertFailures:          make(map[string]int),
		healthDatabase:          "ok",
	}
	fake.server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		path := strings.TrimPrefix(request.URL.Path, "/api/")
		fake.requests = append(fake.requests, request.Method+" "+path)
		if request.Method == http.MethodGet && path == "health" {
			if _, _, ok := request.BasicAuth(); ok {
				t.Errorf("health request unexpectedly contained basic auth")
			}
			writeFakeGrafanaJSON(writer, map[string]interface{}{"database": fake.healthDatabase, "version": "12.3.0", "commit": "abc"})
			return
		}
		username, password, ok := request.BasicAuth()
		if !ok || username != "admin" || password != "secret" {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/apis/dashboard.grafana.app/v2/namespaces/default/dashboards/"):
			uid := strings.TrimPrefix(request.URL.Path, "/apis/dashboard.grafana.app/v2/namespaces/default/dashboards/")
			resource, exists := fake.resources[uid]
			if !exists {
				writer.WriteHeader(http.StatusNotFound)
				return
			}
			writeFakeGrafanaJSON(writer, resource)
		case request.Method == http.MethodPost && request.URL.Path == "/apis/dashboard.grafana.app/v2/namespaces/default/dashboards":
			var resource map[string]interface{}
			_ = json.NewDecoder(request.Body).Decode(&resource)
			metadata, _ := resource["metadata"].(map[string]interface{})
			uid, _ := metadata["name"].(string)
			if _, exists := fake.resources[uid]; exists {
				writer.WriteHeader(http.StatusConflict)
				return
			}
			fake.resources[uid] = resource
			writer.WriteHeader(http.StatusCreated)
		case request.Method == http.MethodPut && strings.HasPrefix(request.URL.Path, "/apis/dashboard.grafana.app/v2/namespaces/default/dashboards/"):
			uid := strings.TrimPrefix(request.URL.Path, "/apis/dashboard.grafana.app/v2/namespaces/default/dashboards/")
			var resource map[string]interface{}
			_ = json.NewDecoder(request.Body).Decode(&resource)
			fake.resources[uid] = resource
			writer.WriteHeader(http.StatusOK)
		case request.Method == http.MethodGet && path == "org":
			writeFakeGrafanaJSON(writer, map[string]interface{}{"id": 1, "name": fake.organization})
		case request.Method == http.MethodPut && path == "org":
			var payload struct {
				Name string `json:"name"`
			}
			_ = json.NewDecoder(request.Body).Decode(&payload)
			fake.organization = payload.Name
			writeFakeGrafanaJSON(writer, map[string]interface{}{"message": "updated"})
		case request.Method == http.MethodGet && path == "search":
			writeFakeGrafanaJSON(writer, fake.search())
		case request.Method == http.MethodPost && path == "folders":
			var payload struct {
				UID   string `json:"uid"`
				Title string `json:"title"`
			}
			_ = json.NewDecoder(request.Body).Decode(&payload)
			if _, exists := fake.folders[payload.UID]; exists {
				writer.WriteHeader(http.StatusConflict)
				return
			}
			fake.folders[payload.UID] = payload.Title
			writeFakeGrafanaJSON(writer, map[string]interface{}{"uid": payload.UID})
		case request.Method == http.MethodPut && strings.HasPrefix(path, "folders/"):
			uid := strings.TrimPrefix(path, "folders/")
			var payload struct {
				Title string `json:"title"`
			}
			_ = json.NewDecoder(request.Body).Decode(&payload)
			fake.folders[uid] = payload.Title
			writeFakeGrafanaJSON(writer, map[string]interface{}{"uid": uid})
		case request.Method == http.MethodDelete && strings.HasPrefix(path, "folders/"):
			uid := strings.TrimPrefix(path, "folders/")
			for _, dashboard := range fake.dashboards {
				if dashboard.folder == uid {
					writer.WriteHeader(http.StatusConflict)
					return
				}
			}
			if _, exists := fake.folders[uid]; !exists {
				writer.WriteHeader(http.StatusNotFound)
				return
			}
			delete(fake.folders, uid)
			writeFakeGrafanaJSON(writer, map[string]interface{}{"message": "deleted"})
		case request.Method == http.MethodPost && path == "dashboards/db":
			var payload struct {
				Dashboard map[string]interface{} `json:"dashboard"`
				FolderUID string                 `json:"folderUid"`
			}
			_ = json.NewDecoder(request.Body).Decode(&payload)
			uid, _ := payload.Dashboard["uid"].(string)
			if status := fake.upsertFailures[uid]; status != 0 {
				writer.WriteHeader(status)
				return
			}
			if existing, exists := fake.dashboards[uid]; exists {
				payload.Dashboard["id"] = existing.value["id"]
			} else {
				payload.Dashboard["id"] = fake.nextID
				fake.nextID++
			}
			fake.dashboards[uid] = fakeGrafanaDashboard{value: payload.Dashboard, folder: payload.FolderUID}
			writeFakeGrafanaJSON(writer, map[string]interface{}{"uid": uid, "status": "success"})
		case request.Method == http.MethodGet && strings.HasPrefix(path, "dashboards/uid/"):
			uid := strings.TrimPrefix(path, "dashboards/uid/")
			dashboard, exists := fake.dashboards[uid]
			if !exists {
				resource, resourceExists := fake.resources[uid]
				if !resourceExists {
					writer.WriteHeader(http.StatusNotFound)
					return
				}
				metadata, _ := resource["metadata"].(map[string]interface{})
				annotations, _ := metadata["annotations"].(map[string]interface{})
				spec, _ := resource["spec"].(map[string]interface{})
				folder, _ := annotations["grafana.app/folder"].(string)
				dashboard = fakeGrafanaDashboard{
					value: map[string]interface{}{
						"id": fake.nextID, "uid": uid, "title": spec["title"], "version": json.Number("1"),
					},
					folder: folder,
				}
			}
			writeFakeGrafanaJSON(writer, map[string]interface{}{
				"dashboard": dashboard.value,
				"meta":      map[string]interface{}{"folderUid": dashboard.folder, "canSave": true, "canEdit": true},
			})
		case request.Method == http.MethodDelete && strings.HasPrefix(path, "dashboards/uid/"):
			uid := strings.TrimPrefix(path, "dashboards/uid/")
			if _, exists := fake.dashboards[uid]; !exists {
				writer.WriteHeader(http.StatusNotFound)
				return
			}
			delete(fake.dashboards, uid)
			writeFakeGrafanaJSON(writer, map[string]interface{}{"message": "deleted"})
		case request.Method == http.MethodPost && strings.HasPrefix(path, "user/stars/dashboard/uid/"):
			fake.starCalls++
			writeFakeGrafanaJSON(writer, map[string]interface{}{"message": "starred"})
		case request.Method == http.MethodGet && path == "org/preferences":
			writeFakeGrafanaJSON(writer, fake.organizationPreferences)
		case request.Method == http.MethodGet && path == "user/preferences":
			writeFakeGrafanaJSON(writer, fake.userPreferences)
		case request.Method == http.MethodPatch && path == "org/preferences":
			fake.organizationPrefs++
			var payload map[string]interface{}
			_ = json.NewDecoder(request.Body).Decode(&payload)
			for key, value := range payload {
				fake.organizationPreferences[key] = value
			}
			writeFakeGrafanaJSON(writer, map[string]interface{}{"message": "updated"})
		case request.Method == http.MethodPatch && path == "user/preferences":
			fake.userPrefs++
			var payload map[string]interface{}
			_ = json.NewDecoder(request.Body).Decode(&payload)
			for key, value := range payload {
				fake.userPreferences[key] = value
			}
			writeFakeGrafanaJSON(writer, map[string]interface{}{"message": "updated"})
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(fake.server.Close)
	return fake
}

func (f *fakeGrafanaServer) search() []map[string]interface{} {
	items := make([]map[string]interface{}, 0, len(f.folders)+len(f.dashboards)+len(f.resources))
	folders := make([]string, 0, len(f.folders))
	for uid := range f.folders {
		folders = append(folders, uid)
	}
	sort.Strings(folders)
	for _, uid := range folders {
		items = append(items, map[string]interface{}{"uid": uid, "title": f.folders[uid], "type": "dash-folder"})
	}
	dashboards := make([]string, 0, len(f.dashboards))
	for uid := range f.dashboards {
		dashboards = append(dashboards, uid)
	}
	sort.Strings(dashboards)
	for _, uid := range dashboards {
		item := f.dashboards[uid]
		items = append(items, map[string]interface{}{
			"uid": uid, "title": fmt.Sprint(item.value["title"]), "type": "dash-db",
			"folderUid": item.folder, "folderTitle": f.folders[item.folder], "url": "/d/" + uid,
		})
	}
	resources := make([]string, 0, len(f.resources))
	for uid := range f.resources {
		if _, exists := f.dashboards[uid]; !exists {
			resources = append(resources, uid)
		}
	}
	sort.Strings(resources)
	for _, uid := range resources {
		resource := f.resources[uid]
		metadata, _ := resource["metadata"].(map[string]interface{})
		annotations, _ := metadata["annotations"].(map[string]interface{})
		spec, _ := resource["spec"].(map[string]interface{})
		folder, _ := annotations["grafana.app/folder"].(string)
		items = append(items, map[string]interface{}{
			"uid": uid, "title": fmt.Sprint(spec["title"]), "type": "dash-db",
			"folderUid": folder, "folderTitle": f.folders[folder], "url": "/d/" + uid,
		})
	}
	return items
}

func (f *fakeGrafanaServer) seedDashboard(uid, folder string) {
	f.dashboards[uid] = fakeGrafanaDashboard{
		value:  map[string]interface{}{"id": f.nextID, "uid": uid, "title": uid, "version": json.Number("1")},
		folder: folder,
	}
	f.nextID++
}

func writeFakeGrafanaJSON(writer http.ResponseWriter, value interface{}) {
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(value)
}

func grafanaFakeOptions(t *testing.T, fake *fakeGrafanaServer, path string) GrafanaOptions {
	t.Helper()
	t.Setenv("GRAFANA_PASSWORD", "secret")
	return GrafanaOptions{Endpoint: fake.server.URL, Username: "admin", Path: path, HTTPClient: fake.server.Client()}
}

func TestPrepareGrafanaUsesControllerDefaultEndpoint(t *testing.T) {
	t.Setenv("GRAFANA_ENDPOINT", "")
	t.Setenv("GRAFANA_USERNAME", "")
	t.Setenv("GRAFANA_PASSWORD", "secret")
	prepared, err := prepareGrafana(GrafanaOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := prepared.client.Endpoint(); got != defaultGrafanaEndpoint {
		t.Fatalf("Grafana endpoint = %q, want %q", got, defaultGrafanaEndpoint)
	}
}

func TestGrafanaPreflightFailureReportsSafeEndpoint(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	endpoint := server.URL
	httpClient := server.Client()
	server.Close()

	root := filepathForGrafanaTest(t)
	writeGrafanaTestDashboard(t, filepath.Join(root, "example.json"), "example")
	t.Setenv("GRAFANA_PASSWORD", "secret")
	result := GrafanaInit(GrafanaOptions{
		Endpoint: endpoint, Username: "admin", Path: root, HTTPClient: httpClient,
	})
	if result.Code != output.CodeStyGrafanaNetworkFailed {
		t.Fatalf("result = %+v", result)
	}
	if !strings.Contains(result.Detail, "api/health") || !strings.Contains(result.Detail, endpoint) {
		t.Fatalf("failure detail = %q", result.Detail)
	}
	if strings.Contains(result.Detail, "secret") {
		t.Fatalf("failure detail leaked credential: %q", result.Detail)
	}
}

func TestGrafanaListReturnsAllDashboardsInStableOrder(t *testing.T) {
	fake := newFakeGrafanaServer(t)
	fake.folders["pgsql"] = "PGSQL"
	fake.seedDashboard("z-general", "")
	fake.seedDashboard("b-pgsql", "pgsql")
	fake.seedDashboard("a-pgsql", "pgsql")

	result := GrafanaList(grafanaFakeOptions(t, fake, ""))
	if !result.Success {
		t.Fatalf("result = %+v", result)
	}
	data, ok := result.Data.(*GrafanaListData)
	if !ok {
		t.Fatalf("data type = %T", result.Data)
	}
	if data.Kind != GrafanaListKind || data.Endpoint != fake.server.URL || data.Organization != "Main Org." || data.Count != 3 {
		t.Fatalf("data = %+v", data)
	}
	want := []string{"z-general", "a-pgsql", "b-pgsql"}
	for index, uid := range want {
		if data.Dashboards[index].UID != uid {
			t.Fatalf("dashboards = %+v", data.Dashboards)
		}
	}
	if item := data.Dashboards[1]; item.FolderUID != "pgsql" || item.FolderTitle != "PGSQL" || item.URL != "/d/a-pgsql" {
		t.Fatalf("folder dashboard = %+v", item)
	}
	text := data.Text()
	if !strings.Contains(text, "dashboards: 3") || !strings.Contains(text, "General") || !strings.Contains(text, "PGSQL") {
		t.Fatalf("text = %q", text)
	}
}

func TestGrafanaInitLoadsDashboardV2Resource(t *testing.T) {
	fake := newFakeGrafanaServer(t)
	root := filepathForGrafanaTest(t)
	writeGrafanaTestDashboard(t, filepath.Join(root, "pigsty.json"), "pigsty")
	filename := filepath.Join(root, "minio", "minio-instance.json")
	if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{
		"apiVersion":"dashboard.grafana.app/v2","kind":"Dashboard",
		"metadata":{"name":"minio-instance","namespace":"default","annotations":{"grafana.app/message":"tabs"},"generation":7},
		"spec":{"title":"MinIO Instance","layout":{"kind":"TabsLayout"}},
		"status":{"operatorStates":{"conversion":{"state":"success"}}}
	}`)
	if err := os.WriteFile(filename, raw, 0644); err != nil {
		t.Fatal(err)
	}
	result := GrafanaInit(grafanaFakeOptions(t, fake, root))
	if !result.Success {
		t.Fatalf("result = %+v", result)
	}
	resource, exists := fake.resources["minio-instance"]
	if !exists {
		t.Fatalf("schema v2 resource was not loaded; requests = %v", fake.requests)
	}
	if _, exists := resource["status"]; exists {
		t.Fatalf("server-managed status was sent: %#v", resource)
	}
	metadata := resource["metadata"].(map[string]interface{})
	annotations := metadata["annotations"].(map[string]interface{})
	spec := resource["spec"].(map[string]interface{})
	layout := spec["layout"].(map[string]interface{})
	if annotations["grafana.app/folder"] != "minio" || layout["kind"] != "TabsLayout" {
		t.Fatalf("resource = %#v", resource)
	}
	requests := strings.Join(fake.requests, "\n")
	if !strings.Contains(requests, "GET /apis/dashboard.grafana.app/v2/namespaces/default/dashboards/minio-instance") ||
		!strings.Contains(requests, "POST /apis/dashboard.grafana.app/v2/namespaces/default/dashboards") {
		t.Fatalf("schema v2 resource routes were not used: %v", fake.requests)
	}
}

func TestGrafanaInfoSeparatesHealthAndAuthentication(t *testing.T) {
	fake := newFakeGrafanaServer(t)
	fake.folders["pgsql"] = "PGSQL"
	fake.seedDashboard("pigsty", "")

	t.Run("health only", func(t *testing.T) {
		t.Setenv("GRAFANA_PASSWORD", "")
		before := len(fake.requests)
		result := GrafanaInfo(GrafanaOptions{Endpoint: fake.server.URL, HTTPClient: fake.server.Client()})
		if !result.Success {
			t.Fatalf("result = %+v", result)
		}
		data := result.Data.(*GrafanaInfoData)
		if data.Status != "healthy" || data.AuthStatus != "not_configured" || data.Version != "12.3.0" || data.Organization != "" {
			t.Fatalf("data = %+v", data)
		}
		if got := fake.requests[before:]; len(got) != 1 || got[0] != "GET health" {
			t.Fatalf("health-only requests = %v", got)
		}
	})

	t.Run("unhealthy database", func(t *testing.T) {
		t.Setenv("GRAFANA_PASSWORD", "")
		fake.healthDatabase = "failing"
		defer func() { fake.healthDatabase = "ok" }()
		before := len(fake.requests)
		result := GrafanaInfo(GrafanaOptions{Endpoint: fake.server.URL, HTTPClient: fake.server.Client()})
		if result.Code != output.CodeStyGrafanaPartialFailure {
			t.Fatalf("result = %+v", result)
		}
		data := result.Data.(*GrafanaInfoData)
		if data.Status != "unhealthy" || data.Database != "failing" {
			t.Fatalf("data = %+v", data)
		}
		if got := fake.requests[before:]; len(got) != 1 || got[0] != "GET health" {
			t.Fatalf("unhealthy requests = %v", got)
		}
	})

	t.Run("authenticated", func(t *testing.T) {
		result := GrafanaInfo(grafanaFakeOptions(t, fake, ""))
		if !result.Success {
			t.Fatalf("result = %+v", result)
		}
		data := result.Data.(*GrafanaInfoData)
		if data.AuthStatus != "authenticated" || data.Organization != "Main Org." || data.DashboardCount == nil || *data.DashboardCount != 1 || data.FolderCount == nil || *data.FolderCount != 1 {
			t.Fatalf("data = %+v", data)
		}
		if data.OrganizationPreferences == nil || data.OrganizationPreferences.Theme != "dark" ||
			data.UserPreferences == nil || data.UserPreferences.Language != "en-US" {
			t.Fatalf("preferences = org:%+v user:%+v", data.OrganizationPreferences, data.UserPreferences)
		}
		if text := data.Text(); !strings.Contains(text, "dashboards: 1") || !strings.Contains(text, "theme=dark") {
			t.Fatalf("text = %q", text)
		}
	})

	t.Run("rejected credential preserves health", func(t *testing.T) {
		result := GrafanaInfo(GrafanaOptions{
			Endpoint: fake.server.URL, Username: "admin", Password: "wrong", HTTPClient: fake.server.Client(),
		})
		if result.Code != output.CodeStyGrafanaPermissionDenied {
			t.Fatalf("result = %+v", result)
		}
		if strings.Contains(fmt.Sprint(result), "wrong") {
			t.Fatalf("result leaked raw --password: %+v", result)
		}
		data := result.Data.(*GrafanaInfoData)
		if data.Status != "healthy" || data.AuthStatus != "unauthorized" || data.Version != "12.3.0" {
			t.Fatalf("data = %+v", data)
		}
	})
}

func TestGrafanaExplicitConnectionBypassesBrokenInventory(t *testing.T) {
	fake := newFakeGrafanaServer(t)
	result := GrafanaInfo(GrafanaOptions{
		Endpoint: fake.server.URL, Username: "admin", Password: "secret",
		Inventory: filepath.Join(t.TempDir(), "missing.yml"), HTTPClient: fake.server.Client(),
	})
	if !result.Success {
		t.Fatalf("explicit connection depended on Inventory: %+v", result)
	}
	data := result.Data.(*GrafanaInfoData)
	if data.AuthStatus != "authenticated" || data.DashboardCount == nil || *data.DashboardCount != 0 || data.FolderCount == nil || *data.FolderCount != 0 {
		t.Fatalf("data = %+v", data)
	}
	raw, err := json.Marshal(data)
	if err != nil || !bytes.Contains(raw, []byte(`"dashboard_count":0`)) || !bytes.Contains(raw, []byte(`"folder_count":0`)) {
		t.Fatalf("zero-count JSON = %s err=%v", raw, err)
	}
}

func TestGrafanaInfoPreservesHealthWhenCredentialInventoryIsUnavailable(t *testing.T) {
	fake := newFakeGrafanaServer(t)
	result := GrafanaInfo(GrafanaOptions{
		Endpoint: fake.server.URL, Inventory: filepath.Join(t.TempDir(), "missing.yml"),
		HTTPClient: fake.server.Client(),
	})
	if result.Code != output.CodeStyGrafanaConfigInvalid {
		t.Fatalf("result = %+v", result)
	}
	data, ok := result.Data.(*GrafanaInfoData)
	if !ok || data.Status != "healthy" || data.Database != "ok" || data.Version != "12.3.0" || data.AuthStatus != "error" {
		t.Fatalf("data = %+v", result.Data)
	}
	if got := fake.requests; len(got) != 1 || got[0] != "GET health" {
		t.Fatalf("requests = %v", got)
	}
}

func TestGrafanaRejectsConflictingPasswordSources(t *testing.T) {
	result := GrafanaInfo(GrafanaOptions{Endpoint: "http://example.invalid", Password: "one", PasswordFile: "/tmp/two"})
	if result.Code != output.CodeStyGrafanaInvalidArgs || !strings.Contains(result.Detail, "mutually exclusive") {
		t.Fatalf("result = %+v", result)
	}
}

func TestGrafanaRejectsMultilineEnvironmentPasswordWithoutLeakingIt(t *testing.T) {
	t.Setenv("GRAFANA_PASSWORD", "secret\nsecond-line")
	result := GrafanaInfo(GrafanaOptions{Endpoint: "http://example.invalid"})
	if result.Code != output.CodeStyGrafanaInvalidArgs || !strings.Contains(result.Detail, "GRAFANA_PASSWORD") {
		t.Fatalf("result = %+v", result)
	}
	if strings.Contains(fmt.Sprint(result), "second-line") {
		t.Fatalf("result leaked environment password: %+v", result)
	}
}

func TestGrafanaDumpDirectoryIsCanonicalFixedPoint(t *testing.T) {
	fake := newFakeGrafanaServer(t)
	fake.folders["pgsql"] = "PGSQL"
	fake.seedDashboard("root-dashboard", "")
	fake.seedDashboard("folder-dashboard", "pgsql")
	fake.dashboards["folder-dashboard"] = fakeGrafanaDashboard{
		value: map[string]interface{}{
			"id": fake.nextID, "uid": "folder-dashboard", "title": "Folder Dashboard",
			"version": json.Number("99"), "description": strings.Repeat("compact", 100),
			"templating": map[string]interface{}{"list": []interface{}{
				map[string]interface{}{"name": "cluster", "current": map[string]interface{}{"text": "volatile", "value": "volatile"}},
			}},
		},
		folder: "pgsql",
	}
	destination := filepath.Join(t.TempDir(), "dashboard-snapshot")
	options := grafanaFakeOptions(t, fake, "")
	options.DumpDir = destination

	first := GrafanaDump(options)
	if !first.Success {
		t.Fatalf("first dump = %+v", first)
	}
	rootFile := filepath.Join(destination, "root-dashboard.json")
	folderFile := filepath.Join(destination, "pgsql", "folder-dashboard.json")
	rootFirst, err := os.ReadFile(rootFile)
	if err != nil {
		t.Fatal(err)
	}
	folderFirst, err := os.ReadFile(folderFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(folderFirst, []byte(`"current":{}`)) || bytes.Contains(folderFirst, []byte(`"version":99`)) {
		t.Fatalf("dump was not normalized:\n%s", folderFirst)
	}

	second := GrafanaDump(options)
	if !second.Success {
		t.Fatalf("second dump = %+v", second)
	}
	data := second.Data.(*GrafanaActionData)
	for _, result := range data.Results {
		if result.ObjectKind == "dashboard" && result.Status != "unchanged" {
			t.Fatalf("second dump result = %+v", result)
		}
	}
	rootSecond, _ := os.ReadFile(rootFile)
	folderSecond, _ := os.ReadFile(folderFile)
	if !bytes.Equal(rootFirst, rootSecond) || !bytes.Equal(folderFirst, folderSecond) {
		t.Fatal("repeated dump changed canonical dashboard bytes")
	}
}

func TestGrafanaDumpPreservesExistingDashboardV2Resource(t *testing.T) {
	fake := newFakeGrafanaServer(t)
	fake.folders["minio"] = "MINIO"
	fake.resources["minio-instance"] = map[string]interface{}{
		"apiVersion": grafanaDashboardV2API,
		"kind":       "Dashboard",
		"metadata": map[string]interface{}{
			"name": "minio-instance", "namespace": "default", "generation": json.Number("12"),
			"annotations": map[string]interface{}{"grafana.app/folder": "minio", "grafana.app/message": "tabs", "grafana.app/createdBy": "server"},
		},
		"spec": map[string]interface{}{
			"title": "MinIO Instance",
			"layout": map[string]interface{}{"kind": "TabsLayout", "spec": map[string]interface{}{
				"tabs": []interface{}{map[string]interface{}{"kind": "Tab", "spec": map[string]interface{}{"title": "Overview"}}},
			}},
		},
		"status": map[string]interface{}{"conversion": map[string]interface{}{"failed": false}},
	}
	root := filepath.Join(t.TempDir(), "grafana")
	target := filepath.Join(root, "minio", "minio-instance.json")
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		t.Fatal(err)
	}
	local := []byte(`{"apiVersion":"dashboard.grafana.app/v2","kind":"Dashboard","metadata":{"name":"minio-instance","namespace":"default"},"spec":{"title":"stale","layout":{"kind":"TabsLayout"}}}`)
	if err := os.WriteFile(target, local, 0644); err != nil {
		t.Fatal(err)
	}
	options := grafanaFakeOptions(t, fake, target)
	first := GrafanaDump(options)
	if !first.Success {
		t.Fatalf("first dump = %+v", first)
	}
	firstRaw, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	resource, err := decodeGrafanaDashboard(firstRaw)
	if err != nil {
		t.Fatal(err)
	}
	format, uid, namespace, err := grafanaDashboardIdentity(resource)
	if err != nil || format != grafanaDashboardV2 || uid != "minio-instance" || namespace != "default" {
		t.Fatalf("dumped identity format=%v uid=%q namespace=%q err=%v", format, uid, namespace, err)
	}
	if _, exists := resource["status"]; exists {
		t.Fatalf("dump retained server status: %#v", resource)
	}
	spec := resource["spec"].(map[string]interface{})
	layout := spec["layout"].(map[string]interface{})
	if spec["title"] != "MinIO Instance" || layout["kind"] != "TabsLayout" {
		t.Fatalf("dump flattened dashboard resource: %#v", resource)
	}

	second := GrafanaDump(options)
	if !second.Success {
		t.Fatalf("second dump = %+v", second)
	}
	data := second.Data.(*GrafanaActionData)
	if len(data.Results) != 1 || data.Results[0].Status != "unchanged" {
		t.Fatalf("second dump results = %+v", data.Results)
	}
	secondRaw, _ := os.ReadFile(target)
	if !bytes.Equal(firstRaw, secondRaw) {
		t.Fatal("repeated schema v2 dump changed canonical bytes")
	}
}

func TestGrafanaLoadFolderHasNoGlobalSideEffects(t *testing.T) {
	fake := newFakeGrafanaServer(t)
	root := filepathForGrafanaTest(t)
	folder := filepath.Join(root, "pgsql")
	writeGrafanaTestDashboard(t, filepath.Join(folder, "pgsql-overview.json"), "pgsql-overview")
	result := GrafanaLoad(grafanaFakeOptions(t, fake, folder))
	if !result.Success {
		t.Fatalf("result = %+v", result)
	}
	if fake.folders["pgsql"] != "PGSQL" || fake.dashboards["pgsql-overview"].folder != "pgsql" {
		t.Fatalf("folders=%v dashboards=%v", fake.folders, fake.dashboards)
	}
	if fake.organization != "Main Org." || fake.starCalls != 0 || fake.organizationPrefs != 0 || fake.userPrefs != 0 {
		t.Fatalf("unexpected bootstrap org=%q star=%d orgprefs=%d userprefs=%d", fake.organization, fake.starCalls, fake.organizationPrefs, fake.userPrefs)
	}
}

func TestGrafanaLoadRootPigstyHasNoBootstrapSideEffects(t *testing.T) {
	fake := newFakeGrafanaServer(t)
	root := filepathForGrafanaTest(t)
	writeGrafanaTestDashboard(t, filepath.Join(root, "pigsty.json"), "pigsty")
	result := GrafanaLoad(grafanaFakeOptions(t, fake, root))
	if !result.Success {
		t.Fatalf("result = %+v", result)
	}
	if _, exists := fake.dashboards["pigsty"]; !exists {
		t.Fatal("pigsty dashboard was not loaded")
	}
	if fake.organization != "Main Org." || fake.starCalls != 0 || fake.organizationPrefs != 0 || fake.userPrefs != 0 {
		t.Fatalf("load bootstrapped org=%q star=%d orgprefs=%d userprefs=%d", fake.organization, fake.starCalls, fake.organizationPrefs, fake.userPrefs)
	}
}

func TestGrafanaRootInitBootstrapsPigsty(t *testing.T) {
	fake := newFakeGrafanaServer(t)
	root := filepathForGrafanaTest(t)
	writeGrafanaTestDashboard(t, filepath.Join(root, "pigsty.json"), "pigsty")
	result := GrafanaInit(grafanaFakeOptions(t, fake, root))
	if !result.Success {
		t.Fatalf("result = %+v", result)
	}
	if fake.organization != "Pigsty" || fake.starCalls != 1 || fake.organizationPrefs != 1 || fake.userPrefs != 1 {
		t.Fatalf("bootstrap org=%q star=%d orgprefs=%d userprefs=%d", fake.organization, fake.starCalls, fake.organizationPrefs, fake.userPrefs)
	}
	if fake.organizationPreferences["homeDashboardUID"] != "pigsty" || fake.userPreferences["homeDashboardUID"] != "pigsty" {
		t.Fatalf("home preferences org=%v user=%v", fake.organizationPreferences, fake.userPreferences)
	}
	if fake.organizationPreferences["theme"] != "dark" || fake.organizationPreferences["language"] != "zh-Hans" ||
		fake.userPreferences["theme"] != "light" || fake.userPreferences["language"] != "en-US" {
		t.Fatalf("boot overwrote unrelated preferences org=%v user=%v", fake.organizationPreferences, fake.userPreferences)
	}
	loadIndex, renameIndex := -1, -1
	for index, request := range fake.requests {
		if request == "POST dashboards/db" && loadIndex < 0 {
			loadIndex = index
		}
		if request == "PUT org" && renameIndex < 0 {
			renameIndex = index
		}
	}
	if loadIndex < 0 || renameIndex < 0 || loadIndex >= renameIndex {
		t.Fatalf("init request order = %v", fake.requests)
	}
}

func TestGrafanaInitRejectsPartialScopeBeforeNetwork(t *testing.T) {
	fake := newFakeGrafanaServer(t)
	root := filepathForGrafanaTest(t)
	folder := filepath.Join(root, "pgsql")
	writeGrafanaTestDashboard(t, filepath.Join(folder, "overview.json"), "overview")
	result := GrafanaInit(grafanaFakeOptions(t, fake, folder))
	if result.Code != output.CodeStyGrafanaInvalidArgs || len(fake.requests) != 0 {
		t.Fatalf("result=%+v requests=%v", result, fake.requests)
	}
}

func TestGrafanaInitSkipsBootWhenLoadFails(t *testing.T) {
	fake := newFakeGrafanaServer(t)
	fake.upsertFailures["pigsty"] = http.StatusInternalServerError
	root := filepathForGrafanaTest(t)
	writeGrafanaTestDashboard(t, filepath.Join(root, "pigsty.json"), "pigsty")
	result := GrafanaInit(grafanaFakeOptions(t, fake, root))
	if result.Code != output.CodeStyGrafanaPartialFailure {
		t.Fatalf("result = %+v", result)
	}
	if fake.organization != "Main Org." || fake.starCalls != 0 || fake.organizationPrefs != 0 || fake.userPrefs != 0 {
		t.Fatalf("boot ran after load failure: org=%q star=%d orgprefs=%d userprefs=%d", fake.organization, fake.starCalls, fake.organizationPrefs, fake.userPrefs)
	}
	foundSkippedBoot := false
	for _, item := range result.Data.(*GrafanaActionData).Results {
		if item.Phase == "boot" && item.Action == "boot" && item.Status == "skipped" {
			foundSkippedBoot = true
		}
	}
	if !foundSkippedBoot {
		t.Fatalf("results = %+v", result.Data)
	}
}

func TestGrafanaBootRequiresPigstyBeforeMutation(t *testing.T) {
	fake := newFakeGrafanaServer(t)
	result := GrafanaBoot(grafanaFakeOptions(t, fake, ""))
	if result.Code != output.CodeStyGrafanaResourceMissing {
		t.Fatalf("result = %+v", result)
	}
	if fake.organization != "Main Org." || fake.starCalls != 0 || fake.organizationPrefs != 0 || fake.userPrefs != 0 {
		t.Fatalf("boot mutated without pigsty: org=%q star=%d orgprefs=%d userprefs=%d", fake.organization, fake.starCalls, fake.organizationPrefs, fake.userPrefs)
	}
}

func TestGrafanaLanguageAndStylePatchOnlySelectedFields(t *testing.T) {
	fake := newFakeGrafanaServer(t)
	options := grafanaFakeOptions(t, fake, "")
	language := GrafanaLang(options, "zh-cn")
	if !language.Success || language.Data.(*GrafanaActionData).Value != "zh-Hans" {
		t.Fatalf("language result = %+v", language)
	}
	if fake.organizationPreferences["language"] != "zh-Hans" || fake.userPreferences["language"] != "zh-Hans" ||
		fake.organizationPreferences["theme"] != "dark" || fake.userPreferences["theme"] != "light" {
		t.Fatalf("language preferences org=%v user=%v", fake.organizationPreferences, fake.userPreferences)
	}
	style := GrafanaStyle(options, "auto")
	if !style.Success || style.Data.(*GrafanaActionData).Value != "system" {
		t.Fatalf("style result = %+v", style)
	}
	if fake.organizationPreferences["theme"] != "system" || fake.userPreferences["theme"] != "system" ||
		fake.organizationPreferences["language"] != "zh-Hans" || fake.userPreferences["language"] != "zh-Hans" {
		t.Fatalf("style preferences org=%v user=%v", fake.organizationPreferences, fake.userPreferences)
	}
}

func TestGrafanaPreferenceValuesFailBeforeNetwork(t *testing.T) {
	fake := newFakeGrafanaServer(t)
	options := grafanaFakeOptions(t, fake, "")
	if result := GrafanaLang(options, "xx-future"); result.Code != output.CodeStyGrafanaInvalidArgs {
		t.Fatalf("language result = %+v", result)
	}
	if result := GrafanaStyle(options, "sepia"); result.Code != output.CodeStyGrafanaInvalidArgs {
		t.Fatalf("style result = %+v", result)
	}
	if len(fake.requests) != 0 {
		t.Fatalf("invalid preferences contacted Grafana: %v", fake.requests)
	}
}

func TestGrafanaCleanDeletesOnlyLocallyDeclaredUIDs(t *testing.T) {
	fake := newFakeGrafanaServer(t)
	fake.folders["pgsql"] = "PGSQL"
	fake.seedDashboard("owned", "pgsql")
	fake.seedDashboard("custom", "pgsql")
	root := filepathForGrafanaTest(t)
	folder := filepath.Join(root, "pgsql")
	writeGrafanaTestDashboard(t, filepath.Join(folder, "owned.json"), "owned")
	options := grafanaFakeOptions(t, fake, folder)
	options.DryRun = true
	plan := GrafanaClean(options)
	if !plan.Success {
		t.Fatalf("plan = %+v", plan)
	}
	data := plan.Data.(*GrafanaActionData)
	if len(data.Results) != 2 || data.Results[0].UID == "custom" || data.Results[1].UID == "custom" {
		t.Fatalf("plan results = %+v", data.Results)
	}
	options.DryRun = false
	options.Yes = true
	result := GrafanaClean(options)
	if !result.Success {
		t.Fatalf("result = %+v", result)
	}
	if _, exists := fake.dashboards["owned"]; exists {
		t.Fatal("owned dashboard was not deleted")
	}
	if _, exists := fake.dashboards["custom"]; !exists {
		t.Fatal("custom dashboard was deleted")
	}
	if _, exists := fake.folders["pgsql"]; !exists {
		t.Fatal("non-empty folder was deleted")
	}
}

func TestGrafanaCleanAllRequiresExplicitModeAndRemovesInstance(t *testing.T) {
	fake := newFakeGrafanaServer(t)
	fake.folders["pgsql"] = "PGSQL"
	fake.seedDashboard("one", "")
	fake.seedDashboard("two", "pgsql")
	options := grafanaFakeOptions(t, fake, "")
	options.All = true
	options.Yes = true
	result := GrafanaClean(options)
	if !result.Success {
		t.Fatalf("result = %+v", result)
	}
	if len(fake.dashboards) != 0 || len(fake.folders) != 0 {
		t.Fatalf("dashboards=%v folders=%v", fake.dashboards, fake.folders)
	}
	options.Path = "/tmp/grafana"
	if result := GrafanaClean(options); result.Code != output.CodeStyGrafanaInvalidArgs {
		t.Fatalf("--all with PATH result = %+v", result)
	}
}

func TestGrafanaCleanRejectsChangedInteractivePlan(t *testing.T) {
	fake := newFakeGrafanaServer(t)
	fake.folders["pgsql"] = "PGSQL"
	fake.seedDashboard("owned", "pgsql")
	root := filepathForGrafanaTest(t)
	folder := filepath.Join(root, "pgsql")
	writeGrafanaTestDashboard(t, filepath.Join(folder, "owned.json"), "owned")
	options := grafanaFakeOptions(t, fake, folder)
	plan := GrafanaClean(options)
	if plan.Code != output.CodeStyGrafanaConfirmationRequired {
		t.Fatalf("plan = %+v", plan)
	}
	if len(plan.NextActions) != 0 {
		t.Fatalf("destructive clean plan synthesized next actions: %+v", plan.NextActions)
	}
	digest := plan.Data.(*GrafanaActionData).PlanDigest
	delete(fake.dashboards, "owned")
	options.Yes = true
	options.PlanDigest = digest
	result := GrafanaClean(options)
	if result.Code != output.CodeStyGrafanaPlanChanged {
		t.Fatalf("changed plan result = %+v", result)
	}
}

func filepathForGrafanaTest(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "grafana")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	return root
}
