package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ohchanwu/job-scraper/internal/ai"
	"github.com/ohchanwu/job-scraper/internal/profile"
	"github.com/ohchanwu/job-scraper/internal/scraper"
)

// aiKeysTempPath points a server's key file at a temp dir so a test never
// touches the user's real ai_keys.json.
func aiKeysTempPath(t *testing.T, srv *Server) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "ai_keys.json")
	srv.SetAIKeysPath(p)
	return p
}

func saveProfileJSON(t *testing.T, srv *Server, p profile.Profile) {
	t.Helper()
	j, err := profile.Marshal(p)
	if err != nil {
		t.Fatalf("marshal profile: %v", err)
	}
	if _, _, err := srv.store.SaveProfile(context.Background(), j); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
}

func TestReconfigureAI(t *testing.T) {
	srv, _ := newTestServer(t, &fakeScraper{})
	keysPath := aiKeysTempPath(t, srv)
	ctx := context.Background()

	t.Run("provider chosen but no key → AI stays off", func(t *testing.T) {
		saveProfileJSON(t, srv, profile.Profile{AIProvider: "anthropic"})
		if err := srv.ReconfigureAI(ctx); err != nil {
			t.Fatalf("ReconfigureAI: %v", err)
		}
		if srv.ai != nil {
			t.Fatal("a provider with no saved key must leave AI off (regex fallback)")
		}
	})

	t.Run("provider + key → AI on with the right version", func(t *testing.T) {
		if err := ai.SaveKeys(keysPath, map[string]string{"anthropic": "sk-ant-test"}); err != nil {
			t.Fatalf("SaveKeys: %v", err)
		}
		saveProfileJSON(t, srv, profile.Profile{AIProvider: "anthropic", AIModel: "claude-x"})
		if err := srv.ReconfigureAI(ctx); err != nil {
			t.Fatalf("ReconfigureAI: %v", err)
		}
		if srv.ai == nil {
			t.Fatal("a provider WITH a key must turn AI on")
		}
		if srv.ai.Name() != "anthropic" || srv.aiModel != "claude-x" {
			t.Fatalf("provider=%q model=%q, want anthropic/claude-x", srv.ai.Name(), srv.aiModel)
		}
		if srv.aiVersion != ai.AIVersion("anthropic", "claude-x") {
			t.Fatal("aiVersion not derived from the configured provider+model")
		}
	})

	t.Run("blank model falls back to the provider default", func(t *testing.T) {
		saveProfileJSON(t, srv, profile.Profile{AIProvider: "anthropic"}) // no model
		if err := srv.ReconfigureAI(ctx); err != nil {
			t.Fatalf("ReconfigureAI: %v", err)
		}
		if srv.aiModel != ai.DefaultModel("anthropic") {
			t.Fatalf("model = %q, want the default %q", srv.aiModel, ai.DefaultModel("anthropic"))
		}
	})

	t.Run("selecting 없음 turns AI back off", func(t *testing.T) {
		saveProfileJSON(t, srv, profile.Profile{AIProvider: ""})
		if err := srv.ReconfigureAI(ctx); err != nil {
			t.Fatalf("ReconfigureAI: %v", err)
		}
		if srv.ai != nil {
			t.Fatal("an empty provider must turn AI off")
		}
	})

	t.Run("daily cap from the profile is applied", func(t *testing.T) {
		saveProfileJSON(t, srv, profile.Profile{AIProvider: "", AIDailyTokenCap: 12345})
		if err := srv.ReconfigureAI(ctx); err != nil {
			t.Fatalf("ReconfigureAI: %v", err)
		}
		if srv.aiDailyTokenCap != 12345 {
			t.Fatalf("daily cap = %d, want 12345 from the profile", srv.aiDailyTokenCap)
		}
	})

	t.Run("estimated USD caps are mapped into runtime token ceilings", func(t *testing.T) {
		saveProfileJSON(t, srv, profile.Profile{
			AIProvider:           "",
			AIMonthlyUSDCapCents: 2,
			AIDailyUSDCapCents:   3,
			AIRunUSDCapCents:     4,
		})
		if err := srv.ReconfigureAI(ctx); err != nil {
			t.Fatalf("ReconfigureAI: %v", err)
		}
		if srv.aiMonthlyTokenCap != aiMonthlyTokenCapForUSDCents(2) {
			t.Fatalf("monthly token cap = %d, want mapped cap", srv.aiMonthlyTokenCap)
		}
		if srv.aiDailyTokenCap != aiDailyTokenCapForUSDCents(3) {
			t.Fatalf("daily token cap = %d, want mapped cap", srv.aiDailyTokenCap)
		}
		if srv.aiRunTokenCap != aiRunTokenCapForUSDCents(4) {
			t.Fatalf("run token cap = %d, want mapped cap", srv.aiRunTokenCap)
		}
	})
}

