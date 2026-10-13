//go:build browserfixture

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ohchanwu/jobcron/internal/ai"
	"github.com/ohchanwu/jobcron/internal/credential"
	"github.com/ohchanwu/jobcron/internal/profile"
	"github.com/ohchanwu/jobcron/internal/scraper"
	"github.com/ohchanwu/jobcron/internal/storage"
)

type liveFixtureCipher struct {
	credential.Cipher
	opens atomic.Int64
}

func (c *liveFixtureCipher) Open(uid int64, p string, b, n []byte, v int16) (string, error) {
	c.opens.Add(1)
	return c.Cipher.Open(uid, p, b, n, v)
}

// This opt-in fixture serves the actual authenticated app, never a copied page.
// Only synthetic task-owned SQL and stub providers are reachable. Control routes
// exist solely in this tagged test binary. Normal and broad tests unset its opt-in.
func TestLivePostingBrowserFixture(t *testing.T) {
	if os.Getenv("JOBCRON_LIVE_BROWSER_FIXTURE") != "1" {
		t.Skip("owned browser opt-in not set")
	}
	srv, st := newPostgresTestServer(t, &fakeScraper{})
	srv.SetProductionMode(true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	uid, cookie := createSessionUser(t, st, "live-a@example.invalid", "live-a-synthetic-session")
	other, otherCookie := createSessionUser(t, st, "live-b@example.invalid", "live-b-synthetic-session")
	threshold := 40
	prof := profile.Profile{Stacks: []profile.StackPref{{Name: "Go", Weight: 25}}, CareerYears: 0, MinScore: &threshold, AIProvider: "anthropic", AIModel: ai.DefaultModel("anthropic"), JobLikes: "백엔드 서버 개발"}
	saveAIRuntimeProfile(t, st, uid, prof)
	saveAIRuntimeProfile(t, st, other, profile.Profile{CareerYears: 0, MinScore: &threshold})
	cipher := &liveFixtureCipher{Cipher: newAIRuntimeTestCipher(t, 0x71)}
	srv.SetCredentialCipher(cipher)
	saveAIRuntimeCredential(t, st, cipher, uid, "anthropic", "live-fixture-synthetic-key")
	rows := 8
	if value := os.Getenv("JOBCRON_LIVE_FIXTURE_ROWS"); value != "" {
		n, err := strconv.Atoi(value)
		if err != nil || n < 8 || n > 2000 {
			t.Fatal("invalid synthetic row count")
		}
		rows = n
	}
	if rows > 8 {
		prof.Dealbreakers = []string{"야근"}
		saveAIRuntimeProfile(t, st, uid, prof)
		srv.sources = append(srv.sources, &fakeScraper{source: "rallit"})
	}
	now := time.Now().UTC()
	postings := make([]scraper.Posting, 0, rows)
	for i := 0; i < rows; i++ {
		p := listingPosting(fmt.Sprintf("live-%04d", i), fmt.Sprintf("신입 Go 서버 개발자 LIVE-%04d", i))
		p.StackTags = []string{"Go"}
		p.Description = "서버 개발자를 찾습니다. Go API 업무를 맡아요."
		p.Newcomer = true
		p.FirstSeenAt = now.Add(-time.Duration(i/50)*24*time.Hour - time.Duration(i%50)*time.Minute)
		p.LastSeenAt = now
		if rows > 8 {
			if i%10 == 0 {
				p.StackTags = nil
			}
			if i%17 == 0 {
				p.Title += " 야근"
			}
			if i%37 == 0 {
				deadline := now.Add(-time.Hour)
				p.ClosedAt = &deadline
			}
			if i%31 == 30 {
				p.Source = "rallit"
			}
		}
		p.ID = mustUpsert(t, st, p)
		postings = append(postings, p)
		if i != 0 && (rows == 8 || i%4 != 0) {
			if err := st.AddBookmark(ctx, uid, p.ID); err != nil {
				t.Fatal(err)
			}
		}
		_, hash, _ := ai.ModelInput(p)
		if err := st.UpsertAIExtraction(ctx, p.ID, hash, ai.ExtractionContractVersion(), ai.Extraction{Newcomer: true}, now); err != nil {
			t.Fatal(err)
		}
		if rows > 8 {
			if i%31 == 30 {
				if err := st.MarkDuplicate(ctx, p.ID, postings[i-1].ID); err != nil {
					t.Fatal(err)
				}
			}
			goal, version := profile.AIInputHash(prof), ai.ScoreVersion(prof.AIProvider, prof.AIModel)
			var err error
			switch i % 6 {
			case 0:
				delta := 20
				if i%12 == 0 {
					delta = -30
				}
				d := ai.GateDelta([]ai.RawDeltaItem{{Signal: "백엔드", Kind: ai.KindPresence, Delta: delta, Quote: "서버 개발자를 찾습니다", MatchedGoal: "좋아하는 업무"}}, p.Description, p.Description)
				err = st.UpsertAIResult(ctx, uid, p.ID, goal, version, d, storage.AIScoreOutcome{State: storage.AIScoreRated, Proposed: 1, Accepted: 1, ComputedAt: now})
			case 1:
				err = st.UpsertAIResult(ctx, uid, p.ID, goal, version, ai.Delta{}, storage.AIScoreOutcome{State: storage.AIScoreNoSignal, ComputedAt: now})
			case 2:
				err = st.UpsertAIScoreFailure(ctx, uid, p.ID, goal, version, storage.AIScoreOutcome{State: storage.AIScoreRejected, Proposed: 1, ComputedAt: now})
			case 3:
				err = st.UpsertAIScoreFailure(ctx, uid, p.ID, goal, version, storage.AIScoreOutcome{State: storage.AIScoreFailed, ComputedAt: now})
			case 4:
				err = st.UpsertAIScore(ctx, uid, p.ID, goal, version, ai.Delta{}, now)
			}
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	var calls, factories atomic.Int64
	releases := make([]chan struct{}, 8)
	for i := range releases {
		releases[i] = make(chan struct{})
	}
	var releaseMu sync.Mutex
	released := make([]bool, 8)
	stub := &ai.StubProvider{NameVal: "anthropic", ScoreDeltaFn: func(ctx context.Context, text, goal string) ([]ai.RawDeltaItem, ai.Usage, error) {
		calls.Add(1)
		i := -1
		for j := 0; j < 8; j++ {
			if strings.Contains(text, fmt.Sprintf("LIVE-%04d", j)) {
				i = j
				break
			}
		}
		if i < 0 {
			return nil, ai.Usage{}, fmt.Errorf("unadmitted synthetic row")
		}
		select {
		case <-ctx.Done():
			return nil, ai.Usage{}, ctx.Err()
		case <-releases[i]:
		}
		switch i {
		case 2:
			return nil, ai.Usage{}, nil
		case 3:
			return []ai.RawDeltaItem{{Signal: "unsupported", Kind: ai.KindPresence, Delta: 20, Quote: "fabricated absent quote", MatchedGoal: "좋아하는 업무"}}, ai.Usage{}, nil
		case 4:
			return nil, ai.Usage{}, fmt.Errorf("synthetic failure")
		}
		delta := 20
		if i == 0 {
			delta = -30
		}
		return []ai.RawDeltaItem{{Signal: "백엔드", Kind: ai.KindPresence, Delta: delta, Quote: "서버 개발자를 찾습니다", MatchedGoal: "좋아하는 업무"}}, ai.Usage{}, nil
	}}
	srv.newAIProvider = func(_, _, _ string, _ time.Duration) (ai.Provider, error) { factories.Add(1); return stub, nil }
	var serving atomic.Pointer[Server]
	serving.Store(srv)
	var measurementMu sync.Mutex
	measurements := []map[string]any{}
	var sequence atomic.Int64
	var holdMu sync.Mutex
	var heldMutation chan struct{}
	ln, err := net.Listen("tcp", "127.0.0.1:17777")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	stop := make(chan struct{})
	var once sync.Once
	var runMu sync.Mutex
	running := false
	var runDone atomic.Bool
	var workers sync.WaitGroup
	mux.HandleFunc("/__live/login", func(w http.ResponseWriter, r *http.Request) {
		c := cookie
		if r.URL.Query().Get("user") == "b" {
			c = otherCookie
		}
		http.SetCookie(w, &http.Cookie{Name: c.Name, Value: c.Value, Path: "/"})
		http.Redirect(w, r, "/", 303)
	})
	mux.HandleFunc("POST /__live/start", func(w http.ResponseWriter, r *http.Request) {
		mode := r.URL.Query().Get("mode")
		if mode != "" && mode != "manual" && mode != "scrape" && mode != "scheduled" {
			w.WriteHeader(400)
			return
		}
		runMu.Lock()
		if running {
			runMu.Unlock()
			w.WriteHeader(409)
			return
		}
		running = true
		runMu.Unlock()
		lease := srv.flight.tryAcquire(scrapeAllKey)
		if lease == nil {
			w.WriteHeader(409)
			return
		}
		workers.Add(1)
		go func() {
			defer workers.Done()
			defer lease.release()
			runtime, err := srv.aiRuntimeForUser(ctx, uid)
			if err != nil {
				return
			}
			currentProfile, _, err := srv.loadProfile(ctx, uid)
			if err != nil {
				return
			}
			if mode == "scheduled" {
				currentProfile.ScheduledAIEnabled = true
				pj, _ := profile.Marshal(currentProfile)
				if _, _, err := srv.saveProfileJSON(ctx, uid, pj); err != nil {
					return
				}
				_, _ = srv.runScheduledScrapeWithHistory(ctx)
				runDone.Store(true)
				return
			}
			run := srv.rerates.start(uid, "today", "")
			emit := func(event, data string) { srv.rerates.record(uid, "today", run.RunID, event, data) }
			switch mode {
			case "manual":
				_, _ = srv.runRerate(ctx, "today", emit, uid, runtime)
			case "scrape":
				_, _ = srv.runScrape(ctx, emit, uid, runtime)
			default:
				srv.rateStage2(ctx, postings[:8], currentProfile, uid, runtime, srv.newAIBudget(ctx, uid, runtime), &callCap{max: 200}, emit, true)
			}
			srv.rerates.complete(uid, "today", run.RunID, rerateOutcomeChanged, "저장된 결과를 반영했어요.")
			runDone.Store(true)
		}()
		w.WriteHeader(202)
	})
	mux.HandleFunc("POST /__live/release", func(w http.ResponseWriter, r *http.Request) {
		i, err := strconv.Atoi(r.URL.Query().Get("row"))
		if err != nil || i < 0 || i >= 8 {
			w.WriteHeader(400)
			return
		}
		releaseMu.Lock()
		if !released[i] {
			close(releases[i])
			released[i] = true
		}
		releaseMu.Unlock()
		w.WriteHeader(204)
	})
	mux.HandleFunc("POST /__live/reset", func(w http.ResponseWriter, r *http.Request) {
		// A distinct synthetic goal creates a new test cohort without deleting
		// caches or changing earlier results. No prior worker may still be active.
		if !runDone.Load() {
			w.WriteHeader(409)
			return
		}
		lease := srv.flight.tryAcquire(scrapeAllKey)
		if lease == nil {
			w.WriteHeader(409)
			return
		}
		defer lease.release()
		currentProfile, _, err := srv.loadProfile(ctx, uid)
		currentProfile.ShortTermGoals = fmt.Sprintf("synthetic cohort %d", sequence.Add(1))
		pj, marshalErr := profile.Marshal(currentProfile)
		if err == nil {
			err = marshalErr
		}
		if err == nil {
			_, _, err = srv.saveProfileJSON(ctx, uid, pj)
		}
		if err != nil {
			w.WriteHeader(500)
			return
		}
		releaseMu.Lock()
		for i := range releases {
			releases[i] = make(chan struct{})
			released[i] = false
		}
		releaseMu.Unlock()
		runMu.Lock()
		running = false
		runMu.Unlock()
		runDone.Store(false)
		w.WriteHeader(204)
	})
	mux.HandleFunc("POST /__live/insert", func(w http.ResponseWriter, r *http.Request) {
		p := listingPosting(fmt.Sprintf("live-insert-%d", time.Now().UnixNano()), "신입 Go 새 공고")
		p.StackTags = []string{"Go"}
		p.FirstSeenAt = time.Now().UTC()
		p.LastSeenAt = p.FirstSeenAt
		err := serving.Load().writePostingState(0, true, func() error { _, _, err := st.UpsertPosting(ctx, p); return err })
		if err != nil {
			w.WriteHeader(500)
			return
		}
		w.WriteHeader(204)
	})
	// Intentional synthetic writers are separate from read-only measurement
	// windows. They exercise the native persistence/barrier, never a provider.
	mux.HandleFunc("POST /__live/change", func(w http.ResponseWriter, r *http.Request) {
		n := sequence.Add(1)
		p := postings[int(n)%len(postings)]
		delta := 20
		if n%2 == 0 {
			delta = -30
		}
		d := ai.GateDelta([]ai.RawDeltaItem{{Signal: "백엔드", Kind: ai.KindPresence, Delta: delta, Quote: "서버 개발자를 찾습니다", MatchedGoal: "좋아하는 업무"}}, p.Description, p.Description)
		current := serving.Load()
		currentProfile, _, err := current.loadProfile(ctx, uid)
		if err == nil {
			err = current.writePostingState(uid, false, func() error {
				return st.UpsertAIResult(ctx, uid, p.ID, profile.AIInputHash(currentProfile), ai.ScoreVersion(currentProfile.AIProvider, currentProfile.AIModel), d, storage.AIScoreOutcome{State: storage.AIScoreRated, Proposed: 1, Accepted: 1, ComputedAt: time.Now()})
			})
		}
		if err != nil {
			w.WriteHeader(500)
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("POST /__live/context", func(w http.ResponseWriter, r *http.Request) {
		current := serving.Load()
		p, _, err := current.loadProfile(ctx, uid)
		p.ShortTermGoals = fmt.Sprintf("synthetic context %d", sequence.Add(1))
		pj, marshalErr := profile.Marshal(p)
		if err == nil {
			err = marshalErr
		}
		if err == nil {
			if r.URL.Query().Get("metadata") == "canceled" {
				commitCtx, commitCancel := context.WithCancel(ctx)
				defer commitCancel()
				err = current.writePostingContext(commitCtx, uid, func() error {
					_, _, writeErr := st.SaveProfileForUser(ctx, uid, pj)
					commitCancel()
					return writeErr
				})
			} else {
				_, _, err = current.saveProfileJSON(ctx, uid, pj)
			}
		}
		if err != nil {
			w.WriteHeader(500)
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("POST /__live/restart", func(w http.ResponseWriter, r *http.Request) {
		// No startup recovery is invoked; a fresh in-memory server reads the same
		// committed store. Close old streams so native recovery observes its epoch.
		old := serving.Load()
		next := New(st, srv.sources...)
		next.SetProductionMode(true)
		next.SetCredentialCipher(cipher)
		next.newAIProvider = srv.newAIProvider
		serving.Store(next)
		old.postingUpdates.revokeUser(uid, "")
		old.postingUpdates.revokeUser(other, "")
		w.WriteHeader(204)
	})
	mux.HandleFunc("POST /__live/revoke", func(w http.ResponseWriter, r *http.Request) {
		if err := st.RevokeSessionToken(ctx, cookie.Value); err != nil {
			w.WriteHeader(500)
			return
		}
		serving.Load().postingUpdates.revokeUser(uid, "")
		w.WriteHeader(204)
	})
	mux.HandleFunc("/__live/metrics", func(w http.ResponseWriter, r *http.Request) {
		measurementMu.Lock()
		defer measurementMu.Unlock()
		json.NewEncoder(w).Encode(measurements)
	})
	mux.HandleFunc("/__live/stats", func(w http.ResponseWriter, r *http.Request) {
		current := serving.Load()
		current.postingUpdates.mu.Lock()
		listeners, reads := current.postingUpdates.total, current.postingUpdates.totalReads
		current.postingUpdates.mu.Unlock()
		json.NewEncoder(w).Encode(map[string]any{"calls": calls.Load(), "factories": factories.Load(), "decrypts": cipher.opens.Load(), "run_done": runDone.Load(), "listeners": listeners, "reads": reads, "rows": rows, "user": uid})
	})
	mux.HandleFunc("POST /__live/stop", func(w http.ResponseWriter, r *http.Request) { once.Do(func() { close(stop) }); w.WriteHeader(204) })
	mux.HandleFunc("POST /__live/hold-mutation", func(w http.ResponseWriter, r *http.Request) {
		holdMu.Lock()
		defer holdMu.Unlock()
		if heldMutation != nil {
			w.WriteHeader(409)
			return
		}
		heldMutation = make(chan struct{})
		w.WriteHeader(204)
	})
	mux.HandleFunc("POST /__live/release-mutation", func(w http.ResponseWriter, r *http.Request) {
		holdMu.Lock()
		defer holdMu.Unlock()
		if heldMutation != nil {
			close(heldMutation)
			heldMutation = nil
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		holdMu.Lock()
		held := heldMutation
		holdMu.Unlock()
		if held != nil && strings.HasPrefix(r.URL.Path, "/api/bookmark/") {
			// Delay only transport of the REAL native mutation response; its
			// commit/notice and concurrent stored-state reads are not held.
			recorded := httptest.NewRecorder()
			serving.Load().Handler().ServeHTTP(recorded, r)
			select {
			case <-held:
			case <-ctx.Done():
				return
			case <-r.Context().Done():
				return
			}
			for key, values := range recorded.Header() {
				w.Header()[key] = values
			}
			w.WriteHeader(recorded.Code)
			_, _ = w.Write(recorded.Body.Bytes())
			return
		}
		serving.Load().Handler().ServeHTTP(w, r)
		if r.URL.Path == "/api/posting-view" {
			measurementMu.Lock()
			measurements = append(measurements, map[string]any{"surface": r.URL.Query().Get("surface"), "sort": r.URL.Query().Get("sort"), "seconds": time.Since(start).Seconds()})
			measurementMu.Unlock()
		}
	})
	// Production mode requires HTTPS Origin. Use native TLS, not an Origin
	// rewrite or an observer-policy bypass; the certificate is fixture-only.
	httpSrv := httptest.NewUnstartedServer(mux)
	httpSrv.Listener = ln
	httpSrv.Config.ReadHeaderTimeout = 5 * time.Second
	httpSrv.StartTLS()
	t.Cleanup(func() { cancel(); httpSrv.CloseClientConnections(); httpSrv.Close(); workers.Wait() })
	t.Log("LIVE_FIXTURE_URL=https://127.0.0.1:17777/__live/login")
	t.Logf("LIVE_FIXTURE synthetic rows=%d; no external scraper/provider transport", rows)
	select {
	case <-stop:
	case <-time.After(20 * time.Minute):
		t.Fatal("owned browser fixture watchdog")
	}
	// Terminate only this fixture's streams/worker and consume their completion.
	cancel()
	httpSrv.CloseClientConnections()
	httpSrv.Close()
	workers.Wait()
}
