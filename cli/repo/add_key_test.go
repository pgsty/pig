package repo

import (
	"bytes"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"pig/internal/config"
	"pig/internal/output"
)

func setupAddKeyTest(t *testing.T, osType, code, arch string) (*Manager, string) {
	t.Helper()
	origType, origCode, origArch, origDir := config.OSType, config.OSCode, config.OSArch, config.ConfigDir
	origRPMPath, origAPTPath, origPut := pigstyRPMKeyPath, pigstyAPTKeyPath, putPigstyKey
	origStat, origImport := statPigstyKey, importPigstyKeys
	origManager, origRun, origAdd := newAddRepoManager, runAddRepoCommand, addRepoModule
	t.Cleanup(func() {
		config.OSType, config.OSCode, config.OSArch, config.ConfigDir = origType, origCode, origArch, origDir
		pigstyRPMKeyPath, pigstyAPTKeyPath, putPigstyKey = origRPMPath, origAPTPath, origPut
		statPigstyKey, importPigstyKeys = origStat, origImport
		newAddRepoManager, runAddRepoCommand, addRepoModule = origManager, origRun, origAdd
	})
	config.OSType, config.OSCode, config.OSArch, config.ConfigDir = osType, code, arch, t.TempDir()
	pigstyRPMKeyPath = filepath.Join(t.TempDir(), "rpm-gpg", "RPM-GPG-KEY-pigsty")
	pigstyAPTKeyPath = filepath.Join(t.TempDir(), "keyrings", "pigsty.asc")
	importPigstyKeys = func([]string) error {
		t.Fatal("unexpected RPM key import")
		return nil
	}
	manager, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	manager.RepoDir = t.TempDir()
	manager.BackupDir = filepath.Join(manager.RepoDir, "backup")
	manager.UpdateCmd = []string{"cache-update"}
	newAddRepoManager = func(...string) (*Manager, error) { return manager, nil }
	keyPath := pigstyRPMKeyPath
	if osType == config.DistroDEB {
		keyPath = pigstyAPTKeyPath
	}
	return manager, keyPath
}

func assertInstalledPigstyKey(t *testing.T, path string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(content, embeddedPigstyKey) {
		t.Fatal("installed signing key differs from the embedded key")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("key permissions = %o, want 644", info.Mode().Perm())
	}
}

func TestConfigureReposAutomaticallyInstallsPigstyKey(t *testing.T) {
	platforms := []struct{ osType, code string }{
		{config.DistroEL, "el9"},
		{config.DistroDEB, "d12"},
		{config.DistroDEB, "u24"},
	}
	selections := []struct {
		name string
		args []string
	}{
		{"default", nil},
		{"all", []string{"all"}},
		{"pigsty", []string{"pigsty"}},
		{"infra", []string{"infra"}},
		{"pgsql", []string{"pgsql"}},
		{"comma", []string{"node,pigsty"}},
		{"duplicates", []string{"pigsty", "infra", "pigsty"}},
	}
	for _, platform := range platforms {
		for _, arch := range []string{"x86_64", "aarch64"} {
			for _, selection := range selections {
				t.Run(platform.code+"/"+arch+"/"+selection.name, func(t *testing.T) {
					manager, keyPath := setupAddKeyTest(t, platform.osType, platform.code, arch)
					writes := 0
					writeKey := putPigstyKey
					putPigstyKey = func(path string, content []byte, mode os.FileMode) error {
						writes++
						return writeKey(path, content, mode)
					}
					addRepoModule = func(m *Manager, module string) error {
						assertInstalledPigstyKey(t, keyPath)
						return m.AddModule(module)
					}
					updates := 0
					runAddRepoCommand = func([]string) error {
						updates++
						assertInstalledPigstyKey(t, keyPath)
						return nil
					}
					for attempt := 1; attempt <= 2; attempt++ {
						if attempt > 1 {
							fresh, err := NewManager()
							if err != nil {
								t.Fatal(err)
							}
							fresh.RepoDir, fresh.BackupDir, fresh.UpdateCmd = manager.RepoDir, manager.BackupDir, manager.UpdateCmd
							*manager = *fresh
						}
						result := ConfigureRepos(selection.args, AddOptions{Region: "default", Update: true})
						if !result.Success {
							t.Fatalf("attempt %d: %+v", attempt, result)
						}
						if writes != 1 || updates != attempt {
							t.Fatalf("attempt %d: key installs=%d cache updates=%d", attempt, writes, updates)
						}
					}
					for _, name := range []string{"pigsty-infra", "pigsty-pgsql"} {
						meta := manager.Map[name].Meta
						field, expected := "gpgkey", "file://"+keyPath
						if platform.osType == config.DistroDEB {
							field, expected = "signed-by", keyPath
						}
						if (selection.name == "infra" && name == "pigsty-pgsql") || (selection.name == "pgsql" && name == "pigsty-infra") {
							expected = ""
						}
						if meta[field] != expected {
							t.Fatalf("%s %s = %q, want %q", name, field, meta[field], expected)
						}
						if platform.osType == config.DistroEL && meta["gpgcheck"] != "0" {
							t.Fatalf("key installation changed signature policy: %v", meta)
						}
						if platform.osType == config.DistroDEB && meta["trusted"] != "yes" {
							t.Fatalf("key installation changed signature policy: %v", meta)
						}
					}
				})
			}
		}
	}
}

