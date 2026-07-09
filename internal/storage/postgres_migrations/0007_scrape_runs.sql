-- 0007_scrape_runs.sql — durable history for manual and scheduled scrape runs.

CREATE TABLE scrape_runs (
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

CREATE INDEX idx_scrape_runs_started_at ON scrape_runs(started_at DESC);
