package sty

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"pig/internal/output"
	"pig/inventory"
)

func TestCheckDefaultIsLocalAndPerformsNoNetworkIO(t *testing.T) {
	home, inventoryPath := makeCheckFixture(t, "127.0.0.1")
	runtime := fakeCheckRuntime()
	var commands atomic.Int32
	runtime.run = func(_ context.Context, command checkCommand) (checkCommandOutput, error) {
		commands.Add(1)
		switch command.kind {
		case checkCommandAnsibleVersion:
			return checkCommandOutput{stdout: "ansible-playbook [core 2.19.0]\n"}, nil
		case checkCommandSSHVersion:
			return checkCommandOutput{stderr: "OpenSSH_10.2p1, LibreSSL 3.3.6\n"}, nil
		default:
			t.Fatalf("default Check executed non-local command: %+v", command)
			return checkCommandOutput{}, errors.New("unexpected command")
		}
	}
	runtime.dial = func(context.Context, string, string) (net.Conn, error) {
		t.Fatal("default Check attempted network I/O")
		return nil, errors.New("unexpected dial")
	}
	runtime.validateAnsible = func(context.Context, string, *inventory.Document) []inventory.Diagnostic {
		t.Fatal("default Check attempted the opt-in Ansible profile")
		return nil
	}

	result := checkWithRuntime(CheckOptions{Path: inventoryPath, Home: home}, runtime)
	if !result.Success || result.Code != 0 {
		t.Fatalf("default Check failed: %+v", result)
	}
	data, ok := result.Data.(*CheckData)
	if !ok || !data.Ready || data.Kind != CheckKind {
		t.Fatalf("Check data = %#v", result.Data)
	}
	if got := strings.Join(data.Profiles, ","); got != "local" {
		t.Fatalf("profiles = %q", got)
	}
	if len(data.Targets) != 1 || data.Targets[0] != "127.0.0.1" {
		t.Fatalf("targets = %#v", data.Targets)
	}
	if len(data.Tools) != 2 || data.Tools[0].Version != "2.19.0" || data.Tools[1].Version != "OpenSSH_10.2p1" {
		t.Fatalf("tools = %#v", data.Tools)
	}
	if got := commands.Load(); got != 2 {
		t.Fatalf("local version command count = %d, want 2", got)
	}
}

func TestCheckOptionsFailImmediately(t *testing.T) {
	tests := []struct {
		name string
		opts CheckOptions
	}{
		{name: "unknown profile", opts: CheckOptions{Profiles: []string{"docker"}}},
		{name: "sudo without ssh", opts: CheckOptions{Sudo: true}},
		{name: "invalid port", opts: CheckOptions{Port: 70000}},
		{name: "invalid user", opts: CheckOptions{User: "root;id"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, failure := normalizeCheckOptions(&test.opts)
			if failure == nil || failure.Code != output.CodeStyCheckInvalidArgs || failure.ExitCode() != 2 {
				t.Fatalf("failure = %+v", failure)
			}
		})
	}

	opts := CheckOptions{Profiles: []string{"ssh,network", "local", "ssh"}, Sudo: true}
	profiles, enabled, failure := normalizeCheckOptions(&opts)
	if failure != nil || strings.Join(profiles, ",") != "local,network,ssh" || !enabled[checkProfileSSH] || opts.Port != 22 {
		t.Fatalf("normalized profiles=%v enabled=%v opts=%+v failure=%+v", profiles, enabled, opts, failure)
	}
}

