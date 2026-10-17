package build

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"pig/cli/ext"
	"pig/internal/config"
)

func TestBuildPackageInstallsEachRequestedPGDependency(t *testing.T) {
	oldHome, oldType, oldUser, oldCatalog := config.HomeDir, config.OSType, config.CurrentUser, ext.Catalog
	t.Cleanup(func() {
		config.HomeDir, config.OSType, config.CurrentUser, ext.Catalog = oldHome, oldType, oldUser, oldCatalog
	})
	home := t.TempDir()
	config.HomeDir, config.OSType, config.CurrentUser = home, config.DistroDEB, "root"
	if err := os.MkdirAll(filepath.Join(home, "ext", "src"), 0700); err != nil {
		t.Fatal(err)
	}
	writeBuildFixture(t, filepath.Join(home, "ext", "src", "dependency_probe"), "cached source fixture", 0600)
	e := &ext.Extension{Name: "dependency_probe", Pkg: "dependency_probe", DebPkg: "postgresql-$v-dependency-probe", PgVer: []string{"17", "18"}, DebPg: []string{"17", "18"}}
	ext.Catalog = &ext.ExtensionCatalog{ExtNameMap: map[string]*ext.Extension{e.Name: e}, ExtPkgMap: map[string]*ext.Extension{e.Pkg: e}}
	recipe := filepath.Join(home, "debbuild", e.Pkg)
	if err := os.MkdirAll(filepath.Join(recipe, "debian"), 0700); err != nil {
		t.Fatal(err)
	}
	writeBuildFixture(t, filepath.Join(recipe, "debian", "control.in"), "Source: dependency-probe\nBuild-Depends: postgresql-server-dev-PGVERSION\n", 0600)
	// The build requires both completed dependency commands, even though the
	// production pipeline intentionally continues after dependency warnings.
	writeBuildFixture(t, filepath.Join(recipe, "Makefile"), "all:\n\ttest -f \"$$PIG_DEP_HOME/dep17\" -a -f \"$$PIG_DEP_HOME/dep18\"\n\tmkdir -p \"$$PIG_DEP_HOME/ext/pkg\"\n\tfor pg in 17 18; do printf fixture > \"$$PIG_DEP_HOME/ext/pkg/postgresql-$$pg-dependency-probe_1_arm64.deb\"; done\n", 0600)
	bin := t.TempDir()
	script := `#!/bin/sh
set -e
printf '%s\n' "$*" >> "$PIG_DEP_HOME/dependency-calls"
if [ "$PIG_TEST_REAL_APT" = 1 ]; then /usr/bin/apt "$@"; fi
for arg do
 case "$arg" in postgresql-server-dev-17|postgresql-server-dev-18) touch "$PIG_DEP_HOME/dep${arg##*-}";; esac
done
`
	if os.Getenv("PIG_TEST_REAL_APT") == "1" && (runtime.GOOS != "linux" || os.Geteuid() != 0) {
		t.Fatal("real apt acceptance requires a disposable root Linux container")
	}
	writeBuildFixture(t, filepath.Join(bin, "apt"), script, 0700)
	t.Setenv("PIG_NO_SUDO", "1")
	t.Setenv("PIG_DEP_HOME", home)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := BuildPackage(e.Name, "17,18", false, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(home, "dependency-calls"))
	if err != nil || string(data) != "install -y postgresql-server-dev-17\ninstall -y postgresql-server-dev-18\n" {
		t.Fatalf("dependencies were not installed per major: %q %v", data, err)
	}
	t.Logf("executed dependency commands:\n%s", data)
}

