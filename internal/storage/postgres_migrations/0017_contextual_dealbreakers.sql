ALTER TABLE ai_extractions
    RENAME COLUMN evidence TO career_evidence;

ALTER TABLE ai_extractions
    ADD COLUMN education_evidence TEXT NOT NULL DEFAULT '';

CREATE TABLE ai_dealbreaker_validations (
    user_id       BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    posting_id    BIGINT NOT NULL REFERENCES postings(id) ON DELETE CASCADE,
    content_hash  TEXT NOT NULL,
    ai_version    TEXT NOT NULL,
    keyword_hash  TEXT NOT NULL,
    verdict       TEXT NOT NULL CHECK (
        verdict IN ('applies', 'not_applicable', 'uncertain')
    ),
    evidence      TEXT NOT NULL,
    computed_at   TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (
        user_id,
        posting_id,
        content_hash,
        ai_version,
        keyword_hash
    )
);
