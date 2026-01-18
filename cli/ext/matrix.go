package ext

import (
	"fmt"
	"pig/internal/config"
	"sort"
	"strconv"
	"strings"
)

/********************
* Package Availability Matrix
* Parse and display extension package availability across OS/Arch/PG combinations
********************/

// PkgState represents the availability state of a package
type PkgState string

const (
	PkgAvail PkgState = "A" // AVAIL - available
	PkgMiss  PkgState = "M" // MISS  - missing
	PkgHide  PkgState = "H" // HIDE  - hidden
	PkgBreak PkgState = "B" // BREAK - broken
	PkgThrow PkgState = "T" // THROW - thrown away
	PkgFork  PkgState = "F" // FORK  - forked
)

// PkgOrg represents the package origin/organization
type PkgOrg string

const (
	OrgPigsty  PkgOrg = "P" // Pigsty repository
	OrgPGDG    PkgOrg = "G" // PGDG repository
	OrgUnknown PkgOrg = ""  // Unknown origin
)

// PkgMatrixEntry represents a single entry in the package availability matrix
type PkgMatrixEntry struct {
	OS      string   // OS code (e.g., "d12", "el9", "u24")
	Arch    string   // Architecture: "amd64" or "arm64"
	PG      int      // PostgreSQL major version
	State   PkgState // Availability state
	Hide    bool     // Hidden flag
	Count   int      // Package count
	Org     PkgOrg   // Package organization
	Version string   // Package version string
}

// PkgMatrix is a collection of package availability entries
type PkgMatrix []*PkgMatrixEntry

// Display constants
const (
	cellWidth  = 14
	osColWidth = 14
)

// Standard OS display order and valid codes
var (
	osDisplayOrder = []string{"el8", "el9", "el10", "d12", "d13", "u22", "u24"}
	validOSCodes   = map[string]bool{"el8": true, "el9": true, "el10": true, "d12": true, "d13": true, "u22": true, "u24": true}
)

/********************
* Parsing
********************/

// ParsePkgMatrixEntry parses a compressed matrix entry string
// Format: "d12a:18:A:f:1:p:2.24.0" -> OS+Arch:PG:State:Hide:Count:Org:Version
func ParsePkgMatrixEntry(s string) *PkgMatrixEntry {
	parts := strings.Split(s, ":")
	if len(parts) < 4 {
		return nil
	}

	osArch := parts[0]
	if len(osArch) < 2 {
		return nil
	}

	// Last char: 'i' = amd64, 'a' = arm64
	var arch string
	switch osArch[len(osArch)-1] {
	case 'i':
		arch = "amd64"
	case 'a':
		arch = "arm64"
	default:
		return nil
	}

	pg, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil
	}

	entry := &PkgMatrixEntry{
		OS:    osArch[:len(osArch)-1],
		Arch:  arch,
		PG:    pg,
		State: PkgState(parts[2]),
		Hide:  len(parts) > 3 && parts[3] == "t",
	}

	if len(parts) > 4 {
		entry.Count, _ = strconv.Atoi(parts[4])
	}
	if len(parts) > 5 {
		switch strings.ToUpper(parts[5]) {
		case "P":
			entry.Org = OrgPigsty
		case "G", "D":
			entry.Org = OrgPGDG
		}
	}
	if len(parts) > 6 {
		entry.Version = parts[6]
	}

	return entry
}

// GetPkgMatrix extracts and parses the package matrix from Extension.Extra["matrix"]
func (e *Extension) GetPkgMatrix() PkgMatrix {
	if e == nil || e.Extra == nil {
		return nil
	}

	arr, ok := e.Extra["matrix"].([]interface{})
	if !ok {
		return nil
	}

	matrix := make(PkgMatrix, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			if entry := ParsePkgMatrixEntry(s); entry != nil {
				matrix = append(matrix, entry)
			}
		}
	}
	return matrix
}

/********************
* Query Methods
********************/

