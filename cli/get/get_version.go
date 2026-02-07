package get

import (
	"bufio"
	"fmt"
	"io"
	"pig/internal/config"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// ParseChecksums parses checksum file content into VersionInfo structs
func ParseChecksums(r io.Reader, source string) ([]VersionInfo, error) {
	var versions []VersionInfo
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			continue
		}

		checksum, filename := fields[0], fields[1]
		version, err := GetVerFromName(filename)
		if err != nil {
			continue
		}

		var downloadURL string
		if source == ViaIO {
			downloadURL = fmt.Sprintf("%s/src/%s", config.RepoPigstyIO, filename)
		} else {
			downloadURL = fmt.Sprintf("%s/src/%s", config.RepoPigstyCC, filename)
		}
		versions = append(versions, VersionInfo{
			Version:     version,
			Checksum:    checksum,
			DownloadURL: downloadURL,
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to parse checksums: %w", err)
	}

	// Sort versions in descending order
	sort.Slice(versions, func(i, j int) bool {
		return CompareVersions(versions[i].Version, versions[j].Version) > 0
	})

	return versions, nil
}

// GetVerFromName extracts semantic version from filename
// Format: pigsty-vX.Y.Z[-{a|b|c|alpha|beta|rc}N].tgz
func GetVerFromName(filename string) (string, error) {
	re := regexp.MustCompile(`^pigsty-(v\d+\.\d+\.\d+(?:-(?:a|b|c|alpha|beta|rc)\d+)?)\.tgz$`)
	matches := re.FindStringSubmatch(filename)
	if len(matches) != 2 {
		return "", fmt.Errorf("invalid filename format: %s", filename)
	}
	return matches[1], nil
}

// CompareVersions compares two semantic versions
// Returns: 1 if v1 > v2, -1 if v1 < v2, 0 if equal
func CompareVersions(v1, v2 string) int {
	v1 = strings.TrimPrefix(v1, "v")
	v2 = strings.TrimPrefix(v2, "v")

	v1Parts := strings.Split(v1, "-")
	v2Parts := strings.Split(v2, "-")

	if cmp := compareMainVersion(v1Parts[0], v2Parts[0]); cmp != 0 {
		return cmp
	}
	return comparePreRelease(v1Parts, v2Parts)
}

// compareMainVersion compares the main version numbers (X.Y.Z)
func compareMainVersion(v1, v2 string) int {
	nums1 := strings.Split(v1, ".")
	nums2 := strings.Split(v2, ".")

	for i := 0; i < len(nums1) || i < len(nums2); i++ {
		n1, n2 := 0, 0
		if i < len(nums1) {
			n1, _ = strconv.Atoi(nums1[i])
		}
		if i < len(nums2) {
			n2, _ = strconv.Atoi(nums2[i])
		}
		if n1 != n2 {
			return n1 - n2
		}
	}
	return 0
}

// CompleteVersion will complete half-baked version string into latest match stable version
func CompleteVersion(version string) string {
	if version == "latest" {
		return LatestVersion
	}
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}

	// If it's already a valid version, return as-is
	if IsValidVersion(version) != nil {
		return version
	}

	// Split version into parts
	prefix := version

	// Find highest matching stable version
	var highest string
	for _, v := range AllVersions {
		if strings.Contains(v.Version, "-") {
			continue // Skip pre-release versions
		}
		// Check if version matches our prefix
		if strings.HasPrefix(v.Version, prefix) {
			if highest == "" || CompareVersions(v.Version, highest) > 0 {
				highest = v.Version
			}
		}
	}
	if highest != "" {
		return highest
	}
	return version
}

// comparePreRelease compares pre-release versions
// Priority: release > rc/c > beta/b > alpha/a
func comparePreRelease(v1Parts, v2Parts []string) int {
	// Release versions take precedence
	switch {
	case len(v1Parts) == 1 && len(v2Parts) == 1:
		return 0
	case len(v1Parts) == 1:
		return 1
	case len(v2Parts) == 1:
		return -1
	}

	type preRelease struct {
		typ string
		num int
	}

	parse := func(s string) preRelease {
		var typ string
		var num int
		switch {
		case strings.HasPrefix(s, "alpha"):
			typ = "a"
			num, _ = strconv.Atoi(s[5:])
		case strings.HasPrefix(s, "beta"):
			typ = "b"
			num, _ = strconv.Atoi(s[4:])
		case strings.HasPrefix(s, "rc"):
			typ = "c"
			num, _ = strconv.Atoi(s[2:])
		default:
			typ = s[0:1]
			num, _ = strconv.Atoi(s[1:])
		}
		return preRelease{typ, num}
	}

	pr1 := parse(v1Parts[1])
	pr2 := parse(v2Parts[1])

	if pr1.typ != pr2.typ {
		if pr1.typ > pr2.typ {
			return 1
		}
		return -1
	}

	return pr1.num - pr2.num
}
