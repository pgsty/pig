package sty

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"pig/internal/grafana"
	"pig/internal/output"
)

type dashboardDumpFile struct {
	item DashboardManifestDashboard
	raw  []byte
}

func DashboardInit(options DashboardOptions) *output.Result {
	prepared, err := prepareDashboard(options, true)
	if err != nil {
		return dashboardSetupFailure("dashboard init setup failed", err)
	}
	state, err := fetchDashboardLiveState(context.Background(), prepared.client)
	if err != nil {
		return dashboardAPIFailure("Grafana state could not be read before dashboard init", err)
	}
	if options.Prune && !options.Yes {
		data := dashboardPrunePlan(prepared, state)
		return output.Fail(output.CodeStyDashboardConfirmationRequired, "dashboard init --prune requires explicit confirmation").
			WithData(data).
			WithNextActions(
				output.NextAction{Command: "pig sty dashboard init --prune --yes", Reason: "apply the displayed tombstone deletion plan", Required: true},
				output.NextAction{Command: "pig sty dashboard diff", Reason: "review managed Grafana drift", Required: false},
			)
	}
	assets, err := loadDashboardRepositoryAssets(prepared.manifest)
	if err != nil {
		return dashboardSetupFailure("dashboard repository assets are invalid", err)
	}
	data := &DashboardActionData{
		Kind: DashboardInitKind, Endpoint: prepared.client.Endpoint(), Manifest: prepared.manifest.Source,
		ManifestRevision: prepared.manifest.Manifest.Revision, Prune: options.Prune, Results: []DashboardObjectResult{},
	}
	ctx := context.Background()

	for _, folder := range sortedDashboardManifestFolders(prepared.manifest.Manifest) {
		live, exists := state.Folders[folder.FolderUID]
		if !exists {
			err := prepared.client.CreateFolder(ctx, folder.FolderUID, folder.FolderTitle)
			data.Results = append(data.Results, dashboardMutationResult("folder", folder.FolderUID, "create", err, false))
			continue
		}
		if live.Title != folder.FolderTitle {
			err := prepared.client.UpdateFolder(ctx, folder.FolderUID, folder.FolderTitle)
			data.Results = append(data.Results, dashboardMutationResult("folder", folder.FolderUID, "update", err, false))
		} else {
			data.Results = append(data.Results, dashboardUnchangedResult("folder", folder.FolderUID, "upsert"))
		}
	}

	for _, asset := range assets {
		liveIndex, exists := state.Dashboards[asset.Manifest.UID]
		needsUpdate := !exists
		provisioned := false
		if exists {
			live, getErr := prepared.client.GetDashboard(ctx, asset.Manifest.UID)
			if getErr != nil {
				data.Results = append(data.Results, dashboardMutationResult("dashboard", asset.Manifest.UID, "inspect", getErr, false))
				continue
			}
			provisioned = live.Meta.Provisioned
			desired, normalizeErr := normalizeDashboardJSON(asset.Value)
			if normalizeErr != nil {
				data.Results = append(data.Results, dashboardInternalObjectFailure("dashboard", asset.Manifest.UID, "normalize"))
				continue
			}
			actual, normalizeErr := normalizeDashboardJSON(live.Dashboard)
			if normalizeErr != nil {
				data.Results = append(data.Results, dashboardInternalObjectFailure("dashboard", asset.Manifest.UID, "normalize"))
				continue
			}
			needsUpdate = string(desired.Canonical) != string(actual.Canonical) || live.Meta.FolderUID != asset.Manifest.FolderUID || liveIndex.FolderUID != asset.Manifest.FolderUID
		}
		if !needsUpdate {
			data.Results = append(data.Results, dashboardUnchangedResult("dashboard", asset.Manifest.UID, "upsert"))
			continue
		}
		dashboard, cloneErr := cloneJSONObject(asset.Value)
		if cloneErr != nil {
			data.Results = append(data.Results, dashboardInternalObjectFailure("dashboard", asset.Manifest.UID, "prepare"))
			continue
		}
		dashboard["id"] = nil
		action := "create"
		if exists {
			action = "update"
		}
		mutationErr := prepared.client.UpsertDashboard(ctx, dashboard, asset.Manifest.FolderUID)
		data.Results = append(data.Results, dashboardMutationResult("dashboard", asset.Manifest.UID, action, mutationErr, provisioned))
	}

	for _, item := range prepared.manifest.Manifest.Datasources {
		live, exists := state.Datasources[item.UID]
		payload, _, payloadErr := dashboardDatasourcePayload(item, prepared.inventory, !exists)
		if payloadErr != nil {
			data.Results = append(data.Results, DashboardObjectResult{
				ObjectKind: "datasource", UID: item.UID, Action: map[bool]string{true: "update", false: "create"}[exists],
				Status: "failed", Message: "required protected datasource input is unavailable", ResultCode: output.CodeStyDashboardConfigInvalid,
			})
			continue
		}
		needsUpdate := !exists
		if exists {
			desired, desiredErr := desiredDatasourceValue(item)
			if desiredErr != nil {
				data.Results = append(data.Results, dashboardInternalObjectFailure("datasource", item.UID, "normalize"))
				continue
			}
			_, _, fields, compareErr := normalizeDatasourceAgainst(desired, live)
			if compareErr != nil {
				data.Results = append(data.Results, dashboardInternalObjectFailure("datasource", item.UID, "normalize"))
				continue
			}
			needsUpdate = len(fields) > 0
		}
		if !needsUpdate {
			data.Results = append(data.Results, dashboardUnchangedResult("datasource", item.UID, "upsert"))
			continue
		}
		action := "create"
		var mutationErr error
		if exists {
			action = "update"
			mutationErr = prepared.client.UpdateDatasource(ctx, item.UID, payload)
		} else {
			mutationErr = prepared.client.CreateDatasource(ctx, payload)
		}
		data.Results = append(data.Results, dashboardMutationResult("datasource", item.UID, action, mutationErr, dashboardDatasourceReadOnly(live)))
	}

	if options.Prune {
		applyDashboardTombstonePrune(ctx, prepared, state, data)
	}
	return finishDashboardAction("dashboard init completed", data)
}

