package repo

import (
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"pig/internal/config"
	"pig/internal/output"
)

func setupAPTTrustTest(t *testing.T, code string) *Manager {
	t.Helper()
	t.Setenv("PIG_NO_SUDO", "1")
	m, _ := setupAddKeyTest(t, config.DistroDEB, code, "aarch64")
	oldCodename := config.OSVersionCode
	t.Cleanup(func() { config.OSVersionCode = oldCodename })
	config.OSVersionCode = "bookworm"
	if code == "u24" {
		config.OSVersionCode = "noble"
	}
	m.RepoDir = filepath.Join(t.TempDir(), "sources.list.d")
	m.BackupDir = filepath.Join(m.RepoDir, "backup")
	if err := os.Mkdir(m.RepoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return m
}

func freshAPTManager(t *testing.T, previous *Manager) *Manager {
	t.Helper()
	m, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	m.RepoDir, m.BackupDir, m.UpdateCmd = previous.RepoDir, previous.BackupDir, previous.UpdateCmd
	newAddRepoManager = func(...string) (*Manager, error) { return m, nil }
	return m
}

func TestAPTTrustBootThenAddIsIdempotent(t *testing.T) {
	for _, code := range []string{"d12", "u24"} {
		t.Run(code, func(t *testing.T) {
			m := setupAPTTrustTest(t, code)
			boot := ConfigureRepos([]string{"all"}, AddOptions{Region: "default", GPGCheck: true})
			if !boot.Success {
				t.Fatal(boot)
			}
			before := make(map[string]string)
			for _, module := range []string{"infra", "pgsql", "node"} {
				content, err := os.ReadFile(m.getModulePath(module))
				if err != nil {
					t.Fatal(err)
				}
				before[module] = string(content)
			}
			var generated string
			for i := 0; i < 2; i++ {
				m = freshAPTManager(t, m)
				result := ConfigureRepos([]string{"pigsty"}, AddOptions{Region: "default"})
				if !result.Success {
					t.Fatal(result)
				}
				for module, content := range before {
					assertRollbackFixture(t, m.getModulePath(module), content)
				}
				content, err := os.ReadFile(m.getModulePath("pigsty"))
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(content), "trusted=") || strings.Count(string(content), "signed-by="+pigstyAPTKeyPath) != 2 {
					t.Fatalf("boot trust policy changed: %s", content)
				}
				if i > 0 && string(content) != generated {
					t.Fatal("repeated add changed generated definitions")
				}
				generated = string(content)
			}
		})
	}
}

func TestAPTTrustInheritsRetainedListAndDeb822(t *testing.T) {
	for _, tc := range []struct{ name, file, content string }{
		{"list", "custom.list", "# retained\ndeb-src [arch=amd64 signed-by=/custom/key.asc trusted=no] https://repo.pigsty.io/apt/infra generic other\n"},
		{"deb822", "custom.sources", "# retained\nTypes: deb deb-src\nURIs: https://unrelated.example/apt https://repo.pigsty.io/apt/infra/\nSuites: generic another\nComponents: other\nArchitectures: amd64\nTrusted: no\nSigned-By: /custom/key.asc\n\n"},
		{"main list", "../sources.list", "deb [signed-by=/custom/key.asc trusted=no] https://repo.pigsty.io/apt/infra/ generic main\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := setupAPTTrustTest(t, "d12")
			path := filepath.Join(m.RepoDir, tc.file)
			writeRollbackFixture(t, path, tc.content)
			unrelated := "deb [trusted=yes] https://repo.pigsty.io/apt/infra different main\n"
			writeRollbackFixture(t, filepath.Join(m.RepoDir, "unrelated.list"), unrelated)
			result := ConfigureRepos([]string{"infra"}, AddOptions{Region: "default"})
			if !result.Success {
				t.Fatal(result)
			}
			if r := m.Map["pigsty-infra"]; r.Meta["trusted"] != "no" || r.Meta["signed-by"] != "/custom/key.asc" {
				t.Fatalf("retained policy not inherited: %v", r.Meta)
			}
			assertRollbackFixture(t, path, tc.content)
			assertRollbackFixture(t, filepath.Join(m.RepoDir, "unrelated.list"), unrelated)
		})
	}
}

