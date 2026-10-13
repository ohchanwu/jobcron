package server

import (
	"context"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/ohchanwu/jobcron/internal/auth"
	"github.com/ohchanwu/jobcron/internal/storage"
)

func TestPostingAccountRevocationClosesOnlyRevokedSessions(t *testing.T) {
	for _, dialect := range []string{"sqlite", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var srv *Server
			var st *storage.Store
			if dialect == "sqlite" {
				srv, st = newTestServer(t, &fakeScraper{})
			} else {
				srv, st = newPostgresTestServer(t, &fakeScraper{})
			}
			srv.SetProductionMode(true)
			user := createAccountTestUser(t, st, "observer-account@example.invalid", accountCurrentPassword, "current-session")
			if err := st.CreateSession(context.Background(), user.ID, auth.HashSessionToken("other-session"), time.Now().Add(time.Hour)); err != nil {
				t.Fatal(err)
			}
			current, _ := srv.postingUpdates.subscribe(user.ID, auth.HashSessionToken("current-session"))
			defer current.close()
			other, _ := srv.postingUpdates.subscribe(user.ID, auth.HashSessionToken("other-session"))
			defer other.close()
			r := accountFormRequest(t, srv, "/account/password", "current-session", passwordChangeForm(accountCurrentPassword, accountNewPassword, accountNewPassword))
			w := httptest.NewRecorder()
			srv.Handler().ServeHTTP(w, r)
			if w.Code != 303 {
				t.Fatal(w.Code, w.Body.String())
			}
			select {
			case <-other.done:
			default:
				t.Fatal("password change left revoked observer connected")
			}
			select {
			case <-current.done:
				t.Fatal("password change closed retained submitting session")
			default:
			}
			r = accountFormRequest(t, srv, "/account/delete", "current-session", url.Values{"current_password": {accountNewPassword}, "confirm_email": {"observer-account@example.invalid"}})
			w = httptest.NewRecorder()
			srv.Handler().ServeHTTP(w, r)
			if w.Code != 303 {
				t.Fatal(w.Code, w.Body.String())
			}
			select {
			case <-current.done:
			default:
				t.Fatal("deleted account retained an observer")
			}
		})
	}
}
