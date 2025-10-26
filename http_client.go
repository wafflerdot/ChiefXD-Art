package main

import (
	"net"
	"net/http"
	"time"
)

// Shared HTTP client and transport for connection reuse across outbound requests.
// Using a single Transport significantly reduces TLS handshakes and improves latency
// under repeated calls to the same hosts.

// sharedTransport is reused by all clients; customise knobs as needed.
var sharedTransport = &http.Transport{
	Proxy:               http.ProxyFromEnvironment,
	DialContext:         (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
	ForceAttemptHTTP2:   true,
	MaxIdleConns:        100,
	MaxIdleConnsPerHost: 10,
	IdleConnTimeout:     90 * time.Second,
	TLSHandshakeTimeout: 10 * time.Second,
}

// sharedHTTPClient is a default client with a sensible timeout.
var sharedHTTPClient = &http.Client{
	Timeout:   30 * time.Second,
	Transport: sharedTransport,
}

// newHTTPClientWithTimeout returns a client that reuses the shared transport but
// with a custom timeout, useful for APIs that need different request ceilings.
func newHTTPClientWithTimeout(d time.Duration) *http.Client {
	return &http.Client{Timeout: d, Transport: sharedTransport}
}
