package cmd

import (
	"bytes"
	"fmt"
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

func TestStyConfRunsAsCurrentUser(t *testing.T) {
	if got := pigstyConfCmd.Annotations["os_user"]; got != "current" {
		t.Fatalf("pig sty conf os_user = %q, want current", got)
	}
}

func TestStyConfAcceptsOptionalModeArgument(t *testing.T) {
	if err := pigstyConfCmd.ValidateArgs(nil); err != nil {
		t.Fatalf("pig sty conf rejected zero arguments: %v", err)
	}
	if err := pigstyConfCmd.ValidateArgs([]string{"rich"}); err != nil {
		t.Fatalf("pig sty conf rejected one mode argument: %v", err)
	}
	if err := pigstyConfCmd.ValidateArgs([]string{"rich", "extra"}); err == nil {
		t.Fatal("pig sty conf accepted more than one mode argument")
	}
}

func TestResolveStyConfigureMode(t *testing.T) {
	tests := []struct {
		name     string
		flagMode string
		args     []string
		want     string
		wantErr  bool
	}{
		{name: "default"},
		{name: "flag", flagMode: " rich ", want: "rich"},
		{name: "positional", args: []string{" ha/full "}, want: "ha/full"},
		{name: "both", flagMode: "rich", args: []string{"meta"}, wantErr: true},
		{name: "too many", args: []string{"rich", "meta"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveStyConfigureMode(tt.flagMode, tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("resolveStyConfigureMode() = %q, want error", got)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("resolveStyConfigureMode() = %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}

func TestStyDeployHasNoInstallAlias(t *testing.T) {
	if got := strings.Join(pigstyDeployCmd.Aliases, ","); got != "d,de" {
		t.Fatalf("deploy aliases = %q, want d,de", got)
	}
	for _, spelling := range []string{"install", "ins"} {
		found, _, _ := rootCmd.Find([]string{"sty", spelling})
		if found == pigstyDeployCmd {
			t.Fatalf("removed spelling %q still resolves to deploy", spelling)
		}
		for _, help := range []string{styCmd.Long, styCmd.Example, pigstyDeployCmd.Long, pigstyDeployCmd.Example} {
			if strings.Contains(help, "pig sty "+spelling) {
				t.Fatalf("removed spelling %q remains in Sty help", spelling)
			}
		}
	}
}

func TestStyConfHasNoRawFlag(t *testing.T) {
	if flag := pigstyConfCmd.Flags().Lookup("raw"); flag != nil {
		t.Fatalf("did not expect --raw on pig sty conf, got %+v", flag)
	}
}

func TestStyConfDomainFlagDefaultsToPlaceholder(t *testing.T) {
	flag := pigstyConfCmd.Flags().Lookup("domain")
	if flag == nil {
		t.Fatal("expected --domain flag on pig sty conf")
	}
	if flag.DefValue != stycli.DefaultConfigureDomain {
		t.Fatalf("--domain default = %q, want %q", flag.DefValue, stycli.DefaultConfigureDomain)
	}
	ipFlag := pigstyConfCmd.Flags().Lookup("ip")
	if ipFlag == nil || !strings.Contains(ipFlag.Usage, fmt.Sprintf("%d", stycli.MaxConfigureIPs)) {
		t.Fatalf("--ip usage does not derive its maximum from MaxConfigureIPs: %+v", ipFlag)
	}
}

func TestStyConfStructuredModeConflictUsesParamError(t *testing.T) {
	restore := saveStyConfState(t)
	originalFormat := config.OutputFormat
	defer func() {
		restore()
		config.OutputFormat = originalFormat
	}()

	config.OutputFormat = config.OUTPUT_JSON
	pigstyConfName = "rich"
	var runErr error
	raw := captureStdout(t, func() {
		runErr = pigstyConfCmd.RunE(pigstyConfCmd, []string{"meta"})
	})
	if runErr == nil {
		t.Fatal("expected structured positional/flag mode conflict")
	}
	for _, expected := range []string{
		`"message":"invalid configuration mode"`,
		`"command":"pig sty conf"`,
		"either positionally or with --conf",
	} {
		if !strings.Contains(raw, expected) {
			t.Fatalf("structured mode conflict omitted %q: %s", expected, raw)
		}
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
	if !strings.Contains(pigstyDeployCmd.Long, "starts immediately") || !strings.Contains(styCmd.Long, "pig sty get") {
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
	for _, name := range []string{"skip", "ip"} {
		flag := pigstyConfCmd.Flags().Lookup(name)
		if flag == nil || !strings.Contains(strings.Join(flag.Annotations[annotation], " "), "skip ip") {
			t.Fatalf("%s --%s lacks skip/ip exclusion: %+v", pigstyConfCmd.CommandPath(), name, flag)
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

func TestStyListAndGetAreCanonicalTopLevelCommands(t *testing.T) {
	for _, test := range []struct {
		args []string
		want *cobra.Command
	}{
		{args: []string{"sty", "list"}, want: pigstyListcmd},
		{args: []string{"sty", "ls"}, want: pigstyListcmd},
		{args: []string{"sty", "get"}, want: pigstyGetcmd},
		{args: []string{"sty", "g"}, want: pigstyGetcmd},
		{args: []string{"sty", "download"}, want: pigstyGetcmd},
	} {
		found, _, err := rootCmd.Find(test.args)
		if err != nil {
			t.Fatalf("Find(%v): %v", test.args, err)
		}
		if found != test.want {
			t.Fatalf("Find(%v) = %v, want %v", test.args, found, test.want)
		}
	}
	if found, remaining, err := rootCmd.Find([]string{"sty", "release"}); err == nil && found != nil && len(remaining) == 0 {
		t.Fatalf("removed release namespace remains registered: %s", found.CommandPath())
	}
	for _, flag := range []string{"version", "dir", "mirror"} {
		if pigstyGetcmd.Flags().Lookup(flag) == nil {
			t.Fatalf("%s missing --%s", pigstyGetcmd.CommandPath(), flag)
		}
	}

	originalExec := pigstyListExec
	originalFormat := config.OutputFormat
	defer func() {
		pigstyListExec = originalExec
		config.OutputFormat = originalFormat
	}()
	pigstyListExec = func(stycli.ListOptions) error { return nil }

	var stderr bytes.Buffer
	pigstyListcmd.SetErr(&stderr)
	defer pigstyListcmd.SetErr(os.Stderr)
	config.OutputFormat = config.OUTPUT_TEXT
	if err := pigstyListcmd.RunE(pigstyListcmd, nil); err != nil {
		t.Fatal(err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("canonical list wrote stderr: %q", stderr.String())
	}

	config.OutputFormat = config.OUTPUT_JSON
	raw := captureStdout(t, func() {
		if err := pigstyListcmd.RunE(pigstyListcmd, nil); err != nil {
			t.Fatal(err)
		}
	})
	if stderr.Len() != 0 || strings.Contains(raw, "Deprecated:") || !strings.Contains(raw, `"command":"pig sty list"`) || strings.Contains(raw, `"kind":`) {
		t.Fatalf("structured list stdout=%q stderr=%q", raw, stderr.String())
	}
}

func TestStyGetUsesTopLevelContract(t *testing.T) {
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
	var got stycli.DownloadOptions
	pigstyGetExec = func(opts stycli.DownloadOptions) error {
		got = opts
		return nil
	}
	var stderr bytes.Buffer
	pigstyGetcmd.SetErr(&stderr)
	defer pigstyGetcmd.SetErr(os.Stderr)
	if err := pigstyGetcmd.RunE(pigstyGetcmd, []string{"v4"}); err != nil {
		t.Fatal(err)
	}
	if got.Version != pigstyVersion || got.DownloadDir != pigstyDownloadDir || !got.Mirror || strings.Join(got.Args, "\x00") != "v4" {
		t.Fatalf("get options = %+v", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("canonical get wrote stderr: %q", stderr.String())
	}

	config.OutputFormat = config.OUTPUT_JSON
	raw := captureStdout(t, func() {
		if err := pigstyGetcmd.RunE(pigstyGetcmd, nil); err != nil {
			t.Fatal(err)
		}
	})
	if stderr.Len() != 0 || strings.Contains(raw, "Deprecated:") || !strings.Contains(raw, `"command":"pig sty get"`) || strings.Contains(raw, `"kind":`) {
		t.Fatalf("structured get stdout=%q stderr=%q", raw, stderr.String())
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

func TestStyBootMirrorFlagIsVisible(t *testing.T) {
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

func TestStyConfPositionalModeUsesNativeRoute(t *testing.T) {
	restore := saveStyConfState(t)
	defer restore()

	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "conf"), 0o755); err != nil {
		t.Fatalf("mkdir conf: %v", err)
	}
	template := `all:
  vars:
    admin_ip: 10.10.10.10
    home_domain: i.pigsty
    region: default
    node_tune: oltp
    pg_version: 18
    pg_conf: oltp.yml
`
	if err := os.WriteFile(filepath.Join(home, "conf", "rich.yml"), []byte(template), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}
	if err := writeExecutable(filepath.Join(home, "configure"), "#!/bin/sh\nprintf raw > route_raw.txt\n"); err != nil {
		t.Fatalf("write configure script: %v", err)
	}

	config.PigstyHome = home
	pigstyConfName = ""
	pigstyConfDomain = "infra.example.com"
	pigstyConfRegion = "default"
	pigstyConfSkip = true
	pigstyConfNonInteractive = true
	pigstyConfOutput = "native.yml"

	if err := pigstyConfCmd.RunE(pigstyConfCmd, []string{"rich"}); err != nil {
		t.Fatalf("pig sty conf failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(home, "route_raw.txt")); err == nil {
		t.Fatal("legacy configure script should not be called")
	}
	rendered, err := os.ReadFile(filepath.Join(home, "native.yml"))
	if err != nil {
		t.Fatalf("expected native output file: %v", err)
	}
	if !strings.Contains(string(rendered), "home_domain: infra.example.com") {
		t.Fatalf("native --domain replacement missing:\n%s", rendered)
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
	origDomain := pigstyConfDomain
	origVer := pigstyConfVer
	origRegion := pigstyConfRegion
	origOutput := pigstyConfOutput
	origSkip := pigstyConfSkip
	origProxy := pigstyConfProxy
	origNonInteractive := pigstyConfNonInteractive
	origPort := pigstyConfPort
	origGenerate := pigstyConfGenerate
	origMirror := pigstyConfMirror

	return func() {
		config.PigstyHome = origHome
		pigstyConfName = origName
		pigstyConfIP = origIP
		pigstyConfDomain = origDomain
		pigstyConfVer = origVer
		pigstyConfRegion = origRegion
		pigstyConfOutput = origOutput
		pigstyConfSkip = origSkip
		pigstyConfProxy = origProxy
		pigstyConfNonInteractive = origNonInteractive
		pigstyConfPort = origPort
		pigstyConfGenerate = origGenerate
		pigstyConfMirror = origMirror
	}
}
