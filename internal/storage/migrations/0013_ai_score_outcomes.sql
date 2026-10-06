-- Add bounded Stage-2 provenance without rewriting legacy score rows.
CREATE TABLE ai_score_outcomes (
    user_id       INTEGER NOT NULL,
    posting_id    INTEGER NOT NULL REFERENCES postings(id) ON DELETE CASCADE,
    ai_input_hash TEXT NOT NULL,
    ai_version    TEXT NOT NULL,
    state         TEXT NOT NULL CHECK (state IN ('rated', 'no_signal', 'rejected', 'failed')),
    proposed      INTEGER NOT NULL CHECK (proposed >= 0),
    accepted      INTEGER NOT NULL CHECK (accepted >= 0 AND accepted <= proposed),
    computed_at   DATETIME NOT NULL,
    PRIMARY KEY (user_id, posting_id, ai_input_hash, ai_version)
);
