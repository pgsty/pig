package pgext

import (
	"bytes"
	_ "embed"
	"encoding/csv"
	"fmt"
	"os"
	"pig/cli/pgsql"
	"pig/internal/config"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"text/template"

	"github.com/sirupsen/logrus"
)

//go:embed assets/pigsty.csv
var embedExtensionData []byte

var (
	Extensions  []*Extension
	ExtNameMap  map[string]*Extension
	ExtAliasMap map[string]*Extension
	NeedBy      map[string][]string = make(map[string][]string)
	Postgres    *pgsql.PostgresInstallation
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

func (e *Extension) PackageName(pgVer int) string {
	if config.OSType == config.DistroEL && e.RpmPkg != "" {
		return strings.Replace(e.RpmPkg, "$v", strconv.Itoa(pgVer), 1)
	}
	if config.OSType == config.DistroDEB && e.DebPkg != "" {
		return strings.Replace(e.DebPkg, "$v", strconv.Itoa(pgVer), 1)
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

func (e *Extension) NeedBy() []string {
	if len(NeedBy[e.Name]) == 0 {
		return []string{}
	}
	return NeedBy[e.Name]
}

/********************
* Init Extension
********************/
// InitExtensionData initializes extension data from embedded CSV or file
func InitExtensionData(data []byte) error {
	var csvReader *csv.Reader
	if data == nil { // Use embedded data
		data = embedExtensionData
	}
	csvReader = csv.NewReader(bytes.NewReader(data))
	if _, err := csvReader.Read(); err != nil {
		return fmt.Errorf("failed to read CSV header: %v", err)
	}

	// Read records
	records, err := csvReader.ReadAll()
	if err != nil {
		return fmt.Errorf("failed to read CSV records: %v", err)
	}

	// Parse all records first
	extensions := make([]Extension, 0, len(records))
	for _, record := range records {

		ext, err := ParseExtension(record)
		if err != nil {
			logrus.Errorf("failed to parse extension record: %v", err)
			return fmt.Errorf("failed to parse extension record: %v", err)
		}
		extensions = append(extensions, *ext)
	}

	// Sort extensions by ID
	sort.Slice(extensions, func(i, j int) bool {
		return extensions[i].ID < extensions[j].ID
	})

	// Store sorted extensions and update maps with references to array elements
	Extensions = make([]*Extension, len(extensions))
	ExtNameMap = make(map[string]*Extension)
	ExtAliasMap = make(map[string]*Extension)
	for i := range extensions {
		ext := &extensions[i]
		Extensions[i] = ext
		ExtNameMap[ext.Name] = ext
		if ext.Alias != "" && ext.Lead {
			ExtAliasMap[ext.Alias] = ext
		}
		// Update NeedBy map for extensions with dependencies
		if len(ext.Requires) > 0 {
			for _, req := range ext.Requires {
				// Add this extension to the NeedBy list of the required extension
				if _, exists := NeedBy[req]; !exists {
					NeedBy[req] = []string{ext.Name}
				} else {
					NeedBy[req] = append(NeedBy[req], ext.Name)
				}
			}
		}
	}

	return nil
}

/********************
* Parse Extension
********************/

// ParseExtension parses a CSV record into an Extension struct
func ParseExtension(record []string) (*Extension, error) {
	if len(record) != 34 {
		return nil, fmt.Errorf("invalid record length: got %d, want 34", len(record))
	}

	id, err := strconv.Atoi(record[0])
	if err != nil {
		return nil, fmt.Errorf("invalid ID: %v", err)
	}

	// Helper function to parse boolean values
	parseBool := func(s string) bool {
		return strings.ToLower(strings.TrimSpace(s)) == "t"
	}

	ext := &Extension{
		ID:          id,
		Name:        strings.TrimSpace(record[1]),
		Alias:       strings.TrimSpace(record[2]),
		Category:    strings.TrimSpace(record[3]),
		URL:         strings.TrimSpace(record[4]),
		License:     strings.TrimSpace(record[5]),
		Tags:        splitAndTrim(record[6]),
		Version:     strings.TrimSpace(record[7]),
		Repo:        strings.TrimSpace(record[8]),
		Lang:        strings.TrimSpace(record[9]),
		Utility:     parseBool(record[10]),
		Lead:        parseBool(record[11]),
		HasSolib:    parseBool(record[12]),
		NeedDDL:     parseBool(record[13]),
		NeedLoad:    parseBool(record[14]),
		Trusted:     record[15],
		Relocatable: record[16],
		Schemas:     splitAndTrim(record[17]),
		PgVer:       splitAndTrim(record[18]),
		Requires:    splitAndTrim(record[19]),
		RpmVer:      strings.TrimSpace(record[20]),
		RpmRepo:     strings.TrimSpace(record[21]),
		RpmPkg:      strings.TrimSpace(record[22]),
		RpmPg:       splitAndTrim(record[23]),
		RpmDeps:     splitAndTrim(record[24]),
		DebVer:      strings.TrimSpace(record[25]),
		DebRepo:     strings.TrimSpace(record[26]),
		DebPkg:      strings.TrimSpace(record[27]),
		DebDeps:     splitAndTrim(record[28]),
		DebPg:       splitAndTrim(record[29]),
		BadCase:     splitAndTrim(record[30]),
		EnDesc:      strings.TrimSpace(record[31]),
		ZhDesc:      strings.TrimSpace(record[32]),
		Comment:     strings.TrimSpace(record[33]),
	}

	return ext, nil
}

// splitAndTrim splits a comma-separated string and trims whitespace
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
* Tabulate Extension
********************/

func TabulateExtension(filter func(*Extension) bool) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Name\tAlias\tCategory\tVersion\tLicense\tDescription")

	for _, ext := range Extensions {
		if filter == nil || filter(ext) {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				ext.Name,
				ext.Alias,
				ext.Category,
				ext.Version,
				ext.License,
				ext.EnDesc,
			)
		}
	}
	w.Flush()
}

func Tabulate(data []*Extension) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Name\tAlias\tCategory\tVersion\tLicense\tDescription")

	for _, ext := range data {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			ext.Name,
			ext.Alias,
			ext.Category,
			ext.Version,
			ext.License,
			ext.EnDesc,
		)
	}
	w.Flush()
}

