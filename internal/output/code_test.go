package output

import "testing"

func TestModuleConstants(t *testing.T) {
	// Verify module codes follow the 222 structure (MMCCNN where MM is module)
	modules := map[string]int{
		"MODULE_EXT":    MODULE_EXT,
		"MODULE_REPO":   MODULE_REPO,
		"MODULE_BUILD":  MODULE_BUILD,
		"MODULE_PG":     MODULE_PG,
		"MODULE_PB":     MODULE_PB,
		"MODULE_PT":     MODULE_PT,
		"MODULE_PITR":   MODULE_PITR,
		"MODULE_PE":     MODULE_PE,
		"MODULE_STY":    MODULE_STY,
		"MODULE_DO":     MODULE_DO,
		"MODULE_CONFIG": MODULE_CONFIG,
		"MODULE_SYSTEM": MODULE_SYSTEM,
	}

	// Verify expected values (module codes start from 10 to avoid octal issues)
	expectedModules := map[string]int{
		"MODULE_EXT":    100000,
		"MODULE_REPO":   110000,
		"MODULE_BUILD":  120000,
		"MODULE_PG":     130000,
		"MODULE_PB":     140000,
		"MODULE_PT":     150000,
		"MODULE_PITR":   160000,
		"MODULE_PE":     170000,
		"MODULE_STY":    200000,
		"MODULE_DO":     210000,
		"MODULE_CONFIG": 900000,
		"MODULE_SYSTEM": 990000,
	}

	for name, expected := range expectedModules {
		if modules[name] != expected {
			t.Errorf("%s = %d, want %d", name, modules[name], expected)
		}
	}
}

func TestCategoryConstants(t *testing.T) {
	// Verify category codes follow the 222 structure (CC part)
	categories := map[string]int{
		"CAT_SUCCESS":   CAT_SUCCESS,
		"CAT_PARAM":     CAT_PARAM,
		"CAT_PERM":      CAT_PERM,
		"CAT_DEPEND":    CAT_DEPEND,
		"CAT_NETWORK":   CAT_NETWORK,
		"CAT_RESOURCE":  CAT_RESOURCE,
		"CAT_STATE":     CAT_STATE,
		"CAT_CONFIG":    CAT_CONFIG,
		"CAT_OPERATION": CAT_OPERATION,
		"CAT_INTERNAL":  CAT_INTERNAL,
	}

	expectedCategories := map[string]int{
		"CAT_SUCCESS":   0,
		"CAT_PARAM":     100,
		"CAT_PERM":      200,
		"CAT_DEPEND":    300,
		"CAT_NETWORK":   400,
		"CAT_RESOURCE":  500,
		"CAT_STATE":     600,
		"CAT_CONFIG":    700,
		"CAT_OPERATION": 800,
		"CAT_INTERNAL":  900,
	}

	for name, expected := range expectedCategories {
		if categories[name] != expected {
			t.Errorf("%s = %d, want %d", name, categories[name], expected)
		}
	}
}

