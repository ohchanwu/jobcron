-- 0002_bookmarks.sql — user-saved postings.
--
-- A bookmark is a soft, user-controlled signal that exists outside the
-- daily-briefing window: bookmarks survive across days, are unaffected by
-- the profile_hash / scores cycle, and follow the posting they reference
-- (ON DELETE CASCADE — if the posting row is ever removed, its bookmark
-- goes with it).

CREATE TABLE bookmarks (
    posting_id     INTEGER PRIMARY KEY REFERENCES postings(id) ON DELETE CASCADE,
    bookmarked_at  DATETIME NOT NULL
);

CREATE INDEX idx_bookmarks_bookmarked_at ON bookmarks(bookmarked_at DESC);
