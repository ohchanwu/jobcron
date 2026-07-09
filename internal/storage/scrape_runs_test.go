package storage

import (
	"context"
	"testing"
	"time"
)

func TestScrapeRunsStartFinishAndLatest(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	first, err := st.StartScrapeRun(ctx, ScrapeTriggerScheduled)
	if err != nil {
		t.Fatalf("StartScrapeRun first: %v", err)
	}
	if first.ID == 0 {
		t.Fatal("first run ID = 0, want database-assigned ID")
	}
	if first.Trigger != ScrapeTriggerScheduled {
		t.Fatalf("first trigger = %q, want %q", first.Trigger, ScrapeTriggerScheduled)
	}

	run, err := st.StartScrapeRun(ctx, ScrapeTriggerManual)
	if err != nil {
		t.Fatalf("StartScrapeRun: %v", err)
	}
	if run.ID <= first.ID {
		t.Fatalf("run ID = %d, want greater than first ID %d", run.ID, first.ID)
	}
	if run.Trigger != ScrapeTriggerManual {
		t.Errorf("Trigger = %q, want %q", run.Trigger, ScrapeTriggerManual)
	}
	if run.Status != ScrapeRunStatusRunning {
		t.Errorf("Status = %q, want %q", run.Status, ScrapeRunStatusRunning)
	}
	if run.StartedAt.IsZero() {
		t.Error("StartedAt is zero")
	}
	if run.FinishedAt != nil {
		t.Fatalf("FinishedAt = %v, want nil for a running scrape", run.FinishedAt)
	}

	result := ScrapeResult{
		Listed:     11,
		New:        3,
		Refreshed:  2,
		Scored:     9,
		Removed:    1,
		Duplicates: 4,
		Failed:     5,
	}
	if err := st.FinishScrapeRun(ctx, run.ID, result, ScrapeRunStatusFailure, "jumpit timeout"); err != nil {
		t.Fatalf("FinishScrapeRun: %v", err)
	}

	latest, ok, err := st.LatestScrapeRun(ctx)
	if err != nil {
		t.Fatalf("LatestScrapeRun: %v", err)
	}
	if !ok {
		t.Fatal("LatestScrapeRun ok = false, want true")
	}
	if latest.ID != run.ID {
		t.Fatalf("LatestScrapeRun ID = %d, want %d", latest.ID, run.ID)
	}
	if latest.Trigger != ScrapeTriggerManual {
		t.Errorf("latest Trigger = %q, want %q", latest.Trigger, ScrapeTriggerManual)
	}
	if latest.Status != ScrapeRunStatusFailure {
		t.Errorf("latest Status = %q, want %q", latest.Status, ScrapeRunStatusFailure)
	}
	if latest.FinishedAt == nil {
		t.Fatal("FinishedAt = nil, want finish time")
	}
	if latest.FinishedAt.Before(latest.StartedAt) {
		t.Fatalf("FinishedAt %s is before StartedAt %s", latest.FinishedAt.Format(time.RFC3339Nano), latest.StartedAt.Format(time.RFC3339Nano))
	}
	if latest.Result != result {
		t.Errorf("latest Result = %+v, want %+v", latest.Result, result)
	}
	if latest.ErrorSummary != "jumpit timeout" {
		t.Errorf("ErrorSummary = %q, want jumpit timeout", latest.ErrorSummary)
	}
}

func TestLatestScrapeRunEmpty(t *testing.T) {
	st := newTestStore(t)

	latest, ok, err := st.LatestScrapeRun(context.Background())
	if err != nil {
		t.Fatalf("LatestScrapeRun: %v", err)
	}
	if ok {
		t.Fatalf("LatestScrapeRun ok = true with row %+v, want false", latest)
	}
}
