package server

import (
	"encoding/json"
	"net/http"
	"sync"
)

type rerateState string

const (
	rerateStateIdle    rerateState = "idle"
	rerateStateRunning rerateState = "running"
	rerateStateDone    rerateState = "done"
	rerateStateFailed  rerateState = "failed"
)

type rerateKey struct {
	userID  int64
	surface string
}

type rerateStatus struct {
	RunID    uint64      `json:"run_id"`
	State    rerateState `json:"state"`
	Status   string      `json:"status,omitempty"`
	Progress string      `json:"progress,omitempty"`
	Message  string      `json:"message,omitempty"`
}

type rerateTracker struct {
	mu     sync.RWMutex
	nextID uint64
	runs   map[rerateKey]rerateStatus
}

func newRerateTracker() *rerateTracker {
	return &rerateTracker{runs: make(map[rerateKey]rerateStatus)}
}

func (t *rerateTracker) start(userID int64, surface string) rerateStatus {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.nextID++
	status := rerateStatus{RunID: t.nextID, State: rerateStateRunning}
	t.runs[rerateKey{userID: userID, surface: surface}] = status
	return status
}

func (t *rerateTracker) record(userID int64, surface string, runID uint64, event, data string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	key := rerateKey{userID: userID, surface: surface}
	status, ok := t.runs[key]
	if !ok || status.RunID != runID {
		return
	}
	switch event {
	case "status":
		status.Status = data
	case "progress":
		status.Progress = data
	case "done":
		status.State = rerateStateDone
		status.Message = data
	case "failed":
		status.State = rerateStateFailed
		status.Message = data
	default:
		return
	}
	t.runs[key] = status
}

func (t *rerateTracker) snapshot(userID int64, surface string) (rerateStatus, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	status, ok := t.runs[rerateKey{userID: userID, surface: surface}]
	return status, ok
}

func (s *Server) handleRerateStatus(w http.ResponseWriter, r *http.Request) {
	surface := r.URL.Query().Get("surface")
	if !validRerateSurface(surface) {
		http.Error(w, "알 수 없는 화면이에요.", http.StatusBadRequest)
		return
	}
	userID, err := s.stateUserID(r.Context(), r)
	if err != nil {
		writeAuthUnauthorized(w)
		return
	}
	status, ok := s.rerates.snapshot(userID, surface)
	if !ok {
		status = rerateStatus{State: rerateStateIdle}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(status); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
