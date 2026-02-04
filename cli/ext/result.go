/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>
*/
package ext

import (
	"fmt"
	"pig/internal/config"
	"pig/internal/output"
)

/********************
 * Data Transfer Objects (DTOs) for ANCS Output
 * These structures are used for structured YAML/JSON output
 ********************/

// ExtensionListData is the DTO for ext list command
type ExtensionListData struct {
	Query      string              `json:"query,omitempty" yaml:"query,omitempty"`
	PgVersion  int                 `json:"pg_version,omitempty" yaml:"pg_version,omitempty"`
	OSCode     string              `json:"os_code,omitempty" yaml:"os_code,omitempty"`
	Arch       string              `json:"arch,omitempty" yaml:"arch,omitempty"`
	Count      int                 `json:"count" yaml:"count"`
	Extensions []*ExtensionSummary `json:"extensions" yaml:"extensions"`
}

// ExtensionSummary is a compact representation of an extension for list output
type ExtensionSummary struct {
	Name        string   `json:"name" yaml:"name"`
	Pkg         string   `json:"pkg" yaml:"pkg"`
	Version     string   `json:"version" yaml:"version"`
	Category    string   `json:"category" yaml:"category"`
	License     string   `json:"license" yaml:"license"`
	Repo        string   `json:"repo" yaml:"repo"`
	Status      string   `json:"status" yaml:"status"`
	PackageName string   `json:"package_name" yaml:"package_name"`
	Description string   `json:"description" yaml:"description"`
	PgVer       []string `json:"pg_ver,omitempty" yaml:"pg_ver,omitempty"`
}

// ExtensionInfoData is the DTO for ext info command
type ExtensionInfoData struct {
	Name        string               `json:"name" yaml:"name"`
	Pkg         string               `json:"pkg" yaml:"pkg"`
	LeadExt     string               `json:"lead_ext,omitempty" yaml:"lead_ext,omitempty"`
	Category    string               `json:"category" yaml:"category"`
	License     string               `json:"license" yaml:"license"`
	Language    string               `json:"language" yaml:"language"`
	Version     string               `json:"version" yaml:"version"`
	URL         string               `json:"url,omitempty" yaml:"url,omitempty"`
	Source      string               `json:"source,omitempty" yaml:"source,omitempty"`
	Description string               `json:"description" yaml:"description"`
	ZhDesc      string               `json:"zh_desc,omitempty" yaml:"zh_desc,omitempty"`
	Properties  *ExtensionProperties `json:"properties" yaml:"properties"`
	Requires    []string             `json:"requires,omitempty" yaml:"requires,omitempty"`
	RequiredBy  []string             `json:"required_by,omitempty" yaml:"required_by,omitempty"`
	SeeAlso     []string             `json:"see_also,omitempty" yaml:"see_also,omitempty"`
	PgVer       []string             `json:"pg_ver" yaml:"pg_ver"`
	Schemas     []string             `json:"schemas,omitempty" yaml:"schemas,omitempty"`
	RpmPackage  *PackageInfo         `json:"rpm_package,omitempty" yaml:"rpm_package,omitempty"`
	DebPackage  *PackageInfo         `json:"deb_package,omitempty" yaml:"deb_package,omitempty"`
	Operations  *ExtensionOperations `json:"operations" yaml:"operations"`
	Comment     string               `json:"comment,omitempty" yaml:"comment,omitempty"`
}

// ExtensionProperties contains extension property flags
type ExtensionProperties struct {
	HasBin      bool   `json:"has_bin" yaml:"has_bin"`
	HasLib      bool   `json:"has_lib" yaml:"has_lib"`
	NeedLoad    bool   `json:"need_load" yaml:"need_load"`
	NeedDDL     bool   `json:"need_ddl" yaml:"need_ddl"`
	Relocatable string `json:"relocatable" yaml:"relocatable"`
	Trusted     string `json:"trusted" yaml:"trusted"`
}

