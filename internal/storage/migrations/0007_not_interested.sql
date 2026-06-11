-- 0007_not_interested.sql — user-muted postings ("관심 없음").
--
-- A manual mute is the inverse of a bookmark: a soft, user-controlled signal
-- that a posting should disappear from the briefing and the 전체 공고 list
-- entirely (not merely sink into the "관심 밖" collapsible the way a
-- below-MinScore score does). Like a bookmark it lives outside the
-- daily-briefing window and the profile_hash / scores cycle, and follows the
-- posting it references (ON DELETE CASCADE — if the posting row is swept, its
-- mute goes with it). A muted posting that is also bookmarked stays visible on
-- /bookmarks: the mute only hides it from the discovery surfaces.

CREATE TABLE not_interested (
    posting_id  INTEGER PRIMARY KEY REFERENCES postings(id) ON DELETE CASCADE,
    muted_at    DATETIME NOT NULL
);

CREATE INDEX idx_not_interested_muted_at ON not_interested(muted_at DESC);
