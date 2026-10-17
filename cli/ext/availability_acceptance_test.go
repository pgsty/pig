package ext

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
	"pig/internal/config"
)

func TestAvailabilityMatchesAllSummaryFormats(t *testing.T) {
	oldCatalog, oldPG := Catalog, Postgres
	oldOS, oldCode, oldArch := config.OSType, config.OSCode, config.OSArch
	t.Cleanup(func() {
		Catalog, Postgres = oldCatalog, oldPG
		config.OSType, config.OSCode, config.OSArch = oldOS, oldCode, oldArch
	})
	Postgres = nil
	for _, tc := range []struct {
		name, os, code, arch, matrix string
		versions                     []string
		pg                           int
		want                         bool
	}{
		{"matrix-overrides-metadata", config.DistroEL, "el9", "x86_64", "el9i:17:A:f:1:P:1.0", []string{}, 17, true},
		{"matrix-missing", config.DistroEL, "el9", "amd64", "el9i:17:M:f", []string{"17"}, 17, false},
		{"arm-alias", config.DistroDEB, "d12", "aarch64", "d12a:17:A:f:1:P:1.0", []string{}, 17, true},
		{"wrong-arch", config.DistroDEB, "d12", "amd64", "d12a:17:A:f:1:P:1.0", []string{"17"}, 17, false},
		{"wrong-pg", config.DistroDEB, "d12", "arm64", "d12a:17:A:f:1:P:1.0", []string{"18"}, 18, false},
		{"metadata-fallback", config.DistroDEB, "u24", "aarch64", "", []string{"17"}, 17, true},
		{"metadata-rejects", config.DistroEL, "el9", "x86_64", "", []string{"17"}, 18, false},
		{"badcase-alias", config.DistroEL, "el9", "aarch64", "", []string{"14"}, 14, false},
		{"mac-theoretical", config.DistroMAC, "a23", "arm64", "el9i:17:M:f", []string{"17"}, 17, true},
		{"mac-unsupported-pg", config.DistroMAC, "a23", "arm64", "", []string{"17"}, 18, false},
		{"mac-no-evidence", config.DistroMAC, "a23", "arm64", "", []string{}, 17, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config.OSType, config.OSCode, config.OSArch = tc.os, tc.code, tc.arch
			e := &Extension{Name: "pllua", Pkg: "pllua", Lead: true, PgVer: tc.versions, RpmPg: tc.versions, DebPg: tc.versions, RpmPkg: "pllua_$v", DebPkg: "postgresql-$v-pllua"}
			if tc.matrix != "" {
				e.Extra = map[string]interface{}{"matrix": []interface{}{tc.matrix}}
			}
			Catalog = &ExtensionCatalog{Extensions: []*Extension{e}, ExtNameMap: map[string]*Extension{e.Name: e}, ExtPkgMap: map[string]*Extension{e.Pkg: e}}
			wantStatus := "not_avail"
			if tc.want {
				wantStatus = "available"
			}
			if e.Available(tc.pg) != tc.want {
				t.Errorf("Available=%v, want %v", e.Available(tc.pg), tc.want)
			}
			summary := e.ToSummary(tc.pg)
			if summary.Status != wantStatus {
				t.Errorf("summary status=%s, want %s", summary.Status, wantStatus)
			}
			for _, marshal := range []func(any) ([]byte, error){json.Marshal, yaml.Marshal} {
				encoded, err := marshal(summary)
				if err != nil || !strings.Contains(string(encoded), wantStatus) {
					t.Fatalf("serialized summary=%s err=%v", encoded, err)
				}
			}
			text := (&ExtensionListData{PgVersion: tc.pg, Extensions: []*ExtensionSummary{summary}}).Text()
			if !strings.Contains(text, wantStatus) {
				t.Fatalf("text disagrees: %s", text)
			}
			if tc.os == config.DistroMAC && summary.PackageName != "" {
				t.Fatalf("theoretical support claimed a package: %s", summary.PackageName)
			}
		})
	}
}

func TestAvailabilityGlobalTextRecognizesArchitectureAliases(t *testing.T) {
	oldCatalog := Catalog
	t.Cleanup(func() { Catalog = oldCatalog })
	e := &Extension{Name: "fixture", Pkg: "fixture", Lead: true, Extra: map[string]interface{}{
		"matrix": []interface{}{"d12i:17:A:f:1:P:7.6.543", "d12a:17:A:f:1:P:7.6.543"},
	}}
	Catalog = &ExtensionCatalog{Extensions: []*Extension{e}}
	for _, tc := range []struct{ arch, label string }{
		{"amd64", "d12.x86_64"}, {"x86_64", "d12.x86_64"},
		{"arm64", "d12.aarch64"}, {"aarch64", "d12.aarch64"},
	} {
		t.Run(tc.arch, func(t *testing.T) {
			result := getGlobalAvailability("d12", tc.arch)
			data := result.Data.(*ExtensionAvailData)
			if !result.Success || data.PackageCount != 1 || data.Packages[0].Versions["17"] != "7.6.543" {
				t.Fatalf("structured availability missing package: %+v", data)
			}
			if text := data.Text(); !strings.Contains(text, "7.6.543") || !strings.Contains(text, tc.label) {
				t.Fatalf("text disagrees with structured availability: %s", text)
			}
			if data.Arch != tc.arch {
				t.Fatalf("text rendering changed structured arch: %s", data.Arch)
			}
		})
	}
}