// PackageInfo contains package-specific information
type PackageInfo struct {
	Package    string   `json:"package" yaml:"package"`
	Repository string   `json:"repository" yaml:"repository"`
	Version    string   `json:"version" yaml:"version"`
	PgVer      []string `json:"pg_ver" yaml:"pg_ver"`
	Deps       []string `json:"deps,omitempty" yaml:"deps,omitempty"`
}

// ExtensionOperations contains operational commands
type ExtensionOperations struct {
	Install string `json:"install" yaml:"install"`
	Config  string `json:"config,omitempty" yaml:"config,omitempty"`
	Create  string `json:"create,omitempty" yaml:"create,omitempty"`
	Build   string `json:"build" yaml:"build"`
}

// ExtensionStatusData is the DTO for ext status command
type ExtensionStatusData struct {
	PgInfo     *PostgresInfo         `json:"pg_info,omitempty" yaml:"pg_info,omitempty"`
	Summary    *ExtensionSummaryInfo `json:"summary" yaml:"summary"`
	Extensions []*ExtensionSummary   `json:"extensions" yaml:"extensions"`
	NotFound   []string              `json:"not_found,omitempty" yaml:"not_found,omitempty"`
}

// PostgresInfo contains PostgreSQL installation information
type PostgresInfo struct {
	Version      string `json:"version" yaml:"version"`
	MajorVersion int    `json:"major_version" yaml:"major_version"`
	BinDir       string `json:"bin_dir" yaml:"bin_dir"`
	DataDir      string `json:"data_dir,omitempty" yaml:"data_dir,omitempty"` // Reserved for future use
	ExtensionDir string `json:"extension_dir" yaml:"extension_dir"`
}

// ExtensionSummaryInfo contains extension count statistics
type ExtensionSummaryInfo struct {
	TotalInstalled int            `json:"total_installed" yaml:"total_installed"`
	ByRepo         map[string]int `json:"by_repo" yaml:"by_repo"`
}

// ExtensionAvailData is the DTO for ext avail command
type ExtensionAvailData struct {
	// Global availability mode (no arguments)
	OSCode       string                 `json:"os_code,omitempty" yaml:"os_code,omitempty"`
	Arch         string                 `json:"arch,omitempty" yaml:"arch,omitempty"`
	PackageCount int                    `json:"package_count,omitempty" yaml:"package_count,omitempty"`
	Packages     []*PackageAvailability `json:"packages,omitempty" yaml:"packages,omitempty"`

	// Single extension availability mode (with arguments)
	Extension string         `json:"extension,omitempty" yaml:"extension,omitempty"`
	Matrix    []*MatrixEntry `json:"matrix,omitempty" yaml:"matrix,omitempty"`
	Summary   string         `json:"summary,omitempty" yaml:"summary,omitempty"`
	LatestVer string         `json:"latest_version,omitempty" yaml:"latest_version,omitempty"`
}

// PackageAvailability represents package availability by PG version
type PackageAvailability struct {
	Pkg      string            `json:"pkg" yaml:"pkg"`
	Versions map[string]string `json:"versions" yaml:"versions"`
}

// MatrixEntry represents a single entry in the availability matrix
type MatrixEntry struct {
	OS      string `json:"os" yaml:"os"`
	Arch    string `json:"arch" yaml:"arch"`
	PG      int    `json:"pg" yaml:"pg"`
	State   string `json:"state" yaml:"state"`
	Version string `json:"version" yaml:"version"`
	Org     string `json:"org" yaml:"org"`
}

// ExtensionAddData is the DTO for ext add command
type ExtensionAddData struct {
	PgVersion   int                 `json:"pg_version" yaml:"pg_version"`
	OSCode      string              `json:"os_code" yaml:"os_code"`
	Arch        string              `json:"arch" yaml:"arch"`
	Requested   []string            `json:"requested" yaml:"requested"`
	Packages    []string            `json:"packages" yaml:"packages"`
	Installed   []*InstalledExtItem `json:"installed" yaml:"installed"`
	Failed      []*FailedExtItem    `json:"failed,omitempty" yaml:"failed,omitempty"`
	DurationMs  int64               `json:"duration_ms" yaml:"duration_ms"`
	AutoConfirm bool                `json:"auto_confirm" yaml:"auto_confirm"`
}

