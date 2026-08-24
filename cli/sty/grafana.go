package sty

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"pig/internal/grafana"
	"pig/internal/output"
	"pig/inventory"
)

const (
	GrafanaInfoKind  = "pig.sty.grafana.info/v1"
	GrafanaListKind  = "pig.sty.grafana.list/v1"
	GrafanaBootKind  = "pig.sty.grafana.boot/v1"
	GrafanaLoadKind  = "pig.sty.grafana.load/v1"
	GrafanaInitKind  = "pig.sty.grafana.init/v1"
	GrafanaDumpKind  = "pig.sty.grafana.dump/v1"
	GrafanaCleanKind = "pig.sty.grafana.clean/v1"
	GrafanaLangKind  = "pig.sty.grafana.lang/v1"
	GrafanaStyleKind = "pig.sty.grafana.style/v1"

	defaultGrafanaEndpoint  = "http://i.pigsty/ui"
	maxGrafanaPasswordBytes = 64 << 10
	grafanaLanguageSet      = "Grafana 13.1.0"
)

// GrafanaOptions contains Grafana connection details and one local dashboard
// scope. Secrets are consumed while preparing the client and are never copied
// into result DTOs.
type GrafanaOptions struct {
	Context      context.Context
	Home         string
	Inventory    string
	Path         string
	DumpDir      string
	Endpoint     string
	Username     string
	Password     string
	PasswordFile string
	All          bool
	Yes          bool
	DryRun       bool
	PlanDigest   string
	HTTPClient   *http.Client
}

type GrafanaObjectResult struct {
	Phase      string `json:"phase,omitempty" yaml:"phase,omitempty"`
	ObjectKind string `json:"object_kind" yaml:"object_kind"`
	UID        string `json:"uid" yaml:"uid"`
	FolderUID  string `json:"folder_uid,omitempty" yaml:"folder_uid,omitempty"`
	Action     string `json:"action" yaml:"action"`
	Status     string `json:"status" yaml:"status"`
	Message    string `json:"message,omitempty" yaml:"message,omitempty"`
	ResultCode int    `json:"result_code,omitempty" yaml:"result_code,omitempty"`
}

type GrafanaPreferenceInfo struct {
	HomeDashboardUID string `json:"home_dashboard_uid,omitempty" yaml:"home_dashboard_uid,omitempty"`
	Theme            string `json:"theme,omitempty" yaml:"theme,omitempty"`
	Language         string `json:"language,omitempty" yaml:"language,omitempty"`
	Timezone         string `json:"timezone,omitempty" yaml:"timezone,omitempty"`
	WeekStart        string `json:"week_start,omitempty" yaml:"week_start,omitempty"`
}

type GrafanaInfoData struct {
	Kind                    string                 `json:"kind" yaml:"kind"`
	Endpoint                string                 `json:"endpoint" yaml:"endpoint"`
	Status                  string                 `json:"status" yaml:"status"`
	Database                string                 `json:"database,omitempty" yaml:"database,omitempty"`
	Version                 string                 `json:"version,omitempty" yaml:"version,omitempty"`
	Commit                  string                 `json:"commit,omitempty" yaml:"commit,omitempty"`
	AuthStatus              string                 `json:"auth_status" yaml:"auth_status"`
	Username                string                 `json:"username,omitempty" yaml:"username,omitempty"`
	OrganizationID          int64                  `json:"organization_id,omitempty" yaml:"organization_id,omitempty"`
	Organization            string                 `json:"organization,omitempty" yaml:"organization,omitempty"`
	DashboardCount          *int                   `json:"dashboard_count,omitempty" yaml:"dashboard_count,omitempty"`
	FolderCount             *int                   `json:"folder_count,omitempty" yaml:"folder_count,omitempty"`
	OrganizationPreferences *GrafanaPreferenceInfo `json:"organization_preferences,omitempty" yaml:"organization_preferences,omitempty"`
	UserPreferences         *GrafanaPreferenceInfo `json:"user_preferences,omitempty" yaml:"user_preferences,omitempty"`
}

func (d *GrafanaInfoData) Text() string {
	if d == nil {
		return ""
	}
	lines := []string{
		"  endpoint: " + d.Endpoint,
		"  status: " + emptyGrafanaText(d.Status),
		"  database: " + emptyGrafanaText(d.Database),
		"  version: " + emptyGrafanaText(d.Version),
		"  commit: " + emptyGrafanaText(d.Commit),
		"  authentication: " + emptyGrafanaText(d.AuthStatus),
	}
	if d.Username != "" {
		lines = append(lines, "  username: "+d.Username)
	}
	if d.Organization != "" {
		lines = append(lines, "  organization: "+d.Organization)
	}
	if d.DashboardCount != nil {
		lines = append(lines, fmt.Sprintf("  dashboards: %d", *d.DashboardCount))
	}
	if d.FolderCount != nil {
		lines = append(lines, fmt.Sprintf("  folders: %d", *d.FolderCount))
	}
	if d.OrganizationPreferences != nil {
		lines = append(lines, "  org preferences: "+formatGrafanaPreferenceInfo(*d.OrganizationPreferences))
	}
	if d.UserPreferences != nil {
		lines = append(lines, "  user preferences: "+formatGrafanaPreferenceInfo(*d.UserPreferences))
	}
	return strings.Join(lines, "\n")
}

type GrafanaDashboardListItem struct {
	UID         string `json:"uid" yaml:"uid"`
	Title       string `json:"title" yaml:"title"`
	FolderUID   string `json:"folder_uid,omitempty" yaml:"folder_uid,omitempty"`
	FolderTitle string `json:"folder_title,omitempty" yaml:"folder_title,omitempty"`
	URL         string `json:"url,omitempty" yaml:"url,omitempty"`
}

type GrafanaListData struct {
	Kind         string                     `json:"kind" yaml:"kind"`
	Endpoint     string                     `json:"endpoint" yaml:"endpoint"`
	Organization string                     `json:"organization,omitempty" yaml:"organization,omitempty"`
	Count        int                        `json:"count" yaml:"count"`
	Dashboards   []GrafanaDashboardListItem `json:"dashboards" yaml:"dashboards"`
}

func (d *GrafanaListData) Text() string {
	if d == nil {
		return ""
	}
	metadata := fmt.Sprintf("  endpoint: %s\n  organization: %s\n  dashboards: %d", d.Endpoint, emptyGrafanaText(d.Organization), d.Count)
	if len(d.Dashboards) == 0 {
		return metadata
	}
	rows := make([][]string, 0, len(d.Dashboards))
	for _, dashboard := range d.Dashboards {
		folder := dashboard.FolderTitle
		if folder == "" {
			folder = dashboard.FolderUID
		}
		if folder == "" {
			folder = "General"
		}
		rows = append(rows, []string{dashboard.UID, dashboard.Title, folder, dashboard.URL})
	}
	return metadata + "\n" + strings.TrimSuffix(output.RenderTable([]string{"UID", "TITLE", "FOLDER", "URL"}, rows), "\n")
}

type GrafanaActionData struct {
	Kind         string                `json:"kind" yaml:"kind"`
	Endpoint     string                `json:"endpoint" yaml:"endpoint"`
	Organization string                `json:"organization,omitempty" yaml:"organization,omitempty"`
	Scope        string                `json:"scope" yaml:"scope"`
	Path         string                `json:"path,omitempty" yaml:"path,omitempty"`
	All          bool                  `json:"all,omitempty" yaml:"all,omitempty"`
	DryRun       bool                  `json:"dry_run,omitempty" yaml:"dry_run,omitempty"`
	PlanDigest   string                `json:"plan_digest,omitempty" yaml:"plan_digest,omitempty"`
	Setting      string                `json:"setting,omitempty" yaml:"setting,omitempty"`
	Value        string                `json:"value,omitempty" yaml:"value,omitempty"`
	Results      []GrafanaObjectResult `json:"results" yaml:"results"`
	Failed       int                   `json:"failed" yaml:"failed"`
}

