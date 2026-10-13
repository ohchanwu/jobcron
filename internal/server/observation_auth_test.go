package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireObservationAuthLocalAndOrigin(t *testing.T) {
	srv, _ := newTestServer(t, &fakeScraper{})
	for _, tc := range []struct {
		name, target, origin, site string
		demo                       bool
		want                       int
	}{
		{"local", "http://example.com/api/posting-view?surface=today", "", "", false, 204},
		{"same origin", "http://example.com/api/posting-view", "http://example.com", "same-origin", false, 204},
		{"foreign origin", "http://example.com/api/posting-view", "http://foreign.invalid", "", false, 403},
		{"cross site", "http://example.com/api/posting-view", "", "cross-site", false, 403},
		{"url credentials", "http://user:password@example.com/api/posting-view", "", "", false, 403},
		{"user selector", "http://example.com/api/posting-view?user_id=2", "", "", false, 400},
		{"demo", "http://example.com/api/posting-view", "", "", true, 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv.SetDemoMode(tc.demo)
			r := httptest.NewRequest(http.MethodGet, tc.target, nil)
			r.Header.Set("Origin", tc.origin)
			r.Header.Set("Sec-Fetch-Site", tc.site)
			w := httptest.NewRecorder()
			srv.requireObservationAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				id, err := srv.observationUserID(r.Context(), r)
				if err != nil || id != 0 {
					t.Fatalf("fixed SQLite identity %d %v", id, err)
				}
				w.WriteHeader(http.StatusNoContent)
			})).ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("status=%d want=%d body=%q", w.Code, tc.want, w.Body.String())
			}
		})
	}
}

func TestRequireObservationAuthExactSessionDoesNotTouchActivity(t *testing.T) {
	srv, st := newPostgresTestServer(t, &fakeScraper{})
	srv.SetProductionMode(true)
	user, cookie := createSessionUser(t, st, "observer@example.invalid", "observer-session")
	var before, after string
	if err := st.SQLDB().QueryRow(`SELECT last_seen_at::text FROM sessions WHERE user_id=$1`, user).Scan(&before); err != nil {
		t.Fatal(err)
	}
	called := 0
	h := srv.requireObservationAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := srv.observationUserID(r.Context(), r)
		if err != nil || id != user {
			t.Fatalf("identity=%d err=%v", id, err)
		}
		called++
		w.WriteHeader(204)
	}))
	for _, valid := range []bool{true, false} {
		r := httptest.NewRequest("GET", "https://example.com/api/posting-view", nil)
		if valid {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		want := 401
		if valid {
			want = 204
		}
		if w.Code != want {
			t.Fatalf("status=%d want=%d", w.Code, want)
		}
	}
	if err := st.SQLDB().QueryRow(`SELECT last_seen_at::text FROM sessions WHERE user_id=$1`, user).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after || called != 1 {
		t.Fatal("observation mutated activity or bypassed auth")
	}
}
