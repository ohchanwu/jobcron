package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const ownerRole = "owner"

// User is an application account row. Milestone A only creates one owner.
type User struct {
	ID           int64
	Email        string
	PasswordHash string
	Role         string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// CreateOwnerUser creates the first and only owner account for the production
// app. It fails once any user already exists.
func (s *Store) CreateOwnerUser(ctx context.Context, email, passwordHash string) (User, error) {
	if email == "" {
		return User{}, errors.New("storage: owner email is required")
	}
	if passwordHash == "" {
		return User{}, errors.New("storage: owner password hash is required")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return User{}, fmt.Errorf("storage: begin owner creation: %w", err)
	}
	defer tx.Rollback()

	if err := s.lockUsersForOwnerChange(ctx, tx); err != nil {
		return User{}, err
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM users`).Scan(&count); err != nil {
		return User{}, fmt.Errorf("storage: count owner users: %w", err)
	}
	if count > 0 {
		return User{}, errors.New("storage: owner user already exists")
	}

	row := tx.QueryRowContext(ctx, `
INSERT INTO users (email, password_hash, created_at, updated_at)
VALUES ($1, $2, now(), now())
RETURNING id, email, password_hash, created_at, updated_at`, email, passwordHash)
	user, err := scanOwnerUser(row)
	if err != nil {
		return User{}, fmt.Errorf("storage: create owner user: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return User{}, fmt.Errorf("storage: commit owner creation: %w", err)
	}
	return user, nil
}

// ResetOwnerPassword replaces the password hash for the sole owner account.
func (s *Store) ResetOwnerPassword(ctx context.Context, email, passwordHash string) (User, error) {
	if email == "" {
		return User{}, errors.New("storage: owner email is required")
	}
	if passwordHash == "" {
		return User{}, errors.New("storage: owner password hash is required")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return User{}, fmt.Errorf("storage: begin owner password reset: %w", err)
	}
	defer tx.Rollback()

	if err := s.lockUsersForOwnerChange(ctx, tx); err != nil {
		return User{}, err
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM users`).Scan(&count); err != nil {
		return User{}, fmt.Errorf("storage: count owner users: %w", err)
	}
	if count == 0 {
		return User{}, errors.New("storage: owner user does not exist")
	}
	if count > 1 {
		return User{}, errors.New("storage: multiple users exist; refusing owner password reset")
	}

	row := tx.QueryRowContext(ctx, `
UPDATE users
   SET password_hash = $2,
       updated_at = now()
 WHERE email = $1
 RETURNING id, email, password_hash, created_at, updated_at`, email, passwordHash)
	user, err := scanOwnerUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, errors.New("storage: owner user does not match email")
	}
	if err != nil {
		return User{}, fmt.Errorf("storage: reset owner password: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return User{}, fmt.Errorf("storage: commit owner password reset: %w", err)
	}
	return user, nil
}

func (s *Store) lockUsersForOwnerChange(ctx context.Context, tx *sql.Tx) error {
	if s.dialect == DialectPostgres {
		if _, err := tx.ExecContext(ctx, `LOCK TABLE users IN EXCLUSIVE MODE`); err != nil {
			return fmt.Errorf("storage: lock users table: %w", err)
		}
	}
	return nil
}

func scanOwnerUser(row rowScanner) (User, error) {
	var user User
	if err := row.Scan(&user.ID, &user.Email, &user.PasswordHash, &user.CreatedAt, &user.UpdatedAt); err != nil {
		return User{}, err
	}
	user.Role = ownerRole
	return user, nil
}