/********************
* Tabulate Extension
********************/

// FilterByDistro returns a filter function that filters extensions by distribution
func FilterByDistro(distro string) func(*Extension) bool {
	if distro == "" || distro == "all" {
		return nil
	}
	return func(ext *Extension) bool {
		switch distro {
		case "rpm":
			return ext.RpmRepo != ""
		case "deb":
			return ext.DebRepo != ""
		case "el7", "el8", "el9":
			return ext.RpmRepo != ""
		case "d11", "d12", "u20", "u22", "u24":
			return ext.DebRepo != ""
		default:
			return true
		}
	}
}

// FilterByCategory returns a filter function that filters extensions by category
func FilterByCategory(category string) func(*Extension) bool {
	if category == "" || category == "all" {
		return func(ext *Extension) bool {
			return true
		}
	}
	cate := strings.ToUpper(category)
	return func(ext *Extension) bool {
		return ext.Category == cate
	}
}

// CombineFilters combines multiple filter functions into a single filter
func CombineFilters(filters ...func(*Extension) bool) func(*Extension) bool {
	return func(ext *Extension) bool {
		for _, filter := range filters {
			if filter != nil && !filter(ext) {
				return false
			}
		}
		return true
	}
}

// FilterExtensions returns a filtered list of extensions based on distro and category
func FilterExtensions(distro, category string) func(*Extension) bool {
	return CombineFilters(
		FilterByDistro(distro),
		FilterByCategory(category),
	)
}

/********************
* Extension Info
********************/

