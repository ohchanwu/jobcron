package pacing

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
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
