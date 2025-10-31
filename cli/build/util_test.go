package build

import (
	"testing"
)

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
