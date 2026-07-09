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
	if s.dialect == DialectPostgres {
		userID, ok, err := s.firstUserID(ctx)
		if err != nil || !ok {
			return err
		}
		return s.addBookmarkAt(ctx, userID, postingID, at)
	}
	_, err := s.db.ExecContext(ctx, s.query(`
INSERT INTO bookmarks (posting_id, bookmarked_at) VALUES (?, ?)
ON CONFLICT(posting_id) DO NOTHING`), postingID, at.UTC())
	if err != nil {
		return fmt.Errorf("storage: set bookmark: %w", err)
	}
	return nil
}

// AddBookmark inserts a bookmark for one user. The timestamp is owned by the
// storage layer because production callers should not fabricate ownership
// state; tests that need pinned timestamps use the transitional SetBookmark
// wrapper on SQLite.
func (s *Store) AddBookmark(ctx context.Context, userID, postingID int64) error {
	return s.addBookmarkAt(ctx, userID, postingID, time.Now())
}

func (s *Store) addBookmarkAt(ctx context.Context, userID, postingID int64, at time.Time) error {
	if s.dialect == DialectSQLite {
		return s.SetBookmark(ctx, postingID, at)
	}
	if userID == 0 {
		return errors.New("storage: bookmark user id is required")
	}
	_, err := s.db.ExecContext(ctx, s.query(`
INSERT INTO bookmarks (user_id, posting_id, bookmarked_at) VALUES (?, ?, ?)
ON CONFLICT(user_id, posting_id) DO NOTHING`), userID, postingID, at.UTC())
	if err != nil {
		return fmt.Errorf("storage: add bookmark: %w", err)
	}
	return nil
}

// ClearBookmark removes the bookmark for postingID. It is a no-op when the
// posting is not bookmarked.
func (s *Store) ClearBookmark(ctx context.Context, postingID int64) error {
	if s.dialect == DialectPostgres {
		userID, ok, err := s.firstUserID(ctx)
		if err != nil || !ok {
			return err
		}
		return s.ClearBookmarkForUser(ctx, userID, postingID)
	}
	if _, err := s.db.ExecContext(ctx,
		s.query(`DELETE FROM bookmarks WHERE posting_id = ?`), postingID); err != nil {
		return fmt.Errorf("storage: clear bookmark: %w", err)
	}
	return nil
}

func (s *Store) ClearBookmarkForUser(ctx context.Context, userID, postingID int64) error {
	if s.dialect == DialectSQLite {
		return s.ClearBookmark(ctx, postingID)
	}
	if userID == 0 {
		return errors.New("storage: bookmark user id is required")
	}
	if _, err := s.db.ExecContext(ctx,
		s.query(`DELETE FROM bookmarks WHERE user_id = ? AND posting_id = ?`), userID, postingID); err != nil {
		return fmt.Errorf("storage: clear user bookmark: %w", err)
	}
	return nil
}

// IsBookmarked reports whether postingID is bookmarked.
func (s *Store) IsBookmarked(ctx context.Context, postingID int64) (bool, error) {
	if s.dialect == DialectPostgres {
		userID, ok, err := s.firstUserID(ctx)
		if err != nil || !ok {
			return false, err
		}
		return s.IsBookmarkedForUser(ctx, userID, postingID)
	}
	var one int
	err := s.db.QueryRowContext(ctx,
		s.query(`SELECT 1 FROM bookmarks WHERE posting_id = ?`), postingID).Scan(&one)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("storage: check bookmark: %w", err)
	default:
		return true, nil
	}
}

func (s *Store) IsBookmarkedForUser(ctx context.Context, userID, postingID int64) (bool, error) {
	if s.dialect == DialectSQLite {
		return s.IsBookmarked(ctx, postingID)
	}
	if userID == 0 {
		return false, errors.New("storage: bookmark user id is required")
	}
	var one int
	err := s.db.QueryRowContext(ctx,
		s.query(`SELECT 1 FROM bookmarks WHERE user_id = ? AND posting_id = ?`), userID, postingID).Scan(&one)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("storage: check user bookmark: %w", err)
	default:
		return true, nil
	}
}

// BookmarkedIDs returns the set of currently-bookmarked posting ids — used
// by the dashboard to render the bookmark icon's filled state.
func (s *Store) BookmarkedIDs(ctx context.Context) (map[int64]bool, error) {
	if s.dialect == DialectPostgres {
		userID, ok, err := s.firstUserID(ctx)
		if err != nil || !ok {
			return map[int64]bool{}, err
		}
		return s.BookmarkedIDsForUser(ctx, userID)
	}
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

func (s *Store) BookmarkedIDsForUser(ctx context.Context, userID int64) (map[int64]bool, error) {
	if s.dialect == DialectSQLite {
		return s.BookmarkedIDs(ctx)
	}
	if userID == 0 {
		return nil, errors.New("storage: bookmark user id is required")
	}
	rows, err := s.db.QueryContext(ctx, s.query(`SELECT posting_id FROM bookmarks WHERE user_id = ?`), userID)
	if err != nil {
		return nil, fmt.Errorf("storage: query user bookmarked ids: %w", err)
	}
	defer rows.Close()
	ids := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("storage: scan user bookmarked id: %w", err)
		}
		ids[id] = true
	}
	return ids, rows.Err()
}

// BookmarkedPostings returns every bookmarked posting joined with the
// posting row, ordered by bookmarked_at descending (most recently saved
// first). Used by the /bookmarks page.
func (s *Store) BookmarkedPostings(ctx context.Context) ([]scraper.Posting, error) {
	if s.dialect == DialectPostgres {
		userID, ok, err := s.firstUserID(ctx)
		if err != nil || !ok {
			return nil, err
		}
		return s.BookmarkedPostingsForUser(ctx, userID)
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT p.id, `+postingColumns+`, p.duplicate_of
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

func (s *Store) BookmarkedPostingsForUser(ctx context.Context, userID int64) ([]scraper.Posting, error) {
	if s.dialect == DialectSQLite {
		return s.BookmarkedPostings(ctx)
	}
	if userID == 0 {
		return nil, errors.New("storage: bookmark user id is required")
	}
	rows, err := s.db.QueryContext(ctx, s.query(`
SELECT p.id, `+postingColumns+`, p.duplicate_of
FROM postings p
JOIN bookmarks b ON b.posting_id = p.id
WHERE b.user_id = ?
ORDER BY b.bookmarked_at DESC, p.id DESC`), userID)
	if err != nil {
		return nil, fmt.Errorf("storage: query user bookmarked postings: %w", err)
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