func TestCheckFindingsDeduplicateByContractKey(t *testing.T) {
	findings := []Finding{
		newCheckFinding("DUP", inventory.SeverityError, checkStageNetwork, "/same", "10.0.0.1", "later", output.CodeStyCheckNetworkFailed),
		newCheckFinding("KEEP", inventory.SeverityWarning, checkStageLocal, "/one", "", "one", 0),
		newCheckFinding("DUP", inventory.SeverityError, checkStageValidate, "/same", "10.0.0.1", "earlier", output.CodeStyCheckValidationFailed),
		newCheckFinding("DUP", inventory.SeverityError, checkStageSSH, "/same", "10.0.0.2", "different target", output.CodeStyCheckSSHFailed),
		newCheckFinding("DUP", inventory.SeverityError, checkStageSSH, "/other", "10.0.0.1", "different path", output.CodeStyCheckSSHFailed),
	}
	got := deduplicateCheckFindings(findings)
	if len(got) != 4 {
		t.Fatalf("deduplicated findings = %#v", got)
	}
	var duplicate *Finding
	for index := range got {
		if got[index].Code == "DUP" && got[index].Path == "/same" && got[index].Target == "10.0.0.1" {
			duplicate = &got[index]
		}
	}
	if duplicate == nil || duplicate.Stage != checkStageValidate || duplicate.Message != "earlier" {
		t.Fatalf("retained duplicate = %#v", duplicate)
	}
}

func TestCheckFailureSelectionUsesSeverityStageAndStableCode(t *testing.T) {
	findings := []Finding{
		newCheckFinding("A_NETWORK", inventory.SeverityError, checkStageNetwork, "", "10.0.0.1", "network", output.CodeStyCheckNetworkFailed),
		newCheckFinding("Z_VALIDATE", inventory.SeverityError, checkStageValidate, "/z", "", "validation", output.CodeStyCheckValidationFailed),
		newCheckFinding("A_VALIDATE", inventory.SeverityError, checkStageValidate, "/a", "", "validation", output.CodeStyCheckProbeFailed),
		newCheckFinding("A_LOCAL", inventory.SeverityWarning, checkStageLocal, "ssh", "", "warning", output.CodeStyCheckDependencyMissing),
	}
	selected := selectCheckFailure(findings)
	if selected == nil || selected.Code != "A_VALIDATE" || selected.ResultCode != output.CodeStyCheckProbeFailed {
		t.Fatalf("selected = %#v", selected)
	}
	if selectedExit := output.ExitCode(selected.ResultCode); selectedExit != 1 {
		t.Fatalf("selected shell exit = %d, want 1", selectedExit)
	}
	if !checkFindingPrecedes(
		newCheckFinding("LATE", inventory.SeverityError, checkStagePrivilege, "", "", "", 1),
		newCheckFinding("EARLY", inventory.SeverityWarning, checkStageValidate, "", "", "", 1),
	) {
		t.Fatal("higher severity must precede an earlier-stage warning")
	}
}

func TestCheckStatusCodesKeepGlobalShellMapping(t *testing.T) {
	for _, test := range []struct {
		code int
		exit int
	}{
		{output.CodeStyCheckInvalidArgs, 2},
		{output.CodeStyCheckPermissionDenied, 3},
		{output.CodeStyCheckDependencyMissing, 4},
		{output.CodeStyCheckNetworkFailed, 5},
		{output.CodeStyCheckResourceMissing, 6},
		{output.CodeStyCheckValidationFailed, 8},
		{output.CodeStyCheckProbeFailed, 1},
		{output.CodeStyCheckSSHFailed, 1},
	} {
		if got := output.ExitCode(test.code); got != test.exit {
			t.Errorf("ExitCode(%d) = %d, want %d", test.code, got, test.exit)
		}
	}
}

func TestCheckMixedCategoryResultSelectsValidationStage(t *testing.T) {
	home, inventoryPath := makeCheckFixtureWithSource(t, `all:
  vars:
    version: v4.4.0
    region: default
  children:
    infra:
      hosts:
        127.0.0.1:
          infra_seq: 1
`)
	runtime := fakeCheckRuntime()
	runtime.lookPath = func(string) (string, error) { return "", execNotFoundError{} }
	result := checkWithRuntime(CheckOptions{Path: inventoryPath, Home: home}, runtime)
	if result.Success || result.Code != output.CodeStyCheckValidationFailed || result.ExitCode() != 8 {
		t.Fatalf("mixed result = %+v", result)
	}
	data := result.Data.(*CheckData)
	var validation, dependency bool
	for _, finding := range data.Findings {
		validation = validation || finding.Stage == checkStageValidate && finding.Severity == inventory.SeverityError
		dependency = dependency || finding.Stage == checkStageLocal && finding.ResultCode == output.CodeStyCheckDependencyMissing
	}
	if !validation || !dependency {
		t.Fatalf("mixed findings = %#v", data.Findings)
	}
}

