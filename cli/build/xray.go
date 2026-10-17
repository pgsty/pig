package build

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"pig/cli/ext"
	"pig/internal/output"
	"pig/internal/utils"

	"github.com/sirupsen/logrus"
)

// ProxySetupResult contains operational facts only, never connection credentials.
type ProxySetupResult struct {
	Role           string `json:"role" yaml:"role"`
	Listen         string `json:"listen" yaml:"listen"`
	Config         string `json:"config" yaml:"config"`
	Changed        bool   `json:"changed" yaml:"changed"`
	HTTP           bool   `json:"http,omitempty" yaml:"http,omitempty"`
	SOCKS          bool   `json:"socks,omitempty" yaml:"socks,omitempty"`
	Export         string `json:"export,omitempty" yaml:"export,omitempty"`
	PublicVerified bool   `json:"public_verified" yaml:"public_verified"`
}

type proxyRuntime struct {
	os, config, serviceFile, shell, label, asset string
	run                                          func(string, ...string) error
	capture                                      func(string, ...string) (string, error)
}

func newProxyRuntime() (proxyRuntime, error) {
	r := proxyRuntime{os: runtime.GOOS, label: "com.pigsty.xray-proxy", asset: "/usr/share/xray", run: proxyRun, capture: proxyCapture}
	switch r.os {
	case "linux":
		r.config, r.serviceFile, r.shell = "/etc/xray.json", "/etc/systemd/system/xray.service.d/pig-proxy.conf", "/etc/profile.d/proxy.sh"
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return r, fmt.Errorf("cannot resolve user home")
		}
		r.config, r.shell = filepath.Join(home, ".config/xray/pig-proxy.json"), filepath.Join(home, ".config/xray/proxy.sh")
		r.serviceFile = filepath.Join(home, "Library/LaunchAgents", r.label+".plist")
	default:
		return r, fmt.Errorf("xray setup supports Linux and macOS clients")
	}
	return r, nil
}

// Child output is private: Xray validation errors can contain private keys or seeds.
func proxyCapture(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	var out utils.LimitedBuffer = *utils.NewLimitedBuffer(proxyInputLimit)
	cmd.Stdout, cmd.Stderr = &out, io.Discard
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s operation failed", filepath.Base(name))
	}
	return out.String(), nil
}

func proxyRun(name string, args ...string) error {
	_, err := proxyCapture(name, args...)
	return err
}

func installXray() error {
	_, binaryErr := exec.LookPath("xray")
	if runtime.GOOS == "linux" {
		_, binaryErr = os.Stat("/usr/bin/xray")
	}
	if binaryErr == nil && runtime.GOOS == "darwin" {
		return nil
	}
	if binaryErr == nil && runtime.GOOS == "linux" {
		if _, err := os.Stat("/usr/lib/systemd/system/xray.service"); err == nil {
			return nil
		}
		if _, err := os.Stat("/lib/systemd/system/xray.service"); err == nil {
			return nil
		}
	}
	var args []string
	if runtime.GOOS == "darwin" {
		if os.Geteuid() == 0 {
			return fmt.Errorf("run macOS client setup as your regular user")
		}
		if _, err := exec.LookPath("brew"); err != nil {
			return fmt.Errorf("macOS client setup requires Homebrew")
		}
		args = []string{"brew", "install", "xray"}
	} else if runtime.GOOS == "linux" {
		if os.Geteuid() != 0 {
			return fmt.Errorf("xray installation requires root; run with sudo")
		}
		manager := ext.PackageManagerCmd()
		if manager != "dnf" && manager != "yum" && manager != "apt-get" {
			return fmt.Errorf("configure an apt or yum repository providing xray first")
		}
		args = []string{manager, "install", "-y", "xray"}
	} else {
		return fmt.Errorf("unsupported Xray platform")
	}
	logrus.Info("installing Xray from the configured package source")
	if err := proxyRun(args[0], args[1:]...); err != nil {
		return fmt.Errorf("xray package installation failed; configure a repository providing xray first")
	}
	return nil
}

func readProxyFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot inspect proxy file; check permissions")
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("proxy files must be regular files, not symlinks")
	}
	if info.Size() > proxyInputLimit {
		return nil, fmt.Errorf("proxy file exceeds 64 KiB")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read proxy file; check permissions")
	}
	return b, nil
}

