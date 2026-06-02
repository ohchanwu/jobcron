-- BYOK AI v2.0 schema. One migration creates all three AI tables; Stage 1
-- (v2.0.0-alpha) only writes ai_extractions, but the schema is locked as a
-- whole so the runner bumps user_version 7 -> 8 exactly once. (0006 was
-- intentionally skipped; the runner keys on the filename prefix, not contiguity.)

-- Stage-1 extraction cache. CASCADE so the staleness sweep cleans it with the posting.
CREATE TABLE ai_extractions (
    posting_id     INTEGER NOT NULL REFERENCES postings(id) ON DELETE CASCADE,
    content_hash   TEXT NOT NULL,   -- sha256(pre-truncation normalized model text)[:12]
    ai_version     TEXT NOT NULL,   -- hash(provider + model + prompt_template_version)
    min_career     INTEGER NOT NULL,
    max_career     INTEGER,         -- NULL = open upper bound -> experienceUpperOpen(99) at read
    newcomer       INTEGER NOT NULL,
    education_enum TEXT NOT NULL,   -- raw AI enum none..doctorate; ordinal derived at read
    evidence       TEXT NOT NULL,
    computed_at    DATETIME NOT NULL,
    PRIMARY KEY (posting_id, content_hash, ai_version)
);

-- Stage-2 delta cache. Keyed on the AI-INPUT hash (not full profile_hash).
CREATE TABLE ai_scores (
    posting_id    INTEGER NOT NULL REFERENCES postings(id) ON DELETE CASCADE,
    ai_input_hash TEXT NOT NULL,    -- sha256(buildStage2ProfileText(profile))[:12]
    ai_version    TEXT NOT NULL,
    items_json    TEXT NOT NULL,    -- surviving {signal,kind,delta,evidence,matched_goal}
    net_delta     INTEGER NOT NULL,
    computed_at   DATETIME NOT NULL,
    PRIMARY KEY (posting_id, ai_input_hash, ai_version)
);
CREATE INDEX idx_ai_scores_latest ON ai_scores(posting_id, ai_version, computed_at DESC);

-- Rolling daily token ledger. One row per UTC day.
CREATE TABLE ai_usage (
    day           TEXT PRIMARY KEY,           -- "2026-06-02" (UTC)
    input_tokens  INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0
);