// Get returns the entry matching OS, arch, and PG version
func (m PkgMatrix) Get(os, arch string, pg int) *PkgMatrixEntry {
	for _, e := range m {
		if e != nil && e.OS == os && e.Arch == arch && e.PG == pg {
			return e
		}
	}
	return nil
}

// FilterByOS returns entries matching the given OS code
func (m PkgMatrix) FilterByOS(os string) PkgMatrix {
	return m.filter(func(e *PkgMatrixEntry) bool { return e.OS == os })
}

// FilterByArch returns entries matching the given architecture
func (m PkgMatrix) FilterByArch(arch string) PkgMatrix {
	return m.filter(func(e *PkgMatrixEntry) bool { return e.Arch == arch })
}

// FilterByPG returns entries matching the given PostgreSQL version
func (m PkgMatrix) FilterByPG(pg int) PkgMatrix {
	return m.filter(func(e *PkgMatrixEntry) bool { return e.PG == pg })
}

// FilterAvailable returns entries with Available state
func (m PkgMatrix) FilterAvailable() PkgMatrix {
	return m.filter(func(e *PkgMatrixEntry) bool { return e.State == PkgAvail })
}

func (m PkgMatrix) filter(fn func(*PkgMatrixEntry) bool) PkgMatrix {
	if m == nil {
		return nil
	}
	var result PkgMatrix
	for _, e := range m {
		if e != nil && fn(e) {
			result = append(result, e)
		}
	}
	return result
}

// IsAvailable checks if a package is available for the given OS, arch, and PG
func (m PkgMatrix) IsAvailable(os, arch string, pg int) bool {
	entry := m.Get(os, arch, pg)
	return entry != nil && entry.State == PkgAvail
}

// GetVersion returns the version string for the given OS, arch, and PG
func (m PkgMatrix) GetVersion(os, arch string, pg int) string {
	if entry := m.Get(os, arch, pg); entry != nil {
		return entry.Version
	}
	return ""
}

// OSList returns a sorted list of unique OS codes
func (m PkgMatrix) OSList() []string {
	if m == nil {
		return nil
	}
	seen := make(map[string]bool)
	for _, e := range m {
		if e != nil {
			seen[e.OS] = true
		}
	}
	result := make([]string, 0, len(seen))
	for os := range seen {
		result = append(result, os)
	}
	sort.Strings(result)
	return result
}

// PGList returns a sorted list of unique PostgreSQL versions (descending)
func (m PkgMatrix) PGList() []int {
	if m == nil {
		return nil
	}
	seen := make(map[int]bool)
	for _, e := range m {
		if e != nil {
			seen[e.PG] = true
		}
	}
	result := make([]int, 0, len(seen))
	for pg := range seen {
		result = append(result, pg)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(result)))
	return result
}

// LatestVersion returns the highest available version string
func (m PkgMatrix) LatestVersion() string {
	var latest string
	for _, e := range m {
		if e != nil && e.State == PkgAvail && e.Version != "" {
			if latest == "" || compareVersions(e.Version, latest) > 0 {
				latest = e.Version
			}
		}
	}
	return latest
}

func compareVersions(v1, v2 string) int {
	parts1, parts2 := strings.Split(v1, "."), strings.Split(v2, ".")
	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}
	for i := 0; i < maxLen; i++ {
		var n1, n2 int
		if i < len(parts1) {
			n1, _ = strconv.Atoi(parts1[i])
		}
		if i < len(parts2) {
			n2, _ = strconv.Atoi(parts2[i])
		}
		if n1 > n2 {
			return 1
		}
		if n1 < n2 {
			return -1
		}
	}
	return 0
}

/********************
* Display Helpers
********************/

func osFullName(os, arch string) string {
	archName := "x86_64"
	if arch == "arm64" {
		archName = "aarch64"
	}
	return os + "." + archName
}

