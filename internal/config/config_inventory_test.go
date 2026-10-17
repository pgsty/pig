package config

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestResolveInventorySourcePrecedenceAndConcreteExplicitSource(t *testing.T) {
	originalHome := HomeDir
	defer func() {
		HomeDir = originalHome
		viper.Reset()
	}()

	root := t.TempDir()
	HomeDir = filepath.Join(root, "user")
	t.Chdir(root)
	t.Setenv("PIGSTY_INVENTORY", "from-env.yml")
	viper.Reset()
	viper.Set("inventory", "from-config.yml")

	home := filepath.Join(root, "pigsty")
	if err := os.MkdirAll(home, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "ansible.cfg"), []byte("[defaults]\ninventory = from-ansible.yml\n"), 0600); err != nil {
		t.Fatal(err)
	}

	// A missing -i target is still authoritative; falling through would make a
	// typo operate on an unrelated configured Inventory.
	want := filepath.Join(root, "missing-explicit.yml")
	if got := resolveInventorySource("missing-explicit.yml", home).Path; got != want {
		t.Fatalf("explicit inventory = %q, want %q", got, want)
	}
	if got := resolveInventorySource("", home).Path; got != filepath.Join(root, "from-env.yml") {
		t.Fatalf("environment inventory = %q", got)
	}

	t.Setenv("PIGSTY_INVENTORY", "")
	if got := resolveInventorySource("", home).Path; got != filepath.Join(home, "from-config.yml") {
		t.Fatalf("config inventory = %q", got)
	}

	viper.Set("inventory", "")
	if got := resolveInventorySource("", home).Path; got != filepath.Join(home, "from-ansible.yml") {
		t.Fatalf("ansible.cfg inventory = %q", got)
	}
}

func TestResolveInventorySourceFallsBackWithinResolvedHome(t *testing.T) {
	originalHome := HomeDir
	defer func() {
		HomeDir = originalHome
		viper.Reset()
	}()

	root := t.TempDir()
	HomeDir = filepath.Join(root, "user")
	t.Setenv("PIGSTY_INVENTORY", "")
	viper.Reset()
	home := filepath.Join(root, "selected-home")
	if err := os.MkdirAll(home, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "ansible.cfg"), []byte("[defaults]\nroles_path = roles\n"), 0600); err != nil {
		t.Fatal(err)
	}

	if got, want := resolveInventorySource("", home).Path, filepath.Join(home, "pigsty.yml"); got != want {
		t.Fatalf("fallback inventory = %q, want %q", got, want)
	}
}

