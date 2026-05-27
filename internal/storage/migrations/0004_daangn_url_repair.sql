-- One-time URL repair for 당근 postings.
--
-- Before 2026-05-27, the daangn scraper stored Greenhouse's
-- `absolute_url` field as the click target: `about.daangn.com?gh_jid=…`.
-- That URL turned out to land on the daangn marketing home for every
-- job, not the actual posting page. The fix in the scraper was to
-- build the URL from team.daangn.com/jobs/{id}/ instead.
--
-- UpsertPosting only updates last_seen_at on already-seen rows (it
-- does not refresh URL), so without this migration, existing daangn
-- rows in a user's database would keep the broken click target until
-- the natural sweep deletes them. This migration converts in place.
UPDATE postings
   SET url = 'https://team.daangn.com/jobs/' || source_posting_id || '/'
 WHERE source = 'daangn'
   AND url LIKE 'https://about.daangn.com%';
