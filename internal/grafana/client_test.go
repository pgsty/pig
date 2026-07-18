package grafana

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientUsesPrefixBasicAuthAndBoundedRoutes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		username, password, ok := request.BasicAuth()
		switch request.URL.Path {
		case "/ui/api/health":
			if ok {
				t.Errorf("health request unexpectedly included basic auth for %q", username)
			}
			_, _ = writer.Write([]byte(`{"database":"ok","version":"12.3.0","commit":"abc"}`))
		case "/ui/api/org":
			if !ok || username != "admin" || password != "api-secret" {
				t.Errorf("basic auth = %q/%q/%v", username, password, ok)
			}
			_, _ = writer.Write([]byte(`{"id":1,"name":"Pigsty"}`))
		case "/ui/api/search":
			if !ok || username != "admin" || password != "api-secret" {
				t.Errorf("basic auth = %q/%q/%v", username, password, ok)
			}
			if request.URL.Query().Get("limit") != "1000" || request.URL.Query().Get("page") != "1" {
				t.Errorf("search query = %s", request.URL.RawQuery)
			}
			_, _ = writer.Write([]byte(`[]`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	client, err := NewClient(Config{Endpoint: server.URL + "/ui/", Username: "admin", Password: "api-secret", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if client.Endpoint() != server.URL+"/ui" {
		t.Fatalf("endpoint = %q", client.Endpoint())
	}
	health, err := client.Health(context.Background())
	if err != nil || health.Version != "12.3.0" {
		t.Fatalf("health=%+v err=%v", health, err)
	}
	organization, err := client.CurrentOrganization(context.Background())
	if err != nil || organization.ID != 1 {
		t.Fatalf("organization=%+v err=%v", organization, err)
	}
	if _, err := client.Search(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestClientDashboardBootstrapRoutes(t *testing.T) {
	var routes []string
	var payloads []map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		routes = append(routes, request.Method+" "+request.URL.Path)
		if request.Method == http.MethodPatch || request.Method == http.MethodPut {
			var payload map[string]interface{}
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Errorf("decode payload: %v", err)
			}
			payloads = append(payloads, payload)
		}
		if request.URL.Path == "/ui/api/user/stars/dashboard/uid/pigsty" {
			writer.WriteHeader(http.StatusPreconditionFailed)
			return
		}
		if request.Method == http.MethodGet {
			_, _ = writer.Write([]byte(`{"homeDashboardUID":"pigsty","theme":"dark","language":"zh-Hans"}`))
			return
		}
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	client, err := NewClient(Config{Endpoint: server.URL + "/ui", Username: "admin", Password: "secret", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := client.UpdateCurrentOrganization(ctx, "Pigsty"); err != nil {
		t.Fatal(err)
	}
	if err := client.StarDashboard(ctx, "pigsty"); err != nil {
		t.Fatalf("already-starred dashboard should be idempotent: %v", err)
	}
	orgPreference, err := client.CurrentOrganizationPreferences(ctx)
	if err != nil || orgPreference.Language != "zh-Hans" {
		t.Fatalf("organization preference=%+v err=%v", orgPreference, err)
	}
	userPreference, err := client.CurrentUserPreferences(ctx)
	if err != nil || userPreference.Theme != "dark" {
		t.Fatalf("user preference=%+v err=%v", userPreference, err)
	}
	homeUID := "pigsty"
	if err := client.PatchOrganizationPreferences(ctx, PreferencesPatch{HomeDashboardUID: &homeUID}); err != nil {
		t.Fatal(err)
	}
	if err := client.PatchUserPreferences(ctx, PreferencesPatch{HomeDashboardUID: &homeUID}); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"PUT /ui/api/org",
		"POST /ui/api/user/stars/dashboard/uid/pigsty",
		"GET /ui/api/org/preferences",
		"GET /ui/api/user/preferences",
		"PATCH /ui/api/org/preferences",
		"PATCH /ui/api/user/preferences",
	}
	if fmt.Sprint(routes) != fmt.Sprint(want) {
		t.Fatalf("routes = %v, want %v", routes, want)
	}
	if len(payloads) != 3 || fmt.Sprint(payloads[1]) != "map[homeDashboardUID:pigsty]" || fmt.Sprint(payloads[2]) != "map[homeDashboardUID:pigsty]" {
		t.Fatalf("payloads = %#v", payloads)
	}
}

func TestClientErrorsNeverCopyResponseBodiesOrCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = writer.Write([]byte(`{"message":"password=server-secret token=leaked"}`))
	}))
	defer server.Close()
	client, err := NewClient(Config{Endpoint: server.URL, Username: "admin", Password: "client-secret", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.CurrentOrganization(context.Background())
	var apiError *APIError
	if !errors.As(err, &apiError) || apiError.Status != http.StatusUnauthorized {
		t.Fatalf("error = %#v", err)
	}
	message := fmt.Sprint(err)
	for _, secret := range []string{"server-secret", "leaked", "client-secret"} {
		if strings.Contains(message, secret) {
			t.Fatalf("API error leaked %q: %q", secret, message)
		}
	}
}

func TestClientRefusesRedirectsAndUnsafeEndpoints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, "https://example.invalid/steal", http.StatusFound)
	}))
	defer server.Close()
	client, err := NewClient(Config{Endpoint: server.URL, Username: "admin", Password: "secret", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Health(context.Background()); err == nil {
		t.Fatal("redirect was followed")
	}
	for _, endpoint := range []string{"file:///tmp/grafana", "http://admin:secret@example.com", "http://example.com?token=secret", "//example.com"} {
		if _, err := NewClient(Config{Endpoint: endpoint}); err == nil {
			t.Errorf("unsafe endpoint accepted: %s", endpoint)
		}
	}
}
