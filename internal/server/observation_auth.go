package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/ohchanwu/jobcron/internal/storage"
)

// observationUserID is deliberately independent of stateUserID/userFromRequest:
func listingReadPath(r *http.Request) bool {
	if r.Method != http.MethodGet {
		return false
	}
	switch r.URL.Path {
	case "/", "/briefing", "/bookmarks":
		return true
	}
	return false
}

// those helpers record activity. Each observation admission/emission rechecks
// the exact cookie without extending it or loading account/credential details.
func (s *Server) observationUserID(ctx context.Context, r *http.Request) (int64, error) {
	if s.demoMode {
		return 0, http.ErrNoCookie
	}
	if !s.productionMode {
		if s.store.Dialect() == storage.DialectSQLite {
			return 0, nil
		}
		if s.localUserID <= 0 {
			return 0, fmt.Errorf("server: observation local identity unavailable")
		}
		return s.localUserID, nil
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return 0, http.ErrNoCookie
	}
	id, ok, err := s.store.SessionIdentityReadOnly(ctx, cookie.Value)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, http.ErrNoCookie
	}
	return id.UserID, nil
}

func (s *Server) requireObservationAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, private")
		if r.URL.Path == "/api/posting-view" {
			ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
			defer cancel()
			r = r.WithContext(ctx)
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "observation requires GET", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.User != nil || r.Header.Get("Sec-Fetch-Site") == "cross-site" || r.Header.Get("Sec-Fetch-Site") == "same-site" {
			http.Error(w, "same-origin observation required", http.StatusForbidden)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			scheme := "http"
			if r.TLS != nil || s.productionMode {
				scheme = "https"
			}
			if err != nil || u.User != nil || u.Host != r.Host || u.Scheme != scheme || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
				http.Error(w, "same-origin observation required", http.StatusForbidden)
				return
			}
		}
		q, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil {
			http.Error(w, "invalid observation query", 400)
			return
		}
		for key := range q {
			switch key {
			case "surface", "sort", "generation":
			default:
				http.Error(w, "invalid observation query", 400)
				return
			}
		}
		lookupCtx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		_, err = s.observationUserID(lookupCtx, r)
		cancel()
		if err != nil {
			writeObservationAuthError(w, err)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Failure to read session storage is not evidence that the session was revoked.
// Retain the last good UI as unavailable; only a definite missing/expired identity
// is a 401 and causes the client to clear private content.
func writeObservationAuthError(w http.ResponseWriter, err error) {
	if errors.Is(err, http.ErrNoCookie) {
		writeAuthUnauthorized(w)
	} else {
		http.Error(w, "observation unavailable", http.StatusServiceUnavailable)
	}
}
