package server

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/ohchanwu/jobcron/internal/profile"
)

func TestPostingNativeListenerAdmissionAndRelease(t *testing.T) {
	srv, st := newPostgresTestServer(t, &fakeScraper{})
	srv.SetProductionMode(true)
	server := httptest.NewServer(srv.Handler())
	defer server.Close()
	var responses []*http.Response
	var cancelers []context.CancelFunc
	defer func() {
		for _, cancel := range cancelers {
			cancel()
		}
		for _, r := range responses {
			r.Body.Close()
		}
	}()
	cookies := []*http.Cookie{}
	for i := 0; i < 8; i++ {
		uid, cookie := createSessionUser(t, st, fmt.Sprintf("listener-%d@example.invalid", i), fmt.Sprintf("listener-%d-synthetic-session", i))
		saveAIRuntimeProfile(t, st, uid, profile.Profile{})
		cookies = append(cookies, cookie)
		for j := 0; j < 8; j++ {
			ctx, cancel := context.WithCancel(context.Background())
			cancelers = append(cancelers, cancel)
			req, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/api/posting-updates", nil)
			req.AddCookie(cookie)
			response, err := server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			responses = append(responses, response)
			if response.StatusCode != 200 {
				t.Fatal(response.StatusCode)
			}
			scan := bufio.NewScanner(response.Body)
			if !scan.Scan() || scan.Text() != "event: dirty" {
				t.Fatal("native observer did not flush")
			}
		}
	}
	req, _ := http.NewRequest("GET", server.URL+"/api/posting-updates", nil)
	req.AddCookie(cookies[0])
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 429 {
		t.Fatal("65th/ninth listener bypassed bound", response.StatusCode)
	}
	srv.postingUpdates.mu.Lock()
	peak := srv.postingUpdates.total
	srv.postingUpdates.mu.Unlock()
	if peak != 64 {
		t.Fatal("native peak listeners", peak)
	}
	// The queued payload remains one slot regardless of writer burst length.
	start := time.Now()
	for i := 0; i < 1000; i++ {
		end := srv.postingUpdates.beginWrite(0, true)
		end(true, "")
	}
	writerElapsed := time.Since(start)
	for _, cancel := range cancelers {
		cancel()
	}
	for _, r := range responses {
		r.Body.Close()
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		srv.postingUpdates.mu.Lock()
		total, maps := srv.postingUpdates.total, len(srv.postingUpdates.users)
		srv.postingUpdates.mu.Unlock()
		if total == 0 && maps == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("native streams retained listener/map capacity", total, maps)
		}
		time.Sleep(time.Millisecond)
	}
	t.Logf("native admission: peak listeners=%d rejected=1 one-slot burst=1000 writer elapsed=%s; all listeners/maps released", peak, writerElapsed)
}

type postingStalledWriter struct {
	headers  http.Header
	mu       sync.Mutex
	deadline time.Time
	entered  chan struct{}
	once     sync.Once
}

func (w *postingStalledWriter) Header() http.Header { return w.headers }
func (w *postingStalledWriter) WriteHeader(int)     {}
func (w *postingStalledWriter) SetWriteDeadline(deadline time.Time) error {
	w.mu.Lock()
	w.deadline = deadline
	w.mu.Unlock()
	return nil
}
func (w *postingStalledWriter) Write(b []byte) (int, error) {
	w.once.Do(func() { close(w.entered) })
	w.mu.Lock()
	deadline := w.deadline
	w.mu.Unlock()
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	<-timer.C
	return 0, context.DeadlineExceeded
}
func (w *postingStalledWriter) Flush() {}

func TestPostingStalledTransferDoesNotHoldCommitBarrier(t *testing.T) {
	srv, _ := newTestServer(t, &fakeScraper{})
	w := &postingStalledWriter{headers: http.Header{}, entered: make(chan struct{})}
	request := httptest.NewRequest("GET", "/api/posting-updates", nil)
	ended := make(chan struct{})
	start := time.Now()
	go func() { defer close(ended); srv.Handler().ServeHTTP(w, request) }()
	select {
	case <-w.entered:
	case <-time.After(time.Second):
		t.Fatal("stalled transfer not reached")
	}
	done := make(chan struct{})
	go func() { end := srv.postingUpdates.beginWrite(0, true); end(true, ""); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("socket held the commit barrier")
	}
	select {
	case <-ended:
	case <-time.After(6 * time.Second):
		t.Fatal("five-second stalled-write deadline did not disconnect")
	}
	srv.postingUpdates.mu.Lock()
	listeners := srv.postingUpdates.total
	srv.postingUpdates.mu.Unlock()
	if listeners != 0 {
		t.Fatal("stalled observer retained admission")
	}
	t.Logf("stalled socket disconnected in %s using five-second native writer deadline; commit proceeded before disconnect", time.Since(start))
}
