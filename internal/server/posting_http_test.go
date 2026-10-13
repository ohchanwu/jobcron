package server

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ohchanwu/jobcron/internal/profile"
)

func TestPostingSnapshotNativeReadOnlyAndEscaped(t *testing.T) {
	srv, st := newPostgresTestServer(t, &fakeScraper{})
	srv.SetProductionMode(true)
	uid, cookie := createSessionUser(t, st, "snapshot-native@example.invalid", "snapshot-native-session")
	zero := 0
	saveAIRuntimeProfile(t, st, uid, profile.Profile{CareerYears: 0, MinScore: &zero})
	p := listingPosting("snapshot", "<img src=x onerror=alert(1)>")
	p.FirstSeenAt = time.Now()
	p.LastSeenAt = p.FirstSeenAt
	p.URL = "javascript:alert(1)"
	id := mustUpsert(t, st, p)
	if err := st.AddBookmark(context.Background(), uid, id); err != nil {
		t.Fatal(err)
	}
	var before, after string
	if err := st.SQLDB().QueryRow(`SELECT last_seen_at::text FROM sessions WHERE user_id=$1`, uid).Scan(&before); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []struct{ surface, sort string }{{"today", "score"}, {"bookmarks", "saved"}, {"archive", "date"}, {"archive", "score"}} {
		r := httptest.NewRequest("GET", "/api/posting-view?surface="+mode.surface+"&sort="+mode.sort+"&generation=7", nil)
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("snapshot %s/%s=%d %s", mode.surface, mode.sort, w.Code, w.Body.String())
		}
		var body struct {
			Schema, Surface, Sort, Generation, Context string
			Rows                                       []struct{ ID, HTML string }
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Schema != "posting-view-v1" || body.Surface != mode.surface || body.Sort != mode.sort || body.Generation != "7" || body.Context == "" || len(body.Rows) != 1 {
			t.Fatalf("envelope=%+v", body)
		}
		if strings.Contains(body.Rows[0].HTML, "<img") || strings.Contains(body.Rows[0].HTML, "href=\"javascript:") || !strings.Contains(body.Rows[0].HTML, "&lt;img") {
			t.Fatalf("unsafe fragment=%s", body.Rows[0].HTML)
		}
		page := "/briefing"
		if mode.surface == "bookmarks" {
			page = "/bookmarks"
		}
		if mode.surface == "archive" {
			page = "/?sort=" + mode.sort
		}
		initial := httptest.NewRequest("GET", page, nil)
		initial.AddCookie(cookie)
		pageW := httptest.NewRecorder()
		srv.Handler().ServeHTTP(pageW, initial)
		if pageW.Code != 200 || !strings.Contains(pageW.Body.String(), strings.TrimSpace(body.Rows[0].HTML)) {
			t.Fatal("initial navigation doesn't share escaped keyed fragment", page)
		}
		if w.Header().Get("Cache-Control") != "no-store, private" {
			t.Fatal(w.Header())
		}
	}
	if err := st.SQLDB().QueryRow(`SELECT last_seen_at::text FROM sessions WHERE user_id=$1`, uid).Scan(&after); err != nil || before != after {
		t.Fatal("snapshot wrote session activity", err)
	}
	scores, err := st.ScoresByPostingID(context.Background(), uid)
	if err != nil || len(scores) != 0 {
		t.Fatal("snapshot wrote scores", err)
	}
}

func TestPostingSnapshotAdmissionAndRevocationDuringRead(t *testing.T) {
	srv, st := newPostgresTestServer(t, &fakeScraper{})
	srv.SetProductionMode(true)
	uid, cookie := createSessionUser(t, st, "snapshot-revoke@example.invalid", "snapshot-revoke-session")
	saveAIRuntimeProfile(t, st, uid, profile.Profile{})
	entered := make(chan struct{}, 3)
	release := make(chan struct{})
	srv.afterRenderProfileSnapshot = func() { entered <- struct{}{}; <-release }
	read := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/api/posting-view?surface=archive&sort=date&generation=1", nil)
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, r)
		return w
	}
	done := make(chan *httptest.ResponseRecorder, 3)
	for i := 0; i < 2; i++ {
		go func() { done <- read() }()
		select {
		case <-entered:
		case <-time.After(3 * time.Second):
			t.Fatal("reads didn't enter")
		}
	}
	go func() { done <- read() }()
	select {
	case w := <-done:
		if w.Code != 429 {
			t.Fatalf("third concurrent read=%d", w.Code)
		}
	case <-time.After(time.Second):
		close(release)
		for i := 0; i < 3; i++ {
			<-done
		}
		t.Fatal("third read was not rejected at admission")
	}
	if err := st.RevokeSessionToken(context.Background(), cookie.Value); err != nil {
		t.Fatal(err)
	}
	close(release)
	for i := 0; i < 2; i++ {
		w := <-done
		if w.Code != http.StatusUnauthorized || strings.Contains(w.Body.String(), "\"rows\"") {
			t.Fatal("revoked read emitted private fragments", w.Code)
		}
	}
}

