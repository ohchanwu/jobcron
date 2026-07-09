package server

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/ohchanwu/job-scraper/internal/auth"
	"github.com/ohchanwu/job-scraper/internal/storage"
)

func TestProductionBookmarksUseSessionOwnerState(t *testing.T) {
	srv, st := newPostgresTestServer(t, &fakeScraper{})
	srv.SetProductionMode(true)
	ctx := context.Background()

	userA, cookieA := createSessionUser(t, st, "owner-a@example.com", "session-a")
	userB, cookieB := createSessionUser(t, st, "owner-b@example.com", "session-b")
	postingID := mustUpsert(t, st, listingPosting("shared-bookmark", "공유 북마크 공고"))
	if err := st.AddBookmark(ctx, userB, postingID); err != nil {
		t.Fatalf("seed userB bookmark: %v", err)
	}

	assertPageMissing(t, srv, cookieA, "/bookmarks", "공유 북마크 공고")
	assertPageContains(t, srv, cookieB, "/bookmarks", "공유 북마크 공고")

	putReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/bookmark/%d", postingID), nil)
	putReq.AddCookie(cookieA)
	addCSRFToRequest(putReq, srv, cookieA)
	putRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("bookmark add status = %d, want 200", putRec.Code)
	}
	assertBookmarkedForUser(t, st, userA, postingID, true)
	assertBookmarkedForUser(t, st, userB, postingID, true)
	assertPageContains(t, srv, cookieA, "/bookmarks", "공유 북마크 공고")

	deleteReq := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/bookmark/%d", postingID), nil)
	deleteReq.AddCookie(cookieA)
	addCSRFToRequest(deleteReq, srv, cookieA)
	deleteRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("bookmark delete status = %d, want 200", deleteRec.Code)
	}
	assertBookmarkedForUser(t, st, userA, postingID, false)
	assertBookmarkedForUser(t, st, userB, postingID, true)
	assertPageMissing(t, srv, cookieA, "/bookmarks", "공유 북마크 공고")
	assertPageContains(t, srv, cookieB, "/bookmarks", "공유 북마크 공고")

	userBOnlyPostingID := mustUpsert(t, st, listingPosting("user-b-bookmark", "유저B 전용 북마크"))
	putBReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/bookmark/%d", userBOnlyPostingID), nil)
	putBReq.AddCookie(cookieB)
	addCSRFToRequest(putBReq, srv, cookieB)
	putBRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(putBRec, putBReq)
	if putBRec.Code != http.StatusOK {
		t.Fatalf("userB bookmark add status = %d, want 200", putBRec.Code)
	}
	assertBookmarkedForUser(t, st, userA, userBOnlyPostingID, false)
	assertBookmarkedForUser(t, st, userB, userBOnlyPostingID, true)
	assertPageMissing(t, srv, cookieA, "/bookmarks", "유저B 전용 북마크")
	assertPageContains(t, srv, cookieB, "/bookmarks", "유저B 전용 북마크")
}

func TestProductionHiddenUseSessionOwnerState(t *testing.T) {
	srv, st := newPostgresTestServer(t, &fakeScraper{})
	srv.SetProductionMode(true)
	ctx := context.Background()

	userA, cookieA := createSessionUser(t, st, "owner-a@example.com", "session-a")
	userB, cookieB := createSessionUser(t, st, "owner-b@example.com", "session-b")
	postingID := mustUpsert(t, st, listingPosting("shared-hidden", "공유 숨김 공고"))
	mutedAt := time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)
	if err := st.AddNotInterested(ctx, userB, postingID, mutedAt); err != nil {
		t.Fatalf("seed userB hidden row: %v", err)
	}

	assertPageMissing(t, srv, cookieA, "/hidden", "공유 숨김 공고")
	assertPageContains(t, srv, cookieB, "/hidden", "공유 숨김 공고")

	putReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/not-interested/%d", postingID), nil)
	putReq.AddCookie(cookieA)
	addCSRFToRequest(putReq, srv, cookieA)
	putRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("not-interested add status = %d, want 200", putRec.Code)
	}
	assertHiddenForUser(t, st, userA, postingID, true)
	assertHiddenForUser(t, st, userB, postingID, true)
	assertPageContains(t, srv, cookieA, "/hidden", "공유 숨김 공고")

	deleteReq := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/not-interested/%d", postingID), nil)
	deleteReq.AddCookie(cookieA)
	addCSRFToRequest(deleteReq, srv, cookieA)
	deleteRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("not-interested delete status = %d, want 200", deleteRec.Code)
	}
	assertHiddenForUser(t, st, userA, postingID, false)
	assertHiddenForUser(t, st, userB, postingID, true)
	assertPageMissing(t, srv, cookieA, "/hidden", "공유 숨김 공고")
	assertPageContains(t, srv, cookieB, "/hidden", "공유 숨김 공고")

	userBOnlyPostingID := mustUpsert(t, st, listingPosting("user-b-hidden", "유저B 전용 숨김"))
	putBReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/not-interested/%d", userBOnlyPostingID), nil)
	putBReq.AddCookie(cookieB)
	addCSRFToRequest(putBReq, srv, cookieB)
	putBRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(putBRec, putBReq)
	if putBRec.Code != http.StatusOK {
		t.Fatalf("userB not-interested add status = %d, want 200", putBRec.Code)
	}
	assertHiddenForUser(t, st, userA, userBOnlyPostingID, false)
	assertHiddenForUser(t, st, userB, userBOnlyPostingID, true)
	assertPageMissing(t, srv, cookieA, "/hidden", "유저B 전용 숨김")
	assertPageContains(t, srv, cookieB, "/hidden", "유저B 전용 숨김")
}

