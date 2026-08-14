package sty

import (
	"bytes"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"pig/cli/get"
	"pig/internal/config"
	"pig/internal/output"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCheckPackageManagerWarningsDebWithoutManager(t *testing.T) {
	restore := savePreflightHooks()
	defer restore()

	goosFn = func() string { return "linux" }
	config.OSType = config.DistroDEB
	lookPathFn = func(name string) (string, error) {
		if name == "dpkg" {
			return "/usr/bin/dpkg", nil
		}
		return "", exec.ErrNotFound
	}

	_, err := checkPackageManagerWarnings()
	if err == nil {
		t.Fatal("expected error when deb package manager cannot be determined")
	}
}

func TestCheckPackageManagerWarningsEL8RequiresDNF(t *testing.T) {
	restore := savePreflightHooks()
	defer restore()

	goosFn = func() string { return "linux" }
	config.OSType = config.DistroEL
	config.OSMajor = 8
	lookPathFn = func(name string) (string, error) {
		if name == "yum" || name == "rpm" {
			return "/usr/bin/" + name, nil
		}
		return "", exec.ErrNotFound
	}

	if _, err := checkPackageManagerWarnings(); err == nil {
		t.Fatal("expected EL8 preflight to reject yum-only environment")
	}
}

func TestCheckPackageManagerWarningsEL7AllowsYum(t *testing.T) {
	restore := savePreflightHooks()
	defer restore()

	goosFn = func() string { return "linux" }
	config.OSType = config.DistroEL
	config.OSMajor = 7
	lookPathFn = func(name string) (string, error) {
		if name == "yum" || name == "rpm" {
			return "/usr/bin/" + name, nil
		}
		return "", exec.ErrNotFound
	}

	if _, err := checkPackageManagerWarnings(); err != nil {
		t.Fatalf("expected EL7 preflight to allow yum: %v", err)
	}
}

func TestCheckLocalSSHWarningsUsesPort(t *testing.T) {
	restore := savePreflightHooks()
	defer restore()

	goosFn = func() string { return "linux" }
	lookPathFn = func(name string) (string, error) {
		if name == "ssh" {
			return "/usr/bin/ssh", nil
		}
		return "", exec.ErrNotFound
	}
	currentUserFn = func() string { return "alice" }

	var calledName string
	var calledArgs []string
	runCommandFn = func(timeout time.Duration, name string, args ...string) error {
		calledName = name
		calledArgs = append([]string(nil), args...)
		return nil
	}

	warnings := checkLocalSSHWarnings("2222")
	if len(warnings) > 0 {
		t.Fatalf("expected no warnings, got: %v", warnings)
	}
	if calledName != "ssh" {
		t.Fatalf("expected ssh command, got %q", calledName)
	}
	if !containsArgPair(calledArgs, "-p", "2222") {
		t.Fatalf("expected -p 2222 in ssh args: %v", calledArgs)
	}
	if !containsArg(calledArgs, "alice@127.0.0.1") {
		t.Fatalf("expected target alice@127.0.0.1 in ssh args: %v", calledArgs)
	}
}

func TestCheckAdminWarningsSkipNoCommandRun(t *testing.T) {
	restore := savePreflightHooks()
	defer restore()

	goosFn = func() string { return "linux" }
	runCalled := false
	runCommandFn = func(timeout time.Duration, name string, args ...string) error {
		runCalled = true
		return nil
	}

	warnings := checkAdminWarnings("meta", "10.10.10.10", true, "")
	if runCalled {
		t.Fatal("admin ssh check should not run when --skip is set")
	}
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "--skip") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected skip warning, got: %v", warnings)
	}
}

