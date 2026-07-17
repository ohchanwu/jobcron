package worknet

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"
)

func TestCallRateLimits(t *testing.T) {
	var (
		mu    sync.Mutex
		times []time.Time
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		times = append(times, time.Now())
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := newClient(srv.URL, "test-key", 40*time.Millisecond)
	for range 2 {
		if _, err := c.call(context.Background(), url.Values{}); err != nil {
			t.Fatalf("call: %v", err)
		}
	}
	if gap := times[1].Sub(times[0]); gap < 30*time.Millisecond {
		t.Fatalf("request gap = %v, want at least 30ms", gap)
	}
}

func TestCallUsesCanonicalUserAgent(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Header.Get("User-Agent"), "jobcron/0.1 (+github.com/ohchanwu/jobcron)"; got != want {
			t.Errorf("User-Agent = %q, want %q", got, want)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newClient(server.URL, "test-key", 0)
	if _, err := client.call(context.Background(), url.Values{"callTp": {"L"}}); err != nil {
		t.Fatalf("call: %v", err)
	}
}
