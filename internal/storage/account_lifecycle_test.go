package storage

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestChangePasswordUpdatesHashAndKeepsOnlyCurrentSession(t *testing.T) {
	st := newPostgresTestStore(t)
	ctx := context.Background()
	user, err := st.CreateUser(ctx, "member@example.com", "old-hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	for _, hash := range []string{"old-session", "current-session", "other-session"} {
		if err := st.CreateSession(ctx, user.ID, hash, time.Now().Add(time.Hour)); err != nil {
			t.Fatalf("CreateSession(%q): %v", hash, err)
		}
	}

	if err := st.ChangePassword(ctx, user.ID, "new-hash", "current-session"); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	var passwordHash string
	if err := st.db.QueryRowContext(ctx, `SELECT password_hash FROM users WHERE id = $1`, user.ID).Scan(&passwordHash); err != nil {
		t.Fatalf("query password hash: %v", err)
	}
	if passwordHash != "new-hash" {
		t.Fatalf("password hash = %q, want new-hash", passwordHash)
	}
	var sessionHashes []string
	rows, err := st.db.QueryContext(ctx, `SELECT session_token_hash FROM sessions WHERE user_id = $1`, user.ID)
	if err != nil {
		t.Fatalf("query sessions: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var hash string
		if err := rows.Scan(&hash); err != nil {
			t.Fatalf("scan session: %v", err)
		}
		sessionHashes = append(sessionHashes, hash)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate sessions: %v", err)
	}
	if len(sessionHashes) != 1 || sessionHashes[0] != "current-session" {
		t.Fatalf("session hashes = %v, want [current-session]", sessionHashes)
	}
}

func TestChangePasswordRollsBackWhenSessionRevocationFails(t *testing.T) {
	st := newPostgresTestStore(t)
	ctx := context.Background()
	user, err := st.CreateUser(ctx, "member@example.com", "old-hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := st.CreateSession(ctx, user.ID, "current-session", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := st.CreateSession(ctx, user.ID, "other-session", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("CreateSession other: %v", err)
	}
	installRejectingSessionDeleteTrigger(t, st)

	if err := st.ChangePassword(ctx, user.ID, "new-hash", "current-session"); err == nil {
		t.Fatal("ChangePassword error = nil, want session revocation failure")
	}
	assertPasswordHash(t, st, user.ID, "old-hash")
}

func TestResetUserPasswordNormalizesEmailAndRevokesOnlyTargetSessions(t *testing.T) {
	st := newPostgresTestStore(t)
	ctx := context.Background()
	target, err := st.CreateUser(ctx, "member@example.com", "old-hash")
	if err != nil {
		t.Fatalf("CreateUser target: %v", err)
	}
	other, err := st.CreateUser(ctx, "other@example.com", "other-hash")
	if err != nil {
		t.Fatalf("CreateUser other: %v", err)
	}
	for _, session := range []struct {
		userID int64
		hash   string
	}{{target.ID, "target-one"}, {target.ID, "target-two"}, {other.ID, "other-one"}} {
		if err := st.CreateSession(ctx, session.userID, session.hash, time.Now().Add(time.Hour)); err != nil {
			t.Fatalf("CreateSession(%q): %v", session.hash, err)
		}
	}

	updated, err := st.ResetUserPassword(ctx, " MEMBER@EXAMPLE.COM ", "new-hash")
	if err != nil {
		t.Fatalf("ResetUserPassword: %v", err)
	}
	if updated.ID != target.ID || updated.Email != "member@example.com" || updated.PasswordHash != "new-hash" {
		t.Fatalf("updated user = %#v", updated)
	}
	assertRowCount(t, st, "sessions", "user_id", target.ID, 0)
	assertRowCount(t, st, "sessions", "user_id", other.ID, 1)
	assertPasswordHash(t, st, other.ID, "other-hash")
}

func TestResetUserPasswordRollsBackWhenSessionRevocationFails(t *testing.T) {
	st := newPostgresTestStore(t)
	ctx := context.Background()
	user, err := st.CreateUser(ctx, "member@example.com", "old-hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := st.CreateSession(ctx, user.ID, "session", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	installRejectingSessionDeleteTrigger(t, st)

	if _, err := st.ResetUserPassword(ctx, user.Email, "new-hash"); err == nil {
		t.Fatal("ResetUserPassword error = nil, want session revocation failure")
	}
	assertPasswordHash(t, st, user.ID, "old-hash")
}

func TestDeleteUserCascadesPrivateStateAndPreservesSharedState(t *testing.T) {
	st := newPostgresTestStore(t)
	ctx := context.Background()
	target, err := st.CreateUser(ctx, "delete@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser target: %v", err)
	}
	survivor, err := st.CreateUser(ctx, "keep@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser survivor: %v", err)
	}
	postingID, _, err := st.UpsertPosting(ctx, samplePosting())
	if err != nil {
		t.Fatalf("UpsertPosting: %v", err)
	}
	if err := seedAccountLifecycleState(ctx, st, target.ID, postingID); err != nil {
		t.Fatalf("seed target state: %v", err)
	}
	if err := seedAccountLifecycleState(ctx, st, survivor.ID, postingID); err != nil {
		t.Fatalf("seed survivor state: %v", err)
	}
	if err := seedSharedAccountLifecycleState(ctx, st, postingID); err != nil {
		t.Fatalf("seed shared state: %v", err)
	}

	deleted, err := st.DeleteUser(ctx, target.ID+survivor.ID+1000)
	if err != nil || deleted {
		t.Fatalf("DeleteUser missing = deleted:%v err:%v, want false nil", deleted, err)
	}
	deleted, err = st.DeleteUser(ctx, target.ID)
	if err != nil || !deleted {
		t.Fatalf("DeleteUser target = deleted:%v err:%v, want true nil", deleted, err)
	}

	for _, table := range []string{
		"sessions", "profiles", "bookmarks", "not_interested", "scores",
		"ai_scores", "ai_usage", "user_ai_credentials", "local_data_imports",
		"ai_dealbreaker_validations",
	} {
		assertRowCount(t, st, table, "user_id", target.ID, 0)
		assertRowCount(t, st, table, "user_id", survivor.ID, 1)
	}
	assertRowCount(t, st, "users", "id", survivor.ID, 1)
	assertRowCount(t, st, "postings", "id", postingID, 1)
	assertRowCount(t, st, "ai_extractions", "posting_id", postingID, 1)
	assertTotalRowCount(t, st, "scrape_runs", 1)
}

