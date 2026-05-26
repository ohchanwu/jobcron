package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ohchanwu/job-scraper/internal/scraper"
)

// postingColumns is the column list shared by the insert and select queries,
// in a fixed order so the parameter list and the scan list stay in lockstep.
const postingColumns = `source, source_posting_id, url, title, company, location,
	newcomer, min_career, max_career, career_level, education, education_name,
	stack_tags_json, tags_json, description, raw_json, published_at, closed_at,
	always_open, first_seen_at, last_seen_at`

// UpsertPosting inserts p as a new posting or, when a posting with the same
// (Source, SourcePostingID) already exists, refreshes its last_seen_at. It
// reports the row id and whether the posting was newly inserted.
//
// On the already-seen path only last_seen_at advances: per the design's
// scrape flow, postings already in the DB are not re-fetched, so no other
// field has fresh data to write.
func (s *Store) UpsertPosting(ctx context.Context, p scraper.Posting) (id int64, isNew bool, err error) {
	var existingID int64
	err = s.db.QueryRowContext(ctx,
		`SELECT id FROM postings WHERE source = ? AND source_posting_id = ?`,
		p.Source, p.SourcePostingID).Scan(&existingID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		// New posting — fall through to INSERT.
	case err != nil:
		return 0, false, fmt.Errorf("storage: look up posting: %w", err)
	default:
		if _, err := s.db.ExecContext(ctx,
			`UPDATE postings SET last_seen_at = ? WHERE id = ?`,
			p.LastSeenAt.UTC(), existingID); err != nil {
			return 0, false, fmt.Errorf("storage: update last_seen_at: %w", err)
		}
		return existingID, false, nil
	}

	stackJSON, err := json.Marshal(nonNilSlice(p.StackTags))
	if err != nil {
		return 0, false, fmt.Errorf("storage: marshal stack tags: %w", err)
	}
	tagsJSON, err := json.Marshal(nonNilTags(p.Tags))
	if err != nil {
		return 0, false, fmt.Errorf("storage: marshal tags: %w", err)
	}
	res, err := s.db.ExecContext(ctx, `
INSERT INTO postings (`+postingColumns+`)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		p.Source, p.SourcePostingID, p.URL, p.Title, p.Company, p.Location,
		p.Newcomer, p.MinCareer, p.MaxCareer, p.CareerLevel, p.Education, p.EducationName,
		string(stackJSON), string(tagsJSON), p.Description, p.RawJSON, utcPtr(p.PublishedAt), utcPtr(p.ClosedAt),
		p.AlwaysOpen, p.FirstSeenAt.UTC(), p.LastSeenAt.UTC())
	if err != nil {
		return 0, false, fmt.Errorf("storage: insert posting: %w", err)
	}
	id, err = res.LastInsertId()
	if err != nil {
		return 0, false, fmt.Errorf("storage: insert posting id: %w", err)
	}
	return id, true, nil
}

// selectColumns is the column list used by SELECT queries — postingColumns
// plus the fields that scanPosting reads beyond what UpsertPosting writes
// (duplicate_of is set later by the server's dedup pass, never on insert).
const selectColumns = `id, ` + postingColumns + `, duplicate_of`

// PostingByID returns the posting with the given row id, or ok=false if none.
func (s *Store) PostingByID(ctx context.Context, id int64) (scraper.Posting, bool, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+selectColumns+` FROM postings WHERE id = ?`, id)
	p, err := scanPosting(row)
	if errors.Is(err, sql.ErrNoRows) {
		return scraper.Posting{}, false, nil
	}
	if err != nil {
		return scraper.Posting{}, false, err
	}
	return p, true, nil
}

// KnownSourceIDs returns the set of source_posting_id values already stored
// for the given source — used to tell new postings from already-seen ones.
func (s *Store) KnownSourceIDs(ctx context.Context, source string) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT source_posting_id FROM postings WHERE source = ?`, source)
	if err != nil {
		return nil, fmt.Errorf("storage: query known ids: %w", err)
	}
	defer rows.Close()
	ids := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("storage: scan known id: %w", err)
		}
		ids[id] = true
	}
	return ids, rows.Err()
}