func DashboardClean(options DashboardOptions) *output.Result {
	prepared, err := prepareDashboard(options, true)
	if err != nil {
		return dashboardSetupFailure("dashboard clean setup failed", err)
	}
	state, err := fetchDashboardLiveState(context.Background(), prepared.client)
	if err != nil {
		return dashboardAPIFailure("Grafana state could not be read before dashboard clean", err)
	}
	if !options.Yes {
		data := dashboardCleanPlan(prepared, state)
		return output.Fail(output.CodeStyDashboardConfirmationRequired, "dashboard clean requires explicit confirmation").
			WithData(data).
			WithNextActions(
				output.NextAction{Command: "pig sty dashboard clean --yes", Reason: "delete only active manifest-owned resources in the displayed plan", Required: true},
				output.NextAction{Command: "pig sty dashboard list", Reason: "review managed and unmanaged Grafana resources", Required: false},
			)
	}
	data := &DashboardActionData{
		Kind: DashboardCleanKind, Endpoint: prepared.client.Endpoint(), Manifest: prepared.manifest.Source,
		ManifestRevision: prepared.manifest.Manifest.Revision, Results: []DashboardObjectResult{},
	}
	ctx := context.Background()
	for _, item := range prepared.manifest.Manifest.Dashboards {
		live, exists := state.Dashboards[item.UID]
		if !exists {
			data.Results = append(data.Results, dashboardUnchangedResult("dashboard", item.UID, "delete"))
			continue
		}
		envelope, getErr := prepared.client.GetDashboard(ctx, item.UID)
		provisioned := false
		if getErr == nil {
			provisioned = envelope.Meta.Provisioned
		}
		deleteErr := prepared.client.DeleteDashboard(ctx, live.UID)
		data.Results = append(data.Results, dashboardMutationResult("dashboard", item.UID, "delete", deleteErr, provisioned))
	}
	for _, item := range prepared.manifest.Manifest.Datasources {
		live, exists := state.Datasources[item.UID]
		if !exists {
			data.Results = append(data.Results, dashboardUnchangedResult("datasource", item.UID, "delete"))
			continue
		}
		deleteErr := prepared.client.DeleteDatasource(ctx, item.UID)
		data.Results = append(data.Results, dashboardMutationResult("datasource", item.UID, "delete", deleteErr, dashboardDatasourceReadOnly(live)))
	}
	deleteEmptyManagedFolders(ctx, prepared, dashboardManifestFolders(prepared.manifest.Manifest), data)
	return finishDashboardAction("dashboard clean completed", data)
}

