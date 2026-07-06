package ext

import "testing"

func TestResolvePGHomePolarDBAliases(t *testing.T) {
	for _, alias := range []string{"polar", "polardb", "polardb-17"} {
		t.Run(alias, func(t *testing.T) {
			got, err := resolvePGHome(alias)
			if err != nil {
				t.Fatalf("resolvePGHome(%q) returned error: %v", alias, err)
			}
			if got != "/usr/polar-17" {
				t.Fatalf("resolvePGHome(%q) = %q, want %q", alias, got, "/usr/polar-17")
			}
		})
	}
}
