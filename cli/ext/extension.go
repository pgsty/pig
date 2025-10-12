package ext

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"pig/internal/config"
	"sort"
	"strconv"
	"strings"
)

// Extension represents a PostgreSQL extension record
type Extension struct {
	ID          int                    `csv:"id"`          // Primary key
	Name        string                 `csv:"name"`        // Extension name
	Pkg         string                 `csv:"pkg"`         // Normalized extension package name
	LeadExt     string                 `csv:"lead_ext"`    // The leading extension in this package
	Category    string                 `csv:"category"`    // Extension category
	State       string                 `csv:"state"`       // Extension State (available, deprecated, removed, not-ready)
	URL         string                 `csv:"url"`         // Project URL
	License     string                 `csv:"license"`     // License type
	Tags        []string               `csv:"tags"`        // Extension tags
	Version     string                 `csv:"version"`     // Extension version
	Repo        string                 `csv:"repo"`        // Repository name
	Lang        string                 `csv:"lang"`        // Programming language
	Contrib     bool                   `csv:"contrib"`     // Is contrib extension
	Lead        bool                   `csv:"lead"`        // Is lead extension
	HasBin      bool                   `csv:"has_bin"`     // Has binary utilities
	HasLib      bool                   `csv:"has_lib"`     // Has shared library
	NeedDDL     bool                   `csv:"need_ddl"`    // Needs DDL changes
	NeedLoad    bool                   `csv:"need_load"`   // Needs loading
	Trusted     string                 `csv:"trusted"`     // Is trusted extension
	Relocatable string                 `csv:"relocatable"` // Is relocatable
	Schemas     []string               `csv:"schemas"`     // Target schemas
	PgVer       []string               `csv:"pg_ver"`      // Supported PG versions
	Requires    []string               `csv:"requires"`    // Required extensions
	RequireBy   []string               `csv:"require_by"`  // Extensions that depend on this
	SeeAlso     []string               `csv:"see_also"`    // Related extensions
	RpmVer      string                 `csv:"rpm_ver"`     // RPM version
	RpmRepo     string                 `csv:"rpm_repo"`    // RPM repository
	RpmPkg      string                 `csv:"rpm_pkg"`     // RPM package name
	RpmPg       []string               `csv:"rpm_pg"`      // RPM PG versions
	RpmDeps     []string               `csv:"rpm_deps"`    // RPM dependencies
	DebVer      string                 `csv:"deb_ver"`     // DEB version
	DebRepo     string                 `csv:"deb_repo"`    // DEB repository
	DebPkg      string                 `csv:"deb_pkg"`     // DEB package name
	DebDeps     []string               `csv:"deb_deps"`    // DEB dependencies
	DebPg       []string               `csv:"deb_pg"`      // DEB PG versions
	Source      string                 `csv:"source"`      // Source code tarball name
	Extra       map[string]interface{} `csv:"extra"`       // Extra information (JSONB)
	EnDesc      string                 `csv:"en_desc"`     // English description
	ZhDesc      string                 `csv:"zh_desc"`     // Chinese description
	Comment     string                 `csv:"comment"`     // Additional comments
	Mtime       string                 `csv:"mtime"`       // Last modify time
}

// SummaryURL returns the URL to the pigsty.io catalog summary page
func (e *Extension) SummaryURL() string {
	return fmt.Sprintf("https://ext.pgsty.com/e/%s", e.Name)
}

// CompactVersion returns the compact version string like 17-13
func CompactVersion(pgVers []string) string {
	// Remove version "12" from the list
	filteredVers := []int{}
	for _, ver := range pgVers {
		if ver != "12" {
			verInt, err := strconv.Atoi(ver)
			if err == nil {
				filteredVers = append(filteredVers, verInt)
			}
		}
	}
	// If no versions left after filtering, return empty string
	if len(filteredVers) == 0 {
		return ""
	}

	// Sort the versions
	sort.Ints(filteredVers)

	// If only one version, return it
	if len(filteredVers) == 1 {
		return strconv.Itoa(filteredVers[0])
	}

	// Return the range in "min-max" format
	return fmt.Sprintf("%d-%d", filteredVers[0], filteredVers[len(filteredVers)-1])
}

