package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ohchanwu/jobcron/internal/ai"
)

// UpsertAIExtraction caches one Stage-1 extraction, keyed by
// (posting_id, content_hash, ai_version). Re-extracting the same content under
// the same version overwrites the row (which only refreshes computed_at). The
// row lives only in ai_extractions — the postings columns stay a faithful
// source mirror (D2, cache-read).
func (s *Store) UpsertAIExtraction(
	ctx context.Context, postingID int64, contentHash, aiVersion string,
	ext ai.Extraction, computedAt time.Time,
) error {
	if s.dialect == DialectSQLite && ext.EducationEvidence != "" {
		return errors.New("storage: legacy SQLite cannot store education evidence")
	}
	var maxCareer sql.NullInt64
	if ext.MaxCareer != nil {
		maxCareer = sql.NullInt64{Int64: int64(*ext.MaxCareer), Valid: true}
	}
	if s.dialect == DialectPostgres {
		_, err := s.db.ExecContext(ctx, `
INSERT INTO ai_extractions
    (posting_id, content_hash, ai_version, min_career, max_career, newcomer, education_enum, career_evidence, education_evidence, computed_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
ON CONFLICT(posting_id, content_hash, ai_version) DO UPDATE SET
    min_career         = excluded.min_career,
    max_career         = excluded.max_career,
    newcomer           = excluded.newcomer,
    education_enum     = excluded.education_enum,
    career_evidence    = excluded.career_evidence,
    education_evidence = excluded.education_evidence,
    computed_at        = excluded.computed_at`,
			postingID, contentHash, aiVersion, ext.MinCareer, maxCareer,
			ext.Newcomer, ext.EducationEnum, ext.CareerEvidence, ext.EducationEvidence, computedAt.UTC())
		if err != nil {
			return fmt.Errorf("storage: upsert ai extraction: %w", err)
		}
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO ai_extractions
    (posting_id, content_hash, ai_version, min_career, max_career, newcomer, education_enum, evidence, computed_at)
VALUES (?,?,?,?,?,?,?,?,?)
ON CONFLICT(posting_id, content_hash, ai_version) DO UPDATE SET
    min_career     = excluded.min_career,
    max_career     = excluded.max_career,
    newcomer       = excluded.newcomer,
    education_enum = excluded.education_enum,
    evidence       = excluded.evidence,
	computed_at    = excluded.computed_at`,
		postingID, contentHash, aiVersion, ext.MinCareer, maxCareer,
		ext.Newcomer, ext.EducationEnum, ext.CareerEvidence, computedAt.UTC())
	if err != nil {
		return fmt.Errorf("storage: upsert ai extraction: %w", err)
	}
	return nil
}

// AIExtraction returns the cached extraction for the exact
// (posting_id, content_hash, ai_version) key, or ok=false on a miss. The
// scrape pipeline (T4) uses it to skip the provider call on a content hit.
func (s *Store) AIExtraction(
	ctx context.Context, postingID int64, contentHash, aiVersion string,
) (ai.Extraction, bool, error) {
	query := `
SELECT min_career, max_career, newcomer, education_enum, evidence
FROM ai_extractions
WHERE posting_id = ? AND content_hash = ? AND ai_version = ?`
	if s.dialect == DialectPostgres {
		query = `
SELECT min_career, max_career, newcomer, education_enum, career_evidence, education_evidence
FROM ai_extractions
WHERE posting_id = ? AND content_hash = ? AND ai_version = ?`
	}
	row := s.db.QueryRowContext(ctx, s.query(query), postingID, contentHash, aiVersion)
	ext, err := scanExtraction(row, s.dialect == DialectPostgres)
	if errors.Is(err, sql.ErrNoRows) {
		return ai.Extraction{}, false, nil
	}
	if err != nil {
		return ai.Extraction{}, false, err
	}
	return ext, true, nil
}

// AIExtractionsByPostingID returns, per posting id, the latest cached
// extraction for the given ai_version (newest computed_at wins when a posting
// has more than one content_hash row). One query for the whole corpus — the
// scoring merge (scoreAll) calls it once and looks up by posting id, never
// N+1. Postings with no extraction are simply absent from the map.
func (s *Store) AIExtractionsByPostingID(ctx context.Context, aiVersion string) (map[int64]ai.Extraction, error) {
	query := `
SELECT posting_id, min_career, max_career, newcomer, education_enum, evidence
FROM ai_extractions
WHERE ai_version = ?
ORDER BY posting_id, computed_at DESC`
	if s.dialect == DialectPostgres {
		query = `
SELECT posting_id, min_career, max_career, newcomer, education_enum, career_evidence, education_evidence
FROM ai_extractions
WHERE ai_version = ?
ORDER BY posting_id, computed_at DESC`
	}
	rows, err := s.db.QueryContext(ctx, s.query(query), aiVersion)
	if err != nil {
		return nil, fmt.Errorf("storage: query ai extractions: %w", err)
	}
	defer rows.Close()
	out := map[int64]ai.Extraction{}
	for rows.Next() {
		var pid int64
		ext, err := scanExtractionWithID(rows, &pid, s.dialect == DialectPostgres)
		if err != nil {
			return nil, err
		}
		if _, seen := out[pid]; seen {
			continue // ORDER BY computed_at DESC: first row per posting is the latest
		}
		out[pid] = ext
	}
	return out, rows.Err()
}

func scanExtraction(row rowScanner, splitEvidence bool) (ai.Extraction, error) {
	var (
		ext       ai.Extraction
		maxCareer sql.NullInt64
	)
	values := []any{&ext.MinCareer, &maxCareer, &ext.Newcomer, &ext.EducationEnum, &ext.CareerEvidence}
	if splitEvidence {
		values = append(values, &ext.EducationEvidence)
	}
	if err := row.Scan(values...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ai.Extraction{}, sql.ErrNoRows
		}
		return ai.Extraction{}, fmt.Errorf("storage: scan ai extraction: %w", err)
	}
	if maxCareer.Valid {
		v := int(maxCareer.Int64)
		ext.MaxCareer = &v
	}
	return ext, nil
}

// scanExtractionWithID is scanExtraction for the batched query, which selects
// posting_id as the leading column.
func scanExtractionWithID(rows *sql.Rows, pid *int64, splitEvidence bool) (ai.Extraction, error) {
	var (
		ext       ai.Extraction
		maxCareer sql.NullInt64
	)
	values := []any{pid, &ext.MinCareer, &maxCareer, &ext.Newcomer, &ext.EducationEnum, &ext.CareerEvidence}
	if splitEvidence {
		values = append(values, &ext.EducationEvidence)
	}
	if err := rows.Scan(values...); err != nil {
		return ai.Extraction{}, fmt.Errorf("storage: scan ai extraction: %w", err)
	}
	if maxCareer.Valid {
		v := int(maxCareer.Int64)
		ext.MaxCareer = &v
	}
	return ext, nil
}
