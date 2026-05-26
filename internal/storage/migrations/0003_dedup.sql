-- 0003_dedup.sql — cross-portal duplicate marker.
--
-- When the same role appears on two source portals (e.g. the same posting
-- on 점핏 and 랠릿), the second-seen row is marked duplicate_of the
-- first-seen row. The dedup pass in server.runScrape sets this column
-- after sweep and before re-scoring; the dashboard then filters out
-- non-canonical rows and renders an "also on …" badge on the canonical.
--
-- Why the column lives on postings rather than in a join table: there is
-- exactly one canonical for each duplicate, and the canonical itself
-- carries duplicate_of = NULL. This keeps the dashboard query a simple
-- WHERE filter and lets ON DELETE SET NULL handle the sweep case
-- automatically — if the canonical is swept out, its former duplicates
-- become canonical themselves and re-enter the list rather than
-- disappearing alongside the deleted row.

ALTER TABLE postings ADD COLUMN duplicate_of INTEGER
    REFERENCES postings(id) ON DELETE SET NULL;

-- Index supports two queries: "which postings point at this canonical?"
-- (for the badge) and "which postings are canonical?" (the dashboard's
-- `WHERE duplicate_of IS NULL` filter — partial indexes on NULL get used
-- by SQLite when the query expresses the same predicate).
CREATE INDEX idx_postings_duplicate_of ON postings(duplicate_of);
