package storage

import (
	"context"
	"strings"
	"testing"

	"github.com/ohchanwu/jobcron/internal/auth"
)

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
	if user.Role != "owner" {
		t.Fatalf("User.Role = %q, want owner", user.Role)
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

	updated, err := st.ResetOwnerPassword(ctx, "owner@example.com", "new-hash")
	if err != nil {
		t.Fatalf("ResetOwnerPassword: %v", err)
	}
	if updated.ID != created.ID {
		t.Fatalf("updated ID = %d, want %d", updated.ID, created.ID)
	}
	if updated.PasswordHash != "new-hash" {
		t.Fatalf("updated password hash = %q", updated.PasswordHash)
	}
	if updated.Role != "owner" {
		t.Fatalf("updated role = %q, want owner", updated.Role)
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
