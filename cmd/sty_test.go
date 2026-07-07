package cmd

import (
	"os"
	"path/filepath"
	stycli "pig/cli/sty"
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

func TestStyConfOutputFileFlagKeepsGlobalOutputFlag(t *testing.T) {
	flag := pigstyConfCmd.Flags().Lookup("output-file")
	if flag == nil {
		t.Fatal("expected --output-file flag on pig sty conf")
	}
	if flag.Shorthand != "O" {
		t.Fatalf("expected shorthand -O for output-file, got -%s", flag.Shorthand)
	}
	if local := pigstyConfCmd.Flags().Lookup("output"); local != nil {
		t.Fatalf("did not expect local --output flag on pig sty conf, got %+v", local)
	}
	if inherited := pigstyConfCmd.InheritedFlags().Lookup("output"); inherited == nil {
		t.Fatal("expected inherited global --output flag on pig sty conf")
	}
}

func TestStyInitMirrorFlagPassesMirrorOption(t *testing.T) {
	flag := pigstyInitCmd.Flags().Lookup("mirror")
	if flag == nil {
		t.Fatal("expected --mirror flag on pig sty init")
	}
	if flag.Shorthand != "m" {
		t.Fatalf("expected shorthand -m for mirror, got -%s", flag.Shorthand)
	}
	if flag.Hidden {
		t.Fatal("pig sty init --mirror should be visible")
	}

	origInitExec := pigstyInitExec
	origMirror := flag.Value.String()
	origPath := pigstyInitPath
	origForce := pigstyInitForce
	origVersion := pigstyVersion
	origDir := pigstyDownloadDir
	defer func() {
		pigstyInitExec = origInitExec
		pigstyInitPath = origPath
		pigstyInitForce = origForce
		pigstyVersion = origVersion
		pigstyDownloadDir = origDir
		_ = flag.Value.Set(origMirror)
	}()

	var got stycli.InitOptions
	pigstyInitExec = func(opts stycli.InitOptions) error {
		got = opts
		return nil
	}

	pigstyInitPath = "/tmp/pigsty"
	pigstyInitForce = true
	pigstyVersion = "v1.2.3"
	pigstyDownloadDir = "/tmp"
	if err := flag.Value.Set("true"); err != nil {
		t.Fatalf("set --mirror: %v", err)
	}

	if err := pigstyInitCmd.RunE(pigstyInitCmd, nil); err != nil {
		t.Fatalf("pig sty init --mirror failed: %v", err)
	}
	if !got.Mirror {
		t.Fatal("expected InitOptions.Mirror to be true")
	}
}

func TestStyGetMirrorFlagPassesMirrorOption(t *testing.T) {
	flag := pigstyGetcmd.Flags().Lookup("mirror")
	if flag == nil {
		t.Fatal("expected --mirror flag on pig sty get")
	}
	if flag.Shorthand != "m" {
		t.Fatalf("expected shorthand -m for mirror, got -%s", flag.Shorthand)
	}
	if flag.Hidden {
		t.Fatal("pig sty get --mirror should be visible")
	}

	origGetExec := pigstyGetExec
	origMirror := flag.Value.String()
	origVersion := pigstyVersion
	origDir := pigstyDownloadDir
	defer func() {
		pigstyGetExec = origGetExec
		pigstyVersion = origVersion
		pigstyDownloadDir = origDir
		_ = flag.Value.Set(origMirror)
	}()

	var got stycli.DownloadOptions
	pigstyGetExec = func(opts stycli.DownloadOptions) error {
		got = opts
		return nil
	}

	pigstyVersion = "v1.2.3"
	pigstyDownloadDir = "/tmp"
	if err := flag.Value.Set("true"); err != nil {
		t.Fatalf("set --mirror: %v", err)
	}

	if err := pigstyGetcmd.RunE(pigstyGetcmd, []string{"v1.2.3"}); err != nil {
		t.Fatalf("pig sty get --mirror failed: %v", err)
	}
	if !got.Mirror {
		t.Fatal("expected DownloadOptions.Mirror to be true")
	}
}

func TestStyListMirrorFlagPassesMirrorOption(t *testing.T) {
	flag := pigstyListcmd.Flags().Lookup("mirror")
	if flag == nil {
		t.Fatal("expected --mirror flag on pig sty list")
	}
	if flag.Shorthand != "m" {
		t.Fatalf("expected shorthand -m for mirror, got -%s", flag.Shorthand)
	}
	if flag.Hidden {
		t.Fatal("pig sty list --mirror should be visible")
	}

	origListExec := pigstyListExec
	origMirror := flag.Value.String()
	defer func() {
		pigstyListExec = origListExec
		_ = flag.Value.Set(origMirror)
	}()

	var got stycli.ListOptions
	pigstyListExec = func(opts stycli.ListOptions) error {
		got = opts
		return nil
	}

	if err := flag.Value.Set("true"); err != nil {
		t.Fatalf("set --mirror: %v", err)
	}

	if err := pigstyListcmd.RunE(pigstyListcmd, []string{"v3"}); err != nil {
		t.Fatalf("pig sty list --mirror failed: %v", err)
	}
	if !got.Mirror {
		t.Fatal("expected ListOptions.Mirror to be true")
	}
}

func TestStyBootMirrorFlagSetsRegionChina(t *testing.T) {
	flag := pigstyBootCmd.Flags().Lookup("mirror")
	if flag == nil {
		t.Fatal("expected --mirror flag on pig sty boot")
	}
	if flag.Shorthand != "m" {
		t.Fatalf("expected shorthand -m for mirror, got -%s", flag.Shorthand)
	}
	if flag.Hidden {
		t.Fatal("pig sty boot --mirror should be visible")
	}

	origMirror := flag.Value.String()
	origHome := config.PigstyHome
	origRegion := pigstyBootRegion
	origPackage := pigstyBootPackage
	origKeep := pigstyBootKeep
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() {
		config.PigstyHome = origHome
		pigstyBootRegion = origRegion
		pigstyBootPackage = origPackage
		pigstyBootKeep = origKeep
		_ = flag.Value.Set(origMirror)
		_ = os.Chdir(cwd)
	}()

	home := t.TempDir()
	if err := writeExecutable(filepath.Join(home, "bootstrap"), "#!/bin/sh\nprintf '%s\\n' \"$@\" > bootstrap.args\n"); err != nil {
		t.Fatalf("write bootstrap script: %v", err)
	}

	config.PigstyHome = home
	pigstyBootRegion = "europe"
	pigstyBootPackage = ""
	pigstyBootKeep = false
	if err := flag.Value.Set("true"); err != nil {
		t.Fatalf("set --mirror: %v", err)
	}

	if err := pigstyBootCmd.RunE(pigstyBootCmd, nil); err != nil {
		t.Fatalf("pig sty boot --mirror failed: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(home, "bootstrap.args"))
	if err != nil {
		t.Fatalf("read bootstrap args: %v", err)
	}
	if fields := strings.Fields(string(got)); strings.Join(fields, " ") != "-r china" {
		t.Fatalf("bootstrap args = %q, want -r china", strings.TrimSpace(string(got)))
	}
}

func TestStyConfMirrorFlagSetsRegionChina(t *testing.T) {
	flag := pigstyConfCmd.Flags().Lookup("mirror")
	if flag == nil {
		t.Fatal("expected --mirror flag on pig sty conf")
	}
	if flag.Shorthand != "m" {
		t.Fatalf("expected shorthand -m for mirror, got -%s", flag.Shorthand)
	}
	if flag.Hidden {
		t.Fatal("pig sty conf --mirror should be visible")
	}

	restore := saveStyConfState(t)
	origMirror := flag.Value.String()
	defer func() {
		restore()
		_ = flag.Value.Set(origMirror)
	}()

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

	config.PigstyHome = home
	pigstyConfRaw = false
	pigstyConfName = "meta"
	pigstyConfRegion = "europe"
	pigstyConfSkip = true
	pigstyConfNonInteractive = true
	pigstyConfOutput = "mirror.yml"
	if err := flag.Value.Set("true"); err != nil {
		t.Fatalf("set --mirror: %v", err)
	}

	if err := pigstyConfCmd.RunE(pigstyConfCmd, nil); err != nil {
		t.Fatalf("pig sty conf --mirror failed: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(home, "mirror.yml"))
	if err != nil {
		t.Fatalf("read generated config: %v", err)
	}
	if !strings.Contains(string(got), "region: china") {
		t.Fatalf("expected mirror to set region china, got:\n%s", string(got))
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
	origMirror := pigstyConfMirror

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
		pigstyConfMirror = origMirror
	}
}
