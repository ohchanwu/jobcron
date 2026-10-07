package server

import (
	"context"
	"testing"
	"time"

	"github.com/ohchanwu/jobcron/internal/ai"
)

func TestSQLiteRerateFixtureKeepsProviderCallsConcurrent(t *testing.T) {
	srv, stub, runtime := seedRerate(t)
	if got := srv.store.SQLDB().Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("SQLite fixture connection limit = %d, want 1", got)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	stub.ScoreDeltaFn = func(ctx context.Context, _, _ string) ([]ai.RawDeltaItem, ai.Usage, error) {
		entered <- struct{}{}
		select {
		case <-release:
			return nil, ai.Usage{}, nil
		case <-ctx.Done():
			return nil, ai.Usage{}, ctx.Err()
		}
	}
	type result struct {
		summary rerateSummary
		err     error
	}
	done := make(chan result, 1)
	go func() {
		summary, err := srv.runRerate(ctx, "today", noopEmit, 1, runtime)
		done <- result{summary, err}
	}()
	// Both providers must start BEFORE either is released: the fixture only
	// queues SQLite I/O, not provider calls or the production worker pool.
	concurrent := true
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-ctx.Done():
			concurrent = false
		}
	}
	close(release)
	got := <-done // runRerate drains its workers, also on context cancellation.
	if !concurrent {
		t.Fatal("SQLite fixture serialized provider calls")
	}
	if got.err != nil || got.summary.Analyzed != 2 || got.summary.NoSignal != 2 || stub.ScoreDeltaCalls != 2 {
		t.Fatalf("concurrent fixture result = %+v, err=%v, calls=%d", got.summary, got.err, stub.ScoreDeltaCalls)
	}
	if got := srv.store.SQLDB().Stats().OpenConnections; got != 1 {
		t.Fatalf("SQLite fixture opened %d connections, want 1", got)
	}
}