func TestAPTTrustExplicitConflictFailsBeforeRepositoryChanges(t *testing.T) {
	for _, check := range []bool{false, true} {
		t.Run(fmt.Sprintf("signature-request=%v", check), func(t *testing.T) {
			m := setupAPTTrustTest(t, "d12")
			trusted := "yes"
			if !check {
				// Explicit trusted=yes must remain distinguishable from the manager default.
				data := []byte(`- {name: pigsty-infra, module: infra, releases: [12], arch: [aarch64], baseurl: {default: 'https://repo.pigsty.io/apt/infra/ generic main'}, meta: {trusted: 'yes'}}`)
				if err := m.LoadData(data); err != nil {
					t.Fatal(err)
				}
				trusted = "no"
			}
			old := fmt.Sprintf("deb [signed-by=%s trusted=%s] https://repo.pigsty.io/apt/infra/ generic main\n", pigstyAPTKeyPath, trusted)
			path := filepath.Join(m.RepoDir, "custom.list")
			writeRollbackFixture(t, path, old)
			runAddRepoCommand = func([]string) error { t.Fatal("trust conflict reached cache update or backup"); return nil }
			result := ConfigureRepos([]string{"infra"}, AddOptions{Region: "default", GPGCheck: check, Update: true})
			if result.Success || result.Code != output.CodeRepoAddFailed || !strings.Contains(result.Message, "pig repo set") {
				t.Fatalf("explicit conflict was not rejected: %+v", result)
			}
			data := result.Data.(*RepoAddData)
			if len(data.AddedRepos) != 0 || data.BackupInfo != nil || data.UpdateResult != nil {
				t.Fatalf("preflight changed repositories: %+v", data)
			}
			assertRollbackFixture(t, path, old)
			if _, err := os.Stat(m.getModulePath("infra")); !os.IsNotExist(err) {
				t.Fatalf("new source written on conflict: %v", err)
			}
		})
	}
}

func TestAPTTrustKeyFailurePreservesRetainedSignaturePolicy(t *testing.T) {
	m := setupAPTTrustTest(t, "d12")
	old := fmt.Sprintf("deb [signed-by=%s] https://repo.pigsty.io/apt/infra/ generic main\n", pigstyAPTKeyPath)
	path := filepath.Join(m.RepoDir, "custom.list")
	writeRollbackFixture(t, path, old)
	putPigstyKey = func(string, []byte, os.FileMode) error { return os.ErrPermission }
	result := ConfigureRepos([]string{"infra"}, AddOptions{Region: "default"})
	if !result.Success {
		t.Fatal(result)
	}
	if _, exists := m.Map["pigsty-infra"].Meta["trusted"]; exists {
		t.Fatal("key failure downgraded retained verification")
	}
	data := result.Data.(*RepoAddData)
	if len(data.Warnings) != 1 || !strings.Contains(data.Warnings[0], "preserving retained APT trust settings") {
		t.Fatalf("inaccurate fallback warning: %v", data.Warnings)
	}
	assertRollbackFixture(t, path, old)
}

func TestAPTTrustOverwritingOnlyDefinitionPreservesDefaultsButAllowsExplicitChange(t *testing.T) {
	m := setupAPTTrustTest(t, "d12")
	result := ConfigureRepos([]string{"infra"}, AddOptions{Region: "default", GPGCheck: true})
	if !result.Success {
		t.Fatal(result)
	}
	path := m.getModulePath("infra")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m = freshAPTManager(t, m)
	result = ConfigureRepos([]string{"infra"}, AddOptions{Region: "default"})
	if !result.Success {
		t.Fatal(result)
	}
	assertRollbackFixture(t, path, string(before))

	m = freshAPTManager(t, m)
	data := []byte(`- {name: pigsty-infra, module: infra, releases: [12], arch: [aarch64], baseurl: {default: 'https://repo.pigsty.io/apt/infra/ generic main'}, meta: {trusted: 'yes'}}`)
	if err := m.LoadData(data); err != nil {
		t.Fatal(err)
	}
	result = ConfigureRepos([]string{"infra"}, AddOptions{Region: "default"})
	if !result.Success {
		t.Fatal(result)
	}
	after, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(after), "trusted=yes") {
		t.Fatalf("explicit own-file policy not applied: %v\n%s", err, after)
	}
}