// InstalledExtItem represents a successfully installed extension
type InstalledExtItem struct {
	Name    string `json:"name" yaml:"name"`
	Package string `json:"package" yaml:"package"`
	Version string `json:"version,omitempty" yaml:"version,omitempty"`
}

// FailedExtItem represents a failed extension operation
type FailedExtItem struct {
	Name    string `json:"name" yaml:"name"`
	Package string `json:"package,omitempty" yaml:"package,omitempty"`
	Error   string `json:"error" yaml:"error"`
	Code    int    `json:"code" yaml:"code"`
}

// ExtensionRmData is the DTO for ext rm command
type ExtensionRmData struct {
	PgVersion   int              `json:"pg_version" yaml:"pg_version"`
	OSCode      string           `json:"os_code" yaml:"os_code"`
	Arch        string           `json:"arch" yaml:"arch"`
	Requested   []string         `json:"requested" yaml:"requested"`
	Packages    []string         `json:"packages" yaml:"packages"`
	Removed     []string         `json:"removed" yaml:"removed"`
	Failed      []*FailedExtItem `json:"failed,omitempty" yaml:"failed,omitempty"`
	DurationMs  int64            `json:"duration_ms" yaml:"duration_ms"`
	AutoConfirm bool             `json:"auto_confirm" yaml:"auto_confirm"`
}

// ExtensionUpdateData is the DTO for ext update command
type ExtensionUpdateData struct {
	PgVersion   int              `json:"pg_version" yaml:"pg_version"`
	OSCode      string           `json:"os_code" yaml:"os_code"`
	Arch        string           `json:"arch" yaml:"arch"`
	Requested   []string         `json:"requested" yaml:"requested"`
	Packages    []string         `json:"packages" yaml:"packages"`
	Updated     []string         `json:"updated" yaml:"updated"`
	Failed      []*FailedExtItem `json:"failed,omitempty" yaml:"failed,omitempty"`
	DurationMs  int64            `json:"duration_ms" yaml:"duration_ms"`
	AutoConfirm bool             `json:"auto_confirm" yaml:"auto_confirm"`
}

// ImportResultData is the DTO for ext import command
type ImportResultData struct {
	PgVersion  int      `json:"pg_version" yaml:"pg_version"`
	OSCode     string   `json:"os_code" yaml:"os_code"`
	Arch       string   `json:"arch" yaml:"arch"`
	RepoDir    string   `json:"repo_dir" yaml:"repo_dir"`
	Requested  []string `json:"requested" yaml:"requested"`
	Packages   []string `json:"packages" yaml:"packages"`
	PkgCount   int      `json:"pkg_count" yaml:"pkg_count"`
	Downloaded []string `json:"downloaded,omitempty" yaml:"downloaded,omitempty"`
	Failed     []string `json:"failed,omitempty" yaml:"failed,omitempty"`
	DurationMs int64    `json:"duration_ms" yaml:"duration_ms"`
}

// ScanResultData is the DTO for ext scan command
type ScanResultData struct {
	PgInfo        *PostgresInfo   `json:"pg_info" yaml:"pg_info"`
	ExtCount      int             `json:"extension_count" yaml:"extension_count"`
	Extensions    []*ScanExtEntry `json:"extensions" yaml:"extensions"`
	UnmatchedLibs []string        `json:"unmatched_libs,omitempty" yaml:"unmatched_libs,omitempty"`
	EncodingLibs  []string        `json:"encoding_libs,omitempty" yaml:"encoding_libs,omitempty"`
	BuiltInLibs   []string        `json:"builtin_libs,omitempty" yaml:"builtin_libs,omitempty"`
}

// ScanExtEntry represents a scanned extension entry
type ScanExtEntry struct {
	Name        string            `json:"name" yaml:"name"`
	ControlName string            `json:"control_name,omitempty" yaml:"control_name,omitempty"`
	Version     string            `json:"version" yaml:"version"`
	Description string            `json:"description,omitempty" yaml:"description,omitempty"`
	Libraries   []string          `json:"libraries,omitempty" yaml:"libraries,omitempty"`
	InCatalog   bool              `json:"in_catalog" yaml:"in_catalog"`
	ControlMeta map[string]string `json:"control_meta,omitempty" yaml:"control_meta,omitempty"`
}

