package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func TestFindInventoryPathPrecedenceAndConcreteExplicitSource(t *testing.T) {
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
	if got := findInventoryPath("missing-explicit.yml", home); got != want {
		t.Fatalf("explicit inventory = %q, want %q", got, want)
	}
	if got := findInventoryPath("", home); got != filepath.Join(root, "from-env.yml") {
		t.Fatalf("environment inventory = %q", got)
	}

	t.Setenv("PIGSTY_INVENTORY", "")
	if got := findInventoryPath("", home); got != filepath.Join(root, "from-config.yml") {
		t.Fatalf("config inventory = %q", got)
	}

	viper.Set("inventory", "")
	if got := findInventoryPath("", home); got != filepath.Join(home, "from-ansible.yml") {
		t.Fatalf("ansible.cfg inventory = %q", got)
	}
}

func TestFindInventoryPathFallsBackWithinResolvedHome(t *testing.T) {
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

	if got, want := findInventoryPath("", home), filepath.Join(home, "pigsty.yml"); got != want {
		t.Fatalf("fallback inventory = %q, want %q", got, want)
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
	got, err := readAnsibleInventoryDirective(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "conf/pigsty.yml" {
		t.Fatalf("inventory directive = %q, want conf/pigsty.yml", got)
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
