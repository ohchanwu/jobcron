package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ohchanwu/jobcron/internal/ai"
)

// Opt-in COPY-only exports: real templates and real stub-run events, never a
// live provider or a browser-accessible mutating server. Broad tests unset this.
func TestGeminiCopyBrowserFixture(t *testing.T) {
	out := os.Getenv("JOBCRON_RERATE_PROBE_OUT")
	if os.Getenv("JOBCRON_RERATE_BROWSER_PROBE") != "1" || out == "" {
		t.Skip("owned copy browser export not requested")
	}
	info, err := os.Lstat(out)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		t.Fatal("export needs an existing non-symlink owner-only directory")
	}
	srv, cookie, stub := newProductionRerateFixture(t, 2, 0)
	for _, tc := range []struct{ route, file string }{{"/briefing", "briefing.html"}, {"/", "all.html"}, {"/bookmarks", "bookmarks.html"}} {
		req := httptest.NewRequest(http.MethodGet, tc.route, nil)
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("page=%s status=%d", tc.route, rec.Code)
		}
		if err := os.WriteFile(filepath.Join(out, tc.file), rec.Body.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}

	// Production fixture user identity is cookie scoped, not the local-user fallback.
	user, ok, err := srv.store.UserBySessionToken(context.Background(), cookie.Value)
	if err != nil || !ok {
		t.Fatal("synthetic fixture session missing")
	}
	runtime, err := srv.aiRuntimeForUser(context.Background(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	events := &captureEmit{}
	if _, err := srv.runRerate(context.Background(), "today", events.emit, user.ID, runtime); err != nil {
		t.Fatal(err)
	}
	if stub.ScoreDeltaCalls != 2 {
		t.Fatalf("stub calls=%d", stub.ScoreDeltaCalls)
	}
	payload := struct {
		Events   [][2]string `json:"events"`
		Failures []string    `json:"failures"`
	}{Events: events.snapshot()}
	for _, body := range []string{
		`{"error":{"details":[{"@type":"type.googleapis.com/google.rpc.QuotaFailure","violations":[{"quotaId":"GenerateRequestsPerMinutePerProjectPerModel-FreeTier"}]}]}}`,
		`{"error":{"details":[{"@type":"type.googleapis.com/google.rpc.QuotaFailure","violations":[{"quotaId":"GenerateRequestsPerDayPerProjectPerModel-FreeTier"}]}]}}`,
		`{"error":{}}`,
	} {
		payload.Failures = append(payload.Failures, providerFailureMessage(&ai.APIError{Provider: "gemini", Status: 429, Body: body}))
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "events.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Log("exported three native synthetic pages and actual stub-run events; no browser server or provider access")
}