func installRejectingSessionDeleteTrigger(t *testing.T, st *Store) {
	t.Helper()
	if _, err := st.db.Exec(`
CREATE FUNCTION reject_session_delete() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'reject session delete';
END;
$$;
CREATE TRIGGER reject_session_delete
BEFORE DELETE ON sessions
FOR EACH ROW EXECUTE FUNCTION reject_session_delete()`); err != nil {
		t.Fatalf("install rejecting session trigger: %v", err)
	}
}

func assertPasswordHash(t *testing.T, st *Store, userID int64, want string) {
	t.Helper()
	var got string
	if err := st.db.QueryRow(`SELECT password_hash FROM users WHERE id = $1`, userID).Scan(&got); err != nil {
		t.Fatalf("query password hash: %v", err)
	}
	if got != want {
		t.Fatalf("password hash = %q, want %q", got, want)
	}
}

func assertRowCount(t *testing.T, st *Store, table, column string, value int64, want int) {
	t.Helper()
	var got int
	query := fmt.Sprintf("SELECT count(*) FROM %s WHERE %s = $1", table, column)
	if err := st.db.QueryRow(query, value).Scan(&got); err != nil {
		t.Fatalf("count %s rows: %v", table, err)
	}
	if got != want {
		t.Fatalf("%s rows = %d, want %d", table, got, want)
	}
}

func assertTotalRowCount(t *testing.T, st *Store, table string, want int) {
	t.Helper()
	var got int
	if err := st.db.QueryRow("SELECT count(*) FROM " + table).Scan(&got); err != nil {
		t.Fatalf("count %s rows: %v", table, err)
	}
	if got != want {
		t.Fatalf("%s rows = %d, want %d", table, got, want)
	}
}

func seedAccountLifecycleState(ctx context.Context, st *Store, userID, postingID int64) error {
	if err := st.CreateSession(ctx, userID, fmt.Sprintf("delete-session-%d", userID), time.Now().Add(time.Hour)); err != nil {
		return err
	}
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO profiles (user_id, profile_json, profile_hash, updated_at) VALUES ($1, '{}', 'profile', now())`, []any{userID}},
		{`INSERT INTO bookmarks (user_id, posting_id, bookmarked_at) VALUES ($1, $2, now())`, []any{userID, postingID}},
		{`INSERT INTO not_interested (user_id, posting_id, muted_at) VALUES ($1, $2, now())`, []any{userID, postingID}},
		{`INSERT INTO scores (user_id, posting_id, profile_hash, total, breakdown_json, computed_at) VALUES ($1, $2, 'profile', 50, '{}', now())`, []any{userID, postingID}},
		{`INSERT INTO ai_scores (user_id, posting_id, ai_input_hash, ai_version, items_json, net_delta, computed_at) VALUES ($1, $2, 'input', 'version', '[]', 0, now())`, []any{userID, postingID}},
		{`INSERT INTO ai_usage (user_id, day, input_tokens, output_tokens) VALUES ($1, CURRENT_DATE, 1, 1)`, []any{userID}},
		{`INSERT INTO user_ai_credentials (user_id, provider, ciphertext, nonce, encryption_version) VALUES ($1, 'anthropic', $2, $3, 1)`, []any{userID, bytes.Repeat([]byte{1}, 17), bytes.Repeat([]byte{2}, 12)}},
		{`INSERT INTO local_data_imports (user_id, source_sha256, source_counts, imported_counts) VALUES ($1, $2, '{}'::jsonb, '{}'::jsonb)`, []any{userID, strings.Repeat("a", 64)}},
		{`INSERT INTO ai_dealbreaker_validations (user_id, posting_id, content_hash, ai_version, keyword_hash, verdict, evidence, computed_at) VALUES ($1, $2, 'content', 'version', 'keyword', 'applies', 'evidence', now())`, []any{userID, postingID}},
	}
	for _, statement := range statements {
		if _, err := st.db.ExecContext(ctx, statement.query, statement.args...); err != nil {
			return err
		}
	}
	return nil
}

func seedSharedAccountLifecycleState(ctx context.Context, st *Store, postingID int64) error {
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO ai_extractions (posting_id, content_hash, ai_version, min_career, newcomer, education_enum, career_evidence, education_evidence, computed_at) VALUES ($1, 'content', 'version', 0, true, 'none', 'career', 'education', now())`, []any{postingID}},
		{`INSERT INTO scrape_runs (trigger, status, started_at) VALUES ('manual', 'completed', now())`, nil},
	}
	for _, statement := range statements {
		if _, err := st.db.ExecContext(ctx, statement.query, statement.args...); err != nil {
			return err
		}
	}
	return nil
}
