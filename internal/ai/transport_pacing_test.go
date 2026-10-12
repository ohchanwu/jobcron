package ai

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

// Client preparation happens after Do. Pace transport entry, not Do invocation.
type delayedPacingJar struct {
	calls   atomic.Int64
	entered chan struct{}
}

func (j *delayedPacingJar) Cookies(*url.URL) []*http.Cookie {
	if j.calls.Add(1) == 1 {
		close(j.entered)
		time.Sleep(20 * time.Second)
	}
	return nil
}
func (j *delayedPacingJar) SetCookies(*url.URL, []*http.Cookie) {}

func TestProviderPacesActualTransportAfterClientPreparation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p, _ := newHTTPProvider(geminiSpec, "synthetic", "model", "https://synthetic.invalid", 10*time.Second)
		jar := &delayedPacingJar{entered: make(chan struct{})}
		release := make(chan struct{})
		var starts []time.Time
		var mu sync.Mutex
		p.http = &http.Client{Jar: jar, Transport: pacingTransportFunc(func(r *http.Request) (*http.Response, error) {
			mu.Lock()
			starts = append(starts, time.Now())
			t.Logf("transport after client prep=%s", time.Now().Format(time.RFC3339Nano))
			mu.Unlock()
			<-release
			return &http.Response{StatusCode: 429, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}, nil
		})}
		var wg sync.WaitGroup
		wg.Go(func() { _, _, _ = p.complete(context.Background(), "s", "u") })
		<-jar.entered
		for range 2 {
			wg.Go(func() { _, _, _ = p.complete(context.Background(), "s", "u") })
		}
		synctest.Wait()
		time.Sleep(30 * time.Second)
		synctest.Wait()
		if len(starts) != 3 {
			t.Errorf("starts=%d want3", len(starts))
		}
		for i := 1; i < len(starts); i++ {
			if gap := starts[i].Sub(starts[i-1]); gap < 10*time.Second {
				t.Errorf("actual transport gap=%v", gap)
			}
		}
		close(release)
		wg.Wait()
	})
}

func TestProviderResponseTimeoutExcludesPacingWait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p, _ := newHTTPProvider(geminiSpec, "synthetic", "model", "https://synthetic.invalid", 10*time.Second)
		p.http = &http.Client{Timeout: 15 * time.Second, Transport: pacingTransportFunc(func(r *http.Request) (*http.Response, error) {
			select {
			case <-time.After(5 * time.Second):
			case <-r.Context().Done():
				return nil, r.Context().Err()
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"{}"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)), Header: make(http.Header)}, nil
		})}
		var wg sync.WaitGroup
		for range 3 {
			wg.Go(func() {
				if _, _, err := p.complete(context.Background(), "s", "u"); err != nil {
					t.Error(err)
				}
			})
		}
		wg.Wait()
	})
}
