package ext

import (
	"fmt"
	"strconv"
	"strings"
)

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
