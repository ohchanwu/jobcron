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
	// Private controlled-time seam, fixed before any callers start.
	now  func() time.Time
	wait func(context.Context, time.Duration) error
}

func New(spacing time.Duration) *Pacer {
	return &Pacer{spacing: spacing, now: time.Now, wait: waitTimer}
}

func (p *Pacer) Wait(ctx context.Context) error {
	for {
		p.mu.Lock()
		if err := ctx.Err(); err != nil {
			p.mu.Unlock()
			return err
		}
		// Preserve the usable, unpaced zero value as well as New's test seam.
		if p.now == nil {
			p.now = time.Now
		}
		if p.wait == nil {
			p.wait = waitTimer
		}
		now := p.now()
		// Deadline timer callbacks can race an admission timer at the boundary.
		if deadline, ok := ctx.Deadline(); ok && !now.Before(deadline) {
			p.mu.Unlock()
			return context.DeadlineExceeded
		}
		wait := p.spacing - now.Sub(p.lastStart)
		if p.spacing <= 0 || p.lastStart.IsZero() || wait <= 0 {
			p.lastStart = now
			p.mu.Unlock()
			return nil
		}
		p.mu.Unlock()
		if err := p.wait(ctx, wait); err != nil {
			return err
		}
		// Recheck after waking: overdue reservations must not become a burst.
	}
}

func waitTimer(ctx context.Context, wait time.Duration) error {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