func TestCheckCommandAllowlistIsExact(t *testing.T) {
	sshArgs, err := buildSSHCheckArgs("127.0.0.1", "admin", 2222, 5*time.Second, false, "", "")
	if err != nil {
		t.Fatal(err)
	}
	command := checkCommand{
		kind: checkCommandSSH, executable: "/usr/bin/ssh", args: sshArgs,
		target: "127.0.0.1", user: "admin", port: 2222, timeout: 5 * time.Second,
	}
	if !isAllowedCheckCommand(command) || sshArgs[len(sshArgs)-1] != "true" {
		t.Fatalf("allowlisted SSH command = %#v", command)
	}
	joined := strings.Join(sshArgs, " ")
	for _, required := range []string{"-F /dev/null", "StrictHostKeyChecking=yes", "UpdateHostKeys=no", "PermitLocalCommand=no", "ProxyCommand=none"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("SSH allowlist omitted %q: %s", required, joined)
		}
	}

	sudoArgs, err := buildSSHCheckArgs("127.0.0.1", "", 22, 5*time.Second, true, "", "")
	if err != nil || strings.Join(sudoArgs[len(sudoArgs)-3:], " ") != "sudo -n true" {
		t.Fatalf("sudo allowlist = %v, err=%v", sudoArgs, err)
	}

	mutated := command
	mutated.args = append([]string(nil), command.args...)
	mutated.args[len(mutated.args)-1] = "uname"
	if isAllowedCheckCommand(mutated) {
		t.Fatal("mutated remote command passed the allowlist")
	}
	var ran atomic.Bool
	runtime := fakeCheckRuntime()
	runtime.run = func(context.Context, checkCommand) (checkCommandOutput, error) {
		ran.Store(true)
		return checkCommandOutput{}, nil
	}
	if _, runErr := runAllowedCheckCommand(context.Background(), runtime, mutated); !errors.Is(runErr, errCheckCommandBlocked) || ran.Load() {
		t.Fatalf("blocked command err=%v ran=%v", runErr, ran.Load())
	}

	for _, invalid := range []struct {
		target, user, knownHosts, identity string
	}{
		{target: "example.com"},
		{target: "127.0.0.1", user: "root;id"},
		{target: "127.0.0.1", knownHosts: "relative"},
		{target: "127.0.0.1", identity: "/tmp/key with space"},
	} {
		if _, buildErr := buildSSHCheckArgs(invalid.target, invalid.user, 22, time.Second, false, invalid.knownHosts, invalid.identity); buildErr == nil {
			t.Fatalf("unsafe SSH arguments accepted: %+v", invalid)
		}
	}
}

