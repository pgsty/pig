package build

import (
	"crypto/ecdh"
	"crypto/mldsa"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/gofrs/uuid/v5"
)

const (
	DefaultProxyListen  = "127.0.0.1:12345"
	defaultProxyBackend = "127.0.0.1:9443"
	visionFlow          = "xtls-rprx-vision"
	proxyInputLimit     = 64 * 1024
)

// ProxyConnection is sensitive input. Never put it in a result or diagnostic.
type ProxyConnection struct {
	Server, ID, SNI, PublicKey, ShortID, PQV string
	Port                                     int
}

type ProxyClientOptions struct {
	URI, From, Server, ID, SNI, PublicKey, ShortID, PQV, Listen string
	ListenSet, Replace, Yes                                     bool
}

type ProxyServerOptions struct {
	Host, Target, SNI, Listen, Export                      string
	Port                                                   int
	ListenSet, ProxyProtocol, ProxyProtocolSet, ExportOnly bool
}

type xrayConfig struct {
	Log       xrayLog         `json:"log"`
	Inbounds  []xrayInbound   `json:"inbounds"`
	Outbounds []xrayOutbound  `json:"outbounds"`
	Routing   json.RawMessage `json:"routing,omitempty"`
}

type xrayLog struct {
	Level  string `json:"loglevel"`
	Access string `json:"access"`
}

type xrayInbound struct {
	Tag      string       `json:"tag"`
	Listen   string       `json:"listen"`
	Port     int          `json:"port"`
	Protocol string       `json:"protocol"`
	Settings xraySettings `json:"settings"`
	Stream   *xrayStream  `json:"streamSettings,omitempty"`
}

type xrayOutbound struct {
	Tag      string        `json:"tag"`
	Protocol string        `json:"protocol"`
	Settings *xraySettings `json:"settings,omitempty"`
	Stream   *xrayStream   `json:"streamSettings,omitempty"`
	Mux      *xrayMux      `json:"mux,omitempty"`
}

type xraySettings struct {
	Auth       string       `json:"auth,omitempty"`
	UDP        bool         `json:"udp,omitempty"`
	IP         string       `json:"ip,omitempty"`
	Decryption string       `json:"decryption,omitempty"`
	Clients    []xrayUser   `json:"clients,omitempty"`
	VNext      []xrayRemote `json:"vnext,omitempty"`
}

type xrayUser struct {
	ID         string `json:"id"`
	Flow       string `json:"flow,omitempty"`
	Encryption string `json:"encryption,omitempty"`
}

type xrayRemote struct {
	Address string     `json:"address"`
	Port    int        `json:"port"`
	Users   []xrayUser `json:"users"`
}

type xrayStream struct {
	Network  string      `json:"network"`
	Security string      `json:"security"`
	Raw      *xrayRaw    `json:"rawSettings,omitempty"`
	Reality  xrayReality `json:"realitySettings"`
}

type xrayRaw struct {
	AcceptProxyProtocol bool `json:"acceptProxyProtocol"`
}
type xrayMux struct {
	Enabled bool `json:"enabled"`
}
type xrayReality struct {
	Show        bool     `json:"show,omitempty"`
	Target      string   `json:"target,omitempty"`
	Dest        string   `json:"dest,omitempty"`
	ServerNames []string `json:"serverNames,omitempty"`
	PrivateKey  string   `json:"privateKey,omitempty"`
	ShortIDs    []string `json:"shortIds,omitempty"`
	MLDSASeed   string   `json:"mldsa65Seed,omitempty"`
	Xver        int      `json:"xver,omitempty"`
	ServerName  string   `json:"serverName,omitempty"`
	Fingerprint string   `json:"fingerprint,omitempty"`
	Password    string   `json:"password,omitempty"`
	PublicKey   string   `json:"publicKey,omitempty"`
	ShortID     string   `json:"shortId,omitempty"`
	MLDSAVerify string   `json:"mldsa65Verify,omitempty"`
	SpiderX     string   `json:"spiderX,omitempty"`
}

func validProxyHost(host string) bool {
	if net.ParseIP(host) != nil {
		return true
	}
	if host == "" || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}

func proxyEndpoint(value string) (string, int, error) {
	host, port, err := net.SplitHostPort(value)
	if err != nil || !validProxyHost(host) {
		return "", 0, fmt.Errorf("expected a valid host:port endpoint")
	}
	p, err := strconv.Atoi(port)
	if err != nil || p < 1 || p > 65535 {
		return "", 0, fmt.Errorf("port must be between 1 and 65535")
	}
	return host, p, nil
}

