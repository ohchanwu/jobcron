package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ohchanwu/job-scraper/internal/scraper"
)

// SetBookmark inserts a bookmark for postingID with the given timestamp,
// leaving the existing bookmarked_at intact if the posting is already
// bookmarked. The caller passes the time so tests can pin it.
func (s *Store) SetBookmark(ctx context.Context, postingID int64, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO bookmarks (posting_id, bookmarked_at) VALUES (?, ?)
ON CONFLICT(posting_id) DO NOTHING`, postingID, at.UTC())
	if err != nil {
		return fmt.Errorf("storage: set bookmark: %w", err)
	}
	return nil
}

// ClearBookmark removes the bookmark for postingID. It is a no-op when the
// posting is not bookmarked.
func (s *Store) ClearBookmark(ctx context.Context, postingID int64) error {
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM bookmarks WHERE posting_id = ?`, postingID); err != nil {
		return fmt.Errorf("storage: clear bookmark: %w", err)
	}
	return nil
}

// IsBookmarked reports whether postingID is bookmarked.
func (s *Store) IsBookmarked(ctx context.Context, postingID int64) (bool, error) {
	var one int
	err := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM bookmarks WHERE posting_id = ?`, postingID).Scan(&one)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("storage: check bookmark: %w", err)
	default:
		return true, nil
	}
}

// BookmarkedIDs returns the set of currently-bookmarked posting ids — used
// by the dashboard to render the bookmark icon's filled state.
func (s *Store) BookmarkedIDs(ctx context.Context) (map[int64]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT posting_id FROM bookmarks`)
	if err != nil {
		return nil, fmt.Errorf("storage: query bookmarked ids: %w", err)
	}
	defer rows.Close()
	ids := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("storage: scan bookmarked id: %w", err)
		}
		ids[id] = true
	}
	return ids, rows.Err()
}

// BookmarkedPostings returns every bookmarked posting joined with the
// posting row, ordered by bookmarked_at descending (most recently saved
// first). Used by the /bookmarks page.
func (s *Store) BookmarkedPostings(ctx context.Context) ([]scraper.Posting, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT p.id, `+postingColumns+`
FROM postings p
JOIN bookmarks b ON b.posting_id = p.id
ORDER BY b.bookmarked_at DESC, p.id DESC`)
	if err != nil {
		return nil, fmt.Errorf("storage: query bookmarked postings: %w", err)
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