const extensionInfoTmpl = `
╭────────────────────────────────────────────────────────────────────────────╮
│ {{ printf "%-74s" .Name   }} │
├────────────────────────────────────────────────────────────────────────────┤
│ {{ printf "%-74s" .EnDesc }} │
├────────────────────────────────────────────────────────────────────────────┤
│ Extension : {{ printf "%-62s" .Name        }} │
│ Alias     : {{ printf "%-62s" .Alias       }} │
│ Category  : {{ printf "%-62s" .Category    }} │
│ Version   : {{ printf "%-62s" .Version     }} │
│ License   : {{ printf "%-62s" .License     }} │
│ Website   : {{ printf "%-62s" .URL         }} │
│ Details   : {{ printf "%-62s" .SummaryURL  }} │
├────────────────────────────────────────────────────────────────────────────┤
│ Extension Properties                                                       │
├────────────────────────────────────────────────────────────────────────────┤
│ PostgreSQL Ver │  Available on: {{ printf "%-42s" (join .PgVer ", ") }} │
│ CREATE  :  {{ if .NeedDDL  }}Yes{{ else }}No {{ end }} │  {{ printf "%-56s" .CreateSQL }} │
│ DYLOAD  :  {{ if .NeedLoad }}Yes{{ else }}No {{ end }} │  {{ printf "%-56s" .SharedLib }} │
│ {{ printf "%-74s" .SuperUser }} │
│ Reloc   :  {{ if eq .Relocatable "t" }}Yes{{ else }}No {{ end }} │  {{ printf "%-56s" .SchemaStr }} │
{{- if .Requires }}
│ Depend  :  Yes │  {{ printf "%-56s" (join .Requires ", ") }} │
{{- else }}
│ Depend  :  No  │                                                           │
{{- end }}
{{- if .NeedBy }}
├────────────────────────────────────────────────────────────────────────────┤
│ Required By                                                                │
├────────────────────────────────────────────────────────────────────────────┤
{{- range .NeedBy }}
│ - {{ printf "%-72s" . }} │
{{- end }}
{{- end }}

{{- if .RpmRepo }}
├────────────────────────────────────────────────────────────────────────────┤
│ RPM Package                                                                │
├────────────────────────────────────────────────────────────────────────────┤
│ Repository     │  {{ printf "%-56s" .RpmRepo }} │
│ Package        │  {{ printf "%-56s" .RpmPkg  }} │
│ Version        │  {{ printf "%-56s" .RpmVer  }} │
│ Availability   │  {{ printf "%-56s" (join .RpmPg ", ") }} │
{{- if .DebDeps }}
│ Dependencies   │  {{ printf "%-56s" (join .RpmDeps ", ") }} │
{{- end }}
{{- end }}

{{- if .DebRepo }}
├────────────────────────────────────────────────────────────────────────────┤
│ DEB Package                                                                │
├────────────────────────────────────────────────────────────────────────────┤
│ Repository     │  {{ printf "%-56s" .DebRepo }} │
│ Package        │  {{ printf "%-56s" .DebPkg  }} │
│ Version        │  {{ printf "%-56s" .DebVer  }} │
│ Availability   │  {{ printf "%-56s" (join .DebPg ", ") }} │
{{- if .DebDeps }}
│ Dependencies   │  {{ printf "%-56s" (join .DebDeps ", ") }} │
{{- end }}
{{- end }}

{{- if .BadCase }}
├────────────────────────────────────────────────────────────────────────────┤
│ Known Issues                                                               │
├────────────────────────────────────────────────────────────────────────────┤
{{- range .BadCase }}
│ {{ printf "%-74s" . }} │
{{- end }}
{{- end }}

{{- if .Comment }}
├────────────────────────────────────────────────────────────────────────────┤
│ Additional Comments                                                        │
├────────────────────────────────────────────────────────────────────────────┤
│ {{ printf "%-74s" .Comment }} │
{{- end }}
╰────────────────────────────────────────────────────────────────────────────╯
`

func join(strs []string, sep string) string {
	return strings.Join(strs, sep)
}

func (e *Extension) PrintInfo() {
	tmpl, err := template.New("extension").Funcs(template.FuncMap{
		"join": join,
	}).Parse(extensionInfoTmpl)
	if err != nil {
		fmt.Printf("Error parsing template: %v\n", err)
		return
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, e); err != nil {
		fmt.Printf("Error executing template: %v\n", err)
		return
	}

	fmt.Println(buf.String())
}
