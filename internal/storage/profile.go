package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// profileHash is the design's profile hash: the first 12 hex characters of
// sha256(canonicalJSON).
func profileHash(canonicalJSON string) string {
	sum := sha256.Sum256([]byte(canonicalJSON))
	return hex.EncodeToString(sum[:])[:12]
}

// SaveProfile stores the canonical profile JSON in the single-row profile
// table, computing its hash (sha256(canonical_json)[:12]).
//
// The write is skipped when the new hash matches the stored one, so a no-op
// save neither bumps updated_at nor invalidates existing scores. It returns
// the hash and whether the stored profile actually changed.
func (s *Store) SaveProfile(ctx context.Context, canonicalJSON string) (hash string, changed bool, err error) {
	hash = profileHash(canonicalJSON)

	var currentHash string
	err = s.db.QueryRowContext(ctx,
		`SELECT profile_hash FROM profile WHERE id = 1`).Scan(&currentHash)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		// No profile saved yet — fall through to the write.
	case err != nil:
		return "", false, fmt.Errorf("storage: read current profile hash: %w", err)
	case currentHash == hash:
		return hash, false, nil
	}

	if _, err := s.db.ExecContext(ctx, `
INSERT INTO profile (id, profile_json, profile_hash, updated_at)
VALUES (1, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    profile_json = excluded.profile_json,
    profile_hash = excluded.profile_hash,
    updated_at   = excluded.updated_at`,
		canonicalJSON, hash, time.Now().UTC()); err != nil {
		return "", false, fmt.Errorf("storage: save profile: %w", err)
	}
	return hash, true, nil
}

// Profile returns the stored canonical profile JSON and its hash, or
// ok=false when no profile has been saved yet.
func (s *Store) Profile(ctx context.Context) (canonicalJSON, hash string, ok bool, err error) {
	err = s.db.QueryRowContext(ctx,
		`SELECT profile_json, profile_hash FROM profile WHERE id = 1`).Scan(&canonicalJSON, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, fmt.Errorf("storage: query profile: %w", err)
	}
	return canonicalJSON, hash, true, nil
}