func TestAPTTrustReplacingConflictingTargetAdoptsRetainedPolicy(t *testing.T) {
	m := setupAPTTrustTest(t, "d12")
	old := "deb [trusted=no] https://repo.pigsty.io/apt/infra/ generic main\n"
	writeRollbackFixture(t, filepath.Join(m.RepoDir, "custom.list"), old)
	writeRollbackFixture(t, m.getModulePath("infra"), strings.Replace(old, "trusted=no", "trusted=yes", 1))
	result := ConfigureRepos([]string{"infra"}, AddOptions{Region: "default"})
	if !result.Success || m.Map["pigsty-infra"].Meta["trusted"] != "no" {
		t.Fatalf("target conflict not repaired: %+v", result)
	}
	assertRollbackFixture(t, filepath.Join(m.RepoDir, "custom.list"), old)
}

func TestAPTTrustExplicitSignedByConflictDoesNotRewriteCustomSource(t *testing.T) {
	m := setupAPTTrustTest(t, "d12")
	key := filepath.Join(t.TempDir(), "explicit.asc")
	writeRollbackFixture(t, key, string(embeddedPigstyKey))
	m.Map["pigsty-infra"].Meta["signed-by"] = key
	path := filepath.Join(m.RepoDir, "custom.list")
	old := "deb [signed-by=/custom/retained.asc trusted=yes] https://repo.pigsty.io/apt/infra/ generic main\n"
	writeRollbackFixture(t, path, old)
	result := ConfigureRepos([]string{"infra"}, AddOptions{Region: "default"})
	if result.Success || !strings.Contains(result.Message, "conflicting APT signed-by") {
		t.Fatalf("explicit key conflict accepted: %+v", result)
	}
	assertRollbackFixture(t, path, old)
}

func TestAPTTrustUnknownModuleDoesNotExcludeRetainedFile(t *testing.T) {
	m := setupAPTTrustTest(t, "d12")
	data := []byte(`- {name: pigsty-infra, module: infra, releases: [12], arch: [aarch64], baseurl: {default: 'https://repo.pigsty.io/apt/infra/ generic main'}, meta: {trusted: 'yes'}}`)
	if err := m.LoadData(data); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(m.RepoDir, "unknown.list")
	old := "deb [trusted=no] https://repo.pigsty.io/apt/infra/ generic main\n"
	writeRollbackFixture(t, path, old)
	result := ConfigureRepos([]string{"infra", "unknown"}, AddOptions{Region: "default"})
	if result.Success || !strings.Contains(result.Message, "conflicting APT trusted") {
		t.Fatalf("unknown module hid retained conflict: %+v", result)
	}
	assertRollbackFixture(t, path, old)
	if len(result.Data.(*RepoAddData).AddedRepos) != 0 {
		t.Fatal("wrote conflicting source alongside an unknown module")
	}
}

func TestAPTTrustRejectsMatchingInlineKeyWithoutChangingSources(t *testing.T) {
	m := setupAPTTrustTest(t, "d12")
	path := filepath.Join(m.RepoDir, "custom.sources")
	old := "Types: deb\nURIs: https://repo.pigsty.io/apt/infra/\nSuites: generic\nComponents: main\nSigned-By:\n -----BEGIN PGP PUBLIC KEY BLOCK-----\n .\n inline-key-data\n -----END PGP PUBLIC KEY BLOCK-----\n"
	writeRollbackFixture(t, path, old)
	result := ConfigureRepos([]string{"infra"}, AddOptions{Region: "default"})
	if result.Success || !strings.Contains(result.Message, "inline APT signing key") || strings.Contains(result.Message, "inline-key-data") {
		t.Fatalf("inline key silently changed: %+v", result)
	}
	assertRollbackFixture(t, path, old)
}