func TestConfigureReposSkipsUnselectedOrUnavailablePigstyKey(t *testing.T) {
	for _, selection := range []string{"node", "pgdg", "missing", "unavailable", "empty"} {
		t.Run(selection, func(t *testing.T) {
			manager, keyPath := setupAddKeyTest(t, config.DistroEL, "el9", "x86_64")
			if selection == "unavailable" {
				manager.Module[selection] = []string{"pigsty-infra"}
				delete(manager.Map, "pigsty-infra")
			}
			if selection == "empty" {
				manager.Module[selection] = nil
			}
			putPigstyKey = func(string, []byte, os.FileMode) error {
				t.Fatal("installed an unselected or unavailable Pigsty key")
				return nil
			}
			result := ConfigureRepos([]string{selection}, AddOptions{Region: "default"})
			if result.Success != (selection != "missing") {
				t.Fatalf("unexpected result: %+v", result)
			}
			if _, err := os.Stat(keyPath); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("unexpected key file: %v", err)
			}
		})
	}
}

func TestConfigureReposInstallsKeyForCatalogDefinedModule(t *testing.T) {
	manager, keyPath := setupAddKeyTest(t, config.DistroDEB, "d12", "aarch64")
	manager.Module["custom"] = []string{"pigsty-pgsql"}
	result := ConfigureRepos([]string{"custom"}, AddOptions{Mirror: true})
	if !result.Success {
		t.Fatalf("custom module: %+v", result)
	}
	assertInstalledPigstyKey(t, keyPath)
	content, err := os.ReadFile(manager.getModulePath("custom"))
	if err != nil || !strings.Contains(string(content), "repo.pigsty.cc") {
		t.Fatalf("China mirror content = %q, err = %v", content, err)
	}
}

func TestConfigureReposExplicitKeyFailurePreservesRepositories(t *testing.T) {
	for _, platform := range []struct{ osType, code, field string }{
		{config.DistroEL, "el9", "gpgkey"},
		{config.DistroDEB, "d12", "signed-by"},
	} {
		for _, remove := range []bool{false, true} {
			name := platform.code + "/add"
			if remove {
				name = platform.code + "/set"
			}
			t.Run(name, func(t *testing.T) {
				manager, keyPath := setupAddKeyTest(t, platform.osType, platform.code, "x86_64")
				manager.Map["pigsty-infra"].Meta[platform.field] = keyPath
				meta := maps.Clone(manager.Map["pigsty-infra"].Meta)
				legacyPath := filepath.Join(manager.RepoDir, "legacy.repo")
				writeRollbackFixture(t, legacyPath, "original repository\n")
				putPigstyKey = func(string, []byte, os.FileMode) error { return os.ErrPermission }
				runAddRepoCommand = func([]string) error {
					t.Fatal("backup or cache update ran after key installation failed")
					return nil
				}
				addRepoModule = func(*Manager, string) error {
					t.Fatal("repository write ran after key installation failed")
					return nil
				}
				result := ConfigureRepos([]string{"pigsty"}, AddOptions{Region: "default", Remove: remove, Update: true})
				if result.Success || result.Code != output.CodeRepoAddFailed || !strings.Contains(result.Message, "install Pigsty signing key") {
					t.Fatalf("unexpected key failure: %+v", result)
				}
				assertRollbackFixture(t, legacyPath, "original repository\n")
				if !maps.Equal(manager.Map["pigsty-infra"].Meta, meta) {
					t.Fatal("explicit signing metadata changed after key installation failure")
				}
				data := result.Data.(*RepoAddData)
				if len(data.AddedRepos) != 0 || data.BackupInfo != nil || data.UpdateResult != nil {
					t.Fatalf("key failure reported repository changes: %+v", data)
				}
			})
		}
	}
}