func (d *GrafanaActionData) Text() string {
	if d == nil {
		return ""
	}
	hasPhase := false
	for _, result := range d.Results {
		if result.Phase != "" {
			hasPhase = true
			break
		}
	}
	rows := make([][]string, 0, len(d.Results))
	for _, result := range d.Results {
		if hasPhase {
			rows = append(rows, []string{result.Phase, result.ObjectKind, result.UID, result.FolderUID, result.Action, result.Status, result.Message})
		} else {
			rows = append(rows, []string{result.ObjectKind, result.UID, result.FolderUID, result.Action, result.Status, result.Message})
		}
	}
	metadata := fmt.Sprintf("  endpoint: %s\n  organization: %s\n  scope: %s", d.Endpoint, emptyGrafanaText(d.Organization), d.Scope)
	if d.Path != "" {
		metadata += "\n  path: " + d.Path
	}
	if d.Setting != "" {
		metadata += "\n  setting: " + d.Setting + "\n  value: " + d.Value
	}
	if len(rows) == 0 {
		return metadata
	}
	headings := []string{"KIND", "UID", "FOLDER", "ACTION", "STATUS", "MESSAGE"}
	if hasPhase {
		headings = []string{"PHASE", "KIND", "UID", "FOLDER", "ACTION", "STATUS", "MESSAGE"}
	}
	return metadata + "\n" + strings.TrimSuffix(output.RenderTable(headings, rows), "\n")
}

type grafanaAPI interface {
	Endpoint() string
	Health(context.Context) (grafana.Health, error)
	CurrentOrganization(context.Context) (grafana.Organization, error)
	Search(context.Context) ([]grafana.SearchItem, error)
	GetDashboard(context.Context, string) (grafana.DashboardEnvelope, error)
	GetDashboardResource(context.Context, string, string) (map[string]interface{}, error)
	UpsertDashboard(context.Context, map[string]interface{}, string) error
	UpsertDashboardResource(context.Context, map[string]interface{}, string, string) error
	DeleteDashboard(context.Context, string) error
	CreateFolder(context.Context, string, string) error
	UpdateFolder(context.Context, string, string) error
	DeleteFolder(context.Context, string) error
	UpdateCurrentOrganization(context.Context, string) error
	StarDashboard(context.Context, string) error
	CurrentOrganizationPreferences(context.Context) (grafana.Preferences, error)
	CurrentUserPreferences(context.Context) (grafana.Preferences, error)
	PatchOrganizationPreferences(context.Context, grafana.PreferencesPatch) error
	PatchUserPreferences(context.Context, grafana.PreferencesPatch) error
}

type grafanaLoadAPI interface {
	Endpoint() string
	CreateFolder(context.Context, string, string) error
	UpdateFolder(context.Context, string, string) error
	UpsertDashboard(context.Context, map[string]interface{}, string) error
	UpsertDashboardResource(context.Context, map[string]interface{}, string, string) error
}

type grafanaPrepared struct {
	client               grafanaAPI
	username             string
	credentialConfigured bool
}

type grafanaLiveState struct {
	Dashboards map[string]grafana.SearchItem
	Folders    map[string]grafana.SearchItem
	Search     []grafana.SearchItem
}

func prepareGrafana(options GrafanaOptions) (*grafanaPrepared, error) {
	return prepareGrafanaConnection(options, true)
}

func prepareGrafanaInfo(options GrafanaOptions) (*grafanaPrepared, error) {
	return prepareGrafanaConnection(options, false)
}

// prepareGrafanaInfoHealth keeps an explicitly selected endpoint observable
// even when the active Inventory cannot be used for credential resolution. If
// the endpoint itself still depends on Inventory, the regular preparation path
// remains authoritative so Pig never probes a guessed instance.
func prepareGrafanaInfoHealth(options GrafanaOptions) (grafanaAPI, *grafanaPrepared, error) {
	// Explicit credential sources are validated locally before the health probe.
	// Inventory is intentionally excluded here because reading it is the failure
	// mode this two-phase info path is designed to keep observable.
	if _, err := grafanaAPIPassword(options.Password, options.PasswordFile, nil); err != nil {
		return nil, nil, err
	}
	endpoint := strings.TrimSpace(options.Endpoint)
	if endpoint == "" {
		endpoint = strings.TrimSpace(os.Getenv("GRAFANA_ENDPOINT"))
	}
	if endpoint != "" {
		client, err := grafana.NewClient(grafana.Config{Endpoint: endpoint, HTTPClient: options.HTTPClient})
		return client, nil, err
	}
	prepared, err := prepareGrafanaInfo(options)
	if err != nil {
		return nil, nil, err
	}
	return prepared.client, prepared, nil
}

func prepareGrafanaConnection(options GrafanaOptions, requireAuthentication bool) (*grafanaPrepared, error) {
	// A complete explicit connection is an emergency escape hatch: it must not
	// depend on an unrelated malformed or unavailable Inventory.
	explicitConnection := strings.TrimSpace(options.Endpoint) != "" &&
		strings.TrimSpace(options.Username) != "" &&
		(options.Password != "" || strings.TrimSpace(options.PasswordFile) != "")
	var doc *inventory.Document
	var err error
	if !explicitConnection {
		doc, err = loadGrafanaCredentialInventory(options.Inventory)
		if err != nil {
			return nil, err
		}
	}
	endpoint := strings.TrimSpace(options.Endpoint)
	if endpoint == "" {
		endpoint = strings.TrimSpace(os.Getenv("GRAFANA_ENDPOINT"))
	}
	if endpoint == "" {
		endpoint, _ = grafanaInventoryString(doc, "grafana_endpoint")
	}
	if endpoint == "" {
		endpoint = defaultGrafanaEndpoint
	}
	username := strings.TrimSpace(options.Username)
	if username == "" {
		username = strings.TrimSpace(os.Getenv("GRAFANA_USERNAME"))
	}
	if username == "" {
		username, _ = grafanaInventoryString(doc, "grafana_admin_username")
	}
	if username == "" {
		username = "admin"
	}
	password, err := grafanaAPIPassword(options.Password, options.PasswordFile, doc)
	if err != nil {
		return nil, err
	}
	credentialConfigured := password != ""
	if requireAuthentication && !credentialConfigured {
		return nil, errors.New("grafana password is not available from --password, --password-file, GRAFANA_PASSWORD, or all.vars.grafana_admin_password")
	}
	client, err := grafana.NewClient(grafana.Config{
		Endpoint: endpoint, Username: username, Password: password, HTTPClient: options.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	return &grafanaPrepared{client: client, username: username, credentialConfigured: credentialConfigured}, nil
}

func loadGrafanaCredentialInventory(filename string) (*inventory.Document, error) {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return nil, nil
	}
	doc, err := inventory.Load(filename)
	if err != nil {
		return nil, errors.New("active Inventory could not be parsed for Grafana credential resolution")
	}
	return doc, nil
}

func grafanaAPIPassword(password, passwordFile string, doc *inventory.Document) (string, error) {
	if password != "" && strings.TrimSpace(passwordFile) != "" {
		return "", errors.New("--password and --password-file are mutually exclusive")
	}
	if password != "" {
		return validateGrafanaPassword(password, "--password")
	}
	passwordFile = strings.TrimSpace(passwordFile)
	if passwordFile != "" {
		absolute, err := filepath.Abs(passwordFile)
		if err != nil {
			return "", fmt.Errorf("resolve Grafana password file: %w", err)
		}
		info, err := os.Lstat(absolute)
		if err != nil {
			return "", errors.New("grafana password file is unavailable")
		}
		if !info.Mode().IsRegular() || info.Size() > maxGrafanaPasswordBytes {
			return "", errors.New("grafana password file must be one bounded regular file")
		}
		if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
			return "", errors.New("grafana password file must be owner-only")
		}
		raw, err := os.ReadFile(absolute)
		if err != nil {
			return "", errors.New("grafana password file could not be read")
		}
		password := strings.TrimSuffix(strings.TrimSuffix(string(raw), "\n"), "\r")
		return validateGrafanaPassword(password, "Grafana password file")
	}
	if password := os.Getenv("GRAFANA_PASSWORD"); password != "" {
		return validateGrafanaPassword(password, "GRAFANA_PASSWORD")
	}
	password, _ = grafanaInventoryString(doc, "grafana_admin_password")
	return validateGrafanaPassword(password, "all.vars.grafana_admin_password")
}

