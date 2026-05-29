package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ohchanwu/job-scraper/internal/scraper"
)

// SetNotInterested marks postingID as "관심 없음" with the given timestamp,
// leaving an existing muted_at intact if the posting is already muted. The
// caller passes the time so tests can pin it. Mirrors SetBookmark.
func (s *Store) SetNotInterested(ctx context.Context, postingID int64, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO not_interested (posting_id, muted_at) VALUES (?, ?)
ON CONFLICT(posting_id) DO NOTHING`, postingID, at.UTC())
	if err != nil {
		return fmt.Errorf("storage: set not-interested: %w", err)
	}
	return nil
}

// ClearNotInterested un-mutes postingID. It is a no-op when the posting is
// not muted.
func (s *Store) ClearNotInterested(ctx context.Context, postingID int64) error {
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM not_interested WHERE posting_id = ?`, postingID); err != nil {
		return fmt.Errorf("storage: clear not-interested: %w", err)
	}
	return nil
}

// IsNotInterested reports whether postingID is muted.
func (s *Store) IsNotInterested(ctx context.Context, postingID int64) (bool, error) {
	var one int
	err := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM not_interested WHERE posting_id = ?`, postingID).Scan(&one)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("storage: check not-interested: %w", err)
	default:
		return true, nil
	}
}

// NotInterestedIDs returns the set of currently-muted posting ids — used by
// the briefing and 관심 공고 views to filter muted postings out entirely.
func (s *Store) NotInterestedIDs(ctx context.Context) (map[int64]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT posting_id FROM not_interested`)
	if err != nil {
		return nil, fmt.Errorf("storage: query not-interested ids: %w", err)
	}
	defer rows.Close()
	ids := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("storage: scan not-interested id: %w", err)
		}
		ids[id] = true
	}
	return ids, rows.Err()
}

// NotInterestedPostings returns every muted posting joined with the posting
// row, ordered by muted_at descending (most recently muted first). Used by
// the profile form's unmute list.
func (s *Store) NotInterestedPostings(ctx context.Context) ([]scraper.Posting, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT p.id, `+postingColumns+`, p.duplicate_of
FROM postings p
JOIN not_interested n ON n.posting_id = p.id
ORDER BY n.muted_at DESC, p.id DESC`)
	if err != nil {
		return nil, fmt.Errorf("storage: query not-interested postings: %w", err)
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
