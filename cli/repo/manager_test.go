package repo

import (
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

func TestEmbeddedEL7CatalogDoesNotUseModuleHotfixes(t *testing.T) {
	withELConfig(t, 7, "7", "7.9")
	m := newELManager("7")
	if err := m.LoadData(nil); err != nil {
		t.Fatalf("LoadData returned error: %v", err)
	}

	for _, name := range []string{"pigsty-local", "pigsty-infra", "pigsty-pgsql", "pgdg-common", "pgdg14", "pgdg15", "pgdg-extras"} {
		repository := m.Map[name]
		if repository == nil {
			t.Fatalf("EL7 repository %s not loaded", name)
		}
		if _, ok := repository.Meta["module_hotfixes"]; ok {
			t.Errorf("EL7 repository %s unexpectedly enables module_hotfixes", name)
		}
	}
}