func TestConfigureReposImplicitKeyFailureFallsBack(t *testing.T) {
	for _, platform := range []struct{ osType, code string }{
		{config.DistroEL, "el9"}, {config.DistroDEB, "d12"},
	} {
		for _, check := range []bool{false, true} {
			for _, remove := range []bool{false, true} {
				name := platform.code
				if check {
					name += "/check"
				}
				if remove {
					name += "/set"
				}
				t.Run(name, func(t *testing.T) {
					manager, _ := setupAddKeyTest(t, platform.osType, platform.code, "x86_64")
					for _, name := range []string{"pigsty-infra", "pigsty-pgsql"} {
						if platform.osType == config.DistroDEB {
							manager.Map[name].Meta["trusted"] = "no"
						} else {
							manager.Map[name].Meta["gpgcheck"] = "1"
							manager.Map[name].Meta["repo_gpgcheck"] = "1"
						}
					}
					thirdParty := "pgdg-common"
					if platform.osType == config.DistroDEB {
						thirdParty = "pgdg"
					}
					manager.Map[thirdParty].Meta["signed-by"] = "/custom/pgdg-key.asc"
					thirdPartyMeta := maps.Clone(manager.Map[thirdParty].Meta)
					putPigstyKey = func(string, []byte, os.FileMode) error { return os.ErrPermission }
					updates := 0
					runAddRepoCommand = func(args []string) error {
						if args[0] == "cache-update" {
							updates++
						}
						return nil
					}
					result := ConfigureRepos([]string{"pigsty", "pgdg"}, AddOptions{Region: "default", Remove: remove, Update: true, GPGCheck: check})
					if !result.Success || updates != 1 {
						t.Fatalf("key failure blocked repository setup: %+v, updates=%d", result, updates)
					}
					data := result.Data.(*RepoAddData)
					if len(data.Warnings) != 1 || !strings.Contains(data.Warnings[0], "signature verification disabled") || len(data.Failed) != 0 {
						t.Fatalf("missing fallback warning or spurious failure: %+v", data)
					}
					content, err := os.ReadFile(manager.getModulePath("pigsty"))
					if err != nil {
						t.Fatal(err)
					}
					if platform.osType == config.DistroDEB {
						if !strings.Contains(string(content), "trusted=yes") || strings.Count(string(content), "signed-by="+pigstyAPTKeyPath) != 2 {
							t.Fatalf("APT fallback = %s", content)
						}
					} else if !strings.Contains(string(content), "gpgcheck=0") || !strings.Contains(string(content), "repo_gpgcheck=0") {
						t.Fatalf("RPM fallback = %s", content)
					}
					if !maps.Equal(manager.Map[thirdParty].Meta, thirdPartyMeta) {
						t.Fatal("Pigsty fallback changed third-party signing metadata")
					}
				})
			}
		}
	}
}

func TestConfigureReposUnselectedExplicitKeyDoesNotBlockFallback(t *testing.T) {
	manager, _ := setupAddKeyTest(t, config.DistroEL, "el9", "x86_64")
	manager.Map["pigsty-infra"].Meta["gpgkey"] = "file:///custom/infra-key"
	putPigstyKey = func(string, []byte, os.FileMode) error { return os.ErrPermission }
	result := ConfigureRepos([]string{"pgsql"}, AddOptions{Region: "default"})
	if !result.Success || len(result.Data.(*RepoAddData).Warnings) != 1 {
		t.Fatalf("unselected key made fallback fatal: %+v", result)
	}
	if manager.Map["pigsty-infra"].Meta["gpgkey"] != "file:///custom/infra-key" {
		t.Fatal("unselected Pigsty metadata changed")
	}
}

func TestConfigureReposPreservesExplicitKey(t *testing.T) {
	for _, platform := range []struct{ osType, code, field string }{
		{config.DistroEL, "el9", "gpgkey"}, {config.DistroDEB, "d12", "signed-by"},
	} {
		t.Run(platform.code, func(t *testing.T) {
			for _, check := range []bool{false, true} {
				manager, _ := setupAddKeyTest(t, platform.osType, platform.code, "x86_64")
				customKey := filepath.Join(t.TempDir(), "custom-key.asc")
				writeRollbackFixture(t, customKey, string(embeddedPigstyKey))
				manager.Map["pigsty-infra"].Meta[platform.field] = customKey
				putPigstyKey = func(string, []byte, os.FileMode) error {
					t.Fatal("custom key preparation tried to write an unrelated default key")
					return nil
				}
				importPigstyKeys = func(args []string) error {
					if strings.Join(args, " ") != "rpm --import -- "+customKey {
						t.Fatalf("imported unrelated key: %v", args)
					}
					return nil
				}
				result := ConfigureRepos([]string{"infra"}, AddOptions{Region: "default", GPGCheck: check})
				if !result.Success || manager.Map["pigsty-infra"].Meta[platform.field] != customKey {
					t.Fatalf("explicit key was replaced (check=%v): %+v", check, result)
				}
			}
		})
	}
}

