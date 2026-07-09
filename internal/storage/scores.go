package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Score is a stored scoring result for one posting. BreakdownJSON holds the
// scoring breakdown as raw JSON, keeping the storage layer decoupled from the
// scoring package's types.
type Score struct {
	PostingID     int64
	ProfileHash   string // the profile this score was computed against
	Total         int    // -1 for a dealbreaker hit, else 0..100
	BreakdownJSON string
	ComputedAt    time.Time
}

// UpsertScore stores the score for a posting. posting_id is the primary key,
// so re-scoring a posting overwrites its previous score row — the design keeps
// exactly one score per posting, with no history.
func (s *Store) UpsertScore(ctx context.Context, sc Score) error {
	_, err := s.db.ExecContext(ctx, s.query(`
INSERT INTO scores (posting_id, profile_hash, total, breakdown_json, computed_at)
VALUES (?,?,?,?,?)
ON CONFLICT(posting_id) DO UPDATE SET
    profile_hash   = excluded.profile_hash,
    total          = excluded.total,
    breakdown_json = excluded.breakdown_json,
    computed_at    = excluded.computed_at`),
		sc.PostingID, sc.ProfileHash, sc.Total, sc.BreakdownJSON, sc.ComputedAt.UTC())
	if err != nil {
		return fmt.Errorf("storage: upsert score: %w", err)
	}
	return nil
}

// ScoresByPostingID returns every stored score, keyed by posting id.
func (s *Store) ScoresByPostingID(ctx context.Context) (map[int64]Score, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT posting_id, profile_hash, total, breakdown_json, computed_at FROM scores`)
	if err != nil {
		return nil, fmt.Errorf("storage: query scores: %w", err)
	}
	defer rows.Close()
	scores := map[int64]Score{}
	for rows.Next() {
		var (
			sc         Score
			computedAt time.Time
		)
		if err := rows.Scan(&sc.PostingID, &sc.ProfileHash, &sc.Total,
			&sc.BreakdownJSON, &computedAt); err != nil {
			return nil, fmt.Errorf("storage: scan score: %w", err)
		}
		sc.ComputedAt = computedAt.UTC()
		scores[sc.PostingID] = sc
	}
	return scores, rows.Err()
}

// ScoreByPostingID returns the stored score for a posting, or ok=false if none.
func (s *Store) ScoreByPostingID(ctx context.Context, postingID int64) (Score, bool, error) {
	var (
		sc         Score
		computedAt time.Time
	)
	err := s.db.QueryRowContext(ctx, s.query(`
SELECT posting_id, profile_hash, total, breakdown_json, computed_at
FROM scores WHERE posting_id = ?`), postingID).Scan(
		&sc.PostingID, &sc.ProfileHash, &sc.Total, &sc.BreakdownJSON, &computedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Score{}, false, nil
	}
	if err != nil {
		return Score{}, false, fmt.Errorf("storage: query score: %w", err)
	}
	sc.ComputedAt = computedAt.UTC()
	return sc, true, nil
}
