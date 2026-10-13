package storage

import (
	"context"
	"testing"
	"time"

	"github.com/ohchanwu/jobcron/internal/auth"
)

func TestSessionIdentityReadOnly(t *testing.T) {
	forEachAccountStore(t, func(t *testing.T, st *Store) {
		ctx := context.Background()
		u, err := st.CreateUser(ctx, "observer@example.invalid", "synthetic-hash")
		if err != nil {
			t.Fatal(err)
		}
		// PostgreSQL stores microseconds; keep exact equality with a representable fixture.
		expiry := time.Now().UTC().Truncate(time.Second).Add(time.Hour + 123456*time.Microsecond)
		for _, token := range []string{"valid", "expired", "revoked"} {
			ex := expiry
			if token == "expired" {
				ex = time.Now().UTC().Add(-time.Hour)
			}
			if err := st.CreateSession(ctx, u.ID, auth.HashSessionToken(token), ex); err != nil {
				t.Fatal(err)
			}
		}
		if err := st.RevokeSessionToken(ctx, "revoked"); err != nil {
			t.Fatal(err)
		}
		var before time.Time
		if err := st.db.QueryRowContext(ctx, st.query(`SELECT last_seen_at FROM sessions WHERE session_token_hash=?`), auth.HashSessionToken("valid")).Scan(&before); err != nil {
			t.Fatal(err)
		}
		got, ok, err := st.SessionIdentityReadOnly(ctx, "valid")
		if err != nil || !ok || got.UserID != u.ID || !got.ExpiresAt.Equal(expiry) {
			t.Fatalf("identity=%+v ok=%v err=%v", got, ok, err)
		}
		var after time.Time
		if err := st.db.QueryRowContext(ctx, st.query(`SELECT last_seen_at FROM sessions WHERE session_token_hash=?`), auth.HashSessionToken("valid")).Scan(&after); err != nil {
			t.Fatal(err)
		}
		if !before.Equal(after) {
			t.Fatal("observation touched session activity")
		}
		for _, token := range []string{"", "expired", "revoked", "unknown"} {
			if _, ok, err := st.SessionIdentityReadOnly(ctx, token); err != nil || ok {
				t.Fatalf("accepted %q: %v %v", token, ok, err)
			}
		}
		if _, err := st.db.ExecContext(ctx, st.query(`DELETE FROM users WHERE id=?`), u.ID); err != nil {
			t.Fatal(err)
		}
		if _, ok, err := st.SessionIdentityReadOnly(ctx, "valid"); err != nil || ok {
			t.Fatalf("deleted user accepted: %v %v", ok, err)
		}
	})
}

func TestHasUserAICredentialReadsPresenceOnly(t *testing.T) {
	st := newPostgresTestStore(t)
	ctx := context.Background()
	u, err := st.CreateUser(ctx, "presence@example.invalid", "synthetic-hash")
	if err != nil {
		t.Fatal(err)
	}
	other, err := st.CreateUser(ctx, "other@example.invalid", "synthetic-hash")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.ExecContext(ctx, `DROP TABLE user_ai_credentials; CREATE TABLE user_ai_credentials(user_id bigint, provider text)`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.ExecContext(ctx, `INSERT INTO user_ai_credentials VALUES ($1,'gemini')`, u.ID); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		user     int64
		provider string
		want     bool
	}{{u.ID, "gemini", true}, {u.ID, "anthropic", false}, {other.ID, "gemini", false}} {
		got, err := st.HasUserAICredential(ctx, tc.user, tc.provider)
		if err != nil || got != tc.want {
			t.Fatalf("presence=%v err=%v want=%v", got, err, tc.want)
		}
	}
	if _, err := st.db.ExecContext(ctx, `DROP TABLE user_ai_credentials`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.HasUserAICredential(ctx, u.ID, "gemini"); err == nil {
		t.Fatal("query failure guessed AI off")
	}
}