// TestProfileSaveWritesKeyAt0600AndEnablesAI covers the verify bar: the settings
// UI saves a key at 0600 and AI then renders.
func TestProfileSaveWritesKeyAt0600AndEnablesAI(t *testing.T) {
	srv, _ := newTestServer(t, &fakeScraper{})
	keysPath := aiKeysTempPath(t, srv)

	form := url.Values{}
	form.Set("ai_provider", "anthropic")
	form.Set("ai_model", "claude-x")
	form.Set("ai_key", "sk-ant-secret")
	form.Set("min_score", "40")
	req := httptest.NewRequest(http.MethodPost, "/profile", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303 (redirect to /)", rec.Code)
	}
	// The key file landed at 0600.
	info, err := os.Stat(keysPath)
	if err != nil {
		t.Fatalf("stat key file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("key file mode = %o, want 600", perm)
	}
	keys, err := ai.LoadKeys(keysPath)
	if err != nil || keys["anthropic"] != "sk-ant-secret" {
		t.Fatalf("saved keys = %v err=%v, want anthropic→sk-ant-secret", keys, err)
	}
	// AI is now live on the running server.
	if srv.ai == nil {
		t.Fatal("AI must be enabled after saving a provider + key")
	}

	t.Run("re-saving with a blank key keeps the existing key", func(t *testing.T) {
		form := url.Values{}
		form.Set("ai_provider", "anthropic")
		form.Set("ai_key", "") // blank — the user didn't retype the secret
		form.Set("min_score", "40")
		req := httptest.NewRequest(http.MethodPost, "/profile", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		srv.Handler().ServeHTTP(httptest.NewRecorder(), req)

		keys, _ := ai.LoadKeys(keysPath)
		if keys["anthropic"] != "sk-ant-secret" {
			t.Fatalf("blank key must not wipe the saved key; got %q", keys["anthropic"])
		}
		if srv.ai == nil {
			t.Fatal("AI must stay enabled when the key is preserved")
		}
	})

	t.Run("the form shows the key as saved, never the secret", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/profile", nil))
		body := rec.Body.String()
		if strings.Contains(body, "sk-ant-secret") {
			t.Fatal("the profile form must NEVER re-render the secret key")
		}
		if !strings.Contains(body, "저장됨") {
			t.Fatal("the form should show the key as saved (•••• 저장됨)")
		}
	})
}

// TestDailyTokenCapHaltsAIRegexContinues covers: the daily cap halts AI while
// regex scoring still runs.
func TestDailyTokenCapHaltsAIRegexContinues(t *testing.T) {
	f := &fakeScraper{
		listing: []scraper.Posting{listingPosting("1", "백엔드 신입")},
		details: map[string]scraper.Posting{"1": listingPosting("1", "백엔드 신입")},
	}
	srv, st := newTestServer(t, f)
	stub := newcomerStub()
	srv.SetAIProvider(stub, "test-model")
	srv.aiDailyTokenCap = 50 // tiny daily cap
	saveSinipProfile(t, srv)
	ctx := context.Background()

	// Pre-spend today's budget so the run starts already over the daily cap.
	day := time.Now().UTC().Format("2006-01-02")
	if err := st.AddAIUsage(ctx, day, 100, 0); err != nil {
		t.Fatalf("seed usage: %v", err)
	}

	res, err := srv.runScrape(ctx, noopEmit)
	if err != nil {
		t.Fatalf("runScrape: %v", err)
	}
	if stub.Calls != 0 {
		t.Errorf("Extract calls = %d, want 0 (daily cap already exhausted)", stub.Calls)
	}
	if n := aiExtractionCount(t, srv); n != 0 {
		t.Errorf("ai_extractions rows = %d, want 0 (AI halted)", n)
	}
	// Regex scoring still ran: the posting is present and scored.
	if res.New != 1 || res.Scored != 1 {
		t.Errorf("ScrapeResult = %+v, want the posting inserted + scored by regex", res)
	}
}

func TestDailyUSDCapHaltsManualRerate(t *testing.T) {
	srv, _ := seedRerate(t)
	ctx := context.Background()
	saveProfileJSON(t, srv, profile.Profile{
		CareerYears:        0,
		JobLikes:           "백엔드 서버 개발",
		AIDailyUSDCapCents: 1,
	})
	if err := srv.ReconfigureAI(ctx); err != nil {
		t.Fatalf("ReconfigureAI: %v", err)
	}
	stub := rerateStub()
	srv.SetAIProvider(stub, "test-model")
	day := time.Now().UTC().Format("2006-01-02")
	if err := srv.store.AddAIUsage(ctx, day, aiDailyTokenCapForUSDCents(1), 0); err != nil {
		t.Fatalf("seed daily usage: %v", err)
	}

	if _, _, err := srv.runRerate(ctx, "today", noopEmit); err != nil {
		t.Fatalf("runRerate: %v", err)
	}
	if stub.ScoreDeltaCalls != 0 {
		t.Fatalf("ScoreDelta calls = %d, want 0 when daily USD cap is exhausted", stub.ScoreDeltaCalls)
	}
}

func TestMonthlyUSDCapHaltsManualRerate(t *testing.T) {
	srv, _ := seedRerate(t)
	ctx := context.Background()
	saveProfileJSON(t, srv, profile.Profile{
		CareerYears:          0,
		JobLikes:             "백엔드 서버 개발",
		AIMonthlyUSDCapCents: 1,
	})
	if err := srv.ReconfigureAI(ctx); err != nil {
		t.Fatalf("ReconfigureAI: %v", err)
	}
	stub := rerateStub()
	srv.SetAIProvider(stub, "test-model")
	now := time.Now().UTC()
	firstOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	if err := srv.store.AddAIUsage(ctx, firstOfMonth, aiMonthlyTokenCapForUSDCents(1), 0); err != nil {
		t.Fatalf("seed monthly usage: %v", err)
	}

	if _, _, err := srv.runRerate(ctx, "today", noopEmit); err != nil {
		t.Fatalf("runRerate: %v", err)
	}
	if stub.ScoreDeltaCalls != 0 {
		t.Fatalf("ScoreDelta calls = %d, want 0 when monthly USD cap is exhausted", stub.ScoreDeltaCalls)
	}
}

// TestBudgetLedgerPersistsAcrossRestart covers: the ledger accumulates across a
// simulated restart (a fresh Server on the same store sees the prior spend).
func TestBudgetLedgerPersistsAcrossRestart(t *testing.T) {
	f := &fakeScraper{
		listing: []scraper.Posting{listingPosting("1", "백엔드 신입")},
		details: map[string]scraper.Posting{"1": listingPosting("1", "백엔드 신입")},
	}
	srv, st := newTestServer(t, f)
	srv.SetAIProvider(newcomerStub(), "test-model") // each Extract spends 120 tokens
	saveSinipProfile(t, srv)
	ctx := context.Background()

	if _, err := srv.runScrape(ctx, noopEmit); err != nil {
		t.Fatalf("runScrape: %v", err)
	}
	day := time.Now().UTC().Format("2006-01-02")
	in, out, _ := st.AIUsageForDay(ctx, day)
	if in != 100 || out != 20 {
		t.Fatalf("ledger after scrape = (%d,%d), want (100,20) — the debit must persist", in, out)
	}

	// "Restart": a brand-new Server over the same store. Its budget must start
	// from the persisted daily total, not zero.
	srv2 := New(st, f)
	srv2.SetAIProvider(newcomerStub(), "test-model")
	b := srv2.newAIBudget(ctx)
	if b == nil || b.dailyAtStart != 120 {
		t.Fatalf("new run's dailyAtStart = %v, want 120 (read from the persisted ledger)", b)
	}
}
