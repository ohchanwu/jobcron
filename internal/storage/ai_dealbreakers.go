package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ohchanwu/jobcron/internal/ai"
)

type AIDealbreakerValidation struct {
	PostingID   int64
	ContentHash string
	AIVersion   string
	KeywordHash string
	Validation  ai.DealbreakerValidation
	ComputedAt  time.Time
}

func (s *Store) UpsertAIDealbreakerValidation(
	ctx context.Context,
	userID int64,
	postingID int64,
	contentHash string,
	aiVersion string,
	keywordHash string,
	validation ai.DealbreakerValidation,
	computedAt time.Time,
) error {
	if err := s.validateAIDealbreakerStore(userID); err != nil {
		return err
	}
	if validation.CandidateID != keywordHash {
		return errors.New("storage: dealbreaker validation candidate does not match keyword hash")
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO ai_dealbreaker_validations (
    user_id, posting_id, content_hash, ai_version, keyword_hash,
    verdict, evidence, computed_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT (user_id, posting_id, content_hash, ai_version, keyword_hash) DO UPDATE SET
    verdict = EXCLUDED.verdict,
    evidence = EXCLUDED.evidence,
    computed_at = EXCLUDED.computed_at`,
		userID, postingID, contentHash, aiVersion, keywordHash,
		validation.Verdict, validation.Evidence, computedAt.UTC())
	if err != nil {
		return fmt.Errorf("storage: upsert ai dealbreaker validation: %w", err)
	}
	return nil
}

func (s *Store) AIDealbreakerValidationsByPostingID(
	ctx context.Context,
	userID int64,
	aiVersion string,
) (map[int64]map[string]AIDealbreakerValidation, error) {
	if err := s.validateAIDealbreakerStore(userID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT posting_id, content_hash, ai_version, keyword_hash, verdict, evidence, computed_at
  FROM ai_dealbreaker_validations
 WHERE user_id = $1 AND ai_version = $2`, userID, aiVersion)
	if err != nil {
		return nil, fmt.Errorf("storage: query ai dealbreaker validations: %w", err)
	}
	defer rows.Close()

	out := make(map[int64]map[string]AIDealbreakerValidation)
	for rows.Next() {
		var row AIDealbreakerValidation
		if err := rows.Scan(
			&row.PostingID,
			&row.ContentHash,
			&row.AIVersion,
			&row.KeywordHash,
			&row.Validation.Verdict,
			&row.Validation.Evidence,
			&row.ComputedAt,
		); err != nil {
			return nil, fmt.Errorf("storage: scan ai dealbreaker validation: %w", err)
		}
		row.Validation.CandidateID = row.KeywordHash
		if out[row.PostingID] == nil {
			out[row.PostingID] = make(map[string]AIDealbreakerValidation)
		}
		out[row.PostingID][row.ContentHash+"\x00"+row.KeywordHash] = row
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("storage: query ai dealbreaker validations: %w", err)
	}
	return out, nil
}

func (s *Store) validateAIDealbreakerStore(userID int64) error {
	if userID <= 0 {
		return errors.New("storage: dealbreaker validation user ID must be positive")
	}
	if s.dialect != DialectPostgres {
		return errors.New("storage: dealbreaker validations require PostgreSQL")
	}
	return nil
}
