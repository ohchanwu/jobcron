package server

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ohchanwu/job-scraper/internal/profile"
	"github.com/ohchanwu/job-scraper/internal/scraper"
	"github.com/ohchanwu/job-scraper/internal/storage"
)

func TestNextScheduledRunTodayWhenTimeStillAheadInKST(t *testing.T) {
	loc := kstLocation()
	now := time.Date(2026, 7, 10, 7, 0, 0, 0, loc)

	next, err := nextScheduledRun(now, "08:00")
	if err != nil {
		t.Fatalf("nextScheduledRun: %v", err)
	}

	want := time.Date(2026, 7, 10, 8, 0, 0, 0, loc)
	if !next.Equal(want) {
		t.Fatalf("next = %s, want %s", next.Format(time.RFC3339), want.Format(time.RFC3339))
	}
}

func TestNextScheduledRunTomorrowWhenTimeAlreadyPassedInKST(t *testing.T) {
	loc := kstLocation()
	now := time.Date(2026, 7, 10, 9, 0, 0, 0, loc)

	next, err := nextScheduledRun(now, "08:00")
	if err != nil {
		t.Fatalf("nextScheduledRun: %v", err)
	}

	want := time.Date(2026, 7, 11, 8, 0, 0, 0, loc)
	if !next.Equal(want) {
		t.Fatalf("next = %s, want %s", next.Format(time.RFC3339), want.Format(time.RFC3339))
	}
}

func TestNextScheduledRunInvalidTimeReturnsClearError(t *testing.T) {
	_, err := nextScheduledRun(time.Date(2026, 7, 10, 7, 0, 0, 0, time.UTC), "8am")
	if err == nil {
		t.Fatal("nextScheduledRun succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "daily scrape time") || !strings.Contains(err.Error(), "HH:MM") {
		t.Fatalf("error = %q, want clear HH:MM daily scrape time error", err.Error())
	}
}

func TestStartSchedulerRunsScheduledScrapeAfterSleep(t *testing.T) {
	f := &fakeScraper{listing: []scraper.Posting{listingPosting("1", "백엔드 신입")}}
	srv, st := newTestServer(t, f)
	ctx := context.Background()
	profJSON, _ := profile.Marshal(profile.Profile{CareerYears: 0})
	if _, _, err := st.SaveProfile(ctx, profJSON); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	schedulerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sleepCalls := 0
	_, err := StartScheduler(schedulerCtx, SchedulerConfig{
		Server:          srv,
		DailyScrapeTime: "08:00",
		Now: func() time.Time {
			return time.Date(2026, 7, 10, 7, 0, 0, 0, kstLocation())
		},
		Sleep: func(ctx context.Context, d time.Duration) error {
			sleepCalls++
			if sleepCalls == 1 {
				if d != time.Hour {
					t.Fatalf("sleep duration = %s, want 1h", d)
				}
				return nil
			}
			cancel()
			return ctx.Err()
		},
	})
	if err != nil {
		t.Fatalf("StartScheduler: %v", err)
	}

	waitForScheduler(t, schedulerCtx.Done(), func() bool {
		run, ok, err := st.LatestScrapeRun(context.Background())
		return err == nil && ok && run.Trigger == storage.ScrapeTriggerScheduled && run.Status == storage.ScrapeRunStatusSuccess
	})
}

func TestStartSchedulerRecordsSkippedRunWhenScrapeLockBusy(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	if !srv.flight.tryAcquire(scrapeAllKey) {
		t.Fatal("failed to arrange busy scrape lock")
	}
	defer srv.flight.release(scrapeAllKey)

	schedulerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sleepCalls := 0
	_, err := StartScheduler(schedulerCtx, SchedulerConfig{
		Server:          srv,
		DailyScrapeTime: "08:00",
		Now: func() time.Time {
			return time.Date(2026, 7, 10, 7, 0, 0, 0, kstLocation())
		},
		Sleep: func(ctx context.Context, d time.Duration) error {
			sleepCalls++
			if sleepCalls == 1 {
				return nil
			}
			cancel()
			return ctx.Err()
		},
	})
	if err != nil {
		t.Fatalf("StartScheduler: %v", err)
	}

	waitForScheduler(t, schedulerCtx.Done(), func() bool {
		run, ok, err := st.LatestScrapeRun(context.Background())
		return err == nil && ok &&
			run.Trigger == storage.ScrapeTriggerScheduled &&
			run.Status == storage.ScrapeRunStatusFailure &&
			strings.Contains(run.ErrorSummary, "skipped") &&
			strings.Contains(run.ErrorSummary, "scrape already running")
	})
}

func TestRecordSkippedScheduledRunFinishesAfterCallerContextCanceled(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	ctx, cancel := context.WithCancel(context.Background())

	srv.recordSkippedScheduledRunAfterStart(ctx, "skipped: scrape already running", cancel)

	run, ok, err := st.LatestScrapeRun(context.Background())
	if err != nil || !ok {
		t.Fatalf("LatestScrapeRun ok=%v err=%v", ok, err)
	}
	if run.Trigger != storage.ScrapeTriggerScheduled {
		t.Fatalf("Trigger = %q, want scheduled", run.Trigger)
	}
	if run.Status != storage.ScrapeRunStatusFailure {
		t.Fatalf("Status = %q, want failure", run.Status)
	}
	if run.FinishedAt == nil {
		t.Fatal("FinishedAt = nil, want skipped run finalized")
	}
	if run.ErrorSummary != "skipped: scrape already running" {
		t.Fatalf("ErrorSummary = %q, want skipped reason", run.ErrorSummary)
	}
}

func waitForScheduler(t *testing.T, done <-chan struct{}, ok func() bool) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if ok() {
			return
		}
		select {
		case <-done:
			if ok() {
				return
			}
			t.Fatal("scheduler stopped before expected condition")
		case <-deadline:
			t.Fatal("timed out waiting for scheduler")
		case <-ticker.C:
		}
	}
}
