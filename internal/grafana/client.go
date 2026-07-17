package grafana

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

const (
	defaultTimeout   = 10 * time.Second
	maxRequestBytes  = 16 << 20
	maxResponseBytes = 32 << 20
	searchPageSize   = 1000
	maxSearchPages   = 100
)

type Config struct {
	Endpoint   string
	Username   string
	Password   string
	HTTPClient *http.Client
}

type Client struct {
	base       *url.URL
	endpoint   string
	username   string
	password   string
	httpClient *http.Client
}

type Health struct {
	Database string `json:"database"`
	Version  string `json:"version"`
	Commit   string `json:"commit"`
}

type Organization struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type SearchItem struct {
	ID          int64  `json:"id"`
	UID         string `json:"uid"`
	Title       string `json:"title"`
	Type        string `json:"type"`
	URI         string `json:"uri"`
	URL         string `json:"url"`
	FolderID    int64  `json:"folderId"`
	FolderUID   string `json:"folderUid"`
	FolderTitle string `json:"folderTitle"`
	FolderURL   string `json:"folderUrl"`
	ParentUID   string `json:"parentUid"`
}

type DashboardEnvelope struct {
	Dashboard map[string]interface{} `json:"dashboard"`
	Meta      DashboardMeta          `json:"meta"`
}

type DashboardMeta struct {
	FolderID    int64  `json:"folderId"`
	FolderUID   string `json:"folderUid"`
	FolderTitle string `json:"folderTitle"`
	Provisioned bool   `json:"provisioned"`
	CanSave     bool   `json:"canSave"`
	CanEdit     bool   `json:"canEdit"`
}

type APIError struct {
	Method string
	Route  string
	Status int
}

func (e *APIError) Error() string {
	if e == nil {
		return "Grafana API error"
	}
	return fmt.Sprintf("Grafana %s %s returned HTTP %d", e.Method, e.Route, e.Status)
}

type RequestError struct {
	Method string
	Route  string
	Err    error
}

func (e *RequestError) Error() string {
	if e == nil {
		return "Grafana request failed"
	}
	return fmt.Sprintf("Grafana %s %s request failed", e.Method, e.Route)
}

func (e *RequestError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func NewClient(config Config) (*Client, error) {
	endpoint := strings.TrimSpace(config.Endpoint)
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse Grafana endpoint: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("Grafana endpoint must use http or https")
	}
	if parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("Grafana endpoint must be an origin and optional path prefix without credentials, query, or fragment")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""

	client := &http.Client{Timeout: defaultTimeout}
	if config.HTTPClient != nil {
		clone := *config.HTTPClient
		client = &clone
		if client.Timeout <= 0 {
			client.Timeout = defaultTimeout
		}
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return errors.New("Grafana API redirects are refused")
	}
	return &Client{
		base: parsed, endpoint: parsed.String(), username: config.Username,
		password: config.Password, httpClient: client,
	}, nil
}

func (c *Client) Endpoint() string {
	if c == nil {
		return ""
	}
	return c.endpoint
}

func (c *Client) Health(ctx context.Context) (Health, error) {
	var result Health
	err := c.do(ctx, http.MethodGet, "health", nil, nil, &result)
	return result, err
}

func (c *Client) CurrentOrganization(ctx context.Context) (Organization, error) {
	var result Organization
	err := c.do(ctx, http.MethodGet, "org", nil, nil, &result)
	return result, err
}

func (c *Client) Search(ctx context.Context) ([]SearchItem, error) {
	var result []SearchItem
	for pageNumber := 1; pageNumber <= maxSearchPages; pageNumber++ {
		query := url.Values{"limit": {strconv.Itoa(searchPageSize)}, "page": {strconv.Itoa(pageNumber)}}
		var pageItems []SearchItem
		if err := c.do(ctx, http.MethodGet, "search", query, nil, &pageItems); err != nil {
			return nil, err
		}
		result = append(result, pageItems...)
		if len(pageItems) < searchPageSize {
			return result, nil
		}
	}
	return nil, errors.New("Grafana search pagination exceeded the safety limit")
}

