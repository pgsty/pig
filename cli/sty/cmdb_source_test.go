package sty

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pig/internal/cmdb"
	"pig/internal/output"
)

func TestResolveCMDBSourceExactPrecedenceAndPersistentDescriptor(t *testing.T) {
	fixture := newCMDBSourceFixture(t)
	cliPath := fixture.writeInventory("cli.yml", "all: {}\n")
	envPath := fixture.writeInventory("env.yml", "all: {}\n")
	configPath := fixture.writeInventory("configured.yml", "all: {}\n")
	persistentPath := fixture.writeInventory("persistent.yml", "all: {}\n")
	homeDefault := filepath.Join(fixture.home, "pigsty.yml")
	writeCMDBSourceFile(t, homeDefault, "all: {}\n", 0o600)
	writeCMDBSourceFile(t, fixture.ansibleConfig, "[defaults]\ninventory = ../persistent.yml\n", 0o600)
	writeCMDBSourceFile(t, fixture.userConfig, "inventory: configured.yml\n", 0o600)

	options := fixture.options()
	options.InvocationInventory = "cli.yml"
	options.LookupEnv = func(name string) (string, bool) {
		return "env.yml", name == "PIGSTY_INVENTORY"
	}
	resolution, failure := ResolveCMDBSource(options)
	assertCMDBSource(t, failure, resolution.Effective, CMDBSourceStatic, CMDBSourceCLI, cliPath, false)
	assertCMDBSource(t, nil, resolution.Persistent, CMDBSourceStatic, CMDBSourceAnsibleConfig, persistentPath, true)

	options.InvocationInventory = ""
	resolution, failure = ResolveCMDBSource(options)
	assertCMDBSource(t, failure, resolution.Effective, CMDBSourceStatic, CMDBSourceEnvironment, envPath, false)

	options.LookupEnv = func(string) (string, bool) { return "", false }
	resolution, failure = ResolveCMDBSource(options)
	assertCMDBSource(t, failure, resolution.Effective, CMDBSourceStatic, CMDBSourceUserConfig, configPath, false)

	writeCMDBSourceFile(t, fixture.userConfig, "license: test\n", 0o600)
	resolution, failure = ResolveCMDBSource(options)
	assertCMDBSource(t, failure, resolution.Effective, CMDBSourceStatic, CMDBSourceAnsibleConfig, persistentPath, true)

	if err := os.Remove(fixture.ansibleConfig); err != nil {
		t.Fatal(err)
	}
	resolution, failure = ResolveCMDBSource(options)
	assertCMDBSource(t, failure, resolution.Effective, CMDBSourceStatic, CMDBSourceHomeDefault, homeDefault, false)
	if resolution.PersistentFinding != "CMDB_ANSIBLE_CONFIG_MISSING" {
		t.Fatalf("missing persistent finding = %q", resolution.PersistentFinding)
	}
}

func TestResolveCMDBSourceSelectedInvalidTierNeverFallsThrough(t *testing.T) {
	fixture := newCMDBSourceFixture(t)
	fallback := fixture.writeInventory("fallback.yml", "all: {}\n")
	writeCMDBSourceFile(t, fixture.userConfig, "inventory: fallback.yml\n", 0o600)
	writeCMDBSourceFile(t, fixture.ansibleConfig, "[defaults]\ninventory = ../fallback.yml\n", 0o600)
	executable := filepath.Join(fixture.root, "dynamic")
	writeCMDBSourceFile(t, executable, "#!/bin/sh\nexit 0\n", 0o700)
	plugin := fixture.writeInventory("plugin.yml", "plugin: amazon.aws.aws_ec2\n")
	target := fixture.writeInventory("target.yml", "all: {}\n")
	symlink := filepath.Join(fixture.root, "linked.yml")
	if err := os.Symlink(target, symlink); err != nil {
		t.Fatal(err)
	}

	for name, selected := range map[string]string{
		"directory":  fixture.root,
		"executable": executable,
		"list":       "one.yml,two.yml",
		"plugin":     plugin,
		"symlink":    symlink,
	} {
		t.Run(name, func(t *testing.T) {
			options := fixture.options()
			options.LookupEnv = func(env string) (string, bool) {
				return selected, env == "PIGSTY_INVENTORY"
			}
			resolution, failure := ResolveCMDBSource(options)
			if failure == nil || resolution.Effective.Kind != CMDBSourceUnsupported ||
				resolution.Effective.Origin != CMDBSourceEnvironment || resolution.Effective.Locator == fallback {
				t.Fatalf("selected %q fell through: resolution=%+v failure=%+v", selected, resolution, failure)
			}
		})
	}

	missing := "missing.yml"
	options := fixture.options()
	options.LookupEnv = func(env string) (string, bool) { return missing, env == "PIGSTY_INVENTORY" }
	resolution, failure := ResolveCMDBSource(options)
	assertCMDBSource(t, failure, resolution.Effective, CMDBSourceStatic, CMDBSourceEnvironment, filepath.Join(fixture.root, missing), false)
}