func proxyListener(value string) (string, int, error) {
	h, p, err := proxyEndpoint(value)
	if err != nil {
		return "", 0, fmt.Errorf("invalid --listen: expected an IP address and port")
	}
	if h == "localhost" {
		h = "127.0.0.1"
	}
	if net.ParseIP(h) == nil {
		return "", 0, fmt.Errorf("--listen must use an IP address or localhost")
	}
	return h, p, nil
}

func rawKey(value string, size int) ([]byte, error) {
	b, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(b) != size {
		return nil, fmt.Errorf("invalid authentication material")
	}
	return b, nil
}

func (c ProxyConnection) validate() error {
	if !validProxyHost(c.Server) || c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("invalid server endpoint")
	}
	if _, err := uuid.FromString(c.ID); err != nil {
		return fmt.Errorf("invalid client UUID")
	}
	if !validProxyHost(c.SNI) {
		return fmt.Errorf("invalid REALITY server name")
	}
	if _, err := rawKey(c.PublicKey, 32); err != nil {
		return fmt.Errorf("invalid REALITY public key")
	}
	b, err := hex.DecodeString(c.ShortID)
	if err != nil || len(b) > 8 || len(b) == 0 {
		return fmt.Errorf("short ID must contain 2 to 16 hexadecimal characters with even length")
	}
	if c.PQV != "" {
		b, err := rawKey(c.PQV, 1952)
		if err != nil {
			return fmt.Errorf("invalid ML-DSA-65 verification key")
		}
		if _, err := mldsa.NewPublicKey(mldsa.MLDSA65(), b); err != nil {
			return fmt.Errorf("invalid ML-DSA-65 verification key")
		}
	}
	return nil
}

// ParseProxyURI accepts only the supported REALITY/RAW/Vision connection contract.
func ParseProxyURI(value string) (ProxyConnection, error) {
	var c ProxyConnection
	u, err := url.Parse(strings.TrimSpace(value))
	if err != nil || u.Scheme != "vless" || u.User == nil || u.Path != "" || u.Opaque != "" {
		return c, fmt.Errorf("expected a VLESS REALITY connection URI")
	}
	if _, hasPassword := u.User.Password(); hasPassword {
		return c, fmt.Errorf("VLESS userinfo must contain only a UUID")
	}
	c.Server, c.Port, err = proxyEndpoint(u.Host)
	if err != nil {
		return c, err
	}
	c.ID = u.User.Username()
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return c, fmt.Errorf("invalid URI query")
	}
	allowed := map[string]string{"encryption": "none", "security": "reality", "type": "tcp", "flow": visionFlow, "fp": "chrome", "spx": "/"}
	for key, values := range q {
		if len(values) != 1 {
			return c, fmt.Errorf("duplicate connection parameter")
		}
		if expected, ok := allowed[key]; ok {
			if values[0] != expected {
				return c, fmt.Errorf("unsupported connection parameter value")
			}
		} else if key != "sni" && key != "pbk" && key != "sid" && key != "pqv" {
			return c, fmt.Errorf("unsupported connection parameter")
		}
	}
	if q.Get("security") != "reality" || q.Get("flow") != visionFlow {
		return c, fmt.Errorf("URI must select REALITY security and Vision flow")
	}
	c.SNI, c.PublicKey, c.ShortID, c.PQV = q.Get("sni"), q.Get("pbk"), q.Get("sid"), q.Get("pqv")
	return c, c.validate()
}

func (c ProxyConnection) URI() string {
	q := url.Values{"encryption": {"none"}, "security": {"reality"}, "type": {"tcp"}, "flow": {visionFlow}, "sni": {c.SNI}, "fp": {"chrome"}, "pbk": {c.PublicKey}, "sid": {c.ShortID}}
	if c.PQV != "" {
		q.Set("pqv", c.PQV)
	}
	u := url.URL{Scheme: "vless", User: url.User(c.ID), Host: net.JoinHostPort(c.Server, strconv.Itoa(c.Port)), RawQuery: q.Encode()}
	return u.String()
}

func clientConnection(opts ProxyClientOptions, stdin io.Reader) (ProxyConnection, error) {
	direct := opts.Server != "" || opts.ID != "" || opts.SNI != "" || opts.PublicKey != "" || opts.ShortID != "" || opts.PQV != ""
	forms := 0
	for _, present := range []bool{opts.URI != "", opts.From != "", direct} {
		if present {
			forms++
		}
	}
	if forms != 1 {
		return ProxyConnection{}, fmt.Errorf("choose one connection input: URI, --from, or connection flags")
	}
	if opts.From != "" {
		reader := stdin
		if opts.From != "-" {
			f, err := os.Open(opts.From)
			if err != nil {
				return ProxyConnection{}, fmt.Errorf("cannot read connection file")
			}
			defer f.Close()
			reader = f
		}
		b, err := io.ReadAll(io.LimitReader(reader, proxyInputLimit+1))
		if err != nil || len(b) > proxyInputLimit {
			return ProxyConnection{}, fmt.Errorf("cannot read connection input or input exceeds 64 KiB")
		}
		opts.URI = strings.TrimSpace(string(b))
	}
	if opts.URI != "" {
		return ParseProxyURI(opts.URI)
	}
	c := ProxyConnection{ID: opts.ID, SNI: opts.SNI, PublicKey: opts.PublicKey, ShortID: opts.ShortID, PQV: opts.PQV}
	var err error
	c.Server, c.Port, err = proxyEndpoint(opts.Server)
	if err != nil {
		return c, err
	}
	return c, c.validate()
}

