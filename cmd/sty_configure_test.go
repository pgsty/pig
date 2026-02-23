package cmd

import (
	"os"
	"path/filepath"
	"pig/internal/config"
	"strings"
	"testing"
)

func TestStyConfigureAliasResolvesToConfCommand(t *testing.T) {
	found, _, err := rootCmd.Find([]string{"sty", "configure"})
	if err != nil {
		t.Fatalf("failed to find 'pig sty configure': %v", err)
	}
	if found == nil {
		t.Fatal("expected non-nil command for 'pig sty configure'")
	}
	if found != pigstyConfCmd {
		t.Fatalf("resolved command mismatch: got %q, want %q", found.CommandPath(), pigstyConfCmd.CommandPath())
	}
}

func TestStyConfHasConfigureAlias(t *testing.T) {
	aliases := map[string]bool{}
	for _, alias := range pigstyConfCmd.Aliases {
		aliases[alias] = true
	}
	if !aliases["c"] {
		t.Fatal("expected alias 'c' on pig sty conf")
	}
	if !aliases["configure"] {
		t.Fatal("expected alias 'configure' on pig sty conf")
	}
}

func TestStyConfHasRawFlagDefaultFalse(t *testing.T) {
	flag := pigstyConfCmd.Flags().Lookup("raw")
	if flag == nil {
		t.Fatal("expected --raw flag on pig sty conf")
	}
	if pigstyConfRaw {
		t.Fatal("--raw should default to false")
	}
}

func TestStyConfHasNoNativeFlag(t *testing.T) {
	if flag := pigstyConfCmd.Flags().Lookup("native"); flag != nil {
		t.Fatalf("did not expect --native on pig sty conf, got %+v", flag)
	}
}

func TestStyConfDefaultsToNativeRoute(t *testing.T) {
	restore := saveStyConfState(t)
	defer restore()

	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "conf"), 0o755); err != nil {
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
	if err := os.WriteFile(filepath.Join(home, "conf", "meta.yml"), []byte(template), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}
	if err := writeExecutable(filepath.Join(home, "configure"), "#!/bin/sh\nprintf raw > route_raw.txt\n"); err != nil {
		t.Fatalf("write configure script: %v", err)
	}

	config.PigstyHome = home
	pigstyConfRaw = false
	pigstyConfName = "meta"
	pigstyConfRegion = "default"
	pigstyConfSkip = true
	pigstyConfNonInteractive = true
	pigstyConfOutput = "native.yml"

	if err := pigstyConfCmd.RunE(pigstyConfCmd, nil); err != nil {
		t.Fatalf("pig sty conf failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(home, "route_raw.txt")); err == nil {
		t.Fatal("raw configure script should not be called without --raw")
	}
	if _, err := os.Stat(filepath.Join(home, "native.yml")); err != nil {
		t.Fatalf("expected native output file: %v", err)
	}
}

func TestStyConfRawFlagUsesRawRoute(t *testing.T) {
	restore := saveStyConfState(t)
	defer restore()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	home := t.TempDir()
	if err := writeExecutable(filepath.Join(home, "configure"), "#!/bin/sh\nprintf raw > route_raw.txt\n"); err != nil {
		t.Fatalf("write configure script: %v", err)
	}

	config.PigstyHome = home
	pigstyConfRaw = true

	if err := pigstyConfCmd.RunE(pigstyConfCmd, nil); err != nil {
		t.Fatalf("pig sty conf --raw failed: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(home, "route_raw.txt"))
	if err != nil {
		t.Fatalf("expected raw marker file: %v", err)
	}
	if strings.TrimSpace(string(got)) != "raw" {
		t.Fatalf("unexpected raw marker content: %q", string(got))
	}
}

func writeExecutable(path, content string) error {
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		return err
	}
	return os.Chmod(path, 0o755)
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
	origRaw := pigstyConfRaw

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
		pigstyConfRaw = origRaw
	}
}