func validateGrafanaPassword(password, source string) (string, error) {
	if len(password) > maxGrafanaPasswordBytes || strings.ContainsAny(password, "\x00\r\n") {
		return "", fmt.Errorf("%s must contain one bounded credential line", source)
	}
	return password, nil
}

func grafanaInventoryString(doc *inventory.Document, key string) (string, bool) {
	if doc == nil {
		return "", false
	}
	value, ok := doc.Lookup(inventory.AllVars(key))
	if !ok || value == nil || value.Kind != inventory.ScalarValue || value.Scalar == nil || value.Scalar.Kind != inventory.StringScalar {
		return "", false
	}
	if value.Alias != "" || value.Tag != "" && !strings.HasPrefix(value.Tag, "!!") && !strings.HasPrefix(value.Tag, "tag:yaml.org,2002:") {
		return "", false
	}
	text, ok := value.Scalar.Value.(string)
	return text, ok && text != ""
}

func grafanaContext(options GrafanaOptions) context.Context {
	if options.Context != nil {
		return options.Context
	}
	return context.Background()
}

func formatGrafanaPreferenceInfo(preference GrafanaPreferenceInfo) string {
	parts := make([]string, 0, 5)
	for _, pair := range [][2]string{
		{"home", preference.HomeDashboardUID},
		{"theme", preference.Theme},
		{"language", preference.Language},
		{"timezone", preference.Timezone},
		{"week_start", preference.WeekStart},
	} {
		if pair[1] != "" {
			parts = append(parts, pair[0]+"="+pair[1])
		}
	}
	if len(parts) == 0 {
		return "inherited defaults"
	}
	return strings.Join(parts, ", ")
}

func grafanaPreferenceInfo(preference grafana.Preferences) *GrafanaPreferenceInfo {
	return &GrafanaPreferenceInfo{
		HomeDashboardUID: preference.HomeDashboardUID,
		Theme:            preference.Theme,
		Language:         preference.Language,
		Timezone:         preference.Timezone,
		WeekStart:        preference.WeekStart,
	}
}

// GrafanaInfo separates public endpoint health from authenticated API health.
// This makes it useful both as a readiness probe and as a credential check.
func GrafanaInfo(options GrafanaOptions) *output.Result {
	healthClient, prepared, err := prepareGrafanaInfoHealth(options)
	if err != nil {
		return grafanaSetupFailure("Grafana info setup failed", err)
	}
	data := &GrafanaInfoData{
		Kind: GrafanaInfoKind, Endpoint: healthClient.Endpoint(), Status: "unknown",
		AuthStatus: "not_configured",
	}
	ctx := grafanaContext(options)
	health, err := healthClient.Health(ctx)
	if err != nil {
		data.Status = "unreachable"
		return grafanaAPIFailure("Grafana health check failed", healthClient, err).WithData(data)
	}
	data.Database, data.Version, data.Commit = health.Database, health.Version, health.Commit
	if strings.EqualFold(strings.TrimSpace(health.Database), "ok") {
		data.Status = "healthy"
	} else {
		data.Status = "unhealthy"
		return output.Fail(output.CodeStyGrafanaPartialFailure, "Grafana reported an unhealthy database").WithData(data)
	}
	if prepared == nil {
		prepared, err = prepareGrafanaInfo(options)
		if err != nil {
			data.AuthStatus = "error"
			return grafanaSetupFailure("Grafana is healthy but authentication setup failed", err).WithData(data)
		}
	}
	data.Username = prepared.username
	if !prepared.credentialConfigured {
		return output.OK("Grafana is healthy; authentication is not configured", data)
	}

	organization, err := prepared.client.CurrentOrganization(ctx)
	if err != nil {
		data.AuthStatus = grafanaAuthenticationStatus(err)
		return grafanaAPIFailure("Grafana authentication check failed", prepared.client, err).WithData(data)
	}
	data.AuthStatus = "authenticated"
	data.OrganizationID, data.Organization = organization.ID, organization.Name
	state, err := fetchGrafanaLiveState(ctx, prepared.client)
	if err != nil {
		return grafanaAPIFailure("Grafana dashboard inventory check failed", prepared.client, err).WithData(data)
	}
	dashboardCount, folderCount := len(state.Dashboards), len(state.Folders)
	data.DashboardCount, data.FolderCount = &dashboardCount, &folderCount
	organizationPreferences, err := prepared.client.CurrentOrganizationPreferences(ctx)
	if err != nil {
		return grafanaAPIFailure("Grafana organization preferences check failed", prepared.client, err).WithData(data)
	}
	data.OrganizationPreferences = grafanaPreferenceInfo(organizationPreferences)
	userPreferences, err := prepared.client.CurrentUserPreferences(ctx)
	if err != nil {
		return grafanaAPIFailure("Grafana user preferences check failed", prepared.client, err).WithData(data)
	}
	data.UserPreferences = grafanaPreferenceInfo(userPreferences)
	return output.OK("Grafana is healthy and authentication succeeded", data)
}

func grafanaAuthenticationStatus(err error) string {
	switch grafana.StatusCode(err) {
	case http.StatusUnauthorized:
		return "unauthorized"
	case http.StatusForbidden:
		return "forbidden"
	default:
		return "error"
	}
}

// GrafanaList returns a stable, read-only inventory of all dashboards visible
// in the active Grafana organization.
func GrafanaList(options GrafanaOptions) *output.Result {
	prepared, err := prepareGrafana(options)
	if err != nil {
		return grafanaSetupFailure("Grafana list setup failed", err)
	}
	organization, state, err := preflightGrafana(grafanaContext(options), prepared.client)
	if err != nil {
		return grafanaAPIFailure("Grafana list preflight failed", prepared.client, err)
	}
	dashboards := make([]GrafanaDashboardListItem, 0, len(state.Dashboards))
	for _, item := range state.Dashboards {
		dashboards = append(dashboards, GrafanaDashboardListItem{
			UID: item.UID, Title: item.Title, FolderUID: item.FolderUID,
			FolderTitle: item.FolderTitle, URL: item.URL,
		})
	}
	sort.Slice(dashboards, func(i, j int) bool {
		if dashboards[i].FolderUID != dashboards[j].FolderUID {
			return dashboards[i].FolderUID < dashboards[j].FolderUID
		}
		if dashboards[i].Title != dashboards[j].Title {
			return dashboards[i].Title < dashboards[j].Title
		}
		return dashboards[i].UID < dashboards[j].UID
	})
	data := &GrafanaListData{
		Kind: GrafanaListKind, Endpoint: prepared.client.Endpoint(),
		Organization: organization.Name, Count: len(dashboards), Dashboards: dashboards,
	}
	return output.OK("Grafana dashboards listed", data)
}

