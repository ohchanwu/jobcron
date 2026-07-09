package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const (
	ScrapeTriggerManual    = "manual"
	ScrapeTriggerScheduled = "scheduled"

	ScrapeRunStatusRunning = "running"
	ScrapeRunStatusSuccess = "success"
	ScrapeRunStatusFailure = "failure"
)

// ScrapeResult summarizes one scrape run across every active source.
type ScrapeResult struct {
	Listed     int `json:"listed"`
	New        int `json:"new"`
	Refreshed  int `json:"refreshed"`
	Scored     int `json:"scored"`
	Removed    int `json:"removed"`
	Duplicates int `json:"duplicates"`
	Failed     int `json:"failed"`
}

// ScrapeRun is one durable scrape-run history row.
type ScrapeRun struct {
	ID           int64
	Trigger      string
	Status       string
	StartedAt    time.Time
	FinishedAt   *time.Time
	Result       ScrapeResult
	ErrorSummary string
}

func (s *Store) StartScrapeRun(ctx context.Context, trigger string) (ScrapeRun, error) {
	if trigger != ScrapeTriggerManual && trigger != ScrapeTriggerScheduled {
		return ScrapeRun{}, fmt.Errorf("storage: unsupported scrape trigger %q", trigger)
	}
	now := time.Now().UTC()
	if s.dialect == DialectPostgres {
		var id int64
		if err := s.db.QueryRowContext(ctx, `
INSERT INTO scrape_runs (trigger, status, started_at)
VALUES ($1, $2, $3)
RETURNING id`, trigger, ScrapeRunStatusRunning, now).Scan(&id); err != nil {
			return ScrapeRun{}, fmt.Errorf("storage: start scrape run: %w", err)
		}
		return ScrapeRun{
			ID:        id,
			Trigger:   trigger,
			Status:    ScrapeRunStatusRunning,
			StartedAt: now,
		}, nil
	}
	res, err := s.db.ExecContext(ctx, s.query(`
INSERT INTO scrape_runs (trigger, status, started_at)
VALUES (?, ?, ?)`), trigger, ScrapeRunStatusRunning, now)
	if err != nil {
		return ScrapeRun{}, fmt.Errorf("storage: start scrape run: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return ScrapeRun{}, fmt.Errorf("storage: start scrape run id: %w", err)
	}
	return ScrapeRun{
		ID:        id,
		Trigger:   trigger,
		Status:    ScrapeRunStatusRunning,
		StartedAt: now,
	}, nil
}

func (s *Store) FinishScrapeRun(ctx context.Context, id int64, result ScrapeResult, status string, errorSummary string) error {
	if status != ScrapeRunStatusSuccess && status != ScrapeRunStatusFailure {
		return fmt.Errorf("storage: unsupported scrape run status %q", status)
	}
	finishedAt := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, s.query(`
UPDATE scrape_runs
   SET status = ?,
       finished_at = ?,
       listed = ?,
       new_count = ?,
       refreshed = ?,
       scored = ?,
       removed = ?,
       duplicates = ?,
       failed = ?,
       error_summary = ?
 WHERE id = ?`),
		status, finishedAt, result.Listed, result.New, result.Refreshed, result.Scored,
		result.Removed, result.Duplicates, result.Failed, errorSummary, id)
	if err != nil {
		return fmt.Errorf("storage: finish scrape run: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) LatestScrapeRun(ctx context.Context) (ScrapeRun, bool, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, trigger, status, started_at, finished_at, listed, new_count, refreshed, scored, removed, duplicates, failed, error_summary
  FROM scrape_runs
 ORDER BY started_at DESC, id DESC
 LIMIT 1`)
	run, err := scanScrapeRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return ScrapeRun{}, false, nil
	}
	if err != nil {
		return ScrapeRun{}, false, fmt.Errorf("storage: latest scrape run: %w", err)
	}
	return run, true, nil
}

type scrapeRunScanner interface {
	Scan(dest ...any) error
}

func scanScrapeRun(row scrapeRunScanner) (ScrapeRun, error) {
	var run ScrapeRun
	var finishedAt sql.NullTime
	if err := row.Scan(
		&run.ID,
		&run.Trigger,
		&run.Status,
		&run.StartedAt,
		&finishedAt,
		&run.Result.Listed,
		&run.Result.New,
		&run.Result.Refreshed,
		&run.Result.Scored,
		&run.Result.Removed,
		&run.Result.Duplicates,
		&run.Result.Failed,
		&run.ErrorSummary,
	); err != nil {
		return ScrapeRun{}, err
	}
	if finishedAt.Valid {
		t := finishedAt.Time
		run.FinishedAt = &t
	}
	return run, nil
}
