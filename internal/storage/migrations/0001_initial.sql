-- 0001_initial.sql — initial schema for job-scraper.

CREATE TABLE postings (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    source             TEXT NOT NULL,                    -- "jumpit"
    source_posting_id  TEXT NOT NULL,                    -- 점핏's position ID (stringified int)
    url                TEXT NOT NULL,
    title              TEXT NOT NULL,
    company            TEXT NOT NULL,
    location           TEXT,
    newcomer           INTEGER NOT NULL DEFAULT 0,       -- 점핏 `newcomer` flag (0/1)
    min_career         INTEGER NOT NULL DEFAULT 0,       -- raw `minCareer` (years)
    max_career         INTEGER NOT NULL DEFAULT 0,       -- raw `maxCareer` (years)
    career_level       TEXT,                             -- derived: "신입" | "1-3년" | etc.
    education          INTEGER,                          -- 점핏 `education` code
    education_name     TEXT,
    stack_tags_json    TEXT NOT NULL DEFAULT '[]',       -- JSON array of strings
    tags_json          TEXT NOT NULL DEFAULT '[]',       -- JSON array of {id,name,category}
    description        TEXT NOT NULL DEFAULT '',         -- composed JD text, for FTS matching
    raw_json           TEXT NOT NULL,                    -- full upstream payload (forward compat)
    published_at       DATETIME,
    closed_at          DATETIME,                         -- NULL when always_open=1
    always_open        INTEGER NOT NULL DEFAULT 0,
    first_seen_at      DATETIME NOT NULL,
    last_seen_at       DATETIME NOT NULL,
    UNIQUE (source, source_posting_id)
);

CREATE INDEX idx_postings_first_seen_at ON postings(first_seen_at DESC);
CREATE INDEX idx_postings_closed_at     ON postings(closed_at);

-- FTS5 over title + company + description ONLY. raw_json is NOT indexed.
-- unicode61 remove_diacritics 0 tokenizes Korean by codepoint runs separated
-- by whitespace/punctuation (verified by the Step 0 spike).
CREATE VIRTUAL TABLE postings_fts USING fts5(
    title, company, description,
    content='postings', content_rowid='id',
    tokenize='unicode61 remove_diacritics 0'
);

CREATE TRIGGER postings_ai AFTER INSERT ON postings BEGIN
    INSERT INTO postings_fts(rowid, title, company, description)
      VALUES (new.id, new.title, new.company, new.description);
END;
CREATE TRIGGER postings_ad AFTER DELETE ON postings BEGIN
    INSERT INTO postings_fts(postings_fts, rowid, title, company, description)
      VALUES('delete', old.id, old.title, old.company, old.description);
END;
CREATE TRIGGER postings_au AFTER UPDATE ON postings BEGIN
    INSERT INTO postings_fts(postings_fts, rowid, title, company, description)
      VALUES('delete', old.id, old.title, old.company, old.description);
    INSERT INTO postings_fts(rowid, title, company, description)
      VALUES (new.id, new.title, new.company, new.description);
END;

CREATE TABLE profile (
    id            INTEGER PRIMARY KEY CHECK (id = 1),    -- single-row table
    profile_json  TEXT NOT NULL,                          -- canonical JSON
    profile_hash  TEXT NOT NULL,                          -- sha256(canonical_json)[:12]
    updated_at    DATETIME NOT NULL
);

CREATE TABLE scores (
    posting_id     INTEGER PRIMARY KEY REFERENCES postings(id) ON DELETE CASCADE,
    profile_hash   TEXT NOT NULL,
    total          INTEGER NOT NULL,                      -- -1 dealbreaker, else 0..100
    breakdown_json TEXT NOT NULL,
    computed_at    DATETIME NOT NULL
);

CREATE INDEX idx_scores_total ON scores(total DESC);
