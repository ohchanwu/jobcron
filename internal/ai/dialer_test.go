package ai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// TestDialerAllowsConfiguredHost proves the pin is not a blanket block: a
// client pinned to the test server's host completes a round trip.
func TestDialerAllowsConfiguredHost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "ok")
	}))
	defer srv.Close()

	host := mustHost(t, srv.URL)
	client := newPinnedHTTPClient(host, 5*time.Second)
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("pinned client to allowed host %q: %v", host, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

// TestDialerRefusesNonProviderHost drives a request through the real
// http.Client.Do path (not the closure directly) to a host the client is NOT
// pinned to, and asserts the dial is refused before any connection opens.
// This is the explicit T1 verify gate.
func TestDialerRefusesNonProviderHost(t *testing.T) {
	var reached bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		io.WriteString(w, "should never get here")
	}))
	defer srv.Close()

	// Pin to a different host than the test server actually runs on.
	client := newPinnedHTTPClient("api.anthropic.com", 5*time.Second)
	resp, err := client.Get(srv.URL) // srv is on 127.0.0.1, pin is api.anthropic.com
	if err == nil {
		resp.Body.Close()
		t.Fatal("request to non-pinned host succeeded; want dial refusal")
	}
	if !strings.Contains(err.Error(), "refused") || !strings.Contains(err.Error(), "pinned") {
		t.Fatalf("error %q does not look like the egress-pin refusal", err)
	}
	if reached {
		t.Fatal("the request reached the non-pinned server — the pin did not gate the connection")
	}
}

func mustHost(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	return u.Hostname()
}
