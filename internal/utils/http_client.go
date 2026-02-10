package utils

import (
	"net"
	"net/http"
	"time"
)

// DefaultHTTPConnectTimeout limits connection establishment + TLS handshake, and
// also bounds how long we wait for response headers. It does NOT cap the full
// body download time (downloads may legitimately take minutes).
const DefaultHTTPConnectTimeout = 30 * time.Second

var defaultHTTPClient = newHTTPClient(DefaultHTTPConnectTimeout)

func newHTTPClient(connectTimeout time.Duration) *http.Client {
	// Clone default transport to preserve sane defaults (proxy, http2, pool, etc).
	if base, ok := http.DefaultTransport.(*http.Transport); ok {
		tr := base.Clone()
		dialer := &net.Dialer{
			Timeout:   connectTimeout,
			KeepAlive: 30 * time.Second,
		}
		tr.DialContext = dialer.DialContext
		tr.TLSHandshakeTimeout = connectTimeout
		tr.ResponseHeaderTimeout = connectTimeout
		return &http.Client{Transport: tr}
	}

	// Fallback: should be rare, but keep behavior reasonable.
	tr := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   connectTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   connectTimeout,
		ResponseHeaderTimeout: connectTimeout,
		IdleConnTimeout:       90 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		MaxIdleConns:          100,
	}
	return &http.Client{Transport: tr}
}

func defaultClient() *http.Client {
	return defaultHTTPClient
}

