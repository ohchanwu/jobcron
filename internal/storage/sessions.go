package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ohchanwu/job-scraper/internal/auth"
)

// CreateSession stores a hashed session token for a user. Callers must pass the
// SHA-256 token hash, never the raw bearer token.
func (s *Store) CreateSession(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) error {
	if userID == 0 {
		return errors.New("storage: session user id is required")
	}
	if tokenHash == "" {
		return errors.New("storage: session token hash is required")
	}
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, s.query(`
INSERT INTO sessions (user_id, session_token_hash, created_at, expires_at, last_seen_at)
VALUES (?, ?, ?, ?, ?)`), userID, tokenHash, now, expiresAt.UTC(), now)
	if err != nil {
		return fmt.Errorf("storage: create session: %w", err)
	}
	return nil
}

// UserBySessionToken returns the user for an unexpired raw session token.
func (s *Store) UserBySessionToken(ctx context.Context, token string) (User, bool, error) {
	if token == "" {
		return User{}, false, nil
	}
	tokenHash := auth.HashSessionToken(token)
	now := time.Now().UTC()
	row := s.db.QueryRowContext(ctx, s.query(`
SELECT u.id, u.email, u.password_hash, u.created_at, u.updated_at
  FROM sessions s
  JOIN users u ON u.id = s.user_id
 WHERE s.session_token_hash = ?
   AND s.expires_at > ?`), tokenHash, now)
	user, err := scanOwnerUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, fmt.Errorf("storage: user by session token: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, s.query(`
UPDATE sessions
   SET last_seen_at = ?
 WHERE session_token_hash = ?`), now, tokenHash); err != nil {
		return User{}, false, fmt.Errorf("storage: update session last seen: %w", err)
	}
	return user, true, nil
}
