package utils

import (
	"testing"
)

func TestParsePostgresVersion(t *testing.T) {
	tests := []struct {
		input       string
		expectedMaj int
		expectedMin int
		expectErr   bool
	}{
		{"PostgreSQL 17.1", 17, 1, false},
		{"PostgreSQL 17beta2", 17, 0, false},
		{"PostgreSQL 16rc3", 16, 0, false},
		{"PostgreSQL 14alpha1", 14, 0, false},
		{"PostgreSQL 14alpha1.3", 14, 3, false}, // Assuming this format is valid
		{"PostgreSQL 15.10 (PolarDB 15.10.2.0 build 35199b32) on x86_64-linux-gnu", 15, 10, false},
		{"PostgreSQL 9.6", 9, 6, false},
		{"PostgreSQL 10.0", 10, 0, false},
		{"PostgreSQL 13", 13, 0, false},
		{"PostgreSQL 17Beta", 17, 0, false},            // Invalid format
		{"PostgreSQL nothing", 0, 0, true},             // Invalid format
		{"   PostgreSQL 17rc1  ", 17, 0, false},        // With spaces
		{"PostgreSQL 16rc3.1", 16, 1, false},           // rc + minor
		{"PostgreSQL 18", 18, 0, false},                // New major version
		{"PostgreSQL 18.0.1", 18, 0, false},            // Extra minor version ignored
		{"PostgreSQL 19alpha", 19, 0, false},           // Alpha without number
		{"PostgreSQL 19(alpha1)", 19, 0, false},        // Alpha with shit
		{"PostgreSQL 12(foo)", 12, 0, false},           // 12
		{"PostgreSQL 193", 0, 0, true},                 // insane version
		{"someotherfork 16", 16, 0, false},             // insane prefix
		{"PostgreSQL 19alpha2.3-lalala", 19, 3, false}, // insane version
		{"anyprefix 99.10", 99, 10, false},             // terrible prefix
		{"anyprefix 0.10", 0, 0, true},                 // terrible prefix
		{"PostgreSQL 123", 0, 0, true},                 // terrible prefix
		{"PostgreSQL 9.6.5", 9, 6, false},              // terrible prefix
		{"12.7", 12, 7, false},                         // major.minor
		{"13", 13, 0, false},                           // just major
		{"13beta0", 13, 0, false},                      // just major
		{"14alpha1", 14, 0, false},                     // just major
		{"15rc2", 15, 0, false},                        // just major

	}

	for _, test := range tests {
		major, minor, err := ParsePostgresVersion(test.input)
		if (err != nil) != test.expectErr {
			t.Errorf("ParsePostgresVersion(%q) unexpected error status: got %v, want error: %v", test.input, err, test.expectErr)
		}
		if major != test.expectedMaj || minor != test.expectedMin {
			t.Errorf("ParsePostgresVersion(%q) = major %d, minor %d; want major %d, minor %d", test.input, major, minor, test.expectedMaj, test.expectedMin)
		}
	}
}
