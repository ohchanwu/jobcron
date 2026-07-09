package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"
)

func TestUserBySessionTokenFindsUnexpiredSessionFromHashOnly(t *testing.T) {
	st := newPostgresTestStore(t)
	ctx := context.Background()
	user, err := st.CreateOwnerUser(ctx, "owner@example.com", "password-hash")
	if err != nil {
		t.Fatalf("CreateOwnerUser: %v", err)
	}

	rawToken := "raw-session-token"
	tokenHash := sessionHashForTest(rawToken)
	expiresAt := time.Now().Add(time.Hour).UTC()
	if err := st.CreateSession(ctx, user.ID, tokenHash, expiresAt); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	var storedHash string
	if err := st.SQLDB().QueryRowContext(ctx, `SELECT session_token_hash FROM sessions WHERE user_id = $1`, user.ID).Scan(&storedHash); err != nil {
		t.Fatalf("query stored session hash: %v", err)
	}
	if storedHash != tokenHash {
		t.Fatalf("stored hash = %q, want %q", storedHash, tokenHash)
	}
	if storedHash == rawToken {
		t.Fatal("stored session token is the raw token, want hash only")
	}

	got, ok, err := st.UserBySessionToken(ctx, rawToken)
	if err != nil {
		t.Fatalf("UserBySessionToken: %v", err)
	}
	if !ok {
		t.Fatal("UserBySessionToken ok=false, want true")
	}
	if got.ID != user.ID || got.Email != user.Email {
		t.Fatalf("user = %+v, want ID=%d email=%s", got, user.ID, user.Email)
	}
}

func TestUserBySessionTokenRejectsExpiredSession(t *testing.T) {
	st := newPostgresTestStore(t)
	ctx := context.Background()
	user, err := st.CreateOwnerUser(ctx, "owner@example.com", "password-hash")
	if err != nil {
		t.Fatalf("CreateOwnerUser: %v", err)
	}
	rawToken := "expired-session-token"
	if err := st.CreateSession(ctx, user.ID, sessionHashForTest(rawToken), time.Now().Add(-time.Minute).UTC()); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	_, ok, err := st.UserBySessionToken(ctx, rawToken)
	if err != nil {
		t.Fatalf("UserBySessionToken: %v", err)
	}
	if ok {
		t.Fatal("expired session accepted")
	}
}

func TestRevokeSessionTokenRemovesMatchingSession(t *testing.T) {
	st := newPostgresTestStore(t)
	ctx := context.Background()
	user, err := st.CreateOwnerUser(ctx, "owner@example.com", "password-hash")
	if err != nil {
		t.Fatalf("CreateOwnerUser: %v", err)
	}
	rawToken := "revoked-session-token"
	if err := st.CreateSession(ctx, user.ID, sessionHashForTest(rawToken), time.Now().Add(time.Hour).UTC()); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	if err := st.RevokeSessionToken(ctx, rawToken); err != nil {
		t.Fatalf("RevokeSessionToken: %v", err)
	}

	_, ok, err := st.UserBySessionToken(ctx, rawToken)
	if err != nil {
		t.Fatalf("UserBySessionToken after revoke: %v", err)
	}
	if ok {
		t.Fatal("revoked session still authenticates")
	}
}

func sessionHashForTest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
