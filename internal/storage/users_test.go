package storage

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ohchanwu/jobcron/internal/auth"
)

func TestPostgresCreateUserWithSessionIsAtomic(t *testing.T) {
	st := newPostgresTestStore(t)
	ctx := context.Background()
	expiresAt := time.Now().Add(time.Hour)

	rawToken := "token-one"
	user, err := st.CreateUserWithSession(ctx, " First@Example.COM ", "hash-one", auth.HashSessionToken(rawToken), expiresAt)
	if err != nil {
		t.Fatalf("CreateUserWithSession: %v", err)
	}
	if user.Email != "first@example.com" {
		t.Fatalf("created email = %q, want canonical email", user.Email)
	}
	if _, ok, err := st.UserBySessionToken(ctx, rawToken); err != nil || !ok {
		t.Fatalf("UserBySessionToken: ok=%v err=%v", ok, err)
	}
	if _, err := st.CreateUserWithSession(ctx, "first@example.com", "hash-two", "token-two", expiresAt); !errors.Is(err, ErrEmailAlreadyExists) {
		t.Fatalf("duplicate error = %v, want ErrEmailAlreadyExists", err)
	}

	if _, err := st.SQLDB().ExecContext(ctx, `
CREATE FUNCTION fail_signup_session() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'forced session failure';
END
$$;
CREATE TRIGGER fail_signup_session BEFORE INSERT ON sessions
FOR EACH ROW EXECUTE FUNCTION fail_signup_session()`); err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}
	if _, err := st.CreateUserWithSession(ctx, "rollback@example.com", "hash-three", "token-three", expiresAt); err == nil {
		t.Fatal("CreateUserWithSession error = nil, want forced session failure")
	}
	if _, ok, err := st.UserByEmail(ctx, "rollback@example.com"); err != nil || ok {
		t.Fatalf("rolled-back user lookup: ok=%v err=%v", ok, err)
	}
}

func TestCreateOwnerUserCreatesOnlyOwner(t *testing.T) {
	st := newPostgresTestStore(t)
	ctx := context.Background()

	user, err := st.CreateOwnerUser(ctx, "owner@example.com", "hash-one")
	if err != nil {
		t.Fatalf("CreateOwnerUser: %v", err)
	}
	if user.ID == 0 {
		t.Fatal("User.ID = 0, want persisted id")
	}
	if user.Email != "owner@example.com" {
		t.Fatalf("User.Email = %q", user.Email)
	}
	if user.PasswordHash != "hash-one" {
		t.Fatalf("User.PasswordHash = %q", user.PasswordHash)
	}
	if user.CreatedAt.IsZero() || user.UpdatedAt.IsZero() {
		t.Fatalf("timestamps not populated: created=%v updated=%v", user.CreatedAt, user.UpdatedAt)
	}

	_, err = st.CreateOwnerUser(ctx, "second@example.com", "hash-two")
	if err == nil {
		t.Fatal("second CreateOwnerUser error = nil, want owner-exists error")
	}
	if !strings.Contains(err.Error(), "owner user already exists") {
		t.Fatalf("second CreateOwnerUser error = %v", err)
	}
}

func TestCreateUserAllowsMultipleAccountsAndLookups(t *testing.T) {
	st := newPostgresTestStore(t)
	ctx := context.Background()

	first, err := st.CreateUser(ctx, " First@Example.COM ", "hash-one")
	if err != nil {
		t.Fatalf("first CreateUser: %v", err)
	}
	second, err := st.CreateUser(ctx, "second@example.com", "hash-two")
	if err != nil {
		t.Fatalf("second CreateUser: %v", err)
	}
	if first.Email != "first@example.com" || second.Email != "second@example.com" {
		t.Fatalf("created emails = %q, %q", first.Email, second.Email)
	}
	if first.ID <= 0 || second.ID <= first.ID {
		t.Fatalf("created IDs = %d, %d, want ascending positive IDs", first.ID, second.ID)
	}

	byEmail, ok, err := st.UserByEmail(ctx, "  FIRST@EXAMPLE.COM ")
	if err != nil || !ok || byEmail.ID != first.ID {
		t.Fatalf("UserByEmail = ID %d ok %v err %v, want ID %d", byEmail.ID, ok, err, first.ID)
	}
	byID, ok, err := st.UserByID(ctx, second.ID)
	if err != nil || !ok || byID.Email != second.Email {
		t.Fatalf("UserByID = email %q ok %v err %v, want %q", byID.Email, ok, err, second.Email)
	}
	if _, err := st.SQLDB().ExecContext(ctx, `
INSERT INTO users (id, email, password_hash) VALUES (0, 'zero@example.com', 'hash-zero')`); err != nil {
		t.Fatalf("seed zero-ID user: %v", err)
	}
	ids, err := st.UserIDs(ctx)
	if err != nil {
		t.Fatalf("UserIDs: %v", err)
	}
	if len(ids) != 2 || ids[0] != first.ID || ids[1] != second.ID {
		t.Fatalf("UserIDs = %v, want [%d %d]", ids, first.ID, second.ID)
	}
}

