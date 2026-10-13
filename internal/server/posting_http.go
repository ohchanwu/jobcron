package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/ohchanwu/jobcron/internal/auth"
	"github.com/ohchanwu/jobcron/internal/profile"
)

type postingViewRow struct {
	ID      string `json:"id"`
	Section string `json:"section"`
	HTML    string `json:"html"`
}
type postingViewSection struct {
	Key      string `json:"key"`
	Title    string `json:"title"`
	IsToday  bool   `json:"is_today"`
	Excluded bool   `json:"excluded"`
}
type postingViewEnvelope struct {
	Schema string `json:"schema"`
	postingNotice
	Generation      string                  `json:"generation"`
	Surface         string                  `json:"surface"`
	Sort            string                  `json:"sort"`
	ProfileRequired bool                    `json:"profile_required"`
	Rows            []postingViewRow        `json:"rows"`
	Sections        []postingViewSection    `json:"sections"`
	Total           int                     `json:"total"`
	Main            int                     `json:"main"`
	Excluded        int                     `json:"excluded"`
	Rerate          *rerateInfo             `json:"rerate"`
	Runs            map[string]rerateStatus `json:"runs"`
}

func postingViewRequest(r *http.Request) (surface, sort, generation string, ok bool) {
	q := r.URL.Query()
	surface, sort, generation = q.Get("surface"), q.Get("sort"), q.Get("generation")
	if len(q["surface"]) != 1 || len(q["sort"]) != 1 || len(q["generation"]) != 1 || len(generation) == 0 || len(generation) > 16 {
		return
	}
	n, err := strconv.ParseUint(generation, 10, 53)
	if err != nil || strconv.FormatUint(n, 10) != generation {
		return
	}
	ok = (surface == "today" && sort == "score") || (surface == "bookmarks" && sort == "saved") || (surface == "archive" && (sort == "date" || sort == "score"))
	return
}

func (s *Server) postingEnvelope(ctx context.Context, userID int64, surface, sort, generation string) (postingViewEnvelope, error) {
	v, err := s.loadPostingProjection(ctx, userID)
	if err != nil {
		return postingViewEnvelope{}, err
	}
	e := postingViewEnvelope{Schema: "posting-view-v1", postingNotice: v.revision, Generation: generation, Surface: surface, Sort: sort, ProfileRequired: !v.data.HasProfile, Rows: []postingViewRow{}, Sections: []postingViewSection{}, Runs: map[string]rerateStatus{}}
	// Tracker locks are acquired only AFTER the established read and released tx.
	for _, name := range []string{"today", "bookmarks", "archive"} {
		if run, ok := s.rerates.snapshot(userID, name); ok {
			e.Runs[name] = run
		}
	}
	appendSection := func(key, title string, today, excluded bool, rows []dashboardPosting) error {
		e.Sections = append(e.Sections, postingViewSection{key, title, today, excluded})
		for _, row := range rows {
			if err := ctx.Err(); err != nil {
				return err
			}
			var fragment bytes.Buffer
			if err := s.tmpl.ExecuteTemplate(&fragment, "posting", row); err != nil {
				return err
			}
			e.Rows = append(e.Rows, postingViewRow{fmt.Sprint(row.Posting.ID), key, fragment.String()})
		}
		return nil
	}
	now := time.Now()
	switch surface {
	case "today":
		b := s.briefingFromProjection(v, now)
		e.Total = len(b.Today) + len(b.Excluded)
		e.Main = len(b.Today)
		e.Excluded = len(b.Excluded)
		e.Rerate = b.Rerate
		if err = appendSection("main", "", false, false, b.Today); err == nil {
			err = appendSection("excluded", "관심 밖으로 분류된 공고", false, true, b.Excluded)
		}
	case "bookmarks":
		b := s.bookmarksFromProjection(v, now)
		e.Total = len(b.Postings)
		e.Main = e.Total
		e.Rerate = b.Rerate
		err = appendSection("main", "", false, false, b.Postings)
	case "archive":
		a := s.archiveFromProjection(v, now)
		if sort == "score" {
			a.applyScoreSort()
		}
		e.Total = a.Total
		e.Excluded = len(a.Excluded)
		e.Main = a.Total - e.Excluded
		e.Rerate = a.Rerate
		for _, day := range a.Days {
			key := day.Date
			if key == "" {
				key = "score"
			}
			if err = appendSection("day-"+key, day.Date, day.IsToday, false, day.Postings); err != nil {
				break
			}
		}
		if err == nil {
			err = appendSection("excluded", "관심 밖으로 분류된 공고", false, true, a.Excluded)
		}
	}
	return e, err
}

