package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/ohchanwu/jobcron/internal/ai"
	"github.com/ohchanwu/jobcron/internal/auth"
	"github.com/ohchanwu/jobcron/internal/credential"
	"github.com/ohchanwu/jobcron/internal/profile"
	"github.com/ohchanwu/jobcron/internal/storage"
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

type countingCipher struct {
	inner credential.Cipher
	opens atomic.Int32
}

type failingSealCipher struct{ err error }

func (c failingSealCipher) Seal(int64, string, string) ([]byte, []byte, int16, error) {
	return nil, nil, 0, c.err
}

type sealCountingCipher struct {
	inner credential.Cipher
	seals atomic.Int32
}

func (c *sealCountingCipher) Seal(userID int64, provider, plaintext string) ([]byte, []byte, int16, error) {
	c.seals.Add(1)
	return c.inner.Seal(userID, provider, plaintext)
}
func (c *sealCountingCipher) Open(userID int64, provider string, ciphertext, nonce []byte, version int16) (string, error) {
	return c.inner.Open(userID, provider, ciphertext, nonce, version)
}
func (c failingSealCipher) Open(int64, string, []byte, []byte, int16) (string, error) {
	return "", c.err
}

func TestProductionProfileSaveEncryptsKeyAndBlankKeepsCredential(t *testing.T) {
	srv, st := newPostgresTestServer(t, &fakeScraper{})
	srv.SetProductionMode(true)
	userID, cookie := createSessionUser(t, st, "profile-key@example.invalid", "profile-key-session")
	cipher := newAIRuntimeTestCipher(t, 0x51)
	srv.SetCredentialCipher(cipher)
	srv.newAIProvider = func(string, string, string, time.Duration) (ai.Provider, error) { return rerateStub(), nil }

	postProfile := func(key, likes string) *httptest.ResponseRecorder {
		form := url.Values{
			"ai_provider": {"anthropic"},
			"ai_model":    {"claude-haiku-4-5-20251001"},
			"ai_key":      {key},
			"job_likes":   {likes},
			"min_score":   {"0"},
		}
		req := httptest.NewRequest(http.MethodPost, "/profile", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(cookie)
		addCSRFToRequest(req, srv, cookie)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		return rec
	}
	if rec := postProfile("synthetic-profile-provider-key", "first goal"); rec.Code != http.StatusSeeOther {
		t.Fatalf("new-key save status=%d body=%q", rec.Code, rec.Body.String())
	}
	before, ok, err := st.UserAICredential(context.Background(), userID, "anthropic")
	if err != nil || !ok {
		t.Fatalf("UserAICredential after save: ok=%v err=%v", ok, err)
	}
	opened, err := cipher.Open(userID, "anthropic", before.Ciphertext, before.Nonce, before.EncryptionVersion)
	if err != nil || opened != "synthetic-profile-provider-key" {
		t.Fatalf("stored credential did not decrypt to submitted value: matched=%v err=%v", opened == "synthetic-profile-provider-key", err)
	}
	if rec := postProfile("", "second goal"); rec.Code != http.StatusSeeOther {
		t.Fatalf("blank-key save status=%d body=%q", rec.Code, rec.Body.String())
	}
	after, ok, err := st.UserAICredential(context.Background(), userID, "anthropic")
	if err != nil || !ok || string(after.Ciphertext) != string(before.Ciphertext) || string(after.Nonce) != string(before.Nonce) || after.EncryptionVersion != before.EncryptionVersion {
		t.Fatalf("blank key changed credential: ok=%v err=%v", ok, err)
	}
}

func TestProductionProfileSaveEncryptionFailureChangesNothing(t *testing.T) {
	srv, st := newPostgresTestServer(t, &fakeScraper{})
	srv.SetProductionMode(true)
	userID, cookie := createSessionUser(t, st, "profile-seal-fail@example.invalid", "profile-seal-fail-session")
	saveAIRuntimeProfile(t, st, userID, profile.Profile{JobLikes: "unchanged goal"})
	srv.SetCredentialCipher(failingSealCipher{err: errors.New("synthetic seal failure")})
	form := url.Values{"ai_provider": {"anthropic"}, "ai_key": {"synthetic-new-key"}, "job_likes": {"must not persist"}}
	req := httptest.NewRequest(http.MethodPost, "/profile", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	addCSRFToRequest(req, srv, cookie)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "synthetic-new-key") {
		t.Fatalf("seal failure status=%d body=%q", rec.Code, rec.Body.String())
	}
	got, _, ok, err := st.ProfileForUser(context.Background(), userID)
	if err != nil || !ok || !strings.Contains(got, "unchanged goal") || strings.Contains(got, "must not persist") {
		t.Fatalf("profile changed after encryption failure: ok=%v err=%v profile=%s", ok, err, got)
	}
	if _, ok, err := st.UserAICredential(context.Background(), userID, "anthropic"); err != nil || ok {
		t.Fatalf("credential persisted after encryption failure: ok=%v err=%v", ok, err)
	}
}

func TestProductionProfileFailureAfterKeyPreparationRollsBackCredential(t *testing.T) {
	srv, st := newPostgresTestServer(t, &fakeScraper{})
	srv.SetProductionMode(true)
	userID, cookie := createSessionUser(t, st, "profile-rollback@example.invalid", "profile-rollback-session")
	base := newAIRuntimeTestCipher(t, 0x52)
	saveAIRuntimeProfile(t, st, userID, profile.Profile{AIProvider: "anthropic", JobLikes: "old goal"})
	saveAIRuntimeCredential(t, st, base, userID, "anthropic", "synthetic-old-key")
	before, _, _ := st.UserAICredential(context.Background(), userID, "anthropic")
	spy := &sealCountingCipher{inner: base}
	srv.SetCredentialCipher(spy)
	if _, err := st.SQLDB().ExecContext(context.Background(), `
CREATE FUNCTION fail_handler_profile_save() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'synthetic profile failure'; END $$;
CREATE TRIGGER fail_handler_profile_save
BEFORE INSERT OR UPDATE ON profiles
FOR EACH ROW EXECUTE FUNCTION fail_handler_profile_save()`); err != nil {
		t.Fatalf("install failure trigger: %v", err)
	}
	form := url.Values{"ai_provider": {"anthropic"}, "ai_key": {"synthetic-new-key"}, "job_likes": {"new goal"}}
	req := httptest.NewRequest(http.MethodPost, "/profile", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	addCSRFToRequest(req, srv, cookie)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError || spy.seals.Load() != 1 || strings.Contains(rec.Body.String(), "synthetic-new-key") {
		t.Fatalf("profile failure status=%d seals=%d body=%q", rec.Code, spy.seals.Load(), rec.Body.String())
	}
	after, ok, err := st.UserAICredential(context.Background(), userID, "anthropic")
	if err != nil || !ok || string(after.Ciphertext) != string(before.Ciphertext) || string(after.Nonce) != string(before.Nonce) || after.EncryptionVersion != before.EncryptionVersion {
		t.Fatalf("credential changed after profile failure: ok=%v err=%v", ok, err)
	}
	got, _, ok, err := st.ProfileForUser(context.Background(), userID)
	if err != nil || !ok || !strings.Contains(got, "old goal") || strings.Contains(got, "new goal") {
		t.Fatalf("profile changed after rollback: ok=%v err=%v profile=%s", ok, err, got)
	}
}

func (c *countingCipher) Seal(userID int64, provider, plaintext string) ([]byte, []byte, int16, error) {
	return c.inner.Seal(userID, provider, plaintext)
}

func (c *countingCipher) Open(userID int64, provider string, ciphertext, nonce []byte, version int16) (string, error) {
	c.opens.Add(1)
	return c.inner.Open(userID, provider, ciphertext, nonce, version)
}

func TestProductionRerateDecryptsCredentialOnceForMultipleRows(t *testing.T) {
	srv, st := newPostgresTestServer(t, &fakeScraper{})
	ctx := context.Background()
	userID := insertAIRuntimeTestUser(t, st, "decrypt-once@example.invalid")
	prof := profile.Profile{CareerYears: 0, JobLikes: "백엔드 서버 개발", AIProvider: "anthropic", AIModel: "synthetic-model"}
	zero := 0
	prof.MinScore = &zero
	saveAIRuntimeProfile(t, st, userID, prof)
	base := newAIRuntimeTestCipher(t, 0x61)
	saveAIRuntimeCredential(t, st, base, userID, "anthropic", "synthetic-decrypt-once-key")
	spy := &countingCipher{inner: base}
	srv.SetCredentialCipher(spy)
	provider := rerateStub()
	srv.newAIProvider = func(string, string, string, time.Duration) (ai.Provider, error) { return provider, nil }
	now := time.Now().UTC()
	for _, sourceID := range []string{"decrypt-a", "decrypt-b", "decrypt-c"} {
		p := listingPosting(sourceID, "신입 백엔드 개발자")
		p.Description = "서버 개발자를 찾습니다"
		p.FirstSeenAt, p.LastSeenAt = now, now
		mustUpsert(t, st, p)
	}
	runtime, err := srv.aiRuntimeForUser(ctx, userID)
	if err != nil {
		t.Fatalf("aiRuntimeForUser: %v", err)
	}
	if _, err := srv.scoreAll(ctx, userID, runtime); err != nil {
		t.Fatalf("scoreAll: %v", err)
	}
	if _, err := srv.runRerate(ctx, "today", noopEmit, userID, runtime); err != nil {
		t.Fatalf("runRerate: %v", err)
	}
	if got := spy.opens.Load(); got != 1 {
		t.Fatalf("credential decrypts = %d, want exactly 1 for the operation", got)
	}
	if provider.ScoreDeltaCalls != 3 {
		t.Fatalf("ScoreDelta calls = %d, want 3 rows", provider.ScoreDeltaCalls)
	}
}

type isolatedProvider struct {
	name  string
	delta int
	mu    sync.Mutex
	calls int
}

func (p *isolatedProvider) Name() string { return p.name }
func (p *isolatedProvider) Extract(context.Context, string) (ai.Extraction, ai.Usage, error) {
	return ai.Extraction{}, ai.Usage{}, ai.ErrNotImplemented
}
func (p *isolatedProvider) ScoreDelta(context.Context, string, string) ([]ai.RawDeltaItem, ai.Usage, error) {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()
	return []ai.RawDeltaItem{{Signal: p.name, Kind: ai.KindPresence, Delta: p.delta, Quote: "서버 개발자를 찾습니다"}}, ai.Usage{InputTokens: 10, OutputTokens: 2}, nil
}

func TestProductionConcurrentReratesIsolateUserRuntimeScoresAndUsage(t *testing.T) {
	srv, st := newPostgresTestServer(t, &fakeScraper{})
	ctx := context.Background()
	userA := insertAIRuntimeTestUser(t, st, "concurrent-a@example.invalid")
	userB := insertAIRuntimeTestUser(t, st, "concurrent-b@example.invalid")
	zero := 0
	prof := profile.Profile{CareerYears: 0, MinScore: &zero, JobLikes: "백엔드 서버 개발"}
	saveAIRuntimeProfile(t, st, userA, prof)
	saveAIRuntimeProfile(t, st, userB, prof)
	now := time.Now().UTC()
	p := listingPosting("concurrent-shared", "신입 백엔드 개발자")
	p.Description = "서버 개발자를 찾습니다"
	p.FirstSeenAt, p.LastSeenAt = now, now
	postingID := mustUpsert(t, st, p)
	providerA := &isolatedProvider{name: "user-a-provider", delta: 7}
	providerB := &isolatedProvider{name: "user-b-provider", delta: -4}
	runtimeA := testAIRuntime(userA, providerA, "model-a")
	runtimeB := testAIRuntime(userB, providerB, "model-b")
	if _, err := srv.scoreAll(ctx, userA, runtimeA); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.scoreAll(ctx, userB, runtimeB); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, op := range []struct {
		userID  int64
		runtime *AIRuntime
	}{{userA, runtimeA}, {userB, runtimeB}} {
		wg.Add(1)
		go func(userID int64, runtime *AIRuntime) {
			defer wg.Done()
			_, err := srv.runRerate(ctx, "today", noopEmit, userID, runtime)
			errs <- err
		}(op.userID, op.runtime)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent rerate: %v", err)
		}
	}
	scoreA, ok, err := st.ScoreByPostingIDForUser(ctx, userA, postingID)
	if err != nil || !ok {
		t.Fatalf("user A score: ok=%v err=%v", ok, err)
	}
	scoreB, ok, err := st.ScoreByPostingIDForUser(ctx, userB, postingID)
	if err != nil || !ok {
		t.Fatalf("user B score: ok=%v err=%v", ok, err)
	}
	if scoreA.Total == scoreB.Total || scoreA.Total <= scoreB.Total {
		t.Fatalf("user scores crossed: A=%s B=%s", scoreA.BreakdownJSON, scoreB.BreakdownJSON)
	}
	hash := profile.AIInputHash(prof)
	deltaA, ok, err := st.AIScore(ctx, userA, postingID, hash, runtimeA.Version)
	if err != nil || !ok || deltaA.NetDelta != 7 {
		t.Fatalf("user A delta=%+v ok=%v err=%v", deltaA, ok, err)
	}
	deltaB, ok, err := st.AIScore(ctx, userB, postingID, hash, runtimeB.Version)
	if err != nil || !ok || deltaB.NetDelta != -4 {
		t.Fatalf("user B delta=%+v ok=%v err=%v", deltaB, ok, err)
	}
	day := time.Now().UTC().Format("2006-01-02")
	for _, userID := range []int64{userA, userB} {
		in, out, err := st.AIUsageForDay(ctx, userID, day)
		if err != nil || in != 10 || out != 2 {
			t.Fatalf("user %d daily usage=(%d,%d) err=%v, want (10,2)", userID, in, out, err)
		}
		monthIn, monthOut, err := st.AIUsageForMonth(ctx, userID, day[:7])
		if err != nil || monthIn != 10 || monthOut != 2 {
			t.Fatalf("user %d monthly usage=(%d,%d) err=%v, want (10,2)", userID, monthIn, monthOut, err)
		}
	}
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
	if p, err := profile.Unmarshal(gotB); err != nil || p.EffectiveAIDailyTokenCap() != 222222 {
		t.Fatalf("userB effective AI daily cap not persisted: profile=%s err=%v", gotB, err)
	}

	assertPageContains(t, srv, cookieA, "/profile", "유저A 새 목표")
	assertPageMissing(t, srv, cookieA, "/profile", "유저B 새 목표")
	assertPageContains(t, srv, cookieB, "/profile", "유저B 새 목표")
	assertPageMissing(t, srv, cookieB, "/profile", "유저A 새 목표")
}

func newPostgresTestServer(t *testing.T, f *fakeScraper) (*Server, *storage.Store) {
	t.Helper()
	databaseURL := os.Getenv("JOBCRON_TEST_POSTGRES_URL")
	if databaseURL == "" {
		t.Skip("JOBCRON_TEST_POSTGRES_URL not set")
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
