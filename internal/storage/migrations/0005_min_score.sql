-- 0005_min_score.sql — backfill the profile.min_score field.
--
-- Profile.MinScore was added so the user can hide low-scoring postings
-- from the main "Today" list. Default 40 (= DefaultMinScore in
-- internal/profile/profile.go). The Go-side EffectiveMinScore helper
-- already returns 40 when the field is missing from the loaded JSON,
-- so the scoring engine stays correct without this migration. But
-- backfilling makes the persisted JSON match what the form would save
-- on first edit — the database state is self-describing instead of
-- relying on a Go-side fallback a future reader might miss.
--
-- The profile_hash will be stale on the backfilled row until the user
-- next saves their profile (at which point SaveProfile detects the
-- mismatch, writes the new hash, and re-scores). That's fine: the
-- staleness is bounded and the next save re-syncs.

UPDATE profile
   SET profile_json = json_set(profile_json, '$.min_score', 40)
 WHERE id = 1
   AND json_extract(profile_json, '$.min_score') IS NULL;
