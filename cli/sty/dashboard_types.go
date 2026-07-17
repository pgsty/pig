package sty

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"

	"pig/internal/grafana"
	"pig/internal/output"
)

const (
	DashboardListKind  = "pig.sty.dashboard.list/v1"
	DashboardCheckKind = "pig.sty.dashboard.check/v1"
	DashboardDiffKind  = "pig.sty.dashboard.diff/v1"
	DashboardInitKind  = "pig.sty.dashboard.init/v1"
	DashboardDumpKind  = "pig.sty.dashboard.dump/v1"
	DashboardCleanKind = "pig.sty.dashboard.clean/v1"
)

type DashboardObject struct {
	ObjectKind  string `json:"object_kind" yaml:"object_kind"`
	UID         string `json:"uid" yaml:"uid"`
	Title       string `json:"title,omitempty" yaml:"title,omitempty"`
	FolderUID   string `json:"folder_uid,omitempty" yaml:"folder_uid,omitempty"`
	Managed     bool   `json:"managed" yaml:"managed"`
	Tombstoned  bool   `json:"tombstoned,omitempty" yaml:"tombstoned,omitempty"`
	Repository  bool   `json:"repository" yaml:"repository"`
	Live        bool   `json:"live" yaml:"live"`
	Provisioned bool   `json:"provisioned,omitempty" yaml:"provisioned,omitempty"`
	Status      string `json:"status" yaml:"status"`
}

type DashboardListData struct {
	Kind             string            `json:"kind" yaml:"kind"`
	Endpoint         string            `json:"endpoint" yaml:"endpoint"`
	Manifest         string            `json:"manifest" yaml:"manifest"`
	ManifestRevision int               `json:"manifest_revision" yaml:"manifest_revision"`
	Objects          []DashboardObject `json:"objects" yaml:"objects"`
}

func (d *DashboardListData) Text() string {
	if d == nil {
		return ""
	}
	rows := make([][]string, 0, len(d.Objects))
	for _, object := range d.Objects {
		rows = append(rows, []string{object.ObjectKind, object.UID, object.FolderUID, object.Status})
	}
	return strings.TrimSuffix(output.RenderTable([]string{"KIND", "UID", "FOLDER", "STATUS"}, rows), "\n")
}

type DashboardCheckData struct {
	Kind             string `json:"kind" yaml:"kind"`
	Endpoint         string `json:"endpoint" yaml:"endpoint"`
	Manifest         string `json:"manifest" yaml:"manifest"`
	ManifestRevision int    `json:"manifest_revision" yaml:"manifest_revision"`
	Normalization    string `json:"normalization" yaml:"normalization"`
	DashboardCount   int    `json:"dashboard_count" yaml:"dashboard_count"`
	DatasourceCount  int    `json:"datasource_count" yaml:"datasource_count"`
	GrafanaVersion   string `json:"grafana_version,omitempty" yaml:"grafana_version,omitempty"`
	Organization     string `json:"organization,omitempty" yaml:"organization,omitempty"`
	Compatible       bool   `json:"compatible" yaml:"compatible"`
}

func (d *DashboardCheckData) Text() string {
	if d == nil {
		return ""
	}
	return fmt.Sprintf("  manifest: revision %d, %d dashboards, %d datasources\n  Grafana: %s, organization: %s",
		d.ManifestRevision, d.DashboardCount, d.DatasourceCount, emptyDashboardText(d.GrafanaVersion), emptyDashboardText(d.Organization))
}

type DashboardDiffItem struct {
	ObjectKind  string   `json:"object_kind" yaml:"object_kind"`
	UID         string   `json:"uid" yaml:"uid"`
	Status      string   `json:"status" yaml:"status"`
	FolderUID   string   `json:"folder_uid,omitempty" yaml:"folder_uid,omitempty"`
	Fields      []string `json:"fields,omitempty" yaml:"fields,omitempty"`
	DesiredHash string   `json:"desired_hash,omitempty" yaml:"desired_hash,omitempty"`
	LiveHash    string   `json:"live_hash,omitempty" yaml:"live_hash,omitempty"`
	Provisioned bool     `json:"provisioned,omitempty" yaml:"provisioned,omitempty"`
	ResultCode  int      `json:"result_code,omitempty" yaml:"result_code,omitempty"`
}

type DashboardDiffData struct {
	Kind             string              `json:"kind" yaml:"kind"`
	Endpoint         string              `json:"endpoint" yaml:"endpoint"`
	Manifest         string              `json:"manifest" yaml:"manifest"`
	ManifestRevision int                 `json:"manifest_revision" yaml:"manifest_revision"`
	Drift            bool                `json:"drift" yaml:"drift"`
	Differences      []DashboardDiffItem `json:"differences" yaml:"differences"`
}

func (d *DashboardDiffData) Text() string {
	if d == nil || len(d.Differences) == 0 {
		return ""
	}
	rows := make([][]string, 0, len(d.Differences))
	for _, difference := range d.Differences {
		rows = append(rows, []string{difference.ObjectKind, difference.UID, difference.Status, strings.Join(difference.Fields, ",")})
	}
	return strings.TrimSuffix(output.RenderTable([]string{"KIND", "UID", "STATUS", "FIELDS"}, rows), "\n")
}

type DashboardObjectResult struct {
	ObjectKind string `json:"object_kind" yaml:"object_kind"`
	UID        string `json:"uid" yaml:"uid"`
	Action     string `json:"action" yaml:"action"`
	Status     string `json:"status" yaml:"status"`
	Message    string `json:"message,omitempty" yaml:"message,omitempty"`
	ResultCode int    `json:"result_code,omitempty" yaml:"result_code,omitempty"`
}

