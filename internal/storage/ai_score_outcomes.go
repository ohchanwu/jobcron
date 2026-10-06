package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ohchanwu/jobcron/internal/ai"
)

const (
	AIScoreRated    = "rated"
	AIScoreNoSignal = "no_signal"
	AIScoreRejected = "rejected"
	AIScoreFailed   = "failed"
)

// AIScoreOutcome is bounded provenance, never raw model output or an attempt ledger.
// The key exactly matches the successful score cache's user/posting/goal/version.
type AIScoreOutcome struct {
	State      string
	Proposed   int
	Accepted   int
	ComputedAt time.Time
}

// UpsertAIResult atomically persists a successful delta and its provenance.
func (s *Store) UpsertAIResult(ctx context.Context, userID, postingID int64, hash, version string, d ai.Delta, outcome AIScoreOutcome) error {
	if err := s.validateOutcomeUserID(userID); err != nil {
		return err
	}
	if outcome.State != AIScoreRated && outcome.State != AIScoreNoSignal {
		return fmt.Errorf("storage: AI result requires successful provenance")
	}
	if (outcome.State == AIScoreNoSignal && (outcome.Proposed != 0 || len(d.Items) != 0 || d.NetDelta != 0)) ||
		(outcome.State == AIScoreRated && (outcome.Accepted != len(d.Items) || outcome.Accepted == 0)) {
		return fmt.Errorf("storage: inconsistent AI result provenance")
	}
	return s.upsertAIScore(ctx, userID, postingID, hash, version, d, outcome.ComputedAt, &outcome)
}

func upsertAIScoreOutcome(ctx context.Context, tx *sql.Tx, s *Store, userID, postingID int64, hash, version string, o AIScoreOutcome) error {
	if err := s.validateOutcomeUserID(userID); err != nil {
		return err
	}
	if o.Proposed < 0 || o.Accepted < 0 || o.Accepted > o.Proposed ||
		(o.State == AIScoreNoSignal && (o.Proposed != 0 || o.Accepted != 0)) ||
		(o.State == AIScoreRejected && (o.Proposed == 0 || o.Accepted != 0)) ||
		(o.State == AIScoreRated && o.Accepted == 0) ||
		(o.State != AIScoreRated && o.State != AIScoreNoSignal && o.State != AIScoreRejected && o.State != AIScoreFailed) {
		return fmt.Errorf("storage: invalid AI outcome")
	}
	query := `INSERT INTO ai_score_outcomes
 (user_id, posting_id, ai_input_hash, ai_version, state, proposed, accepted, computed_at)
 VALUES (?,?,?,?,?,?,?,?)
 ON CONFLICT(user_id, posting_id, ai_input_hash, ai_version) DO UPDATE SET
 state=excluded.state, proposed=excluded.proposed, accepted=excluded.accepted, computed_at=excluded.computed_at`
	if o.State == AIScoreRejected || o.State == AIScoreFailed {
		// Protect an actual cache hit, not orphan provenance left by normal
		// cross-version pruning (including writes through compatible recovery).
		cacheQuery := `SELECT 1 FROM ai_scores WHERE posting_id=? AND ai_input_hash=? AND ai_version=?`
		cacheArgs := []any{postingID, hash, version}
		if s.dialect == DialectPostgres {
			cacheQuery += ` AND user_id=?`
			cacheArgs = append(cacheArgs, userID)
		}
		var hit int
		if err := tx.QueryRowContext(ctx, s.query(cacheQuery), cacheArgs...).Scan(&hit); err != sql.ErrNoRows {
			if err != nil {
				return err
			}
			return fmt.Errorf("storage: AI outcome conflicts with a successful result")
		}
	}
	result, err := tx.ExecContext(ctx, s.query(query),
		userID, postingID, hash, version, o.State, o.Proposed, o.Accepted, o.ComputedAt.UTC())
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return fmt.Errorf("storage: AI outcome conflicts with a successful result")
	}
	return nil
}

func (s *Store) AIScoreOutcomesByPostingID(ctx context.Context, userID int64, hash, version string) (map[int64]AIScoreOutcome, error) {
	if err := s.validateOutcomeUserID(userID); err != nil {
		return nil, err
	}
	// A successful outcome has no independent lifetime: without its exact
	// score cache row it must not present a current completed-analysis card.
	cacheUser := ""
	if s.dialect == DialectPostgres {
		cacheUser = ` AND ai_scores.user_id=ai_score_outcomes.user_id`
	}
	rows, err := s.db.QueryContext(ctx, s.query(`SELECT posting_id, state, proposed, accepted, computed_at FROM ai_score_outcomes
 WHERE user_id=? AND ai_input_hash=? AND ai_version=?
 AND (state NOT IN ('rated','no_signal') OR EXISTS (
 SELECT 1 FROM ai_scores WHERE ai_scores.posting_id=ai_score_outcomes.posting_id
 AND ai_scores.ai_input_hash=ai_score_outcomes.ai_input_hash
 AND ai_scores.ai_version=ai_score_outcomes.ai_version`+cacheUser+`))`), userID, hash, version)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int64]AIScoreOutcome)
	for rows.Next() {
		var id int64
		var o AIScoreOutcome
		if err := rows.Scan(&id, &o.State, &o.Proposed, &o.Accepted, &o.ComputedAt); err != nil {
			return nil, err
		}
		out[id] = o
	}
	return out, rows.Err()
}

// UpsertAIScoreFailure stores a definite unsuccessful outcome, never a score cache hit.
func (s *Store) UpsertAIScoreFailure(ctx context.Context, userID, postingID int64, hash, version string, o AIScoreOutcome) error {
	if err := s.validateOutcomeUserID(userID); err != nil {
		return err
	}
	if o.State != AIScoreRejected && o.State != AIScoreFailed {
		return fmt.Errorf("storage: expected unsuccessful AI outcome")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if s.dialect == DialectPostgres {
		// Serialize against score writes before examining the cache. A concurrent
		// successful transaction must not be overwritten by a failure's older view.
		if _, err := tx.ExecContext(ctx, s.query(`SELECT id FROM postings WHERE id=? FOR UPDATE`), postingID); err != nil {
			return err
		}
	}
	if err := upsertAIScoreOutcome(ctx, tx, s, userID, postingID, hash, version, o); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) validateOutcomeUserID(userID int64) error {
	if err := validateAIUserID(userID); err != nil {
		return err
	}
	// SQLite is the single-user legacy/demo fixture, not a multi-user runtime.
	if s.dialect == DialectSQLite && userID != 1 {
		return fmt.Errorf("storage: SQLite AI outcomes require the legacy sole user")
	}
	return nil
}
