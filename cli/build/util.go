package build

import (
	"fmt"
	"pig/cli/ext"
	"pig/internal/config"
	"strconv"
	"strings"
)

// ResolvePackage resolves user input to standard extension package
// Handles various input formats: extension name, package name, aliases
func ResolvePackage(input string) (*ext.Extension, error) {
	if input == "" {
		return nil, fmt.Errorf("empty package name")
	}
	input = strings.TrimSpace(input)
	if e, ok := ext.Catalog.ExtNameMap[input]; ok {
		return e, nil
	}
	if e, ok := ext.Catalog.ExtPkgMap[input]; ok {
		return e, nil
	}
	return nil, fmt.Errorf("package '%s' not found in extension catalog", input)
}

// parsePgVersions parses comma-separated PG version string
func parsePgVersions(pgVersions string) ([]int, error) {
	if pgVersions == "" {
		return nil, nil
	}

	var versions []int
	seen := make(map[int]bool)

	for _, v := range strings.Split(pgVersions, ",") {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}

		ver, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid PG version: %s", v)
		}

		// Validate version range
		if ver < 10 || ver > 20 {
			return nil, fmt.Errorf("PG version %d out of valid range (10-20)", ver)
		}

		if !seen[ver] {
			versions = append(versions, ver)
			seen[ver] = true
		}
	}
	return versions, nil
}

// ParsePGVersions is the exported wrapper used by tests and external callers.
func ParsePGVersions(pgVersions string) ([]int, error) {
	return parsePgVersions(pgVersions)
}

// formatSize formats byte size to human-readable format
func formatSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%dB", size)
	} else if size < 1024*1024 {
		return fmt.Sprintf("%dKB", size/1024)
	} else {
		return fmt.Sprintf("%dMB", size/(1024*1024))
	}
}

// getElArch returns the system architecture alias used by EL
func getElArch() string {
	switch strings.ToLower(config.OSArch) {
	case "amd64", "x86_64", "x64":
		return "x86_64"
	case "arm64", "aarch64", "armv8":
		return "aarch64"
	default:
		return "x86_64" // default
	}
}

// truncateLine truncates a line to specified length
func truncateLine(line string, maxLen int) string {
	if len(line) <= maxLen {
		return line
	}
	return line[:maxLen-3] + "..."
}
