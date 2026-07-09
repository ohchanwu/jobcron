-- 0002_user_state.sql — user-controlled saved and muted posting state.

CREATE TABLE bookmarks (
    posting_id     BIGINT PRIMARY KEY REFERENCES postings(id) ON DELETE CASCADE,
    bookmarked_at  TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_bookmarks_bookmarked_at ON bookmarks(bookmarked_at DESC);

CREATE TABLE not_interested (
    posting_id  BIGINT PRIMARY KEY REFERENCES postings(id) ON DELETE CASCADE,
    muted_at    TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_not_interested_muted_at ON not_interested(muted_at DESC);
