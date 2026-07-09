-- 0001_initial.sql — PostgreSQL base schema for job-scraper.

CREATE TABLE postings (
    id                 BIGSERIAL PRIMARY KEY,
    source             TEXT NOT NULL,
    source_posting_id  TEXT NOT NULL,
    url                TEXT NOT NULL,
    title              TEXT NOT NULL,
    company            TEXT NOT NULL,
    location           TEXT,
    newcomer           BOOLEAN NOT NULL DEFAULT false,
    min_career         INTEGER NOT NULL DEFAULT 0,
    max_career         INTEGER NOT NULL DEFAULT 0,
    career_level       TEXT,
    education          INTEGER,
    education_name     TEXT,
    stack_tags_json    TEXT NOT NULL DEFAULT '[]',
    tags_json          TEXT NOT NULL DEFAULT '[]',
    description        TEXT NOT NULL DEFAULT '',
    raw_json           TEXT NOT NULL,
    published_at       TIMESTAMPTZ,
    closed_at          TIMESTAMPTZ,
    always_open        BOOLEAN NOT NULL DEFAULT false,
    first_seen_at      TIMESTAMPTZ NOT NULL,
    last_seen_at       TIMESTAMPTZ NOT NULL,
    duplicate_of       BIGINT REFERENCES postings(id) ON DELETE SET NULL,
    detail_fetched_at  TIMESTAMPTZ,
    UNIQUE (source, source_posting_id)
);

CREATE INDEX idx_postings_first_seen_at ON postings(first_seen_at DESC);
CREATE INDEX idx_postings_closed_at ON postings(closed_at);
CREATE INDEX idx_postings_duplicate_of ON postings(duplicate_of);
CREATE INDEX idx_postings_search ON postings
    USING GIN (to_tsvector('simple', coalesce(title, '') || ' ' || coalesce(company, '') || ' ' || coalesce(description, '')));

CREATE TABLE profile (
    id            INTEGER PRIMARY KEY CHECK (id = 1),
    profile_json  TEXT NOT NULL,
    profile_hash  TEXT NOT NULL,
    updated_at    TIMESTAMPTZ NOT NULL
);

CREATE TABLE scores (
    posting_id     BIGINT PRIMARY KEY REFERENCES postings(id) ON DELETE CASCADE,
    profile_hash   TEXT NOT NULL,
    total          INTEGER NOT NULL,
    breakdown_json TEXT NOT NULL,
    computed_at    TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_scores_total ON scores(total DESC);

CREATE TABLE ai_extractions (
    posting_id     BIGINT NOT NULL REFERENCES postings(id) ON DELETE CASCADE,
    content_hash   TEXT NOT NULL,
    ai_version     TEXT NOT NULL,
    min_career     INTEGER NOT NULL,
    max_career     INTEGER,
    newcomer       BOOLEAN NOT NULL,
    education_enum TEXT NOT NULL,
    evidence       TEXT NOT NULL,
    computed_at    TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (posting_id, content_hash, ai_version)
);

CREATE TABLE ai_scores (
    posting_id    BIGINT NOT NULL REFERENCES postings(id) ON DELETE CASCADE,
    ai_input_hash TEXT NOT NULL,
    ai_version    TEXT NOT NULL,
    items_json    TEXT NOT NULL,
    net_delta     INTEGER NOT NULL,
    computed_at   TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (posting_id, ai_input_hash, ai_version)
);

CREATE INDEX idx_ai_scores_latest ON ai_scores(posting_id, ai_version, computed_at DESC);

CREATE TABLE ai_usage (
    day           TEXT PRIMARY KEY,
    input_tokens  INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0
);