// GrafanaLoad is the pure dashboard/folder loader. Its API surface is narrowed
// at compile time so loading cannot rename an organization, star a dashboard,
// or alter preferences.
func GrafanaLoad(options GrafanaOptions) *output.Result {
	scope, err := resolveGrafanaScope(options, grafanaActionLoad)
	if err != nil {
		return grafanaSetupFailure("Grafana load scope is invalid", err)
	}
	if err := loadGrafanaScopeAssets(scope); err != nil {
		return grafanaSetupFailure("Grafana load source is invalid", err)
	}
	prepared, err := prepareGrafana(options)
	if err != nil {
		return grafanaSetupFailure("Grafana load setup failed", err)
	}
	ctx := grafanaContext(options)
	organization, state, err := preflightGrafana(ctx, prepared.client)
	if err != nil {
		return grafanaAPIFailure("Grafana load preflight failed", prepared.client, err)
	}
	data := newGrafanaActionData(GrafanaLoadKind, prepared.client, organization, scope)
	applyGrafanaLoad(ctx, prepared.client, state, scope, data, "load")
	return finishGrafanaAction("Grafana dashboards loaded", data)
}

func applyGrafanaLoad(ctx context.Context, client grafanaLoadAPI, state *grafanaLiveState, scope *grafanaScope, data *GrafanaActionData, phase string) {
	failedFolders := make(map[string]struct{})
	for _, folderUID := range scope.Folders {
		action := "create"
		var mutationErr error
		if _, exists := state.Folders[folderUID]; exists {
			action = "update"
			mutationErr = client.UpdateFolder(ctx, folderUID, strings.ToUpper(folderUID))
		} else {
			mutationErr = client.CreateFolder(ctx, folderUID, strings.ToUpper(folderUID))
			if status := grafana.StatusCode(mutationErr); status == http.StatusConflict || status == http.StatusPreconditionFailed {
				action = "update"
				mutationErr = client.UpdateFolder(ctx, folderUID, strings.ToUpper(folderUID))
			}
		}
		data.Results = append(data.Results, withGrafanaPhase(grafanaMutationResult("folder", folderUID, folderUID, action, mutationErr), phase))
		if mutationErr != nil {
			failedFolders[folderUID] = struct{}{}
		}
	}

	for _, asset := range scope.Assets {
		if _, failed := failedFolders[asset.FolderUID]; failed && asset.FolderUID != "" {
			data.Results = append(data.Results, GrafanaObjectResult{
				Phase:      phase,
				ObjectKind: "dashboard", UID: asset.UID, FolderUID: asset.FolderUID,
				Action: "upsert", Status: "skipped", Message: "target folder could not be prepared",
			})
			continue
		}
		var mutationErr error
		if asset.Format == grafanaDashboardV2 {
			var resource map[string]interface{}
			resource, mutationErr = dashboardResourceForGrafanaInit(asset.Dashboard, asset.Namespace, asset.UID, asset.FolderUID)
			if mutationErr == nil {
				mutationErr = client.UpsertDashboardResource(ctx, resource, asset.Namespace, asset.UID)
			}
		} else {
			dashboard := dashboardForGrafanaInit(asset.Dashboard)
			mutationErr = client.UpsertDashboard(ctx, dashboard, asset.FolderUID)
		}
		data.Results = append(data.Results, withGrafanaPhase(grafanaMutationResult("dashboard", asset.UID, asset.FolderUID, "upsert", mutationErr), phase))
	}
}

// GrafanaBoot applies only the instance-level Pigsty bootstrap. It verifies the
// home dashboard before making any mutation and PATCHes only the home UID.
func GrafanaBoot(options GrafanaOptions) *output.Result {
	prepared, err := prepareGrafana(options)
	if err != nil {
		return grafanaSetupFailure("Grafana boot setup failed", err)
	}
	ctx := grafanaContext(options)
	organization, err := preflightGrafanaOrganization(ctx, prepared.client)
	if err != nil {
		return grafanaAPIFailure("Grafana boot preflight failed", prepared.client, err)
	}
	data := newGrafanaActionData(GrafanaBootKind, prepared.client, organization, nil)
	data.Scope = "instance"
	if err := verifyGrafanaHomeDashboard(ctx, prepared.client); err != nil {
		return grafanaAPIFailure("Grafana boot requires dashboard UID pigsty", prepared.client, err).WithData(data)
	}
	applyGrafanaBoot(ctx, prepared.client, organization, data, "boot")
	return finishGrafanaAction("Grafana instance bootstrapped", data)
}

func verifyGrafanaHomeDashboard(ctx context.Context, client grafanaAPI) error {
	_, err := client.GetDashboard(ctx, "pigsty")
	return err
}

func applyGrafanaBoot(ctx context.Context, client grafanaAPI, organization grafana.Organization, data *GrafanaActionData, phase string) {
	if organization.Name == "Pigsty" {
		data.Results = append(data.Results, GrafanaObjectResult{
			Phase: phase, ObjectKind: "organization", UID: fmt.Sprint(organization.ID),
			Action: "rename", Status: "unchanged",
		})
		data.Organization = organization.Name
	} else {
		err := client.UpdateCurrentOrganization(ctx, "Pigsty")
		data.Results = append(data.Results, withGrafanaPhase(grafanaMutationResult("organization", fmt.Sprint(organization.ID), "", "rename", err), phase))
		if err == nil {
			data.Organization = "Pigsty"
		}
	}
	starErr := client.StarDashboard(ctx, "pigsty")
	data.Results = append(data.Results, withGrafanaPhase(grafanaMutationResult("preference", "pigsty", "", "star", starErr), phase))
	homeUID := "pigsty"
	patch := grafana.PreferencesPatch{HomeDashboardUID: &homeUID}
	orgErr := client.PatchOrganizationPreferences(ctx, patch)
	data.Results = append(data.Results, withGrafanaPhase(grafanaMutationResult("preference", "org", "", "set-home", orgErr), phase))
	userErr := client.PatchUserPreferences(ctx, patch)
	data.Results = append(data.Results, withGrafanaPhase(grafanaMutationResult("preference", "user", "", "set-home", userErr), phase))
}

// GrafanaInit is the root-only composition load then boot. A failed or skipped
// load phase prevents every boot mutation.
func GrafanaInit(options GrafanaOptions) *output.Result {
	scope, err := resolveGrafanaScope(options, grafanaActionLoad)
	if err != nil {
		return grafanaSetupFailure("Grafana init scope is invalid", err)
	}
	if scope.Kind != grafanaScopeRoot {
		return grafanaSetupFailure("Grafana init scope is invalid", errors.New("init requires a complete grafana root; use load for a folder or dashboard"))
	}
	if err := loadGrafanaScopeAssets(scope); err != nil {
		return grafanaSetupFailure("Grafana init source is invalid", err)
	}
	prepared, err := prepareGrafana(options)
	if err != nil {
		return grafanaSetupFailure("Grafana init setup failed", err)
	}
	ctx := grafanaContext(options)
	organization, state, err := preflightGrafana(ctx, prepared.client)
	if err != nil {
		return grafanaAPIFailure("Grafana init preflight failed", prepared.client, err)
	}
	data := newGrafanaActionData(GrafanaInitKind, prepared.client, organization, scope)
	applyGrafanaLoad(ctx, prepared.client, state, scope, data, "load")
	if grafanaResultsPreventBoot(data.Results) {
		data.Results = append(data.Results, GrafanaObjectResult{
			Phase: "boot", ObjectKind: "instance", UID: "pigsty", Action: "boot", Status: "skipped",
			Message: "load phase did not complete successfully",
		})
		return finishGrafanaAction("Grafana init stopped before boot", data)
	}
	if err := verifyGrafanaHomeDashboard(ctx, prepared.client); err != nil {
		data.Results = append(data.Results, withGrafanaPhase(grafanaMutationResult("preference", "pigsty", "", "verify-home", err), "boot"))
		return finishGrafanaAction("Grafana dashboards loaded but boot failed", data)
	}
	applyGrafanaBoot(ctx, prepared.client, organization, data, "boot")
	return finishGrafanaAction("Grafana dashboards initialized", data)
}

