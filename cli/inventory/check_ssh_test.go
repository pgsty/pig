package inventorycli

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestCheckSSHProfileUsesDisposableTargetWithoutPersistentMutation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("OpenSSH server integration is not available on Windows")
	}
	sshd, err := exec.LookPath("sshd")
	if err != nil {
		if _, statErr := os.Stat("/usr/sbin/sshd"); statErr != nil {
			t.Skip("sshd is not installed")
		}
		sshd = "/usr/sbin/sshd"
	}
	keygen, err := exec.LookPath("ssh-keygen")
	if err != nil {
		t.Skip("ssh-keygen is not installed")
	}
	ssh, err := exec.LookPath("ssh")
	if err != nil {
		t.Skip("ssh is not installed")
	}
	currentUser, err := user.Current()
	if err != nil || !checkUserPattern.MatchString(currentUser.Username) {
		t.Skip("current account cannot be represented by the Check SSH user contract")
	}

	temp := t.TempDir()
	hostKey := filepath.Join(temp, "host_ed25519")
	clientKey := filepath.Join(temp, "client_ed25519")
	generateSSHTestKey(t, keygen, hostKey)
	generateSSHTestKey(t, keygen, clientKey)
	clientPublic, err := os.ReadFile(clientKey + ".pub")
	if err != nil {
		t.Fatal(err)
	}
	authorizedKeys := filepath.Join(temp, "authorized_keys")
	if err := os.WriteFile(authorizedKeys, clientPublic, 0600); err != nil {
		t.Fatal(err)
	}

	port := reserveSSHTestPort(t)
	hostPublic, err := os.ReadFile(hostKey + ".pub")
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(string(hostPublic))
	if len(fields) < 2 {
		t.Fatalf("unexpected host public key: %q", hostPublic)
	}
	knownHosts := filepath.Join(temp, "known_hosts")
	knownLine := fmt.Sprintf("[127.0.0.1]:%d %s %s\n", port, fields[0], fields[1])
	if err := os.WriteFile(knownHosts, []byte(knownLine), 0600); err != nil {
		t.Fatal(err)
	}
	knownBefore, err := os.ReadFile(knownHosts)
	if err != nil {
		t.Fatal(err)
	}

	configPath := filepath.Join(temp, "sshd_config")
	config := fmt.Sprintf(`Port %d
ListenAddress 127.0.0.1
AddressFamily inet
HostKey %s
PidFile %s
AuthorizedKeysFile %s
AuthenticationMethods publickey
PubkeyAuthentication yes
PasswordAuthentication no
KbdInteractiveAuthentication no
UsePAM no
PermitRootLogin yes
StrictModes no
AllowUsers %s
AllowAgentForwarding no
AllowTcpForwarding no
X11Forwarding no
PermitTunnel no
PermitTTY no
GatewayPorts no
PrintMotd no
PrintLastLog no
UseDNS no
LogLevel ERROR
`, port, hostKey, filepath.Join(temp, "sshd.pid"), authorizedKeys, currentUser.Username)
	if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	server := exec.Command(sshd, "-D", "-e", "-f", configPath)
	server.Stdout = &stderr
	server.Stderr = &stderr
	if err := server.Start(); err != nil {
		t.Fatalf("start disposable sshd: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Wait() }()
	t.Cleanup(func() {
		if server.Process != nil {
			_ = server.Process.Kill()
		}
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	})
	waitForSSHTestServer(t, port, done, &stderr)

	home, inventoryPath := makeCheckFixture(t, "127.0.0.1")
	fixtureBefore := checkFixtureSnapshot(t, home, inventoryPath)
	checkRuntime := defaultCheckRuntime()
	checkRuntime.lookPath = func(name string) (string, error) {
		if name == "ansible-playbook" {
			return "/test/bin/ansible-playbook", nil
		}
		if name == "ssh" {
			return ssh, nil
		}
		return exec.LookPath(name)
	}
	checkRuntime.run = func(ctx context.Context, command checkCommand) (checkCommandOutput, error) {
		if command.kind == checkCommandAnsibleVersion {
			return checkCommandOutput{stdout: "ansible-playbook [core 2.19.0]"}, nil
		}
		return runSystemCheckCommand(ctx, command)
	}
	checkRuntime.knownHostsFile = knownHosts
	checkRuntime.identityFile = clientKey
	checkRuntime.timeouts.ssh = 8 * time.Second

	result := checkWithRuntime(CheckOptions{
		Path: inventoryPath, Home: home, Profiles: []string{"ssh"},
		Port: port, User: currentUser.Username,
	}, checkRuntime)
	if !result.Success {
		t.Fatalf("disposable SSH Check failed: %+v; sshd stderr=%q", result, stderr.String())
	}
	knownAfter, err := os.ReadFile(knownHosts)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(knownBefore, knownAfter) {
		t.Fatalf("SSH Check mutated known_hosts: before=%q after=%q", knownBefore, knownAfter)
	}
	if fixtureAfter := checkFixtureSnapshot(t, home, inventoryPath); fixtureBefore != fixtureAfter {
		t.Fatalf("SSH Check mutated its Pigsty fixture: before=%q after=%q", fixtureBefore, fixtureAfter)
	}
}

func generateSSHTestKey(t *testing.T, keygen, path string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, keygen, "-q", "-t", "ed25519", "-N", "", "-f", path)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("generate disposable SSH key: %v: %s", err, output)
	}
}

func reserveSSHTestPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}

func waitForSSHTestServer(t *testing.T, port int, done <-chan error, stderr *bytes.Buffer) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	for time.Now().Before(deadline) {
		select {
		case err := <-done:
			t.Fatalf("disposable sshd exited before readiness: %v: %s", err, stderr.String())
		default:
		}
		connection, err := net.DialTimeout("tcp", address, 50*time.Millisecond)
		if err == nil {
			_ = connection.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("disposable sshd did not listen on %s: %s", address, stderr.String())
}
