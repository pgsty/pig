package repo

import (
	"slices"
	"strings"
	"testing"

	"pig/internal/config"
)

func withELConfig(t *testing.T, major int, version, versionFull string) {
	t.Helper()
	originalOSType := config.OSType
	originalOSCode := config.OSCode
	originalOSArch := config.OSArch
	originalOSMajor := config.OSMajor
	originalOSVersion := config.OSVersion
	originalOSVersionFull := config.OSVersionFull
	t.Cleanup(func() {
		config.OSType = originalOSType
		config.OSCode = originalOSCode
		config.OSArch = originalOSArch
		config.OSMajor = originalOSMajor
		config.OSVersion = originalOSVersion
		config.OSVersionFull = originalOSVersionFull
	})
	config.OSType = config.DistroEL
	config.OSCode = "el" + version
	config.OSArch = "x86_64"
	config.OSMajor = major
	config.OSVersion = version
	config.OSVersionFull = versionFull
}

func newELManager(major string) *Manager {
	return &Manager{
		OsDistroCode: "el" + major,
		OsType:       config.DistroEL,
		OsArch:       "x86_64",
	}
}

func TestELRepoModuleHotfixesAreOptIn(t *testing.T) {
	withELConfig(t, 9, "9", "9.6")
	m := newELManager("9")
	data := []byte(`
- name: appstream
  description: EL AppStream
  module: node
  releases: [8, 9, 10]
  arch: [x86_64, aarch64]
  baseurl: { default: "https://example.com/appstream" }
- name: custom-hotfix
  description: Custom Hotfix
  module: extra
  releases: [8, 9, 10]
  arch: [x86_64, aarch64]
  baseurl: { default: "https://example.com/hotfix" }
  meta: { module_hotfixes: "1" }
`)

	if err := m.LoadData(data); err != nil {
		t.Fatalf("LoadData returned error: %v", err)
	}
	appstream := m.Map["appstream"]
	if appstream == nil {
		t.Fatal("appstream repository not loaded")
	}
	if _, ok := appstream.Meta["module_hotfixes"]; ok {
		t.Fatalf("appstream unexpectedly enables module_hotfixes: %#v", appstream.Meta)
	}
	if strings.Contains(appstream.Content(), "module_hotfixes=") {
		t.Fatalf("appstream content unexpectedly disables native module filtering:\n%s", appstream.Content())
	}

	hotfix := m.Map["custom-hotfix"]
	if hotfix == nil {
		t.Fatal("custom hotfix repository not loaded")
	}
	if got := hotfix.Meta["module_hotfixes"]; got != "1" {
		t.Fatalf("custom hotfix module_hotfixes = %q, want 1", got)
	}
	if !strings.Contains(hotfix.Content(), "module_hotfixes=1") {
		t.Fatalf("custom hotfix content missing explicit opt-in:\n%s", hotfix.Content())
	}
}

func TestEL7DropsModuleHotfixesFromSharedRepo(t *testing.T) {
	withELConfig(t, 7, "7", "7.9")
	m := newELManager("7")
	data := []byte(`
- name: shared-hotfix
  description: Shared Hotfix
  module: extra
  releases: [7, 8, 9, 10]
  arch: [x86_64, aarch64]
  baseurl: { default: "https://example.com/$releasever/$basearch" }
  meta: { module_hotfixes: "1", priority: "11" }
`)

	if err := m.LoadData(data); err != nil {
		t.Fatalf("LoadData returned error: %v", err)
	}
	repository := m.Map["shared-hotfix"]
	if repository == nil {
		t.Fatal("shared EL repository not loaded on EL7")
	}
	if _, ok := repository.Meta["module_hotfixes"]; ok {
		t.Fatalf("EL7 repository unexpectedly retains module_hotfixes: %#v", repository.Meta)
	}
	if got := repository.Meta["priority"]; got != "11" {
		t.Fatalf("EL7 repository priority = %q, want 11", got)
	}
	if strings.Contains(repository.Content(), "module_hotfixes=") {
		t.Fatalf("EL7 content unexpectedly renders module_hotfixes:\n%s", repository.Content())
	}
}

func TestEmbeddedELCatalogModuleHotfixPolicy(t *testing.T) {
	withELConfig(t, 9, "9", "9.6")
	m := newELManager("9")
	if err := m.LoadData(nil); err != nil {
		t.Fatalf("LoadData returned error: %v", err)
	}

	for _, name := range []string{"baseos", "appstream", "extras", "crb", "epel"} {
		repository := m.Map[name]
		if repository == nil {
			t.Fatalf("system repository %s not loaded", name)
		}
		if _, ok := repository.Meta["module_hotfixes"]; ok {
			t.Errorf("system repository %s unexpectedly enables module_hotfixes", name)
		}
	}

	for _, name := range []string{"pigsty-local", "pigsty-infra", "pigsty-pgsql", "pgdg-common", "pgdg18"} {
		repository := m.Map[name]
		if repository == nil {
			t.Fatalf("hotfix repository %s not loaded", name)
		}
		if got := repository.Meta["module_hotfixes"]; got != "1" {
			t.Errorf("repository %s module_hotfixes = %q, want 1", name, got)
		}
	}
}

func TestEmbeddedEL7CatalogIsAvailableWithoutModuleHotfixes(t *testing.T) {
	withELConfig(t, 7, "7", "7.9")
	m := newELManager("7")
	if err := m.LoadData(nil); err != nil {
		t.Fatalf("LoadData returned error: %v", err)
	}

	if len(m.List) != 19 || len(m.Map) != 19 {
		t.Fatalf("EL7 repository count: list=%d map=%d, want 19", len(m.List), len(m.Map))
	}
	for _, repository := range m.List {
		if !slices.Contains(repository.Releases, 7) {
			t.Fatalf("non-EL7 repository loaded for EL7: %+v", repository)
		}
		if _, ok := repository.Meta["module_hotfixes"]; ok {
			t.Errorf("EL7 repository %s unexpectedly enables module_hotfixes", repository.Name)
		}
	}
	for _, name := range []string{"pigsty-local", "pigsty-infra", "pigsty-pgsql", "base", "epel", "pgdg-common", "pgdg14", "pgdg15", "timescaledb", "grafana"} {
		if m.Map[name] == nil {
			t.Errorf("EL7 repository %s not loaded", name)
		}
	}
	for _, name := range []string{"mysql", "redis", "kubernetes"} {
		if m.Map[name] != nil {
			t.Errorf("removed optional EL7 repository %s unexpectedly loaded", name)
		}
	}
}