func (r proxyRuntime) checkPlatform(role string) error {
	if r.os == "linux" {
		if os.Geteuid() != 0 {
			return fmt.Errorf("xray setup requires root; run with sudo")
		}
		if _, err := os.Stat("/run/systemd/system"); err != nil {
			return fmt.Errorf("xray setup requires running systemd; use a VM or a systemd-enabled container")
		}
	} else if role == "server" {
		return fmt.Errorf("xray server setup supports Linux only")
	} else if os.Geteuid() == 0 {
		return fmt.Errorf("run macOS client setup as your regular user")
	}
	return nil
}

func (r proxyRuntime) active() bool {
	if r.os == "linux" {
		return r.run("systemctl", "is-active", "--quiet", "xray") == nil
	}
	return r.run("launchctl", "print", r.domain()+"/"+r.label) == nil
}

func (r proxyRuntime) enabled() bool {
	if r.os == "linux" {
		return r.run("systemctl", "is-enabled", "--quiet", "xray") == nil
	}
	state, err := r.capture("launchctl", "print-disabled", r.domain())
	if err != nil {
		return false
	}
	for _, line := range strings.Split(state, "\n") {
		name, disabled, found := strings.Cut(line, "=>")
		if found && strings.TrimSpace(name) == strconv.Quote(r.label) {
			value := strings.TrimSpace(disabled)
			return value == "false" || value == "enabled"
		}
	}
	return true // An absent override means the LaunchAgent is enabled by default.
}

func (r proxyRuntime) domain() string { return "gui/" + strconv.Itoa(os.Getuid()) }

func (r proxyRuntime) start(restart bool) error {
	if r.os == "linux" {
		if err := r.run("systemctl", "daemon-reload"); err != nil {
			return err
		}
		if !r.enabled() {
			if err := r.run("systemctl", "enable", "xray"); err != nil {
				return err
			}
		}
		action := "start"
		if restart {
			action = "restart"
		}
		return r.run("systemctl", action, "xray")
	}
	if err := r.run("launchctl", "enable", r.domain()+"/"+r.label); err != nil {
		return err
	}
	if restart && r.active() {
		if err := r.run("launchctl", "bootout", r.domain()+"/"+r.label); err != nil {
			return err
		}
	}
	if !r.active() {
		return r.run("launchctl", "bootstrap", r.domain(), r.serviceFile)
	}
	return nil
}

