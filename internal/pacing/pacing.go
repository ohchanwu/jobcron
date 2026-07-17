package pacing

import (
	"context"
	"sync"
	"time"
)

type Pacer struct {
	spacing   time.Duration
	mu        sync.Mutex
	lastStart time.Time
}

func New(spacing time.Duration) *Pacer {
	return &Pacer{spacing: spacing}
}

func (p *Pacer) Wait(ctx context.Context) error {
	p.mu.Lock()
	var wait time.Duration
	if !p.lastStart.IsZero() {
		if elapsed := time.Since(p.lastStart); elapsed < p.spacing {
			wait = p.spacing - elapsed
		}
	}
	p.lastStart = time.Now().Add(wait)
	p.mu.Unlock()
	if wait <= 0 {
		return nil
	}
	select {
	case <-time.After(wait):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