func TestConfigureNativeCarriesSSHPort(t *testing.T) {
	restore := savePreflightHooks()
	defer restore()

	goosFn = func() string { return "darwin" }
	regionDetectFn = func() string { return "default" }

	tmp := t.TempDir()
	confDir := filepath.Join(tmp, "conf")
	if err := os.MkdirAll(confDir, 0755); err != nil {
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
	if err := os.WriteFile(filepath.Join(confDir, "meta.yml"), []byte(template), 0644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	result := ConfigureNative(ConfigureOptions{
		PigstyHome:       tmp,
		Mode:             "meta",
		PrimaryIP:        "10.10.10.10",
		Region:           "default",
		OutputFile:       "with-port.yml",
		SSHPort:          "2222",
		NonInteractive:   true,
		DisablePreflight: true,
	})
	if result == nil || !result.Success {
		t.Fatalf("expected success result, got: %+v", result)
	}
	data, ok := result.Data.(*ConfigureData)
	if !ok {
		t.Fatalf("expected ConfigureData, got %T", result.Data)
	}
	if data.SSHPort != "2222" {
		t.Fatalf("unexpected ssh_port: %q", data.SSHPort)
	}
}

func TestDetectRegionFromNetworkConditionUsesSource(t *testing.T) {
	restore := savePreflightHooks()
	defer restore()

	networkConditionFn = func() string { return get.ViaCC }
	internetAccessFn = func() bool { return true }
	networkSourceFn = func() string { return get.ViaCC }
	networkRegionFn = func() string { return "" }
	if region := detectRegionFromNetworkCondition(); region != "china" {
		t.Fatalf("expected china region from ViaCC, got %q", region)
	}

	networkConditionFn = func() string { return get.ViaIO }
	networkSourceFn = func() string { return get.ViaIO }
	networkRegionFn = func() string { return "" }
	if region := detectRegionFromNetworkCondition(); region != "default" {
		t.Fatalf("expected default region from ViaIO, got %q", region)
	}

	networkConditionFn = func() string { return get.ViaNA }
	internetAccessFn = func() bool { return false }
	if region := detectRegionFromNetworkCondition(); region != "default" {
		t.Fatalf("expected default region with no internet, got %q", region)
	}
}

func TestResolvePrimaryIPInteractivePrompt(t *testing.T) {
	restore := savePreflightHooks()
	defer restore()
	t.Setenv("PIG_NON_INTERACTIVE", "")

	goosFn = func() string { return "linux" }
	localIPv4CandidatesFn = func() []localIPv4Candidate {
		return []localIPv4Candidate{
			{IP: "10.0.0.2", Interfaces: []string{"eth0"}},
			{IP: "10.0.0.3", Interfaces: []string{"eth1"}},
		}
	}
	inReader = strings.NewReader("10.0.0.8\n")
	var errBuf bytes.Buffer
	errWriter = &errBuf

	ip, warnings, err := resolvePrimaryIPForTest(ConfigureOptions{NonInteractive: false}, "meta")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "10.0.0.8" {
		t.Fatalf("expected interactive input ip, got %q", ip)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if !strings.Contains(errBuf.String(), "INPUT primary_ip") {
		t.Fatalf("expected prompt output, got %q", errBuf.String())
	}
	for _, expected := range []string{"Multiple IP address candidates found:", "(1) 10.0.0.2", "eth0", "(2) 10.0.0.3", "eth1"} {
		if !strings.Contains(errBuf.String(), expected) {
			t.Fatalf("expected candidate output %q, got %q", expected, errBuf.String())
		}
	}
}

func TestResolvePrimaryIPInteractiveSelectionNumber(t *testing.T) {
	restore := savePreflightHooks()
	defer restore()
	t.Setenv("PIG_NON_INTERACTIVE", "")

	goosFn = func() string { return "linux" }
	localIPv4CandidatesFn = func() []localIPv4Candidate {
		return []localIPv4Candidate{
			{IP: "10.0.0.2", Interfaces: []string{"eth0"}},
			{IP: "10.0.0.3", Interfaces: []string{"eth1"}},
		}
	}
	inReader = strings.NewReader("2\n")
	var errBuf bytes.Buffer
	errWriter = &errBuf

	ip, warnings, err := resolvePrimaryIPForTest(ConfigureOptions{}, "meta")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "10.0.0.3" {
		t.Fatalf("expected candidate 2 IP, got %q", ip)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if !strings.Contains(errBuf.String(), "candidate number [1-2] or IPv4 address") {
		t.Fatalf("expected selection-aware prompt, got %q", errBuf.String())
	}
}

func TestConfigureIPPromptUsesLegacyColors(t *testing.T) {
	style := configureTextStyle{color: true}
	var out bytes.Buffer
	printLocalIPv4Candidates(&out, []localIPv4Candidate{
		{IP: "10.0.0.2", Interfaces: []string{"eth0"}},
	}, style)
	out.WriteString(style.inputPrompt("INPUT primary_ip:"))

	text := out.String()
	for _, expected := range []string{
		configureANSIYellow + "WARN",
		configureANSICyan + "1",
		configureANSIRed + "10.0.0.2",
		configureANSIYellow + "eth0",
		configureANSIMagenta + " IN ",
		configureANSIMagenta + "INPUT primary_ip:\n=> ",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("colored IP prompt omitted %q: %q", expected, text)
		}
	}
}

func TestConfigureNativePrintsKnownValuesBeforeIPPrompt(t *testing.T) {
	restore := savePreflightHooks()
	defer restore()
	t.Setenv("PIG_NON_INTERACTIVE", "")
	originalFormat := config.OutputFormat
	defer func() { config.OutputFormat = originalFormat }()
	t.Setenv("NO_COLOR", "1")
	config.OutputFormat = config.OUTPUT_TEXT

	goosFn = func() string { return "linux" }
	localIPv4CandidatesFn = func() []localIPv4Candidate {
		return []localIPv4Candidate{
			{IP: "10.0.0.2", Interfaces: []string{"eth0"}},
			{IP: "10.0.0.3", Interfaces: []string{"eth1"}},
		}
	}
	inReader = strings.NewReader("1\n")
	var errBuf bytes.Buffer
	errWriter = &errBuf
	progressWriter = &errBuf

	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "conf"), 0755); err != nil {
		t.Fatal(err)
	}
	template := `all:
  vars:
    admin_ip: 10.10.10.10
    region: default
    pg_version: 18
`
	if err := os.WriteFile(filepath.Join(home, "conf", "meta.yml"), []byte(template), 0644); err != nil {
		t.Fatal(err)
	}

	result := ConfigureNative(ConfigureOptions{
		PigstyHome:       home,
		Mode:             "meta",
		Domain:           "infra.example.com",
		PGVersion:        "17",
		SSHPort:          "2222",
		Region:           "default",
		DisablePreflight: true,
	})
	if result == nil || !result.Success {
		t.Fatalf("configure failed: %+v", result)
	}
	progress := errBuf.String()
	begin := strings.Index(progress, "configure pigsty")
	mode := strings.Index(progress, "[ OK ] mode")
	domain := strings.Index(progress, "[ OK ] domain")
	pgVersion := strings.Index(progress, "[ OK ] pg_version = 17")
	sshPort := strings.Index(progress, "[ OK ] ssh_port   = 2222")
	candidates := strings.Index(progress, "Multiple IP address candidates found:")
	prompt := strings.Index(progress, "INPUT primary_ip")
	if begin < 0 || mode < begin || domain < mode || pgVersion < domain || sshPort < pgVersion || candidates < sshPort || prompt < candidates {
		t.Fatalf("known values were not printed before candidate list and prompt:\n%s", progress)
	}

	data, ok := result.Data.(*ConfigureData)
	if !ok {
		t.Fatalf("configure data type = %T", result.Data)
	}
	finalText := data.Text()
	if strings.Contains(finalText, "configure pigsty") || strings.Contains(finalText, "[ OK ] mode") ||
		strings.Contains(finalText, "[ OK ] pg_version") || strings.Contains(finalText, "[ OK ] ssh_port") {
		t.Fatalf("final output repeated the pre-prompt prelude:\n%s", finalText)
	}
	for _, expected := range []string{
		"[ OK ] primary_ip = 10.0.0.2",
		"proceed with:",
		"pig inv edit",
		"pig sty deploy",
	} {
		if !strings.Contains(finalText, expected) {
			t.Fatalf("final output omitted %q:\n%s", expected, finalText)
		}
	}
}

func TestConfigureNativeStructuredOutputNeverPromptsForIP(t *testing.T) {
	restore := savePreflightHooks()
	defer restore()
	originalFormat := config.OutputFormat
	defer func() { config.OutputFormat = originalFormat }()
	t.Setenv("PIG_NON_INTERACTIVE", "")
	config.OutputFormat = config.OUTPUT_JSON

	goosFn = func() string { return "linux" }
	localIPv4CandidatesFn = func() []localIPv4Candidate {
		return []localIPv4Candidate{
			{IP: "10.0.0.2", Interfaces: []string{"eth0"}},
			{IP: "10.0.0.3", Interfaces: []string{"eth1"}},
		}
	}
	inReader = strings.NewReader("1\n")
	var progressBuf bytes.Buffer
	var errBuf bytes.Buffer
	progressWriter = &progressBuf
	errWriter = &errBuf

	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "conf"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "conf", "meta.yml"), []byte("all:\n  vars:\n    admin_ip: 10.10.10.10\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result := ConfigureNative(ConfigureOptions{PigstyHome: home, Mode: "meta", Region: "default", DisablePreflight: true})
	if result == nil || result.Success || result.Code != output.CodeStyConfigureInvalidArgs {
		t.Fatalf("structured multi-IP configure result = %+v", result)
	}
	if !strings.Contains(result.Message, "specify --ip") {
		t.Fatalf("structured multi-IP error = %q", result.Message)
	}
	if progressBuf.Len() != 0 || errBuf.Len() != 0 {
		t.Fatalf("structured configure emitted interactive output: stdout=%q stderr=%q", progressBuf.String(), errBuf.String())
	}
}

func TestResolvePrimaryIPRejectsInvalidSelectionNumber(t *testing.T) {
	restore := savePreflightHooks()
	defer restore()
	t.Setenv("PIG_NON_INTERACTIVE", "")

	goosFn = func() string { return "linux" }
	localIPv4CandidatesFn = func() []localIPv4Candidate {
		return []localIPv4Candidate{
			{IP: "10.0.0.2", Interfaces: []string{"eth0"}},
			{IP: "10.0.0.3", Interfaces: []string{"eth1"}},
		}
	}
	inReader = strings.NewReader("3\n")
	errWriter = io.Discard

	if _, _, err := resolvePrimaryIPForTest(ConfigureOptions{}, "meta"); err == nil || !strings.Contains(err.Error(), "valid range: 1-2") {
		t.Fatalf("expected out-of-range selection error, got %v", err)
	}
}

func TestLocalIPv4CandidatesIncludesInterfacesAndSortsNumerically(t *testing.T) {
	restore := savePreflightHooks()
	defer restore()

	networkInterfacesFn = func() ([]net.Interface, error) {
		return []net.Interface{
			{Index: 1, Name: "lo", Flags: net.FlagUp | net.FlagLoopback},
			{Index: 2, Name: "eth9", Flags: net.FlagUp},
			{Index: 3, Name: "eth1", Flags: net.FlagUp},
			{Index: 4, Name: "down0"},
		}, nil
	}
	interfaceAddrsFn = func(iface net.Interface) ([]net.Addr, error) {
		switch iface.Name {
		case "lo":
			return []net.Addr{
				&net.IPNet{IP: net.ParseIP("127.0.0.1"), Mask: net.CIDRMask(8, 32)},
				&net.IPNet{IP: net.ParseIP("10.0.0.20"), Mask: net.CIDRMask(32, 32)},
			}, nil
		case "eth9":
			return []net.Addr{
				&net.IPNet{IP: net.ParseIP("10.0.0.10"), Mask: net.CIDRMask(24, 32)},
				&net.IPNet{IP: net.ParseIP("10.0.0.2"), Mask: net.CIDRMask(24, 32)},
				&net.IPNet{IP: net.ParseIP("2001:db8::1"), Mask: net.CIDRMask(64, 128)},
			}, nil
		case "eth1":
			return []net.Addr{
				&net.IPAddr{IP: net.ParseIP("10.0.0.2")},
				&net.IPAddr{IP: net.ParseIP("192.168.1.5")},
			}, nil
		default:
			return nil, nil
		}
	}

	got := localIPv4Candidates()
	want := []localIPv4Candidate{
		{IP: "10.0.0.2", Interfaces: []string{"eth1", "eth9"}},
		{IP: "10.0.0.10", Interfaces: []string{"eth9"}},
		{IP: "192.168.1.5", Interfaces: []string{"eth1"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected local IPv4 candidates: got=%v want=%v", got, want)
	}
}

func TestDetectLocaleAvailableFromShellOutput(t *testing.T) {
	restore := savePreflightHooks()
	defer restore()

	shellOutputFn = func(name string, args ...string) (string, error) {
		return "C\nC.UTF-8\nPOSIX\n", nil
	}
	if !detectLocaleAvailable(nil) {
		t.Fatal("expected locale detection to return true with C.UTF-8")
	}
}

func TestDetectLocaleAvailableOverrideAndError(t *testing.T) {
	restore := savePreflightHooks()
	defer restore()

	override := false
	if detectLocaleAvailable(&override) {
		t.Fatal("expected override=false to be respected")
	}

	shellOutputFn = func(name string, args ...string) (string, error) {
		return "", exec.ErrNotFound
	}
	if detectLocaleAvailable(nil) {
		t.Fatal("expected locale detection false on shell command error")
	}
}

func TestDetectRegionFromNetworkConditionPrefersReportedRegion(t *testing.T) {
	restore := savePreflightHooks()
	defer restore()

	networkConditionFn = func() string { return get.ViaCC }
	networkRegionFn = func() string { return "europe" }
	networkSourceFn = func() string { return get.ViaCC }
	internetAccessFn = func() bool { return true }

	if region := detectRegionFromNetworkCondition(); region != "europe" {
		t.Fatalf("expected explicit reported region precedence, got %q", region)
	}
}

func TestResolvePrimaryIPBranches(t *testing.T) {
	restore := savePreflightHooks()
	defer restore()

	goosFn = func() string { return "linux" }

	ip, warnings, err := resolvePrimaryIPForTest(ConfigureOptions{Skip: true}, "meta")
	if err != nil || ip != defaultPrimaryIP || len(warnings) != 0 {
		t.Fatalf("skip branch mismatch: ip=%q warnings=%v err=%v", ip, warnings, err)
	}
	if _, _, err := resolvePrimaryIPForTest(ConfigureOptions{Skip: true, PrimaryIP: "10.0.0.1"}, "meta"); err == nil {
		t.Fatal("expected --skip and --ip conflict")
	}

	if _, _, err := resolvePrimaryIPForTest(ConfigureOptions{PrimaryIP: "bad-ip"}, "meta"); err == nil {
		t.Fatal("expected invalid ip error")
	}

	localIPv4CandidatesFn = func() []localIPv4Candidate { return nil }
	ip, warnings, err = resolvePrimaryIPForTest(ConfigureOptions{}, "meta")
	if err != nil || ip != defaultPrimaryIP {
		t.Fatalf("empty probe fallback mismatch: ip=%q err=%v", ip, err)
	}
	if len(warnings) == 0 {
		t.Fatal("expected warning when probe fails")
	}

	localIPv4CandidatesFn = func() []localIPv4Candidate {
		return []localIPv4Candidate{{IP: "10.0.0.5", Interfaces: []string{"eth0"}}}
	}
	ip, warnings, err = resolvePrimaryIPForTest(ConfigureOptions{}, "meta")
	if err != nil || ip != "10.0.0.5" || len(warnings) != 0 {
		t.Fatalf("single ip branch mismatch: ip=%q warnings=%v err=%v", ip, warnings, err)
	}

	localIPv4CandidatesFn = func() []localIPv4Candidate {
		return []localIPv4Candidate{{IP: "10.0.0.2", Interfaces: []string{"eth0"}}, {IP: "10.0.0.3", Interfaces: []string{"eth1"}}}
	}
	errWriter = io.Discard
	if _, _, err := resolvePrimaryIPForTest(ConfigureOptions{NonInteractive: true}, "meta"); err == nil {
		t.Fatal("expected non-interactive dilemma error for multiple ip candidates")
	}

	localIPv4CandidatesFn = func() []localIPv4Candidate {
		return []localIPv4Candidate{{IP: "10.0.0.2", Interfaces: []string{"eth0"}}, {IP: defaultPrimaryIP, Interfaces: []string{"eth1"}}}
	}
	var demoBuf bytes.Buffer
	errWriter = &demoBuf
	ip, warnings, err = resolvePrimaryIPForTest(ConfigureOptions{}, "meta")
	if err != nil || ip != defaultPrimaryIP || len(warnings) != 0 {
		t.Fatalf("demo-ip auto selection mismatch: ip=%q warnings=%v err=%v", ip, warnings, err)
	}
	if !strings.Contains(demoBuf.String(), "Multiple IP address candidates found:") || !strings.Contains(demoBuf.String(), "[ OK ] primary_ip") {
		t.Fatalf("demo-ip auto selection was not explained: %q", demoBuf.String())
	}

	goosFn = func() string { return "darwin" }
	localIPv4CandidatesFn = func() []localIPv4Candidate {
		return []localIPv4Candidate{{IP: "192.168.1.10", Interfaces: []string{"en0"}}}
	}
	ip, warnings, err = resolvePrimaryIPForTest(ConfigureOptions{}, "meta")
	if err != nil || ip != defaultPrimaryIP {
		t.Fatalf("darwin fallback mismatch: ip=%q err=%v", ip, err)
	}
	if len(warnings) == 0 || !strings.Contains(warnings[0], "macOS") {
		t.Fatalf("expected macOS warning, got %v", warnings)
	}
}

func TestCheckKernelMachineAndVendorWarnings(t *testing.T) {
	restore := savePreflightHooks()
	defer restore()

	goosFn = func() string { return "darwin" }
	kernelWarnings := checkKernelWarnings()
	if len(kernelWarnings) == 0 || !strings.Contains(kernelWarnings[0], "admin node only") {
		t.Fatalf("unexpected kernel warnings: %v", kernelWarnings)
	}

	goarchFn = func() string { return "ppc64le" }
	machineWarnings := checkMachineWarnings()
	if len(machineWarnings) == 0 || !strings.Contains(machineWarnings[0], "not officially supported") {
		t.Fatalf("unexpected machine warnings: %v", machineWarnings)
	}

	goosFn = func() string { return "linux" }
	config.OSVendor = ""
	config.OSVersionFull = ""
	vendorWarnings := checkVendorVersionWarnings()
	if len(vendorWarnings) == 0 {
		t.Fatalf("expected vendor/version warnings, got: %v", vendorWarnings)
	}
}

func TestCheckAutoModeWarnings(t *testing.T) {
	restore := savePreflightHooks()
	defer restore()

	goosFn = func() string { return "linux" }
	config.OSVendor = "ubuntu"
	config.OSVersion = "20"

	warnings := checkAutoModeWarnings("")
	if len(warnings) == 0 || !strings.Contains(warnings[0], "EOL") {
		t.Fatalf("expected ubuntu20 EOL warning, got: %v", warnings)
	}

	if got := checkAutoModeWarnings("meta"); len(got) != 0 {
		t.Fatalf("expected no auto warning with explicit mode, got: %v", got)
	}
}

func TestCheckSudoAndAnsibleWarnings(t *testing.T) {
	restore := savePreflightHooks()
	defer restore()

	goosFn = func() string { return "linux" }
	currentUserFn = func() string { return "alice" }
	lookPathFn = func(name string) (string, error) {
		if name == "sudo" {
			return "", exec.ErrNotFound
		}
		return "", exec.ErrNotFound
	}
	if warnings := checkSudoWarnings(false); len(warnings) == 0 {
		t.Fatalf("expected warning when sudo command missing, got: %v", warnings)
	}
	if warnings := checkAnsibleWarnings(); len(warnings) == 0 {
		t.Fatalf("expected warning when ansible-playbook missing, got: %v", warnings)
	}

	currentUserFn = func() string { return "root" }
	if warnings := checkSudoWarnings(false); len(warnings) != 0 {
		t.Fatalf("expected no sudo warning for root, got: %v", warnings)
	}
}

func TestCheckAdminWarningsArgsAndRootWarning(t *testing.T) {
	restore := savePreflightHooks()
	defer restore()

	goosFn = func() string { return "linux" }
	currentUserFn = func() string { return "root" }
	lookPathFn = func(name string) (string, error) {
		if name == "ssh" {
			return "/usr/bin/ssh", nil
		}
		return "", exec.ErrNotFound
	}
	var called []string
	runCommandFn = func(timeout time.Duration, name string, args ...string) error {
		if name != "ssh" {
			t.Fatalf("expected ssh command, got %q", name)
		}
		called = append([]string(nil), args...)
		return nil
	}

	warnings := checkAdminWarnings("meta", "10.0.0.5", false, "2222")
	if !containsArgPair(called, "-p", "2222") {
		t.Fatalf("expected custom port in ssh args, got: %v", called)
	}
	for _, want := range []string{"10.0.0.5", "sudo", "-n", "ls"} {
		if !containsArg(called, want) {
			t.Fatalf("expected %q in ssh args, got: %v", want, called)
		}
	}
	foundRootWarn := false
	for _, w := range warnings {
		if strings.Contains(w, "user=root") {
			foundRootWarn = true
			break
		}
	}
	if !foundRootWarn {
		t.Fatalf("expected root warning, got: %v", warnings)
	}
}

func savePreflightHooks() func() {
	origLookPath := lookPathFn
	origRunCommand := runCommandFn
	origCurrentUserFn := currentUserFn
	origShellOutput := shellOutputFn
	origRegionDetect := regionDetectFn
	origInReader := inReader
	origErrWriter := errWriter
	origProgressWriter := progressWriter
	origGOOS := goosFn
	origGOARCH := goarchFn
	origNetworkInterfaces := networkInterfacesFn
	origInterfaceAddrs := interfaceAddrsFn
	origLocalIPv4Candidates := localIPv4CandidatesFn
	origNetworkCondition := networkConditionFn
	origInternetAccess := internetAccessFn
	origNetworkSource := networkSourceFn
	origNetworkRegion := networkRegionFn
	origOSType := config.OSType
	origOSVendor := config.OSVendor
	origOSVersion := config.OSVersion
	origOSVersionFull := config.OSVersionFull
	origConfigCurrentUser := config.CurrentUser
	origConfigGOOS := config.GOOS
	origConfigGOARCH := config.GOARCH
	origOSArch := config.OSArch
	origOSMajor := config.OSMajor

	return func() {
		lookPathFn = origLookPath
		runCommandFn = origRunCommand
		currentUserFn = origCurrentUserFn
		shellOutputFn = origShellOutput
		regionDetectFn = origRegionDetect
		inReader = origInReader
		errWriter = origErrWriter
		progressWriter = origProgressWriter
		goosFn = origGOOS
		goarchFn = origGOARCH
		networkInterfacesFn = origNetworkInterfaces
		interfaceAddrsFn = origInterfaceAddrs
		localIPv4CandidatesFn = origLocalIPv4Candidates
		networkConditionFn = origNetworkCondition
		internetAccessFn = origInternetAccess
		networkSourceFn = origNetworkSource
		networkRegionFn = origNetworkRegion
		config.OSType = origOSType
		config.OSVendor = origOSVendor
		config.OSVersion = origOSVersion
		config.OSVersionFull = origOSVersionFull
		config.CurrentUser = origConfigCurrentUser
		config.GOOS = origConfigGOOS
		config.GOARCH = origConfigGOARCH
		config.OSArch = origOSArch
		config.OSMajor = origOSMajor
	}
}

func containsArgPair(args []string, key, value string) bool {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == key && args[i+1] == value {
			return true
		}
	}
	return false
}

func containsArg(args []string, value string) bool {
	for _, a := range args {
		if a == value {
			return true
		}
	}
	return false
}

// resolvePrimaryIPForTest adapts resolvePrimaryIPs to the single-IP signature
// the older tests were written against.
func resolvePrimaryIPForTest(opts ConfigureOptions, mode string) (string, []string, error) {
	primaryIPs, warnings, err := resolvePrimaryIPs(opts, mode, nil)
	if err != nil {
		return "", warnings, err
	}
	return primaryIPs[0], warnings, nil
}

func TestResolvePrimaryIPsPipedEOFGivesActionableError(t *testing.T) {
	restore := savePreflightHooks()
	defer restore()
	goosFn = func() string { return "linux" }
	localIPv4CandidatesFn = func() []localIPv4Candidate {
		return []localIPv4Candidate{{IP: "10.0.0.2", Interfaces: []string{"eth0"}}, {IP: "10.0.0.3", Interfaces: []string{"eth1"}}}
	}
	errWriter = io.Discard
	inReader = strings.NewReader("")

	_, _, err := resolvePrimaryIPs(ConfigureOptions{}, "meta", nil)
	if err == nil || !strings.Contains(err.Error(), "specify --ip in non-interactive mode") {
		t.Fatalf("piped EOF error = %v", err)
	}
}