func (s *Server) handlePostingView(w http.ResponseWriter, r *http.Request) {
	surface, sort, generation, ok := postingViewRequest(r)
	if !ok {
		http.Error(w, "invalid posting view", 400)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	uid, err := s.observationUserID(ctx, r)
	if err != nil {
		writeObservationAuthError(w, err)
		return
	}
	release, ok := s.postingUpdates.admitSnapshot(uid)
	if !ok {
		http.Error(w, "posting view busy", 429)
		return
	}
	defer release()
	view, err := s.postingEnvelope(ctx, uid, surface, sort, generation)
	if err != nil {
		http.Error(w, "posting view unavailable", 503)
		return
	}
	// Buffer the complete representation before the exact-session recheck. Neither
	// a partial render nor an expired/revoked identity may emit private fragments.
	body, err := json.Marshal(view)
	if err != nil || ctx.Err() != nil {
		http.Error(w, "posting view unavailable", 503)
		return
	}
	current, err := s.observationUserID(ctx, r)
	if err != nil {
		writeObservationAuthError(w, err)
		return
	}
	if current != uid {
		writeAuthUnauthorized(w)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, private")
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, _ = w.Write(body)
}

func (s *Server) postingReadNotice(ctx context.Context, uid int64) (postingNotice, error) {
	end, err := s.postingUpdates.beginReadContext(ctx, uid)
	if err != nil {
		return postingNotice{}, err
	}
	defer end()
	tx, err := s.store.BeginPostingView(ctx, uid)
	if err != nil {
		return postingNotice{}, err
	}
	defer tx.Close()
	pj, hash, found := tx.Profile()
	var prof profile.Profile
	if found {
		prof, err = profile.Unmarshal(pj)
		if err != nil {
			return postingNotice{}, err
		}
	}
	present, err := tx.CredentialPresence(ctx, prof.AIProvider)
	if err != nil {
		return postingNotice{}, err
	}
	n := s.postingUpdates.revisions(uid)
	n.Context = s.postingUpdates.contextToken(uid, hash, present)
	n.Resync = true
	return n, nil
}

func (s *Server) handlePostingUpdates(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" {
		http.Error(w, "invalid observation query", 400)
		return
	}
	lookupCtx, lookupCancel := context.WithTimeout(r.Context(), 10*time.Second)
	uid, err := s.observationUserID(lookupCtx, r)
	lookupCancel()
	if err != nil {
		writeObservationAuthError(w, err)
		return
	}
	hash := ""
	if c, err := r.Cookie(sessionCookieName); err == nil {
		hash = auth.HashSessionToken(c.Value)
	}
	l, ok := s.postingUpdates.subscribe(uid, hash)
	if !ok {
		http.Error(w, "observation busy", 429)
		return
	}
	defer l.close()
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	initial, err := s.postingReadNotice(ctx, uid)
	cancel()
	if err != nil {
		http.Error(w, "observation unavailable", 503)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store, private")
	w.Header().Set("X-Accel-Buffering", "no")
	controller := http.NewResponseController(w)
	emit := func(n postingNotice) bool {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		current, err := s.observationUserID(ctx, r)
		if err != nil || current != uid {
			return false
		}
		select {
		case <-l.done:
			return false
		default:
		}
		body, err := json.Marshal(n)
		if err != nil {
			return false
		}
		_ = controller.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if _, err = fmt.Fprintf(w, "event: dirty\ndata: %s\n\n", body); err != nil {
			return false
		}
		return controller.Flush() == nil
	}
	if !emit(initial) {
		return
	}
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-l.done:
			return
		case <-l.wake:
			if !emit(l.take()) {
				return
			}
		case <-heartbeat.C:
			// Session checks run every 15s. The separate client 30s resync catches
			// lost hints/CLI writes; a heartbeat itself must not double that cadence.
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			current, err := s.observationUserID(ctx, r)
			cancel()
			if err != nil || current != uid {
				return
			}
			_ = controller.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
				return
			}
			if controller.Flush() != nil {
				return
			}
		}
	}
}