// ANSI color codes
const (
	colorReset   = "\033[0m"
	colorRed     = "\033[31m"
	colorGreen   = "\033[32m"
	colorYellow  = "\033[33m"
	colorBlue    = "\033[34m"
	colorMagenta = "\033[35m"
	colorGray    = "\033[90m"
)

// Status display strings
const (
	StatusInstalled = "installed"
	StatusAvailable = "available"
	StatusNotAvail  = "not avail"
)

// GetExtensionStatus returns colored status string for an extension
// Priority: 1) Check if installed, 2) Check Matrix availability, 3) Fallback to PgVer
func GetExtensionStatus(e *Extension, pgVer int, osCode, arch string) string {
	if e == nil {
		return colorRed + StatusNotAvail + colorReset
	}

	// Check if installed (Postgres must be detected and extension in map)
	if Postgres != nil && Postgres.ExtensionMap != nil {
		if Postgres.ExtensionMap[e.Name] != nil {
			return colorGreen + StatusInstalled + colorReset
		}
	}

	// Check availability via Matrix (preferred method)
	available := checkMatrixAvailability(e, pgVer, osCode, arch)
	if available {
		return colorYellow + StatusAvailable + colorReset
	}

	return colorRed + StatusNotAvail + colorReset
}

// getLeadExtension returns the lead extension for matrix data lookup
func getLeadExtension(e *Extension) *Extension {
	if e == nil {
		return nil
	}
	if e.Lead || e.LeadExt == "" || Catalog == nil {
		return e
	}
	if lead, ok := Catalog.ExtNameMap[e.LeadExt]; ok {
		return lead
	}
	return e
}

// checkMatrixAvailability checks if extension is available using Matrix data with PgVer fallback
func checkMatrixAvailability(e *Extension, pgVer int, osCode, arch string) bool {
	leadExt := getLeadExtension(e)
	if leadExt == nil {
		return false
	}

	// Try Matrix data first
	matrix := leadExt.GetPkgMatrix()
	if len(matrix) > 0 {
		entry := matrix.Get(osCode, arch, pgVer)
		return entry != nil && entry.State == PkgAvail
	}

	// Fallback: use PgVer field
	pgVerStr := strconv.Itoa(pgVer)
	for _, v := range leadExt.PgVer {
		if strings.TrimSpace(v) == pgVerStr {
			return true
		}
	}
	return false
}

// centerStr centers a string within width
func centerStr(s string, width int) string {
	if len(s) >= width {
		return s
	}
	pad := width - len(s)
	return strings.Repeat(" ", pad/2) + s + strings.Repeat(" ", pad-pad/2)
}

// formatCell formats a matrix entry with ANSI colors, centered
func formatCell(entry *PkgMatrixEntry, width int) string {
	if entry == nil || entry.State == PkgMiss {
		return strings.Repeat(" ", width)
	}

	text := entry.Version
	if text == "" {
		text = "-"
	}

	// Truncate if needed
	if len(text) > width-2 {
		text = text[:width-3] + "~"
	}

	// Center the colored text
	pad := width - len(text)
	left, right := pad/2, pad-pad/2
	color := entryColor(entry)
	return strings.Repeat(" ", left) + color + text + colorReset + strings.Repeat(" ", right)
}

// entryColor returns the ANSI color for a matrix entry
// Colors: Green=PIGSTY, Blue=PGDG, Yellow=BREAK, Red=THROW, Magenta=FORK, Gray=HIDE
func entryColor(entry *PkgMatrixEntry) string {
	if entry == nil {
		return ""
	}
	switch entry.State {
	case PkgBreak:
		return colorYellow
	case PkgThrow:
		return colorRed
	case PkgFork:
		return colorMagenta
	case PkgHide:
		return colorGray
	default:
		switch entry.Org {
		case OrgPigsty:
			return colorGreen
		case OrgPGDG:
			return colorBlue
		}
	}
	return ""
}

