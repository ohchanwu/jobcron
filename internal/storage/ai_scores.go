package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ohchanwu/jobcron/internal/ai"
)

// UpsertAIScore caches one Stage-2 delta, keyed by
// (user_id, posting_id, ai_input_hash, ai_version) in PostgreSQL. The surviving
// gated items are stored as items_json and the net is stored alongside so a read
// need not re-sum. Re-rating the same posting under the same AI-input hash
// overwrites the row (refreshing computed_at). The ai_input_hash is the hash of
// the goal text only (profile.AIInputHash), so a weight/MinScore tweak keeps the
// cached delta fresh — only a goal edit rotates the key (T1).
//
// After the write it prunes the posting's accumulated dead rows, but
// deliberately NOT to nothing: it keeps (a) ALL rows under the just-written
// ai_version (different goal hashes under the current provider/model stay
// reachable by the version-scoped fresh/stale lookups), and (b) the single
// most-recent row from any OTHER ai_version. That one cross-version row is
// load-bearing: it is what LatestAIScoresAnyVersionByPostingID returns so the
// faded "이전 설정 기준" chip survives a provider/model switch, and — because a
// switch back to a previously-used provider rotates ai_version back — it is what
// brings that provider's prior score back without re-rating. Pruning it would
// make a provider round-trip silently lose the old reading.
func (s *Store) UpsertAIScore(
	ctx context.Context, userID, postingID int64, aiInputHash, aiVersion string,
	d ai.Delta, computedAt time.Time,
) error {
	if err := validateAIUserID(userID); err != nil {
		return err
	}
	items := d.Items
	if items == nil {
		items = []ai.DeltaItem{}
	}
	itemsJSON, err := json.Marshal(items)
	if err != nil {
		return fmt.Errorf("storage: marshal ai score items: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("storage: begin upsert ai score: %w", err)
	}
	defer tx.Rollback()
	upsertSQL := `
INSERT INTO ai_scores
    (posting_id, ai_input_hash, ai_version, items_json, net_delta, computed_at)
VALUES (?,?,?,?,?,?)
ON CONFLICT(posting_id, ai_input_hash, ai_version) DO UPDATE SET
    items_json  = excluded.items_json,
    net_delta   = excluded.net_delta,
	computed_at = excluded.computed_at`
	upsertArgs := []any{postingID, aiInputHash, aiVersion, string(itemsJSON), d.NetDelta, computedAt.UTC()}
	if s.dialect == DialectPostgres {
		upsertSQL = `
INSERT INTO ai_scores
    (user_id, posting_id, ai_input_hash, ai_version, items_json, net_delta, computed_at)
VALUES (?,?,?,?,?,?,?)
ON CONFLICT(user_id, posting_id, ai_input_hash, ai_version) DO UPDATE SET
    items_json  = excluded.items_json,
    net_delta   = excluded.net_delta,
    computed_at = excluded.computed_at`
		upsertArgs = []any{userID, postingID, aiInputHash, aiVersion, string(itemsJSON), d.NetDelta, computedAt.UTC()}
	}
	if _, err = tx.ExecContext(ctx, s.query(upsertSQL), upsertArgs...); err != nil {
		return fmt.Errorf("storage: upsert ai score: %w", err)
	}
	// Prune other-version rows for this posting, keeping only the single
	// most-recent one (current-version rows are never in the delete's scope, so
	// they all survive). computed_at, rowid DESC makes the kept row deterministic
	// on a tie.
	pruneSQL := `
DELETE FROM ai_scores
WHERE posting_id = ?
  AND ai_version <> ?
  AND rowid NOT IN (
    SELECT rowid FROM ai_scores
    WHERE posting_id = ? AND ai_version <> ?
    ORDER BY computed_at DESC, rowid DESC
    LIMIT 1
  )`
	if s.dialect == DialectPostgres {
		pruneSQL = `
DELETE FROM ai_scores
WHERE user_id = ?
  AND posting_id = ?
  AND ai_version <> ?
  AND ctid NOT IN (
    SELECT ctid FROM ai_scores
    WHERE user_id = ? AND posting_id = ? AND ai_version <> ?
    ORDER BY computed_at DESC, ctid DESC
    LIMIT 1
  )`
	}
	pruneArgs := []any{postingID, aiVersion, postingID, aiVersion}
	if s.dialect == DialectPostgres {
		pruneArgs = []any{userID, postingID, aiVersion, userID, postingID, aiVersion}
	}
	if _, err = tx.ExecContext(ctx, s.query(pruneSQL), pruneArgs...); err != nil {
		return fmt.Errorf("storage: prune ai scores: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("storage: commit upsert ai score: %w", err)
	}
	return nil
}

// AIScore returns the cached delta for the exact
// (posting_id, ai_input_hash, ai_version) key, or ok=false on a miss. This is
// the "fresh" lookup: a hit is a delta computed against the current goal text,
// so the caller leaves Stale=false.
func (s *Store) AIScore(
	ctx context.Context, userID, postingID int64, aiInputHash, aiVersion string,
) (ai.Delta, bool, error) {
	if err := validateAIUserID(userID); err != nil {
		return ai.Delta{}, false, err
	}
	query := `
SELECT items_json, net_delta
FROM ai_scores
WHERE posting_id = ? AND ai_input_hash = ? AND ai_version = ?`
	args := []any{postingID, aiInputHash, aiVersion}
	if s.dialect == DialectPostgres {
		query = `
SELECT items_json, net_delta
FROM ai_scores
WHERE user_id = ? AND posting_id = ? AND ai_input_hash = ? AND ai_version = ?`
		args = []any{userID, postingID, aiInputHash, aiVersion}
	}
	row := s.db.QueryRowContext(ctx, s.query(query), args...)
	d, err := scanAIScore(row)
	if errors.Is(err, sql.ErrNoRows) {
		return ai.Delta{}, false, nil
	}
	if err != nil {
		return ai.Delta{}, false, err
	}
	return d, true, nil
}

// LatestAIScore returns the most recently computed delta for a posting under the
// given ai_version, regardless of ai_input_hash, or ok=false on a miss. This is
// the stale-fallback lookup (T1): when no row matches the current goal text, the
// caller merges this latest row but marks it Stale ("이전 프로필 기준"). It rides
// idx_ai_scores_latest.
func (s *Store) LatestAIScore(
	ctx context.Context, userID, postingID int64, aiVersion string,
) (ai.Delta, bool, error) {
	if err := validateAIUserID(userID); err != nil {
		return ai.Delta{}, false, err
	}
	query := `
SELECT items_json, net_delta
FROM ai_scores
WHERE posting_id = ? AND ai_version = ?
ORDER BY computed_at DESC
LIMIT 1`
	args := []any{postingID, aiVersion}
	if s.dialect == DialectPostgres {
		query = `
SELECT items_json, net_delta
FROM ai_scores
WHERE user_id = ? AND posting_id = ? AND ai_version = ?
ORDER BY computed_at DESC
LIMIT 1`
		args = []any{userID, postingID, aiVersion}
	}
	row := s.db.QueryRowContext(ctx, s.query(query), args...)
	d, err := scanAIScore(row)
	if errors.Is(err, sql.ErrNoRows) {
		return ai.Delta{}, false, nil
	}
	if err != nil {
		return ai.Delta{}, false, err
	}
	return d, true, nil
}

// AIScoresByPostingID returns, per posting id, the fresh cached delta for the
// exact (ai_input_hash, ai_version). One query for the whole corpus — the
// scoring merge (scoreAll) calls it once and looks up by posting id, never N+1.
// Postings with no fresh row are simply absent from the map.
func (s *Store) AIScoresByPostingID(
	ctx context.Context, userID int64, aiInputHash, aiVersion string,
) (map[int64]ai.Delta, error) {
	if err := validateAIUserID(userID); err != nil {
		return nil, err
	}
	query := `
SELECT posting_id, items_json, net_delta
FROM ai_scores
WHERE ai_input_hash = ? AND ai_version = ?`
	args := []any{aiInputHash, aiVersion}
	if s.dialect == DialectPostgres {
		query = `
SELECT posting_id, items_json, net_delta
FROM ai_scores
WHERE user_id = ? AND ai_input_hash = ? AND ai_version = ?`
		args = []any{userID, aiInputHash, aiVersion}
	}
	rows, err := s.db.QueryContext(ctx, s.query(query), args...)
	if err != nil {
		return nil, fmt.Errorf("storage: query ai scores: %w", err)
	}
	defer rows.Close()
	out := map[int64]ai.Delta{}
	for rows.Next() {
		var pid int64
		d, err := scanAIScoreWithID(rows, &pid)
		if err != nil {
			return nil, err
		}
		out[pid] = d
	}
	return out, rows.Err()
}

// LatestAIScoresByPostingID returns, per posting id, the most recently computed
// delta under the given ai_version (newest computed_at wins when a posting has
// rows for more than one ai_input_hash). The stale-fallback batch read for
// scoreAll: a posting present here but absent from the fresh map gets its delta
// merged with Stale=true. Postings with no AI score at all are absent.
func (s *Store) LatestAIScoresByPostingID(
	ctx context.Context, userID int64, aiVersion string,
) (map[int64]ai.Delta, error) {
	if err := validateAIUserID(userID); err != nil {
		return nil, err
	}
	query := `
SELECT posting_id, items_json, net_delta
FROM ai_scores
WHERE ai_version = ?
ORDER BY posting_id, computed_at DESC`
	args := []any{aiVersion}
	if s.dialect == DialectPostgres {
		query = `
SELECT posting_id, items_json, net_delta
FROM ai_scores
WHERE user_id = ? AND ai_version = ?
ORDER BY posting_id, computed_at DESC`
		args = []any{userID, aiVersion}
	}
	rows, err := s.db.QueryContext(ctx, s.query(query), args...)
	if err != nil {
		return nil, fmt.Errorf("storage: query latest ai scores: %w", err)
	}
	defer rows.Close()
	out := map[int64]ai.Delta{}
	for rows.Next() {
		var pid int64
		d, err := scanAIScoreWithID(rows, &pid)
		if err != nil {
			return nil, err
		}
		if _, seen := out[pid]; seen {
			continue // ORDER BY computed_at DESC: first row per posting is the latest
		}
		out[pid] = d
	}
	return out, rows.Err()
}

// LatestAIScoresAnyVersionByPostingID returns, per posting id, the most recently
// computed delta REGARDLESS of ai_version (newest computed_at wins). It is the
// cross-version stale fallback: when a posting has no delta under the current
// ai_version at all — the case when the user switches provider or model, which
// rotates ai_version (hash of provider+model+prompt) and orphans every prior row
// from the version-scoped fresh and stale lookups — the merge falls back here so
// the chip persists faded ("이전 설정 기준") instead of vanishing. scoreAll
// prefers the current-version row first (LatestAIScoresByPostingID), so this only
// supplies rows for postings the current provider/model has never rated.
func (s *Store) LatestAIScoresAnyVersionByPostingID(
	ctx context.Context, userID int64,
) (map[int64]ai.Delta, error) {
	if err := validateAIUserID(userID); err != nil {
		return nil, err
	}
	query := `
SELECT posting_id, items_json, net_delta
FROM ai_scores
ORDER BY posting_id, computed_at DESC`
	args := []any{}
	if s.dialect == DialectPostgres {
		query = `
SELECT posting_id, items_json, net_delta
FROM ai_scores
WHERE user_id = ?
ORDER BY posting_id, computed_at DESC`
		args = []any{userID}
	}
	rows, err := s.db.QueryContext(ctx, s.query(query), args...)
	if err != nil {
		return nil, fmt.Errorf("storage: query latest ai scores (any version): %w", err)
	}
	defer rows.Close()
	out := map[int64]ai.Delta{}
	for rows.Next() {
		var pid int64
		d, err := scanAIScoreWithID(rows, &pid)
		if err != nil {
			return nil, err
		}
		if _, seen := out[pid]; seen {
			continue // ORDER BY computed_at DESC: first row per posting is the latest
		}
		out[pid] = d
	}
	return out, rows.Err()
}

func validateAIUserID(userID int64) error {
	if userID <= 0 {
		return fmt.Errorf("storage: AI user ID must be positive")
	}
	return nil
}

// scanAIScore reads items_json + net_delta into an ai.Delta. Stale stays false;
// the merge layer flips it on the fallback path.
func scanAIScore(row rowScanner) (ai.Delta, error) {
	var (
		itemsJSON string
		netDelta  int
	)
	if err := row.Scan(&itemsJSON, &netDelta); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ai.Delta{}, sql.ErrNoRows
		}
		return ai.Delta{}, fmt.Errorf("storage: scan ai score: %w", err)
	}
	return unmarshalAIScore(itemsJSON, netDelta)
}

// scanAIScoreWithID is scanAIScore for the batched queries, which select
// posting_id as the leading column.
func scanAIScoreWithID(rows *sql.Rows, pid *int64) (ai.Delta, error) {
	var (
		itemsJSON string
		netDelta  int
	)
	if err := rows.Scan(pid, &itemsJSON, &netDelta); err != nil {
		return ai.Delta{}, fmt.Errorf("storage: scan ai score: %w", err)
	}
	return unmarshalAIScore(itemsJSON, netDelta)
}

// unmarshalAIScore rebuilds an ai.Delta from a stored items_json + net_delta.
func unmarshalAIScore(itemsJSON string, netDelta int) (ai.Delta, error) {
	var items []ai.DeltaItem
	if err := json.Unmarshal([]byte(itemsJSON), &items); err != nil {
		return ai.Delta{}, fmt.Errorf("storage: unmarshal ai score items: %w", err)
	}
	return ai.Delta{Items: items, NetDelta: netDelta}, nil
}
