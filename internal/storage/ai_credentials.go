package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ohchanwu/jobcron/internal/credential"
)

const (
	credentialNonceBytes  = 12
	credentialGCMTagBytes = 16
)

// EncryptedAICredential is the ciphertext-only persistence shape for one
// user's provider credential. Plaintext deliberately has no storage field.
type EncryptedAICredential struct {
	UserID            int64
	Provider          string
	Ciphertext        []byte
	Nonce             []byte
	EncryptionVersion int16
	UpdatedAt         time.Time
}

// UpsertUserAICredential inserts or replaces one encrypted user/provider row.
func (s *Store) UpsertUserAICredential(ctx context.Context, c EncryptedAICredential) error {
	normalizedProvider, err := validateEncryptedAICredential(c)
	if err != nil {
		return err
	}
	if err := s.requirePostgresCredentials(); err != nil {
		return err
	}
	c.Provider = normalizedProvider
	return s.upsertUserAICredential(ctx, s.db, c)
}

func validateEncryptedAICredential(c EncryptedAICredential) (string, error) {
	normalizedProvider, err := validateCredentialKey(c.UserID, c.Provider)
	if err != nil {
		return "", err
	}
	if len(c.Ciphertext) <= credentialGCMTagBytes {
		return "", errors.New("storage: credential ciphertext is too short")
	}
	if len(c.Nonce) != credentialNonceBytes {
		return "", errors.New("storage: credential nonce must be 12 bytes")
	}
	if c.EncryptionVersion <= 0 {
		return "", errors.New("storage: credential encryption version must be positive")
	}
	return normalizedProvider, nil
}

func (s *Store) upsertUserAICredential(ctx context.Context, db queryExecer, c EncryptedAICredential) error {
	_, err := db.ExecContext(ctx, s.query(`
INSERT INTO user_ai_credentials (
    user_id, provider, ciphertext, nonce, encryption_version, created_at, updated_at
)
VALUES (?, ?, ?, ?, ?, now(), now())
ON CONFLICT (user_id, provider) DO UPDATE SET
    ciphertext = EXCLUDED.ciphertext,
    nonce = EXCLUDED.nonce,
    encryption_version = EXCLUDED.encryption_version,
    updated_at = now()`),
		c.UserID,
		c.Provider,
		c.Ciphertext,
		c.Nonce,
		c.EncryptionVersion,
	)
	if err != nil {
		return fmt.Errorf("storage: upsert user AI credential: %w", err)
	}
	return nil
}

// UserAICredential returns one encrypted user/provider row.
func (s *Store) UserAICredential(ctx context.Context, userID int64, provider string) (EncryptedAICredential, bool, error) {
	normalizedProvider, err := validateCredentialKey(userID, provider)
	if err != nil {
		return EncryptedAICredential{}, false, err
	}
	if err := s.requirePostgresCredentials(); err != nil {
		return EncryptedAICredential{}, false, err
	}

	var got EncryptedAICredential
	err = s.db.QueryRowContext(ctx, s.query(`
SELECT user_id, provider, ciphertext, nonce, encryption_version, updated_at
  FROM user_ai_credentials
 WHERE user_id = ? AND provider = ?`), userID, normalizedProvider).Scan(
		&got.UserID,
		&got.Provider,
		&got.Ciphertext,
		&got.Nonce,
		&got.EncryptionVersion,
		&got.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return EncryptedAICredential{}, false, nil
	}
	if err != nil {
		return EncryptedAICredential{}, false, fmt.Errorf("storage: read user AI credential: %w", err)
	}
	return got, true, nil
}

// DeleteUserAICredential removes one user/provider row. Deleting a missing row
// is intentionally idempotent.
func (s *Store) DeleteUserAICredential(ctx context.Context, userID int64, provider string) error {
	normalizedProvider, err := validateCredentialKey(userID, provider)
	if err != nil {
		return err
	}
	if err := s.requirePostgresCredentials(); err != nil {
		return err
	}

	if _, err := s.db.ExecContext(ctx, s.query(`
DELETE FROM user_ai_credentials
 WHERE user_id = ? AND provider = ?`), userID, normalizedProvider); err != nil {
		return fmt.Errorf("storage: delete user AI credential: %w", err)
	}
	return nil
}

func validateCredentialKey(userID int64, provider string) (string, error) {
	if userID <= 0 {
		return "", errors.New("storage: credential user ID must be positive")
	}
	normalizedProvider, err := credential.NormalizeProvider(provider)
	if err != nil {
		return "", errors.New("storage: credential provider is invalid")
	}
	return normalizedProvider, nil
}

func (s *Store) requirePostgresCredentials() error {
	if s.dialect != DialectPostgres {
		return errors.New("storage: user AI credentials require PostgreSQL")
	}
	return nil
}