func TestProductionProfileUsesSessionOwnerState(t *testing.T) {
	srv, st := newPostgresTestServer(t, &fakeScraper{})
	srv.SetProductionMode(true)
	ctx := context.Background()

	userA, cookieA := createSessionUser(t, st, "owner-a@example.com", "session-a")
	userB, cookieB := createSessionUser(t, st, "owner-b@example.com", "session-b")
	if _, _, err := st.SaveProfileForUser(ctx, userA, `{"career_years":0,"job_likes":"유저A 기존 목표"}`); err != nil {
		t.Fatalf("seed userA profile: %v", err)
	}
	if _, _, err := st.SaveProfileForUser(ctx, userB, `{"career_years":0,"job_likes":"유저B 기존 목표"}`); err != nil {
		t.Fatalf("seed userB profile: %v", err)
	}

	assertPageContains(t, srv, cookieA, "/profile", "유저A 기존 목표")
	assertPageMissing(t, srv, cookieA, "/profile", "유저B 기존 목표")
	assertPageContains(t, srv, cookieB, "/profile", "유저B 기존 목표")
	assertPageMissing(t, srv, cookieB, "/profile", "유저A 기존 목표")

	form := url.Values{}
	form.Set("career_years", "0")
	form.Set("job_likes", "유저A 새 목표")
	req := httptest.NewRequest(http.MethodPost, "/profile", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookieA)
	addCSRFToRequest(req, srv, cookieA)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("profile save status = %d, want 303; body=%q", rec.Code, rec.Body.String())
	}

	gotA, _, ok, err := st.ProfileForUser(ctx, userA)
	if err != nil || !ok {
		t.Fatalf("ProfileForUser userA: ok=%v err=%v", ok, err)
	}
	gotB, _, ok, err := st.ProfileForUser(ctx, userB)
	if err != nil || !ok {
		t.Fatalf("ProfileForUser userB: ok=%v err=%v", ok, err)
	}
	if !strings.Contains(gotA, "유저A 새 목표") {
		t.Fatalf("userA profile = %s, want updated goal", gotA)
	}
	if !strings.Contains(gotB, "유저B 기존 목표") || strings.Contains(gotB, "유저A 새 목표") {
		t.Fatalf("userB profile = %s, want unchanged userB goal only", gotB)
	}

	assertPageContains(t, srv, cookieA, "/profile", "유저A 새 목표")
	assertPageMissing(t, srv, cookieA, "/profile", "유저B 기존 목표")
	assertPageContains(t, srv, cookieB, "/profile", "유저B 기존 목표")
	assertPageMissing(t, srv, cookieB, "/profile", "유저A 새 목표")

	form = url.Values{}
	form.Set("career_years", "0")
	form.Set("job_likes", "유저B 새 목표")
	form.Set("ai_daily_token_cap", "222222")
	req = httptest.NewRequest(http.MethodPost, "/profile", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookieB)
	addCSRFToRequest(req, srv, cookieB)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("userB profile save status = %d, want 303; body=%q", rec.Code, rec.Body.String())
	}

	gotA, _, ok, err = st.ProfileForUser(ctx, userA)
	if err != nil || !ok {
		t.Fatalf("ProfileForUser userA after userB save: ok=%v err=%v", ok, err)
	}
	gotB, _, ok, err = st.ProfileForUser(ctx, userB)
	if err != nil || !ok {
		t.Fatalf("ProfileForUser userB after userB save: ok=%v err=%v", ok, err)
	}
	if !strings.Contains(gotA, "유저A 새 목표") || strings.Contains(gotA, "유저B 새 목표") {
		t.Fatalf("userA profile = %s, want unchanged userA goal only", gotA)
	}
	if !strings.Contains(gotB, "유저B 새 목표") {
		t.Fatalf("userB profile = %s, want updated userB goal", gotB)
	}
	if srv.aiDailyTokenCap != 222222 {
		t.Fatalf("aiDailyTokenCap = %d, want userB cap 222222", srv.aiDailyTokenCap)
	}

	assertPageContains(t, srv, cookieA, "/profile", "유저A 새 목표")
	assertPageMissing(t, srv, cookieA, "/profile", "유저B 새 목표")
	assertPageContains(t, srv, cookieB, "/profile", "유저B 새 목표")
	assertPageMissing(t, srv, cookieB, "/profile", "유저A 새 목표")
}