func TestResolveCMDBSourceClassifiesOnlyVerifiedAdapterAsNative(t *testing.T) {
	fixture := newCMDBSourceFixture(t)
	record, err := cmdb.PrepareRuntime(cmdb.RuntimeOptions{
		PigstyHome: fixture.home, RuntimeRoot: fixture.runtimeRoot, PigBinary: fixture.pigBinary,
		ConnectionSource: cmdb.ConnectionMetaSource,
	})
	if err != nil {
		t.Fatal(err)
	}
	writeCMDBSourceFile(t, fixture.ansibleConfig, "[defaults]\ninventory = "+record.AdapterPath+"\n", 0o600)

	options := fixture.options()
	resolution, failure := ResolveCMDBSource(options)
	assertCMDBSource(t, failure, resolution.Effective, CMDBSourceNative, CMDBSourceAnsibleConfig, cmdb.RuntimeAdapterIdentity(record), true)
	if resolution.Persistent != resolution.Effective {
		t.Fatalf("persistent native descriptor diverged: %+v != %+v", resolution.Persistent, resolution.Effective)
	}

	static := fixture.writeInventory("override.yml", "all: {}\n")
	options.InvocationInventory = static
	resolution, failure = ResolveCMDBSource(options)
	assertCMDBSource(t, failure, resolution.Effective, CMDBSourceStatic, CMDBSourceCLI, static, false)
	if got := CMDBSplitBrainNotice(resolution.Effective, resolution.Persistent); got != CMDBStaticShadowsPersistentNotice {
		t.Fatalf("split-brain notice = %q", got)
	}

	options.InvocationInventory = record.AdapterPath
	resolution, failure = ResolveCMDBSource(options)
	if failure == nil || resolution.Effective.Kind != CMDBSourceUnsupported || resolution.Effective.Kind == CMDBSourceNative {
		t.Fatalf("-i reclassified adapter as native: %+v, %+v", resolution, failure)
	}

	options.InvocationInventory = ""
	if err := os.WriteFile(record.AdapterPath, []byte("#!/bin/sh\n# altered\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	resolution, failure = ResolveCMDBSource(options)
	if failure == nil || resolution.Effective.Kind != CMDBSourceUnsupported ||
		resolution.PersistentFinding != "CMDB_RUNTIME_ADAPTER_INVALID" {
		t.Fatalf("altered adapter resolution = %+v, %+v", resolution, failure)
	}
}

func TestResolveCMDBSourcePersistentInvalidBlocksOnlyPersistentTier(t *testing.T) {
	fixture := newCMDBSourceFixture(t)
	cliPath := fixture.writeInventory("cli.yml", "all: {}\n")
	writeCMDBSourceFile(t, filepath.Join(fixture.home, "pigsty.yml"), "all: {}\n", 0o600)
	writeCMDBSourceFile(t, fixture.ansibleConfig, "[defaults]\ninventory = one.yml,two.yml\n", 0o600)

	options := fixture.options()
	resolution, failure := ResolveCMDBSource(options)
	if failure == nil || resolution.Effective.Kind != CMDBSourceUnsupported ||
		resolution.PersistentFinding != "CMDB_ANSIBLE_CONFIG_INVALID" {
		t.Fatalf("invalid persistent source fell through: %+v, %+v", resolution, failure)
	}

	options.InvocationInventory = cliPath
	resolution, failure = ResolveCMDBSource(options)
	assertCMDBSource(t, failure, resolution.Effective, CMDBSourceStatic, CMDBSourceCLI, cliPath, false)
	if resolution.Persistent.Kind != CMDBSourceUnsupported || resolution.PersistentFinding != "CMDB_ANSIBLE_CONFIG_INVALID" {
		t.Fatalf("persistent finding was hidden by override: %+v", resolution)
	}

	writeCMDBSourceFile(t, fixture.userConfig, "inventory: [not, scalar]\n", 0o600)
	options.InvocationInventory = ""
	resolution, failure = ResolveCMDBSource(options)
	if failure == nil || resolution.Effective.Origin != CMDBSourceUserConfig || resolution.Effective.Kind != CMDBSourceUnsupported {
		t.Fatalf("invalid user tier fell through: %+v, %+v", resolution, failure)
	}
}

type cmdbSourceFixture struct {
	t             *testing.T
	root          string
	home          string
	userHome      string
	userConfig    string
	ansibleConfig string
	runtimeRoot   string
	pigBinary     string
}

func newCMDBSourceFixture(t *testing.T) cmdbSourceFixture {
	t.Helper()
	root := symlinkFreeStyTempDir(t)
	home := filepath.Join(root, "pigsty")
	userHome := filepath.Join(root, "user")
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(userHome, ".pig"), 0o700); err != nil {
		t.Fatal(err)
	}
	pigBinary := filepath.Join(root, "pig")
	writeCMDBSourceFile(t, pigBinary, "#!/bin/sh\nexit 0\n", 0o700)
	return cmdbSourceFixture{
		t: t, root: root, home: home, userHome: userHome,
		userConfig:    filepath.Join(userHome, ".pig", "config.yml"),
		ansibleConfig: filepath.Join(home, "ansible.cfg"),
		runtimeRoot:   filepath.Join(root, "runtime"), pigBinary: pigBinary,
	}
}

func (f cmdbSourceFixture) options() CMDBSourceResolveOptions {
	return CMDBSourceResolveOptions{
		PigstyHome: f.home, WorkingDirectory: f.root, UserHome: f.userHome,
		UserConfigPath: f.userConfig, RuntimeRoot: f.runtimeRoot, PigBinary: f.pigBinary,
		LookupEnv: func(string) (string, bool) { return "", false },
	}
}

func (f cmdbSourceFixture) writeInventory(name, body string) string {
	path := filepath.Join(f.root, name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		f.t.Fatal(err)
	}
	return path
}

func writeCMDBSourceFile(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func assertCMDBSource(t *testing.T, failure *output.Result, descriptor CMDBSourceDescriptor, kind CMDBSourceKind, origin CMDBSourceOrigin, locator string, persistent bool) {
	t.Helper()
	if failure != nil || descriptor.Kind != kind || descriptor.Origin != origin ||
		descriptor.Locator != locator || descriptor.Persistent != persistent {
		t.Fatalf("source descriptor = %+v failure=%+v; want %s/%s/%s/%t", descriptor, failure, kind, origin, locator, persistent)
	}
}

func symlinkFreeStyTempDir(t *testing.T) string {
	t.Helper()
	directory, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return directory
}

func TestCMDBSourceResolutionDoesNotEmbedInventoryValues(t *testing.T) {
	fixture := newCMDBSourceFixture(t)
	secret := "source-password-value"
	writeCMDBSourceFile(t, fixture.userConfig, "inventory: one.yml\npassword: "+secret+"\n", 0o600)
	resolution, failure := ResolveCMDBSource(fixture.options())
	text := strings.Join([]string{
		resolution.Effective.Locator, resolution.Persistent.Locator,
		resolution.PersistentFinding,
	}, " ")
	if failure != nil {
		text += failure.Message + failure.Detail
	}
	if strings.Contains(text, secret) {
		t.Fatalf("source resolution leaked user-config value: %s", text)
	}
}