func TestCreateUserRejectsNormalizedDuplicate(t *testing.T) {
	st := newPostgresTestStore(t)
	ctx := context.Background()
	if _, err := st.CreateUser(ctx, " Student@Example.COM ", "hash-one"); err != nil {
		t.Fatalf("first CreateUser: %v", err)
	}
	if _, err := st.CreateUser(ctx, "student@example.com", "hash-two"); !errors.Is(err, ErrEmailAlreadyExists) {
		t.Fatalf("duplicate CreateUser error = %v, want ErrEmailAlreadyExists", err)
	}
	var count int
	if err := st.SQLDB().QueryRowContext(ctx, `SELECT count(*) FROM users WHERE email = 'student@example.com'`).Scan(&count); err != nil {
		t.Fatalf("count normalized users: %v", err)
	}
	if count != 1 {
		t.Fatalf("normalized user rows = %d, want 1", count)
	}
}

func TestCreateUserRejectsMissingFields(t *testing.T) {
	st := newPostgresTestStore(t)
	ctx := context.Background()
	for _, tc := range []struct {
		name        string
		email, hash string
	}{
		{name: "empty email", hash: "hash"},
		{name: "whitespace email", email: "   ", hash: "hash"},
		{name: "empty password hash", email: "user@example.com"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := st.CreateUser(ctx, tc.email, tc.hash); err == nil {
				t.Fatalf("CreateUser(%q, %q) error = nil", tc.email, tc.hash)
			}
		})
	}
	var count int
	if err := st.SQLDB().QueryRowContext(ctx, `SELECT count(*) FROM users`).Scan(&count); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if count != 0 {
		t.Fatalf("users after rejected creates = %d, want 0", count)
	}
}

func TestCreateUserConcurrentNormalizedDuplicateCreatesOneRow(t *testing.T) {
	st := newPostgresTestStore(t)
	ctx := context.Background()
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, email := range []string{" Student@Example.COM ", "student@example.com"} {
		go func() {
			<-start
			_, err := st.CreateUser(ctx, email, "hash")
			results <- err
		}()
	}
	close(start)

	var successes, duplicates int
	for range 2 {
		switch err := <-results; {
		case err == nil:
			successes++
		case errors.Is(err, ErrEmailAlreadyExists):
			duplicates++
		default:
			t.Fatalf("concurrent CreateUser error = %v", err)
		}
	}
	if successes != 1 || duplicates != 1 {
		t.Fatalf("concurrent results = %d success, %d duplicate", successes, duplicates)
	}
	var count int
	if err := st.SQLDB().QueryRowContext(ctx, `SELECT count(*) FROM users WHERE email = 'student@example.com'`).Scan(&count); err != nil {
		t.Fatalf("count concurrent users: %v", err)
	}
	if count != 1 {
		t.Fatalf("concurrent user rows = %d, want 1", count)
	}
}