func TestRelativeConfiguredInventoryRequiresHome(t *testing.T) {
	originalViperString, originalConfigFile := viperString, ConfigFile
	t.Cleanup(func() { viperString, ConfigFile = originalViperString, originalConfigFile })
	t.Setenv("PIGSTY_INVENTORY", "")
	root := t.TempDir()
	t.Chdir(root)
	ConfigFile = filepath.Join(root, "config.yml")
	// An unrelated file in cwd must not turn a home-relative source into a
	// usable inventory when no Pigsty home was selected.
	if err := os.WriteFile("pigsty.yml", []byte("all: {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	viperString = func(string) string { return "pigsty.yml" }
	got := resolveInventorySource("", "")
	if got.Tier != InventorySourceConfig || got.Type != InventorySourceUnsupported || got.Path != "" || got.OriginPath != ConfigFile || !slices.Contains(got.Findings, "SOURCE_CONFIG_HOME_REQUIRED") {
		t.Fatalf("relative configured inventory silently used cwd: %+v", got)
	}
	// Absolute config paths and explicit relative flags remain usable.
	viperString = func(string) string { return filepath.Join(root, "pigsty.yml") }
	for _, explicit := range []string{"", "pigsty.yml"} {
		if got := resolveInventorySource(explicit, ""); got.Type != InventorySourceStaticYAML || got.Path != filepath.Join(root, "pigsty.yml") {
			t.Fatalf("unambiguous source rejected: %+v", got)
		}
	}
}

func TestReadAnsibleInventoryDirectiveIsDefaultsScoped(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ansible.cfg")
	source := `[inventory]
inventory = ignored.yml

[defaults]
# inventory = commented.yml
inventory = "conf/pigsty.yml" ; active static inventory

[ssh_connection]
inventory = also-ignored.yml
`
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	directive, err := ReadAnsibleInventoryDirective(path)
	if err != nil {
		t.Fatal(err)
	}
	if directive.Value != "conf/pigsty.yml" {
		t.Fatalf("inventory directive = %q, want conf/pigsty.yml", directive.Value)
	}
}

func TestFindPigstyHomeKeepsExplicitInvalidHome(t *testing.T) {
	originalHome := HomeDir
	defer func() {
		HomeDir = originalHome
		viper.Reset()
	}()

	root := t.TempDir()
	HomeDir = filepath.Join(root, "user")
	viper.Reset()
	explicit := filepath.Join(root, "not-installed-yet")
	if got := findPigstyHome(explicit); got != explicit {
		t.Fatalf("explicit home = %q, want %q", got, explicit)
	}
}

func TestParseAnsibleInventoryDirectiveStrictAndConfigRelative(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "ansible.cfg")
	body := []byte("[defaults]\nroles_path = roles\ninventory = \"conf/pigsty.yml\" ; selected\n")
	directive, err := ParseAnsibleInventoryDirective(configPath, body)
	if err != nil {
		t.Fatal(err)
	}
	if directive.Value != "conf/pigsty.yml" || directive.Literal != `"conf/pigsty.yml"` {
		t.Fatalf("directive = %+v", directive)
	}
	if want := filepath.Join(filepath.Dir(configPath), "conf", "pigsty.yml"); directive.Path != want {
		t.Fatalf("directive path = %q, want %q", directive.Path, want)
	}
	if got := string(body[directive.ValueStart:directive.ValueEnd]); got != directive.Literal {
		t.Fatalf("directive span = %q, want %q", got, directive.Literal)
	}

	for _, source := range []string{
		"[defaults]\ninventory = one.yml\ninventory = two.yml\n",
		"[defaults]\ninventory = one.yml\n[defaults]\nroles_path = roles\n",
		"[defaults]\ninventory\n",
		"[defaults]\ninventory = {{ dynamic }}\n",
	} {
		if _, err := ParseAnsibleInventoryDirective(configPath, []byte(source)); err == nil {
			t.Fatalf("ambiguous or unsupported source accepted: %q", source)
		}
	}
	if _, err := ParseAnsibleInventoryDirective(configPath, []byte("[defaults]\nroles_path = roles\n")); !errors.Is(err, ErrAnsibleInventoryDirectiveMissing) {
		t.Fatalf("missing directive error = %v", err)
	}
}

func TestResolveInventorySourceClassificationsAndFirstTierAuthority(t *testing.T) {
	originalHome, originalConfigFile := HomeDir, ConfigFile
	originalViperString := viperString
	defer func() {
		HomeDir, ConfigFile = originalHome, originalConfigFile
		viperString = originalViperString
		viper.Reset()
	}()

	root := t.TempDir()
	HomeDir = filepath.Join(root, "user")
	ConfigFile = filepath.Join(HomeDir, ".pig", "config.yml")
	t.Setenv("PIGSTY_INVENTORY", "")
	viperString = func(string) string { return "" }

	home := filepath.Join(root, "pigsty")
	if err := os.MkdirAll(home, 0755); err != nil {
		t.Fatal(err)
	}
	staticPath := filepath.Join(root, "static.yml")
	if err := os.WriteFile(staticPath, []byte("all: {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(root, "inventory.d")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	dynamic := filepath.Join(root, "dynamic.sh")
	if err := os.WriteFile(dynamic, []byte("#!/bin/sh\necho '{}'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "linked.yml")
	if err := os.Symlink(staticPath, link); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		path string
		typ  InventorySourceType
	}{
		{"static", staticPath, InventorySourceStaticYAML},
		{"directory", directory, InventorySourceDirectory},
		{"dynamic", dynamic, InventorySourceExecutableDynamic},
		{"missing", filepath.Join(root, "missing.yml"), InventorySourceMissing},
		{"symlink", link, InventorySourceStaticYAML},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			descriptor := resolveInventorySource(test.path, home)
			if descriptor.Tier != InventorySourceFlag || descriptor.Type != test.typ || descriptor.Path != test.path {
				t.Fatalf("descriptor = %+v", descriptor)
			}
			if test.name == "symlink" && (!descriptor.Symlink || slices.Contains(descriptor.StaticCommands, "edit")) {
				t.Fatalf("symlink descriptor = %+v", descriptor)
			}
		})
	}

	// The explicit missing source remains authoritative over an otherwise valid
	// ansible.cfg source.
	if err := os.WriteFile(filepath.Join(home, "ansible.cfg"), []byte("[defaults]\ninventory = pigsty.yml\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "pigsty.yml"), []byte("all: {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	descriptor := resolveInventorySource(filepath.Join(root, "still-missing.yml"), home)
	if descriptor.Tier != InventorySourceFlag || descriptor.Type != InventorySourceMissing {
		t.Fatalf("first configured tier was not authoritative: %+v", descriptor)
	}
}

func TestResolveInventorySourceManagedShapeAndAmbiguousDirective(t *testing.T) {
	originalViperString := viperString
	defer func() { viperString = originalViperString }()
	viperString = func(string) string { return "" }
	t.Setenv("PIGSTY_INVENTORY", "")

	home := t.TempDir()
	scriptPath := filepath.Join(home, "inventory.sh")
	pigScript := "#!/bin/bash\nexec psql 'service=meta' -AXtwc 'SELECT text FROM pigsty.inventory;'\n"
	if err := os.WriteFile(scriptPath, []byte(pigScript), 0755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(home, "ansible.cfg")
	if err := os.WriteFile(configPath, []byte("[defaults]\ninventory = inventory.sh\n"), 0600); err != nil {
		t.Fatal(err)
	}
	descriptor := resolveInventorySource("", home)
	if descriptor.Type != InventorySourceManagedCMDB || !descriptor.Managed || !descriptor.Executable {
		t.Fatalf("managed descriptor = %+v", descriptor)
	}
	passwordScript := "#!/bin/bash\npsql postgresql://user:secret@db/meta -AXtwc 'SELECT text FROM pigsty.inventory;'\n"
	if err := os.WriteFile(scriptPath, []byte(passwordScript), 0o755); err != nil {
		t.Fatal(err)
	}
	descriptor = resolveInventorySource("", home)
	if descriptor.Type != InventorySourceManagedCMDB || !slices.Contains(descriptor.Findings, "SOURCE_MANAGED_PASSWORD_LITERAL") {
		t.Fatalf("password-bearing managed descriptor = %+v", descriptor)
	}

	if err := os.WriteFile(configPath, []byte("[defaults]\ninventory = inventory.sh\ninventory = pigsty.yml\n"), 0600); err != nil {
		t.Fatal(err)
	}
	descriptor = resolveInventorySource("", home)
	if descriptor.Type != InventorySourceUnsupported || !slices.Contains(descriptor.Findings, "SOURCE_ANSIBLE_CONFIG_INVALID") {
		t.Fatalf("ambiguous descriptor = %+v", descriptor)
	}
}

func TestManagedInventoryScriptShapeIsBounded(t *testing.T) {
	query := " -AXtwc 'SELECT text FROM pigsty.inventory;'\n"
	tests := []struct {
		name     string
		body     string
		kind     ManagedInventoryScriptKind
		ok       bool
		password bool
	}{
		{"pig", "#!/bin/bash\nexec psql 'service=meta'" + query, ManagedInventoryScriptPig, true, false},
		{"pig quoted", "#!/bin/bash\nexec psql 'service=meta application_name='\\''pig'\\'''" + query, ManagedInventoryScriptPig, true, false},
		{"pig password", "#!/bin/bash\nexec psql 'host=db password=secret'" + query, ManagedInventoryScriptPig, true, true},
		{"legacy", "#!/bin/bash\npsql service=meta" + query, ManagedInventoryScriptLegacy, true, false},
		{"legacy password token", "#!/bin/bash\npsql postgresql://user:pass@db/meta" + query, ManagedInventoryScriptLegacy, true, true},
		{"legacy query password", "#!/bin/bash\npsql postgresql://user@db/meta?password=pass" + query, ManagedInventoryScriptLegacy, true, true},
		{"empty pig connection", "#!/bin/bash\nexec psql ''" + query, "", false, false},
		{"legacy space conninfo", "#!/bin/bash\npsql host=db dbname=meta" + query, "", false, false},
		{"third party shell", "#!/bin/bash\npsql service=meta" + query + "echo changed\n", "", false, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kind, ok := ManagedInventoryScriptShape([]byte(test.body))
			if kind != test.kind || ok != test.ok {
				t.Fatalf("shape = (%q, %v), want (%q, %v)", kind, ok, test.kind, test.ok)
			}
			if got := ManagedInventoryScriptHasPassword([]byte(test.body)); got != test.password {
				t.Fatalf("password = %v, want %v", got, test.password)
			}
		})
	}
}

func TestInitConfigWithoutPersistentWrite(t *testing.T) {
	originalOSCode := OSCode
	defer func() { OSCode = originalOSCode }()
	original := struct {
		homeDir, configDir, configFile, pigstyHome, pigstyConfig string
		source                                                   InventorySourceDescriptor
	}{HomeDir, ConfigDir, ConfigFile, PigstyHome, PigstyConfig, PigstyInventorySource}
	defer func() {
		HomeDir, ConfigDir, ConfigFile = original.homeDir, original.configDir, original.configFile
		PigstyHome, PigstyConfig, PigstyInventorySource = original.pigstyHome, original.pigstyConfig, original.source
		viper.Reset()
	}()

	root := t.TempDir()
	userHome := filepath.Join(root, "user")
	pigstyHome := filepath.Join(root, "pigsty")
	if err := os.MkdirAll(pigstyHome, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pigstyHome, "ansible.cfg"), []byte("[defaults]\ninventory = pigsty.yml\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pigstyHome, "pigsty.yml"), []byte("all: {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", userHome)
	t.Setenv("PIGSTY_INVENTORY", "")
	viper.Reset()

	OSCode = "sentinel-no-probe"
	if err := InitConfigWithOptions("", pigstyHome, InitOptions{CreateConfig: false, SkipEnvironment: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(userHome, ".pig")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read-only initialization created ~/.pig: %v", err)
	}
	if PigstyInventorySource.Type != InventorySourceStaticYAML || PigstyConfig != filepath.Join(pigstyHome, "pigsty.yml") {
		t.Fatalf("initialized source = %+v path=%q", PigstyInventorySource, PigstyConfig)
	}
	if OSCode != "sentinel-no-probe" {
		t.Fatalf("read-only Inventory initialization probed the environment: OSCode=%q", OSCode)
	}
}

func TestInitConfigRejectsMalformedExistingFile(t *testing.T) {
	original := struct {
		homeDir, configDir, configFile, pigstyHome, pigstyConfig string
		source                                                   InventorySourceDescriptor
	}{HomeDir, ConfigDir, ConfigFile, PigstyHome, PigstyConfig, PigstyInventorySource}
	t.Cleanup(func() {
		HomeDir, ConfigDir, ConfigFile = original.homeDir, original.configDir, original.configFile
		PigstyHome, PigstyConfig, PigstyInventorySource = original.pigstyHome, original.pigstyConfig, original.source
		viper.Reset()
	})
	home := t.TempDir()
	configDir := filepath.Join(home, ".pig")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.yml"), []byte("inventory: [broken\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	viper.Reset()
	if err := InitConfigWithOptions("", "", InitOptions{CreateConfig: false, SkipEnvironment: true}); err == nil || !strings.Contains(err.Error(), "failed to read config file") {
		t.Fatalf("malformed config error = %v", err)
	}
}