// tableBorders creates Unicode table border strings
func tableBorders(firstColWidth, numCols, colWidth int) (top, headerSep, rowSep, bottom string) {
	first := strings.Repeat("─", firstColWidth)
	cell := strings.Repeat("─", colWidth)

	top = "╭" + first
	headerSep = "├" + first
	rowSep = "├" + first
	bottom = "╰" + first

	for i := 0; i < numCols; i++ {
		top += "┬" + cell
		headerSep += "┼" + cell
		rowSep += "┼" + cell
		bottom += "┴" + cell
	}

	return top + "╮\n", headerSep + "┤\n", rowSep + "┤\n", bottom + "╯\n"
}

/********************
* Matrix Display
********************/

// TabulateAvailability generates an availability matrix table
func (m PkgMatrix) TabulateAvailability() string {
	if len(m) == 0 {
		return "No availability data"
	}

	pgVersions := m.PGList()
	if len(pgVersions) == 0 {
		return "No PG versions found"
	}

	// Collect OS/arch pairs present in the matrix
	type osArch struct{ os, arch string }
	var rows []osArch
	for _, os := range osDisplayOrder {
		if len(m.FilterByOS(os)) > 0 {
			rows = append(rows, osArch{os, "amd64"}, osArch{os, "arm64"})
		}
	}

	top, headerSep, rowSep, bottom := tableBorders(osColWidth, len(pgVersions), cellWidth)

	var sb strings.Builder
	sb.WriteString(top)

	// Header
	sb.WriteString(fmt.Sprintf("│ %-*s", osColWidth-1, "OS \\ PG"))
	for _, pg := range pgVersions {
		sb.WriteString("│" + centerStr(strconv.Itoa(pg), cellWidth))
	}
	sb.WriteString("│\n")
	sb.WriteString(headerSep)

	// Data rows
	for i, row := range rows {
		sb.WriteString("│" + fmt.Sprintf(" %-*s", osColWidth-1, osFullName(row.os, row.arch)))
		for _, pg := range pgVersions {
			sb.WriteString("│" + formatCell(m.Get(row.os, row.arch, pg), cellWidth))
		}
		sb.WriteString("│\n")
		if i < len(rows)-1 {
			sb.WriteString(rowSep)
		}
	}

	sb.WriteString(bottom)
	return sb.String()
}

// Summary returns availability count (e.g., "84/84 avail")
func (m PkgMatrix) Summary() string {
	if len(m) == 0 {
		return "No data"
	}
	return fmt.Sprintf("%d/%d avail", len(m.FilterAvailable()), len(m))
}

/********************
* Print Functions
********************/

// colorLegend returns the color legend string
func colorLegend() string {
	return fmt.Sprintf("(%s%s%s = PIGSTY, %s%s%s = PGDG)",
		colorGreen, "green", colorReset, colorBlue, "blue", colorReset)
}

// PrintAvailability prints the availability matrix for an extension
func PrintAvailability(e *Extension) {
	if e == nil {
		return
	}

	fmt.Printf("\n%s (%s) - %s\n", e.Name, e.Pkg, e.EnDesc)

	// Get lead extension for matrix data
	leadExt := getLeadExtension(e)
	if leadExt != e {
		fmt.Printf("(Matrix data from lead extension: %s)\n", leadExt.Name)
	}

	matrix := leadExt.GetPkgMatrix()
	if len(matrix) == 0 {
		fmt.Println("No availability matrix data available")
		return
	}

	// Build info line: "Latest: x.y.z | N/M avail, PG18, PG17, ..."
	var info strings.Builder
	if ver := matrix.LatestVersion(); ver != "" {
		info.WriteString("Latest: " + ver + " | ")
	}
	info.WriteString(matrix.Summary())
	if pgVers := leadExt.GetPGVersions(); len(pgVers) > 0 {
		pgStrs := make([]string, len(pgVers))
		for i, pg := range pgVers {
			pgStrs[i] = fmt.Sprintf("PG%d", pg)
		}
		info.WriteString(", " + strings.Join(pgStrs, ", "))
	}
	fmt.Println(info.String())

	fmt.Printf("Details: https://pgext.cloud/e/%s  %s\n\n", e.Name, colorLegend())
	fmt.Print(matrix.TabulateAvailability())
}