func (c *Client) GetDashboard(ctx context.Context, uid string) (DashboardEnvelope, error) {
	var result DashboardEnvelope
	err := c.do(ctx, http.MethodGet, "dashboards/uid/"+url.PathEscape(uid), nil, nil, &result)
	return result, err
}

func (c *Client) UpsertDashboard(ctx context.Context, dashboard map[string]interface{}, folderUID string) error {
	payload := map[string]interface{}{
		"dashboard": dashboard, "folderUid": folderUID, "overwrite": true,
		"message": "managed by pig sty dashboard",
	}
	return c.do(ctx, http.MethodPost, "dashboards/db", nil, payload, nil)
}

func (c *Client) DeleteDashboard(ctx context.Context, uid string) error {
	return c.do(ctx, http.MethodDelete, "dashboards/uid/"+url.PathEscape(uid), nil, nil, nil)
}

func (c *Client) ListDatasources(ctx context.Context) ([]map[string]interface{}, error) {
	var result []map[string]interface{}
	err := c.do(ctx, http.MethodGet, "datasources", nil, nil, &result)
	return result, err
}

func (c *Client) CreateDatasource(ctx context.Context, datasource map[string]interface{}) error {
	return c.do(ctx, http.MethodPost, "datasources", nil, datasource, nil)
}

func (c *Client) UpdateDatasource(ctx context.Context, uid string, datasource map[string]interface{}) error {
	return c.do(ctx, http.MethodPut, "datasources/uid/"+url.PathEscape(uid), nil, datasource, nil)
}

func (c *Client) DeleteDatasource(ctx context.Context, uid string) error {
	return c.do(ctx, http.MethodDelete, "datasources/uid/"+url.PathEscape(uid), nil, nil, nil)
}

func (c *Client) CreateFolder(ctx context.Context, uid, title string) error {
	return c.do(ctx, http.MethodPost, "folders", nil, map[string]interface{}{"uid": uid, "title": title}, nil)
}

func (c *Client) UpdateFolder(ctx context.Context, uid, title string) error {
	return c.do(ctx, http.MethodPut, "folders/"+url.PathEscape(uid), nil, map[string]interface{}{"title": title, "overwrite": true}, nil)
}

func (c *Client) DeleteFolder(ctx context.Context, uid string) error {
	return c.do(ctx, http.MethodDelete, "folders/"+url.PathEscape(uid), nil, nil, nil)
}

func (c *Client) do(ctx context.Context, method, route string, query url.Values, payload, destination interface{}) error {
	if c == nil || c.base == nil || c.httpClient == nil {
		return errors.New("Grafana client is not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	requestURL := *c.base
	requestURL.Path = path.Join(c.base.Path, "api", route)
	if strings.HasSuffix(route, "/") {
		requestURL.Path += "/"
	}
	requestURL.RawQuery = query.Encode()

	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encode Grafana request: %w", err)
		}
		if len(encoded) > maxRequestBytes {
			return fmt.Errorf("Grafana request exceeds %d bytes", maxRequestBytes)
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, requestURL.String(), body)
	if err != nil {
		return fmt.Errorf("create Grafana request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if c.username != "" || c.password != "" {
		request.SetBasicAuth(c.username, c.password)
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return &RequestError{Method: method, Route: "api/" + route, Err: err}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxResponseBytes))
		return &APIError{Method: method, Route: "api/" + route, Status: response.StatusCode}
	}
	if destination == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxResponseBytes))
		return nil
	}
	limited := io.LimitReader(response.Body, maxResponseBytes+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return &RequestError{Method: method, Route: "api/" + route, Err: err}
	}
	if len(raw) > maxResponseBytes {
		return fmt.Errorf("Grafana response exceeds %d bytes", maxResponseBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode Grafana response: %w", err)
	}
	return nil
}

func IsNotFound(err error) bool {
	var apiError *APIError
	return errors.As(err, &apiError) && apiError.Status == http.StatusNotFound
}

func StatusCode(err error) int {
	var apiError *APIError
	if errors.As(err, &apiError) {
		return apiError.Status
	}
	return 0
}
