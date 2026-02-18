package cmd

import (
	"os"
	"path/filepath"
	"pig/internal/config"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

func TestStyConfigureCommandRegistered(t *testing.T) {
	found, _, err := rootCmd.Find([]string{"sty", "configure"})
	if err != nil {
		t.Fatalf("failed to find 'pig sty configure': %v", err)
	}
	if found == nil {
		t.Fatal("expected non-nil command for 'pig sty configure'")
	}
	if found != pigstyConfigureCmd {
		t.Fatalf("resolved command mismatch: got %q, want %q", found.CommandPath(), pigstyConfigureCmd.CommandPath())
	}
}

func TestStyConfConfigureAliasIsSplitOut(t *testing.T) {
	for _, alias := range pigstyConfCmd.Aliases {
		if alias == "configure" {
			t.Fatal("configure alias should not remain on 'pig sty conf'")
		}
	}
}

func TestStyConfHasNativeFlagDefaultFalse(t *testing.T) {
	flag := pigstyConfCmd.Flags().Lookup("native")
	if flag == nil {
		t.Fatal("expected --native flag on pig sty conf")
	}
	if pigstyConfNative {
		t.Fatal("--native should default to false")
	}
}

func TestStyConfigureHasNoNativeFlag(t *testing.T) {
	if flag := pigstyConfigureCmd.Flags().Lookup("native"); flag != nil {
		t.Fatalf("did not expect --native on pig sty configure, got %+v", flag)
	}
}

func TestStyConfigureFlagsMatchConf(t *testing.T) {
	confFlags := collectFlagShorthands(pigstyConfCmd.LocalFlags())
	configureFlags := collectFlagShorthands(pigstyConfigureCmd.LocalFlags())

	// conf has one extra transition flag --native
	if _, ok := confFlags["native"]; !ok {
		t.Fatal("expected --native flag on pig sty conf")
	}
	delete(confFlags, "native")
	if len(confFlags) != len(configureFlags) {
		t.Fatalf("flag count mismatch after removing --native: conf=%d configure=%d", len(confFlags), len(configureFlags))
	}

	for name, shorthand := range confFlags {
		got, ok := configureFlags[name]
		if !ok {
			t.Fatalf("configure command missing flag %q", name)
		}
		if got != shorthand {
			t.Fatalf("flag shorthand mismatch for %q: conf=%q configure=%q", name, shorthand, got)
		}
	}
}

func collectFlagShorthands(flags *pflag.FlagSet) map[string]string {
	out := make(map[string]string)
	if flags == nil {
		return out
	}
	flags.VisitAll(func(f *pflag.Flag) {
		out[f.Name] = f.Shorthand
	})
	return out
}

func TestStyConfDefaultsToLegacyRoute(t *testing.T) {
	restore := saveStyConfState(t)
	defer restore()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	home := t.TempDir()
	if err := writeExecutable(filepath.Join(home, "configure"), "#!/bin/sh\nprintf legacy > route_legacy.txt\n"); err != nil {
		t.Fatalf("write configure script: %v", err)
	}

	config.PigstyHome = home
	pigstyConfNative = false

	if err := pigstyConfCmd.RunE(pigstyConfCmd, nil); err != nil {
		t.Fatalf("pig sty conf failed: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(home, "route_legacy.txt"))
	if err != nil {
		t.Fatalf("expected legacy marker file: %v", err)
	}
	if strings.TrimSpace(string(got)) != "legacy" {
		t.Fatalf("unexpected legacy marker content: %q", string(got))
	}
}

func TestStyConfNativeFlagUsesNativeRoute(t *testing.T) {
	restore := saveStyConfState(t)
	defer restore()

	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "conf"), 0755); err != nil {
		t.Fatalf("mkdir conf: %v", err)
	}
	template := `all:
  vars:
    admin_ip: 10.10.10.10
    region: default
    node_tune: oltp
    pg_version: 18
    pg_conf: oltp.yml
    pg_packages: [ pg18-main ]
`
	if err := os.WriteFile(filepath.Join(home, "conf", "meta.yml"), []byte(template), 0644); err != nil {
		t.Fatalf("write template: %v", err)
	}
	if err := writeExecutable(filepath.Join(home, "configure"), "#!/bin/sh\nprintf legacy > route_legacy.txt\n"); err != nil {
		t.Fatalf("write configure script: %v", err)
	}

	config.PigstyHome = home
	pigstyConfNative = true
	pigstyConfName = "meta"
	pigstyConfRegion = "default"
	pigstyConfSkip = true
	pigstyConfNonInteractive = true
	pigstyConfOutput = "native.yml"

	if err := pigstyConfCmd.RunE(pigstyConfCmd, nil); err != nil {
		t.Fatalf("pig sty conf --native failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(home, "route_legacy.txt")); err == nil {
		t.Fatal("legacy configure script should not be called with --native")
	}
	outPath := filepath.Join(home, "native.yml")
	content, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("expected native output file: %v", err)
	}
	if !strings.Contains(string(content), "10.10.10.10") {
		t.Fatalf("expected rendered output content, got:\n%s", string(content))
	}
}

func TestStyConfigureAlwaysUsesNativeRoute(t *testing.T) {
	restore := saveStyConfState(t)
	defer restore()

	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "conf"), 0755); err != nil {
		t.Fatalf("mkdir conf: %v", err)
	}
	template := `all:
  vars:
    admin_ip: 10.10.10.10
    region: default
    node_tune: oltp
    pg_version: 18
    pg_conf: oltp.yml
`
	if err := os.WriteFile(filepath.Join(home, "conf", "meta.yml"), []byte(template), 0644); err != nil {
		t.Fatalf("write template: %v", err)
	}
	if err := writeExecutable(filepath.Join(home, "configure"), "#!/bin/sh\nprintf legacy > route_legacy.txt\n"); err != nil {
		t.Fatalf("write configure script: %v", err)
	}

	config.PigstyHome = home
	pigstyConfNative = false
	pigstyConfName = "meta"
	pigstyConfRegion = "default"
	pigstyConfSkip = true
	pigstyConfNonInteractive = true
	pigstyConfOutput = "configure.yml"

	if err := pigstyConfigureCmd.RunE(pigstyConfigureCmd, nil); err != nil {
		t.Fatalf("pig sty configure failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(home, "route_legacy.txt")); err == nil {
		t.Fatal("legacy configure script should not be called by pig sty configure")
	}
	if _, err := os.Stat(filepath.Join(home, "configure.yml")); err != nil {
		t.Fatalf("expected native output file for configure command: %v", err)
	}
}

func writeExecutable(path, content string) error {
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		return err
	}
	return os.Chmod(path, 0755)
}

func saveStyConfState(t *testing.T) func() {
	t.Helper()
	origHome := config.PigstyHome
	origName := pigstyConfName
	origIP := pigstyConfIP
	origVer := pigstyConfVer
	origRegion := pigstyConfRegion
	origOutput := pigstyConfOutput
	origSkip := pigstyConfSkip
	origProxy := pigstyConfProxy
	origNonInteractive := pigstyConfNonInteractive
	origPort := pigstyConfPort
	origGenerate := pigstyConfGenerate
	origNative := pigstyConfNative

	return func() {
		config.PigstyHome = origHome
		pigstyConfName = origName
		pigstyConfIP = origIP
		pigstyConfVer = origVer
		pigstyConfRegion = origRegion
		pigstyConfOutput = origOutput
		pigstyConfSkip = origSkip
		pigstyConfProxy = origProxy
		pigstyConfNonInteractive = origNonInteractive
		pigstyConfPort = origPort
		pigstyConfGenerate = origGenerate
		pigstyConfNative = origNative
	}
}
