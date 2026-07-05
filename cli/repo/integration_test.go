package repo

import (
	"pig/internal/config"
	"strings"
	"testing"
)

func TestRepoContentGeneration(t *testing.T) {
	// Save original values
	origMajor := config.OSMajor
	origArch := config.OSArch
	origType := config.OSType
	origVersionFull := config.OSVersionFull
	origVersion := config.OSVersion
	origCode := config.OSCode

	tests := []struct {
		name           string
		osMajor        int
		osArch         string
		osType         string
		osVersionFull  string
		osVersion      string
		osCode         string
		repoName       string
		minor          bool
		expectReplace  bool
		expectedString string
	}{
		{
			name:           "EL10 aarch64 pgdg17 should replace",
			osMajor:        10,
			osArch:         "aarch64",
			osType:         config.DistroEL,
			osVersionFull:  "10.0",
			osVersion:      "10",
			osCode:         "el10",
			repoName:       "pgdg17",
			expectReplace:  true,
			expectedString: "10.0",
		},
		{
			name:           "EL10 aarch64 pgdg14 should replace",
			osMajor:        10,
			osArch:         "aarch64",
			osType:         config.DistroEL,
			osVersionFull:  "10.0",
			osVersion:      "10",
			osCode:         "el10",
			repoName:       "pgdg14",
			expectReplace:  true,
			expectedString: "10.0",
		},
		{
			name:           "EL10 x86_64 pgdg17 should replace on minor-specific release",
			osMajor:        10,
			osArch:         "x86_64",
			osType:         config.DistroEL,
			osVersionFull:  "10.0",
			osVersion:      "10",
			osCode:         "el10",
			repoName:       "pgdg17",
			expectReplace:  true,
			expectedString: "10.0",
		},
		{
			name:           "EL9 aarch64 pgdg17 should not replace",
			osMajor:        9,
			osArch:         "aarch64",
			osType:         config.DistroEL,
			osVersionFull:  "9.4",
			osVersion:      "9",
			osCode:         "el9",
			repoName:       "pgdg17",
			expectReplace:  false,
			expectedString: "$releasever",
		},
		{
			name:           "EL10 epel should use z stream",
			osMajor:        10,
			osArch:         "aarch64",
			osType:         config.DistroEL,
			osVersionFull:  "10.2",
			osVersion:      "10",
			osCode:         "el10",
			repoName:       "epel",
			expectReplace:  true,
			expectedString: "10z",
		},
		{
			name:           "EL10 pgdg should use full minor",
			osMajor:        10,
			osArch:         "aarch64",
			osType:         config.DistroEL,
			osVersionFull:  "10.2",
			osVersion:      "10",
			osCode:         "el10",
			repoName:       "pgdg17",
			expectReplace:  true,
			expectedString: "10.2",
		},
		{
			name:           "EL9 pgdg 9.8 patch release should use major minor",
			osMajor:        9,
			osArch:         "aarch64",
			osType:         config.DistroEL,
			osVersionFull:  "9.8.1",
			osVersion:      "9",
			osCode:         "el9",
			repoName:       "pgdg17",
			expectReplace:  true,
			expectedString: "9.8",
		},
		{
			name:           "EL9 pgdg before 9.6 should not replace",
			osMajor:        9,
			osArch:         "aarch64",
			osType:         config.DistroEL,
			osVersionFull:  "9.5",
			osVersion:      "9",
			osCode:         "el9",
			repoName:       "pgdg17",
			expectReplace:  false,
			expectedString: "$releasever",
		},
		{
			name:           "explicit minor should override epel z stream",
			osMajor:        10,
			osArch:         "aarch64",
			osType:         config.DistroEL,
			osVersionFull:  "10.2",
			osVersion:      "10",
			osCode:         "el10",
			repoName:       "epel",
			minor:          true,
			expectReplace:  true,
			expectedString: "10.2",
		},
		{
			name:           "EL10 aarch64 pigsty should not replace",
			osMajor:        10,
			osArch:         "aarch64",
			osType:         config.DistroEL,
			osVersionFull:  "10.0",
			osVersion:      "10",
			osCode:         "el10",
			repoName:       "pigsty",
			expectReplace:  false,
			expectedString: "$releasever",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set test values
			config.OSMajor = tt.osMajor
			config.OSArch = tt.osArch
			config.OSType = tt.osType
			config.OSVersionFull = tt.osVersionFull
			config.OSVersion = tt.osVersion
			config.OSCode = tt.osCode

			// Create test repository
			repo := &Repository{
				Name:     tt.repoName,
				Distro:   config.DistroEL,
				Releases: []int{10},
				Arch:     []string{"x86_64", "aarch64"},
				BaseURL:  map[string]string{"default": "https://repo.example.com/el/$releasever/$basearch"},
				Meta:     map[string]string{"enabled": "1", "gpgcheck": "0"},
				Minor:    tt.minor,
			}

			// Generate content
			content := repo.Content("default")

			// Verify result
			if tt.expectReplace {
				if strings.Contains(content, "$releasever") {
					t.Errorf("Expected $releasever to be replaced for %s, but it wasn't. Content: %s", tt.name, content)
				}
				if !strings.Contains(content, tt.expectedString) {
					t.Errorf("Expected content to contain '%s' for %s, but it didn't. Content: %s", tt.expectedString, tt.name, content)
				}
			} else {
				if !strings.Contains(content, "$releasever") {
					t.Errorf("Expected $releasever to remain for %s, but it was replaced. Content: %s", tt.name, content)
				}
			}
		})
	}

	// Restore original values
	config.OSMajor = origMajor
	config.OSArch = origArch
	config.OSType = origType
	config.OSVersionFull = origVersionFull
	config.OSVersion = origVersion
	config.OSCode = origCode
}
