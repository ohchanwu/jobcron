package ai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

type pacingTransportFunc func(*http.Request) (*http.Response, error)

func (f pacingTransportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestProviderSuggestedSpacing(t *testing.T) {
	for _, tc := range []struct {
		name string
		want time.Duration
	}{{"gemini", 10 * time.Second}, {"anthropic", 1200 * time.Millisecond}, {"openai", 1200 * time.Millisecond}, {"unknown", 1200 * time.Millisecond}} {
		if got := SuggestedRateLimit(tc.name); got != tc.want {
			t.Errorf("%s spacing=%v want %v", tc.name, got, tc.want)
		}
	}
}

// Record transport entry, not reservation time. A delayed request build must not
// let an old reservation collide with another call; responses remain overlapped.
func TestProviderActualStartsAfterDelayedBuild(t *testing.T) {
	for _, tc := range []struct {
		name    string
		spacing time.Duration
	}{{"gemini", 10 * time.Second}, {"openai", 1200 * time.Millisecond}, {"anthropic", 1200 * time.Millisecond}, {"unpaced", 0}} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				spec := geminiSpec
				spec.name = tc.name
				build := spec.buildBody
				building := make(chan struct{})
				spec.buildBody = func(model, system, user string) any {
					if user == "slow" {
						close(building)
						time.Sleep(2 * tc.spacing)
					}
					return build(model, system, user)
				}
				p, _ := newHTTPProvider(spec, "synthetic", "model", "https://synthetic.invalid", tc.spacing)
				release := make(chan struct{})
				var mu sync.Mutex
				var starts []time.Time
				p.http = &http.Client{Transport: pacingTransportFunc(func(r *http.Request) (*http.Response, error) {
					mu.Lock()
					starts = append(starts, time.Now())
					at := starts[len(starts)-1]
					mu.Unlock()
					t.Logf("actual transport %s start=%s", tc.name, at.Format(time.RFC3339Nano))
					<-release
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"{}"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)), Header: make(http.Header)}, nil
				})}
				var wg sync.WaitGroup
				wg.Go(func() { _, _, _ = p.complete(context.Background(), "system", "slow") })
				<-building
				for _, user := range []string{"fast", "fast"} {
					wg.Go(func() {
						if _, _, err := p.complete(context.Background(), "system", user); err != nil {
							t.Errorf("complete: %v", err)
						}
					})
				}
				synctest.Wait()
				if tc.spacing > 0 {
					time.Sleep(3 * tc.spacing)
					synctest.Wait()
				}
				mu.Lock()
				got := append([]time.Time(nil), starts...)
				mu.Unlock()
				if len(got) != 3 {
					t.Fatalf("transport calls while all responses blocked=%d want3", len(got))
				}
				for i := 1; i < len(got); i++ {
					if got[i].Sub(got[i-1]) < tc.spacing {
						t.Errorf("transport gap=%v want >=%v", got[i].Sub(got[i-1]), tc.spacing)
					}
				}
				close(release)
				wg.Wait()
			})
		})
	}
}

func TestProviderSharedExtractScorePacing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p, _ := newHTTPProvider(geminiSpec, "synthetic", "model", "https://synthetic.invalid", 10*time.Second)
		release := make(chan struct{})
		var mu sync.Mutex
		var starts []time.Time
		p.http = &http.Client{Transport: pacingTransportFunc(func(r *http.Request) (*http.Response, error) {
			mu.Lock()
			starts = append(starts, time.Now())
			t.Logf("shared Extract/ScoreDelta transport start=%s", time.Now().Format(time.RFC3339Nano))
			mu.Unlock()
			<-release
			envelope, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `{"items":[],"newcomer":true,"min_career":0,"max_career":0,"education":"none","career_evidence":"신입 개발자 채용","education_evidence":""}`}}}, "usage": map[string]int{"prompt_tokens": 1, "completion_tokens": 1}})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(envelope))), Header: make(http.Header)}, nil
		})}
		var wg sync.WaitGroup
		wg.Go(func() {
			if _, _, err := p.Extract(context.Background(), "신입 개발자 채용"); err != nil {
				t.Error(err)
			}
		})
		wg.Go(func() {
			_, _, err := p.ScoreDelta(context.Background(), "신입 개발자 채용", "백엔드")
			if err != nil {
				t.Error(err)
			}
		})
		synctest.Wait()
		mu.Lock()
		count := len(starts)
		mu.Unlock()
		if count != 1 {
			t.Fatalf("before interval calls=%d", count)
		}
		time.Sleep(10 * time.Second)
		synctest.Wait()
		mu.Lock()
		got := append([]time.Time(nil), starts...)
		mu.Unlock()
		if len(got) != 2 || got[1].Sub(got[0]) < 10*time.Second {
			t.Fatalf("shared starts=%v", got)
		}
		close(release)
		wg.Wait()
	})
}

func TestProviderCanceledStartsNeverReachTransport(t *testing.T) {
	for _, spacing := range []time.Duration{0, 10 * time.Second} {
		t.Run(spacing.String(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				p, _ := newHTTPProvider(geminiSpec, "synthetic", "model", "https://synthetic.invalid", spacing)
				calls := 0
				p.http = &http.Client{Transport: pacingTransportFunc(func(r *http.Request) (*http.Response, error) {
					calls++
					return &http.Response{StatusCode: 429, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}, nil
				})}
				canceled, cancel := context.WithCancel(context.Background())
				cancel()
				_, _, err := p.complete(canceled, "s", "u")
				if !errors.Is(err, context.Canceled) || calls != 0 {
					t.Fatalf("canceled first calls=%d err=%v", calls, err)
				}
				_, _, _ = p.complete(context.Background(), "s", "u")
				if spacing > 0 {
					ctx, cancel := context.WithTimeout(context.Background(), spacing)
					defer cancel()
					var wg sync.WaitGroup
					wg.Go(func() { _, _, err = p.complete(ctx, "s", "u") })
					time.Sleep(spacing)
					synctest.Wait()
					wg.Wait()
					if !errors.Is(err, context.DeadlineExceeded) || calls != 1 {
						t.Fatalf("boundary calls=%d err=%v", calls, err)
					}
				}
			})
		})
	}
}
