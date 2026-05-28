-- 0004_profile_weights.sql — backfill per-category weight defaults.
--
-- Profile.CareerWeight and Profile.SalaryWeight were added so the user
-- can tune how much career-fit and salary-fit each contribute to a
-- posting's score. They replace the formerly-fixed careerExact=25 and
-- salaryClear=10 constants in internal/scoring/rules.go.
--
-- Existing profile rows store JSON without these fields. The Go-side
-- helpers Profile.EffectiveCareerWeight / EffectiveSalaryWeight already
-- fall back to the defaults (25 / 10) when a field is missing or zero,
-- so old DBs score correctly without ANY persisted change. This
-- migration is therefore a no-op at the SQL layer — it exists to bump
-- PRAGMA user_version, signalling to the maintainer that the JSON
-- schema has evolved.
--
-- The DB-level change is "none". JSON evolution is handled in Go.

-- A harmless statement so the migration runner has something to run.
-- PRAGMA user_version is bumped automatically by the migrate driver.
SELECT 1;
