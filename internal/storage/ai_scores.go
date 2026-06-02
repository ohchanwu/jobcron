package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ohchanwu/job-scraper/internal/ai"
)

// UpsertAIScore caches one Stage-2 delta, keyed by (posting_id, ai_input_hash,
// ai_version). The surviving gated items are stored as items_json and the net is
// stored alongside so a read need not re-sum. Re-rating the same posting under
// the same AI-input hash overwrites the row (refreshing computed_at). The
// ai_input_hash is the hash of the goal text only (profile.AIInputHash), so a
// weight/MinScore tweak keeps the cached delta fresh — only a goal edit rotates
// the key (T1).
func (s *Store) UpsertAIScore(
	ctx context.Context, postingID int64, aiInputHash, aiVersion string,
	d ai.Delta, computedAt time.Time,
) error {
	items := d.Items
	if items == nil {
		items = []ai.DeltaItem{}
	}
	itemsJSON, err := json.Marshal(items)
	if err != nil {
		return fmt.Errorf("storage: marshal ai score items: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO ai_scores
    (posting_id, ai_input_hash, ai_version, items_json, net_delta, computed_at)
VALUES (?,?,?,?,?,?)
ON CONFLICT(posting_id, ai_input_hash, ai_version) DO UPDATE SET
    items_json  = excluded.items_json,
    net_delta   = excluded.net_delta,
    computed_at = excluded.computed_at`,
		postingID, aiInputHash, aiVersion, string(itemsJSON), d.NetDelta, computedAt.UTC())
	if err != nil {
		return fmt.Errorf("storage: upsert ai score: %w", err)
	}
	return nil
}

// AIScore returns the cached delta for the exact
// (posting_id, ai_input_hash, ai_version) key, or ok=false on a miss. This is
// the "fresh" lookup: a hit is a delta computed against the current goal text,
// so the caller leaves Stale=false.
func (s *Store) AIScore(
	ctx context.Context, postingID int64, aiInputHash, aiVersion string,
) (ai.Delta, bool, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT items_json, net_delta
FROM ai_scores
WHERE posting_id = ? AND ai_input_hash = ? AND ai_version = ?`,
		postingID, aiInputHash, aiVersion)
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
	ctx context.Context, postingID int64, aiVersion string,
) (ai.Delta, bool, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT items_json, net_delta
FROM ai_scores
WHERE posting_id = ? AND ai_version = ?
ORDER BY computed_at DESC
LIMIT 1`, postingID, aiVersion)
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
	ctx context.Context, aiInputHash, aiVersion string,
) (map[int64]ai.Delta, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT posting_id, items_json, net_delta
FROM ai_scores
WHERE ai_input_hash = ? AND ai_version = ?`, aiInputHash, aiVersion)
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
	ctx context.Context, aiVersion string,
) (map[int64]ai.Delta, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT posting_id, items_json, net_delta
FROM ai_scores
WHERE ai_version = ?
ORDER BY posting_id, computed_at DESC`, aiVersion)
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
