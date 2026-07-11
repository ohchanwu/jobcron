package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ohchanwu/jobcron/internal/scraper"
)

// SetNotInterested marks postingID as "관심 없음" with the given timestamp,
// leaving an existing muted_at intact if the posting is already muted. The
// caller passes the time so tests can pin it. Mirrors SetBookmark.
func (s *Store) SetNotInterested(ctx context.Context, postingID int64, at time.Time) error {
	if s.dialect == DialectPostgres {
		userID, ok, err := s.firstUserID(ctx)
		if err != nil || !ok {
			return err
		}
		return s.addNotInterestedAt(ctx, userID, postingID, at)
	}
	_, err := s.db.ExecContext(ctx, s.query(`
INSERT INTO not_interested (posting_id, muted_at) VALUES (?, ?)
ON CONFLICT(posting_id) DO NOTHING`), postingID, at.UTC())
	if err != nil {
		return fmt.Errorf("storage: set not-interested: %w", err)
	}
	return nil
}

func (s *Store) AddNotInterested(ctx context.Context, userID, postingID int64, at time.Time) error {
	return s.addNotInterestedAt(ctx, userID, postingID, at)
}

func (s *Store) addNotInterestedAt(ctx context.Context, userID, postingID int64, at time.Time) error {
	if s.dialect == DialectSQLite {
		return s.SetNotInterested(ctx, postingID, at)
	}
	if userID == 0 {
		return errors.New("storage: not-interested user id is required")
	}
	_, err := s.db.ExecContext(ctx, s.query(`
INSERT INTO not_interested (user_id, posting_id, muted_at) VALUES (?, ?, ?)
ON CONFLICT(user_id, posting_id) DO NOTHING`), userID, postingID, at.UTC())
	if err != nil {
		return fmt.Errorf("storage: add not-interested: %w", err)
	}
	return nil
}

// ClearNotInterested un-mutes postingID. It is a no-op when the posting is
// not muted.
func (s *Store) ClearNotInterested(ctx context.Context, postingID int64) error {
	if s.dialect == DialectPostgres {
		userID, ok, err := s.firstUserID(ctx)
		if err != nil || !ok {
			return err
		}
		return s.ClearNotInterestedForUser(ctx, userID, postingID)
	}
	if _, err := s.db.ExecContext(ctx,
		s.query(`DELETE FROM not_interested WHERE posting_id = ?`), postingID); err != nil {
		return fmt.Errorf("storage: clear not-interested: %w", err)
	}
	return nil
}

func (s *Store) ClearNotInterestedForUser(ctx context.Context, userID, postingID int64) error {
	if s.dialect == DialectSQLite {
		return s.ClearNotInterested(ctx, postingID)
	}
	if userID == 0 {
		return errors.New("storage: not-interested user id is required")
	}
	if _, err := s.db.ExecContext(ctx,
		s.query(`DELETE FROM not_interested WHERE user_id = ? AND posting_id = ?`), userID, postingID); err != nil {
		return fmt.Errorf("storage: clear user not-interested: %w", err)
	}
	return nil
}

// IsNotInterested reports whether postingID is muted.
func (s *Store) IsNotInterested(ctx context.Context, postingID int64) (bool, error) {
	if s.dialect == DialectPostgres {
		userID, ok, err := s.firstUserID(ctx)
		if err != nil || !ok {
			return false, err
		}
		return s.IsNotInterestedForUser(ctx, userID, postingID)
	}
	var one int
	err := s.db.QueryRowContext(ctx,
		s.query(`SELECT 1 FROM not_interested WHERE posting_id = ?`), postingID).Scan(&one)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("storage: check not-interested: %w", err)
	default:
		return true, nil
	}
}

func (s *Store) IsNotInterestedForUser(ctx context.Context, userID, postingID int64) (bool, error) {
	if s.dialect == DialectSQLite {
		return s.IsNotInterested(ctx, postingID)
	}
	if userID == 0 {
		return false, errors.New("storage: not-interested user id is required")
	}
	var one int
	err := s.db.QueryRowContext(ctx,
		s.query(`SELECT 1 FROM not_interested WHERE user_id = ? AND posting_id = ?`), userID, postingID).Scan(&one)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("storage: check user not-interested: %w", err)
	default:
		return true, nil
	}
}

// NotInterestedIDs returns the set of currently-muted posting ids — used by
// the briefing and 전체 공고 views to filter muted postings out entirely.
func (s *Store) NotInterestedIDs(ctx context.Context, userIDOpt ...int64) (map[int64]bool, error) {
	if len(userIDOpt) > 0 || s.dialect == DialectPostgres {
		userID := int64(0)
		if len(userIDOpt) > 0 {
			userID = userIDOpt[0]
		} else {
			var ok bool
			var err error
			userID, ok, err = s.firstUserID(ctx)
			if err != nil || !ok {
				return map[int64]bool{}, err
			}
		}
		return s.notInterestedIDsForUser(ctx, userID)
	}
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

func (s *Store) notInterestedIDsForUser(ctx context.Context, userID int64) (map[int64]bool, error) {
	if s.dialect == DialectSQLite {
		return s.NotInterestedIDs(ctx)
	}
	if userID == 0 {
		return nil, errors.New("storage: not-interested user id is required")
	}
	rows, err := s.db.QueryContext(ctx, s.query(`SELECT posting_id FROM not_interested WHERE user_id = ?`), userID)
	if err != nil {
		return nil, fmt.Errorf("storage: query user not-interested ids: %w", err)
	}
	defer rows.Close()
	ids := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("storage: scan user not-interested id: %w", err)
		}
		ids[id] = true
	}
	return ids, rows.Err()
}

// NotInterestedPostings returns every muted posting joined with the posting
// row, ordered by muted_at descending (most recently muted first). Used by
// the 숨긴 공고 page (/hidden).
func (s *Store) NotInterestedPostings(ctx context.Context) ([]scraper.Posting, error) {
	if s.dialect == DialectPostgres {
		userID, ok, err := s.firstUserID(ctx)
		if err != nil || !ok {
			return nil, err
		}
		return s.NotInterestedPostingsForUser(ctx, userID)
	}
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

func (s *Store) NotInterestedPostingsForUser(ctx context.Context, userID int64) ([]scraper.Posting, error) {
	if s.dialect == DialectSQLite {
		return s.NotInterestedPostings(ctx)
	}
	if userID == 0 {
		return nil, errors.New("storage: not-interested user id is required")
	}
	rows, err := s.db.QueryContext(ctx, s.query(`
SELECT p.id, `+postingColumns+`, p.duplicate_of
FROM postings p
JOIN not_interested n ON n.posting_id = p.id
WHERE n.user_id = ?
ORDER BY n.muted_at DESC, p.id DESC`), userID)
	if err != nil {
		return nil, fmt.Errorf("storage: query user not-interested postings: %w", err)
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
