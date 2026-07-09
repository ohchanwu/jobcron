-- 0006_user_scoped_state.sql — scope user-owned state to account rows.

INSERT INTO users (email, password_hash, created_at, updated_at)
SELECT 'sqlite-import-owner@job-scraper.local', 'imported-sqlite-no-login', now(), now()
WHERE NOT EXISTS (SELECT 1 FROM users)
  AND (
      EXISTS (SELECT 1 FROM profile)
   OR EXISTS (SELECT 1 FROM bookmarks)
   OR EXISTS (SELECT 1 FROM not_interested)
   OR EXISTS (SELECT 1 FROM scores)
  );

INSERT INTO profiles (user_id, profile_json, profile_hash, updated_at)
SELECT (SELECT id FROM users ORDER BY id LIMIT 1), profile.profile_json, profile.profile_hash, profile.updated_at
  FROM profile
 WHERE EXISTS (SELECT 1 FROM users)
ON CONFLICT (user_id) DO NOTHING;

CREATE TABLE bookmarks_user_scoped (
    user_id       BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    posting_id    BIGINT NOT NULL REFERENCES postings(id) ON DELETE CASCADE,
    bookmarked_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (user_id, posting_id)
);

INSERT INTO bookmarks_user_scoped (user_id, posting_id, bookmarked_at)
SELECT (SELECT id FROM users ORDER BY id LIMIT 1), bookmarks.posting_id, bookmarks.bookmarked_at
  FROM bookmarks
 WHERE EXISTS (SELECT 1 FROM users);

DROP TABLE bookmarks;
ALTER TABLE bookmarks_user_scoped RENAME TO bookmarks;
CREATE INDEX idx_bookmarks_user_bookmarked_at ON bookmarks(user_id, bookmarked_at DESC);

CREATE TABLE not_interested_user_scoped (
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    posting_id BIGINT NOT NULL REFERENCES postings(id) ON DELETE CASCADE,
    muted_at   TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (user_id, posting_id)
);

INSERT INTO not_interested_user_scoped (user_id, posting_id, muted_at)
SELECT (SELECT id FROM users ORDER BY id LIMIT 1), not_interested.posting_id, not_interested.muted_at
  FROM not_interested
 WHERE EXISTS (SELECT 1 FROM users);

DROP TABLE not_interested;
ALTER TABLE not_interested_user_scoped RENAME TO not_interested;
CREATE INDEX idx_not_interested_user_muted_at ON not_interested(user_id, muted_at DESC);

CREATE TABLE scores_user_scoped (
    user_id        BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    posting_id     BIGINT NOT NULL REFERENCES postings(id) ON DELETE CASCADE,
    profile_hash   TEXT NOT NULL,
    total          INTEGER NOT NULL,
    breakdown_json TEXT NOT NULL,
    computed_at    TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (user_id, posting_id)
);

INSERT INTO scores_user_scoped (user_id, posting_id, profile_hash, total, breakdown_json, computed_at)
SELECT (SELECT id FROM users ORDER BY id LIMIT 1), scores.posting_id, scores.profile_hash, scores.total, scores.breakdown_json, scores.computed_at
  FROM scores
 WHERE EXISTS (SELECT 1 FROM users);

DROP TABLE scores;
ALTER TABLE scores_user_scoped RENAME TO scores;
CREATE INDEX idx_scores_user_total ON scores(user_id, total DESC);
