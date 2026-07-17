package jumpit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestClientGetSendsPoliteHeaders(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := newClient(srv.URL, 0)
	body, err := c.get(context.Background(), "/api/positions")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if string(body) != `{"ok":true}` {
		t.Errorf("body = %q, want the response body", body)
	}
	if ua := got.Get("User-Agent"); ua != "jobcron/0.1 (+github.com/ohchanwu/jobcron)" {
		t.Errorf("User-Agent = %q", ua)
	}
	if o := got.Get("Origin"); o != "https://jumpit.saramin.co.kr" {
		t.Errorf("Origin = %q", o)
	}
	if ref := got.Get("Referer"); ref != "https://jumpit.saramin.co.kr/" {
		t.Errorf("Referer = %q", ref)
	}
	if acc := got.Get("Accept"); acc != "application/json" {
		t.Errorf("Accept = %q", acc)
	}
}

func TestClientGetRateLimits(t *testing.T) {
	var (
		mu    sync.Mutex
		times []time.Time
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		times = append(times, time.Now())
		mu.Unlock()
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := newClient(srv.URL, 100*time.Millisecond)
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if _, err := c.get(ctx, "/api/positions"); err != nil {
			t.Fatalf("get %d: %v", i, err)
		}
	}

	if len(times) != 2 {
		t.Fatalf("server saw %d requests, want 2", len(times))
	}
	if gap := times[1].Sub(times[0]); gap < 80*time.Millisecond {
		t.Errorf("request spacing = %v, want >= ~100ms (rate limited)", gap)
	}
}

func TestClientGetErrorsOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newClient(srv.URL, 0)
	if _, err := c.get(context.Background(), "/api/positions"); err == nil {
		t.Fatal("get against a 500 response = nil error, want an error")
	}
}

func TestCheckAccessAllowsWhenRobotsAbsent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r) // a 404 robots.txt means unrestricted (RFC 9309)
	}))
	defer srv.Close()

	c := newClient(srv.URL, 0)
	c.robotsHosts = []string{srv.URL}
	if err := c.checkAccess(context.Background()); err != nil {
		t.Errorf("checkAccess = %v, want nil for a 404 robots.txt", err)
	}
}

func TestCheckAccessRejectsWhenDisallowed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.Write([]byte("User-agent: *\nDisallow: /api\n"))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c := newClient(srv.URL, 0)
	c.robotsHosts = []string{srv.URL}
	if err := c.checkAccess(context.Background()); err == nil {
		t.Error("checkAccess = nil, want an error when robots.txt disallows /api")
	}
}

func TestCheckAccessCachesRobots(t *testing.T) {
	var (
		mu   sync.Mutex
		hits int
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			mu.Lock()
			hits++
			mu.Unlock()
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c := newClient(srv.URL, 0)
	c.robotsHosts = []string{srv.URL}
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := c.checkAccess(ctx); err != nil {
			t.Fatalf("checkAccess: %v", err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if hits != 1 {
		t.Errorf("robots.txt fetched %d times across 3 checks, want 1 (24h cache)", hits)
	}
}
