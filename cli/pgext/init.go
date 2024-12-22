package pgext

import (
	"bytes"
	_ "embed"
	"encoding/csv"
	"fmt"
	"pig/cli/pgsql"
	"sort"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
)

//go:embed assets/pigsty.csv
var embedExtensionData []byte

var (
	Extensions         []*Extension
	ExtNameMap         map[string]*Extension
	ExtAliasMap        map[string]*Extension
	NeedBy             map[string][]string = make(map[string][]string)
	Postgres           *pgsql.PostgresInstallation
	PostgresPackageMap map[string]string
	DefaultPgVer       = 17
)

var (
	PostgresVersions = []string{"17", "16", "15", "14", "13"}
	CategoryList     = []string{"TIME", "GIS", "RAG", "FTS", "OLAP", "FEAT", "LANG", "TYPE", "FUNC", "ADMIN", "STAT", "SEC", "FDW", "SIM", "ETL"}
)

/********************
* Init Postgres
********************/

// InitPostgres will initialize the Postgres instance
func InitPostgres(path string, ver int) (err error) {
	Postgres, err = pgsql.GetPostgres(path, ver)
	if err != nil {
		return fmt.Errorf("failed to get PostgreSQL installation: %v", err)
	}
	if Postgres == nil {
		return fmt.Errorf("failed to get PostgreSQL installation")
	}
	return nil
}

/********************
* Init Extension
********************/

// InitExtension initializes extension data from embedded CSV or file
func InitExtension(data []byte) error {
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