// LinkResultData is the DTO for ext link command
type LinkResultData struct {
	Action       string `json:"action" yaml:"action"`                                   // "link" or "unlink"
	PgHome       string `json:"pg_home,omitempty" yaml:"pg_home,omitempty"`             // PostgreSQL home directory
	SymlinkPath  string `json:"symlink_path" yaml:"symlink_path"`                       // /usr/pgsql
	ProfilePath  string `json:"profile_path" yaml:"profile_path"`                       // /etc/profile.d/pgsql.sh
	ActivatedCmd string `json:"activated_cmd,omitempty" yaml:"activated_cmd,omitempty"` // ". /etc/profile.d/pgsql.sh"
}

// ReloadResultData is the DTO for ext reload command
type ReloadResultData struct {
	SourceURL      string `json:"source_url" yaml:"source_url"`
	ExtensionCount int    `json:"extension_count" yaml:"extension_count"`
	CatalogPath    string `json:"catalog_path" yaml:"catalog_path"`
	DownloadedAt   string `json:"downloaded_at" yaml:"downloaded_at"`
	DurationMs     int64  `json:"duration_ms" yaml:"duration_ms"`
}

/********************
 * Conversion Methods
 ********************/

// ToSummary converts an Extension to ExtensionSummary
func (e *Extension) ToSummary(pgVer int) *ExtensionSummary {
	if e == nil {
		return nil
	}
	status := "not_avail"
	if Postgres != nil && Postgres.ExtensionMap != nil && Postgres.ExtensionMap[e.Name] != nil {
		status = "installed"
	} else if e.Available(pgVer) {
		status = "available"
	}

	return &ExtensionSummary{
		Name:        e.Name,
		Pkg:         e.Pkg,
		Version:     e.Version,
		Category:    e.Category,
		License:     e.License,
		Repo:        e.RepoName(),
		Status:      status,
		PackageName: e.PackageName(pgVer),
		Description: e.EnDesc,
		PgVer:       e.PgVer,
	}
}

// ToInfoData converts an Extension to ExtensionInfoData
func (e *Extension) ToInfoData() *ExtensionInfoData {
	if e == nil {
		return nil
	}

	// Get dependents safely (DependsOn requires Catalog to be initialized)
	var requiredBy []string
	if Catalog != nil && Catalog.ExtNameMap != nil {
		requiredBy = e.DependsOn()
	}

	info := &ExtensionInfoData{
		Name:        e.Name,
		Pkg:         e.Pkg,
		LeadExt:     e.LeadExt,
		Category:    e.Category,
		License:     e.License,
		Language:    e.Lang,
		Version:     e.Version,
		URL:         e.URL,
		Source:      e.Source,
		Description: e.EnDesc,
		ZhDesc:      e.ZhDesc,
		Properties: &ExtensionProperties{
			HasBin:      e.HasBin,
			HasLib:      e.HasLib,
			NeedLoad:    e.NeedLoad,
			NeedDDL:     e.NeedDDL,
			Relocatable: e.Relocatable,
			Trusted:     e.Trusted,
		},
		Requires:   e.Requires,
		RequiredBy: requiredBy,
		SeeAlso:    e.SeeAlso,
		PgVer:      e.PgVer,
		Schemas:    e.Schemas,
		Comment:    e.Comment,
	}

	// RPM package info
	if e.RpmRepo != "" {
		info.RpmPackage = &PackageInfo{
			Package:    e.RpmPkg,
			Repository: e.RpmRepo,
			Version:    e.RpmVer,
			PgVer:      e.RpmPg,
			Deps:       e.RpmDeps,
		}
	}

	// DEB package info
	if e.DebRepo != "" {
		info.DebPackage = &PackageInfo{
			Package:    e.DebPkg,
			Repository: e.DebRepo,
			Version:    e.DebVer,
			PgVer:      e.DebPg,
			Deps:       e.DebDeps,
		}
	}

	// Operations
	info.Operations = &ExtensionOperations{
		Install: fmt.Sprintf("pig ext add %s", e.Pkg),
		Build:   e.GetBuildCommand(),
	}

	if e.NeedLoad {
		libName := e.GetExtraString("lib")
		if libName == "" {
			libName = e.Name
		}
		info.Operations.Config = fmt.Sprintf("shared_preload_libraries = '%s'", libName)
	}

	if e.NeedDDL {
		if len(e.Requires) > 0 {
			info.Operations.Create = fmt.Sprintf("CREATE EXTENSION %s CASCADE;", e.Name)
		} else {
			info.Operations.Create = fmt.Sprintf("CREATE EXTENSION %s;", e.Name)
		}
	}

	return info
}

