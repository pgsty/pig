package build

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func proxyTestPair(t *testing.T) (ProxyServerOptions, xrayConfig, ProxyConnection) {
	t.Helper()
	opts := ProxyServerOptions{Host: "proxy.example.com", Port: 443, Target: "www.example.net:443", SNI: "www.example.net", ProxyProtocol: true}
	cfg, err := newXrayServer(opts, defaultProxyBackend)
	if err != nil {
		t.Fatal(err)
	}
	c, err := serverConnection(cfg, opts.Host, opts.Port)
	if err != nil {
		t.Fatal(err)
	}
	return opts, cfg, c
}

func TestXrayConnectionInputsAndRoundTrip(t *testing.T) {
	_, _, c := proxyTestPair(t)
	got, err := ParseProxyURI(c.URI())
	if err != nil || got != c {
		t.Fatalf("connection URI did not round trip: %v", err)
	}
	path := filepath.Join(t.TempDir(), "arbitrary-name")
	if err := os.WriteFile(path, []byte(c.URI()+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	inputs := []ProxyClientOptions{{URI: c.URI()}, {From: path}, {From: "-"}, {Server: "proxy.example.com:443", ID: c.ID, SNI: c.SNI, PublicKey: c.PublicKey, ShortID: c.ShortID, PQV: c.PQV}}
	for _, opts := range inputs {
		got, err := clientConnection(opts, strings.NewReader(c.URI()+"\n"))
		if err != nil || got != c {
			t.Fatalf("connection inputs differ: %v", err)
		}
	}
	b, err := renderXrayClient(c, DefaultProxyListen)
	if err != nil {
		t.Fatal(err)
	}
	got, listen, err := inspectXrayClient(b)
	if err != nil || got != c || listen != DefaultProxyListen {
		t.Fatalf("rendered client lost connection material: %v", err)
	}
}

func TestXrayRejectsAmbiguousAndUnsupportedInputsWithoutSecrets(t *testing.T) {
	_, _, c := proxyTestPair(t)
	for _, uri := range []string{c.URI() + "&sid=" + c.ShortID, strings.Replace(c.URI(), "flow="+visionFlow, "flow=other", 1), strings.Replace(c.URI(), "security=reality", "security=tls", 1), c.URI() + "&unknown=" + c.ID, "vless://" + c.ID + "@broken"} {
		_, err := ParseProxyURI(uri)
		if err == nil {
			t.Fatal("invalid connection URI accepted")
		}
		for _, secret := range []string{c.ID, c.PublicKey, c.ShortID, c.PQV} {
			if strings.Contains(err.Error(), secret) {
				t.Fatal("connection diagnostic leaked credentials")
			}
		}
	}
	if _, err := clientConnection(ProxyClientOptions{URI: c.URI(), From: "-"}, strings.NewReader("")); err == nil {
		t.Fatal("mixed connection input accepted")
	}
	if _, err := clientConnection(ProxyClientOptions{From: "-"}, strings.NewReader(strings.Repeat("x", proxyInputLimit+1))); err == nil {
		t.Fatal("oversized input accepted")
	}
	if _, _, err := proxyListener("bad';echo secret:12345"); err == nil {
		t.Fatal("unsafe listener accepted")
	}
}

func TestXrayFirstSetupDefaultsDoNotCreateCredentialsInPlan(t *testing.T) {
	r := proxyRuntime{os: "linux", config: filepath.Join(t.TempDir(), "absent.json")}
	for _, tt := range []struct {
		backend bool
		port    int
		listen  string
	}{{false, 443, "0.0.0.0:443"}, {false, 8443, "0.0.0.0:8443"}, {true, 443, defaultProxyBackend}} {
		opts := ProxyServerOptions{Host: "proxy.example.com", Port: tt.port, Target: "www.example.net:443", ProxyProtocol: tt.backend}
		_, listen, b, c, err := prepareXrayServer(r, opts, false)
		if err != nil || listen != tt.listen || b != nil || c != nil {
			t.Fatalf("invalid first-setup preview: %v", err)
		}
		if _, err := os.Stat(r.config); !os.IsNotExist(err) {
			t.Fatal("preview wrote configuration")
		}
	}
	if err := validateServerListener("0.0.0.0:9443", true); err == nil {
		t.Fatal("untrusted PROXY listener accepted")
	}
}

func TestXrayServerRepeatedSetupPreservesConfigCredentialsAndURI(t *testing.T) {
	opts, cfg, _ := proxyTestPair(t)
	// Existing bindings and explicitly chosen SNI must survive omitted flags.
	cfg.Inbounds[0].Port = 19443
	cfg.Inbounds[0].Stream.Reality.ServerNames = []string{"alternate.example.net"}
	c, err := serverConnection(cfg, opts.Host, opts.Port)
	if err != nil {
		t.Fatal(err)
	}
	b, err := marshalXray(cfg)
	if err != nil {
		t.Fatal(err)
	}
	r := proxyRuntime{os: "linux", config: filepath.Join(t.TempDir(), "xray.json")}
	if err := os.WriteFile(r.config, b, 0600); err != nil {
		t.Fatal(err)
	}
	opts.SNI, opts.Listen = "", ""
	for n := 0; n < 3; n++ {
		_, listen, got, exported, err := prepareXrayServer(r, opts, true)
		if err != nil || listen != "127.0.0.1:19443" || !bytes.Equal(got, b) || exported.URI() != c.URI() {
			t.Fatalf("server rerun changed client contract: %v", err)
		}
	}
	opts.SNI = "different.example.net"
	if _, _, _, _, err := prepareXrayServer(r, opts, true); err == nil {
		t.Fatal("SNI change accepted during ordinary setup")
	}
	opts.SNI = ""
	cfg.Inbounds[0].Stream.Reality.PrivateKey = "SECRET-MALFORMED-PRIVATE-KEY"
	bad, _ := marshalXray(cfg)
	if err := os.WriteFile(r.config, bad, 0600); err != nil {
		t.Fatal(err)
	}
	_, _, _, _, err = prepareXrayServer(r, opts, true)
	if err == nil || strings.Contains(err.Error(), "SECRET-MALFORMED-PRIVATE-KEY") {
		t.Fatal("invalid existing key was regenerated or leaked")
	}
}

func TestXrayClientRepeatedSetupPreservesExistingListener(t *testing.T) {
	_, _, c := proxyTestPair(t)
	r := proxyRuntime{config: filepath.Join(t.TempDir(), "xray.json")}
	b, err := renderXrayClient(c, "127.0.0.1:8888")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(r.config, b, 0600); err != nil {
		t.Fatal(err)
	}
	_, listen, got, err := prepareXrayClient(r, ProxyClientOptions{URI: c.URI()}, strings.NewReader(""))
	if err != nil || listen != "127.0.0.1:8888" || !bytes.Equal(b, got) {
		t.Fatalf("client listener was replaced by a default: %v", err)
	}
	_, listen, _, err = prepareXrayClient(r, ProxyClientOptions{URI: c.URI(), ListenSet: true, Listen: DefaultProxyListen}, strings.NewReader(""))
	if err != nil || listen != DefaultProxyListen {
		t.Fatalf("explicit listener override failed: %v", err)
	}
	c.ShortID = "0102030405060708"
	_, listen, _, err = prepareXrayClient(r, ProxyClientOptions{URI: c.URI(), Replace: true}, strings.NewReader(""))
	if err != nil || listen != "127.0.0.1:8888" {
		t.Fatalf("credential replacement lost the existing listener: %v", err)
	}
}

func TestXrayReadOnlyExportAndAmbiguousServerRefusal(t *testing.T) {
	opts, cfg, c := proxyTestPair(t)
	b, _ := marshalXray(cfg)
	r := proxyRuntime{os: "linux", config: filepath.Join(t.TempDir(), "xray.json")}
	if err := os.WriteFile(r.config, b, 0600); err != nil {
		t.Fatal(err)
	}
	query := ProxyServerOptions{Host: opts.Host, Port: 443, ExportOnly: true, Export: "-"}
	_, _, got, exported, err := prepareXrayServer(r, query, false)
	if err != nil || exported.URI() != c.URI() || !bytes.Equal(got, b) {
		t.Fatalf("read-only export failed: %v", err)
	}
	cfg.Inbounds[0].Settings.Clients = append(cfg.Inbounds[0].Settings.Clients, cfg.Inbounds[0].Settings.Clients[0])
	b, _ = marshalXray(cfg)
	if err := os.WriteFile(r.config, b, 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := prepareXrayServer(r, query, false); err == nil {
		t.Fatal("ambiguous server selected an arbitrary client")
	}
}

func TestXrayExportFileIsProtectedAndIdempotent(t *testing.T) {
	_, _, c := proxyTestPair(t)
	path := filepath.Join(t.TempDir(), "client.uri")
	if err := exportProxyConnection(c, path, nil); err != nil {
		t.Fatal(err)
	}
	stamp := time.Unix(1234567890, 0)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	if err := exportProxyConnection(c, path, nil); err != nil {
		t.Fatal(err)
	}
	i, err := os.Stat(path)
	if err != nil || i.Mode().Perm() != 0600 || !i.ModTime().Equal(stamp) {
		t.Fatal("identical export was rewritten or not protected")
	}
	c.ShortID = "0102030405060708"
	if err := exportProxyConnection(c, path, nil); err == nil {
		t.Fatal("different export silently replaced existing credentials")
	}
	var out bytes.Buffer
	if err := exportProxyConnection(c, "-", &out); err != nil || out.String() != c.URI()+"\n" {
		t.Fatal("stdout export is not exactly one URI")
	}
}

func TestXrayServiceCapabilitiesAndSecretFreePlan(t *testing.T) {
	r := proxyRuntime{os: "linux", asset: "/usr/share/xray", config: "/etc/xray.json", serviceFile: "/etc/systemd/system/xray.service.d/pig-proxy.conf"}
	for _, tt := range []struct{ listen, capability string }{{"0.0.0.0:443", "CAP_NET_BIND_SERVICE"}, {defaultProxyBackend, ""}} {
		b, err := r.serviceContent(tt.listen, "/usr/bin/xray")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), "AmbientCapabilities="+tt.capability+"\n") || strings.Contains(string(b), "CAP_NET_ADMIN") {
			t.Fatal("incorrect service privilege")
		}
		if !strings.Contains(string(b), "CapabilityBoundingSet=\nAmbientCapabilities=\n") {
			t.Fatal("package capabilities were not reset before assigning listener capabilities")
		}
	}
	_, _, c := proxyTestPair(t)
	b, _ := json.Marshal(proxyPlan("client", r, DefaultProxyListen))
	for _, secret := range []string{c.ID, c.PublicKey, c.ShortID, c.PQV} {
		if bytes.Contains(b, []byte(secret)) {
			t.Fatal("plan contains connection credentials")
		}
	}
}

func TestXrayServerSetupRefusesUnknownRoutingButAllowsReadOnlyExport(t *testing.T) {
	opts, cfg, _ := proxyTestPair(t)
	cfg.Routing = json.RawMessage(`{"domainStrategy":"AsIs","rules":[]}`)
	b, _ := marshalXray(cfg)
	r := proxyRuntime{os: "linux", config: filepath.Join(t.TempDir(), "xray.json")}
	if err := os.WriteFile(r.config, b, 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := prepareXrayServer(r, opts, true); err == nil {
		t.Fatal("setup took over an unknown routing policy")
	}
	query := ProxyServerOptions{Host: opts.Host, Port: 443, ExportOnly: true, Export: "-"}
	if _, _, _, _, err := prepareXrayServer(r, query, false); err != nil {
		t.Fatalf("read-only export unnecessarily rejects routing: %v", err)
	}
}

func TestXrayConfigOwnershipMustMatchEvenWhenContentMatches(t *testing.T) {
	r := proxyRuntime{os: "linux"}
	config, service := []byte("config"), []byte("service")
	saved := []proxySnapshot{{data: config, mode: 0640, uid: 0, gid: 123}, {data: service, mode: 0644, uid: 0, gid: 0}}
	if r.configFilesChanged(saved, config, service, 123) {
		t.Fatal("matching files were treated as changed")
	}
	for _, tt := range []struct{ file, uid, gid int }{{0, 0, 0}, {0, 42, 123}, {1, 42, 0}} {
		changed := append([]proxySnapshot(nil), saved...)
		changed[tt.file].uid, changed[tt.file].gid = tt.uid, tt.gid
		if !r.configFilesChanged(changed, config, service, 123) {
			t.Fatal("incorrect file ownership was treated as an idempotent success")
		}
	}
}

func TestXrayMacEnabledStateAndRecovery(t *testing.T) {
	const label = "com.pigsty.xray-review"
	for _, tt := range []struct {
		state   string
		want    bool
		failure bool
	}{{"disabled services = {\n}", true, false}, {`"other.service" => true`, true, false}, {`"com.pigsty.xray-review" => false`, true, false}, {`"com.pigsty.xray-review" => true`, false, false}, {`"com.pigsty.xray-review" => enabled`, true, false}, {`"com.pigsty.xray-review" => disabled`, false, false}, {`"com.pigsty.xray-review" => unknown`, false, false}, {"", false, true}} {
		r := proxyRuntime{os: "darwin", label: label, capture: func(name string, args ...string) (string, error) {
			if name != "launchctl" || args[0] != "print-disabled" {
				t.Fatal("unexpected enablement query")
			}
			if tt.failure {
				return "", fmt.Errorf("query failed")
			}
			return tt.state, nil
		}}
		if got := r.enabled(); got != tt.want {
			t.Fatalf("incorrect persistent LaunchAgent enablement: got %v", got)
		}
	}
	var operations []string
	r := proxyRuntime{os: "darwin", label: label, serviceFile: "/test/agent.plist", run: func(name string, args ...string) error {
		if name != "launchctl" {
			t.Fatal("unexpected service operation")
		}
		if args[0] == "print" {
			return fmt.Errorf("not loaded")
		}
		operations = append(operations, args[0])
		return nil
	}}
	if err := r.start(false); err != nil || strings.Join(operations, ",") != "enable,bootstrap" {
		t.Fatal("disabled LaunchAgent was not enabled before loading")
	}
	operations = nil
	if err := r.restoreService(true, false); err != nil || strings.Join(operations, ",") != "enable,bootstrap,disable" {
		t.Fatal("rollback did not restore both running and disabled state")
	}
	operations = nil
	if err := r.restoreService(false, true); err != nil || strings.Join(operations, ",") != "enable" {
		t.Fatal("rollback changed a stopped, enabled LaunchAgent into a disabled one")
	}
}

func TestXrayUnitMustUseTheValidatedIdentityAndConfig(t *testing.T) {
	const command = "{ path=/usr/bin/xray ; argv[]=/usr/bin/xray run -config /etc/xray.json ; ignore_errors=no }"
	for _, tt := range []struct {
		group, command string
		allowed        bool
	}{{"xray", command, true}, {"", command, true}, {"root", command, false}, {"xray", strings.Replace(command, "xray.json ;", "xray.json -confdir /other ;", 1), false}, {"xray", strings.Replace(command, "/etc/xray.json", "/other/config.json", 1), false}, {"xray", command + command, false}} {
		r := proxyRuntime{os: "linux", config: "/etc/xray.json", capture: func(_ string, args ...string) (string, error) {
			switch args[2] {
			case "--property=User":
				return "xray", nil
			case "--property=Group":
				return tt.group, nil
			case "--property=ExecStart":
				return tt.command, nil
			default:
				t.Fatal("unexpected unit property")
				return "", nil
			}
		}}
		if err := r.checkServiceUnit(); (err == nil) != tt.allowed {
			t.Fatal("unit with a different identity or extra configuration was not refused")
		}
	}
}