// PrintGlobalAvailability prints package availability matrix on current OS
func PrintGlobalAvailability() {
	PrintGlobalAvailabilityFor("", "")
}

// PrintGlobalAvailabilityFor prints package availability for specified OS/arch
func PrintGlobalAvailabilityFor(osCode, arch string) {
	if Catalog == nil || len(Catalog.Extensions) == 0 {
		fmt.Println("No extension catalog available")
		return
	}

	if osCode == "" {
		osCode = config.OSCode
	}
	if arch == "" {
		arch = config.OSArch
	}

	if !validOSCodes[osCode] {
		fmt.Printf("\nNote: Current OS '%s' is not a supported Linux distribution.\n", osCode)
		fmt.Println("Supported OS: el8, el9, el10, d12, d13, u22, u24")
		fmt.Println("Showing matrix for el9.x86_64 as example:")
		osCode, arch = "el9", "amd64"
	}

	// Count totals and collect lead packages
	var packages []*Extension
	for _, ext := range Catalog.Extensions {
		if ext.Contrib {
			continue
		}
		if ext.Lead {
			packages = append(packages, ext)
		}
	}

	osName := osFullName(osCode, arch)
	fmt.Printf("\nExtension Availability on %s : https://pgext.cloud/os/%s\n", osName, osName)
	fmt.Printf("Showing %d packages with %d extensions  %s\n\n", len(packages), len(Catalog.Extensions), colorLegend())

	sort.Slice(packages, func(i, j int) bool {
		return packages[i].ID < packages[j].ID
	})

	fmt.Print(tabulateGlobalMatrix(packages, osCode, arch, []int{18, 17, 16, 15, 14, 13}))
}

func tabulateGlobalMatrix(packages []*Extension, osCode, arch string, pgVersions []int) string {
	if len(packages) == 0 {
		return "No packages to display\n"
	}

	// First pass: collect data and find column widths
	type rowData struct {
		name    string
		entries []*PkgMatrixEntry
	}
	rows := make([]rowData, 0, len(packages))
	nameWidth := 3 // minimum "Pkg"
	verWidths := make([]int, len(pgVersions))
	for i, pg := range pgVersions {
		verWidths[i] = len(strconv.Itoa(pg))
	}

	for _, pkg := range packages {
		if len(pkg.Pkg) > nameWidth {
			nameWidth = len(pkg.Pkg)
		}

		matrix := pkg.GetPkgMatrix()
		entries := make([]*PkgMatrixEntry, len(pgVersions))
		for i, pg := range pgVersions {
			if matrix != nil {
				entries[i] = matrix.Get(osCode, arch, pg)
			}
			if entries[i] != nil && len(entries[i].Version) > verWidths[i] {
				verWidths[i] = len(entries[i].Version)
			}
		}
		rows = append(rows, rowData{name: pkg.Pkg, entries: entries})
	}

	// Build output
	var sb strings.Builder

	// Header
	sb.WriteString(fmt.Sprintf("%-*s", nameWidth, "Pkg"))
	for i, pg := range pgVersions {
		sb.WriteString(fmt.Sprintf("  %-*d", verWidths[i], pg))
	}
	sb.WriteString("\n")

	// Data rows
	for _, row := range rows {
		sb.WriteString(fmt.Sprintf("%-*s", nameWidth, row.name))
		for i, entry := range row.entries {
			sb.WriteString("  ")
			if entry == nil || entry.State == PkgMiss {
				sb.WriteString(strings.Repeat(" ", verWidths[i]))
			} else {
				ver := entry.Version
				if ver == "" {
					ver = "-"
				}
				sb.WriteString(entryColor(entry) + fmt.Sprintf("%-*s", verWidths[i], ver) + colorReset)
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
