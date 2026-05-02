// Package proxy handles reverse proxying to backends
package proxy

import (
	"net"
	"net/http"
	"time"
)

// Transport configuration
const (
	MaxIdleConns        = 100
	MaxIdleConnsPerHost = 100
	IdleConnTimeout     = 90 * time.Second
	HandshakeTimeout    = 10 * time.Second
	ResponseTimeout     = 30 * time.Second
)

// newTransport creates an optimized HTTP transport
func newTransport() *http.Transport {
	return &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   HandshakeTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,

		MaxIdleConns:          MaxIdleConns,
		MaxIdleConnsPerHost:   MaxIdleConnsPerHost,
		IdleConnTimeout:       IdleConnTimeout,
		ResponseHeaderTimeout: ResponseTimeout,

		// ForceAttemptHTTP2 only affects h2c (cleartext HTTP/2), not ALPN-negotiated HTTP/2
		// Enable connection reuse
		DisableKeepAlives: false,

		// Pass through backend compression transparently
		DisableCompression: true,
	}
}
