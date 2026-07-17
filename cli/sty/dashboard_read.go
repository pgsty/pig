package sty

import (
	"context"
	"fmt"
	"sort"

	"pig/internal/output"
)

func DashboardCheck(options DashboardOptions) *output.Result {
	prepared, err := prepareDashboard(options, true)
	if err != nil {
		return dashboardSetupFailure("dashboard check setup failed", err)
	}
	health, err := prepared.client.Health(context.Background())
	if err != nil {
		return dashboardAPIFailure("Grafana health check failed", err)
	}
	organization, err := prepared.client.CurrentOrganization(context.Background())
	if err != nil {
		return dashboardAPIFailure("Grafana credential or organization check failed", err)
	}
	compatible := health.Version != "" && organization.ID > 0
	data := &DashboardCheckData{
		Kind: DashboardCheckKind, Endpoint: prepared.client.Endpoint(), Manifest: prepared.manifest.Source,
		ManifestRevision: prepared.manifest.Manifest.Revision, Normalization: prepared.manifest.Manifest.Normalization,
		DashboardCount: len(prepared.manifest.Manifest.Dashboards), DatasourceCount: len(prepared.manifest.Manifest.Datasources),
		GrafanaVersion: health.Version, Organization: organization.Name, Compatible: compatible,
	}
	if !compatible {
		return output.Fail(output.CodeStyDashboardConfigInvalid, "Grafana API response is incompatible").WithData(data)
	}
	return output.OK("dashboard check passed", data)
}

func DashboardList(options DashboardOptions) *output.Result {
	prepared, err := prepareDashboard(options, true)
	if err != nil {
		return dashboardSetupFailure("dashboard list setup failed", err)
	}
	state, err := fetchDashboardLiveState(context.Background(), prepared.client)
	if err != nil {
		return dashboardAPIFailure("Grafana resources could not be listed", err)
	}
	manifest := prepared.manifest.Manifest
	objects := make([]DashboardObject, 0, len(manifest.Dashboards)+len(manifest.Datasources)+len(state.Search))
	activeDashboards := make(map[string]struct{}, len(manifest.Dashboards))
	for _, item := range manifest.Dashboards {
		activeDashboards[item.UID] = struct{}{}
		live, exists := state.Dashboards[item.UID]
		status := "missing"
		folderUID := item.FolderUID
		if exists {
			folderUID = live.FolderUID
			status = "live"
			if live.FolderUID != item.FolderUID {
				status = "moved"
			}
		}
		objects = append(objects, DashboardObject{
			ObjectKind: "dashboard", UID: item.UID, Title: item.Title, FolderUID: folderUID,
			Managed: true, Repository: true, Live: exists, Status: status,
		})
	}
	tombstoneDashboards := make(map[string]struct{}, len(manifest.RemovedDashboards))
	for _, tombstone := range manifest.RemovedDashboards {
		tombstoneDashboards[tombstone.UID] = struct{}{}
		live, exists := state.Dashboards[tombstone.UID]
		objects = append(objects, DashboardObject{
			ObjectKind: "dashboard", UID: tombstone.UID, Title: live.Title, FolderUID: live.FolderUID,
			Tombstoned: true, Repository: true, Live: exists, Status: map[bool]string{true: "removed_live", false: "removed"}[exists],
		})
	}
	for uid, live := range state.Dashboards {
		if _, exists := activeDashboards[uid]; exists {
			continue
		}
		if _, exists := tombstoneDashboards[uid]; exists {
			continue
		}
		objects = append(objects, DashboardObject{
			ObjectKind: "dashboard", UID: uid, Title: live.Title, FolderUID: live.FolderUID,
			Live: true, Status: "unmanaged",
		})
	}

	activeDatasources := make(map[string]struct{}, len(manifest.Datasources))
	for _, item := range manifest.Datasources {
		activeDatasources[item.UID] = struct{}{}
		live, exists := state.Datasources[item.UID]
		objects = append(objects, DashboardObject{
			ObjectKind: "datasource", UID: item.UID, Title: item.Name, Managed: true,
			Repository: true, Live: exists, Provisioned: dashboardDatasourceReadOnly(live),
			Status: map[bool]string{true: "live", false: "missing"}[exists],
		})
	}
	tombstoneDatasources := make(map[string]struct{}, len(manifest.RemovedDatasources))
	for _, tombstone := range manifest.RemovedDatasources {
		tombstoneDatasources[tombstone.UID] = struct{}{}
		live, exists := state.Datasources[tombstone.UID]
		objects = append(objects, DashboardObject{
			ObjectKind: "datasource", UID: tombstone.UID, Title: dashboardString(live["name"]),
			Tombstoned: true, Repository: true, Live: exists, Provisioned: dashboardDatasourceReadOnly(live),
			Status: map[bool]string{true: "removed_live", false: "removed"}[exists],
		})
	}
	for uid, live := range state.Datasources {
		if _, exists := activeDatasources[uid]; exists {
			continue
		}
		if _, exists := tombstoneDatasources[uid]; exists {
			continue
		}
		objects = append(objects, DashboardObject{
			ObjectKind: "datasource", UID: uid, Title: dashboardString(live["name"]), Live: true,
			Provisioned: dashboardDatasourceReadOnly(live), Status: "unmanaged",
		})
	}

	managedFolders := dashboardManifestFolders(manifest)
	for uid, title := range managedFolders {
		live, exists := state.Folders[uid]
		status := "missing"
		if exists {
			status = "managed_placement"
			if live.Title != title {
				status = "changed"
			}
		}
		objects = append(objects, DashboardObject{
			ObjectKind: "folder", UID: uid, Title: title, Repository: true,
			Live: exists, Status: status,
		})
	}
	for uid, folder := range state.Folders {
		if _, placement := managedFolders[uid]; placement {
			continue
		}
		objects = append(objects, DashboardObject{
			ObjectKind: "folder", UID: uid, Title: folder.Title,
			Live: true, Status: "unmanaged",
		})
	}
	sortDashboardObjects(objects)
	data := &DashboardListData{
		Kind: DashboardListKind, Endpoint: prepared.client.Endpoint(), Manifest: prepared.manifest.Source,
		ManifestRevision: manifest.Revision, Objects: objects,
	}
	return output.OK("dashboard resources listed", data)
}