/********************
 * Result Constructors
 ********************/

// ListExtensions returns a structured Result for the ext list command
func ListExtensions(query string, pgVer int) *output.Result {
	if Catalog == nil {
		return output.Fail(output.CodeExtensionCatalogError, "extension catalog not initialized")
	}

	results := Catalog.Extensions
	if query != "" {
		results = SearchExtensions(query, Catalog.Extensions)
	}

	// Build extension list with optional ShowPkg filter
	extensions := make([]*ExtensionSummary, 0, len(results))
	for _, ext := range results {
		if ext == nil {
			continue
		}
		// Apply ShowPkg filter if enabled (only show lead extensions)
		if ShowPkg && !ext.Lead {
			continue
		}
		extensions = append(extensions, ext.ToSummary(pgVer))
	}

	data := &ExtensionListData{
		Query:      query,
		PgVersion:  pgVer,
		OSCode:     config.OSCode,
		Arch:       config.OSArch,
		Count:      len(extensions),
		Extensions: extensions,
	}

	message := fmt.Sprintf("Found %d extensions", data.Count)
	if query != "" {
		message = fmt.Sprintf("Found %d extensions matching '%s'", data.Count, query)
	}

	return output.OK(message, data)
}

// GetExtensionInfo returns a structured Result for the ext info command
func GetExtensionInfo(names []string) *output.Result {
	if len(names) == 0 {
		return output.Fail(output.CodeExtensionInvalidArgs, "no extension name provided")
	}

	if Catalog == nil {
		return output.Fail(output.CodeExtensionCatalogError, "extension catalog not initialized")
	}

	var infos []*ExtensionInfoData
	var notFound []string

	for _, name := range names {
		e, ok := Catalog.ExtNameMap[name]
		if !ok {
			e, ok = Catalog.ExtPkgMap[name]
		}
		if !ok {
			notFound = append(notFound, name)
			continue
		}
		infos = append(infos, e.ToInfoData())
	}

	if len(infos) == 0 {
		return output.Fail(output.CodeExtensionNotFound, fmt.Sprintf("extensions not found: %v", notFound))
	}

	var data interface{}
	var message string
	if len(infos) == 1 {
		data = infos[0]
		message = fmt.Sprintf("Extension: %s", infos[0].Name)
	} else {
		data = infos
		message = fmt.Sprintf("Found %d extensions", len(infos))
	}

	result := output.OK(message, data)
	if len(notFound) > 0 {
		result.Detail = fmt.Sprintf("not found: %v", notFound)
	}
	return result
}

