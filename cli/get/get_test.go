package get

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestNetworkConditionSimple(t *testing.T) {
	start := time.Now()
	NetworkCondition()
	fmt.Printf("time: %v, result: %v, region: %v, latest: %v\n", time.Since(start), Source, Region, LatestVersion)
}

func TestGetVerFromName(t *testing.T) {
	tests := []struct {
		filename string
		expected string
		wantErr  bool
	}{
		{"pigsty-v1.0.0.tgz", "v1.0.0", false},
		{"pigsty-v2.3.1.tgz", "v2.3.1", false},
		{"pigsty-v3.5.1.tgz", "v3.5.1", false},
		{"pigsty-v2.3.1-a1.tgz", "v2.3.1-a1", false},
		{"pigsty-v3.5.1-b1.tgz", "v3.5.1-b1", false},
		{"pigsty-v2.3.1-c1.tgz", "v2.3.1-c1", false},
		{"pigsty-v3.5.1-alpha3.tgz", "v3.5.1-alpha3", false},
		{"pigsty-v3.5.1-beta4.tgz", "v3.5.1-beta4", false},
		{"pigsty-v3.5.1-rc5.tgz", "v3.5.1-rc5", false},
		{"invalid-filename.tgz", "", true},
		{"pgisty-v1.2.3.tgz", "", true},
		{"pigsty-v1.2.3.tar.gz", "", true},
	}

	for _, tt := range tests {
		version, err := GetVerFromName(tt.filename)
		if (err != nil) != tt.wantErr {
			t.Errorf("GetVerFromName(%s) error = %v, wantErr %v", tt.filename, err, tt.wantErr)
			continue
		}
		if version != tt.expected {
			t.Errorf("ExtractVersionFromFilename(%s) = %s, expected %s", tt.filename, version, tt.expected)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		v1       string
		v2       string
		expected int
	}{
		// Compare main version numbers
		{"v1.0.0", "v1.0.0", 0},
		{"v2.0.0", "v1.0.0", 1},
		{"v1.0.0", "v2.0.0", -1},
		{"v2.1.0", "v2.0.9", 1},
		{"v2.0.10", "v2.0.9", 1},
		{"v2.0.0", "v2.0.0.1", -1},
		{"v2.0.0", "v2.0.0", 0},

		// Compare release vs pre-release versions
		{"v2.0.0", "v2.0.0-a1", 1}, // Release is greater than alpha
		{"v2.0.0", "v2.0.0-b1", 1}, // Release is greater than beta
		{"v2.0.0", "v2.0.0-c1", 1}, // Release is greater than rc

		// Compare different pre-release types
		{"v2.0.0-a1", "v2.0.0-b1", -1}, // alpha < beta
		{"v2.0.0-b1", "v2.0.0-c1", -1}, // beta < rc
		{"v2.0.0-a1", "v2.0.0-c1", -1}, // alpha < rc

		// Compare same pre-release type with different numbers
		{"v2.0.0-a1", "v2.0.0-a2", -1},
		{"v2.0.0-b1", "v2.0.0-b2", -1},
		{"v2.0.0-c1", "v2.0.0-c2", -1},

		// Compare versions with pre-release across different versions
		{"v2.0.0-c1", "v2.0.1", -1},    // Lower version with rc < higher version
		{"v2.0.0-b1", "v2.0.1-a1", -1}, // Lower version with beta < higher version with alpha

		// Compare pre-release type with full name
		{"v2.0.0-alpha1", "v2.0.0-b1", -1}, // alpha < beta
		{"v2.0.0-beta1", "v2.0.0-rc1", -1}, // beta < rc
		{"v2.0.0-a1", "v2.0.0-rc1", -1},    // alpha < rc
	}

	for _, tt := range tests {
		result := CompareVersions(tt.v1, tt.v2)
		if result != tt.expected {
			t.Errorf("CompareVersions(%s, %s) = %d, expected %d", tt.v1, tt.v2, result, tt.expected)
		}
	}
}

func TestParseChecksums(t *testing.T) {
	data := `
649c7b9f778c61324cb6d350dbda4f5e  pigsty-v0.8.0.tgz
8436905916465e74bfcb3d8192c11c85  pigsty-v0.9.0.tgz
0b9958a9305775a703a990d7c6728c21  pigsty-v1.0.0.tgz
invalid line without proper format
e62f9ce9f89a58958609da7b234bf2f2  pigsty-v3.1.0.tgz
`

	reader := strings.NewReader(data)
	versions, err := ParseChecksums(reader, "pigsty")

	if err != nil {
		t.Errorf("ParseChecksums error: %v", err)
	}

	if len(versions) != 4 {
		t.Errorf("Expected 4 versions, got %d", len(versions))
	}

	expectedVersions := []string{"v3.1.0", "v1.0.0", "v0.9.0", "v0.8.0"}
	for i, v := range versions {
		if v.Version != expectedVersions[i] {
			t.Errorf("Expected version %s at index %d, got %s", expectedVersions[i], i, v.Version)
		}
	}
}

func TestCompleteVersion(t *testing.T) {
	// Save original AllVersions and restore after test
	NetworkCondition()
	GetAllVersions(true)

	tests := []struct {
		input    string
		expected string
	}{
		// Complete version strings should return as-is
		{"v1.0.0", "v1.0.0"},
		{"v2.0.0", "v2.0.0"},

		// Partial versions should return highest matching stable version
		{"v1", "v1.5.1"},
		{"v1.0", "v1.0.0"},
		{"v2", "v2.7.0"},
		{"v2.0", "v2.0.2"},

		// Version without v prefix should get prefix added
		{"1.0.0", "v1.0.0"},
		{"2.0.0", "v2.0.0"},

		// Non-matching versions should return unchanged
		{"v9.9.9", "v9.9.9"},
		{"v4", "v4"},

		// Pre-release versions should be ignored when completing
		{"v2.0", "v2.0.2"},
	}

	for _, tt := range tests {
		result := CompleteVersion(tt.input)
		if result != tt.expected {
			t.Errorf("CompleteVersion(%s) = %s, expected %s", tt.input, result, tt.expected)
		}
	}
}
