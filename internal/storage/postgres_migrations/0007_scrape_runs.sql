-- 0007_scrape_runs.sql — convert the legacy production scrape_runs table into
-- the new history shape used by Task 9.

CREATE TABLE scrape_runs_new (
    id            BIGSERIAL PRIMARY KEY,
    trigger       TEXT NOT NULL,
    status        TEXT NOT NULL,
    started_at    TIMESTAMPTZ NOT NULL,
    finished_at   TIMESTAMPTZ,
    listed        INTEGER NOT NULL DEFAULT 0,
    new_count     INTEGER NOT NULL DEFAULT 0,
    refreshed     INTEGER NOT NULL DEFAULT 0,
    scored        INTEGER NOT NULL DEFAULT 0,
    removed       INTEGER NOT NULL DEFAULT 0,
    duplicates    INTEGER NOT NULL DEFAULT 0,
    failed        INTEGER NOT NULL DEFAULT 0,
    error_summary TEXT NOT NULL DEFAULT ''
);

INSERT INTO scrape_runs_new (
    id, "trigger", status, started_at, finished_at,
    listed, new_count, refreshed, scored, removed, duplicates, failed, error_summary
)
SELECT
    id,
    'manual',
    status,
    started_at,
    finished_at,
    0,
    COALESCE(new_posting_count, 0),
    0,
    0,
    0,
    0,
    0,
    error_summary
  FROM scrape_runs;

DROP TABLE scrape_runs;
ALTER TABLE scrape_runs_new RENAME TO scrape_runs;

CREATE INDEX idx_scrape_runs_started_at ON scrape_runs(started_at DESC);
