package sty

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	dashboardManifestKind       = "pig.sty.grafana.manifest/v1"
	dashboardNormalizationKind  = "grafana/v1"
	dashboardManifestFilename   = "manifest.json"
	maxDashboardManifestBytes   = 2 << 20
	maxDashboardDefinitionBytes = 8 << 20
)

var (
	//go:embed assets/grafana-manifest.json
	embeddedDashboardManifest []byte

	grafanaUIDPattern       = regexp.MustCompile(`^[A-Za-z0-9_-]{1,40}$`)
	manifestEnvPattern      = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
	manifestInventoryKey    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	manifestSecureFieldName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)
	manifestSensitiveKey    = regexp.MustCompile(`(?i)(password|passwd|secret|token|private.?key|credential)`)
)

// DashboardManifest is the authoritative active UID set plus removed-UID
// lineage. Repository file paths are relative to DashboardDir.
type DashboardManifest struct {
	Kind               string                        `json:"kind"`
	Revision           int                           `json:"revision"`
	Normalization      string                        `json:"normalization"`
	Dashboards         []DashboardManifestDashboard  `json:"dashboards"`
	Datasources        []DashboardManifestDatasource `json:"datasources"`
	RemovedDashboards  []DashboardManifestTombstone  `json:"removed_dashboards"`
	RemovedDatasources []DashboardManifestTombstone  `json:"removed_datasources"`
}

type DashboardManifestDashboard struct {
	UID         string `json:"uid"`
	Path        string `json:"path"`
	FolderUID   string `json:"folder_uid"`
	FolderTitle string `json:"folder_title"`
	Title       string `json:"title"`
}

type DashboardManifestDatasource struct {
	UID          string                         `json:"uid"`
	Name         string                         `json:"name"`
	Type         string                         `json:"type"`
	Spec         map[string]interface{}         `json:"spec"`
	SecureFields []DashboardManifestSecureField `json:"secure_fields,omitempty"`
}

type DashboardManifestSecureField struct {
	Name             string `json:"name"`
	Env              string `json:"env,omitempty"`
	Inventory        string `json:"inventory,omitempty"`
	RequiredOnCreate bool   `json:"required_on_create,omitempty"`
}

type DashboardManifestTombstone struct {
	UID       string `json:"uid"`
	FolderUID string `json:"folder_uid,omitempty"`
}

type dashboardManifestSource struct {
	Manifest     *DashboardManifest
	Source       string
	DashboardDir string
	Raw          []byte
}

type dashboardRepositoryAsset struct {
	Manifest DashboardManifestDashboard
	Raw      []byte
	Value    map[string]interface{}
}

func loadDashboardManifest(home, explicit string) (*dashboardManifestSource, error) {
	home = strings.TrimSpace(home)
	explicit = strings.TrimSpace(explicit)
	var raw []byte
	var source, dashboardDir string
	if explicit != "" {
		absolute, err := filepath.Abs(explicit)
		if err != nil {
			return nil, fmt.Errorf("resolve dashboard manifest: %w", err)
		}
		raw, err = readBoundedRegularFile(absolute, maxDashboardManifestBytes)
		if err != nil {
			return nil, fmt.Errorf("read dashboard manifest: %w", err)
		}
		source, dashboardDir = filepath.Clean(absolute), filepath.Dir(absolute)
	} else {
		if home == "" {
			return nil, errors.New("Pigsty home is not configured")
		}
		dashboardDir = filepath.Join(home, "files", "grafana")
		local := filepath.Join(dashboardDir, dashboardManifestFilename)
		if info, err := os.Lstat(local); err == nil {
			if !info.Mode().IsRegular() {
				return nil, fmt.Errorf("dashboard manifest is not a regular file: %s", local)
			}
			raw, err = readBoundedRegularFile(local, maxDashboardManifestBytes)
			if err != nil {
				return nil, fmt.Errorf("read dashboard manifest: %w", err)
			}
			source = local
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("inspect dashboard manifest: %w", err)
		} else {
			raw = append([]byte(nil), embeddedDashboardManifest...)
			source = "embedded:" + dashboardManifestKind
		}
	}

	manifest := &DashboardManifest{}
	if err := decodeStrictJSON(raw, maxDashboardManifestBytes, manifest); err != nil {
		return nil, fmt.Errorf("parse dashboard manifest: %w", err)
	}
	if err := validateDashboardManifest(manifest); err != nil {
		return nil, err
	}
	return &dashboardManifestSource{Manifest: manifest, Source: source, DashboardDir: dashboardDir, Raw: append([]byte(nil), raw...)}, nil
}