func grafanaResultsPreventBoot(results []GrafanaObjectResult) bool {
	for _, result := range results {
		if result.Phase == "load" && (result.Status == "failed" || result.Status == "skipped") {
			return true
		}
	}
	return false
}

func withGrafanaPhase(result GrafanaObjectResult, phase string) GrafanaObjectResult {
	result.Phase = phase
	return result
}

// GrafanaLang changes only the organization and current-user language fields.
func GrafanaLang(options GrafanaOptions, language string) *output.Result {
	canonical, err := normalizeGrafanaLanguage(language)
	if err != nil {
		return grafanaSetupFailure("Grafana language is invalid", err)
	}
	return patchGrafanaPreference(options, GrafanaLangKind, "language", canonical, func(value string) grafana.PreferencesPatch {
		return grafana.PreferencesPatch{Language: &value}
	})
}

// GrafanaStyle changes only the organization and current-user theme fields.
// Grafana calls its automatic light/dark mode "system" at the API boundary.
func GrafanaStyle(options GrafanaOptions, style string) *output.Result {
	canonical, err := normalizeGrafanaStyle(style)
	if err != nil {
		return grafanaSetupFailure("Grafana style is invalid", err)
	}
	return patchGrafanaPreference(options, GrafanaStyleKind, "style", canonical, func(value string) grafana.PreferencesPatch {
		return grafana.PreferencesPatch{Theme: &value}
	})
}

func patchGrafanaPreference(options GrafanaOptions, kind, setting, value string, patch func(string) grafana.PreferencesPatch) *output.Result {
	prepared, err := prepareGrafana(options)
	if err != nil {
		return grafanaSetupFailure("Grafana "+setting+" setup failed", err)
	}
	ctx := grafanaContext(options)
	organization, err := preflightGrafanaOrganization(ctx, prepared.client)
	if err != nil {
		return grafanaAPIFailure("Grafana "+setting+" preflight failed", prepared.client, err)
	}
	data := newGrafanaActionData(kind, prepared.client, organization, nil)
	data.Scope, data.Setting, data.Value = "instance", setting, value
	payload := patch(value)
	orgErr := prepared.client.PatchOrganizationPreferences(ctx, payload)
	data.Results = append(data.Results, grafanaMutationResult("preference", "org", "", "set-"+setting, orgErr))
	userErr := prepared.client.PatchUserPreferences(ctx, payload)
	data.Results = append(data.Results, grafanaMutationResult("preference", "user", "", "set-"+setting, userErr))
	return finishGrafanaAction("Grafana "+setting+" updated", data)
}

func normalizeGrafanaStyle(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "dark":
		return "dark", nil
	case "light":
		return "light", nil
	case "auto":
		return "system", nil
	default:
		return "", errors.New("style must be dark, light, or auto")
	}
}

func normalizeGrafanaLanguage(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	aliases := map[string]string{
		"en": "en-US", "en-us": "en-US",
		"zh": "zh-Hans", "zh-cn": "zh-Hans", "zh-hans": "zh-Hans",
		"zh-tw": "zh-Hant", "zh-hk": "zh-Hant", "zh-hant": "zh-Hant",
	}
	if canonical, exists := aliases[normalized]; exists {
		return canonical, nil
	}
	languages := []string{
		"cs-CZ", "de-DE", "es-ES", "fr-FR", "hu-HU", "id-ID", "it-IT", "ja-JP",
		"ko-KR", "nl-NL", "pl-PL", "pt-BR", "pt-PT", "ru-RU", "sv-SE", "tr-TR",
	}
	for _, language := range languages {
		if strings.EqualFold(language, normalized) {
			return language, nil
		}
	}
	return "", fmt.Errorf("unsupported language code for %s", grafanaLanguageSet)
}

// GrafanaClean deletes only UIDs declared by the selected local scope. All=true
// is the sole path to instance-wide deletion and is intentionally incompatible
// with an explicit positional path.
func GrafanaClean(options GrafanaOptions) *output.Result {
	if options.All && strings.TrimSpace(options.Path) != "" {
		return grafanaSetupFailure("Grafana clean arguments are invalid", errors.New("--all and PATH are mutually exclusive"))
	}
	var scope *grafanaScope
	var err error
	if !options.All {
		scope, err = resolveGrafanaScope(options, grafanaActionClean)
		if err != nil {
			return grafanaSetupFailure("Grafana clean scope is invalid", err)
		}
		if err := loadGrafanaScopeAssets(scope); err != nil {
			return grafanaSetupFailure("Grafana clean source is invalid", err)
		}
	}
	prepared, err := prepareGrafana(options)
	if err != nil {
		return grafanaSetupFailure("Grafana clean setup failed", err)
	}
	ctx := grafanaContext(options)
	organization, state, err := preflightGrafana(ctx, prepared.client)
	if err != nil {
		return grafanaAPIFailure("Grafana clean preflight failed", prepared.client, err)
	}
	data := grafanaCleanPlan(prepared.client, organization, state, scope, options.All)
	data.DryRun = options.DryRun || !options.Yes
	data.PlanDigest = digestGrafanaPlan(data)
	if options.DryRun {
		return output.OK("Grafana clean plan generated; no changes applied", data)
	}
	if !options.Yes {
		return output.Fail(output.CodeStyGrafanaConfirmationRequired, "Grafana clean requires explicit confirmation").
			WithData(data)
	}
	if options.PlanDigest != "" && options.PlanDigest != data.PlanDigest {
		return output.Fail(output.CodeStyGrafanaPlanChanged, "Grafana clean plan changed before confirmation could be applied").WithData(data)
	}
	data.DryRun = false
	if result := applyGrafanaClean(ctx, prepared.client, state, data); result != nil {
		return result
	}
	return finishGrafanaAction("Grafana dashboards cleaned", data)
}

// GrafanaDump exports the selected remote scope without deleting local files.
// Callers should treat this as a local write operation; it is deliberately
// separate from init/clean validation.
func GrafanaDump(options GrafanaOptions) *output.Result {
	scope, err := resolveGrafanaDumpScope(options)
	if err != nil {
		return grafanaSetupFailure("Grafana dump scope is invalid", err)
	}
	prepared, err := prepareGrafana(options)
	if err != nil {
		return grafanaSetupFailure("Grafana dump setup failed", err)
	}
	ctx := grafanaContext(options)
	organization, state, err := preflightGrafana(ctx, prepared.client)
	if err != nil {
		return grafanaAPIFailure("Grafana dump preflight failed", prepared.client, err)
	}
	data := newGrafanaActionData(GrafanaDumpKind, prepared.client, organization, scope)
	items, err := selectGrafanaDumpItems(ctx, prepared.client, state, scope)
	if err != nil {
		return grafanaAPIFailure("Grafana dump selection failed", prepared.client, err).WithData(data)
	}
	staged := make([]grafanaDumpFile, 0, len(items))
	remoteUIDs := make(map[string]struct{}, len(items))
	targets := make(map[string]string, len(items))
	for _, item := range items {
		file, changed, failure := prepareGrafanaDumpFile(ctx, prepared.client, scope, item, targets, data)
		if failure != nil {
			return failure
		}
		remoteUIDs[item.UID] = struct{}{}
		if changed {
			staged = append(staged, file)
		}
	}
	if failure := preserveLocalGrafanaDumpAssets(scope, remoteUIDs, data); failure != nil {
		return failure
	}
	if failure := applyGrafanaDumpFiles(staged, data); failure != nil {
		return failure
	}
	return finishGrafanaAction("Grafana dashboards dumped; local-only files were preserved", data)
}