func TestExitCode(t *testing.T) {
	tests := []struct {
		name     string
		code     int
		expected int
	}{
		// Success cases
		{"zero code", 0, 0},
		{"success category", MODULE_EXT + CAT_SUCCESS, 0},
		{"success with specific", MODULE_REPO + CAT_SUCCESS + 1, 0},

		// Parameter errors (CC=01) → Exit 2
		{"param error ext", MODULE_EXT + CAT_PARAM, 2},
		{"param error repo", MODULE_REPO + CAT_PARAM + 5, 2},
		{"param error pg", MODULE_PG + CAT_PARAM + 99, 2},

		// Permission errors (CC=02) → Exit 3
		{"perm error ext", MODULE_EXT + CAT_PERM, 3},
		{"perm error pb", MODULE_PB + CAT_PERM + 10, 3},

		// Dependency errors (CC=03) → Exit 4
		{"depend error build", MODULE_BUILD + CAT_DEPEND, 4},
		{"depend error pt", MODULE_PT + CAT_DEPEND + 1, 4},

		// Network errors (CC=04) → Exit 5
		{"network error repo", MODULE_REPO + CAT_NETWORK, 5},
		{"network error sty", MODULE_STY + CAT_NETWORK + 3, 5},

		// Resource errors (CC=05) → Exit 6
		{"resource error pg", MODULE_PG + CAT_RESOURCE, 6},
		{"resource error pitr", MODULE_PITR + CAT_RESOURCE + 2, 6},

		// State errors (CC=06) → Exit 9
		{"state error pt", MODULE_PT + CAT_STATE, 9},
		{"state error pe", MODULE_PE + CAT_STATE + 1, 9},

		// Config errors (CC=07) → Exit 8
		{"config error config", MODULE_CONFIG + CAT_CONFIG, 8},
		{"config error do", MODULE_DO + CAT_CONFIG + 5, 8},

		// Operation errors (CC=08) → Exit 1
		{"operation error ext", MODULE_EXT + CAT_OPERATION, 1},
		{"operation error system", MODULE_SYSTEM + CAT_OPERATION + 99, 1},

		// Internal errors (CC=09) → Exit 1
		{"internal error build", MODULE_BUILD + CAT_INTERNAL, 1},
		{"internal error system", MODULE_SYSTEM + CAT_INTERNAL + 1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExitCode(tt.code); got != tt.expected {
				t.Errorf("ExitCode(%d) = %v, want %v", tt.code, got, tt.expected)
			}
		})
	}
}

func TestExitCodeEdgeCases(t *testing.T) {
	// Test edge cases
	tests := []struct {
		name     string
		code     int
		expected int
	}{
		{"negative code defaults to 1", -1, 1},
		{"very large code", 9999999, 1},
		{"unknown category defaults to 1", 1099, 1}, // Category 10 doesn't exist
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExitCode(tt.code); got != tt.expected {
				t.Errorf("ExitCode(%d) = %v, want %v", tt.code, got, tt.expected)
			}
		})
	}
}

func TestCodeComposition(t *testing.T) {
	// Test that module + category + specific error code can be composed correctly
	tests := []struct {
		name     string
		module   int
		category int
		specific int
		expected int
	}{
		{"ext param error 1", MODULE_EXT, CAT_PARAM, 1, 100101},
		{"repo perm error 5", MODULE_REPO, CAT_PERM, 5, 110205},
		{"pg state error 0", MODULE_PG, CAT_STATE, 0, 130600},
		{"system internal 99", MODULE_SYSTEM, CAT_INTERNAL, 99, 990999},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			composed := tt.module + tt.category + tt.specific
			if composed != tt.expected {
				t.Errorf("Composed code = %d, want %d", composed, tt.expected)
			}
		})
	}
}

func TestCategoryExtraction(t *testing.T) {
	// Verify that ExitCode correctly extracts the category from various codes
	tests := []struct {
		code             int
		expectedCategory int
		expectedExit     int
	}{
		{100101, 1, 2},  // EXT + PARAM + 01 → category 1 → exit 2
		{110205, 2, 3},  // REPO + PERM + 05 → category 2 → exit 3
		{130600, 6, 9},  // PG + STATE + 00 → category 6 → exit 9
		{990999, 9, 1},  // SYSTEM + INTERNAL + 99 → category 9 → exit 1
		{120301, 3, 4},  // BUILD + DEPEND + 01 → category 3 → exit 4
		{170701, 7, 8},  // PE + CONFIG + 01 → category 7 → exit 8
	}

	for _, tt := range tests {
		exitCode := ExitCode(tt.code)
		if exitCode != tt.expectedExit {
			t.Errorf("ExitCode(%d) = %d, want %d", tt.code, exitCode, tt.expectedExit)
		}
	}
}