func renderXrayClient(c ProxyConnection, listen string) ([]byte, error) {
	h, p, err := proxyListener(listen)
	if err != nil {
		return nil, err
	}
	cfg := xrayConfig{Log: xrayLog{"warning", "none"}, Inbounds: []xrayInbound{{Tag: "pig-proxy-in", Listen: h, Port: p, Protocol: "socks", Settings: xraySettings{Auth: "noauth", UDP: true, IP: h}}}, Outbounds: []xrayOutbound{{Tag: "pig-proxy-out", Protocol: "vless", Settings: &xraySettings{VNext: []xrayRemote{{Address: c.Server, Port: c.Port, Users: []xrayUser{{ID: c.ID, Flow: visionFlow, Encryption: "none"}}}}}, Stream: &xrayStream{Network: "raw", Security: "reality", Reality: xrayReality{ServerName: c.SNI, Fingerprint: "chrome", Password: c.PublicKey, ShortID: c.ShortID, MLDSAVerify: c.PQV, SpiderX: "/"}}, Mux: &xrayMux{false}}}}
	return marshalXray(cfg)
}

func marshalXray(cfg xrayConfig) ([]byte, error) {
	b, err := json.MarshalIndent(cfg, "", "  ")
	return append(b, '\n'), err
}

func decodeXray(b []byte) (xrayConfig, error) {
	var cfg xrayConfig
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.DisallowUnknownFields()
	if err := d.Decode(&cfg); err != nil {
		return cfg, fmt.Errorf("existing Xray configuration has an unsupported shape")
	}
	if d.Decode(new(any)) != io.EOF {
		return cfg, fmt.Errorf("existing Xray configuration is not one JSON value")
	}
	return cfg, nil
}

func inspectXrayClient(b []byte) (ProxyConnection, string, error) {
	var c ProxyConnection
	cfg, err := decodeXray(b)
	if err != nil {
		return c, "", err
	}
	if len(cfg.Inbounds) != 1 || len(cfg.Outbounds) != 1 {
		return c, "", fmt.Errorf("expected one supported client inbound and outbound")
	}
	if cfg.Log != (xrayLog{"warning", "none"}) || len(cfg.Routing) != 0 {
		return c, "", fmt.Errorf("existing client logging or routing is not managed by PIG")
	}
	i, o := cfg.Inbounds[0], cfg.Outbounds[0]
	if i.Protocol != "socks" || i.Settings.Auth != "noauth" || !i.Settings.UDP || o.Protocol != "vless" || o.Settings == nil || len(o.Settings.VNext) != 1 || o.Stream == nil || o.Mux == nil || o.Mux.Enabled {
		return c, "", fmt.Errorf("existing client configuration is not supported")
	}
	v, s := o.Settings.VNext[0], o.Stream
	if len(v.Users) != 1 || v.Users[0].Flow != visionFlow || v.Users[0].Encryption != "none" || s.Security != "reality" || s.Network != "raw" && s.Network != "tcp" || s.Reality.Fingerprint != "chrome" || s.Reality.SpiderX != "/" {
		return c, "", fmt.Errorf("existing client connection is not supported")
	}
	r := s.Reality
	key := r.Password
	if key == "" {
		key = r.PublicKey
	}
	c = ProxyConnection{Server: v.Address, Port: v.Port, ID: v.Users[0].ID, SNI: r.ServerName, PublicKey: key, ShortID: r.ShortID, PQV: r.MLDSAVerify}
	listen := net.JoinHostPort(i.Listen, strconv.Itoa(i.Port))
	if _, _, err := proxyListener(listen); err != nil {
		return c, "", err
	}
	return c, listen, c.validate()
}

