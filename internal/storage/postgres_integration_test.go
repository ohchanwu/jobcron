package storage

import (
	"context"
	"testing"
)

func TestPostgresMigrationsCreateCoreTables(t *testing.T) {
	st, schema := newPostgresTestStoreWithSchema(t)

	for _, table := range []string{
		"users",
		"sessions",
		"postings",
		"profiles",
		"bookmarks",
		"not_interested",
		"scores",
		"scrape_runs",
	} {
		var exists bool
		err := st.db.QueryRowContext(context.Background(), `SELECT EXISTS (
			SELECT 1
			  FROM information_schema.tables
			 WHERE table_schema = $1
			   AND table_name = $2
		)`, schema, table).Scan(&exists)
		if err != nil || !exists {
			t.Fatalf("table %s exists=%v err=%v", table, exists, err)
		}
	}
}

func TestPostgresScrapeRunsStartFinishAndLatestRuntime(t *testing.T) {
	st, _ := newPostgresTestStoreWithSchema(t)
	ctx := context.Background()

	first, err := st.StartScrapeRun(ctx, ScrapeTriggerManual)
	if err != nil {
		t.Fatalf("StartScrapeRun first: %v", err)
	}
	if first.Trigger != ScrapeTriggerManual {
		t.Fatalf("first Trigger = %q, want %q", first.Trigger, ScrapeTriggerManual)
	}
	if first.Status != ScrapeRunStatusRunning {
		t.Fatalf("first Status = %q, want %q", first.Status, ScrapeRunStatusRunning)
	}
	if first.FinishedAt != nil {
		t.Fatalf("first FinishedAt = %v, want nil while running", first.FinishedAt)
	}

	firstResult := ScrapeResult{
		Listed:     5,
		New:        2,
		Refreshed:  1,
		Scored:     4,
		Removed:    1,
		Duplicates: 1,
		Failed:     0,
	}
	if err := st.FinishScrapeRun(ctx, first.ID, firstResult, ScrapeRunStatusSuccess, ""); err != nil {
		t.Fatalf("FinishScrapeRun first: %v", err)
	}
	latest, ok, err := st.LatestScrapeRun(ctx)
	if err != nil || !ok {
		t.Fatalf("LatestScrapeRun after first ok=%v err=%v", ok, err)
	}
	if latest.ID != first.ID {
		t.Fatalf("latest ID = %d, want first ID %d", latest.ID, first.ID)
	}
	if latest.Status != ScrapeRunStatusSuccess {
		t.Fatalf("latest Status = %q, want %q", latest.Status, ScrapeRunStatusSuccess)
	}
	if latest.Result != firstResult {
		t.Fatalf("latest Result = %+v, want %+v", latest.Result, firstResult)
	}
	if latest.ErrorSummary != "" {
		t.Fatalf("latest ErrorSummary = %q, want empty", latest.ErrorSummary)
	}
	if latest.FinishedAt == nil {
		t.Fatal("latest FinishedAt = nil, want success finish time")
	}

	second, err := st.StartScrapeRun(ctx, ScrapeTriggerScheduled)
	if err != nil {
		t.Fatalf("StartScrapeRun second: %v", err)
	}
	if second.ID <= first.ID {
		t.Fatalf("second ID = %d, want greater than first ID %d", second.ID, first.ID)
	}
	secondResult := ScrapeResult{
		Listed:     7,
		New:        3,
		Refreshed:  2,
		Scored:     6,
		Removed:    2,
		Duplicates: 0,
		Failed:     1,
	}
	if err := st.FinishScrapeRun(ctx, second.ID, secondResult, ScrapeRunStatusFailure, "jumpit timeout"); err != nil {
		t.Fatalf("FinishScrapeRun second: %v", err)
	}
	latest, ok, err = st.LatestScrapeRun(ctx)
	if err != nil || !ok {
		t.Fatalf("LatestScrapeRun after second ok=%v err=%v", ok, err)
	}
	if latest.ID != second.ID {
		t.Fatalf("latest ID = %d, want second ID %d", latest.ID, second.ID)
	}
	if latest.Trigger != ScrapeTriggerScheduled {
		t.Fatalf("latest Trigger = %q, want %q", latest.Trigger, ScrapeTriggerScheduled)
	}
	if latest.Status != ScrapeRunStatusFailure {
		t.Fatalf("latest Status = %q, want %q", latest.Status, ScrapeRunStatusFailure)
	}
	if latest.Result != secondResult {
		t.Fatalf("latest Result = %+v, want %+v", latest.Result, secondResult)
	}
	if latest.ErrorSummary != "jumpit timeout" {
		t.Fatalf("latest ErrorSummary = %q, want jumpit timeout", latest.ErrorSummary)
	}
	if latest.FinishedAt == nil {
		t.Fatal("latest FinishedAt = nil, want failure finish time")
	}
}
