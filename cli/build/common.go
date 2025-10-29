package build

import (
	"fmt"
	"pig/cli/ext"
	"pig/internal/config"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
)

// ResolvePackage resolves user input to standard extension package
// Handles various input formats: extension name, package name, aliases
func ResolvePackage(input string) (*ext.Extension, error) {
	if input == "" {
		return nil, fmt.Errorf("empty package name")
	}

	// Normalize input
	normalized := normalizePackageName(input)

	// Try direct extension name lookup
	if e, ok := ext.Catalog.ExtNameMap[normalized]; ok {
		return e, nil
	}

	// Try package name lookup
	if e, ok := ext.Catalog.ExtPkgMap[normalized]; ok {
		return e, nil
	}

	// Try without normalization (for exact matches)
	if e, ok := ext.Catalog.ExtNameMap[input]; ok {
		return e, nil
	}

	if e, ok := ext.Catalog.ExtPkgMap[input]; ok {
		return e, nil
	}

	return nil, fmt.Errorf("package '%s' not found in catalog", input)
}

// ResolvePackages resolves multiple package inputs to extensions
// Returns successfully resolved extensions and logs warnings for failures
func ResolvePackages(inputs []string) ([]*ext.Extension, error) {
	if len(inputs) == 0 {
		return nil, fmt.Errorf("no packages specified")
	}

	seen := make(map[string]bool)
	var resolved []*ext.Extension
	var failures []string

	for _, input := range inputs {
		ext, err := ResolvePackage(input)
		if err != nil {
			failures = append(failures, input)
			logrus.Warnf("Failed to resolve package: %s", input)
			continue
		}

		// Avoid duplicates
		if seen[ext.Name] {
			continue
		}
		seen[ext.Name] = true
		resolved = append(resolved, ext)
	}

	if len(resolved) == 0 {
		return nil, fmt.Errorf("no valid packages found from inputs: %v", inputs)
	}

	return resolved, nil
}

// normalizePackageName standardizes various input formats
func normalizePackageName(input string) string {
	input = strings.TrimSpace(input)
	input = strings.ToLower(input)

	// Remove common prefixes
	prefixes := []string{"pg_", "pg-", "postgresql-", "postgresql_"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(input, prefix) {
			input = strings.TrimPrefix(input, prefix)
			break
		}
	}

	// Remove version suffixes (e.g., _17, -17)
	parts := strings.Split(input, "_")
	if len(parts) > 1 {
		if _, err := strconv.Atoi(parts[len(parts)-1]); err == nil {
			input = strings.Join(parts[:len(parts)-1], "_")
		}
	}

	parts = strings.Split(input, "-")
	if len(parts) > 1 {
		if _, err := strconv.Atoi(parts[len(parts)-1]); err == nil {
			input = strings.Join(parts[:len(parts)-1], "-")
		}
	}

	// Convert hyphens to underscores for consistency
	input = strings.ReplaceAll(input, "-", "_")

	return input
}

// ParsePGVersions parses comma-separated PG version string
func ParsePGVersions(pgVersions string) ([]int, error) {
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

// ValidateBuildExtension validates if extension can be built
func ValidateBuildExtension(ext *ext.Extension) error {
	switch config.OSType {
	case config.DistroEL:
		if ext.RpmRepo == "" || ext.Source == "" {
			return fmt.Errorf("extension '%s' does not support RPM build", ext.Name)
		}
	case config.DistroDEB:
		if ext.DebRepo == "" || ext.Source == "" {
			return fmt.Errorf("extension '%s' does not support DEB build", ext.Name)
		}
	case config.DistroMAC:
		return fmt.Errorf("macOS build not supported")
	default:
		return fmt.Errorf("unsupported OS: %s", config.OSType)
	}
	return nil
}

// GetPGVersionsForExtension returns appropriate PG versions for building
func GetPGVersionsForExtension(extension *ext.Extension, userVersions []int) []int {
	// Use user-specified versions if provided
	if len(userVersions) > 0 {
		return userVersions
	}

	// Use extension's supported versions based on OS
	var versions []int
	var versionStrs []string

	switch config.OSType {
	case config.DistroEL:
		versionStrs = extension.PgVer
	case config.DistroDEB:
		versionStrs = extension.PgVer
	default:
		versionStrs = extension.PgVer
	}

	for _, v := range versionStrs {
		if ver, err := strconv.Atoi(v); err == nil {
			versions = append(versions, ver)
		}
	}

	// Default to latest version if no versions found
	if len(versions) == 0 {
		versions = []int{ext.PostgresLatestMajorVersion}
	}

	return versions
}