func validateManagedXrayServer(cfg xrayConfig) error {
	if cfg.Log != (xrayLog{"warning", "none"}) || len(cfg.Outbounds) != 2 {
		return fmt.Errorf("existing server logging or outbounds are not managed by PIG")
	}
	direct, blocked := cfg.Outbounds[0], cfg.Outbounds[1]
	if direct.Protocol != "freedom" || direct.Tag != "direct" || blocked.Protocol != "blackhole" || blocked.Tag != "blocked" || direct.Settings != nil || direct.Stream != nil || direct.Mux != nil || blocked.Settings != nil || blocked.Stream != nil || blocked.Mux != nil {
		return fmt.Errorf("existing server outbounds are not managed by PIG")
	}
	var routing, expected any
	if json.Unmarshal(cfg.Routing, &routing) != nil {
		return fmt.Errorf("existing server routing is invalid")
	}
	_ = json.Unmarshal([]byte(`{"domainStrategy":"IPIfNonMatch","rules":[{"type":"field","ip":["geoip:private"],"outboundTag":"blocked"}]}`), &expected)
	if !reflect.DeepEqual(routing, expected) {
		return fmt.Errorf("existing server routing is not managed by PIG")
	}
	return nil
}

func serverConnection(cfg xrayConfig, host string, port int) (ProxyConnection, error) {
	var c ProxyConnection
	if len(cfg.Inbounds) != 1 {
		return c, fmt.Errorf("expected one supported server inbound")
	}
	i := cfg.Inbounds[0]
	if i.Protocol != "vless" || i.Settings.Decryption != "none" || len(i.Settings.Clients) != 1 || i.Settings.Clients[0].Flow != visionFlow || i.Stream == nil || i.Stream.Security != "reality" || i.Stream.Network != "raw" && i.Stream.Network != "tcp" {
		return c, fmt.Errorf("existing server connection is unsupported or ambiguous")
	}
	r := i.Stream.Reality
	if len(r.ServerNames) != 1 || len(r.ShortIDs) != 1 {
		return c, fmt.Errorf("server SNI or short ID is ambiguous")
	}
	b, err := rawKey(r.PrivateKey, 32)
	if err != nil {
		return c, fmt.Errorf("invalid existing server private key")
	}
	k, err := ecdh.X25519().NewPrivateKey(b)
	if err != nil {
		return c, fmt.Errorf("invalid existing server private key")
	}
	c = ProxyConnection{Server: host, Port: port, ID: i.Settings.Clients[0].ID, SNI: r.ServerNames[0], PublicKey: base64.RawURLEncoding.EncodeToString(k.PublicKey().Bytes()), ShortID: r.ShortIDs[0]}
	if r.MLDSASeed != "" {
		seed, err := rawKey(r.MLDSASeed, 32)
		if err != nil {
			return c, fmt.Errorf("invalid existing ML-DSA seed")
		}
		key, err := mldsa.NewPrivateKey(mldsa.MLDSA65(), seed)
		if err != nil {
			return c, fmt.Errorf("cannot derive ML-DSA verification material")
		}
		c.PQV = base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes())
	}
	return c, c.validate()
}

func newXrayServer(opts ProxyServerOptions, listen string) (xrayConfig, error) {
	h, p, err := proxyListener(listen)
	if err != nil {
		return xrayConfig{}, err
	}
	id, err := uuid.NewV4()
	if err != nil {
		return xrayConfig{}, fmt.Errorf("cannot generate client UUID")
	}
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return xrayConfig{}, fmt.Errorf("cannot generate REALITY keys")
	}
	postQuantum, err := mldsa.GenerateKey(mldsa.MLDSA65())
	if err != nil {
		return xrayConfig{}, fmt.Errorf("cannot generate ML-DSA material")
	}
	sid := make([]byte, 8)
	if _, err := rand.Read(sid); err != nil {
		return xrayConfig{}, fmt.Errorf("cannot generate short ID")
	}
	stream := &xrayStream{Network: "raw", Security: "reality", Reality: xrayReality{Target: opts.Target, ServerNames: []string{opts.SNI}, PrivateKey: base64.RawURLEncoding.EncodeToString(key.Bytes()), ShortIDs: []string{hex.EncodeToString(sid)}, MLDSASeed: base64.RawURLEncoding.EncodeToString(postQuantum.Bytes())}}
	if opts.ProxyProtocol {
		stream.Raw = &xrayRaw{true}
	}
	cfg := xrayConfig{Log: xrayLog{"warning", "none"}, Inbounds: []xrayInbound{{Tag: "pig-reality-in", Listen: h, Port: p, Protocol: "vless", Settings: xraySettings{Decryption: "none", Clients: []xrayUser{{ID: id.String(), Flow: visionFlow}}}, Stream: stream}}, Outbounds: []xrayOutbound{{Tag: "direct", Protocol: "freedom"}, {Tag: "blocked", Protocol: "blackhole"}}, Routing: json.RawMessage(`{"domainStrategy":"IPIfNonMatch","rules":[{"type":"field","ip":["geoip:private"],"outboundTag":"blocked"}]}`)}
	return cfg, nil
}
