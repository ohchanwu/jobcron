-- 0009_purge_orphan_scores.sql — one-time cleanup of orphaned child rows.
--
-- The scores ON DELETE CASCADE (and the AI-cache / bookmark / mute cascades)
-- have been FK-enforced since the storage layer's first commit, so the current
-- code cannot create an orphan — TestDeletePostingCascadesScore guards exactly
-- that invariant. But a 2026-06-05 audit found 288 orphan rows in `scores`
-- referencing posting ids that no longer exist: historical residue from an
-- older binary or a crash inside the WAL window, before the invariant held.
--
-- The orphans are harmless at render time (every surface JOINs scores TO
-- postings, so an orphan is never read), but they inflate the table and break
-- row-count sanity checks (the audit saw 648 scores against 360 postings).
-- Purge them. The other posting-child tables were all 0 orphans in the audit;
-- we sweep them too, defensively and idempotently, so any install with its own
-- residue is cleaned the same way. NOT IN against a NOT-NULL PRIMARY KEY is
-- safe (no NULL-semantics surprise); an empty postings table correctly purges
-- all child rows.

DELETE FROM scores         WHERE posting_id NOT IN (SELECT id FROM postings);
DELETE FROM ai_extractions WHERE posting_id NOT IN (SELECT id FROM postings);
DELETE FROM ai_scores      WHERE posting_id NOT IN (SELECT id FROM postings);
DELETE FROM bookmarks      WHERE posting_id NOT IN (SELECT id FROM postings);
DELETE FROM not_interested WHERE posting_id NOT IN (SELECT id FROM postings);

-- postings.duplicate_of (0003) is a self-FK on the parent table (ON DELETE SET
-- NULL). The same residue mechanism could leave a row pointing at a now-deleted
-- id; the dashboard filters WHERE duplicate_of IS NULL, so such a row would be
-- hidden until the next dedup pass clears it. Null any dangling reference here
-- for completeness (the 2026-06-05 live-DB audit found 0). NULL-safe: NOT IN
-- against the NOT-NULL PRIMARY KEY id.
UPDATE postings SET duplicate_of = NULL
  WHERE duplicate_of IS NOT NULL AND duplicate_of NOT IN (SELECT id FROM postings);