// SweepStalePostings removes postings that have probably gone stale.
//
// A posting is removed when ALL of the following hold:
//   - It is not bookmarked. Bookmarks are user-explicit save signals;
//     they never auto-remove.
//   - It satisfies at least one of:
//   - Stale-from-source: last_seen_at < (max last_seen_at − staleWindow)
//     measured *within that posting's source*. Per-source rather than
//     global so scraping one source heavily does not prematurely stale
//     out postings from a source scraped less often.
//   - Old-and-not-always-open: first_seen_at < (now − oldWindow) AND
//     always_open = 0. The source's own "no expiration" flag wins
//     against the age rule.
//
// activeSources scopes the sweep to sources the user currently has enabled —
// a disabled source's data is frozen in place so re-enabling it does not
// require a fresh scrape. Pass nil to sweep every source present in the DB.
//
// Returns the number of rows removed across all swept sources. ON DELETE
// CASCADE on scores and bookmarks (the latter cannot match here by
// construction) handles the dependent rows; the FTS triggers keep the index
// in sync.
func (s *Store) SweepStalePostings(
	ctx context.Context, now time.Time, staleWindow, oldWindow time.Duration,
	activeSources []string,
) (int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT source, MAX(last_seen_at) FROM postings GROUP BY source`)
	if err != nil {
		return 0, fmt.Errorf("storage: read per-source max last_seen_at: %w", err)
	}
	defer rows.Close()

	type sourceBaseline struct {
		source      string
		maxLastSeen time.Time
	}
	var baselines []sourceBaseline
	for rows.Next() {
		var src string
		// modernc.org/sqlite drops the DATETIME column tag on MAX(), so
		// sql.NullTime cannot scan the result — read as a string and parse.
		var maxRaw sql.NullString
		if err := rows.Scan(&src, &maxRaw); err != nil {
			return 0, fmt.Errorf("storage: scan source baseline: %w", err)
		}
		if !maxRaw.Valid {
			continue
		}
		t, err := time.Parse(timeStoreFormat, maxRaw.String)
		if err != nil {
			return 0, fmt.Errorf("storage: parse max last_seen_at %q: %w", maxRaw.String, err)
		}
		baselines = append(baselines, sourceBaseline{source: src, maxLastSeen: t.UTC()})
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("storage: iterate source baselines: %w", err)
	}

	active := sourceSet(activeSources)
	oldBefore := now.UTC().Add(-oldWindow)
	var total int
	for _, b := range baselines {
		if active != nil && !active[b.source] {
			continue // disabled source: freeze the data, do not sweep
		}
		staleBefore := b.maxLastSeen.Add(-staleWindow)
		res, err := s.db.ExecContext(ctx, `
DELETE FROM postings
WHERE source = ?
  AND id NOT IN (SELECT posting_id FROM bookmarks)
  AND (
    last_seen_at < ?
    OR (first_seen_at < ? AND always_open = 0)
  )`, b.source, staleBefore, oldBefore)
		if err != nil {
			return total, fmt.Errorf("storage: sweep %s: %w", b.source, err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return total, fmt.Errorf("storage: sweep %s rows affected: %w", b.source, err)
		}
		total += int(n)
	}
	return total, nil
}

// sourceSet returns nil for a nil input (caller wants no filtering) and a
// set-shaped map otherwise. Empty input means "no sources are active" — a
// no-op sweep — which is the natural interpretation.
func sourceSet(sources []string) map[string]bool {
	if sources == nil {
		return nil
	}
	m := make(map[string]bool, len(sources))
	for _, s := range sources {
		m[s] = true
	}
	return m
}

// timeStoreFormat matches time.Time.String() — the format modernc.org/sqlite
// uses when binding a time.Time, and therefore the on-disk representation
// of every DATETIME column written by this package. Lexically sortable
// within UTC, so SQLite string comparison agrees with chronological order.
const timeStoreFormat = "2006-01-02 15:04:05.999999999 -0700 MST"

// AllPostings returns every stored posting (canonical + non-canonical),
// newest first. Used by the dedup pass and tests; the dashboard render
// path uses CanonicalPostings instead.
func (s *Store) AllPostings(ctx context.Context) ([]scraper.Posting, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+selectColumns+` FROM postings ORDER BY first_seen_at DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("storage: query postings: %w", err)
	}
	defer rows.Close()
	var postings []scraper.Posting
	for rows.Next() {
		p, err := scanPosting(rows)
		if err != nil {
			return nil, err
		}
		postings = append(postings, p)
	}
	return postings, rows.Err()
}