func TestAPTTrustRejectsInconsistentRetainedSources(t *testing.T) {
	m := setupAPTTrustTest(t, "d12")
	for _, trust := range []string{"yes", "no"} {
		writeRollbackFixture(t, filepath.Join(m.RepoDir, trust+".list"), "deb [trusted="+trust+"] https://user:secret@repo.example/apt stable main\n")
	}
	m.Map["pigsty-infra"].BaseURL["default"] = "https://user:secret@repo.example/apt stable main"
	result := ConfigureRepos([]string{"infra"}, AddOptions{Region: "default"})
	if result.Success || !strings.Contains(result.Message, "conflicting APT trust") || strings.Contains(result.Message, "secret") {
		t.Fatalf("conflict result leaked source or did not reject: %+v", result)
	}
}

func TestAPTTrustDisabledSourcesAndOtherSuitesDoNotOverrideDefaults(t *testing.T) {
	m := setupAPTTrustTest(t, "d12")
	fixtures := map[string]string{
		"disabled.sources": "Enabled: no\nTypes: deb\nURIs: https://repo.pigsty.io/apt/infra/\nSuites: generic\nComponents: main\nTrusted: no\n",
		"old.list.save":    "deb [trusted=no] https://repo.pigsty.io/apt/infra/ generic main\n",
		"different.list":   "deb [trusted=no] https://repo.pigsty.io/apt/infra/ other main\n",
	}
	for file, content := range fixtures {
		writeRollbackFixture(t, filepath.Join(m.RepoDir, file), content)
	}
	result := ConfigureRepos([]string{"infra"}, AddOptions{Region: "default"})
	if !result.Success || m.Map["pigsty-infra"].Meta["trusted"] != "yes" {
		t.Fatalf("fresh-source defaults changed: %+v", result)
	}
	for file, content := range fixtures {
		assertRollbackFixture(t, filepath.Join(m.RepoDir, file), content)
	}
}

func TestAPTTrustReplacementUsesRequestedPolicy(t *testing.T) {
	m := setupAPTTrustTest(t, "d12")
	old := fmt.Sprintf("deb [signed-by=%s] https://repo.pigsty.io/apt/infra/ generic main\n", pigstyAPTKeyPath)
	writeRollbackFixture(t, filepath.Join(m.RepoDir, "custom.list"), old)
	runAddRepoCommand = func(args []string) error {
		// The production backup also handles /etc/apt/sources.list. This unit
		// fixture must never move or touch the host's root source file.
		for _, arg := range args[1:] {
			if arg == "/etc/apt/sources.list" {
				return nil
			}
		}
		return runRepoTestCommand(args)
	}
	result := ConfigureRepos([]string{"pigsty"}, AddOptions{Region: "default", Remove: true})
	if !result.Success || m.Map["pigsty-infra"].Meta["trusted"] != "yes" {
		t.Fatalf("replacement policy = %+v", result)
	}
	assertRollbackFixture(t, filepath.Join(m.BackupDir, "custom.list"), old)
}

func TestAPTTrustSameSourceSelectedThroughDifferentRepositories(t *testing.T) {
	m := setupAPTTrustTest(t, "d12")
	r := m.Map["pigsty-infra"]
	copy := *r
	copy.Name = "custom-infra"
	copy.Meta = maps.Clone(r.Meta)
	copy.Meta["trusted"] = "no"
	m.Map[copy.Name] = &copy
	m.Module["custom"] = []string{copy.Name}
	result := ConfigureRepos([]string{"infra", "custom"}, AddOptions{Region: "default"})
	if !result.Success || r.Meta["trusted"] != "no" {
		t.Fatalf("selected duplicates not reconciled: %+v", result)
	}
}

