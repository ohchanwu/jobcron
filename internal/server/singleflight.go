package server

import "sync"

// singleFlight gates concurrent scrapes: at most one in flight per source.
type singleFlight struct {
	mu      sync.Mutex
	running map[string]bool
}

func newSingleFlight() *singleFlight {
	return &singleFlight{running: map[string]bool{}}
}

// tryAcquire marks source as scraping and returns true, or returns false when
// a scrape for that source is already in progress.
func (s *singleFlight) tryAcquire(source string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running[source] {
		return false
	}
	s.running[source] = true
	return true
}

// release marks source as no longer scraping.
func (s *singleFlight) release(source string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.running, source)
}