func DashboardDiff(options DashboardOptions) *output.Result {
	prepared, err := prepareDashboard(options, true)
	if err != nil {
		return dashboardSetupFailure("dashboard diff setup failed", err)
	}
	data, err := buildDashboardDiff(context.Background(), prepared)
	if err != nil {
		return dashboardAPIFailure("dashboard diff could not read Grafana state", err)
	}
	for _, difference := range data.Differences {
		if difference.ResultCode != 0 {
			return output.Fail(output.CodeStyDashboardPartialFailure, "dashboard diff completed with object failures").WithData(data)
		}
	}
	if data.Drift {
		return output.Fail(output.CodeStyDashboardDrift, "dashboard drift detected").WithData(data)
	}
	return output.OK("dashboards are in sync", data)
}

func buildDashboardDiff(ctx context.Context, prepared *dashboardPrepared) (*DashboardDiffData, error) {
	assets, err := loadDashboardRepositoryAssets(prepared.manifest)
	if err != nil {
		return nil, err
	}
	state, err := fetchDashboardLiveState(ctx, prepared.client)
	if err != nil {
		return nil, err
	}
	manifest := prepared.manifest.Manifest
	differences := make([]DashboardDiffItem, 0)
	for _, folder := range sortedDashboardManifestFolders(manifest) {
		live, exists := state.Folders[folder.FolderUID]
		if !exists {
			differences = append(differences, DashboardDiffItem{ObjectKind: "folder", UID: folder.FolderUID, Status: "missing", Fields: []string{"/title"}})
		} else if live.Title != folder.FolderTitle {
			differences = append(differences, DashboardDiffItem{ObjectKind: "folder", UID: folder.FolderUID, Status: "changed", Fields: []string{"/title"}})
		}
	}
	for _, asset := range assets {
		if _, exists := state.Dashboards[asset.Manifest.UID]; !exists {
			differences = append(differences, DashboardDiffItem{ObjectKind: "dashboard", UID: asset.Manifest.UID, Status: "missing", FolderUID: asset.Manifest.FolderUID})
			continue
		}
		desired, err := normalizeDashboardJSON(asset.Value)
		if err != nil {
			return nil, fmt.Errorf("normalize repository dashboard %s: %w", asset.Manifest.UID, err)
		}
		live, err := prepared.client.GetDashboard(ctx, asset.Manifest.UID)
		if err != nil {
			differences = append(differences, DashboardDiffItem{
				ObjectKind: "dashboard", UID: asset.Manifest.UID, Status: "error",
				ResultCode: dashboardAPIResultCode(err),
			})
			continue
		}
		actual, err := normalizeDashboardJSON(live.Dashboard)
		if err != nil {
			differences = append(differences, DashboardDiffItem{
				ObjectKind: "dashboard", UID: asset.Manifest.UID, Status: "error",
				ResultCode: output.CodeStyDashboardPartialFailure,
			})
			continue
		}
		fields := diffJSONPointers(desired.Value, actual.Value)
		if live.Meta.FolderUID != asset.Manifest.FolderUID {
			fields = append(fields, "/meta/folderUid")
		}
		if len(fields) > 0 {
			sort.Strings(fields)
			differences = append(differences, DashboardDiffItem{
				ObjectKind: "dashboard", UID: asset.Manifest.UID, Status: "changed", FolderUID: live.Meta.FolderUID,
				Fields: fields, DesiredHash: normalizedDigest(desired.Canonical), LiveHash: normalizedDigest(actual.Canonical),
				Provisioned: live.Meta.Provisioned,
			})
		}
	}
	for _, item := range manifest.Datasources {
		live, exists := state.Datasources[item.UID]
		if !exists {
			differences = append(differences, DashboardDiffItem{ObjectKind: "datasource", UID: item.UID, Status: "missing"})
			continue
		}
		desired, err := desiredDatasourceValue(item)
		if err != nil {
			return nil, err
		}
		desiredCanonical, liveCanonical, fields, err := normalizeDatasourceAgainst(desired, live)
		if err != nil {
			return nil, err
		}
		if len(fields) > 0 {
			differences = append(differences, DashboardDiffItem{
				ObjectKind: "datasource", UID: item.UID, Status: "changed", Fields: fields,
				DesiredHash: normalizedDigest(desiredCanonical), LiveHash: normalizedDigest(liveCanonical),
				Provisioned: dashboardDatasourceReadOnly(live),
			})
		}
	}
	for _, tombstone := range manifest.RemovedDashboards {
		if live, exists := state.Dashboards[tombstone.UID]; exists {
			differences = append(differences, DashboardDiffItem{ObjectKind: "dashboard", UID: tombstone.UID, Status: "removed_live", FolderUID: live.FolderUID})
		}
	}
	for _, tombstone := range manifest.RemovedDatasources {
		if live, exists := state.Datasources[tombstone.UID]; exists {
			differences = append(differences, DashboardDiffItem{ObjectKind: "datasource", UID: tombstone.UID, Status: "removed_live", Provisioned: dashboardDatasourceReadOnly(live)})
		}
	}
	sortDashboardDiffs(differences)
	return &DashboardDiffData{
		Kind: DashboardDiffKind, Endpoint: prepared.client.Endpoint(), Manifest: prepared.manifest.Source,
		ManifestRevision: manifest.Revision, Drift: len(differences) > 0, Differences: differences,
	}, nil
}

func dashboardDatasourceReadOnly(value map[string]interface{}) bool {
	if value == nil {
		return false
	}
	readOnly, _ := value["readOnly"].(bool)
	return readOnly
}
