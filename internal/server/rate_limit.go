package server

import (
	"container/list"
	"crypto/sha256"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	loginRateLimitMaxFailures = 5
	loginRateLimitWindow      = 15 * time.Minute
	loginRateLimitMaxKeys     = 1024
	loginRateLimitPruneEvery  = time.Minute
	proxySecretHeaderName     = "X-Jobcron-Proxy"
)

type loginRateLimiter struct {
	mu        sync.Mutex
	now       func() time.Time
	nextPrune time.Time
	attempts  map[[sha256.Size]byte]*list.Element
	order     list.List
}

type loginAttempts struct {
	key       [sha256.Size]byte
	count     int
	windowEnd time.Time
}

func newLoginRateLimiter() *loginRateLimiter {
	return &loginRateLimiter{
		now:      time.Now,
		attempts: map[[sha256.Size]byte]*list.Element{},
	}
}

func (l *loginRateLimiter) reserveFailure(ip, email string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	key := loginRateLimitKey(ip, email)
	now := l.now()
	if !now.Before(l.nextPrune) {
		l.pruneExpired(now)
		l.nextPrune = now.Add(loginRateLimitPruneEvery)
	}
	element, ok := l.attempts[key]
	if ok {
		a := element.Value.(*loginAttempts)
		if now.Before(a.windowEnd) {
			if a.count >= loginRateLimitMaxFailures {
				return false
			}
			a.count++
			return true
		}
		l.remove(element)
	}
	if len(l.attempts) >= loginRateLimitMaxKeys {
		l.remove(l.order.Front())
	}
	// Entries stay in creation order rather than moving on each failure. That
	// keeps the hot path O(1) and makes capacity eviction deterministic.
	a := &loginAttempts{key: key, count: 1, windowEnd: now.Add(loginRateLimitWindow)}
	element = l.order.PushBack(a)
	l.attempts[key] = element
	return true
}

func (l *loginRateLimiter) reserveIP(ip string) bool {
	return l.reserveFailure(ip, "")
}

func (l *loginRateLimiter) reset(ip, email string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if element := l.attempts[loginRateLimitKey(ip, email)]; element != nil {
		l.remove(element)
	}
}

func (l *loginRateLimiter) resetIP(ip string) {
	l.reset(ip, "")
}

func (l *loginRateLimiter) pruneExpired(now time.Time) {
	for element := l.order.Front(); element != nil; {
		next := element.Next()
		if !now.Before(element.Value.(*loginAttempts).windowEnd) {
			l.remove(element)
		}
		element = next
	}
}

func (l *loginRateLimiter) remove(element *list.Element) {
	if element == nil {
		return
	}
	delete(l.attempts, element.Value.(*loginAttempts).key)
	l.order.Remove(element)
}

func loginRateLimitKey(ip, email string) [sha256.Size]byte {
	return sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(ip)) + "\x00" + strings.ToLower(strings.TrimSpace(email))))
}

func (s *Server) clientIP(r *http.Request) string {
	host := remoteHost(r.RemoteAddr)
	if s.trustedProxyRequest(r) {
		if forwarded := forwardedClientIP(r); forwarded != "" {
			return forwarded
		}
	}
	return host
}

func remoteHost(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil && host != "" {
		return host
	}
	return remoteAddr
}

func (s *Server) trustedProxyRequest(r *http.Request) bool {
	if s.proxySecret == "" {
		return false
	}
	return subtleConstantTimeEqual(r.Header.Get(proxySecretHeaderName), s.proxySecret)
}

func subtleConstantTimeEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := 0; i < len(a); i++ {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}

func forwardedClientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		for _, part := range strings.Split(forwarded, ",") {
			candidate := strings.TrimSpace(part)
			if net.ParseIP(candidate) != nil {
				return candidate
			}
		}
	}
	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); net.ParseIP(realIP) != nil {
		return realIP
	}
	return ""
}