func TestCheckEachProbeHasAnIndependentTimeout(t *testing.T) {
	const probeTimeout = 20 * time.Millisecond
	runtime := fakeCheckRuntime()
	runtime.timeouts = checkTimeouts{local: probeTimeout, ansible: probeTimeout, network: probeTimeout, ssh: probeTimeout, privilege: probeTimeout}
	runtime.run = func(ctx context.Context, _ checkCommand) (checkCommandOutput, error) {
		<-ctx.Done()
		return checkCommandOutput{}, ctx.Err()
	}

	_, findings := checkLocalTool(runtime, "ssh", checkCommandSSHVersion)
	assertSingleCheckCode(t, findings, "CHECK_TOOL_VERSION_TIMEOUT")

	runtime.dial = func(ctx context.Context, _, _ string) (net.Conn, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	assertSingleCheckCode(t, checkNetworkTargets(runtime, []string{"127.0.0.1"}, 22), "CHECK_NETWORK_TIMEOUT")

	_, sshFinding := runSSHCheckProbe(runtime, "/usr/bin/ssh", "127.0.0.1", "", 22, false)
	if sshFinding == nil || sshFinding.Code != "CHECK_SSH_TIMEOUT" {
		t.Fatalf("SSH timeout finding = %#v", sshFinding)
	}
	_, sudoFinding := runSSHCheckProbe(runtime, "/usr/bin/ssh", "127.0.0.1", "", 22, true)
	if sudoFinding == nil || sudoFinding.Code != "CHECK_SUDO_TIMEOUT" {
		t.Fatalf("sudo timeout finding = %#v", sudoFinding)
	}

	doc, err := inventory.Parse("test.yml", []byte(validCheckInventory("127.0.0.1")))
	if err != nil {
		t.Fatal(err)
	}
	runtime.validateAnsible = func(ctx context.Context, _ string, _ *inventory.Document) []inventory.Diagnostic {
		<-ctx.Done()
		return nil
	}
	_, findings = checkAnsibleProfile(runtime, doc, "test.yml")
	assertSingleCheckCode(t, findings, "CHECK_ANSIBLE_TIMEOUT")
}

func TestCheckExplicitProfilesRemainSeparate(t *testing.T) {
	home, inventoryPath := makeCheckFixture(t, "127.0.0.1")
	runtime := fakeCheckRuntime()
	var ansibleCalls, dialCalls, sshCalls, sudoCalls atomic.Int32
	runtime.validateAnsible = func(context.Context, string, *inventory.Document) []inventory.Diagnostic {
		ansibleCalls.Add(1)
		return nil
	}
	runtime.dial = func(context.Context, string, string) (net.Conn, error) {
		dialCalls.Add(1)
		left, right := net.Pipe()
		_ = right.Close()
		return left, nil
	}
	runtime.run = func(_ context.Context, command checkCommand) (checkCommandOutput, error) {
		switch command.kind {
		case checkCommandAnsibleVersion:
			return checkCommandOutput{stdout: "ansible-playbook [core 2.19.0]"}, nil
		case checkCommandSSHVersion:
			return checkCommandOutput{stderr: "OpenSSH_10.2p1"}, nil
		case checkCommandSSH:
			if command.privilege {
				sudoCalls.Add(1)
				if got := strings.Join(command.args[len(command.args)-3:], " "); got != "sudo -n true" {
					t.Fatalf("privilege remote command = %q", got)
				}
			} else {
				sshCalls.Add(1)
				if command.args[len(command.args)-1] != "true" {
					t.Fatalf("SSH remote command = %#v", command.args)
				}
			}
			return checkCommandOutput{}, nil
		default:
			return checkCommandOutput{}, fmt.Errorf("unexpected command kind %q", command.kind)
		}
	}
	result := checkWithRuntime(CheckOptions{
		Path: inventoryPath, Home: home, Profiles: []string{"ssh,ansible,network"}, Sudo: true,
	}, runtime)
	if !result.Success {
		t.Fatalf("explicit profile Check failed: %+v", result)
	}
	data := result.Data.(*CheckData)
	if got := strings.Join(data.Profiles, ","); got != "local,ansible,network,ssh" {
		t.Fatalf("profile order = %q", got)
	}
	if ansibleCalls.Load() != 1 || dialCalls.Load() != 1 || sshCalls.Load() != 1 || sudoCalls.Load() != 1 {
		t.Fatalf("calls ansible=%d dial=%d ssh=%d sudo=%d", ansibleCalls.Load(), dialCalls.Load(), sshCalls.Load(), sudoCalls.Load())
	}
}

func TestCheckNetworkProfileUsesDisposableTargetWithoutMutation(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	accepted := make(chan struct{})
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr == nil {
			_ = connection.Close()
		}
		close(accepted)
	}()

	home, inventoryPath := makeCheckFixture(t, "127.0.0.1")
	before := checkFixtureSnapshot(t, home, inventoryPath)
	runtime := fakeCheckRuntime()
	runtime.dial = (&net.Dialer{}).DialContext
	result := checkWithRuntime(CheckOptions{Path: inventoryPath, Home: home, Profiles: []string{"network"}, Port: port}, runtime)
	if !result.Success {
		t.Fatalf("network profile failed: %+v", result)
	}
	select {
	case <-accepted:
	case <-time.After(time.Second):
		t.Fatal("disposable target did not observe the TCP connection")
	}
	after := checkFixtureSnapshot(t, home, inventoryPath)
	if before != after {
		t.Fatalf("network profile mutated its fixture: before=%q after=%q", before, after)
	}
}