func (r proxyRuntime) serviceContent(listen, binary string) ([]byte, error) {
	if r.os == "linux" {
		_, port, err := proxyListener(listen)
		if err != nil {
			return nil, err
		}
		capability := ""
		if port < 1024 {
			capability = "CAP_NET_BIND_SERVICE"
		}
		// Repeated positive assignments are additive in systemd. Reset the package's
		// capability sets before granting only the capability needed by this listener.
		service := "# Managed by pig build proxy\n[Service]\nEnvironment=XRAY_LOCATION_ASSET=" + r.asset + "\nCapabilityBoundingSet=\nAmbientCapabilities=\n"
		if capability != "" {
			service += "CapabilityBoundingSet=" + capability + "\nAmbientCapabilities=" + capability + "\n"
		}
		return []byte(service), nil
	}
	// All interpolated paths are XML-escaped; arguments are never shell source.
	escape := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;", "'", "&apos;")
	return []byte(fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!-- Managed by pig build proxy -->
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>%s</string>
<key>ProgramArguments</key><array><string>%s</string><string>run</string><string>-config</string><string>%s</string></array>
<key>RunAtLoad</key><true/><key>KeepAlive</key><true/>
<key>StandardOutPath</key><string>/dev/null</string>
<key>StandardErrorPath</key><string>/dev/null</string>
</dict></plist>
`, r.label, escape.Replace(binary), escape.Replace(r.config))), nil
}

func (r proxyRuntime) validateConfig(binary, path string) error {
	var err error
	if r.os == "linux" {
		err = r.run("runuser", "-u", "xray", "--", "env", "XRAY_LOCATION_ASSET="+r.asset, binary, "run", "-test", "-config", path)
	} else {
		err = r.run(binary, "run", "-test", "-config", path)
	}
	if err != nil {
		return fmt.Errorf("xray rejected the candidate configuration; validator output withheld because it can contain credentials")
	}
	return nil
}

func prepareXrayClient(r proxyRuntime, opts ProxyClientOptions, stdin io.Reader) (ProxyConnection, string, []byte, error) {
	c, err := clientConnection(opts, stdin)
	if err != nil {
		return c, "", nil, err
	}
	listen := DefaultProxyListen
	if opts.ListenSet {
		listen = opts.Listen
	}
	old, err := readProxyFile(r.config)
	if err != nil {
		return c, "", nil, err
	}
	if old != nil {
		previous, oldListen, inspectErr := inspectXrayClient(old)
		if inspectErr == nil && !opts.ListenSet {
			listen = oldListen
		}
		if inspectErr != nil || previous != c {
			if !opts.Replace {
				return c, "", nil, fmt.Errorf("existing client configuration differs or is unsupported; replacement requires --replace --yes")
			}
		} else {
			if !opts.ListenSet {
				listen = oldListen
			}
			if listen == oldListen {
				return c, listen, old, nil
			}
		}
	}
	b, err := renderXrayClient(c, listen)
	return c, listen, b, err
}

func normalizeServerOptions(opts ProxyServerOptions) (ProxyServerOptions, error) {
	if !validProxyHost(opts.Host) || opts.Port < 1 || opts.Port > 65535 {
		return opts, fmt.Errorf("--host and --port must specify a valid public endpoint")
	}
	if opts.ExportOnly {
		if opts.Export == "" {
			return opts, fmt.Errorf("--export-only requires an explicit --export destination")
		}
		if opts.Target != "" || opts.SNI != "" || opts.ListenSet || opts.ProxyProtocolSet {
			return opts, fmt.Errorf("--export-only cannot be combined with setup options")
		}
		return opts, nil
	}
	host, _, err := proxyEndpoint(opts.Target)
	if err != nil {
		return opts, fmt.Errorf("--target must specify a TLS host:port")
	}
	if opts.SNI == "" {
		opts.SNI = host
	}
	if !validProxyHost(opts.SNI) {
		return opts, fmt.Errorf("invalid --sni")
	}
	return opts, nil
}

func prepareXrayServer(r proxyRuntime, opts ProxyServerOptions, generate bool) (ProxyServerOptions, string, []byte, *ProxyConnection, error) {
	if r.os != "linux" {
		return opts, "", nil, nil, fmt.Errorf("xray server setup supports Linux only")
	}
	// Preserve an existing SNI even when it was explicitly chosen on first setup.
	explicitSNI := opts.SNI != ""
	var err error
	opts, err = normalizeServerOptions(opts)
	if err != nil {
		return opts, "", nil, nil, err
	}
	old, err := readProxyFile(r.config)
	if err != nil {
		return opts, "", nil, nil, err
	}
	listen := net.JoinHostPort("0.0.0.0", strconv.Itoa(opts.Port))
	if opts.ProxyProtocol {
		listen = defaultProxyBackend
	}
	if opts.ListenSet {
		listen = opts.Listen
	}
	if old != nil {
		return prepareExistingXrayServer(old, opts, explicitSNI, listen)
	}
	if opts.ExportOnly {
		return opts, "", nil, nil, fmt.Errorf("no existing server configuration to export")
	}
	if err := validateServerListener(listen, opts.ProxyProtocol); err != nil {
		return opts, "", nil, nil, err
	}
	if !generate {
		return opts, listen, nil, nil, nil
	}
	cfg, err := newXrayServer(opts, listen)
	if err != nil {
		return opts, "", nil, nil, err
	}
	conn, err := serverConnection(cfg, opts.Host, opts.Port)
	if err != nil {
		return opts, "", nil, nil, err
	}
	b, err := marshalXray(cfg)
	return opts, listen, b, &conn, err
}

func prepareExistingXrayServer(old []byte, opts ProxyServerOptions, explicitSNI bool, listen string) (ProxyServerOptions, string, []byte, *ProxyConnection, error) {
	cfg, err := decodeXray(old)
	if err != nil {
		return opts, "", nil, nil, err
	}
	conn, err := serverConnection(cfg, opts.Host, opts.Port)
	if err != nil {
		return opts, "", nil, nil, err
	}
	i := &cfg.Inbounds[0]
	oldListen := net.JoinHostPort(i.Listen, strconv.Itoa(i.Port))
	if !opts.ListenSet {
		listen = oldListen
	}
	if opts.ExportOnly {
		return opts, oldListen, old, &conn, nil
	}
	if err := validateManagedXrayServer(cfg); err != nil {
		return opts, "", nil, nil, err
	}
	target := i.Stream.Reality.Target
	if target == "" {
		target = i.Stream.Reality.Dest
	}
	if !explicitSNI {
		opts.SNI = conn.SNI
	}
	if target != opts.Target || opts.SNI != conn.SNI {
		return opts, "", nil, nil, fmt.Errorf("requested target or SNI conflicts with the existing server; existing clients must remain valid")
	}
	existingProxy := i.Stream.Raw != nil && i.Stream.Raw.AcceptProxyProtocol
	if opts.ProxyProtocolSet && opts.ProxyProtocol != existingProxy {
		return opts, "", nil, nil, fmt.Errorf("requested PROXY protocol mode conflicts with the existing server")
	}
	opts.ProxyProtocol = existingProxy
	if err := validateServerListener(listen, opts.ProxyProtocol); err != nil {
		return opts, "", nil, nil, err
	}
	if listen == oldListen {
		return opts, listen, old, &conn, nil
	}
	i.Listen, i.Port, err = proxyListener(listen)
	if err != nil {
		return opts, "", nil, nil, err
	}
	b, err := marshalXray(cfg)
	return opts, listen, b, &conn, err
}

func validateServerListener(listen string, proxyProtocol bool) error {
	h, _, err := proxyListener(listen)
	if err != nil {
		return err
	}
	if proxyProtocol && !net.ParseIP(h).IsLoopback() {
		return fmt.Errorf("--proxy-protocol requires a loopback listener behind a trusted frontend")
	}
	return nil
}

func PlanXrayClient(opts ProxyClientOptions, stdin io.Reader) (*output.Plan, error) {
	r, err := newProxyRuntime()
	if err != nil {
		return nil, err
	}
	_, listen, _, err := prepareXrayClient(r, opts, stdin)
	if err != nil {
		return nil, err
	}
	plan := proxyPlan("client", r, listen)
	if opts.Replace {
		plan.Confirmation = "required"
		plan.Actions[1].Description = "Replace the requested client configuration with explicit --replace --yes confirmation"
	}
	return plan, nil
}

func PlanXrayServer(opts ProxyServerOptions) (*output.Plan, error) {
	r, err := newProxyRuntime()
	if err != nil {
		return nil, err
	}
	_, listen, _, _, err := prepareXrayServer(r, opts, false)
	if err != nil {
		return nil, err
	}
	plan := proxyPlan("server", r, listen)
	if opts.ExportOnly {
		plan.Actions = []output.Action{{Step: 1, Description: "Read existing server credentials and explicitly export the connection URI"}}
		plan.Affects = nil
		plan.Expected = "Existing server deployment and credentials remain unchanged"
	}
	return plan, nil
}

func proxyPlan(role string, r proxyRuntime, listen string) *output.Plan {
	return &output.Plan{API: 1, Command: "pig build proxy " + role, Boundary: "Xray configuration and its managed service", Confirmation: "none", Actions: []output.Action{{Step: 1, Description: "Install or verify Xray"}, {Step: 2, Description: "Reuse existing credentials; validate and apply configuration only when needed"}, {Step: 3, Description: "Start and enable the service; verify its operation"}}, Affects: []output.Resource{{Type: "file", Name: r.config}, {Type: "file", Name: r.serviceFile}, {Type: "listener", Name: listen}}, Expected: "Xray is ready; existing server credentials are preserved"}
}

type proxySnapshot struct {
	path     string
	data     []byte
	mode     os.FileMode
	uid, gid int
	exists   bool
}

func snapshotProxyFile(path string) (proxySnapshot, error) {
	s := proxySnapshot{path: path, uid: os.Getuid(), gid: os.Getgid()}
	b, err := readProxyFile(path)
	if err != nil {
		return s, err
	}
	if b == nil {
		return s, nil
	}
	i, err := os.Stat(path)
	if err != nil {
		return s, fmt.Errorf("cannot inspect original proxy file")
	}
	s.data, s.mode, s.exists = b, i.Mode().Perm(), true
	if stat, ok := i.Sys().(*syscall.Stat_t); ok {
		s.uid, s.gid = int(stat.Uid), int(stat.Gid)
	}
	return s, nil
}

func (s proxySnapshot) restore() error {
	if !s.exists {
		if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	if err := utils.AtomicWriteFile(s.path, s.data, s.mode); err != nil {
		return err
	}
	return os.Chown(s.path, s.uid, s.gid)
}

func (r proxyRuntime) lock() (func(), error) {
	if err := os.MkdirAll(filepath.Dir(r.config), 0700); err != nil {
		return nil, fmt.Errorf("cannot create proxy configuration directory")
	}
	path := filepath.Join(filepath.Dir(r.config), ".pig-xray.lock")
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return nil, fmt.Errorf("proxy lock must be a regular file")
	}
	fd, err := syscall.Open(path, syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, fmt.Errorf("cannot acquire proxy lock")
	}
	f := os.NewFile(uintptr(fd), path)
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("another proxy setup is running")
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
}

func (r proxyRuntime) writeConfig(path string, b []byte) error {
	mode := os.FileMode(0600)
	if r.os == "linux" {
		mode = 0640
	}
	if err := utils.AtomicWriteFile(path, b, mode); err != nil {
		return fmt.Errorf("cannot write protected Xray configuration")
	}
	if r.os == "linux" {
		if err := r.run("chown", "root:xray", path); err != nil {
			return fmt.Errorf("cannot grant the Xray service access to its configuration")
		}
	}
	return nil
}

func checkProxyPort(r proxyRuntime, listen string, old []byte) error {
	l, err := net.Listen("tcp", listen)
	if err == nil {
		return l.Close()
	}
	if old != nil && r.active() {
		cfg, e := decodeXray(old)
		if e == nil && len(cfg.Inbounds) == 1 && net.JoinHostPort(cfg.Inbounds[0].Listen, strconv.Itoa(cfg.Inbounds[0].Port)) == listen {
			return nil
		}
	}
	return fmt.Errorf("requested listener is unavailable; keep the current service or choose another --listen address")
}

func verifyProxyClient(listen string) error {
	h, p, err := proxyListener(listen)
	if err != nil {
		return err
	}
	if net.ParseIP(h).IsUnspecified() {
		if net.ParseIP(h).To4() != nil {
			h = "127.0.0.1"
		} else {
			h = "::1"
		}
	}
	endpoint := net.JoinHostPort(h, strconv.Itoa(p))
	for _, scheme := range []string{"http", "socks5h"} {
		ok := false
		for attempt := 0; attempt < 3; attempt++ {
			proxyURL := &url.URL{Scheme: scheme, Host: endpoint}
			transport := &http.Transport{Proxy: http.ProxyURL(proxyURL), TLSHandshakeTimeout: 10 * time.Second, DisableKeepAlives: true}
			client := &http.Client{Transport: transport, Timeout: 15 * time.Second}
			response, requestErr := client.Get("https://www.google.com/generate_204")
			if requestErr == nil {
				_ = response.Body.Close()
				ok = response.StatusCode == http.StatusNoContent
			}
			transport.CloseIdleConnections()
			if ok {
				break
			}
			time.Sleep(300 * time.Millisecond)
		}
		if !ok {
			return fmt.Errorf("%s HTTPS proxy check failed; check server connection and outbound HTTPS access", scheme)
		}
	}
	return nil
}

func waitProxyListener(r proxyRuntime, listen string) error {
	h, p, err := proxyListener(listen)
	if err != nil {
		return err
	}
	if net.ParseIP(h).IsUnspecified() {
		if net.ParseIP(h).To4() != nil {
			h = "127.0.0.1"
		} else {
			h = "::1"
		}
	}
	for n := 0; n < 20; n++ {
		c, err := net.DialTimeout("tcp", net.JoinHostPort(h, strconv.Itoa(p)), 300*time.Millisecond)
		if err == nil {
			_ = c.Close()
			if r.active() {
				return nil
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("xray server listener did not become ready")
}

func proxyShell(listen string) []byte {
	return []byte(fmt.Sprintf("# Managed by pig build proxy client\nalias po='export HTTP_PROXY=http://%s HTTPS_PROXY=http://%s ALL_PROXY=http://%s http_proxy=http://%s https_proxy=http://%s all_proxy=http://%s'\nalias px='unset HTTP_PROXY HTTPS_PROXY ALL_PROXY NO_PROXY http_proxy https_proxy all_proxy no_proxy'\nalias pck='curl -I --connect-timeout 10 https://www.google.com'\n", listen, listen, listen, listen, listen, listen))
}

func SetupXrayClient(opts ProxyClientOptions, stdin io.Reader) (*ProxySetupResult, error) {
	r, err := newProxyRuntime()
	if err != nil {
		return nil, err
	}
	// Consume stdin before any privilege, installation, or service operation.
	c, err := clientConnection(opts, stdin)
	if err != nil {
		return nil, err
	}
	// Canonicalize once so file/stdin input is not consumed by a second read.
	opts.URI, opts.From, opts.Server, opts.ID, opts.SNI, opts.PublicKey, opts.ShortID, opts.PQV = c.URI(), "", "", "", "", "", "", ""
	if _, _, _, err := prepareXrayClient(r, opts, strings.NewReader("")); err != nil {
		return nil, err
	}
	if opts.Replace && !opts.Yes {
		return nil, fmt.Errorf("replacement requires --replace --yes")
	}
	return setupPreparedXrayClient(r, opts)
}

func setupPreparedXrayClient(r proxyRuntime, opts ProxyClientOptions) (*ProxySetupResult, error) {
	if err := r.checkPlatform("client"); err != nil {
		return nil, err
	}
	unlock, err := r.lock()
	if err != nil {
		return nil, err
	}
	defer unlock()
	_, listen, candidate, err := prepareXrayClient(r, opts, os.Stdin)
	if err != nil {
		return nil, err
	}
	if opts.Replace && !opts.Yes {
		return nil, fmt.Errorf("replacement requires --replace --yes")
	}
	return r.apply("client", listen, candidate, opts.Replace, nil)
}

func SetupXrayServer(opts ProxyServerOptions, stdout io.Writer) (*ProxySetupResult, error) {
	r, err := newProxyRuntime()
	if err != nil {
		return nil, err
	}
	opts, listen, _, conn, err := prepareXrayServer(r, opts, false)
	if err != nil {
		return nil, err
	}
	if opts.ExportOnly {
		if err := exportProxyConnection(*conn, opts.Export, stdout); err != nil {
			return nil, err
		}
		return &ProxySetupResult{Role: "server", Listen: listen, Config: r.config, Export: opts.Export}, nil
	}
	if err := r.checkPlatform("server"); err != nil {
		return nil, err
	}
	unlock, err := r.lock()
	if err != nil {
		return nil, err
	}
	defer unlock()
	opts, listen, candidate, conn, err := prepareXrayServer(r, opts, true)
	if err != nil {
		return nil, err
	}
	if err := checkProxyExport(*conn, opts.Export); err != nil {
		return nil, err
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	target, err := tls.DialWithDialer(dialer, "tcp", opts.Target, &tls.Config{ServerName: opts.SNI, MinVersion: tls.VersionTLS13, NextProtos: []string{"h2"}})
	if err != nil {
		return nil, fmt.Errorf("REALITY target TLS check failed")
	}
	protocol := target.ConnectionState().NegotiatedProtocol
	_ = target.Close()
	if protocol != "h2" {
		return nil, fmt.Errorf("REALITY target must support TLS 1.3 and HTTP/2")
	}
	result, err := r.apply("server", listen, candidate, false, func() error { return exportProxyConnection(*conn, opts.Export, stdout) })
	if err != nil {
		return nil, err
	}
	result.Export = opts.Export
	return result, nil
}

func checkProxyExport(c ProxyConnection, destination string) error {
	if destination == "" || destination == "-" {
		return nil
	}
	b, err := readProxyFile(destination)
	if err != nil {
		return err
	}
	if b != nil {
		i, err := os.Stat(destination)
		if err != nil || i.Mode().Perm() != 0600 {
			return fmt.Errorf("existing export file must have mode 0600")
		}
		if string(b) != c.URI()+"\n" {
			return fmt.Errorf("existing export file contains different connection information")
		}
	}
	return nil
}

func exportProxyConnection(c ProxyConnection, destination string, stdout io.Writer) error {
	if err := checkProxyExport(c, destination); err != nil {
		return err
	}
	if destination == "" {
		return nil
	}
	if destination == "-" {
		_, err := fmt.Fprintln(stdout, c.URI())
		return err
	}
	if b, err := readProxyFile(destination); err != nil {
		return err
	} else if b != nil {
		return nil
	}
	// O_EXCL makes concurrent exports fail instead of silently replacing another file.
	f, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("cannot create protected connection export")
	}
	if _, err := io.WriteString(f, c.URI()+"\n"); err != nil {
		f.Close()
		_ = os.Remove(destination)
		return fmt.Errorf("cannot write connection export")
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(destination)
		return fmt.Errorf("cannot close connection export")
	}
	return nil
}