type grafanaDumpFile struct {
	target string
	raw    []byte
	uid    string
	folder string
}

func prepareGrafanaDumpFile(ctx context.Context, client grafanaAPI, scope *grafanaScope, item grafana.SearchItem, targets map[string]string, data *GrafanaActionData) (grafanaDumpFile, bool, *output.Result) {
	if err := validateGrafanaUID("dashboard", item.UID); err != nil {
		return grafanaDumpFile{}, false, grafanaSetupFailure("Grafana dump returned an unsafe UID", err).WithData(data)
	}
	if item.FolderUID != "" {
		if err := validateGrafanaUID("folder", item.FolderUID); err != nil {
			return grafanaDumpFile{}, false, grafanaSetupFailure("Grafana dump returned an unsafe folder UID", err).WithData(data)
		}
	}
	envelope, err := client.GetDashboard(ctx, item.UID)
	if err != nil {
		return grafanaDumpFile{}, false, grafanaAPIFailure("Grafana dashboard could not be fetched for dump", client, err).WithData(data)
	}
	remoteFolder := envelope.Meta.FolderUID
	if failure := validateGrafanaDumpTarget(scope, item, remoteFolder, targets, data); failure != nil {
		return grafanaDumpFile{}, false, failure
	}
	target := grafanaDumpTarget(scope, item.UID, remoteFolder)
	dashboard, failure := loadGrafanaDumpDashboard(ctx, client, item.UID, remoteFolder, target, envelope.Dashboard, data)
	if failure != nil {
		return grafanaDumpFile{}, false, failure
	}
	raw, err := marshalGrafanaDashboard(dashboard)
	if err != nil {
		return grafanaDumpFile{}, false, grafanaSetupFailure("Grafana dashboard could not be encoded for dump", err).WithData(data)
	}
	existing, readErr := os.ReadFile(target)
	if readErr == nil && bytes.Equal(existing, raw) {
		data.Results = append(data.Results, GrafanaObjectResult{ObjectKind: "dashboard", UID: item.UID, FolderUID: remoteFolder, Action: "dump", Status: "unchanged"})
		return grafanaDumpFile{}, false, nil
	}
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return grafanaDumpFile{}, false, grafanaSetupFailure("Grafana dump target could not be read", readErr).WithData(data)
	}
	data.Results = append(data.Results, GrafanaObjectResult{ObjectKind: "dashboard", UID: item.UID, FolderUID: remoteFolder, Action: "dump", Status: "staged"})
	return grafanaDumpFile{target: target, raw: raw, uid: item.UID, folder: remoteFolder}, true, nil
}

func validateGrafanaDumpTarget(scope *grafanaScope, item grafana.SearchItem, remoteFolder string, targets map[string]string, data *GrafanaActionData) *output.Result {
	if remoteFolder != item.FolderUID {
		return grafanaSetupFailure("Grafana dump state changed during export", fmt.Errorf("dashboard %s moved from folder %q to %q", item.UID, item.FolderUID, remoteFolder)).WithData(data)
	}
	if scope.Kind == grafanaScopeFile && remoteFolder != scope.FolderUID {
		return grafanaSetupFailure("Grafana dump target folder does not match the live dashboard", fmt.Errorf("dashboard %s is in folder %q", item.UID, remoteFolder)).WithData(data)
	}
	target := grafanaDumpTarget(scope, item.UID, remoteFolder)
	if !pathWithin(scope.Root, target) {
		return grafanaSetupFailure("Grafana dump target escapes its root", errors.New("unsafe dump target")).WithData(data)
	}
	if previous, exists := targets[target]; exists {
		return grafanaSetupFailure("Grafana dump target collision", fmt.Errorf("dashboards %s and %s map to %s", previous, item.UID, target)).WithData(data)
	}
	targets[target] = item.UID
	if err := rejectGrafanaSymlinkPath(scope.Root, target); err != nil {
		return grafanaSetupFailure("Grafana dump target is unsafe", err).WithData(data)
	}
	return nil
}

func loadGrafanaDumpDashboard(ctx context.Context, client grafanaAPI, uid, remoteFolder, target string, legacy map[string]interface{}, data *GrafanaActionData) (map[string]interface{}, *output.Result) {
	format, namespace, failure := localGrafanaDumpFormat(target, remoteFolder, data)
	if failure != nil {
		return nil, failure
	}
	if format != grafanaDashboardV2 {
		dashboard, err := dashboardForGrafanaDump(legacy)
		if err != nil {
			return nil, grafanaSetupFailure("Grafana dashboard could not be normalized for dump", err).WithData(data)
		}
		actualUID, _ := dashboard["uid"].(string)
		if actualUID != uid {
			return nil, grafanaSetupFailure("Grafana dump returned inconsistent dashboard identity", fmt.Errorf("search UID %q differs from dashboard UID %q", uid, actualUID)).WithData(data)
		}
		return dashboard, nil
	}
	resource, err := client.GetDashboardResource(ctx, namespace, uid)
	if err != nil {
		return nil, grafanaAPIFailure("Grafana dashboard resource could not be fetched for dump", client, err).WithData(data)
	}
	actualFormat, actualUID, actualNamespace, identityErr := grafanaDashboardIdentity(resource)
	if identityErr == nil && (actualFormat != grafanaDashboardV2 || actualUID != uid || actualNamespace != namespace) {
		identityErr = fmt.Errorf("resource identity %q/%q differs from expected %q/%q", actualNamespace, actualUID, namespace, uid)
	}
	if identityErr != nil {
		return nil, grafanaSetupFailure("Grafana dump returned inconsistent dashboard resource identity", identityErr).WithData(data)
	}
	dashboard, err := dashboardResourceForGrafanaInit(resource, namespace, uid, remoteFolder)
	if err != nil {
		return nil, grafanaSetupFailure("Grafana dashboard resource could not be normalized for dump", err).WithData(data)
	}
	return dashboard, nil
}

func localGrafanaDumpFormat(target, remoteFolder string, data *GrafanaActionData) (grafanaDashboardFormat, string, *output.Result) {
	if _, err := os.Lstat(target); errors.Is(err, os.ErrNotExist) {
		return grafanaDashboardLegacy, "", nil
	} else if err != nil {
		return 0, "", grafanaSetupFailure("Grafana dump target could not be inspected", err).WithData(data)
	}
	asset, err := readGrafanaAsset(target, remoteFolder)
	if err != nil {
		return 0, "", grafanaSetupFailure("Grafana dump target is invalid", err).WithData(data)
	}
	return asset.Format, asset.Namespace, nil
}

func preserveLocalGrafanaDumpAssets(scope *grafanaScope, remoteUIDs map[string]struct{}, data *GrafanaActionData) *output.Result {
	if _, err := os.Lstat(scope.Path); err != nil {
		return nil
	}
	localScope := *scope
	if err := loadGrafanaScopeAssets(&localScope); err != nil {
		return grafanaSetupFailure("Grafana dump local scope is invalid", err).WithData(data)
	}
	for _, asset := range localScope.Assets {
		if _, exists := remoteUIDs[asset.UID]; !exists {
			data.Results = append(data.Results, GrafanaObjectResult{
				ObjectKind: "dashboard", UID: asset.UID, FolderUID: asset.FolderUID,
				Action: "dump", Status: "kept", Message: "local-only dashboard is never deleted by dump",
			})
		}
	}
	return nil
}