func TestCheckStructuredOutputNeverCopiesRawVersionOutput(t *testing.T) {
	home, inventoryPath := makeCheckFixture(t, "127.0.0.1")
	runtime := fakeCheckRuntime()
	runtime.run = func(context.Context, checkCommand) (checkCommandOutput, error) {
		return checkCommandOutput{stdout: "password=hunter2 token=secret"}, nil
	}
	result := checkWithRuntime(CheckOptions{Path: inventoryPath, Home: home}, runtime)
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "hunter2") || strings.Contains(string(encoded), "token=secret") {
		t.Fatalf("raw executable output escaped into result: %s", encoded)
	}
	data := result.Data.(*CheckData)
	if data.Tools[0].Version != "" || data.Tools[1].Version != "" {
		t.Fatalf("unrecognized versions were retained: %#v", data.Tools)
	}
}

func fakeCheckRuntime() checkRuntime {
	runtime := defaultCheckRuntime()
	runtime.lookPath = func(name string) (string, error) { return "/test/bin/" + name, nil }
	runtime.run = func(_ context.Context, command checkCommand) (checkCommandOutput, error) {
		switch command.kind {
		case checkCommandAnsibleVersion:
			return checkCommandOutput{stdout: "ansible-playbook [core 2.19.0]"}, nil
		case checkCommandSSHVersion:
			return checkCommandOutput{stderr: "OpenSSH_10.2p1"}, nil
		default:
			return checkCommandOutput{}, nil
		}
	}
	return runtime
}

type execNotFoundError struct{}

func (execNotFoundError) Error() string { return "not found" }

func makeCheckFixture(t *testing.T, target string) (string, string) {
	t.Helper()
	return makeCheckFixtureWithSource(t, validCheckInventory(target))
}

func makeCheckFixtureWithSource(t *testing.T, source string) (string, string) {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "pigsty")
	if err := os.MkdirAll(filepath.Join(home, "roles"), 0755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"ansible.cfg": "[defaults]\ninventory = pigsty.yml\n",
		"deploy.yml":  "---\n- hosts: all\n  gather_facts: false\n",
	} {
		if err := os.WriteFile(filepath.Join(home, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	inventoryPath := filepath.Join(home, "pigsty.yml")
	if err := os.WriteFile(inventoryPath, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	return home, inventoryPath
}

func validCheckInventory(target string) string {
	return fmt.Sprintf(`all:
  vars:
    version: v4.4.0
    region: default
    admin_ip: 127.0.0.1
  children:
    infra:
      hosts:
        %s:
          infra_seq: 1
`, target)
}

func assertSingleCheckCode(t *testing.T, findings []Finding, code string) {
	t.Helper()
	if len(findings) != 1 || findings[0].Code != code {
		t.Fatalf("findings = %#v, want one %s", findings, code)
	}
}

func checkFixtureSnapshot(t *testing.T, home, inventoryPath string) string {
	t.Helper()
	var entries []string
	err := filepath.WalkDir(home, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(home, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		value := relative + ":" + info.Mode().String()
		if info.Mode().IsRegular() {
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			value += ":" + string(content)
		}
		entries = append(entries, value)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(inventoryPath); err != nil {
		t.Fatal(err)
	}
	return strings.Join(entries, "\x00")
}
