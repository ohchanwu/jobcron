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

	now := time.Now().UTC()
	row := tx.QueryRowContext(ctx, s.query(`
INSERT INTO users (email, password_hash, created_at, updated_at)
VALUES (?, ?, ?, ?)
RETURNING id, email, password_hash, created_at, updated_at`), email, passwordHash, now, now)
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

	now := time.Now().UTC()
	row := tx.QueryRowContext(ctx, s.query(`
UPDATE users
   SET password_hash = ?,
       updated_at = ?
 WHERE email = ?
 RETURNING id, email, password_hash, created_at, updated_at`), passwordHash, now, email)
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

// UserByEmail returns an application user by email address.
func (s *Store) UserByEmail(ctx context.Context, email string) (User, bool, error) {
	if email == "" {
		return User{}, false, nil
	}
	row := s.db.QueryRowContext(ctx, s.query(`
SELECT id, email, password_hash, created_at, updated_at
  FROM users
 WHERE email = ?`), email)
	user, err := scanOwnerUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, fmt.Errorf("storage: user by email: %w", err)
	}
	return user, true, nil
}

// firstUserID is a transitional compatibility helper for legacy no-user
// storage wrappers. Production request paths should pass the authenticated
// user id and avoid this fallback.
func (s *Store) firstUserID(ctx context.Context) (int64, bool, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM users ORDER BY id LIMIT 1`).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("storage: first user id: %w", err)
	}
	return id, true, nil
}

func scanOwnerUser(row rowScanner) (User, error) {
	var user User
	if err := row.Scan(&user.ID, &user.Email, &user.PasswordHash, &user.CreatedAt, &user.UpdatedAt); err != nil {
		return User{}, err
	}
	user.Role = ownerRole
	return user, nil
}