func TestDebianBuildPreservesInheritedToolchain(t *testing.T) {
	home, bin := t.TempDir(), t.TempDir()
	recipe := filepath.Join(home, "debbuild", "toolchain_probe")
	if err := os.MkdirAll(recipe, 0700); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"cc", "cargo"} {
		writeBuildFixture(t, filepath.Join(bin, tool), "#!/bin/sh\nprintf 'custom-"+tool+"\\n'\n", 0700)
	}
	writeBuildFixture(t, filepath.Join(recipe, "Makefile"), `all:
	cc --version > cc.out
	cargo --version > cargo.out
	mkdir -p "$$PIG_TOOLCHAIN_HOME/ext/pkg"
	printf fixture > "$$PIG_TOOLCHAIN_HOME/ext/pkg/postgresql-18-toolchain-probe_1_arm64.deb"
`, 0600)
	t.Setenv("PIG_TOOLCHAIN_HOME", home)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	b := &ExtensionBuilder{PackageName: "toolchain_probe", Extension: &ext.Extension{Name: "toolchain_probe", Pkg: "toolchain_probe", DebPkg: "postgresql-$v-toolchain-probe"}, OSType: config.DistroDEB, HomeDir: home, PGVersions: []int{18}, Builds: make(map[int]*BuildTask), HeaderWidth: 60}
	b.buildForAll()
	if task := b.Builds[0]; task == nil || !task.Success {
		t.Fatalf("toolchain make failed: %+v", task)
	}
	for _, tool := range []string{"cc", "cargo"} {
		data, err := os.ReadFile(filepath.Join(recipe, tool+".out"))
		if err != nil || strings.TrimSpace(string(data)) != "custom-"+tool {
			t.Errorf("make did not preserve %s from inherited PATH: %q, %v", tool, data, err)
		}
	}
}

func TestDebianBuildExecutesCargoFromUserHome(t *testing.T) {
	if runtime.GOOS != "linux" || os.Getenv("PIG_TEST_BUILD_LINUX") != "1" {
		t.Skip("requires disposable Debian/Ubuntu build environment and PIG_TEST_BUILD_LINUX=1")
	}
	// The parent cannot find Cargo. The actual make child must compile and run
	// a Rust binary through buildForAll's environment before packaging it.
	t.Setenv("PATH", "/usr/bin:/bin")
	if _, err := exec.LookPath("cargo"); err == nil {
		t.Fatal("fixture requires Cargo outside the parent PATH")
	}
	home := t.TempDir()
	recipe := filepath.Join(home, "debbuild", "cargo_probe")
	if err := os.MkdirAll(filepath.Join(recipe, "src"), 0700); err != nil {
		t.Fatal(err)
	}
	writeBuildFixture(t, filepath.Join(recipe, "Cargo.toml"), "[package]\nname = \"pig-cargo-probe\"\nversion = \"0.1.0\"\nedition = \"2021\"\n", 0600)
	writeBuildFixture(t, filepath.Join(recipe, "src", "main.rs"), "fn main() { println!(\"cargo-path-verified\"); }\n", 0600)
	writeBuildFixture(t, filepath.Join(recipe, "Makefile"), `all:
	command -v cargo > cargo-path
	cargo --version
	cargo build --offline
	./target/debug/pig-cargo-probe > executed
	mkdir -p pkg/DEBIAN pkg/usr/bin "$$PIG_CARGO_HOME/ext/pkg"
	printf 'Package: postgresql-18-cargo-probe\nVersion: 1.0\nArchitecture: arm64\nMaintainer: Test <test@example.invalid>\nDescription: Cargo PATH acceptance fixture\n' > pkg/DEBIAN/control
	cp target/debug/pig-cargo-probe pkg/usr/bin/
	dpkg-deb --build pkg "$$PIG_CARGO_HOME/ext/pkg/postgresql-18-cargo-probe_1.0_arm64.deb"
`, 0600)
	t.Setenv("PIG_CARGO_HOME", home)
	b := &ExtensionBuilder{PackageName: "cargo_probe", Extension: &ext.Extension{Name: "cargo_probe", Pkg: "cargo_probe", DebPkg: "postgresql-$v-cargo-probe"}, OSType: config.DistroDEB, HomeDir: home, PGVersions: []int{18}, Builds: make(map[int]*BuildTask), HeaderWidth: 60}
	b.buildForAll()
	if task := b.Builds[0]; task == nil || !task.Success {
		t.Fatalf("Cargo make failed: %+v", task)
	}
	data, _ := os.ReadFile(filepath.Join(recipe, "executed"))
	if strings.TrimSpace(string(data)) != "cargo-path-verified" {
		t.Fatalf("Rust binary did not execute: %q", data)
	}
	path, _ := os.ReadFile(filepath.Join(recipe, "cargo-path"))
	t.Logf("make executed %s and packaged the compiled Rust binary", strings.TrimSpace(string(path)))
}

func writeBuildFixture(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}
