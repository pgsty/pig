package grafana

import (
	"context"
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
		if !ok || username != "admin" || password != "api-secret" {
			t.Errorf("basic auth = %q/%q/%v", username, password, ok)
		}
		switch request.URL.Path {
		case "/ui/api/health":
			_, _ = writer.Write([]byte(`{"database":"ok","version":"12.3.0","commit":"abc"}`))
		case "/ui/api/org":
			_, _ = writer.Write([]byte(`{"id":1,"name":"Pigsty"}`))
		case "/ui/api/search":
			if request.URL.Query().Get("limit") != "1000" || request.URL.Query().Get("page") != "1" {
				t.Errorf("search query = %s", request.URL.RawQuery)
			}
			_, _ = writer.Write([]byte(`[]`))
		case "/ui/api/datasources":
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
	if _, err := client.ListDatasources(context.Background()); err != nil {
		t.Fatal(err)
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
