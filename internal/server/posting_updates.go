package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"sync"
	"time"

	"github.com/ohchanwu/jobcron/internal/profile"
)

// Posting revisions are process-local cursors, not counts or distributed state.
// User cursors use the process monotonic clock, NOT another user's counter. Idle
// user records can therefore be evicted without rolling a returning cursor back
// or disclosing another account's paid progress. Re-registration forces resync.
type postingNotice struct {
	Epoch          string `json:"epoch"`
	GlobalRevision uint64 `json:"global_revision,string"`
	UserRevision   uint64 `json:"user_revision,string"`
	Context        string `json:"context,omitempty"`
	ContextChanged bool   `json:"context_changed,omitempty"`
	Resync         bool   `json:"resync"`
	Status         bool   `json:"status,omitempty"`
}

type postingUserUpdates struct {
	revision  uint64
	listeners map[*postingListener]struct{}
}

type postingUpdateHub struct {
	global         sync.RWMutex
	usersLocks     [64]sync.RWMutex // fixed-size locks; no unbounded idle-user lock map
	mu             sync.Mutex
	epoch          string
	contextSecret  string
	started        time.Time
	globalRevision uint64
	users          map[int64]*postingUserUpdates // only active listeners, at most 64
	total          int
	reads          map[int64]int // only admitted active reads, at most eight entries
	totalReads     int
}

type postingListener struct {
	hub         *postingUpdateHub
	userID      int64
	wake        chan struct{}
	done        chan struct{}
	sessionHash string
	pending     postingNotice // one coalesced slot, guarded by hub.mu
	closed      bool
}

func newPostingUpdateHub() *postingUpdateHub {
	return &postingUpdateHub{epoch: randomToken(), contextSecret: randomToken(), started: time.Now(), users: map[int64]*postingUserUpdates{}}
}
func (h *postingUpdateHub) cursor() uint64 { return uint64(time.Since(h.started).Nanoseconds()) + 1 }
func (h *postingUpdateHub) userLock(userID int64) *sync.RWMutex {
	return &h.usersLocks[uint64(userID)%uint64(len(h.usersLocks))]
}

// Lock order is global then user then metadata. The lease covers ONLY the
// bounded mutation/commit/publication. Flight comes first; budget, tracker,
// provider, rendering, network and sleeps must all remain outside these locks.
func (h *postingUpdateHub) beginWrite(userID int64, global bool) func(bool, string, ...bool) {
	if global {
		h.global.Lock()
	} else {
		h.global.RLock()
		h.userLock(userID).Lock()
	}
	return func(committed bool, context string, changed ...bool) {
		contextChanged := len(changed) > 0 && changed[0]
		if committed {
			h.mu.Lock()
			if global {
				h.globalRevision++
				for _, u := range h.users {
					for l := range u.listeners {
						l.enqueueLocked(postingNotice{Epoch: h.epoch, GlobalRevision: h.globalRevision})
					}
				}
			} else if u := h.users[userID]; u != nil {
				u.revision = max(u.revision+1, h.cursor())
				for l := range u.listeners {
					l.enqueueLocked(postingNotice{Epoch: h.epoch, GlobalRevision: h.globalRevision, UserRevision: u.revision, Context: context, ContextChanged: contextChanged, Resync: contextChanged})
				}
			}
			h.mu.Unlock()
		}
		if global {
			h.global.Unlock()
		} else {
			h.userLock(userID).Unlock()
			h.global.RUnlock()
		}
	}
}
func (h *postingUpdateHub) beginRead(userID int64) func() {
	h.global.RLock()
	h.userLock(userID).RLock()
	return func() { h.userLock(userID).RUnlock(); h.global.RUnlock() }
}

func (h *postingUpdateHub) beginReadContext(ctx context.Context, userID int64) (func(), error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if h.global.TryRLock() {
			if h.userLock(userID).TryRLock() {
				return func() { h.userLock(userID).RUnlock(); h.global.RUnlock() }, nil
			}
			h.global.RUnlock()
		}
		// No lock is held while waiting; canceled requests leave no goroutine.
		timer := time.NewTimer(time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}
func (h *postingUpdateHub) admitSnapshot(userID int64) (func(), bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.totalReads >= 8 || h.reads[userID] >= 2 {
		return nil, false
	}
	if h.reads == nil {
		h.reads = map[int64]int{}
	}
	h.totalReads++
	h.reads[userID]++
	return func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.totalReads--
		h.reads[userID]--
		if h.reads[userID] == 0 {
			delete(h.reads, userID)
		}
	}, true
}