func validateDashboardManifest(manifest *DashboardManifest) error {
	if manifest == nil {
		return errors.New("dashboard manifest is nil")
	}
	if manifest.Kind != dashboardManifestKind {
		return fmt.Errorf("unsupported dashboard manifest kind %q", manifest.Kind)
	}
	if manifest.Revision < 1 {
		return errors.New("dashboard manifest revision must be positive")
	}
	if manifest.Normalization != dashboardNormalizationKind {
		return fmt.Errorf("unsupported dashboard normalization %q", manifest.Normalization)
	}
	if len(manifest.Dashboards) == 0 {
		return errors.New("dashboard manifest has no active dashboards")
	}

	dashboardUIDs := make(map[string]struct{}, len(manifest.Dashboards))
	dashboardPaths := make(map[string]struct{}, len(manifest.Dashboards))
	folders := make(map[string]string)
	for _, item := range manifest.Dashboards {
		if err := validateGrafanaUID("dashboard", item.UID); err != nil {
			return err
		}
		if _, exists := dashboardUIDs[item.UID]; exists {
			return fmt.Errorf("dashboard UID is duplicated: %s", item.UID)
		}
		dashboardUIDs[item.UID] = struct{}{}
		if strings.TrimSpace(item.Title) == "" {
			return fmt.Errorf("dashboard %s has no title", item.UID)
		}
		if !validManifestRelativePath(item.Path) {
			return fmt.Errorf("dashboard %s has an unsafe repository path", item.UID)
		}
		if _, exists := dashboardPaths[item.Path]; exists {
			return fmt.Errorf("dashboard repository path is duplicated: %s", item.Path)
		}
		dashboardPaths[item.Path] = struct{}{}
		if item.FolderUID == "" {
			if item.FolderTitle != "" {
				return fmt.Errorf("root dashboard %s declares a folder title", item.UID)
			}
			continue
		}
		if err := validateGrafanaUID("folder", item.FolderUID); err != nil {
			return err
		}
		if strings.TrimSpace(item.FolderTitle) == "" {
			return fmt.Errorf("folder %s has no title", item.FolderUID)
		}
		if previous, exists := folders[item.FolderUID]; exists && previous != item.FolderTitle {
			return fmt.Errorf("folder %s has conflicting titles", item.FolderUID)
		}
		folders[item.FolderUID] = item.FolderTitle
	}

	datasourceUIDs := make(map[string]struct{}, len(manifest.Datasources))
	for _, item := range manifest.Datasources {
		if err := validateGrafanaUID("datasource", item.UID); err != nil {
			return err
		}
		if _, exists := datasourceUIDs[item.UID]; exists {
			return fmt.Errorf("datasource UID is duplicated: %s", item.UID)
		}
		datasourceUIDs[item.UID] = struct{}{}
		if strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.Type) == "" {
			return fmt.Errorf("datasource %s requires name and type", item.UID)
		}
		if item.Spec == nil {
			return fmt.Errorf("datasource %s has no non-secret spec", item.UID)
		}
		for _, reserved := range []string{"uid", "name", "type", "secureJsonData", "secureJsonFields"} {
			if _, exists := item.Spec[reserved]; exists {
				return fmt.Errorf("datasource %s spec contains reserved field %s", item.UID, reserved)
			}
		}
		if sensitiveManifestValue(item.Spec) {
			return fmt.Errorf("datasource %s spec contains a secret-bearing field", item.UID)
		}
		secureNames := make(map[string]struct{}, len(item.SecureFields))
		for _, field := range item.SecureFields {
			if !manifestSecureFieldName.MatchString(field.Name) {
				return fmt.Errorf("datasource %s has invalid secure field name", item.UID)
			}
			if _, exists := secureNames[field.Name]; exists {
				return fmt.Errorf("datasource %s secure field is duplicated: %s", item.UID, field.Name)
			}
			secureNames[field.Name] = struct{}{}
			if field.Env == "" && field.Inventory == "" {
				return fmt.Errorf("datasource %s secure field %s has no protected source", item.UID, field.Name)
			}
			if field.Env != "" && !manifestEnvPattern.MatchString(field.Env) {
				return fmt.Errorf("datasource %s secure field %s has invalid environment source", item.UID, field.Name)
			}
			if field.Inventory != "" && !manifestInventoryKey.MatchString(field.Inventory) {
				return fmt.Errorf("datasource %s secure field %s has invalid Inventory source", item.UID, field.Name)
			}
		}
	}

	if err := validateManifestTombstones("dashboard", manifest.RemovedDashboards, dashboardUIDs); err != nil {
		return err
	}
	if err := validateManifestTombstones("datasource", manifest.RemovedDatasources, datasourceUIDs); err != nil {
		return err
	}
	return nil
}

