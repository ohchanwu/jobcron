package ai

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

// pinnedTransport returns an http.Transport whose DialContext refuses to open
// a TCP connection to any host other than allowedHost. The check runs at the
// dial layer — before a connection opens — so a redirect, a maliciously-set
// base URL, or a prompt-injection attempt that somehow steered the request
// elsewhere cannot exfiltrate the API key or posting data to another host.
//
// A request-URL check would not be enough: it runs after the decision to
// connect and can be bypassed by a 3xx redirect to a different host. Pinning
// in DialContext gates the actual connect.
//
// allowedHost is the bare hostname (no port), e.g. "api.anthropic.com". The
// dial addr arrives as "host:port"; the port is ignored in the comparison.
func pinnedTransport(allowedHost string) *http.Transport {
	base := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	return &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				host = addr // no port present — compare the whole addr
			}
			if host != allowedHost {
				return nil, fmt.Errorf("ai: egress to %q refused; client is pinned to %q", host, allowedHost)
			}
			return base.DialContext(ctx, network, addr)
		},
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          4,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
}

// newPinnedHTTPClient builds an *http.Client whose transport refuses every
// host but allowedHost. timeout bounds the whole request.
func newPinnedHTTPClient(allowedHost string, timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: pinnedTransport(allowedHost),
		Timeout:   timeout,
	}
}
