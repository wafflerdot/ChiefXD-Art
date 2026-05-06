package httputil

import (
	"net"
	"net/http"
	"time"
)

// Shared HTTP client and transport for connection reuse across outbound requests.
var sharedTransport = &http.Transport{
	Proxy:               http.ProxyFromEnvironment,
	DialContext:         (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
	ForceAttemptHTTP2:   true,
	MaxIdleConns:        100,
	MaxIdleConnsPerHost: 10,
	IdleConnTimeout:     90 * time.Second,
	TLSHandshakeTimeout: 10 * time.Second,
}

var SharedHTTPClient = &http.Client{
	Timeout:   30 * time.Second,
	Transport: sharedTransport,
}

// NewHTTPClientWithTimeout client that reuses shared transport but with configurable timeout
func NewHTTPClientWithTimeout(d time.Duration) *http.Client {
	return &http.Client{Timeout: d, Transport: sharedTransport}
}