func newPostgresTestServer(t *testing.T, f *fakeScraper) (*Server, *storage.Store) {
	t.Helper()
	databaseURL := os.Getenv("JOBSCRAPER_TEST_POSTGRES_URL")
	if databaseURL == "" {
		t.Skip("JOBSCRAPER_TEST_POSTGRES_URL not set")
	}
	schema := postgresTestSchemaName(t)
	admin, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatalf("open postgres admin: %v", err)
	}
	if _, err := admin.Exec(`CREATE SCHEMA ` + schema); err != nil {
		admin.Close()
		t.Fatalf("create postgres schema %s: %v", schema, err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`)
		_ = admin.Close()
	})

	st, err := storage.OpenPostgres(databaseURLWithSearchPath(databaseURL, schema))
	if err != nil {
		t.Fatalf("OpenPostgres: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return New(st, f), st
}

func createSessionUser(t *testing.T, st *storage.Store, email, rawToken string) (int64, *http.Cookie) {
	t.Helper()
	ctx := context.Background()
	passwordHash, err := auth.HashPassword("correct-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	var userID int64
	if err := st.SQLDB().QueryRowContext(ctx, `
INSERT INTO users (email, password_hash, created_at, updated_at)
VALUES ($1, $2, now(), now())
RETURNING id`, email, passwordHash).Scan(&userID); err != nil {
		t.Fatalf("insert user %s: %v", email, err)
	}
	if err := st.CreateSession(ctx, userID, auth.HashSessionToken(rawToken), time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("CreateSession %s: %v", email, err)
	}
	return userID, &http.Cookie{Name: sessionCookieName, Value: rawToken}
}

func addCSRFToRequest(req *http.Request, srv *Server, sessionCookie *http.Cookie) {
	const csrfCookieValue = "test-csrf-cookie"
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrfCookieValue})
	req.Header.Set(csrfHeaderName, srv.csrfToken(csrfCookieValue, sessionCookie.Value))
}

func assertBookmarkedForUser(t *testing.T, st *storage.Store, userID, postingID int64, want bool) {
	t.Helper()
	got, err := st.IsBookmarkedForUser(context.Background(), userID, postingID)
	if err != nil {
		t.Fatalf("IsBookmarkedForUser(%d,%d): %v", userID, postingID, err)
	}
	if got != want {
		t.Fatalf("IsBookmarkedForUser(%d,%d) = %v, want %v", userID, postingID, got, want)
	}
}

func assertHiddenForUser(t *testing.T, st *storage.Store, userID, postingID int64, want bool) {
	t.Helper()
	got, err := st.IsNotInterestedForUser(context.Background(), userID, postingID)
	if err != nil {
		t.Fatalf("IsNotInterestedForUser(%d,%d): %v", userID, postingID, err)
	}
	if got != want {
		t.Fatalf("IsNotInterestedForUser(%d,%d) = %v, want %v", userID, postingID, got, want)
	}
}

func assertPageContains(t *testing.T, srv *Server, cookie *http.Cookie, path, want string) {
	t.Helper()
	body := authedPageBody(t, srv, cookie, path)
	if !strings.Contains(body, want) {
		t.Fatalf("%s body missing %q\n%s", path, want, body)
	}
}

func assertPageMissing(t *testing.T, srv *Server, cookie *http.Cookie, path, unwanted string) {
	t.Helper()
	body := authedPageBody(t, srv, cookie, path)
	if strings.Contains(body, unwanted) {
		t.Fatalf("%s body unexpectedly contains %q\n%s", path, unwanted, body)
	}
}

func authedPageBody(t *testing.T, srv *Server, cookie *http.Cookie, path string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s status = %d, want 200; body=%q", path, rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

var nonSchemaChars = regexp.MustCompile(`[^a-z0-9_]`)

func postgresTestSchemaName(t *testing.T) string {
	t.Helper()
	name := strings.ToLower(t.Name())
	name = strings.ReplaceAll(name, "/", "_")
	name = nonSchemaChars.ReplaceAllString(name, "_")
	return fmt.Sprintf("test_server_%s_%d_%d", name, time.Now().UnixNano(), rand.Uint64())
}

func databaseURLWithSearchPath(databaseURL, schema string) string {
	separator := "?"
	if strings.Contains(databaseURL, "?") {
		separator = "&"
	}
	return databaseURL + separator + "search_path=" + schema
}