// CanonicalPostings returns postings the user should see in the list —
// duplicate_of IS NULL — newest first. Cross-portal duplicates (set by
// the dedup pass) are filtered out; the canonical's render layer can
// fetch its DuplicatesOf siblings to render the "also on …" badge.
func (s *Store) CanonicalPostings(ctx context.Context) ([]scraper.Posting, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+selectColumns+` FROM postings WHERE duplicate_of IS NULL
ORDER BY first_seen_at DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("storage: query canonical postings: %w", err)
	}
	defer rows.Close()
	var postings []scraper.Posting
	for rows.Next() {
		p, err := scanPosting(rows)
		if err != nil {
			return nil, err
		}
		postings = append(postings, p)
	}
	return postings, rows.Err()
}

// MarkDuplicate sets duplicateID's duplicate_of to canonicalID, declaring
// the row as a cross-portal copy of canonicalID. Called by the dedup pass
// after sweep, before re-scoring.
func (s *Store) MarkDuplicate(ctx context.Context, duplicateID, canonicalID int64) error {
	if duplicateID == canonicalID {
		return fmt.Errorf("storage: MarkDuplicate: id %d cannot be a duplicate of itself", duplicateID)
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE postings SET duplicate_of = ? WHERE id = ?`,
		canonicalID, duplicateID)
	if err != nil {
		return fmt.Errorf("storage: mark duplicate %d -> %d: %w", duplicateID, canonicalID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("storage: mark duplicate rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("storage: mark duplicate: posting id %d not found", duplicateID)
	}
	return nil
}

// ClearAllDuplicates resets every duplicate_of to NULL. Used at the start
// of a dedup pass so the rule can be re-evaluated from scratch — cheaper
// than tracking which pairs changed when the matcher is fast.
func (s *Store) ClearAllDuplicates(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE postings SET duplicate_of = NULL WHERE duplicate_of IS NOT NULL`)
	if err != nil {
		return fmt.Errorf("storage: clear duplicates: %w", err)
	}
	return nil
}

// DuplicatesOf returns postings whose duplicate_of equals canonicalID, in
// stable order. Used to render the "also on …" badge on the canonical
// row — only the source is read in practice, but the full posting is
// returned so the caller can also link to alternate URLs if it wants.
func (s *Store) DuplicatesOf(ctx context.Context, canonicalID int64) ([]scraper.Posting, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+selectColumns+` FROM postings WHERE duplicate_of = ? ORDER BY first_seen_at, id`,
		canonicalID)
	if err != nil {
		return nil, fmt.Errorf("storage: query duplicates of %d: %w", canonicalID, err)
	}
	defer rows.Close()
	var postings []scraper.Posting
	for rows.Next() {
		p, err := scanPosting(rows)
		if err != nil {
			return nil, err
		}
		postings = append(postings, p)
	}
	return postings, rows.Err()
}

// rowScanner is satisfied by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanPosting reads one posting row whose columns are selectColumns
// (`id`, postingColumns, `duplicate_of`). It propagates sql.ErrNoRows
// unwrapped.
func scanPosting(row rowScanner) (scraper.Posting, error) {
	var (
		p             scraper.Posting
		location      sql.NullString
		careerLevel   sql.NullString
		education     sql.NullInt64
		educationName sql.NullString
		stackJSON     string
		tagsJSON      string
		publishedAt   sql.NullTime
		closedAt      sql.NullTime
		firstSeen     time.Time
		lastSeen      time.Time
		duplicateOf   sql.NullInt64
	)
	err := row.Scan(
		&p.ID, &p.Source, &p.SourcePostingID, &p.URL, &p.Title, &p.Company, &location,
		&p.Newcomer, &p.MinCareer, &p.MaxCareer, &careerLevel, &education, &educationName,
		&stackJSON, &tagsJSON, &p.Description, &p.RawJSON, &publishedAt, &closedAt,
		&p.AlwaysOpen, &firstSeen, &lastSeen, &duplicateOf,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return scraper.Posting{}, sql.ErrNoRows
	}
	if err != nil {
		return scraper.Posting{}, fmt.Errorf("storage: scan posting: %w", err)
	}

	p.Location = location.String
	p.CareerLevel = careerLevel.String
	if education.Valid {
		v := int(education.Int64)
		p.Education = &v
	}
	p.EducationName = educationName.String
	if err := json.Unmarshal([]byte(stackJSON), &p.StackTags); err != nil {
		return scraper.Posting{}, fmt.Errorf("storage: unmarshal stack tags: %w", err)
	}
	if err := json.Unmarshal([]byte(tagsJSON), &p.Tags); err != nil {
		return scraper.Posting{}, fmt.Errorf("storage: unmarshal tags: %w", err)
	}
	if publishedAt.Valid {
		v := publishedAt.Time.UTC()
		p.PublishedAt = &v
	}
	if closedAt.Valid {
		v := closedAt.Time.UTC()
		p.ClosedAt = &v
	}
	p.FirstSeenAt = firstSeen.UTC()
	p.LastSeenAt = lastSeen.UTC()
	if duplicateOf.Valid {
		v := duplicateOf.Int64
		p.DuplicateOf = &v
	}
	return p, nil
}

// utcPtr returns t normalized to UTC, or nil when t is nil.
func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func nonNilSlice(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func nonNilTags(t []scraper.Tag) []scraper.Tag {
	if t == nil {
		return []scraper.Tag{}
	}
	return t
}
