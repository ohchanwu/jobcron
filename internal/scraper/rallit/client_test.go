package rallit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestClientGetRateLimits(t *testing.T) {
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
	c := newClient(srv.URL, 40*time.Millisecond)
	for range 2 {
		if _, err := c.get(context.Background(), "/jobs"); err != nil {
			t.Fatalf("get: %v", err)
		}
	}
	if gap := times[1].Sub(times[0]); gap < 30*time.Millisecond {
		t.Fatalf("request gap = %v, want at least 30ms", gap)
	}
}

func TestCheckAccessRejectsWhenDisallowed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			_, _ = w.Write([]byte("User-agent: *\nDisallow: /api\n"))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c := newClient(srv.URL, 0)
	if err := c.checkAccess(context.Background()); err == nil {
		t.Error("checkAccess = nil, want an error when robots.txt disallows /api")
	}
}
