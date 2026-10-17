//go:build darwin

package build

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Explicit opt-in: a temporary, uniquely named LaunchAgent using the installed
// Xray and loopback only. No production label, public server, or network sample.
func TestXrayMacOSLegacyServiceAcceptance(t *testing.T) {
	if os.Getenv("PIG_TEST_MACOS_SERVICE") != "1" {
		t.Skip("set PIG_TEST_MACOS_SERVICE=1 for real launchctl/Xray acceptance")
	}
	binary, err := exec.LookPath("xray")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	dir, _ = filepath.EvalSymlinks(dir)
	r := proxyRuntime{os: "darwin", label: fmt.Sprintf("com.pigsty.xray-acceptance-%d", os.Getpid()), config: filepath.Join(dir, "config.json"), serviceFile: filepath.Join(dir, "agent.plist"), run: proxyRun, capture: proxyCapture}
	if r.active() {
		t.Fatal("temporary service label already exists")
	}
	t.Cleanup(func() {
		if r.active() {
			if err := r.run("launchctl", "bootout", r.domain()+"/"+r.label); err != nil {
				t.Error(err)
			}
		}
		if r.active() {
			t.Error("temporary LaunchAgent still loaded")
		}
	})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listen := listener.Addr().String()
	_ = listener.Close()
	// No authentication material or remote target is needed to prove LaunchAgent
	// lifecycle. The local SOCKS listener uses Xray's freedom outbound.
	candidate := []byte(fmt.Sprintf(`{"log":{"loglevel":"none"},"inbounds":[{"listen":"127.0.0.1","port":%d,"protocol":"socks","settings":{"auth":"noauth"}}],"outbounds":[{"protocol":"freedom"}]}`, port))
	legacy, err := os.ReadFile("testdata/xray-legacy-c885c696.plist")
	if err != nil {
		t.Fatal(err)
	}
	legacy = bytes.ReplaceAll(legacy, []byte("com.pigsty.xray-proxy"), []byte(r.label))
	legacy = bytes.ReplaceAll(legacy, []byte("/Users/fixture/.config/xray/pig-proxy.json"), []byte(r.config))
	legacy = bytes.ReplaceAll(legacy, []byte("/opt/homebrew/bin/xray"), []byte(binary))
	if err := os.WriteFile(r.config, candidate, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(r.serviceFile, legacy, 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.validateConfig(binary, r.config); err != nil {
		t.Fatal(err)
	}
	if err := r.start(false); err != nil {
		t.Fatal(err)
	}
	if err := waitProxyListener(r, listen); err != nil {
		t.Fatal(err)
	}
	desired, err := r.serviceContent(listen, binary)
	if err != nil {
		t.Fatal(err)
	}
	if !r.serviceFileReplaceable(legacy, desired) {
		t.Fatal("legacy source format requires --replace")
	}
	tx, err := r.transaction("server")
	if err != nil {
		t.Fatal(err)
	}
	changed := r.configFilesChanged(tx.saved, candidate, desired, os.Getegid())
	if !changed {
		t.Fatal("expected marker migration")
	}
	if err := r.stageCandidate(binary, candidate); err != nil {
		t.Fatal(err)
	}
	if err := tx.commit(candidate, desired, changed); err != nil {
		t.Fatal(err)
	}
	if err := waitProxyListener(r, listen); err != nil {
		t.Fatal(err)
	}
	pid := func() string {
		state, err := r.capture("launchctl", "print", r.domain()+"/"+r.label)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(state, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "pid = ") {
				return line
			}
		}
		t.Fatal("no running pid")
		return ""
	}
	before := pid()
	tx, err = r.transaction("server")
	if err != nil {
		t.Fatal(err)
	}
	changed = r.configFilesChanged(tx.saved, candidate, desired, os.Getegid())
	if changed {
		t.Fatal("identical rerun reports change")
	}
	if err := tx.commit(candidate, desired, changed); err != nil {
		t.Fatal(err)
	}
	if after := pid(); after != before {
		t.Fatalf("identical rerun restarted service: %s -> %s", before, after)
	}
	t.Logf("real launchctl + %s: legacy bootstrap, marker migration/restart, identical rerun preserved %s; loopback %s", binary, before, listen)
}
