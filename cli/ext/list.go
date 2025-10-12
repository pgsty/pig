package ext

import (
	"fmt"
	"os"
	"pig/internal/config"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/sirupsen/logrus"
)

var CategoryMap = map[string]string{
	"time":  "TIME",
	"gis":   "GIS",
	"rag":   "RAG",
	"fts":   "FTS",
	"olap":  "OLAP",
	"feat":  "FEAT",
	"lang":  "LANG",
	"type":  "TYPE",
	"func":  "FUNC",
	"admin": "ADMIN",
	"stat":  "STAT",
	"sec":   "SEC",
	"fdw":   "FDW",
	"sim":   "SIM",
	"etl":   "ETL",
}

// SearchResult represents a search result with similarity score
type SearchResult struct {
	Extension *Extension
	Score     float64
}

// TabulteVersion prints a tabulated list of extensions available to given version
func TabulteVersion(pgVer int, data []*Extension) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Name\tState\tVersion\tCate\tFlags\tLicense\tRepo\tPGVer\tPackage\tDescription")
	fmt.Fprintln(w, "----\t-----\t-------\t----\t------\t-------\t------\t-----\t------------\t---------------------")
	if Postgres != nil {
		pgVer = Postgres.MajorVersion
	}
	for _, ext := range data {
		desc := ext.EnDesc
		if len(desc) > 64 {
			desc = desc[:64]
		}
		pkgStr := ext.PackageName(pgVer)
		if strings.Contains(pkgStr, "$v") {
			pkgStr = fmt.Sprintf("[%s]", pkgStr)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			ext.Name, ext.GetStatus(pgVer), ext.Version, ext.Category, ext.GetFlag(), ext.License, ext.RepoName(), ext.Availability(config.OSCode), pkgStr, desc)
	}
	w.Flush()
	fmt.Printf("\n(%d Rows) (State: added|avail|n/a, Flags: b = HasBin, d = HasDDL, s = HasLib, l = NeedLoad, t = Trusted, r = Relocatable, x = Unknown)\n\n", len(data))
}

func TabulteCommon(data []*Extension) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Name\tVersion\tCate\tFlags\tLicense\tRPM\tDEB\tPG Ver\tDescription")
	fmt.Fprintln(w, "----\t-------\t----\t------\t-------\t------\t------\t------\t---------------------")
	for _, ext := range data {
		desc := ext.EnDesc
		if len(desc) > 64 {
			desc = desc[:64] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			ext.Name, ext.Version, ext.Category, ext.GetFlag(), ext.License, ext.RpmRepo, ext.DebRepo, CompactVersion(ext.PgVer), desc)
	}
	w.Flush()
	fmt.Printf("\n(%d Rows) (Flags: b = HasBin, d = HasDDL, s = HasLib, l = NeedLoad, t = Trusted, r = Relocatable, x = Unknown)\n\n", len(data))
}

// SearchExtensions performs fuzzy search on extensions
func SearchExtensions(query string, exts []*Extension) []*Extension {
	if query == "" {
		return exts
	}
	logrus.Debugf("search extensions with query: %s", query)
	query = strings.ToLower(query)

	// First check: exact category match
	if category, ok := CategoryMap[query]; ok {
		logrus.Debugf("category %s is given", category)
		var categoryResults []*Extension
		for _, ext := range exts {
			if ext.Category == category {
				categoryResults = append(categoryResults, ext)
			}
		}
		if len(categoryResults) > 0 {
			return categoryResults
		}
	}

	// Second check: exact name or pkg match
	for _, ext := range exts {
		// Check exact match in name
		if strings.ToLower(ext.Name) == query {
			return []*Extension{ext}
		}
		// Check exact match in pkg
		if strings.ToLower(ext.Pkg) == query {
			return []*Extension{ext}
		}
	}

	// If no exact matches, proceed with fuzzy search
	var results []SearchResult

	// Fuzzy search pass
	for _, ext := range exts {
		// Calculate best score from name, pkg and descriptions
		var bestScore float64

		// Check name similarity
		nameScore := similarity(query, strings.ToLower(ext.Name))
		bestScore = nameScore

		// Check pkg similarity
		if pkgScore := similarity(query, strings.ToLower(ext.Pkg)); pkgScore > bestScore {
			bestScore = pkgScore
		}

		// Check English description (with lower weight)
		if descScore := similarity(query, strings.ToLower(ext.EnDesc)) * 0.7; descScore > bestScore {
			bestScore = descScore
		}

		// Check Chinese description (with lower weight)
		if descScore := similarity(query, strings.ToLower(ext.ZhDesc)) * 0.7; descScore > bestScore {
			bestScore = descScore
		}

		// Add to results if score is above threshold
		if bestScore > 0.3 {
			results = append(results, SearchResult{ext, bestScore})
		}
	}

	// Sort results by score
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Convert to extension list
	var extensions []*Extension
	for _, result := range results {
		extensions = append(extensions, result.Extension)
	}

	// Limit to 10 results
	if len(extensions) > 10 {
		extensions = extensions[:10]
	}

	return extensions
}

// similarity calculates normalized similarity score between two strings
func similarity(s1, s2 string) float64 {
	distance := levenshteinDistance(s1, s2)
	maxLen := float64(max(len(s1), len(s2)))
	if maxLen == 0 {
		return 0
	}
	return 1 - float64(distance)/maxLen
}

// levenshteinDistance calculates the Levenshtein distance between two strings
func levenshteinDistance(s1, s2 string) int {
	if len(s1) == 0 {
		return len(s2)
	}
	if len(s2) == 0 {
		return len(s1)
	}

	// Create matrix
	matrix := make([][]int, len(s1)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(s2)+1)
	}

	// Initialize first row and column
	for i := 0; i <= len(s1); i++ {
		matrix[i][0] = i
	}
	for j := 0; j <= len(s2); j++ {
		matrix[0][j] = j
	}

	// Fill matrix
	for i := 1; i <= len(s1); i++ {
		for j := 1; j <= len(s2); j++ {
			if s1[i-1] == s2[j-1] {
				matrix[i][j] = matrix[i-1][j-1]
			} else {
				matrix[i][j] = min(
					matrix[i-1][j]+1, // deletion
					min(
						matrix[i][j-1]+1,   // insertion
						matrix[i-1][j-1]+1, // substitution
					),
				)
			}
		}
	}

	return matrix[len(s1)][len(s2)]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
