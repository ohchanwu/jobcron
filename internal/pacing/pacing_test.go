package pacing

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

func TestPacerSpacesConcurrentStarts(t *testing.T) {
	const spacing = 40 * time.Millisecond
	p := New(spacing)
	ready := make(chan struct{})
	times := make(chan time.Time, 3)
	var wg sync.WaitGroup
	for range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-ready
			if err := p.Wait(context.Background()); err != nil {
				t.Errorf("Wait: %v", err)
				return
			}
			times <- time.Now()
		}()
	}
	close(ready)
	wg.Wait()
	close(times)
	var got []time.Time
	for at := range times {
		got = append(got, at)
	}
	sort.Slice(got, func(i, j int) bool { return got[i].Before(got[j]) })
	for i := 1; i < len(got); i++ {
		if gap := got[i].Sub(got[i-1]); gap < 30*time.Millisecond {
			t.Fatalf("start gap = %v, want at least 30ms", gap)
		}
	}
}

func TestPacerWaitHonorsCancellation(t *testing.T) {
	p := New(time.Hour)
	if err := p.Wait(context.Background()); err != nil {
		t.Fatalf("first Wait: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := p.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait error = %v, want context.Canceled", err)
	}
}

func TestPacerDelayedWakeupsCannotBurst(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := New(10 * time.Second)
		var clockMu sync.Mutex
		now := time.Now()
		p.now = func() time.Time { clockMu.Lock(); defer clockMu.Unlock(); return now }
		wakeups := make(chan chan struct{}, 3)
		p.wait = func(ctx context.Context, d time.Duration) error {
			wake := make(chan struct{})
			wakeups <- wake
			select {
			case <-wake:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if err := p.Wait(context.Background()); err != nil {
			t.Fatal(err)
		}
		starts := make(chan time.Time, 3)
		var wg sync.WaitGroup
		for range 3 {
			wg.Go(func() {
				if err := p.Wait(context.Background()); err != nil {
					t.Error(err)
					return
				}
				starts <- p.now()
			})
		}
		synctest.Wait()
		// Release all overdue timers together, not at their originally due times.
		for step := range 3 {
			clockMu.Lock()
			if step == 0 {
				now = now.Add(time.Minute)
			} else {
				now = now.Add(10 * time.Second)
			}
			clockMu.Unlock()
			pending := len(wakeups)
			if pending != 3-step {
				t.Fatalf("pending=%d want %d", pending, 3-step)
			}
			for range pending {
				close(<-wakeups)
			}
			synctest.Wait()
			if len(starts) != step+1 {
				t.Fatalf("late wake burst: starts=%d want %d", len(starts), step+1)
			}
		}
		wg.Wait()
		close(starts)
		var previous time.Time
		for at := range starts {
			t.Logf("delayed admission=%s", at.Format(time.RFC3339Nano))
			if !previous.IsZero() && at.Sub(previous) < 10*time.Second {
				t.Errorf("late-start gap=%v", at.Sub(previous))
			}
			previous = at
		}
	})
}

func TestPacerCancellationDoesNotReserveFutureSlot(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := New(10 * time.Second)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := p.Wait(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("first canceled Wait=%v", err)
		}
		before := time.Now()
		if err := p.Wait(context.Background()); err != nil {
			t.Fatal(err)
		}
		if time.Now() != before {
			t.Fatal("canceled first waiter consumed a slot")
		}
		ctx, cancel = context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := p.Wait(ctx); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal(err)
		}
		if err := p.Wait(context.Background()); err != nil {
			t.Fatal(err)
		}
		if time.Since(before) != 10*time.Second {
			t.Fatalf("canceled waiter delayed next admission: %v", time.Since(before))
		}
	})
}

func TestPacerZeroValueRetainsUnpacedBehavior(t *testing.T) {
	var p Pacer
	if err := p.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := p.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("zero-value canceled Wait=%v", err)
	}
}
