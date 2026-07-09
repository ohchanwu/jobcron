package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// AddAIUsage adds one call's token usage to the rolling daily ledger for the
// given UTC day (e.g. "2026-06-03"). The increment is atomic at the SQL level
// (ON CONFLICT … SET x = x + excluded), so the running total survives across
// process restarts and never loses a debit to a read-modify-write race within a
// single connection. day is the caller's UTC date string so the ledger and the
// budget agree on the boundary.
func (s *Store) AddAIUsage(ctx context.Context, day string, inputTokens, outputTokens int) error {
	_, err := s.db.ExecContext(ctx, s.query(`
INSERT INTO ai_usage (day, input_tokens, output_tokens)
VALUES (?,?,?)
ON CONFLICT(day) DO UPDATE SET
    input_tokens  = ai_usage.input_tokens  + excluded.input_tokens,
    output_tokens = ai_usage.output_tokens + excluded.output_tokens`),
		day, inputTokens, outputTokens)
	if err != nil {
		return fmt.Errorf("storage: add ai usage: %w", err)
	}
	return nil
}

// AIUsageForDay returns the input and output token totals recorded for the given
// UTC day. A day with no row reads as (0, 0) — an unused day is not an error.
func (s *Store) AIUsageForDay(ctx context.Context, day string) (inputTokens, outputTokens int, err error) {
	err = s.db.QueryRowContext(ctx,
		s.query(`SELECT input_tokens, output_tokens FROM ai_usage WHERE day = ?`), day).
		Scan(&inputTokens, &outputTokens)
	if err == nil {
		return inputTokens, outputTokens, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, nil
	}
	return 0, 0, fmt.Errorf("storage: query ai usage: %w", err)
}
