package server

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ohchanwu/job-scraper/internal/storage"
)

// Scheduler runs the optional daily scheduled scrape loop.
type Scheduler struct {
	done chan struct{}
}

// Done closes when the scheduler loop exits.
func (s *Scheduler) Done() <-chan struct{} { return s.done }

// SchedulerConfig contains the dependencies for the daily scrape scheduler.
type SchedulerConfig struct {
	Server          *Server
	DailyScrapeTime string

	// Now and Sleep are injectable so tests can drive the loop without waiting
	// for wall-clock time. Production callers leave them nil.
	Now   func() time.Time
	Sleep func(context.Context, time.Duration) error
}

// StartScheduler starts the daily scheduled scrape loop.
func StartScheduler(ctx context.Context, cfg SchedulerConfig) (*Scheduler, error) {
	if cfg.Server == nil {
		return nil, fmt.Errorf("scheduler: server is required")
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Sleep == nil {
		cfg.Sleep = sleepContext
	}
	if _, err := nextScheduledRun(cfg.Now(), cfg.DailyScrapeTime); err != nil {
		return nil, err
	}

	s := &Scheduler{done: make(chan struct{})}
	go func() {
		defer close(s.done)
		for {
			now := cfg.Now()
			next, err := nextScheduledRun(now, cfg.DailyScrapeTime)
			if err != nil {
				return
			}
			delay := next.Sub(now.In(kstLocation()))
			if delay < 0 {
				delay = 0
			}
			if err := cfg.Sleep(ctx, delay); err != nil {
				return
			}
			cfg.Server.runScheduledScrape(ctx)
		}
	}()
	return s, nil
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (s *Server) runScheduledScrape(ctx context.Context) {
	if !s.flight.tryAcquire(scrapeAllKey) {
		s.recordSkippedScheduledRun(ctx, "skipped: scrape already running")
		return
	}
	defer s.flight.release(scrapeAllKey)
	scrapeCtx, cancel := context.WithTimeout(ctx, scrapeMaxDuration)
	defer cancel()
	_, _ = s.runScrapeWithHistory(scrapeCtx, storage.ScrapeTriggerScheduled, noopSchedulerEmit)
}

func (s *Server) recordSkippedScheduledRun(ctx context.Context, reason string) {
	s.recordSkippedScheduledRunAfterStart(ctx, reason, nil)
}

func (s *Server) recordSkippedScheduledRunAfterStart(ctx context.Context, reason string, afterStart func()) {
	run, err := s.store.StartScrapeRun(ctx, storage.ScrapeTriggerScheduled)
	if err != nil {
		return
	}
	if afterStart != nil {
		afterStart()
	}
	finishCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = s.store.FinishScrapeRun(finishCtx, run.ID, storage.ScrapeResult{}, storage.ScrapeRunStatusFailure, reason)
}

func noopSchedulerEmit(event, data string) {}

func kstLocation() *time.Location {
	return time.FixedZone("KST", 9*60*60)
}

func nextScheduledRun(now time.Time, dailyTime string) (time.Time, error) {
	hour, minute, err := parseDailyScrapeTime(dailyTime)
	if err != nil {
		return time.Time{}, err
	}
	loc := kstLocation()
	kstNow := now.In(loc)
	next := time.Date(kstNow.Year(), kstNow.Month(), kstNow.Day(), hour, minute, 0, 0, loc)
	if !next.After(kstNow) {
		next = next.Add(24 * time.Hour)
	}
	return next, nil
}

func parseDailyScrapeTime(s string) (int, int, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("daily scrape time %q must use HH:MM", s)
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("daily scrape time %q must use HH:MM", s)
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("daily scrape time %q must use HH:MM", s)
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, 0, fmt.Errorf("daily scrape time %q must use HH:MM", s)
	}
	return hour, minute, nil
}