func DashboardDump(options DashboardOptions) *output.Result {
	prepared, err := prepareDashboard(options, true)
	if err != nil {
		return dashboardSetupFailure("dashboard dump setup failed", err)
	}
	destination := strings.TrimSpace(options.DumpDir)
	if destination == "" {
		return output.Fail(output.CodeStyDashboardInvalidArgs, "dashboard dump requires --dir")
	}
	destination, err = filepath.Abs(destination)
	if err != nil {
		return output.Fail(output.CodeStyDashboardInvalidArgs, "dashboard dump destination is invalid")
	}
	if _, err := os.Lstat(destination); err == nil {
		return output.Fail(output.CodeStyDashboardInvalidArgs, "dashboard dump destination already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return output.Fail(output.CodeStyDashboardResourceMissing, "dashboard dump destination cannot be inspected")
	}
	parent := filepath.Dir(destination)
	parentInfo, err := os.Stat(parent)
	if err != nil || !parentInfo.IsDir() {
		return output.Fail(output.CodeStyDashboardResourceMissing, "dashboard dump parent directory is unavailable")
	}

	files := make([]dashboardDumpFile, 0, len(prepared.manifest.Manifest.Dashboards))
	results := make([]DashboardObjectResult, 0, len(prepared.manifest.Manifest.Dashboards))
	ctx := context.Background()
	for _, item := range prepared.manifest.Manifest.Dashboards {
		live, getErr := prepared.client.GetDashboard(ctx, item.UID)
		if getErr != nil {
			results = append(results, dashboardMutationResult("dashboard", item.UID, "export", getErr, false))
			continue
		}
		normalized, normalizeErr := normalizeDashboardJSON(live.Dashboard)
		if normalizeErr != nil {
			results = append(results, dashboardInternalObjectFailure("dashboard", item.UID, "export"))
			continue
		}
		pretty, marshalErr := json.MarshalIndent(normalized.Value, "", "  ")
		if marshalErr != nil {
			results = append(results, dashboardInternalObjectFailure("dashboard", item.UID, "export"))
			continue
		}
		files = append(files, dashboardDumpFile{item: item, raw: append(pretty, '\n')})
		results = append(results, DashboardObjectResult{ObjectKind: "dashboard", UID: item.UID, Action: "export", Status: "staged"})
	}
	data := &DashboardDumpData{
		Kind: DashboardDumpKind, Endpoint: prepared.client.Endpoint(), Manifest: prepared.manifest.Source,
		ManifestRevision: prepared.manifest.Manifest.Revision, Destination: destination,
		DashboardCount: len(files), DatasourcesSkipped: len(prepared.manifest.Manifest.Datasources), Results: results,
	}
	if len(files) != len(prepared.manifest.Manifest.Dashboards) {
		return output.Fail(output.CodeStyDashboardPartialFailure, "dashboard dump fetch failed; destination was not created").WithData(data)
	}
	if err := writeAtomicDashboardDump(parent, destination, prepared.manifest.Manifest, files); err != nil {
		return output.Fail(output.CodeStyDashboardDumpFailed, "dashboard dump staging failed; destination was not committed").WithData(data)
	}
	for index := range data.Results {
		data.Results[index].Status = "exported"
	}
	return output.OK("dashboards exported atomically; datasource credentials were not dumped", data)
}

func dashboardPrunePlan(prepared *dashboardPrepared, state *dashboardLiveState) *DashboardActionData {
	data := &DashboardActionData{
		Kind: DashboardInitKind, Endpoint: prepared.client.Endpoint(), Manifest: prepared.manifest.Source,
		ManifestRevision: prepared.manifest.Manifest.Revision, DryRun: true, Prune: true, Results: []DashboardObjectResult{},
	}
	activeFolders := dashboardManifestFolders(prepared.manifest.Manifest)
	candidateFolders := make(map[string]struct{})
	for _, tombstone := range prepared.manifest.Manifest.RemovedDashboards {
		if _, exists := state.Dashboards[tombstone.UID]; exists {
			data.Results = append(data.Results, DashboardObjectResult{ObjectKind: "dashboard", UID: tombstone.UID, Action: "delete", Status: "planned"})
		}
		if tombstone.FolderUID != "" {
			if _, active := activeFolders[tombstone.FolderUID]; !active {
				candidateFolders[tombstone.FolderUID] = struct{}{}
			}
		}
	}
	for _, tombstone := range prepared.manifest.Manifest.RemovedDatasources {
		if _, exists := state.Datasources[tombstone.UID]; exists {
			data.Results = append(data.Results, DashboardObjectResult{ObjectKind: "datasource", UID: tombstone.UID, Action: "delete", Status: "planned"})
		}
	}
	for uid := range candidateFolders {
		if _, exists := state.Folders[uid]; exists {
			data.Results = append(data.Results, DashboardObjectResult{ObjectKind: "folder", UID: uid, Action: "delete-if-empty", Status: "planned"})
		}
	}
	sortDashboardResults(data.Results)
	return data
}

func dashboardCleanPlan(prepared *dashboardPrepared, state *dashboardLiveState) *DashboardActionData {
	data := &DashboardActionData{
		Kind: DashboardCleanKind, Endpoint: prepared.client.Endpoint(), Manifest: prepared.manifest.Source,
		ManifestRevision: prepared.manifest.Manifest.Revision, DryRun: true, Results: []DashboardObjectResult{},
	}
	for _, item := range prepared.manifest.Manifest.Dashboards {
		if _, exists := state.Dashboards[item.UID]; exists {
			data.Results = append(data.Results, DashboardObjectResult{ObjectKind: "dashboard", UID: item.UID, Action: "delete", Status: "planned"})
		}
	}
	for _, item := range prepared.manifest.Manifest.Datasources {
		if _, exists := state.Datasources[item.UID]; exists {
			data.Results = append(data.Results, DashboardObjectResult{ObjectKind: "datasource", UID: item.UID, Action: "delete", Status: "planned"})
		}
	}
	for uid := range dashboardManifestFolders(prepared.manifest.Manifest) {
		if _, exists := state.Folders[uid]; exists {
			data.Results = append(data.Results, DashboardObjectResult{ObjectKind: "folder", UID: uid, Action: "delete-if-empty", Status: "planned"})
		}
	}
	sortDashboardResults(data.Results)
	return data
}

func applyDashboardTombstonePrune(ctx context.Context, prepared *dashboardPrepared, state *dashboardLiveState, data *DashboardActionData) {
	activeFolders := dashboardManifestFolders(prepared.manifest.Manifest)
	candidateFolders := make(map[string]string)
	for _, tombstone := range prepared.manifest.Manifest.RemovedDashboards {
		if live, exists := state.Dashboards[tombstone.UID]; exists {
			envelope, getErr := prepared.client.GetDashboard(ctx, tombstone.UID)
			provisioned := getErr == nil && envelope.Meta.Provisioned
			deleteErr := prepared.client.DeleteDashboard(ctx, live.UID)
			data.Results = append(data.Results, dashboardMutationResult("dashboard", tombstone.UID, "prune", deleteErr, provisioned))
		}
		if tombstone.FolderUID != "" {
			if _, active := activeFolders[tombstone.FolderUID]; !active {
				candidateFolders[tombstone.FolderUID] = ""
			}
		}
	}
	for _, tombstone := range prepared.manifest.Manifest.RemovedDatasources {
		if live, exists := state.Datasources[tombstone.UID]; exists {
			deleteErr := prepared.client.DeleteDatasource(ctx, tombstone.UID)
			data.Results = append(data.Results, dashboardMutationResult("datasource", tombstone.UID, "prune", deleteErr, dashboardDatasourceReadOnly(live)))
		}
	}
	deleteEmptyManagedFolders(ctx, prepared, candidateFolders, data)
}

func deleteEmptyManagedFolders(ctx context.Context, prepared *dashboardPrepared, candidates map[string]string, data *DashboardActionData) {
	state, err := fetchDashboardLiveState(ctx, prepared.client)
	if err != nil {
		data.Results = append(data.Results, DashboardObjectResult{
			ObjectKind: "folder", UID: "managed-folders", Action: "inspect", Status: "failed",
			Message: "folder contents could not be refreshed", ResultCode: dashboardAPIResultCode(err),
		})
		return
	}
	ordered := make([]string, 0, len(candidates))
	for uid := range candidates {
		ordered = append(ordered, uid)
	}
	sort.Strings(ordered)
	for _, uid := range ordered {
		if _, exists := state.Folders[uid]; !exists {
			continue
		}
		if dashboardFolderHasContents(state.Search, uid) {
			data.Results = append(data.Results, DashboardObjectResult{
				ObjectKind: "folder", UID: uid, Action: "delete", Status: "kept",
				Message: "folder contains unmanaged or remaining resources",
			})
			continue
		}
		deleteErr := prepared.client.DeleteFolder(ctx, uid)
		data.Results = append(data.Results, dashboardMutationResult("folder", uid, "delete", deleteErr, false))
	}
}

func dashboardFolderHasContents(search []grafana.SearchItem, folderUID string) bool {
	for _, item := range search {
		if item.Type == "dash-db" && item.FolderUID == folderUID {
			return true
		}
		if item.Type == "dash-folder" && (item.ParentUID == folderUID || item.FolderUID == folderUID) {
			return true
		}
	}
	return false
}

func dashboardMutationResult(kind, uid, action string, err error, provisioned bool) DashboardObjectResult {
	if err == nil {
		return DashboardObjectResult{ObjectKind: kind, UID: uid, Action: action, Status: "applied"}
	}
	message := "Grafana rejected or failed the object mutation"
	if provisioned {
		message = "provisioned/read-only resource rejected the object mutation"
	}
	return DashboardObjectResult{
		ObjectKind: kind, UID: uid, Action: action, Status: "failed", Message: message,
		ResultCode: dashboardAPIResultCode(err),
	}
}

func dashboardUnchangedResult(kind, uid, action string) DashboardObjectResult {
	return DashboardObjectResult{ObjectKind: kind, UID: uid, Action: action, Status: "unchanged"}
}

func dashboardInternalObjectFailure(kind, uid, action string) DashboardObjectResult {
	return DashboardObjectResult{
		ObjectKind: kind, UID: uid, Action: action, Status: "failed",
		Message: "object could not be normalized safely", ResultCode: output.CodeStyDashboardPartialFailure,
	}
}

func finishDashboardAction(message string, data *DashboardActionData) *output.Result {
	sortDashboardResults(data.Results)
	for _, result := range data.Results {
		if result.Status == "failed" {
			data.Failed++
		}
	}
	if data.Failed > 0 {
		return output.Fail(output.CodeStyDashboardPartialFailure, message+" with per-object failures").WithData(data)
	}
	return output.OK(message, data)
}

func writeAtomicDashboardDump(parent, destination string, manifest *DashboardManifest, files []dashboardDumpFile) error {
	stage, err := os.MkdirTemp(parent, "."+filepath.Base(destination)+".pig-dashboard-dump-")
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(stage)
		}
	}()
	if err := os.Chmod(stage, 0700); err != nil {
		return err
	}
	for _, file := range files {
		filename := filepath.Join(stage, filepath.FromSlash(file.item.Path))
		if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
			return err
		}
		if err := writeSyncedDashboardFile(filename, file.raw); err != nil {
			return err
		}
	}
	manifestRaw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := writeSyncedDashboardFile(filepath.Join(stage, dashboardManifestFilename), append(manifestRaw, '\n')); err != nil {
		return err
	}
	if err := syncDashboardTree(stage); err != nil {
		return err
	}
	if err := os.Rename(stage, destination); err != nil {
		return err
	}
	committed = true
	return syncDashboardDirectory(parent)
}

func writeSyncedDashboardFile(filename string, raw []byte) error {
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err := file.Write(raw); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func syncDashboardTree(root string) error {
	var directories []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			directories = append(directories, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Slice(directories, func(i, j int) bool { return len(directories[i]) > len(directories[j]) })
	for _, directory := range directories {
		if err := syncDashboardDirectory(directory); err != nil {
			return err
		}
	}
	return nil
}

func syncDashboardDirectory(directory string) error {
	handle, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer handle.Close()
	return handle.Sync()
}