// GetExtStatus returns a structured Result for the ext status command
func GetExtStatus(showContrib bool) *output.Result {
	if Catalog == nil {
		return output.Fail(output.CodeExtensionCatalogError, "extension catalog not initialized")
	}

	if Postgres == nil {
		return output.Fail(output.CodeExtensionNoPG, "no PostgreSQL specified and no active PostgreSQL found")
	}

	// Collect installed extensions
	var exts []*ExtensionSummary
	var notFound []string
	repoCount := map[string]int{"CONTRIB": 0, "PGDG": 0, "PIGSTY": 0}

	for _, installedExt := range Postgres.Extensions {
		if installedExt.Extension == nil {
			continue
		}
		extInfo := Catalog.ExtNameMap[installedExt.Name]
		if extInfo == nil {
			notFound = append(notFound, installedExt.Name)
			continue
		}
		if extInfo.RepoName() != "" {
			if _, ok := repoCount[extInfo.RepoName()]; !ok {
				repoCount[extInfo.RepoName()] = 0
			}
			repoCount[extInfo.RepoName()]++
		}
		if !showContrib && extInfo.Repo == "CONTRIB" {
			continue
		}
		exts = append(exts, extInfo.ToSummary(Postgres.MajorVersion))
	}

	totalInstalled := len(exts)
	data := &ExtensionStatusData{
		PgInfo: &PostgresInfo{
			Version:      Postgres.Version,
			MajorVersion: Postgres.MajorVersion,
			BinDir:       Postgres.BinPath,
			ExtensionDir: Postgres.ExtPath,
		},
		Summary: &ExtensionSummaryInfo{
			TotalInstalled: totalInstalled,
			ByRepo:         repoCount,
		},
		Extensions: exts,
		NotFound:   notFound,
	}

	message := fmt.Sprintf("PostgreSQL %d: %d extensions installed", Postgres.MajorVersion, totalInstalled)
	return output.OK(message, data)
}

// GetExtensionAvailability returns a structured Result for the ext avail command
func GetExtensionAvailability(names []string) *output.Result {
	if Catalog == nil || len(Catalog.Extensions) == 0 {
		return output.Fail(output.CodeExtensionCatalogError, "no extension catalog available")
	}

	osCode := config.OSCode
	arch := config.OSArch

	// No arguments: global availability
	if len(names) == 0 {
		return getGlobalAvailability(osCode, arch)
	}

	// With arguments: per-extension availability
	return getExtensionAvailabilities(names, osCode, arch)
}

func getGlobalAvailability(osCode, arch string) *output.Result {
	// Collect lead packages
	var packages []*PackageAvailability

	for _, ext := range Catalog.Extensions {
		if ext.Contrib || !ext.Lead {
			continue
		}

		matrix := ext.GetPkgMatrix()
		versions := make(map[string]string)

		// Use the centralized PostgreSQL version list
		for _, pg := range PostgresActiveMajorVersions {
			if matrix != nil {
				entry := matrix.Get(osCode, arch, pg)
				if entry != nil && entry.State == PkgAvail {
					versions[fmt.Sprintf("%d", pg)] = entry.Version
				}
			}
		}

		if len(versions) > 0 {
			packages = append(packages, &PackageAvailability{
				Pkg:      ext.Pkg,
				Versions: versions,
			})
		}
	}

	data := &ExtensionAvailData{
		OSCode:       osCode,
		Arch:         arch,
		PackageCount: len(packages),
		Packages:     packages,
	}

	message := fmt.Sprintf("Found %d packages available on %s.%s", len(packages), osCode, arch)
	return output.OK(message, data)
}

// buildExtensionAvailData builds availability data for a single extension
// This is a helper function to avoid code duplication
func buildExtensionAvailData(e *Extension) *ExtensionAvailData {
	if e == nil {
		return nil
	}

	leadExt := getLeadExtension(e)
	matrix := leadExt.GetPkgMatrix()

	entries := make([]*MatrixEntry, 0, len(matrix))
	for _, entry := range matrix {
		if entry != nil {
			entries = append(entries, &MatrixEntry{
				OS:      entry.OS,
				Arch:    entry.Arch,
				PG:      entry.PG,
				State:   string(entry.State),
				Version: entry.Version,
				Org:     string(entry.Org),
			})
		}
	}

	return &ExtensionAvailData{
		Extension: e.Name,
		Matrix:    entries,
		Summary:   matrix.Summary(),
		LatestVer: matrix.LatestVersion(),
	}
}

