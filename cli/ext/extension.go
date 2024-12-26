package ext

import (
	"bytes"
	_ "embed"
	"fmt"
	"pig/internal/config"
	"sort"
	"strconv"
	"strings"
)

/********************
* Extension Type
********************/

// Extension represents a PostgreSQL extension record
type Extension struct {
	ID          int      `csv:"id"`          // Primary key
	Name        string   `csv:"name"`        // Extension name
	Alias       string   `csv:"alias"`       // Alternative name
	Category    string   `csv:"category"`    // Extension category
	URL         string   `csv:"url"`         // Project URL
	License     string   `csv:"license"`     // License type
	Tags        []string `csv:"tags"`        // Extension tags
	Version     string   `csv:"version"`     // Extension version
	Repo        string   `csv:"repo"`        // Repository name
	Lang        string   `csv:"lang"`        // Programming language
	Utility     bool     `csv:"utility"`     // Is utility extension
	Lead        bool     `csv:"lead"`        // Is lead extension
	HasSolib    bool     `csv:"has_solib"`   // Has shared library
	NeedDDL     bool     `csv:"need_ddl"`    // Needs DDL changes
	NeedLoad    bool     `csv:"need_load"`   // Needs loading
	Trusted     string   `csv:"trusted"`     // Is trusted extension
	Relocatable string   `csv:"relocatable"` // Is relocatable
	Schemas     []string `csv:"schemas"`     // Target schemas
	PgVer       []string `csv:"pg_ver"`      // Supported PG versions
	Requires    []string `csv:"requires"`    // Required extensions
	RpmVer      string   `csv:"rpm_ver"`     // RPM version
	RpmRepo     string   `csv:"rpm_repo"`    // RPM repository
	RpmPkg      string   `csv:"rpm_pkg"`     // RPM package name
	RpmPg       []string `csv:"rpm_pg"`      // RPM PG versions
	RpmDeps     []string `csv:"rpm_deps"`    // RPM dependencies
	DebVer      string   `csv:"deb_ver"`     // DEB version
	DebRepo     string   `csv:"deb_repo"`    // DEB repository
	DebPkg      string   `csv:"deb_pkg"`     // DEB package name
	DebDeps     []string `csv:"deb_deps"`    // DEB dependencies
	DebPg       []string `csv:"deb_pg"`      // DEB PG versions
	BadCase     []string `csv:"bad_case"`    // Distro BadCase
	EnDesc      string   `csv:"en_desc"`     // English description
	ZhDesc      string   `csv:"zh_desc"`     // Chinese description
	Comment     string   `csv:"comment"`     // Additional comments
}

// SummaryURL returns the URL to the ext.pigsty.io catalog summary page
func (e *Extension) SummaryURL() string {
	return fmt.Sprintf("https://ext.pigsty.io/#/%s", e.Name)
}

func (e *Extension) FullTextSearchSummary() string {
	var buf bytes.Buffer
	buf.WriteString(e.Name)
	if strings.Contains(e.Name, "-") || strings.Contains(e.Name, "_") {
		buf.WriteString(" " + strings.Replace(strings.ReplaceAll(e.Name, "-", " "), "_", " ", -1))
	}
	if e.Alias != e.Name {
		buf.WriteString(" " + strings.ToLower(e.Alias))
	}
	buf.WriteString(" " + strings.ToLower(e.Category))
	if e.RpmPkg != "" {
		buf.WriteString(" " + strings.ToLower(e.RpmPkg))
	}
	if e.DebPkg != "" {
		buf.WriteString(" " + strings.ToLower(e.DebPkg))
	}
	if e.EnDesc != "" {
		buf.WriteString(" " + strings.ToLower(e.EnDesc))
	}
	if e.ZhDesc != "" {
		buf.WriteString(" " + strings.ToLower(e.ZhDesc))
	}
	return buf.String()
}

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

func (e *Extension) Availability(distroCode string) string {
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
		if e.Repo != "" {
			return strings.Replace(e.Repo, "$v", verStr, 1)
		}
	}
	return ""
}

func (e *Extension) GuessRpmNamePattern(pgVer int) string {
	return strings.Replace(e.Name, "-", "_", -1) + "_$v"
}

func (e *Extension) GuessDebNamePattern(pgVer int) string {
	return fmt.Sprintf("postgresql-$v-%s", strings.Replace(e.Name, "_", "-", -1))
}

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

func (e *Extension) CreateSQL() string {
	if len(e.Requires) > 0 {
		return fmt.Sprintf("CREATE EXTENSION %s CASCADE;", e.Name)
	} else {
		return fmt.Sprintf("CREATE EXTENSION %s;", e.Name)
	}
}

func (e *Extension) SharedLib() string {
	if e.NeedLoad {
		return fmt.Sprintf("SET shared_preload_libraries = '%s'", e.Name)
	}
	if e.HasSolib {
		return "no need to load shared libraries"
	}
	return "no shared library"
}

func (e *Extension) SuperUser() string {
	if e.Trusted == "t" {
		return "TRUST   :  Yes │  does not require superuser to install"
	}
	if e.Trusted == "f" {
		return "TRUST   :  No  │  require database superuser to install"
	}
	return "TRUST   :  N/A │ unknown, may require dbsu to install"
}

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
	case "utility":
		if e.Utility {
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
	if e.Utility {
		b = "b"
	}
	if e.NeedDDL {
		d = "d"
	}
	if e.NeedLoad {
		l = "l"
	}
	if e.HasSolib {
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

// NeedBy returns the list of extensions that depend on this extension
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
