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

type queryExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

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
	if s.dialect == DialectPostgres {
		userID, ok, err := s.firstUserID(ctx)
		if err != nil || !ok {
			return "", false, err
		}
		return s.SaveProfileForUser(ctx, userID, canonicalJSON)
	}
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

	if _, err := s.db.ExecContext(ctx, s.query(`
INSERT INTO profile (id, profile_json, profile_hash, updated_at)
VALUES (1, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    profile_json = excluded.profile_json,
    profile_hash = excluded.profile_hash,
    updated_at   = excluded.updated_at`),
		canonicalJSON, hash, time.Now().UTC()); err != nil {
		return "", false, fmt.Errorf("storage: save profile: %w", err)
	}
	return hash, true, nil
}

// SaveProfileForUser stores the canonical profile JSON for one account.
// SQLite falls back to the legacy single-row profile table while local tests
// and demo mode finish migrating to PostgreSQL-backed user state.
func (s *Store) SaveProfileForUser(ctx context.Context, userID int64, canonicalJSON string) (hash string, changed bool, err error) {
	if s.dialect == DialectSQLite {
		return s.SaveProfile(ctx, canonicalJSON)
	}
	return s.saveProfileForUser(ctx, s.db, userID, canonicalJSON)
}

func (s *Store) saveProfileForUser(
	ctx context.Context,
	db queryExecer,
	userID int64,
	canonicalJSON string,
) (hash string, changed bool, err error) {
	if userID <= 0 {
		return "", false, errors.New("storage: profile user id is required")
	}
	hash = profileHash(canonicalJSON)

	var currentHash string
	err = db.QueryRowContext(ctx, s.query(
		`SELECT profile_hash FROM profiles WHERE user_id = ?`), userID).Scan(&currentHash)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return "", false, fmt.Errorf("storage: read current user profile hash: %w", err)
	case currentHash == hash:
		return hash, false, nil
	}

	if _, err := db.ExecContext(ctx, s.query(`
INSERT INTO profiles (user_id, profile_json, profile_hash, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(user_id) DO UPDATE SET
    profile_json = excluded.profile_json,
    profile_hash = excluded.profile_hash,
    updated_at   = excluded.updated_at`),
		userID, canonicalJSON, hash, time.Now().UTC()); err != nil {
		return "", false, fmt.Errorf("storage: save user profile: %w", err)
	}
	return hash, true, nil
}

// SaveProfileAndCredentialForUser atomically saves one user's profile and,
// when provided, replaces that user's encrypted provider credential. A nil
// credential leaves the existing credential row untouched.
func (s *Store) SaveProfileAndCredentialForUser(
	ctx context.Context,
	userID int64,
	canonicalJSON string,
	encryptedCredential *EncryptedAICredential,
) (hash string, changed bool, err error) {
	if s.dialect != DialectPostgres {
		return "", false, errors.New("storage: profile and AI credential save requires PostgreSQL")
	}
	if userID <= 0 {
		return "", false, errors.New("storage: profile user id is required")
	}

	var credentialToSave *EncryptedAICredential
	if encryptedCredential != nil {
		if encryptedCredential.UserID != userID {
			return "", false, errors.New("storage: credential user does not match profile user")
		}
		normalizedProvider, err := validateEncryptedAICredential(*encryptedCredential)
		if err != nil {
			return "", false, err
		}
		copy := *encryptedCredential
		copy.Provider = normalizedProvider
		credentialToSave = &copy
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", false, fmt.Errorf("storage: begin profile and AI credential save: %w", err)
	}
	defer tx.Rollback()

	hash, changed, err = s.saveProfileForUser(ctx, tx, userID, canonicalJSON)
	if err != nil {
		return "", false, err
	}
	if credentialToSave != nil {
		if err := s.upsertUserAICredential(ctx, tx, *credentialToSave); err != nil {
			return "", false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return "", false, fmt.Errorf("storage: commit profile and AI credential save: %w", err)
	}
	return hash, changed, nil
}

// Profile returns the stored canonical profile JSON and its hash, or
// ok=false when no profile has been saved yet.
func (s *Store) Profile(ctx context.Context) (canonicalJSON, hash string, ok bool, err error) {
	if s.dialect == DialectPostgres {
		userID, found, err := s.firstUserID(ctx)
		if err != nil || !found {
			return "", "", false, err
		}
		return s.ProfileForUser(ctx, userID)
	}
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

// ProfileForUser returns one account's saved profile, or ok=false when that
// account has not saved a profile yet. SQLite falls back to the legacy table.
func (s *Store) ProfileForUser(ctx context.Context, userID int64) (canonicalJSON, hash string, ok bool, err error) {
	if s.dialect == DialectSQLite {
		return s.Profile(ctx)
	}
	if userID <= 0 {
		return "", "", false, errors.New("storage: profile user id is required")
	}
	err = s.db.QueryRowContext(ctx, s.query(
		`SELECT profile_json, profile_hash FROM profiles WHERE user_id = ?`), userID).Scan(&canonicalJSON, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, fmt.Errorf("storage: query user profile: %w", err)
	}
	return canonicalJSON, hash, true, nil
}