func getExtensionAvailabilities(names []string, osCode, arch string) *output.Result {
	if len(names) == 1 {
		return getSingleExtensionAvailability(names[0], osCode, arch)
	}

	// Multiple extensions: return array
	var results []*ExtensionAvailData
	var notFound []string

	for _, name := range names {
		e, ok := Catalog.ExtNameMap[name]
		if !ok {
			e, ok = Catalog.ExtPkgMap[name]
		}
		if !ok {
			notFound = append(notFound, name)
			continue
		}

		results = append(results, buildExtensionAvailData(e))
	}

	if len(results) == 0 {
		return output.Fail(output.CodeExtensionNotFound, fmt.Sprintf("extensions not found: %v", notFound))
	}

	message := fmt.Sprintf("Availability for %d extensions", len(results))
	result := output.OK(message, results)
	if len(notFound) > 0 {
		result.Detail = fmt.Sprintf("not found: %v", notFound)
	}
	return result
}

func getSingleExtensionAvailability(name, osCode, arch string) *output.Result {
	e, ok := Catalog.ExtNameMap[name]
	if !ok {
		e, ok = Catalog.ExtPkgMap[name]
	}
	if !ok {
		return output.Fail(output.CodeExtensionNotFound, fmt.Sprintf("extension '%s' not found", name))
	}

	data := buildExtensionAvailData(e)
	message := fmt.Sprintf("Availability for %s: %s", e.Name, data.Summary)
	return output.OK(message, data)
}

// ToScanEntry converts an ExtensionInstall to ScanExtEntry for structured output
func (ei *ExtensionInstall) ToScanEntry() *ScanExtEntry {
	if ei == nil {
		return nil
	}

	entry := &ScanExtEntry{
		Name:        ei.ExtName(),
		ControlName: ei.ControlName,
		Version:     ei.VersionString(),
		Description: ei.Description(),
		InCatalog:   ei.Extension != nil,
		ControlMeta: ei.ControlMeta,
	}

	// Add libraries if present - directly convert from map keys
	if len(ei.Libraries) > 0 {
		libs := make([]string, 0, len(ei.Libraries))
		for lib := range ei.Libraries {
			libs = append(libs, lib)
		}
		entry.Libraries = libs
	}

	return entry
}

// ScanExtensionsResult returns a structured Result for the ext scan command
func ScanExtensionsResult() *output.Result {
	if Catalog == nil {
		return output.Fail(output.CodeExtensionCatalogError, "extension catalog not initialized")
	}

	if Postgres == nil {
		return output.Fail(output.CodeExtensionNoPG, "no PostgreSQL specified and no active PostgreSQL found")
	}

	// Scan extensions
	if err := Postgres.ScanExtensions(); err != nil {
		return output.Fail(output.CodeExtensionCatalogError, fmt.Sprintf("failed to scan extensions: %v", err))
	}

	// Build extension list
	var extensions []*ScanExtEntry
	for _, installedExt := range Postgres.Extensions {
		if installedExt == nil {
			continue
		}
		extensions = append(extensions, installedExt.ToScanEntry())
	}

	// Collect unmatched, encoding, and built-in libs
	var unmatchedLibs []string
	var encodingLibs []string
	var builtInLibs []string

	for libName, matched := range Postgres.SharedLibs {
		if isEncodingLib(libName) {
			encodingLibs = append(encodingLibs, libName)
			continue
		}
		if isBuiltInLib(libName) {
			builtInLibs = append(builtInLibs, libName)
			continue
		}
		if !matched {
			unmatchedLibs = append(unmatchedLibs, libName)
		}
	}

	data := &ScanResultData{
		PgInfo: &PostgresInfo{
			Version:      Postgres.Version,
			MajorVersion: Postgres.MajorVersion,
			BinDir:       Postgres.BinPath,
			ExtensionDir: Postgres.ExtPath,
		},
		ExtCount:      len(extensions),
		Extensions:    extensions,
		UnmatchedLibs: unmatchedLibs,
		EncodingLibs:  encodingLibs,
		BuiltInLibs:   builtInLibs,
	}

	message := fmt.Sprintf("PostgreSQL %d: scanned %d extensions", Postgres.MajorVersion, len(extensions))
	return output.OK(message, data)
}