// Availability returns the availability hint string according to the extension availability
func (e *Extension) Availability(distroCode string) string {
	// TODO: check via distroCode

	switch config.OSType {
	case config.DistroEL:
		if e.RpmRepo == "" {
			return "n/a"
		} else {
			return CompactVersion(e.RpmPg)
		}
	case config.DistroDEB:
		if e.DebRepo == "" {
			return "n/a"
		} else {
			return CompactVersion(e.DebPg)
		}
	case config.DistroMAC:
		if e.Repo == "" {
			return "n/a"
		} else {
			return CompactVersion(e.PgVer)
		}
	}

	return CompactVersion(e.PgVer)
}

// PackageName returns the package name of the extension according to the OS type
func (e *Extension) PackageName(pgVer int) string {
	verStr := strconv.Itoa(pgVer)
	if pgVer == 0 {
		verStr = "$v"
	}
	switch config.OSType {
	case config.DistroEL:
		if e.RpmPkg != "" {
			return strings.Replace(e.RpmPkg, "$v", verStr, 1)
		}
	case config.DistroDEB:
		if e.DebPkg != "" {
			return strings.Replace(e.DebPkg, "$v", verStr, 1)
		}
	case config.DistroMAC:
		return ""
	}
	return ""
}

// GuessRpmNamePattern returns the guessed RPM package name pattern
func (e *Extension) GuessRpmNamePattern(pgVer int) string {
	return strings.Replace(e.Name, "-", "_", -1) + "_$v"
}

// GuessDebNamePattern returns the guessed DEB package name pattern
func (e *Extension) GuessDebNamePattern(pgVer int) string {
	return fmt.Sprintf("postgresql-$v-%s", strings.Replace(e.Name, "_", "-", -1))
}

// RepoName returns the repository name of the extension according to the OS type
func (e *Extension) RepoName() string {
	switch config.OSType {
	case config.DistroEL:
		if e.RpmRepo != "" {
			return e.RpmRepo
		}
	case config.DistroDEB:
		if e.DebRepo != "" {
			return e.DebRepo
		}
	case config.DistroMAC:
		if e.Repo != "" {
			return e.Repo
		}
	}
	return ""
}

// CreateSQL returns the SQL command to create the extension
func (e *Extension) CreateSQL() string {
	if len(e.Requires) > 0 {
		return fmt.Sprintf("CREATE EXTENSION %s CASCADE;", e.Name)
	} else {
		return fmt.Sprintf("CREATE EXTENSION %s;", e.Name)
	}
}

// SharedLib returns the shared library hint string according to the extension availability
func (e *Extension) SharedLib() string {
	if e.NeedLoad {
		return fmt.Sprintf("SET shared_preload_libraries = '%s'", e.Name)
	}
	if e.HasLib {
		return "no need to load shared libraries"
	}
	return "no shared library"
}

// SuperUser returns the superuser hint string according to the extension trust level
func (e *Extension) SuperUser() string {
	if e.Trusted == "t" {
		return "TRUST   :  Yes │  does not require superuser to install"
	}
	if e.Trusted == "f" {
		return "TRUST   :  No  │  require database superuser to install"
	}
	return "TRUST   :  N/A │ unknown, may require dbsu to install"
}

// SchemaStr returns the schema hint string according to the extension schema list
func (e *Extension) SchemaStr() string {
	if len(e.Schemas) == 0 {
		return "Schemas: []"
	}
	return fmt.Sprintf("Schemas: [ %s ]", strings.Join(e.Schemas, ", "))
}

// GetBool returns a string of "Yes" / "No" / "N/A" according to the boolean value
func (e *Extension) GetBool(name string) string {
	// return yes / no n/a  according to the boolean value
	switch name {
	case "ddl":
		if e.NeedDDL {
			return "Yes"
		}
		return "No"
	case "load":
		if e.NeedLoad {
			return "Yes"
		}
		return "No"
	case "has_bin":
		if e.HasBin {
			return "Yes"
		}
		return "No"
	case "has_lib":
		if e.HasLib {
			return "Yes"
		}
		return "No"
	case "contrib":
		if e.Contrib {
			return "Yes"
		}
		return "No"
	case "lead":
		if e.Lead {
			return "Yes"
		}
		return "No"
	case "relocatable":
		if e.Relocatable == "t" {
			return "Yes"
		} else if e.Relocatable == "f" {
			return "No"
		}
		return "N/A"
	case "trusted":
		if e.Trusted == "t" {
			return "Yes"
		} else if e.Trusted == "f" {
			return "No"
		}
		return "N/A"
	}
	return "N/A"
}

