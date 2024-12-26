package repo

import (
	"testing"
)

func TestGetMajorVersionFromCode(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected int
	}{
		// EL Tests
		{name: "valid el7", code: "el7", expected: 7},
		{name: "valid el8", code: "el8", expected: 8},
		{name: "valid el9", code: "el9", expected: 9},
		{name: "invalid el format", code: "elx", expected: -1},
		{name: "el without version", code: "el", expected: -1},

		// Ubuntu Tests
		{name: "ubuntu focal", code: "focal", expected: 20},
		{name: "ubuntu jammy", code: "jammy", expected: 22},
		{name: "invalid ubuntu codename", code: "invalid", expected: -1},

		// Debian Tests
		{name: "debian bullseye", code: "bullseye", expected: 11},
		{name: "debian bookworm", code: "bookworm", expected: 12},
		{name: "debian trixie", code: "trixie", expected: 13},
		{name: "invalid debian codename", code: "invalid", expected: -1},

		// Edge Cases
		{name: "empty string", code: "", expected: -1},
		{name: "numeric only", code: "7", expected: -1},
		{name: "mixed case", code: "EL8", expected: 8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetMajorVersionFromCode(tt.code)
			if result != tt.expected {
				t.Errorf("GetMajorVersionFromCode(%s) = %v, want %v", tt.code, result, tt.expected)
			}
		})
	}
}
