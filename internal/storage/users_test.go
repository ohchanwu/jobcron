package storage

import (
	"context"
	"strings"
	"testing"
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
