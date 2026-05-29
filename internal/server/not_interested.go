package server

import (
	"encoding/json"
	"net/http"
	"time"
)

// handleNotInterestedAdd mutes a posting ("관심 없음"). Idempotent — a repeat
// PUT does not advance muted_at. A muted posting vanishes from the briefing
// and the 관심 공고 list entirely; if it is also bookmarked it stays on
// /bookmarks.
func (s *Server) handleNotInterestedAdd(w http.ResponseWriter, r *http.Request) {
	id, ok := postingID(w, r)
	if !ok {
		return
	}
	if err := s.store.SetNotInterested(r.Context(), id, time.Now()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeNotInterestedState(w, true)
}

// handleNotInterestedRemove un-mutes a posting. Idempotent — un-muting a
// never-muted posting is a no-op success.
func (s *Server) handleNotInterestedRemove(w http.ResponseWriter, r *http.Request) {
	id, ok := postingID(w, r)
	if !ok {
		return
	}
	if err := s.store.ClearNotInterested(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeNotInterestedState(w, false)
}

// writeNotInterestedState replies with the new mute state as JSON so the
// client can mirror its UI to the source of truth without re-reading.
func writeNotInterestedState(w http.ResponseWriter, muted bool) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]bool{"not_interested": muted})
}