func TestCreateOwnerUserRejectsExistingGeneralAccount(t *testing.T) {
	st := newPostgresTestStore(t)
	ctx := context.Background()
	if _, err := st.CreateUser(ctx, "member@example.com", "member-hash"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if _, err := st.CreateOwnerUser(ctx, "owner@example.com", "owner-hash"); err == nil || !strings.Contains(err.Error(), "owner user already exists") {
		t.Fatalf("CreateOwnerUser error = %v, want existing-user refusal", err)
	}
}

func TestUserLookupsReturnNotFoundForInvalidAndMissingKeys(t *testing.T) {
	st := newPostgresTestStore(t)
	ctx := context.Background()
	for _, email := range []string{"", "   ", "missing@example.com"} {
		if user, ok, err := st.UserByEmail(ctx, email); err != nil || ok || user.ID != 0 {
			t.Fatalf("UserByEmail(%q) = user %+v ok %v err %v", email, user, ok, err)
		}
	}
	for _, id := range []int64{0, -1, 1} {
		if user, ok, err := st.UserByID(ctx, id); err != nil || ok || user.ID != 0 {
			t.Fatalf("UserByID(%d) = user %+v ok %v err %v", id, user, ok, err)
		}
	}
}

func TestSoleOwnerUserIDRequiresExactOwnership(t *testing.T) {
	st := newPostgresTestStore(t)
	ctx := context.Background()
	if id, ok, err := st.SoleOwnerUserID(ctx); err != nil || ok || id != 0 {
		t.Fatalf("zero users = id %d ok %v err %v, want 0 false nil", id, ok, err)
	}
	owner, err := st.CreateOwnerUser(ctx, "sole-owner@example.invalid", "synthetic-hash")
	if err != nil {
		t.Fatalf("CreateOwnerUser: %v", err)
	}
	if id, ok, err := st.SoleOwnerUserID(ctx); err != nil || !ok || id != owner.ID {
		t.Fatalf("one user = id %d ok %v err %v, want %d true nil", id, ok, err, owner.ID)
	}
	if _, err := st.SQLDB().ExecContext(ctx, `
INSERT INTO users (email, password_hash, created_at, updated_at)
VALUES ('second-owner@example.invalid', 'synthetic-hash', now(), now())`); err != nil {
		t.Fatalf("insert second user: %v", err)
	}
	if id, ok, err := st.SoleOwnerUserID(ctx); err == nil || ok || id != 0 || !strings.Contains(err.Error(), "multiple users") {
		t.Fatalf("multiple users = id %d ok %v err %v, want stable ambiguity error", id, ok, err)
	}
}

func TestEnsureManagedLocalOwnerCreatesAndReusesSyntheticPositiveUser(t *testing.T) {
	st := newPostgresTestStore(t)
	ctx := context.Background()

	firstID, err := st.EnsureManagedLocalOwner(ctx)
	if err != nil {
		t.Fatalf("first EnsureManagedLocalOwner: %v", err)
	}
	secondID, err := st.EnsureManagedLocalOwner(ctx)
	if err != nil {
		t.Fatalf("second EnsureManagedLocalOwner: %v", err)
	}
	if firstID <= 0 || secondID != firstID {
		t.Fatalf("managed local IDs = %d, %d; want stable positive ID", firstID, secondID)
	}
	user, ok, err := st.UserByEmail(ctx, managedLocalOwnerEmail)
	if err != nil || !ok {
		t.Fatalf("UserByEmail managed owner = ok %v err %v", ok, err)
	}
	if user.ID != firstID || !strings.HasSuffix(user.Email, ".example.invalid") {
		t.Fatalf("managed owner = ID %d email %q, want ID %d and reserved address", user.ID, user.Email, firstID)
	}
	if user.PasswordHash != managedLocalOwnerPasswordHash {
		t.Fatal("managed owner password hash is not the fixed unusable value")
	}
	if matches, err := auth.VerifyPassword(user.PasswordHash, "any-password"); err == nil || matches {
		t.Fatalf("managed owner password hash verified: matches %v err present %v, want unusable hash", matches, err != nil)
	}
}

func TestManagedLocalStartupReusesImportedOwner(t *testing.T) {
	st := newPostgresTestStore(t)
	ctx := context.Background()
	created, err := st.CreateOwnerUser(ctx, "imported-owner@example.invalid", "imported-password-hash")
	if err != nil {
		t.Fatalf("CreateOwnerUser: %v", err)
	}
	resolvedID, err := st.EnsureManagedLocalOwner(ctx)
	if err != nil {
		t.Fatalf("EnsureManagedLocalOwner: %v", err)
	}
	if resolvedID != created.ID || resolvedID <= 0 {
		t.Fatalf("resolved ID = %d, want existing positive ID %d", resolvedID, created.ID)
	}
	user, ok, err := st.UserByEmail(ctx, created.Email)
	if err != nil || !ok {
		t.Fatalf("UserByEmail existing owner = ok %v err %v", ok, err)
	}
	if user.PasswordHash != created.PasswordHash {
		t.Fatal("EnsureManagedLocalOwner changed the existing owner's password hash")
	}
}

func TestEnsureManagedLocalOwnerConcurrentCallersReuseOneUser(t *testing.T) {
	st := newPostgresTestStore(t)
	ctx := context.Background()
	start := make(chan struct{})
	type result struct {
		id  int64
		err error
	}
	results := make(chan result, 2)
	for range 2 {
		go func() {
			<-start
			id, err := st.EnsureManagedLocalOwner(ctx)
			results <- result{id: id, err: err}
		}()
	}
	close(start)
	first, second := <-results, <-results
	if first.err != nil || second.err != nil {
		t.Fatalf("concurrent EnsureManagedLocalOwner errors = %v, %v", first.err, second.err)
	}
	if first.id <= 0 || second.id != first.id {
		t.Fatalf("concurrent managed local IDs = %d, %d; want one positive ID", first.id, second.id)
	}
	var count int
	if err := st.SQLDB().QueryRowContext(ctx, `SELECT count(*) FROM users WHERE email = $1`, managedLocalOwnerEmail).Scan(&count); err != nil {
		t.Fatalf("count managed local owners: %v", err)
	}
	if count != 1 {
		t.Fatalf("managed local owner rows = %d, want exactly 1", count)
	}
}

func TestEnsureManagedLocalOwnerRefusesMultipleExistingUsers(t *testing.T) {
	st := newPostgresTestStore(t)
	ctx := context.Background()
	if _, err := st.SQLDB().ExecContext(ctx, `
INSERT INTO users (email, password_hash, created_at, updated_at)
VALUES ('first@example.invalid', 'synthetic-hash', now(), now()),
       ('second@example.invalid', 'synthetic-hash', now(), now())`); err != nil {
		t.Fatalf("insert multiple users: %v", err)
	}
	if _, err := st.EnsureManagedLocalOwner(ctx); err == nil || !strings.Contains(err.Error(), "multiple users") {
		t.Fatalf("EnsureManagedLocalOwner error = %v, want multiple-user refusal", err)
	}
}

func TestResetOwnerPasswordUpdatesExistingOwner(t *testing.T) {
	st := newPostgresTestStore(t)
	ctx := context.Background()

	created, err := st.CreateOwnerUser(ctx, "owner@example.com", "old-hash")
	if err != nil {
		t.Fatalf("CreateOwnerUser: %v", err)
	}

	updated, err := st.ResetOwnerPassword(ctx, " OWNER@EXAMPLE.COM ", "new-hash")
	if err != nil {
		t.Fatalf("ResetOwnerPassword: %v", err)
	}
	if updated.ID != created.ID {
		t.Fatalf("updated ID = %d, want %d", updated.ID, created.ID)
	}
	if updated.PasswordHash != "new-hash" {
		t.Fatalf("updated password hash = %q", updated.PasswordHash)
	}
}

func TestResetOwnerPasswordRejectsWrongEmailWithoutRenamingOwner(t *testing.T) {
	st := newPostgresTestStore(t)
	ctx := context.Background()

	created, err := st.CreateOwnerUser(ctx, "owner@example.com", "old-hash")
	if err != nil {
		t.Fatalf("CreateOwnerUser: %v", err)
	}

	_, err = st.ResetOwnerPassword(ctx, "wrong@example.com", "new-hash")
	if err == nil {
		t.Fatal("ResetOwnerPassword error = nil, want email mismatch error")
	}
	if !strings.Contains(err.Error(), "owner user does not match email") {
		t.Fatalf("ResetOwnerPassword error = %v", err)
	}

	var email, passwordHash string
	if err := st.SQLDB().QueryRowContext(ctx, `SELECT email, password_hash FROM users WHERE id = $1`, created.ID).
		Scan(&email, &passwordHash); err != nil {
		t.Fatalf("query owner after failed reset: %v", err)
	}
	if email != "owner@example.com" {
		t.Fatalf("owner email = %q, want unchanged owner@example.com", email)
	}
	if passwordHash != "old-hash" {
		t.Fatalf("owner password hash = %q, want unchanged old-hash", passwordHash)
	}
}
