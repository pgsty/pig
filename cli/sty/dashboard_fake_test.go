package sty

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
)

type fakeGrafanaDashboard struct {
	value       map[string]interface{}
	folderUID   string
	provisioned bool
	version     int
}

type fakeGrafana struct {
	t           *testing.T
	server      *httptest.Server
	mu          sync.Mutex
	username    string
	password    string
	dashboards  map[string]*fakeGrafanaDashboard
	folders     map[string]string
	datasources map[string]map[string]interface{}
	dsSecrets   map[string]map[string]string
	failures    map[string]int
	mutations   map[string]int
}

func newFakeGrafana(t *testing.T) *fakeGrafana {
	t.Helper()
	fake := &fakeGrafana{
		t: t, username: "admin", password: "api-secret",
		dashboards: make(map[string]*fakeGrafanaDashboard), folders: make(map[string]string),
		datasources: make(map[string]map[string]interface{}), dsSecrets: make(map[string]map[string]string),
		failures: make(map[string]int), mutations: make(map[string]int),
	}
	fake.server = httptest.NewServer(http.HandlerFunc(fake.handle))
	t.Cleanup(fake.server.Close)
	return fake
}

func (f *fakeGrafana) handle(writer http.ResponseWriter, request *http.Request) {
	if request.URL.Path != "/api/health" {
		username, password, ok := request.BasicAuth()
		if !ok || username != f.username || password != f.password {
			writeFakeGrafanaJSON(writer, http.StatusUnauthorized, map[string]interface{}{"message": "invalid password api-secret"})
			return
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if status := f.failures[request.Method+" "+request.URL.Path]; status != 0 {
		writeFakeGrafanaJSON(writer, status, map[string]interface{}{"message": "forced failure with server-secret"})
		return
	}
	switch {
	case request.Method == http.MethodGet && request.URL.Path == "/api/health":
		writeFakeGrafanaJSON(writer, http.StatusOK, map[string]interface{}{"database": "ok", "version": "12.3.0", "commit": "fake"})
	case request.Method == http.MethodGet && request.URL.Path == "/api/org":
		writeFakeGrafanaJSON(writer, http.StatusOK, map[string]interface{}{"id": 1, "name": "Pigsty"})
	case request.Method == http.MethodGet && request.URL.Path == "/api/search":
		f.handleSearch(writer, request.URL.Query())
	case request.Method == http.MethodGet && request.URL.Path == "/api/datasources":
		f.handleListDatasources(writer)
	case request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/api/dashboards/uid/"):
		f.handleGetDashboard(writer, strings.TrimPrefix(request.URL.Path, "/api/dashboards/uid/"))
	case request.Method == http.MethodPost && request.URL.Path == "/api/dashboards/db":
		f.handleUpsertDashboard(writer, request)
	case request.Method == http.MethodDelete && strings.HasPrefix(request.URL.Path, "/api/dashboards/uid/"):
		f.handleDeleteDashboard(writer, strings.TrimPrefix(request.URL.Path, "/api/dashboards/uid/"))
	case request.Method == http.MethodPost && request.URL.Path == "/api/folders":
		f.handleCreateFolder(writer, request)
	case request.Method == http.MethodPut && strings.HasPrefix(request.URL.Path, "/api/folders/"):
		f.handleUpdateFolder(writer, request, strings.TrimPrefix(request.URL.Path, "/api/folders/"))
	case request.Method == http.MethodDelete && strings.HasPrefix(request.URL.Path, "/api/folders/"):
		f.handleDeleteFolder(writer, strings.TrimPrefix(request.URL.Path, "/api/folders/"))
	case request.Method == http.MethodPost && request.URL.Path == "/api/datasources":
		f.handleCreateDatasource(writer, request)
	case request.Method == http.MethodPut && strings.HasPrefix(request.URL.Path, "/api/datasources/uid/"):
		f.handleUpdateDatasource(writer, request, strings.TrimPrefix(request.URL.Path, "/api/datasources/uid/"))
	case request.Method == http.MethodDelete && strings.HasPrefix(request.URL.Path, "/api/datasources/uid/"):
		f.handleDeleteDatasource(writer, strings.TrimPrefix(request.URL.Path, "/api/datasources/uid/"))
	default:
		writeFakeGrafanaJSON(writer, http.StatusNotFound, map[string]interface{}{"message": "not found"})
	}
}

func (f *fakeGrafana) handleSearch(writer http.ResponseWriter, query url.Values) {
	var items []map[string]interface{}
	for uid, title := range f.folders {
		items = append(items, map[string]interface{}{"id": len(items) + 1, "uid": uid, "title": title, "type": "dash-folder", "url": "/dashboards/f/" + uid})
	}
	for uid, dashboard := range f.dashboards {
		title := dashboardString(dashboard.value["title"])
		item := map[string]interface{}{"id": len(items) + 1, "uid": uid, "title": title, "type": "dash-db", "url": "/d/" + uid}
		if dashboard.folderUID != "" {
			item["folderUid"] = dashboard.folderUID
			item["folderTitle"] = f.folders[dashboard.folderUID]
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		left := dashboardString(items[i]["type"]) + "\x00" + dashboardString(items[i]["uid"])
		right := dashboardString(items[j]["type"]) + "\x00" + dashboardString(items[j]["uid"])
		return left < right
	})
	limit, _ := strconv.Atoi(query.Get("limit"))
	page, _ := strconv.Atoi(query.Get("page"))
	if limit <= 0 {
		limit = len(items) + 1
	}
	if page <= 0 {
		page = 1
	}
	start := (page - 1) * limit
	if start >= len(items) {
		items = nil
	} else {
		end := start + limit
		if end > len(items) {
			end = len(items)
		}
		items = items[start:end]
	}
	writeFakeGrafanaJSON(writer, http.StatusOK, items)
}

func (f *fakeGrafana) handleGetDashboard(writer http.ResponseWriter, uid string) {
	dashboard, exists := f.dashboards[uid]
	if !exists {
		writeFakeGrafanaJSON(writer, http.StatusNotFound, map[string]interface{}{"message": "missing"})
		return
	}
	value := cloneDashboardMapForFake(f.t, dashboard.value)
	value["id"] = 42
	value["version"] = dashboard.version
	meta := map[string]interface{}{
		"folderUid": dashboard.folderUID, "folderTitle": f.folders[dashboard.folderUID],
		"provisioned": dashboard.provisioned, "canSave": !dashboard.provisioned, "canEdit": !dashboard.provisioned,
	}
	writeFakeGrafanaJSON(writer, http.StatusOK, map[string]interface{}{"dashboard": value, "meta": meta})
}

func (f *fakeGrafana) handleUpsertDashboard(writer http.ResponseWriter, request *http.Request) {
	payload := decodeFakeGrafanaBody(f.t, request)
	dashboard, ok := payload["dashboard"].(map[string]interface{})
	if !ok {
		writeFakeGrafanaJSON(writer, http.StatusBadRequest, map[string]interface{}{"message": "dashboard required"})
		return
	}
	uid := dashboardString(dashboard["uid"])
	if uid == "" {
		writeFakeGrafanaJSON(writer, http.StatusBadRequest, map[string]interface{}{"message": "uid required"})
		return
	}
	if existing := f.dashboards[uid]; existing != nil && existing.provisioned {
		writeFakeGrafanaJSON(writer, http.StatusPreconditionFailed, map[string]interface{}{"message": "provisioned dashboard"})
		return
	}
	version := 1
	if existing := f.dashboards[uid]; existing != nil {
		version = existing.version + 1
	}
	delete(dashboard, "id")
	f.dashboards[uid] = &fakeGrafanaDashboard{value: dashboard, folderUID: dashboardString(payload["folderUid"]), version: version}
	f.mutations["dashboard:"+uid]++
	writeFakeGrafanaJSON(writer, http.StatusOK, map[string]interface{}{"status": "success", "uid": uid, "version": version})
}

func (f *fakeGrafana) handleDeleteDashboard(writer http.ResponseWriter, uid string) {
	dashboard, exists := f.dashboards[uid]
	if !exists {
		writeFakeGrafanaJSON(writer, http.StatusNotFound, map[string]interface{}{"message": "missing"})
		return
	}
	if dashboard.provisioned {
		writeFakeGrafanaJSON(writer, http.StatusPreconditionFailed, map[string]interface{}{"message": "provisioned dashboard"})
		return
	}
	delete(f.dashboards, uid)
	f.mutations["dashboard:"+uid]++
	writeFakeGrafanaJSON(writer, http.StatusOK, map[string]interface{}{"message": "deleted"})
}

func (f *fakeGrafana) handleCreateFolder(writer http.ResponseWriter, request *http.Request) {
	payload := decodeFakeGrafanaBody(f.t, request)
	uid, title := dashboardString(payload["uid"]), dashboardString(payload["title"])
	if _, exists := f.folders[uid]; exists {
		writeFakeGrafanaJSON(writer, http.StatusConflict, map[string]interface{}{"message": "exists"})
		return
	}
	f.folders[uid] = title
	f.mutations["folder:"+uid]++
	writeFakeGrafanaJSON(writer, http.StatusOK, map[string]interface{}{"uid": uid, "title": title})
}

func (f *fakeGrafana) handleUpdateFolder(writer http.ResponseWriter, request *http.Request, uid string) {
	if _, exists := f.folders[uid]; !exists {
		writeFakeGrafanaJSON(writer, http.StatusNotFound, map[string]interface{}{"message": "missing"})
		return
	}
	payload := decodeFakeGrafanaBody(f.t, request)
	f.folders[uid] = dashboardString(payload["title"])
	f.mutations["folder:"+uid]++
	writeFakeGrafanaJSON(writer, http.StatusOK, map[string]interface{}{"uid": uid})
}

func (f *fakeGrafana) handleDeleteFolder(writer http.ResponseWriter, uid string) {
	if _, exists := f.folders[uid]; !exists {
		writeFakeGrafanaJSON(writer, http.StatusNotFound, map[string]interface{}{"message": "missing"})
		return
	}
	for _, dashboard := range f.dashboards {
		if dashboard.folderUID == uid {
			writeFakeGrafanaJSON(writer, http.StatusBadRequest, map[string]interface{}{"message": "folder not empty"})
			return
		}
	}
	delete(f.folders, uid)
	f.mutations["folder:"+uid]++
	writeFakeGrafanaJSON(writer, http.StatusOK, map[string]interface{}{"message": "deleted"})
}

func (f *fakeGrafana) handleListDatasources(writer http.ResponseWriter) {
	items := make([]map[string]interface{}, 0, len(f.datasources))
	for uid, datasource := range f.datasources {
		item := cloneDashboardMapForFake(f.t, datasource)
		item["uid"] = uid
		if secrets := f.dsSecrets[uid]; len(secrets) > 0 {
			fields := make(map[string]interface{}, len(secrets))
			for name := range secrets {
				fields[name] = true
			}
			item["secureJsonFields"] = fields
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return dashboardString(items[i]["uid"]) < dashboardString(items[j]["uid"]) })
	writeFakeGrafanaJSON(writer, http.StatusOK, items)
}

func (f *fakeGrafana) handleCreateDatasource(writer http.ResponseWriter, request *http.Request) {
	payload := decodeFakeGrafanaBody(f.t, request)
	uid := dashboardString(payload["uid"])
	if _, exists := f.datasources[uid]; exists {
		writeFakeGrafanaJSON(writer, http.StatusConflict, map[string]interface{}{"message": "exists"})
		return
	}
	f.storeDatasource(uid, payload, false)
	f.mutations["datasource:"+uid]++
	writeFakeGrafanaJSON(writer, http.StatusOK, map[string]interface{}{"message": "created", "uid": uid})
}

func (f *fakeGrafana) handleUpdateDatasource(writer http.ResponseWriter, request *http.Request, uid string) {
	existing, exists := f.datasources[uid]
	if !exists {
		writeFakeGrafanaJSON(writer, http.StatusNotFound, map[string]interface{}{"message": "missing"})
		return
	}
	if readOnly, _ := existing["readOnly"].(bool); readOnly {
		writeFakeGrafanaJSON(writer, http.StatusForbidden, map[string]interface{}{"message": "provisioned datasource"})
		return
	}
	payload := decodeFakeGrafanaBody(f.t, request)
	f.storeDatasource(uid, payload, true)
	f.mutations["datasource:"+uid]++
	writeFakeGrafanaJSON(writer, http.StatusOK, map[string]interface{}{"message": "updated", "uid": uid})
}

func (f *fakeGrafana) handleDeleteDatasource(writer http.ResponseWriter, uid string) {
	datasource, exists := f.datasources[uid]
	if !exists {
		writeFakeGrafanaJSON(writer, http.StatusNotFound, map[string]interface{}{"message": "missing"})
		return
	}
	if readOnly, _ := datasource["readOnly"].(bool); readOnly {
		writeFakeGrafanaJSON(writer, http.StatusForbidden, map[string]interface{}{"message": "provisioned datasource"})
		return
	}
	delete(f.datasources, uid)
	delete(f.dsSecrets, uid)
	f.mutations["datasource:"+uid]++
	writeFakeGrafanaJSON(writer, http.StatusOK, map[string]interface{}{"message": "deleted"})
}

func (f *fakeGrafana) storeDatasource(uid string, payload map[string]interface{}, mergeSecrets bool) {
	clone := cloneDashboardMapForFake(f.t, payload)
	secure, _ := clone["secureJsonData"].(map[string]interface{})
	delete(clone, "secureJsonData")
	if !mergeSecrets || f.dsSecrets[uid] == nil {
		f.dsSecrets[uid] = make(map[string]string)
	}
	for name, value := range secure {
		f.dsSecrets[uid][name] = fmt.Sprint(value)
	}
	f.datasources[uid] = clone
}

func (f *fakeGrafana) seedDashboard(uid, title, folder string, provisioned bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dashboards[uid] = &fakeGrafanaDashboard{
		value:     map[string]interface{}{"uid": uid, "title": title, "id": nil, "version": json.Number("1"), "panels": []interface{}{}},
		folderUID: folder, provisioned: provisioned, version: 7,
	}
}

func (f *fakeGrafana) seedFolder(uid, title string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.folders[uid] = title
}

func (f *fakeGrafana) seedDatasource(value map[string]interface{}, secrets map[string]string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	uid := dashboardString(value["uid"])
	f.datasources[uid] = cloneDashboardMapForFake(f.t, value)
	if secrets != nil {
		f.dsSecrets[uid] = make(map[string]string, len(secrets))
		for name, secret := range secrets {
			f.dsSecrets[uid][name] = secret
		}
	}
}

func (f *fakeGrafana) mutationCount(key string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.mutations[key]
}

func (f *fakeGrafana) hasDashboard(uid string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, exists := f.dashboards[uid]
	return exists
}

func (f *fakeGrafana) hasDatasource(uid string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, exists := f.datasources[uid]
	return exists
}

func (f *fakeGrafana) hasFolder(uid string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, exists := f.folders[uid]
	return exists
}

func (f *fakeGrafana) dashboardFolder(uid string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if dashboard := f.dashboards[uid]; dashboard != nil {
		return dashboard.folderUID
	}
	return ""
}

func (f *fakeGrafana) setDashboardField(uid, name string, value interface{}) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if dashboard := f.dashboards[uid]; dashboard != nil {
		dashboard.value[name] = value
	}
}

func (f *fakeGrafana) setDatasourceField(uid, name string, value interface{}) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if datasource := f.datasources[uid]; datasource != nil {
		datasource[name] = value
	}
}

func (f *fakeGrafana) datasourceSecret(uid, name string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.dsSecrets[uid][name]
}

func writeFakeGrafanaJSON(writer http.ResponseWriter, status int, value interface{}) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func decodeFakeGrafanaBody(t *testing.T, request *http.Request) map[string]interface{} {
	t.Helper()
	payload := make(map[string]interface{})
	decoder := json.NewDecoder(request.Body)
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		t.Fatalf("decode fake Grafana request: %v", err)
	}
	return payload
}

func cloneDashboardMapForFake(t *testing.T, value map[string]interface{}) map[string]interface{} {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	clone := make(map[string]interface{})
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if err := decoder.Decode(&clone); err != nil {
		t.Fatal(err)
	}
	return clone
}
