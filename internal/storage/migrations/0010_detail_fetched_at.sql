-- 0010_detail_fetched_at.sql — track when each posting's DETAIL was last fetched.
--
-- The scrape fetches a posting's full detail (Description, raw_json, career
-- fields) only when it is NEW; an already-seen posting just bumps last_seen_at.
-- So an employer's later edit to a job description never reaches us — the cached
-- Stage-1 extraction (keyed on content_hash) and the score stay frozen at first
-- fetch (T7).
--
-- This column records the wall-clock time of the most recent detail fetch per
-- posting, so the scrape can re-fetch the N postings with the STALEST detail
-- each run (a bounded "edited-JD" refresh) instead of re-fetching everything
-- (cost + politeness) or nothing (the current staleness). It is set to the
-- fetch time on insert and on every detail refresh.
--
-- Backfill existing rows to first_seen_at: their detail WAS fetched then (a new
-- posting fetches detail at first sight), so this is the truthful baseline and
-- it puts the oldest postings first in the refresh queue.

ALTER TABLE postings ADD COLUMN detail_fetched_at DATETIME;

UPDATE postings SET detail_fetched_at = first_seen_at WHERE detail_fetched_at IS NULL;