// Set PIG_APT_TEST_IMAGE to an existing Debian/Ubuntu Docker image on hosts
// without apt-get. The container has no network, a read-only root and fixture
// mount, and a private /tmp; it runs only the real APT source parser.
func runAPTParser(t *testing.T, root string) ([]byte, error) {
	t.Helper()
	fixtureRoot := root
	var prefix []string
	command := "apt-get"
	if image := os.Getenv("PIG_APT_TEST_IMAGE"); image != "" {
		command = "docker"
		fixtureRoot = "/fixtures"
		prefix = []string{"run", "--rm", "--network", "none", "--read-only", "--tmpfs", "/tmp:rw,nosuid,nodev", "--mount", "type=bind,src=" + root + ",dst=/fixtures,readonly", "--entrypoint", "apt-get", image}
	} else if _, err := exec.LookPath(command); err != nil {
		t.Skip("real APT parser unavailable; set PIG_APT_TEST_IMAGE to an existing Docker image")
	}
	args := append(prefix, "indextargets", "-o", "Dir::Etc::sourcelist="+fixtureRoot+"/sources.list", "-o", "Dir::Etc::sourceparts="+fixtureRoot+"/sources.list.d", "-o", "Dir::Etc::main=/dev/null", "-o", "Dir::Etc::parts=-", "-o", "Dir::State="+fixtureRoot+"/state", "-o", "Dir::Cache="+fixtureRoot+"/cache", "-o", "Debug::NoLocking=1")
	return exec.Command(command, args...).CombinedOutput()
}

func TestAPTParserTrustAfterBoot(t *testing.T) {
	for _, code := range []string{"d12", "u24"} {
		t.Run(code, func(t *testing.T) {
			m := setupAPTTrustTest(t, code)
			root := filepath.Dir(m.RepoDir)
			result := ConfigureRepos([]string{"all"}, AddOptions{Region: "default", GPGCheck: true})
			if !result.Success {
				t.Fatal(result)
			}
			m = freshAPTManager(t, m)
			if _, err := m.preparePigstyKey([]string{"pigsty"}, false); err != nil {
				t.Fatal(err)
			}
			// Reproduce the old writer before invoking the corrected workflow.
			writeRollbackFixture(t, m.getModulePath("pigsty"), m.getModuleContent("pigsty"))
			out, err := runAPTParser(t, root)
			if err == nil || !strings.Contains(string(out), "Conflicting values set for option Trusted") {
				t.Fatalf("original conflict not reproduced: %v\n%s", err, out)
			}
			if err := os.Remove(m.getModulePath("pigsty")); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				m = freshAPTManager(t, m)
				result = ConfigureRepos([]string{"pigsty"}, AddOptions{Region: "default"})
				if !result.Success {
					t.Fatal(result)
				}
				out, err = runAPTParser(t, root)
				if err != nil {
					t.Fatalf("APT parser failed after add %d: %v\n%s", i+1, err, out)
				}
			}
		})
	}
}

func TestAPTParserRetainedDeb822AndRootList(t *testing.T) {
	m := setupAPTTrustTest(t, "u24")
	root := filepath.Dir(m.RepoDir)
	key := "/custom/key.asc"
	mainSource := "deb-src [arch=arm64 signed-by=" + key + " trusted=no] https://repo.pigsty.io/apt/infra generic other\n"
	deb822 := "Types: deb\nURIs: https://repo.pigsty.io/apt/infra/\nSuites: generic\nComponents: main\nTrusted: no\nSigned-By: " + key + "\n"
	writeRollbackFixture(t, filepath.Join(root, "sources.list"), mainSource)
	writeRollbackFixture(t, filepath.Join(m.RepoDir, "custom.sources"), deb822)
	result := ConfigureRepos([]string{"infra"}, AddOptions{Region: "default"})
	if !result.Success {
		t.Fatal(result)
	}
	if out, err := runAPTParser(t, root); err != nil {
		t.Fatalf("APT parser rejected inherited Deb822/root policy: %v\n%s", err, out)
	}
	assertRollbackFixture(t, filepath.Join(root, "sources.list"), mainSource)
	assertRollbackFixture(t, filepath.Join(m.RepoDir, "custom.sources"), deb822)
}