func TestConfigureReposRejectsReplacementBeforeKeyInstallation(t *testing.T) {
	_, keyPath := setupAddKeyTest(t, config.DistroEL, "el9", "x86_64")
	putPigstyKey = func(string, []byte, os.FileMode) error {
		t.Fatal("invalid replacement installed a key")
		return nil
	}
	result := ConfigureRepos([]string{"pigsty", "missing"}, AddOptions{Region: "default", Remove: true})
	if result.Success || result.Code != output.CodeRepoModuleNotFound {
		t.Fatalf("invalid replacement: %+v", result)
	}
	if _, err := os.Stat(keyPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected key file: %v", err)
	}
}

func TestConfigureReposMissingExplicitCustomKeyFailsBeforeMutation(t *testing.T) {
	for _, platform := range []struct{ osType, code, field string }{
		{config.DistroEL, "el9", "gpgkey"}, {config.DistroDEB, "d12", "signed-by"},
	} {
		for _, remove := range []bool{false, true} {
			t.Run(platform.code+"/remove="+strconv.FormatBool(remove), func(t *testing.T) {
				manager, defaultKey := setupAddKeyTest(t, platform.osType, platform.code, "aarch64")
				customKey := filepath.Join(t.TempDir(), "missing.asc")
				manager.Map["pigsty-infra"].Meta[platform.field] = customKey
				legacy := filepath.Join(manager.RepoDir, "legacy.repo")
				writeRollbackFixture(t, legacy, "preserve me\n")
				importPigstyKeys = func([]string) error { return os.ErrNotExist }
				runAddRepoCommand = func([]string) error {
					t.Fatal("explicit key failure reached backup/cache update")
					return nil
				}
				result := ConfigureRepos([]string{"infra"}, AddOptions{Remove: remove, Update: true})
				if result.Success || !strings.Contains(result.Message, "explicitly configures a signing key") {
					t.Fatalf("missing custom key was accepted: %+v", result)
				}
				assertRollbackFixture(t, legacy, "preserve me\n")
				data := result.Data.(*RepoAddData)
				if len(data.AddedRepos) != 0 || data.BackupInfo != nil || data.UpdateResult != nil || len(data.Warnings) != 0 {
					t.Fatalf("explicit failure changed repositories or fell back: %+v", data)
				}
				if _, err := os.Stat(defaultKey); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("custom key installed an unrelated default: %v", err)
				}
			})
		}
	}
}

func TestConfigureReposFallbackPreservesExistingAPTReference(t *testing.T) {
	manager, keyPath := setupAddKeyTest(t, config.DistroDEB, "d12", "aarch64")
	result := ConfigureRepos([]string{"infra"}, AddOptions{})
	if !result.Success {
		t.Fatal(result)
	}
	before, err := os.ReadFile(manager.getModulePath("infra"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(keyPath); err != nil {
		t.Fatal(err)
	}
	fresh, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	fresh.RepoDir = manager.RepoDir
	newAddRepoManager = func(...string) (*Manager, error) { return fresh, nil }
	putPigstyKey = func(string, []byte, os.FileMode) error { return os.ErrPermission }
	result = ConfigureRepos([]string{"pigsty"}, AddOptions{})
	if !result.Success || len(result.Data.(*RepoAddData).Warnings) != 1 {
		t.Fatalf("fallback failed: %+v", result)
	}
	assertRollbackFixture(t, manager.getModulePath("infra"), string(before))
	after, err := os.ReadFile(fresh.getModulePath("pigsty"))
	if err != nil {
		t.Fatal(err)
	}
	option := "[signed-by=" + keyPath + " trusted=yes]"
	if !strings.Contains(string(before), option) || strings.Count(string(after), option) != 2 {
		t.Fatalf("APT options differ between retained and new definitions:\n%s\n%s", before, after)
	}
}

func TestConfigureReposFallbackDoesNotDowngradeExplicitCustomKey(t *testing.T) {
	manager, _ := setupAddKeyTest(t, config.DistroDEB, "d12", "aarch64")
	customKey := filepath.Join(t.TempDir(), "custom.asc")
	writeRollbackFixture(t, customKey, string(embeddedPigstyKey))
	manager.Map["pigsty-infra"].Meta["signed-by"] = customKey
	manager.Map["pigsty-infra"].Meta["trusted"] = "no"
	before := maps.Clone(manager.Map["pigsty-infra"].Meta)
	putPigstyKey = func(string, []byte, os.FileMode) error { return os.ErrPermission }
	result := ConfigureRepos([]string{"pigsty"}, AddOptions{})
	if !result.Success || len(result.Data.(*RepoAddData).Warnings) != 1 {
		t.Fatalf("unrelated default key failure blocked custom key: %+v", result)
	}
	if !maps.Equal(before, manager.Map["pigsty-infra"].Meta) || manager.Map["pigsty-pgsql"].Meta["trusted"] != "yes" {
		t.Fatal("fallback crossed the explicit-key boundary")
	}
}
