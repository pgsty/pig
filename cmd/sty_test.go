package cmd

import (
	"bytes"
	"github.com/spf13/cobra"
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

func TestStyInventoryCommandsAreRemoved(t *testing.T) {
	for _, args := range [][]string{
		{"sty", "edit"}, {"sty", "e"}, {"sty", "check"}, {"sty", "ck"},
		{"sty", "inventory"}, {"sty", "inv"}, {"sty", "cmdb"},
		{"sty", "validate"}, {"sty", "val"}, {"sty", "v"},
	} {
		if found, remaining, err := rootCmd.Find(args); err == nil && found != nil && len(remaining) == 0 {
			t.Fatalf("removed Sty Inventory command remains registered: %v -> %s", args, found.CommandPath())
		}
	}
	for _, argument := range []string{"edit", "e", "check", "ck", "inventory", "inv", "cmdb", "validate", "val", "v"} {
		if err := styCmd.ValidateArgs([]string{argument}); err == nil {
			t.Fatalf("removed sty command %q is still accepted as a positional argument", argument)
		}
	}
}

func TestStyOperationalClaimsMatchExecutionSemantics(t *testing.T) {
	for _, test := range []struct {
		command                 *cobra.Command
		parallel, risk, confirm string
	}{
		{pigstyBootCmd, "unsafe", "medium", "none"},
		{pigstyConfCmd, "unsafe", "medium", "recommended"},
		{pigstyGrafanaDumpCmd, "restricted", "low", "none"},
		{pigstyGetcmd, "restricted", "low", "none"},
		{pigstyReleaseGetCmd, "restricted", "low", "none"},
		{pigstyDeployCmd, "unsafe", "high", "none"},
	} {
		annotations := test.command.Annotations
		if annotations["parallel"] != test.parallel || annotations["risk"] != test.risk || annotations["confirm"] != test.confirm {
			t.Errorf("%s annotations = %v", test.command.CommandPath(), annotations)
		}
	}
	if pigstyDeployCmd.Annotations["consent"] != "invocation" {
		t.Fatalf("deploy consent = %q, want invocation", pigstyDeployCmd.Annotations["consent"])
	}
	if !strings.Contains(pigstyDeployCmd.Long, "starts immediately") || !strings.Contains(styCmd.Long, "release get") {
		t.Fatal("Sty help does not describe immediate deploy and canonical release commands")
	}
}

func TestStyDeployInvokesExecutorImmediately(t *testing.T) {
	originalExec := pigstyDeployExec
	originalFormat := config.OutputFormat
	defer func() {
		pigstyDeployExec = originalExec
		config.OutputFormat = originalFormat
	}()
	calls := 0
	pigstyDeployExec = func(stycli.DeployOptions) error {
		calls++
		return nil
	}
	config.OutputFormat = config.OUTPUT_TEXT
	if err := pigstyDeployCmd.RunE(pigstyDeployCmd, nil); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("deploy executor calls = %d, want immediate single execution", calls)
	}

	config.OutputFormat = config.OUTPUT_JSON
	raw := captureStdout(t, func() {
		if err := pigstyDeployCmd.RunE(pigstyDeployCmd, nil); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(raw, `"kind":"pig.sty.deploy/v1"`) {
		t.Fatalf("structured deploy payload = %q", raw)
	}
}

func TestStyMirrorAndRegionFlagsAreMutuallyExclusive(t *testing.T) {
	const annotation = "cobra_annotation_mutually_exclusive"
	for _, command := range []*cobra.Command{pigstyBootCmd, pigstyConfCmd} {
		for _, name := range []string{"mirror", "region"} {
			flag := command.Flags().Lookup(name)
			if flag == nil || !strings.Contains(strings.Join(flag.Annotations[annotation], " "), "mirror region") {
				t.Fatalf("%s --%s lacks mirror/region exclusion: %+v", command.CommandPath(), name, flag)
			}
		}
	}
}

func TestStyInitAcceptEULAFlagPassesOption(t *testing.T) {
	flag := pigstyInitCmd.Flags().Lookup("accept-eula")
	if flag == nil {
		t.Fatal("pig sty init missing --accept-eula")
	}
	originalExec, originalValue := pigstyInitExec, pigstyInitEULA
	defer func() {
		pigstyInitExec, pigstyInitEULA = originalExec, originalValue
		_ = flag.Value.Set("false")
	}()
	var got stycli.InitOptions
	pigstyInitExec = func(options stycli.InitOptions) error { got = options; return nil }
	if err := flag.Value.Set("true"); err != nil {
		t.Fatal(err)
	}
	if err := pigstyInitCmd.RunE(pigstyInitCmd, nil); err != nil {
		t.Fatal(err)
	}
	if !got.AcceptEULA {
		t.Fatal("--accept-eula was not passed to InitOptions")
	}
}

func TestStyReleaseNamespaceAndListDeprecation(t *testing.T) {
	for _, test := range []struct {
		args []string
		want *cobra.Command
	}{
		{args: []string{"sty", "release"}, want: pigstyReleaseCmd},
		{args: []string{"sty", "release", "list"}, want: pigstyReleaseListCmd},
		{args: []string{"sty", "release", "ls"}, want: pigstyReleaseListCmd},
		{args: []string{"sty", "release", "get"}, want: pigstyReleaseGetCmd},
		{args: []string{"sty", "list"}, want: pigstyListcmd},
		{args: []string{"sty", "ls"}, want: pigstyListcmd},
		{args: []string{"sty", "get"}, want: pigstyGetcmd},
	} {
		found, _, err := rootCmd.Find(test.args)
		if err != nil {
			t.Fatalf("Find(%v): %v", test.args, err)
		}
		if found != test.want {
			t.Fatalf("Find(%v) = %v, want %v", test.args, found, test.want)
		}
	}
	for _, command := range []*cobra.Command{pigstyGetcmd, pigstyReleaseGetCmd} {
		for _, flag := range []string{"version", "dir", "mirror"} {
			if command.Flags().Lookup(flag) == nil {
				t.Fatalf("%s missing --%s", command.CommandPath(), flag)
			}
		}
	}

	originalExec := pigstyListExec
	originalFormat := config.OutputFormat
	defer func() {
		pigstyListExec = originalExec
		config.OutputFormat = originalFormat
	}()
	pigstyListExec = func(stycli.ListOptions) error { return nil }

	config.OutputFormat = config.OUTPUT_TEXT
	var legacyErr bytes.Buffer
	pigstyListcmd.SetErr(&legacyErr)
	defer pigstyListcmd.SetErr(os.Stderr)
	if err := pigstyListcmd.RunE(pigstyListcmd, nil); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(legacyErr.String(), "Deprecated:"); got != 1 {
		t.Fatalf("legacy list deprecation count = %d in %q", got, legacyErr.String())
	}

	var canonicalErr bytes.Buffer
	pigstyReleaseListCmd.SetErr(&canonicalErr)
	defer pigstyReleaseListCmd.SetErr(os.Stderr)
	if err := pigstyReleaseListCmd.RunE(pigstyReleaseListCmd, nil); err != nil {
		t.Fatal(err)
	}
	if canonicalErr.Len() != 0 {
		t.Fatalf("canonical list wrote deprecation: %q", canonicalErr.String())
	}

	config.OutputFormat = config.OUTPUT_JSON
	legacyErr.Reset()
	raw := captureStdout(t, func() {
		if err := pigstyListcmd.RunE(pigstyListcmd, nil); err != nil {
			t.Fatal(err)
		}
	})
	if legacyErr.Len() != 0 || strings.Contains(raw, "Deprecated:") || !strings.Contains(raw, `"command":"pig sty list"`) || strings.Contains(raw, `"kind":`) {
		t.Fatalf("structured legacy list stdout=%q stderr=%q", raw, legacyErr.String())
	}

	raw = captureStdout(t, func() {
		if err := pigstyReleaseListCmd.RunE(pigstyReleaseListCmd, nil); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(raw, `"command":"pig sty release list"`) || !strings.Contains(raw, `"kind":"pig.sty.release.list/v1"`) {
		t.Fatalf("structured canonical list = %q", raw)
	}
}

func TestStyReleaseGetDelegateUsesIdenticalOptions(t *testing.T) {
	originalExec := pigstyGetExec
	originalFormat := config.OutputFormat
	originalVersion, originalDir, originalMirror := pigstyVersion, pigstyDownloadDir, pigstyGetMirror
	defer func() {
		pigstyGetExec = originalExec
		config.OutputFormat = originalFormat
		pigstyVersion, pigstyDownloadDir, pigstyGetMirror = originalVersion, originalDir, originalMirror
	}()
	config.OutputFormat = config.OUTPUT_TEXT
	pigstyVersion, pigstyDownloadDir, pigstyGetMirror = "v4.4.0", "/tmp/releases", true
	var calls []stycli.DownloadOptions
	pigstyGetExec = func(opts stycli.DownloadOptions) error {
		calls = append(calls, opts)
		return nil
	}
	var legacyErr bytes.Buffer
	pigstyGetcmd.SetErr(&legacyErr)
	defer pigstyGetcmd.SetErr(os.Stderr)
	if err := pigstyGetcmd.RunE(pigstyGetcmd, []string{"v4"}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(legacyErr.String(), "Deprecated:"); got != 1 {
		t.Fatalf("legacy get deprecation count = %d in %q", got, legacyErr.String())
	}
	var canonicalErr bytes.Buffer
	pigstyReleaseGetCmd.SetErr(&canonicalErr)
	defer pigstyReleaseGetCmd.SetErr(os.Stderr)
	if err := pigstyReleaseGetCmd.RunE(pigstyReleaseGetCmd, []string{"v4"}); err != nil {
		t.Fatal(err)
	}
	if canonicalErr.Len() != 0 {
		t.Fatalf("canonical get wrote deprecation: %q", canonicalErr.String())
	}
	if len(calls) != 2 || calls[0].Version != calls[1].Version || calls[0].DownloadDir != calls[1].DownloadDir || calls[0].Mirror != calls[1].Mirror || strings.Join(calls[0].Args, "\x00") != strings.Join(calls[1].Args, "\x00") {
		t.Fatalf("release get delegate calls = %+v", calls)
	}

	config.OutputFormat = config.OUTPUT_JSON
	legacyErr.Reset()
	legacy := captureStdout(t, func() {
		if err := pigstyGetcmd.RunE(pigstyGetcmd, nil); err != nil {
			t.Fatal(err)
		}
	})
	canonical := captureStdout(t, func() {
		if err := pigstyReleaseGetCmd.RunE(pigstyReleaseGetCmd, nil); err != nil {
			t.Fatal(err)
		}
	})
	if legacyErr.Len() != 0 || strings.Contains(legacy, "Deprecated:") || !strings.Contains(legacy, `"command":"pig sty get"`) || !strings.Contains(legacy, `"kind":"pig.sty.release.get/v1"`) {
		t.Fatalf("legacy release get stdout=%q stderr=%q", legacy, legacyErr.String())
	}
	if !strings.Contains(canonical, `"command":"pig sty release get"`) || !strings.Contains(canonical, `"kind":"pig.sty.release.get/v1"`) {
		t.Fatalf("canonical release get payload = %q", canonical)
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
    # docker_registry_mirrors: ["https://docker.1ms.run"]
    # PIP_MIRROR_URL: "https://pypi.tuna.tsinghua.edu.cn/simple"
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
	for _, declaration := range []string{"docker_registry_mirrors:", "PIP_MIRROR_URL:"} {
		if !strings.Contains(string(got), "    "+declaration) || strings.Contains(string(got), "# "+declaration) {
			t.Fatalf("expected mirror to activate %s, got:\n%s", declaration, string(got))
		}
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

func TestStyConfRawStructuredOutputRetainsGeneratedSecrets(t *testing.T) {
	restore := saveStyConfState(t)
	originalFormat := config.OutputFormat
	defer func() {
		restore()
		config.OutputFormat = originalFormat
	}()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	home := t.TempDir()
	if err := writeExecutable(filepath.Join(home, "configure"), "#!/bin/sh\nprintf 'generated_password=raw-random-secret\\n'\n"); err != nil {
		t.Fatal(err)
	}
	config.PigstyHome = home
	config.OutputFormat = config.OUTPUT_JSON
	pigstyConfRaw = true
	pigstyConfGenerate = true

	raw := captureStdout(t, func() {
		if err := pigstyConfCmd.RunE(pigstyConfCmd, nil); err != nil {
			t.Fatal(err)
		}
	})
	for _, expected := range []string{
		`"kind":"pig.sty.configure.raw/v1"`,
		`"secret_bearing_output":true`,
		`generated_password=raw-random-secret`,
	} {
		if !strings.Contains(raw, expected) {
			t.Fatalf("raw structured configure omitted %q: %s", expected, raw)
		}
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
