/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>
*/
package ext

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
