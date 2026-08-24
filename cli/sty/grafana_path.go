package sty

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"pig/internal/utils"
	"regexp"
	"sort"
	"strings"
)

const (
	maxGrafanaDashboardBytes = 16 << 20
	maxGrafanaInlineLength   = 400
	maxGrafanaInlineDepth    = 10
	grafanaDashboardAuthor   = "Ruohang Feng (rh@vonng.com)"
	grafanaDashboardLicense  = "https://pigsty.io/docs/about/license/"
	grafanaDashboardV2API    = "dashboard.grafana.app/v2"
	defaultGrafanaNamespace  = "default"
)

var grafanaUIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,40}$`)
var grafanaNamespacePattern = regexp.MustCompile(`^[a-z0-9](?:[-a-z0-9]{0,61}[a-z0-9])?$`)

type grafanaAction uint8

const (
	grafanaActionLoad grafanaAction = iota + 1
	grafanaActionDump
	grafanaActionClean
)

type grafanaScopeKind string

const (
	grafanaScopeRoot   grafanaScopeKind = "root"
	grafanaScopeFolder grafanaScopeKind = "folder"
	grafanaScopeFile   grafanaScopeKind = "file"
)

type grafanaDashboardFormat uint8

const (
	grafanaDashboardLegacy grafanaDashboardFormat = iota + 1
	grafanaDashboardV2
)

type grafanaAsset struct {
	Path      string
	UID       string
	FolderUID string
	Namespace string
	Format    grafanaDashboardFormat
	Dashboard map[string]interface{}
}

type grafanaScope struct {
	Root      string
	Path      string
	Kind      grafanaScopeKind
	FolderUID string
	UID       string
	Assets    []grafanaAsset
	Folders   []string
}

func resolveGrafanaScope(options GrafanaOptions, action grafanaAction) (*grafanaScope, error) {
	homeRoot := ""
	if strings.TrimSpace(options.Home) != "" {
		home, err := absoluteGrafanaPath(options.Home)
		if err != nil {
			return nil, fmt.Errorf("resolve Pigsty home: %w", err)
		}
		homeRoot = filepath.Join(home, "files", "grafana")
	}

	selectedText := strings.TrimSpace(options.Path)
	selected := ""
	root := ""
	if selectedText == "" {
		if homeRoot == "" {
			return nil, errors.New("grafana path is required because Pigsty home could not be resolved")
		}
		selected, root = homeRoot, homeRoot
	} else {
		absolute, err := absoluteGrafanaPath(selectedText)
		if err != nil {
			return nil, fmt.Errorf("resolve Grafana path: %w", err)
		}
		selected = absolute
		if homeRoot != "" && pathWithin(homeRoot, selected) {
			root = homeRoot
		} else {
			root = nearestGrafanaRoot(selected)
		}
		if root == "" {
			return nil, errors.New("explicit path must be a grafana root or a direct folder/dashboard below one")
		}
	}

	relative, err := filepath.Rel(root, selected)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) || filepath.IsAbs(relative) {
		return nil, errors.New("grafana path escapes its root")
	}
	scope := &grafanaScope{Root: root, Path: selected}
	parts := splitGrafanaRelative(relative)
	switch len(parts) {
	case 0:
		scope.Kind = grafanaScopeRoot
	case 1:
		if strings.EqualFold(filepath.Ext(parts[0]), ".json") {
			scope.Kind = grafanaScopeFile
			scope.UID = strings.TrimSuffix(parts[0], filepath.Ext(parts[0]))
			if err := validateGrafanaUID("dashboard", scope.UID); err != nil {
				return nil, err
			}
		} else {
			scope.Kind = grafanaScopeFolder
			scope.FolderUID = parts[0]
			if err := validateGrafanaUID("folder", scope.FolderUID); err != nil {
				return nil, err
			}
		}
	case 2:
		if !strings.EqualFold(filepath.Ext(parts[1]), ".json") {
			return nil, errors.New("only JSON dashboards may appear below a Grafana folder")
		}
		scope.Kind = grafanaScopeFile
		scope.FolderUID = parts[0]
		scope.UID = strings.TrimSuffix(parts[1], filepath.Ext(parts[1]))
		if err := validateGrafanaUID("folder", scope.FolderUID); err != nil {
			return nil, err
		}
		if err := validateGrafanaUID("dashboard", scope.UID); err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("grafana paths may contain only one folder layer")
	}

	if err := inspectGrafanaScopePath(scope, action); err != nil {
		return nil, err
	}
	return scope, nil
}

func resolveGrafanaDumpScope(options GrafanaOptions) (*grafanaScope, error) {
	directory := strings.TrimSpace(options.DumpDir)
	if directory == "" {
		return resolveGrafanaScope(options, grafanaActionDump)
	}
	if strings.TrimSpace(options.Path) != "" {
		return nil, errors.New("--dir and PATH are mutually exclusive")
	}
	root, err := absoluteGrafanaPath(directory)
	if err != nil {
		return nil, fmt.Errorf("resolve Grafana dump directory: %w", err)
	}
	scope := &grafanaScope{Root: root, Path: root, Kind: grafanaScopeRoot}
	if err := inspectGrafanaScopePath(scope, grafanaActionDump); err != nil {
		return nil, err
	}
	return scope, nil
}

func absoluteGrafanaPath(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "~" || strings.HasPrefix(value, "~"+string(os.PathSeparator)) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if value == "~" {
			value = home
		} else {
			value = filepath.Join(home, strings.TrimPrefix(value, "~"+string(os.PathSeparator)))
		}
	}
	return filepath.Abs(filepath.Clean(value))
}

func nearestGrafanaRoot(selected string) string {
	current := selected
	if strings.EqualFold(filepath.Ext(current), ".json") {
		current = filepath.Dir(current)
	}
	for {
		if filepath.Base(current) == "grafana" {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
		current = parent
	}
}

func pathWithin(root, selected string) bool {
	relative, err := filepath.Rel(root, selected)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator)) && !filepath.IsAbs(relative)
}

func splitGrafanaRelative(relative string) []string {
	if relative == "." || relative == "" {
		return nil
	}
	return strings.Split(relative, string(os.PathSeparator))
}

func validateGrafanaUID(kind, uid string) error {
	if !grafanaUIDPattern.MatchString(uid) {
		return fmt.Errorf("unsafe %s UID %q", kind, uid)
	}
	return nil
}

func inspectGrafanaScopePath(scope *grafanaScope, action grafanaAction) error {
	if scope == nil {
		return errors.New("grafana scope is nil")
	}
	if err := rejectGrafanaSymlink(scope.Root); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := rejectGrafanaSymlinkPath(scope.Root, scope.Path); err != nil {
		return err
	}
	info, err := os.Lstat(scope.Path)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("grafana scope must not be a symlink: %s", scope.Path)
		}
		if scope.Kind == grafanaScopeFile && !info.Mode().IsRegular() {
			return fmt.Errorf("grafana dashboard must be a regular file: %s", scope.Path)
		}
		if scope.Kind != grafanaScopeFile && !info.IsDir() {
			return fmt.Errorf("grafana scope must be a directory: %s", scope.Path)
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect Grafana scope: %w", err)
	}
	if action != grafanaActionDump {
		return fmt.Errorf("grafana scope does not exist: %s", scope.Path)
	}
	parent := filepath.Dir(scope.Path)
	if scope.Kind == grafanaScopeRoot {
		parent = filepath.Dir(scope.Root)
	}
	parentInfo, parentErr := os.Stat(parent)
	if parentErr != nil || !parentInfo.IsDir() {
		return fmt.Errorf("grafana scope parent is unavailable: %s", parent)
	}
	return nil
}

func rejectGrafanaSymlinkPath(root, selected string) error {
	if !pathWithin(root, selected) {
		return errors.New("grafana path escapes its root")
	}
	current := root
	if err := rejectGrafanaSymlink(current); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	relative, _ := filepath.Rel(root, selected)
	for _, part := range splitGrafanaRelative(relative) {
		current = filepath.Join(current, part)
		if err := rejectGrafanaSymlink(current); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
	}
	return nil
}

func rejectGrafanaSymlink(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("grafana paths must not contain symlinks: %s", path)
	}
	return nil
}

func loadGrafanaScopeAssets(scope *grafanaScope) error {
	if scope == nil {
		return errors.New("grafana scope is nil")
	}
	assets, folders, err := readGrafanaScopeAssets(scope)
	if err != nil {
		return err
	}
	if err := rejectDuplicateGrafanaAssets(assets); err != nil {
		return err
	}
	sort.Slice(assets, func(i, j int) bool {
		if assets[i].FolderUID != assets[j].FolderUID {
			return assets[i].FolderUID < assets[j].FolderUID
		}
		return assets[i].UID < assets[j].UID
	})
	sort.Strings(folders)
	scope.Assets = assets
	scope.Folders = folders
	return nil
}

func readGrafanaScopeAssets(scope *grafanaScope) ([]grafanaAsset, []string, error) {
	switch scope.Kind {
	case grafanaScopeFile:
		asset, err := readGrafanaAsset(scope.Path, scope.FolderUID)
		if err != nil {
			return nil, nil, err
		}
		return []grafanaAsset{asset}, nil, nil
	case grafanaScopeFolder:
		assets, err := readGrafanaDirectory(scope.Path, scope.FolderUID)
		if err != nil {
			return nil, nil, err
		}
		return assets, []string{scope.FolderUID}, nil
	case grafanaScopeRoot:
		return readGrafanaRoot(scope.Root)
	default:
		return nil, nil, errors.New("unsupported Grafana scope")
	}
}

func readGrafanaRoot(root string) ([]grafanaAsset, []string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, nil, fmt.Errorf("read Grafana root: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	var assets []grafanaAsset
	var folders []string
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") || name == "__pycache__" {
			continue
		}
		path := filepath.Join(root, name)
		if entry.Type()&os.ModeSymlink != 0 {
			return nil, nil, fmt.Errorf("grafana corpus must not contain symlinks: %s", path)
		}
		if entry.IsDir() {
			if err := validateGrafanaUID("folder", name); err != nil {
				return nil, nil, err
			}
			folderAssets, err := readGrafanaDirectory(path, name)
			if err != nil {
				return nil, nil, err
			}
			folders = append(folders, name)
			assets = append(assets, folderAssets...)
		} else if strings.EqualFold(filepath.Ext(name), ".json") {
			asset, err := readGrafanaAsset(path, "")
			if err != nil {
				return nil, nil, err
			}
			assets = append(assets, asset)
		}
	}
	return assets, folders, nil
}

func rejectDuplicateGrafanaAssets(assets []grafanaAsset) error {
	seen := make(map[string]string, len(assets))
	for _, asset := range assets {
		if previous, exists := seen[asset.UID]; exists {
			return fmt.Errorf("dashboard UID %s is duplicated in %s and %s", asset.UID, previous, asset.Path)
		}
		seen[asset.UID] = asset.Path
	}
	return nil
}

func readGrafanaDirectory(directory, folderUID string) ([]grafanaAsset, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("read Grafana directory %s: %w", directory, err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	assets := make([]grafanaAsset, 0)
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		path := filepath.Join(directory, name)
		if entry.Type()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("grafana corpus must not contain symlinks: %s", path)
		}
		if entry.IsDir() {
			return nil, fmt.Errorf("grafana corpus may contain only one folder layer: %s", path)
		}
		if !strings.EqualFold(filepath.Ext(name), ".json") {
			continue
		}
		asset, readErr := readGrafanaAsset(path, folderUID)
		if readErr != nil {
			return nil, readErr
		}
		assets = append(assets, asset)
	}
	return assets, nil
}

func readGrafanaAsset(path, folderUID string) (grafanaAsset, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return grafanaAsset{}, fmt.Errorf("inspect dashboard %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return grafanaAsset{}, fmt.Errorf("dashboard must be a regular non-symlink file: %s", path)
	}
	if info.Size() > maxGrafanaDashboardBytes {
		return grafanaAsset{}, fmt.Errorf("dashboard exceeds %d bytes: %s", maxGrafanaDashboardBytes, path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return grafanaAsset{}, fmt.Errorf("read dashboard %s: %w", path, err)
	}
	dashboard, err := decodeGrafanaDashboard(raw)
	if err != nil {
		return grafanaAsset{}, fmt.Errorf("parse dashboard %s: %w", path, err)
	}
	format, uid, namespace, err := grafanaDashboardIdentity(dashboard)
	if err != nil {
		return grafanaAsset{}, fmt.Errorf("%s: %w", path, err)
	}
	if err := validateGrafanaUID("dashboard", uid); err != nil {
		return grafanaAsset{}, fmt.Errorf("%s: %w", path, err)
	}
	filenameUID := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if filenameUID != uid {
		return grafanaAsset{}, fmt.Errorf("dashboard filename UID %q does not match JSON UID %q: %s", filenameUID, uid, path)
	}
	return grafanaAsset{
		Path: path, UID: uid, FolderUID: folderUID, Namespace: namespace,
		Format: format, Dashboard: dashboard,
	}, nil
}

func grafanaDashboardIdentity(dashboard map[string]interface{}) (grafanaDashboardFormat, string, string, error) {
	apiVersion, _ := dashboard["apiVersion"].(string)
	kind, _ := dashboard["kind"].(string)
	isDashboardResource := strings.HasPrefix(apiVersion, "dashboard.grafana.app/") || (apiVersion != "" && kind == "Dashboard")
	if !isDashboardResource {
		uid, _ := dashboard["uid"].(string)
		return grafanaDashboardLegacy, uid, "", nil
	}
	if apiVersion != grafanaDashboardV2API || kind != "Dashboard" {
		return 0, "", "", fmt.Errorf("unsupported Grafana dashboard resource %q kind %q", apiVersion, kind)
	}
	metadata, ok := dashboard["metadata"].(map[string]interface{})
	if !ok {
		return 0, "", "", errors.New("grafana dashboard resource metadata must be an object")
	}
	uid, _ := metadata["name"].(string)
	namespace, _ := metadata["namespace"].(string)
	if namespace == "" {
		namespace = defaultGrafanaNamespace
	}
	if !grafanaNamespacePattern.MatchString(namespace) {
		return 0, "", "", fmt.Errorf("unsafe Grafana namespace %q", namespace)
	}
	if _, ok := dashboard["spec"].(map[string]interface{}); !ok {
		return 0, "", "", errors.New("grafana dashboard resource spec must be an object")
	}
	return grafanaDashboardV2, uid, namespace, nil
}

func decodeGrafanaDashboard(raw []byte) (map[string]interface{}, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	value := make(map[string]interface{})
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var extra interface{}
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("dashboard contains multiple JSON values")
		}
		return nil, err
	}
	return value, nil
}

func cloneGrafanaDashboard(value map[string]interface{}) (map[string]interface{}, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return decodeGrafanaDashboard(raw)
}

func dashboardForGrafanaInit(value map[string]interface{}) map[string]interface{} {
	clone := maps.Clone(value)
	clone["id"] = nil
	return clone
}

func dashboardResourceForGrafanaInit(value map[string]interface{}, namespace, name, folderUID string) (map[string]interface{}, error) {
	clone, err := cloneGrafanaDashboard(value)
	if err != nil {
		return nil, err
	}
	spec, ok := clone["spec"].(map[string]interface{})
	if !ok {
		return nil, errors.New("grafana dashboard resource spec must be an object")
	}
	annotations := make(map[string]interface{})
	if metadata, ok := clone["metadata"].(map[string]interface{}); ok {
		if source, ok := metadata["annotations"].(map[string]interface{}); ok {
			if message, exists := source["grafana.app/message"]; exists {
				annotations["grafana.app/message"] = message
			}
		}
	}
	if folderUID != "" {
		annotations["grafana.app/folder"] = folderUID
	}
	return map[string]interface{}{
		"apiVersion": grafanaDashboardV2API,
		"kind":       "Dashboard",
		"metadata": map[string]interface{}{
			"name": name, "namespace": namespace, "annotations": annotations,
		},
		"spec": spec,
	}, nil
}

func dashboardForGrafanaDump(value map[string]interface{}) (map[string]interface{}, error) {
	clone, err := cloneGrafanaDashboard(value)
	if err != nil {
		return nil, err
	}
	clone["id"] = nil
	clone["version"] = json.Number("1")
	clone["author"] = grafanaDashboardAuthor
	clone["license"] = grafanaDashboardLicense
	if templating, ok := clone["templating"].(map[string]interface{}); ok {
		if list, ok := templating["list"].([]interface{}); ok {
			for _, item := range list {
				if variable, ok := item.(map[string]interface{}); ok {
					variable["current"] = map[string]interface{}{}
				}
			}
		}
	}
	return clone, nil
}

func marshalGrafanaDashboard(value map[string]interface{}) ([]byte, error) {
	var buffer bytes.Buffer
	if err := writeCompactGrafanaJSON(&buffer, value, 0); err != nil {
		return nil, err
	}
	buffer.WriteByte('\n')
	return buffer.Bytes(), nil
}

func writeCompactGrafanaJSON(buffer *bytes.Buffer, value interface{}, depth int) error {
	inline, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if compactGrafanaJSONCanInline(value, inline, depth) {
		buffer.Write(inline)
		return nil
	}
	switch typed := value.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		buffer.WriteString("{\n")
		for index, key := range keys {
			writeGrafanaJSONIndent(buffer, depth+1)
			encodedKey, _ := json.Marshal(key)
			buffer.Write(encodedKey)
			buffer.WriteByte(':')
			if err := writeCompactGrafanaJSON(buffer, typed[key], depth+1); err != nil {
				return err
			}
			if index+1 < len(keys) {
				buffer.WriteByte(',')
			}
			buffer.WriteByte('\n')
		}
		writeGrafanaJSONIndent(buffer, depth)
		buffer.WriteByte('}')
	case []interface{}:
		buffer.WriteString("[\n")
		for index, item := range typed {
			writeGrafanaJSONIndent(buffer, depth+1)
			if err := writeCompactGrafanaJSON(buffer, item, depth+1); err != nil {
				return err
			}
			if index+1 < len(typed) {
				buffer.WriteByte(',')
			}
			buffer.WriteByte('\n')
		}
		writeGrafanaJSONIndent(buffer, depth)
		buffer.WriteByte(']')
	default:
		buffer.Write(inline)
	}
	return nil
}

func compactGrafanaJSONCanInline(value interface{}, inline []byte, depth int) bool {
	switch value.(type) {
	case map[string]interface{}, []interface{}:
		return len(inline)+depth*2 <= maxGrafanaInlineLength && grafanaJSONDepth(value) <= maxGrafanaInlineDepth
	default:
		return true
	}
}

func grafanaJSONDepth(value interface{}) int {
	maximum := 0
	switch typed := value.(type) {
	case map[string]interface{}:
		for _, child := range typed {
			if depth := grafanaJSONDepth(child); depth > maximum {
				maximum = depth
			}
		}
		return maximum + 1
	case []interface{}:
		for _, child := range typed {
			if depth := grafanaJSONDepth(child); depth > maximum {
				maximum = depth
			}
		}
		return maximum + 1
	default:
		return 0
	}
}

func writeGrafanaJSONIndent(buffer *bytes.Buffer, depth int) {
	for index := 0; index < depth; index++ {
		buffer.WriteString("  ")
	}
}

func writeAtomicGrafanaFile(filename string, raw []byte) error {
	directory := filepath.Dir(filename)
	if err := os.MkdirAll(directory, 0755); err != nil {
		return err
	}
	if err := rejectGrafanaSymlink(directory); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".pig-grafana-*.tmp")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryName)
		}
	}()
	mode := os.FileMode(0644)
	if info, statErr := os.Lstat(filename); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("grafana dump target must be a regular non-symlink file: %s", filename)
		}
		mode = info.Mode().Perm()
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}
	if err := temporary.Chmod(mode); err != nil {
		return err
	}
	if _, err := temporary.Write(raw); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryName, filename); err != nil {
		return err
	}
	committed = true
	return utils.SyncDirectory(directory)
}