func applyGrafanaDumpFiles(files []grafanaDumpFile, data *GrafanaActionData) *output.Result {
	for _, file := range files {
		if err := writeAtomicGrafanaFile(file.target, file.raw); err != nil {
			data.Results = append(data.Results, grafanaInternalFailure("dashboard", file.uid, file.folder, "dump", err))
			return finishGrafanaAction("Grafana dump stopped after a local write failure", data)
		}
		for index := range data.Results {
			result := &data.Results[index]
			if result.UID == file.uid && result.FolderUID == file.folder && result.Status == "staged" {
				result.Status = "exported"
				break
			}
		}
	}
	return nil
}

func preflightGrafana(ctx context.Context, client grafanaAPI) (grafana.Organization, *grafanaLiveState, error) {
	organization, err := preflightGrafanaOrganization(ctx, client)
	if err != nil {
		return grafana.Organization{}, nil, err
	}
	state, err := fetchGrafanaLiveState(ctx, client)
	return organization, state, err
}

func preflightGrafanaOrganization(ctx context.Context, client grafanaAPI) (grafana.Organization, error) {
	health, err := client.Health(ctx)
	if err != nil {
		return grafana.Organization{}, err
	}
	if !strings.EqualFold(strings.TrimSpace(health.Database), "ok") {
		return grafana.Organization{}, errors.New("grafana database health is not ok")
	}
	return client.CurrentOrganization(ctx)
}

func fetchGrafanaLiveState(ctx context.Context, client grafanaAPI) (*grafanaLiveState, error) {
	search, err := client.Search(ctx)
	if err != nil {
		return nil, err
	}
	state := &grafanaLiveState{
		Dashboards: make(map[string]grafana.SearchItem),
		Folders:    make(map[string]grafana.SearchItem),
		Search:     append([]grafana.SearchItem(nil), search...),
	}
	for _, item := range search {
		switch item.Type {
		case "dash-db":
			if item.UID != "" {
				state.Dashboards[item.UID] = item
			}
		case "dash-folder":
			if item.UID != "" {
				state.Folders[item.UID] = item
			}
		}
	}
	return state, nil
}

func newGrafanaActionData(kind string, client grafanaAPI, organization grafana.Organization, scope *grafanaScope) *GrafanaActionData {
	data := &GrafanaActionData{
		Kind: kind, Endpoint: client.Endpoint(), Organization: organization.Name,
		Results: []GrafanaObjectResult{},
	}
	if scope != nil {
		data.Scope = string(scope.Kind)
		data.Path = scope.Path
	}
	return data
}

func grafanaCleanPlan(client grafanaAPI, organization grafana.Organization, state *grafanaLiveState, scope *grafanaScope, all bool) *GrafanaActionData {
	data := newGrafanaActionData(GrafanaCleanKind, client, organization, scope)
	data.All = all
	if all {
		data.Scope = "all"
		data.Path = ""
		uids := make([]string, 0, len(state.Dashboards))
		for uid := range state.Dashboards {
			uids = append(uids, uid)
		}
		sort.Strings(uids)
		for _, uid := range uids {
			item := state.Dashboards[uid]
			data.Results = append(data.Results, GrafanaObjectResult{ObjectKind: "dashboard", UID: uid, FolderUID: item.FolderUID, Action: "delete", Status: "planned"})
		}
		for _, uid := range orderGrafanaFoldersForDelete(state, mapKeysGrafanaFolders(state.Folders)) {
			data.Results = append(data.Results, GrafanaObjectResult{ObjectKind: "folder", UID: uid, FolderUID: uid, Action: "delete-if-empty", Status: "planned"})
		}
		return data
	}

	for _, asset := range scope.Assets {
		if item, exists := state.Dashboards[asset.UID]; exists {
			data.Results = append(data.Results, GrafanaObjectResult{ObjectKind: "dashboard", UID: asset.UID, FolderUID: item.FolderUID, Action: "delete", Status: "planned"})
		} else {
			data.Results = append(data.Results, GrafanaObjectResult{ObjectKind: "dashboard", UID: asset.UID, FolderUID: asset.FolderUID, Action: "delete", Status: "missing"})
		}
	}
	for _, folderUID := range scope.Folders {
		if _, exists := state.Folders[folderUID]; exists {
			data.Results = append(data.Results, GrafanaObjectResult{ObjectKind: "folder", UID: folderUID, FolderUID: folderUID, Action: "delete-if-empty", Status: "planned"})
		}
	}
	sortGrafanaResults(data.Results)
	return data
}

func applyGrafanaClean(ctx context.Context, client grafanaAPI, state *grafanaLiveState, data *GrafanaActionData) *output.Result {
	for index := range data.Results {
		item := &data.Results[index]
		if item.ObjectKind != "dashboard" || item.Status != "planned" {
			continue
		}
		err := client.DeleteDashboard(ctx, item.UID)
		if grafana.IsNotFound(err) {
			item.Status = "missing"
			item.Message = "dashboard disappeared after planning"
			delete(state.Dashboards, item.UID)
			continue
		}
		if err != nil {
			item.Status = "failed"
			item.Message = "Grafana rejected or failed the deletion"
			item.ResultCode = grafanaAPIResultCode(err)
			return finishGrafanaAction("Grafana clean stopped after a dashboard deletion failure", data)
		}
		item.Status = "deleted"
		delete(state.Dashboards, item.UID)
		state.Search = removeGrafanaSearchItem(state.Search, "dash-db", item.UID)
	}

	refreshed, err := fetchGrafanaLiveState(ctx, client)
	if err != nil {
		data.Results = append(data.Results, grafanaInternalFailure("folder", "selected-folders", "", "inspect", err))
		return finishGrafanaAction("Grafana clean could not verify folder contents", data)
	}
	state = refreshed
	candidates := make([]string, 0)
	for _, item := range data.Results {
		if item.ObjectKind == "folder" && item.Status == "planned" {
			candidates = append(candidates, item.UID)
		}
	}
	ordered := orderGrafanaFoldersForDelete(state, candidates)
	for _, uid := range ordered {
		index := grafanaResultIndex(data.Results, "folder", uid, "planned")
		if index < 0 {
			continue
		}
		item := &data.Results[index]
		if _, exists := state.Folders[uid]; !exists {
			item.Status = "missing"
			continue
		}
		if grafanaFolderHasContents(state.Search, uid) {
			item.Status = "kept"
			item.Message = "folder contains unmanaged or remaining resources"
			continue
		}
		deleteErr := client.DeleteFolder(ctx, uid)
		if grafana.IsNotFound(deleteErr) {
			item.Status = "missing"
			delete(state.Folders, uid)
			continue
		}
		if deleteErr != nil {
			item.Status = "failed"
			item.Message = "Grafana rejected or failed the folder deletion"
			item.ResultCode = grafanaAPIResultCode(deleteErr)
			return finishGrafanaAction("Grafana clean stopped after a folder deletion failure", data)
		}
		item.Status = "deleted"
		delete(state.Folders, uid)
		state.Search = removeGrafanaSearchItem(state.Search, "dash-folder", uid)
	}
	return nil
}

