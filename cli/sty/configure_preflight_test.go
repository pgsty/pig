package sty

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"pig/cli/get"
	"pig/internal/config"
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
		Skip:             true,
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

	goosFn = func() string { return "linux" }
	localIPv4sFn = func() []string { return []string{"10.0.0.2", "10.0.0.3"} }
	inReader = strings.NewReader("10.0.0.8\n")
	var errBuf bytes.Buffer
	errWriter = &errBuf

	ip, warnings, err := resolvePrimaryIP(ConfigureOptions{NonInteractive: false}, "meta")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "10.0.0.8" {
		t.Fatalf("expected interactive input ip, got %q", ip)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if !strings.Contains(errBuf.String(), "INPUT primary_ip address") {
		t.Fatalf("expected prompt output, got %q", errBuf.String())
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

	ip, warnings, err := resolvePrimaryIP(ConfigureOptions{Skip: true}, "meta")
	if err != nil || ip != defaultPrimaryIP || len(warnings) != 0 {
		t.Fatalf("skip branch mismatch: ip=%q warnings=%v err=%v", ip, warnings, err)
	}

	if _, _, err := resolvePrimaryIP(ConfigureOptions{PrimaryIP: "bad-ip"}, "meta"); err == nil {
		t.Fatal("expected invalid ip error")
	}

	localIPv4sFn = func() []string { return nil }
	ip, warnings, err = resolvePrimaryIP(ConfigureOptions{}, "meta")
	if err != nil || ip != defaultPrimaryIP {
		t.Fatalf("empty probe fallback mismatch: ip=%q err=%v", ip, err)
	}
	if len(warnings) == 0 {
		t.Fatal("expected warning when probe fails")
	}

	localIPv4sFn = func() []string { return []string{"10.0.0.5"} }
	ip, warnings, err = resolvePrimaryIP(ConfigureOptions{}, "meta")
	if err != nil || ip != "10.0.0.5" || len(warnings) != 0 {
		t.Fatalf("single ip branch mismatch: ip=%q warnings=%v err=%v", ip, warnings, err)
	}

	localIPv4sFn = func() []string { return []string{"10.0.0.2", "10.0.0.3"} }
	if _, _, err := resolvePrimaryIP(ConfigureOptions{NonInteractive: true}, "meta"); err == nil {
		t.Fatal("expected non-interactive dilemma error for multiple ip candidates")
	}

	localIPv4sFn = func() []string { return []string{"10.0.0.2", defaultPrimaryIP} }
	ip, warnings, err = resolvePrimaryIP(ConfigureOptions{}, "meta")
	if err != nil || ip != defaultPrimaryIP || len(warnings) != 0 {
		t.Fatalf("demo-ip auto selection mismatch: ip=%q warnings=%v err=%v", ip, warnings, err)
	}

	goosFn = func() string { return "darwin" }
	localIPv4sFn = func() []string { return []string{"192.168.1.10"} }
	ip, warnings, err = resolvePrimaryIP(ConfigureOptions{}, "meta")
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
	origGOOS := goosFn
	origGOARCH := goarchFn
	origLocalIPv4s := localIPv4sFn
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

	return func() {
		lookPathFn = origLookPath
		runCommandFn = origRunCommand
		currentUserFn = origCurrentUserFn
		shellOutputFn = origShellOutput
		regionDetectFn = origRegionDetect
		inReader = origInReader
		errWriter = origErrWriter
		goosFn = origGOOS
		goarchFn = origGOARCH
		localIPv4sFn = origLocalIPv4s
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