// GetFlag returns a string of flags for the extension
func (e *Extension) GetFlag() string {
	b, d, s, l, t, r := "-", "-", "-", "-", "-", "-"
	if e.HasBin {
		b = "b"
	}
	if e.NeedDDL {
		d = "d"
	}
	if e.NeedLoad {
		l = "l"
	}
	if e.HasLib {
		s = "s"
	}
	if e.Trusted == "t" {
		t = "t"
	} else {
		if e.Trusted == "f" {
			t = "-"
		} else {
			t = "x"
		}
	}
	if e.Relocatable == "t" {
		r = "r"
	} else {
		if e.Relocatable == "f" {
			r = "-"
		} else {
			r = "x"
		}
	}

	return b + d + s + l + t + r
}

// GetStatus returns the status of the extension
// If the global Postgres is not nil, it will check the installation status
func (e *Extension) GetStatus(ver int) string {
	if Postgres != nil {
		if Postgres.ExtensionMap[e.Name] != nil {
			return "added"
		} else {
			if e.Available(ver) {
				return "avail"
			} else {
				return "n/a"
			}
		}
	} else {
		if e.Available(ver) {
			return "avail"
		} else {
			return "n/a"
		}
	}
}

// DependsOn returns the list of extensions that depend on this extension
// This function depends on the global Catalog.DependsMap
func (e *Extension) DependsOn() []string {
	if Catalog == nil || Catalog.Dependency == nil {
		return []string{}
	}
	if v, ok := Catalog.Dependency[e.Name]; ok {
		return v
	}
	return nil
}

/********************
* Parse Extension
********************/

// ParseExtension parses a CSV record into an Extension struct
func ParseExtension(record []string) (*Extension, error) {
	if len(record) != 41 {
		return nil, fmt.Errorf("invalid record length: got %d, want 41", len(record))
	}

	id, err := strconv.Atoi(record[0])
	if err != nil {
		return nil, fmt.Errorf("invalid ID: %v", err)
	}

	// Helper function to parse boolean values
	parseBool := func(s string) bool {
		return strings.ToLower(strings.TrimSpace(s)) == "t"
	}

	// Parse Extra JSON field
	var extra map[string]interface{}
	extraStr := strings.TrimSpace(record[36])
	if extraStr != "" && extraStr != "{}" {
		if err := json.Unmarshal([]byte(extraStr), &extra); err != nil {
			extra = make(map[string]interface{})
		}
	} else {
		extra = make(map[string]interface{})
	}

	ext := &Extension{
		ID:          id,
		Name:        strings.TrimSpace(record[1]),
		Pkg:         strings.TrimSpace(record[2]),
		LeadExt:     strings.TrimSpace(record[3]),
		Category:    strings.TrimSpace(record[4]),
		State:       strings.TrimSpace(record[5]),
		URL:         strings.TrimSpace(record[6]),
		License:     strings.TrimSpace(record[7]),
		Tags:        splitAndTrim(record[8]),
		Version:     strings.TrimSpace(record[9]),
		Repo:        strings.TrimSpace(record[10]),
		Lang:        strings.TrimSpace(record[11]),
		Contrib:     parseBool(record[12]),
		Lead:        parseBool(record[13]),
		HasBin:      parseBool(record[14]),
		HasLib:      parseBool(record[15]),
		NeedDDL:     parseBool(record[16]),
		NeedLoad:    parseBool(record[17]),
		Trusted:     record[18],
		Relocatable: record[19],
		Schemas:     splitAndTrim(record[20]),
		PgVer:       splitAndTrim(record[21]),
		Requires:    splitAndTrim(record[22]),
		RequireBy:   splitAndTrim(record[23]),
		SeeAlso:     splitAndTrim(record[24]),
		RpmVer:      strings.TrimSpace(record[25]),
		RpmRepo:     strings.TrimSpace(record[26]),
		RpmPkg:      strings.TrimSpace(record[27]),
		RpmPg:       splitAndTrim(record[28]),
		RpmDeps:     splitAndTrim(record[29]),
		DebVer:      strings.TrimSpace(record[30]),
		DebRepo:     strings.TrimSpace(record[31]),
		DebPkg:      strings.TrimSpace(record[32]),
		DebDeps:     splitAndTrim(record[33]),
		DebPg:       splitAndTrim(record[34]),
		Source:      strings.TrimSpace(record[35]),
		Extra:       extra,
		EnDesc:      strings.TrimSpace(record[37]),
		ZhDesc:      strings.TrimSpace(record[38]),
		Comment:     strings.TrimSpace(record[39]),
		Mtime:       strings.TrimSpace(record[40]),
	}

	return ext, nil
}

