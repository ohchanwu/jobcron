package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ohchanwu/jobcron/internal/ai"
	"github.com/ohchanwu/jobcron/internal/profile"
)

func TestDemoModeRejectsWriteRoutes(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	srv.SetDemoMode(true)
	id := mustUpsert(t, st, listingPosting("1", "데모 공고"))

	routes := []struct {
		method string
		path   string
		body   url.Values
	}{
		{http.MethodPost, "/profile", url.Values{"career_years": {"0"}}},
		{http.MethodPut, "/api/bookmark/" + strconv.FormatInt(id, 10), nil},
		{http.MethodDelete, "/api/bookmark/" + strconv.FormatInt(id, 10), nil},
		{http.MethodPut, "/api/not-interested/" + strconv.FormatInt(id, 10), nil},
		{http.MethodDelete, "/api/not-interested/" + strconv.FormatInt(id, 10), nil},
	}

	for _, tc := range routes {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			var body *strings.Reader
			if tc.body != nil {
				body = strings.NewReader(tc.body.Encode())
			} else {
				body = strings.NewReader("")
			}
			req := httptest.NewRequest(tc.method, tc.path, body)
			if tc.body != nil {
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			}
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403", rec.Code)
			}
			if !strings.Contains(rec.Body.String(), "데모 모드에서는 변경할 수 없어요") {
				t.Fatalf("body = %q, want calm demo refusal", rec.Body.String())
			}
		})
	}
}

func TestDemoModeScrapeRequiresAdminToken(t *testing.T) {
	srv, _ := newTestServer(t, &fakeScraper{})
	srv.SetDemoMode(true)
	srv.SetAdminToken("secret-token")

	for _, tc := range []struct {
		name string
		path string
		want int
	}{
		{"missing token", "/api/scrape", http.StatusForbidden},
		{"wrong token", "/api/scrape?token=wrong", http.StatusForbidden},
		{"correct token reaches handler", "/api/scrape?token=secret-token", http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d; body=%q", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestDemoModeRerateAlwaysRefused(t *testing.T) {
	srv, _ := seedRerate(t)
	srv.SetDemoMode(true)
	srv.SetAdminToken("secret-token")

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/rerate?surface=today&token=secret-token", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "데모 모드에서는 AI 재평가를 실행할 수 없어요") {
		t.Fatalf("body = %q, want demo rerate refusal", rec.Body.String())
	}
}

func TestDemoPagesRenderDemoState(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	srv.SetDemoMode(true)
	ctx := context.Background()
	profJSON, _ := profile.Marshal(profile.Profile{CareerYears: 0})
	if _, _, err := st.SaveProfile(ctx, profJSON); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	p := listingPosting("today", "데모 렌더 공고")
	p.FirstSeenAt = time.Now().UTC()
	p.LastSeenAt = p.FirstSeenAt
	id := mustUpsert(t, st, p)
	scoreEach(t, st, map[int64]int{id: 50})

	for _, path := range []string{"/", "/archive", "/bookmarks", "/hidden", "/profile"} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			body := rec.Body.String()
			if !strings.Contains(body, `data-demo="true"`) {
				t.Fatalf("%s missing demo data flag", path)
			}
			if path == "/" && strings.Contains(body, `id="scrape"`) {
				t.Fatal("dashboard still renders scrape button")
			}
			if path == "/profile" && !strings.Contains(body, "데모 모드에서는 설정을 볼 수만 있어요") {
				t.Fatal("profile missing demo disabled banner")
			}
		})
	}
}

func TestDemoBookmarksAndHiddenRenderAllPostingsForLocalStorageFiltering(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	srv.SetDemoMode(true)
	mustUpsert(t, st, listingPosting("a", "방문자 저장 후보"))
	mustUpsert(t, st, listingPosting("b", "방문자 숨김 후보"))

	for _, tc := range []struct {
		path string
		want string
	}{
		{"/bookmarks", "방문자 저장 후보"},
		{"/hidden", "방문자 숨김 후보"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			if !strings.Contains(rec.Body.String(), tc.want) {
				t.Fatalf("%s should render all postings in demo mode for client filtering", tc.path)
			}
		})
	}
}

func TestDemoRescoreUsesCachedAIScoreWithoutProviderAsCurrent(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	srv.SetDemoMode(true)
	ctx := context.Background()

	profJSON, _ := profile.Marshal(profile.Profile{CareerYears: 0, JobLikes: "백엔드"})
	if _, _, err := st.SaveProfile(ctx, profJSON); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	id := mustUpsert(t, st, listingPosting("ai", "AI 캐시 공고"))
	d := ai.Delta{
		Items: []ai.DeltaItem{{
			Signal:   "백엔드 업무",
			Kind:     ai.KindPresence,
			Delta:    7,
			Evidence: "백엔드 업무를 맡아요",
		}},
		NetDelta: 7,
	}
	if err := st.UpsertAIScore(ctx, id, profile.AIInputHash(profile.Profile{CareerYears: 0, JobLikes: "백엔드"}), "anthropic-old", d, time.Now()); err != nil {
		t.Fatalf("UpsertAIScore: %v", err)
	}

	if _, err := srv.RescoreAll(ctx); err != nil {
		t.Fatalf("RescoreAll: %v", err)
	}
	scores, err := st.ScoresByPostingID(ctx)
	if err != nil {
		t.Fatalf("ScoresByPostingID: %v", err)
	}
	got := scores[id].BreakdownJSON
	if !strings.Contains(got, `"Label":"AI 분석"`) {
		t.Fatalf("cached AI delta missing from demo rescore: %s", got)
	}
	if strings.Contains(got, `"stale":true`) {
		t.Fatalf("demo cached AI delta should render current, not stale: %s", got)
	}
}
