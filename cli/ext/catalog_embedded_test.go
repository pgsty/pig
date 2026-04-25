package ext

import (
	"strings"
	"testing"
)

func loadEmbeddedCatalogForTest(t *testing.T) *ExtensionCatalog {
	t.Helper()

	ec := &ExtensionCatalog{}
	if err := ec.Load(nil); err != nil {
		t.Fatalf("failed to load embedded catalog: %v", err)
	}
	return ec
}

func countAvailableEntriesByOS(ec *ExtensionCatalog) map[string]int {
	counts := make(map[string]int)
	if ec == nil {
		return counts
	}
	for _, ext := range ec.Extensions {
		if ext == nil {
			continue
		}
		for _, entry := range ext.GetPkgMatrix() {
			if entry != nil && entry.State == PkgAvail {
				counts[entry.OS]++
			}
		}
	}
	return counts
}

func TestEmbeddedCatalogRetainsSupportedAvailabilityCoverage(t *testing.T) {
	ec := loadEmbeddedCatalogForTest(t)
	counts := countAvailableEntriesByOS(ec)

	for _, osCode := range []string{"d12", "el9", "u22", "u24", "u26"} {
		if counts[osCode] == 0 {
			t.Fatalf("expected embedded catalog to retain non-zero available entries for %s, got counts=%v", osCode, counts)
		}
	}
}

func TestEmbeddedCatalogAvailabilityMatrixRendersU26Rows(t *testing.T) {
	ec := loadEmbeddedCatalogForTest(t)
	ext := ec.ExtNameMap["postgis"]
	if ext == nil {
		t.Fatal("expected embedded catalog to include postgis")
	}

	origCatalog := Catalog
	Catalog = ec
	defer func() { Catalog = origCatalog }()

	text := buildExtensionAvailData(ext).Text()
	if !strings.Contains(text, "u26.x86_64") {
		t.Fatalf("expected availability matrix to render u26.x86_64 row, got:\n%s", text)
	}
	if !strings.Contains(text, "u26.aarch64") {
		t.Fatalf("expected availability matrix to render u26.aarch64 row, got:\n%s", text)
	}
}