func validateManifestTombstones(kind string, tombstones []DashboardManifestTombstone, active map[string]struct{}) error {
	seen := make(map[string]struct{}, len(tombstones))
	for _, tombstone := range tombstones {
		if err := validateGrafanaUID(kind+" tombstone", tombstone.UID); err != nil {
			return err
		}
		if _, exists := active[tombstone.UID]; exists {
			return fmt.Errorf("%s UID is reused after removal: %s", kind, tombstone.UID)
		}
		if _, exists := seen[tombstone.UID]; exists {
			return fmt.Errorf("%s tombstone is duplicated: %s", kind, tombstone.UID)
		}
		seen[tombstone.UID] = struct{}{}
		if tombstone.FolderUID != "" {
			if kind != "dashboard" {
				return fmt.Errorf("%s tombstone %s declares a folder", kind, tombstone.UID)
			}
			if err := validateGrafanaUID("folder", tombstone.FolderUID); err != nil {
				return err
			}
		}
	}
	return nil
}

func loadDashboardRepositoryAssets(source *dashboardManifestSource) ([]dashboardRepositoryAsset, error) {
	if source == nil || source.Manifest == nil {
		return nil, errors.New("dashboard manifest source is nil")
	}
	assets := make([]dashboardRepositoryAsset, 0, len(source.Manifest.Dashboards))
	for _, item := range source.Manifest.Dashboards {
		filename := filepath.Join(source.DashboardDir, filepath.FromSlash(item.Path))
		raw, err := readBoundedRegularFile(filename, maxDashboardDefinitionBytes)
		if err != nil {
			return nil, fmt.Errorf("read dashboard %s: %w", item.UID, err)
		}
		value := make(map[string]interface{})
		if err := decodeStrictJSON(raw, maxDashboardDefinitionBytes, &value); err != nil {
			return nil, fmt.Errorf("parse dashboard %s: %w", item.UID, err)
		}
		uid, _ := value["uid"].(string)
		title, _ := value["title"].(string)
		if uid != item.UID {
			return nil, fmt.Errorf("dashboard %s file declares UID %q", item.UID, uid)
		}
		if title != item.Title {
			return nil, fmt.Errorf("dashboard %s file title does not match the manifest", item.UID)
		}
		assets = append(assets, dashboardRepositoryAsset{Manifest: item, Raw: raw, Value: value})
	}
	return assets, nil
}

func dashboardManifestFolders(manifest *DashboardManifest) map[string]string {
	folders := make(map[string]string)
	if manifest == nil {
		return folders
	}
	for _, item := range manifest.Dashboards {
		if item.FolderUID != "" {
			folders[item.FolderUID] = item.FolderTitle
		}
	}
	return folders
}

func sortedDashboardManifestFolders(manifest *DashboardManifest) []DashboardManifestDashboard {
	folders := dashboardManifestFolders(manifest)
	result := make([]DashboardManifestDashboard, 0, len(folders))
	for uid, title := range folders {
		result = append(result, DashboardManifestDashboard{FolderUID: uid, FolderTitle: title})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].FolderUID < result[j].FolderUID })
	return result
}

func validManifestRelativePath(value string) bool {
	if value == "" || strings.Contains(value, `\`) || path.IsAbs(value) || path.Ext(value) != ".json" {
		return false
	}
	clean := path.Clean(value)
	return clean == value && clean != "." && clean != ".." && !strings.HasPrefix(clean, "../")
}

func validateGrafanaUID(kind, uid string) error {
	if !grafanaUIDPattern.MatchString(uid) {
		return fmt.Errorf("%s UID is invalid: %q", kind, uid)
	}
	return nil
}

func sensitiveManifestValue(value interface{}) bool {
	switch typed := value.(type) {
	case map[string]interface{}:
		for key, child := range typed {
			if manifestSensitiveKey.MatchString(key) || sensitiveManifestValue(child) {
				return true
			}
		}
	case []interface{}:
		for _, child := range typed {
			if sensitiveManifestValue(child) {
				return true
			}
		}
	}
	return false
}

func readBoundedRegularFile(filename string, limit int64) ([]byte, error) {
	info, err := os.Lstat(filename)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("source is not a regular file")
	}
	if info.Size() > limit {
		return nil, fmt.Errorf("source exceeds %d bytes", limit)
	}
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(io.LimitReader(file, limit+1))
}

func decodeStrictJSON(raw []byte, limit int64, destination interface{}) error {
	if int64(len(raw)) > limit {
		return fmt.Errorf("JSON exceeds %d bytes", limit)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("JSON contains multiple values")
		}
		return err
	}
	return nil
}