type DashboardActionData struct {
	Kind             string                  `json:"kind" yaml:"kind"`
	Endpoint         string                  `json:"endpoint" yaml:"endpoint"`
	Manifest         string                  `json:"manifest" yaml:"manifest"`
	ManifestRevision int                     `json:"manifest_revision" yaml:"manifest_revision"`
	DryRun           bool                    `json:"dry_run,omitempty" yaml:"dry_run,omitempty"`
	Prune            bool                    `json:"prune,omitempty" yaml:"prune,omitempty"`
	Results          []DashboardObjectResult `json:"results" yaml:"results"`
	Failed           int                     `json:"failed" yaml:"failed"`
}

func (d *DashboardActionData) Text() string {
	if d == nil || len(d.Results) == 0 {
		return ""
	}
	rows := make([][]string, 0, len(d.Results))
	for _, result := range d.Results {
		rows = append(rows, []string{result.ObjectKind, result.UID, result.Action, result.Status, result.Message})
	}
	return strings.TrimSuffix(output.RenderTable([]string{"KIND", "UID", "ACTION", "STATUS", "MESSAGE"}, rows), "\n")
}

type DashboardDumpData struct {
	Kind               string                  `json:"kind" yaml:"kind"`
	Endpoint           string                  `json:"endpoint" yaml:"endpoint"`
	Manifest           string                  `json:"manifest" yaml:"manifest"`
	ManifestRevision   int                     `json:"manifest_revision" yaml:"manifest_revision"`
	Destination        string                  `json:"destination" yaml:"destination"`
	DashboardCount     int                     `json:"dashboard_count" yaml:"dashboard_count"`
	DatasourcesSkipped int                     `json:"datasources_skipped" yaml:"datasources_skipped"`
	Results            []DashboardObjectResult `json:"results" yaml:"results"`
}

func (d *DashboardDumpData) Text() string {
	if d == nil {
		return ""
	}
	return fmt.Sprintf("  destination: %s\n  dashboards: %d; datasources intentionally skipped: %d", d.Destination, d.DashboardCount, d.DatasourcesSkipped)
}

type dashboardLiveState struct {
	Dashboards  map[string]grafana.SearchItem
	Folders     map[string]grafana.SearchItem
	Datasources map[string]map[string]interface{}
	Search      []grafana.SearchItem
}

func fetchDashboardLiveState(ctx context.Context, client *grafana.Client) (*dashboardLiveState, error) {
	search, err := client.Search(ctx)
	if err != nil {
		return nil, err
	}
	datasources, err := client.ListDatasources(ctx)
	if err != nil {
		return nil, err
	}
	state := &dashboardLiveState{
		Dashboards: make(map[string]grafana.SearchItem), Folders: make(map[string]grafana.SearchItem),
		Datasources: make(map[string]map[string]interface{}), Search: search,
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
	for _, datasource := range datasources {
		uid, _ := datasource["uid"].(string)
		if uid != "" {
			state.Datasources[uid] = datasource
		}
	}
	return state, nil
}

func dashboardAPIResultCode(err error) int {
	status := grafana.StatusCode(err)
	if status == 401 || status == 403 {
		return output.CodeStyDashboardPermissionDenied
	}
	var netError net.Error
	if errors.As(err, &netError) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return output.CodeStyDashboardNetworkFailed
	}
	var requestError *grafana.RequestError
	if errors.As(err, &requestError) {
		return output.CodeStyDashboardNetworkFailed
	}
	return output.CodeStyDashboardPartialFailure
}

func dashboardAPIFailure(message string, err error) *output.Result {
	return output.Fail(dashboardAPIResultCode(err), message)
}

func dashboardSetupFailure(message string, err error) *output.Result {
	if err == nil {
		return output.Fail(output.CodeStyDashboardConfigInvalid, message)
	}
	text := err.Error()
	code := output.CodeStyDashboardConfigInvalid
	if strings.Contains(text, "unavailable") || strings.Contains(text, "not configured") || strings.Contains(text, "does not exist") || strings.Contains(text, "no such file") {
		code = output.CodeStyDashboardResourceMissing
	}
	return output.Fail(code, message).WithDetail(text)
}

func sortDashboardObjects(objects []DashboardObject) {
	sort.Slice(objects, func(i, j int) bool {
		if objects[i].ObjectKind != objects[j].ObjectKind {
			return dashboardObjectKindRank(objects[i].ObjectKind) < dashboardObjectKindRank(objects[j].ObjectKind)
		}
		return objects[i].UID < objects[j].UID
	})
}

func sortDashboardDiffs(differences []DashboardDiffItem) {
	sort.Slice(differences, func(i, j int) bool {
		if differences[i].ObjectKind != differences[j].ObjectKind {
			return dashboardObjectKindRank(differences[i].ObjectKind) < dashboardObjectKindRank(differences[j].ObjectKind)
		}
		return differences[i].UID < differences[j].UID
	})
}

func sortDashboardResults(results []DashboardObjectResult) {
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].ObjectKind != results[j].ObjectKind {
			return dashboardObjectKindRank(results[i].ObjectKind) < dashboardObjectKindRank(results[j].ObjectKind)
		}
		if results[i].UID != results[j].UID {
			return results[i].UID < results[j].UID
		}
		return results[i].Action < results[j].Action
	})
}

func dashboardObjectKindRank(kind string) int {
	switch kind {
	case "dashboard":
		return 0
	case "datasource":
		return 1
	case "folder":
		return 2
	default:
		return 3
	}
}

func dashboardString(value interface{}) string {
	text, _ := value.(string)
	return text
}

func emptyDashboardText(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}
