package build

import (
	"testing"
)

func TestNormalizePackageName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Prefix removal tests
		{"Remove pg_ prefix", "pg_stat_kcache", "stat_kcache"},
		{"Remove pg- prefix", "pg-stat-kcache", "stat_kcache"},
		{"Remove postgresql- prefix", "postgresql-repack", "repack"},
		{"Remove postgresql_ prefix", "postgresql_repack", "repack"},

		// Version suffix removal
		{"Remove version suffix with underscore", "pg_repack_17", "repack"},
		{"Remove version suffix with hyphen", "pg-repack-17", "repack"},
		{"Keep non-numeric suffix", "pg_repack_test", "repack_test"},

		// Hyphen to underscore conversion
		{"Convert hyphens to underscores", "pg-stat-kcache", "stat_kcache"},
		{"Mixed separators", "pg_stat-kcache", "stat_kcache"},

		// Case normalization
		{"Lowercase conversion", "PG_STAT_KCACHE", "stat_kcache"},
		{"Mixed case", "Pg_Stat_Kcache", "stat_kcache"},

		// Complex cases
		{"Full normalization", "PostgreSQL-pg_repack-17", "pg_repack"},
		{"Already normalized", "pgvector", "pgvector"},
		{"Empty input", "", ""},
		{"Whitespace", "  pg_repack  ", "repack"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizePackageName(tt.input)
			if result != tt.expected {
				t.Errorf("normalizePackageName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParsePGVersions(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expected  []int
		expectErr bool
	}{
		// Valid inputs
		{"Single version", "16", []int{16}, false},
		{"Multiple versions", "14,15,16", []int{14, 15, 16}, false},
		{"With spaces", "14, 15, 16", []int{14, 15, 16}, false},
		{"Duplicates removed", "16,16,15", []int{16, 15}, false},
		{"Empty string", "", nil, false},

		// Invalid inputs
		{"Invalid number", "abc", nil, true},
		{"Mixed valid and invalid", "14,abc,16", nil, true},
		{"Version too low", "9", nil, true},
		{"Version too high", "25", nil, true},
		{"Negative version", "-1", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParsePGVersions(tt.input)

			if tt.expectErr {
				if err == nil {
					t.Errorf("ParsePGVersions(%q) expected error, got nil", tt.input)
				}
			} else {
				if err != nil {
					t.Errorf("ParsePGVersions(%q) unexpected error: %v", tt.input, err)
				}
				if !intSlicesEqual(result, tt.expected) {
					t.Errorf("ParsePGVersions(%q) = %v, want %v", tt.input, result, tt.expected)
				}
			}
		})
	}
}

// Helper function to compare int slices
func intSlicesEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}