func TestPostingObserverFlushesAndClosesExactLogout(t *testing.T) {
	srv, st := newPostgresTestServer(t, &fakeScraper{})
	srv.SetProductionMode(true)
	uid, cookie := createSessionUser(t, st, "observer-stream@example.invalid", "observer-stream-session")
	saveAIRuntimeProfile(t, st, uid, profile.Profile{})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/api/posting-updates", nil)
	r.AddCookie(cookie)
	response, err := server.Client().Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 || response.Header.Get("Content-Type") != "text/event-stream" || response.Header.Get("X-Accel-Buffering") != "no" || response.Header.Get("Cache-Control") != "no-store, private" {
		t.Fatalf("stream=%d headers=%v", response.StatusCode, response.Header)
	}
	lines := make(chan string, 8)
	ended := make(chan struct{})
	go func() {
		defer close(ended)
		scan := bufio.NewScanner(response.Body)
		for scan.Scan() {
			lines <- scan.Text()
		}
	}()
	select {
	case line := <-lines:
		if line != "event: dirty" {
			t.Fatal(line)
		}
	case <-time.After(time.Second):
		t.Fatal("initial resync not flushed")
	}
	logout := httptest.NewRequest("POST", "/logout", nil)
	logout.AddCookie(cookie)
	addCSRFToRequest(logout, srv, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, logout)
	if w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	select {
	case <-ended:
	case <-time.After(time.Second):
		t.Fatal("exact revoked observer remained connected")
	}
}

func TestPostingObserverHeartbeatDoesNotRequestSnapshot(t *testing.T) {
	srv, _ := newTestServer(t, &fakeScraper{})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()
	client := &http.Client{Timeout: 18 * time.Second}
	response, err := client.Get(server.URL + "/api/posting-updates")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	scan := bufio.NewScanner(response.Body)
	for i := 0; i < 3; i++ {
		if !scan.Scan() {
			t.Fatal("initial resync wasn't flushed")
		}
	}
	if !scan.Scan() || scan.Text() != ": heartbeat" {
		t.Fatalf("heartbeat must not force reads more often than 30s: %q", scan.Text())
	}
}

func TestPostingSnapshotRejectsHeadWithoutSessionMutation(t *testing.T) {
	srv, st := newPostgresTestServer(t, &fakeScraper{})
	srv.SetProductionMode(true)
	uid, cookie := createSessionUser(t, st, "observer-head@example.invalid", "observer-head-session")
	var before, after string
	st.SQLDB().QueryRow(`SELECT last_seen_at::text FROM sessions WHERE user_id=$1`, uid).Scan(&before)
	r := httptest.NewRequest("HEAD", "/api/posting-view?surface=archive&sort=date&generation=1", nil)
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, r)
	st.SQLDB().QueryRow(`SELECT last_seen_at::text FROM sessions WHERE user_id=$1`, uid).Scan(&after)
	if w.Code != 405 || before != after {
		t.Fatalf("HEAD observation status=%d activity changed=%v", w.Code, before != after)
	}
}

func TestPostingContextIsPrivateUserScopedEvenForIdenticalProfiles(t *testing.T) {
	srv, st := newPostgresTestServer(t, &fakeScraper{})
	srv.SetProductionMode(true)
	a, ca := createSessionUser(t, st, "same-profile-a@example.invalid", "same-profile-a-session")
	b, cb := createSessionUser(t, st, "same-profile-b@example.invalid", "same-profile-b-session")
	saveAIRuntimeProfile(t, st, a, profile.Profile{})
	saveAIRuntimeProfile(t, st, b, profile.Profile{})
	var tokens []string
	for _, cookie := range []*http.Cookie{ca, cb} {
		r := httptest.NewRequest("GET", "/api/posting-view?surface=archive&sort=date&generation=1", nil)
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, r)
		var e struct{ Context string }
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &e) != nil {
			t.Fatal(w.Code)
		}
		tokens = append(tokens, e.Context)
	}
	if tokens[0] == tokens[1] {
		t.Fatal("identical profiles collided across private observation scopes")
	}
}

func TestPostingAuthenticationReadFailureIsUnavailableNotAuthLoss(t *testing.T) {
	srv, st := newPostgresTestServer(t, &fakeScraper{})
	srv.SetProductionMode(true)
	_, cookie := createSessionUser(t, st, "auth-unavailable@example.invalid", "auth-unavailable-session")
	if _, err := st.SQLDB().Exec(`DROP TABLE sessions`); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/posting-view?surface=archive&sort=date&generation=1", "/api/posting-updates"} {
		r := httptest.NewRequest("GET", path, nil)
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, r)
		if w.Code != 503 || strings.Contains(w.Body.String(), "rows") {
			t.Fatal("a database read failure must retain last good UI, not assert sign-out", w.Code)
		}
	}
}

func TestPostingSnapshotDeadlineIncludesMiddleware(t *testing.T) {
	srv, _ := newTestServer(t, &fakeScraper{})
	for _, path := range []string{"/api/posting-view", "/api/posting-updates"} {
		r := httptest.NewRequest("GET", path, nil)
		checked := false
		srv.requireObservationAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			checked = true
			deadline, bounded := r.Context().Deadline()
			if path == "/api/posting-view" && (!bounded || time.Until(deadline) > 10*time.Second) {
				t.Error("snapshot auth/read/render is not within the whole ten-second bound")
			}
			if path == "/api/posting-updates" && bounded {
				t.Error("initial auth bound must not terminate an admitted healthy stream")
			}
		})).ServeHTTP(httptest.NewRecorder(), r)
		if !checked {
			t.Fatal("handler not admitted")
		}
	}
}
