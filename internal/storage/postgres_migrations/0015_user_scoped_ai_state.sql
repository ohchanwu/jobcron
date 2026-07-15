-- 0015_user_scoped_ai_state.sql — scope preference-derived AI state to users.

DO $$
DECLARE
    user_count  BIGINT;
    score_count BIGINT;
    usage_count BIGINT;
BEGIN
    SELECT count(*) INTO user_count FROM users;
    SELECT count(*) INTO score_count FROM ai_scores;
    SELECT count(*) INTO usage_count FROM ai_usage;

    IF (score_count > 0 OR usage_count > 0) AND user_count <> 1 THEN
        RAISE EXCEPTION
            'migration 0015 cannot assign legacy AI state: found % users, % score rows, and % usage rows; create exactly one owner or empty the AI tables before retrying',
            user_count, score_count, usage_count;
    END IF;
END $$;

CREATE TABLE ai_scores_user_scoped (
    user_id       BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    posting_id    BIGINT NOT NULL REFERENCES postings(id) ON DELETE CASCADE,
    ai_input_hash TEXT NOT NULL,
    ai_version    TEXT NOT NULL,
    items_json    TEXT NOT NULL,
    net_delta     INTEGER NOT NULL,
    computed_at   TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (user_id, posting_id, ai_input_hash, ai_version)
);

WITH sole_owner AS (
    SELECT min(id) AS user_id
      FROM users
    HAVING count(*) = 1
)
INSERT INTO ai_scores_user_scoped (
    user_id,
    posting_id,
    ai_input_hash,
    ai_version,
    items_json,
    net_delta,
    computed_at
)
SELECT sole_owner.user_id,
       ai_scores.posting_id,
       ai_scores.ai_input_hash,
       ai_scores.ai_version,
       ai_scores.items_json,
       ai_scores.net_delta,
       ai_scores.computed_at
  FROM ai_scores
 CROSS JOIN sole_owner;

CREATE TABLE ai_usage_user_scoped (
    user_id       BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    day           DATE NOT NULL,
    input_tokens  BIGINT NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
    output_tokens BIGINT NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
    PRIMARY KEY (user_id, day)
);

WITH sole_owner AS (
    SELECT min(id) AS user_id
      FROM users
    HAVING count(*) = 1
)
INSERT INTO ai_usage_user_scoped (user_id, day, input_tokens, output_tokens)
SELECT sole_owner.user_id,
       ai_usage.day::date,
       ai_usage.input_tokens,
       ai_usage.output_tokens
  FROM ai_usage
 CROSS JOIN sole_owner;

DO $$
DECLARE
    source_score_count      BIGINT;
    destination_score_count BIGINT;
    source_usage_count      BIGINT;
    destination_usage_count BIGINT;
BEGIN
    SELECT count(*) INTO source_score_count FROM ai_scores;
    SELECT count(*) INTO destination_score_count FROM ai_scores_user_scoped;
    SELECT count(*) INTO source_usage_count FROM ai_usage;
    SELECT count(*) INTO destination_usage_count FROM ai_usage_user_scoped;

    IF source_score_count <> destination_score_count THEN
        RAISE EXCEPTION
            'migration 0015 AI score copy mismatch: source %, destination %',
            source_score_count, destination_score_count;
    END IF;
    IF source_usage_count <> destination_usage_count THEN
        RAISE EXCEPTION
            'migration 0015 AI usage copy mismatch: source %, destination %',
            source_usage_count, destination_usage_count;
    END IF;
END $$;

DROP TABLE ai_scores;
ALTER TABLE ai_scores_user_scoped RENAME TO ai_scores;
CREATE INDEX idx_ai_scores_user_latest
    ON ai_scores(user_id, posting_id, ai_version, computed_at DESC);

DROP TABLE ai_usage;
ALTER TABLE ai_usage_user_scoped RENAME TO ai_usage;