func selectGrafanaDumpItems(ctx context.Context, client grafanaAPI, state *grafanaLiveState, scope *grafanaScope) ([]grafana.SearchItem, error) {
	if scope.Kind == grafanaScopeFile {
		envelope, err := client.GetDashboard(ctx, scope.UID)
		if err != nil {
			return nil, err
		}
		return []grafana.SearchItem{{UID: scope.UID, Type: "dash-db", FolderUID: envelope.Meta.FolderUID}}, nil
	}
	items := make([]grafana.SearchItem, 0)
	for _, item := range state.Search {
		if item.Type != "dash-db" {
			continue
		}
		if scope.Kind == grafanaScopeFolder && item.FolderUID != scope.FolderUID {
			continue
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].FolderUID != items[j].FolderUID {
			return items[i].FolderUID < items[j].FolderUID
		}
		return items[i].UID < items[j].UID
	})
	return items, nil
}

func grafanaDumpTarget(scope *grafanaScope, uid, folderUID string) string {
	if scope.Kind == grafanaScopeFile {
		return scope.Path
	}
	if scope.Kind == grafanaScopeFolder {
		return filepath.Join(scope.Path, uid+".json")
	}
	if folderUID == "" {
		return filepath.Join(scope.Root, uid+".json")
	}
	return filepath.Join(scope.Root, folderUID, uid+".json")
}

func orderGrafanaFoldersForDelete(state *grafanaLiveState, candidates []string) []string {
	unique := make(map[string]struct{}, len(candidates))
	for _, uid := range candidates {
		unique[uid] = struct{}{}
	}
	ordered := make([]string, 0, len(unique))
	for uid := range unique {
		ordered = append(ordered, uid)
	}
	depth := func(uid string) int {
		seen := make(map[string]struct{})
		value := 0
		for uid != "" {
			if _, exists := seen[uid]; exists {
				break
			}
			seen[uid] = struct{}{}
			item, exists := state.Folders[uid]
			if !exists || item.ParentUID == "" {
				break
			}
			uid = item.ParentUID
			value++
		}
		return value
	}
	sort.Slice(ordered, func(i, j int) bool {
		left, right := depth(ordered[i]), depth(ordered[j])
		if left != right {
			return left > right
		}
		return ordered[i] < ordered[j]
	})
	return ordered
}

func mapKeysGrafanaFolders(values map[string]grafana.SearchItem) []string {
	keys := make([]string, 0, len(values))
	for uid := range values {
		keys = append(keys, uid)
	}
	return keys
}

func grafanaFolderHasContents(search []grafana.SearchItem, folderUID string) bool {
	for _, item := range search {
		if item.Type == "dash-db" && item.FolderUID == folderUID {
			return true
		}
		if item.Type == "dash-folder" && item.ParentUID == folderUID {
			return true
		}
	}
	return false
}

func removeGrafanaSearchItem(search []grafana.SearchItem, kind, uid string) []grafana.SearchItem {
	filtered := search[:0]
	for _, item := range search {
		if item.Type == kind && item.UID == uid {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func grafanaResultIndex(results []GrafanaObjectResult, kind, uid, status string) int {
	for index := range results {
		if results[index].ObjectKind == kind && results[index].UID == uid && results[index].Status == status {
			return index
		}
	}
	return -1
}

func digestGrafanaPlan(data *GrafanaActionData) string {
	if data == nil {
		return ""
	}
	projection := struct {
		Endpoint string
		Org      string
		Scope    string
		Path     string
		All      bool
		Results  []GrafanaObjectResult
	}{data.Endpoint, data.Organization, data.Scope, data.Path, data.All, data.Results}
	raw, _ := json.Marshal(projection)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func grafanaMutationResult(kind, uid, folderUID, action string, err error) GrafanaObjectResult {
	if err == nil {
		return GrafanaObjectResult{ObjectKind: kind, UID: uid, FolderUID: folderUID, Action: action, Status: "applied"}
	}
	return GrafanaObjectResult{
		ObjectKind: kind, UID: uid, FolderUID: folderUID, Action: action, Status: "failed",
		Message: "Grafana rejected or failed the object mutation", ResultCode: grafanaAPIResultCode(err),
	}
}

func grafanaInternalFailure(kind, uid, folderUID, action string, err error) GrafanaObjectResult {
	message := "object could not be processed safely"
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "password") {
		message = err.Error()
	}
	return GrafanaObjectResult{
		ObjectKind: kind, UID: uid, FolderUID: folderUID, Action: action, Status: "failed",
		Message: message, ResultCode: output.CodeStyGrafanaPartialFailure,
	}
}

func finishGrafanaAction(message string, data *GrafanaActionData) *output.Result {
	if data == nil {
		return output.Fail(output.CodeStyGrafanaPartialFailure, message)
	}
	sortGrafanaResults(data.Results)
	data.Failed = 0
	for _, result := range data.Results {
		if result.Status == "failed" {
			data.Failed++
		}
	}
	if data.Failed > 0 {
		return output.Fail(output.CodeStyGrafanaPartialFailure, message+" with per-object failures").WithData(data)
	}
	return output.OK(message, data)
}

func sortGrafanaResults(results []GrafanaObjectResult) {
	phaseRank := func(phase string) int {
		switch phase {
		case "load":
			return 0
		case "boot":
			return 1
		default:
			return 2
		}
	}
	rank := func(kind string) int {
		switch kind {
		case "organization":
			return 0
		case "folder":
			return 1
		case "dashboard":
			return 2
		case "preference":
			return 3
		default:
			return 4
		}
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Phase != results[j].Phase {
			return phaseRank(results[i].Phase) < phaseRank(results[j].Phase)
		}
		if results[i].ObjectKind != results[j].ObjectKind {
			return rank(results[i].ObjectKind) < rank(results[j].ObjectKind)
		}
		if results[i].FolderUID != results[j].FolderUID {
			return results[i].FolderUID < results[j].FolderUID
		}
		if results[i].UID != results[j].UID {
			return results[i].UID < results[j].UID
		}
		return results[i].Action < results[j].Action
	})
}

func grafanaAPIResultCode(err error) int {
	status := grafana.StatusCode(err)
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return output.CodeStyGrafanaPermissionDenied
	}
	if status == http.StatusNotFound {
		return output.CodeStyGrafanaResourceMissing
	}
	var netError net.Error
	if errors.As(err, &netError) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return output.CodeStyGrafanaNetworkFailed
	}
	var requestError *grafana.RequestError
	if errors.As(err, &requestError) {
		return output.CodeStyGrafanaNetworkFailed
	}
	return output.CodeStyGrafanaPartialFailure
}

func grafanaAPIFailure(message string, client grafanaAPI, err error) *output.Result {
	detail := err.Error()
	if client != nil && client.Endpoint() != "" {
		detail += " (endpoint: " + client.Endpoint() + ")"
	}
	return output.Fail(grafanaAPIResultCode(err), message).WithDetail(detail)
}

func grafanaSetupFailure(message string, err error) *output.Result {
	if err == nil {
		return output.Fail(output.CodeStyGrafanaConfigInvalid, message)
	}
	text := err.Error()
	code := output.CodeStyGrafanaConfigInvalid
	if strings.Contains(text, "required") || strings.Contains(text, "requires") || strings.Contains(text, "must") ||
		strings.Contains(text, "invalid") || strings.Contains(text, "unsupported") || strings.Contains(text, "unsafe") ||
		strings.Contains(text, "mutually exclusive") {
		code = output.CodeStyGrafanaInvalidArgs
	}
	if strings.Contains(text, "unavailable") || strings.Contains(text, "does not exist") || strings.Contains(text, "no such file") {
		code = output.CodeStyGrafanaResourceMissing
	}
	return output.Fail(code, message).WithDetail(text)
}

func emptyGrafanaText(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}