// splitAndTrim splits a comma-separated string and trims whitespace
// used as auxiliary function for parsing extension data
func splitAndTrim(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "{")
	s = strings.TrimSuffix(s, "}")
	if s == "" {
		return []string{}
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

/********************
* Distro Availability
********************/

// DistroBadCase is a map of bad cases for extensions
var DistroBadCase = map[string]map[string][]int{
	"el8.amd64":  {"pg_duckdb": {}, "pg_mooncake": {}},
	"el8.arm64":  {"pg_dbms_job": {}, "jdbc_fdw": {}, "pllua": {15, 14, 13}, "pg_duckdb": {}, "pg_mooncake": {}, "pg_dbms_metadata": {15}},
	"el9.amd64":  {},
	"el9.arm64":  {"pg_dbms_job": {}, "jdbc_fdw": {}, "pllua": {15, 14, 13}},
	"el10.amd64": {},
	"el10.arm64": {"pg_dbms_job": {}, "jdbc_fdw": {}, "pllua": {15, 14, 13}},

	"u22.amd64": {},
	"u22.arm64": {},
	"u24.amd64": {"pg_partman": {13}},
	"u24.arm64": {"pg_partman": {13}, "timeseries": {13}},

	"d11.amd64": {"babelfishpg_common": {}, "babelfishpg_tsql": {}, "babelfishpg_tds": {}, "babelfishpg_money": {}},
	"d11.arm64": {"babelfishpg_common": {}, "babelfishpg_tsql": {}, "babelfishpg_tds": {}, "babelfishpg_money": {}},
	"d12.amd64": {"babelfishpg_common": {}, "babelfishpg_tsql": {}, "babelfishpg_tds": {}, "babelfishpg_money": {}},
	"d12.arm64": {"babelfishpg_common": {}, "babelfishpg_tsql": {}, "babelfishpg_tds": {}, "babelfishpg_money": {}},
	"d13.amd64": {"babelfishpg_common": {}, "babelfishpg_tsql": {}, "babelfishpg_tds": {}, "babelfishpg_money": {}}, // TBD
	"d13.arm64": {"babelfishpg_common": {}, "babelfishpg_tsql": {}, "babelfishpg_tds": {}, "babelfishpg_money": {}}, // TBD
}

// RpmRenameMap is a map of RPM package rename rules
var RpmRenameMap = map[string]map[int]string{
	"pgaudit": {15: "pgaudit17_15*", 14: "pgaudit17_14*", 13: "pgaudit17_13*"},
}

// Available check if the extension is available for the given pg version
func (e *Extension) Available(pgVer int) bool {
	verStr := strconv.Itoa(pgVer)

	// test1: check rpm/deb version compatibility
	switch config.OSType {
	case config.DistroEL:
		if e.RpmPg != nil {
			found := false
			for _, ver := range e.RpmPg {
				if ver == verStr {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	case config.DistroDEB:
		if e.DebPg != nil {
			found := false
			for _, ver := range e.DebPg {
				if ver == verStr {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	case config.DistroMAC:
		return true
	}

	// test2 will check bad base according to DistroCode and OSArch
	distroCodeArch := fmt.Sprintf("%s.%s", config.OSCode, config.OSArch)
	badCases := DistroBadCase[distroCodeArch]
	if badCases == nil {
		return true
	}
	v, ok := badCases[e.Name]
	if !ok {
		return true
	} else {
		if len(v) == 0 { // match all version
			return false
		}
		for _, ver := range v {
			if ver == pgVer {
				return false
			}
		}
		return true
	}
}