func (h *postingUpdateHub) revisions(userID int64) postingNotice {
	h.mu.Lock()
	defer h.mu.Unlock()
	rev := h.cursor()
	if u := h.users[userID]; u != nil {
		rev = u.revision
	}
	return postingNotice{Epoch: h.epoch, GlobalRevision: h.globalRevision, UserRevision: rev}
}
func (h *postingUpdateHub) subscribe(userID int64, sessionHash ...string) (*postingListener, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	u := h.users[userID]
	if h.total >= 64 || (u != nil && len(u.listeners) >= 8) {
		return nil, false
	}
	if u == nil {
		u = &postingUserUpdates{revision: h.cursor(), listeners: map[*postingListener]struct{}{}}
		h.users[userID] = u
	}
	l := &postingListener{hub: h, userID: userID, wake: make(chan struct{}, 1), done: make(chan struct{})}
	if len(sessionHash) > 0 {
		l.sessionHash = sessionHash[0]
	}
	u.listeners[l] = struct{}{}
	h.total++
	return l, true
}
func (l *postingListener) enqueueLocked(n postingNotice) {
	if l.closed {
		return
	}
	if len(l.wake) > 0 {
		n.Resync = true
	}
	n.Resync = n.Resync || l.pending.Resync
	n.Status = n.Status || l.pending.Status
	if n.Context == "" && !n.ContextChanged {
		n.Context = l.pending.Context
	}
	n.ContextChanged = n.ContextChanged || l.pending.ContextChanged
	n.UserRevision = max(n.UserRevision, l.pending.UserRevision)
	n.GlobalRevision = max(n.GlobalRevision, l.pending.GlobalRevision)
	l.pending = n
	select {
	case l.wake <- struct{}{}:
	default:
	}
}
func (l *postingListener) take() postingNotice {
	l.hub.mu.Lock()
	defer l.hub.mu.Unlock()
	n := l.pending
	l.pending = postingNotice{}
	return n
}
func (l *postingListener) close() {
	h := l.hub
	h.mu.Lock()
	defer h.mu.Unlock()
	l.closeLocked()
}
func (l *postingListener) closeLocked() {
	h := l.hub
	if l.closed {
		return
	}
	l.closed = true
	close(l.done)
	u := h.users[l.userID]
	delete(u.listeners, l)
	h.total--
	if len(u.listeners) == 0 {
		delete(h.users, l.userID)
	}
}

func (h *postingUpdateHub) revokeSession(sessionHash string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, u := range h.users {
		for l := range u.listeners {
			if l.sessionHash == sessionHash {
				l.closeLocked()
			}
		}
	}
}

func (h *postingUpdateHub) revokeUser(userID int64, keepSession string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if u := h.users[userID]; u != nil {
		for l := range u.listeners {
			if keepSession == "" || l.sessionHash != keepSession {
				l.closeLocked()
			}
		}
	}
}

func (h *postingUpdateHub) statusChanged(userID int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if u := h.users[userID]; u != nil {
		for l := range u.listeners {
			l.enqueueLocked(postingNotice{Epoch: h.epoch, GlobalRevision: h.globalRevision, UserRevision: u.revision, Status: true})
		}
	}
}

func (s *Server) writePostingState(userID int64, global bool, mutation func() error) error {
	finish := s.postingUpdates.beginWrite(userID, global)
	err := mutation()
	finish(err == nil, "")
	return err
}

// Tokens cover stored nonsecret configuration, never a runtime. A private
// process salt prevents the public token from exposing profile/goal hashes.
func (h *postingUpdateHub) contextToken(userID int64, profileHash string, present bool) string {
	m := hmac.New(sha256.New, []byte(h.contextSecret))
	var owner [8]byte
	binary.BigEndian.PutUint64(owner[:], uint64(userID))
	m.Write(owner[:])
	m.Write([]byte(profileHash))
	if present {
		m.Write([]byte{1})
	} else {
		m.Write([]byte{0})
	}
	return hex.EncodeToString(m.Sum(nil))
}

func (s *Server) writePostingContext(ctx context.Context, userID int64, mutation func() error) error {
	finish := s.postingUpdates.beginWrite(userID, false)
	err := mutation()
	token := ""
	if err == nil {
		// Commit, metadata read and publication remain within the same barrier.
		tx, readErr := s.store.BeginPostingView(ctx, userID)
		if readErr == nil {
			pj, hash, _ := tx.Profile()
			prof, parseErr := profile.Unmarshal(pj)
			if parseErr == nil {
				present, presenceErr := tx.CredentialPresence(ctx, prof.AIProvider)
				if presenceErr == nil {
					token = s.postingUpdates.contextToken(userID, hash, present)
				}
			}
			tx.Close()
		}
	}
	// A metadata read can fail AFTER a successful commit (including request
	// cancellation). Invalidate old reads without inventing an unmatchable token;
	// the next complete read establishes the real context after recovery.
	finish(err == nil, token, true)
	return err
}
