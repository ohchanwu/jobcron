package storage

import (
	"bytes"
	"context"
	"testing"
)

func TestSaveProfileAndCredentialForUserCommitsBoth(t *testing.T) {
	st := newPostgresTestStore(t)
	ctx := context.Background()
	userID := insertCredentialTestUser(t, st, "atomic-commit")
	credential := validEncryptedAICredential(userID, " ANTHROPIC ", 0x31)
	profileJSON := `{"stacks":["Go"],"ai_provider":"anthropic"}`

	hash, changed, err := st.SaveProfileAndCredentialForUser(ctx, userID, profileJSON, &credential)
	if err != nil {
		t.Fatalf("SaveProfileAndCredentialForUser: %v", err)
	}
	if !changed || hash != profileHash(profileJSON) {
		t.Fatalf("save result = hash %q changed=%v, want hash %q changed=true", hash, changed, profileHash(profileJSON))
	}
	gotProfile, gotHash, found, err := st.ProfileForUser(ctx, userID)
	if err != nil || !found || gotProfile != profileJSON || gotHash != hash {
		t.Fatalf("saved profile = json %q hash %q found=%v err=%v", gotProfile, gotHash, found, err)
	}
	gotCredential, found, err := st.UserAICredential(ctx, userID, "anthropic")
	if err != nil || !found {
		t.Fatalf("saved credential found=%v err=%v", found, err)
	}
	assertEncryptedCredentialPayload(t, gotCredential, credential)
}

func TestSaveProfileAndCredentialForUserBlankCredentialKeepsExisting(t *testing.T) {
	st := newPostgresTestStore(t)
	ctx := context.Background()
	userID := insertCredentialTestUser(t, st, "atomic-blank")
	existing := validEncryptedAICredential(userID, "anthropic", 0x42)
	if err := st.UpsertUserAICredential(ctx, existing); err != nil {
		t.Fatalf("seed credential: %v", err)
	}
	before, found, err := st.UserAICredential(ctx, userID, "anthropic")
	if err != nil || !found {
		t.Fatalf("read seed credential: found=%v err=%v", found, err)
	}

	if _, _, err := st.SaveProfileAndCredentialForUser(ctx, userID, `{"stacks":["Rust"]}`, nil); err != nil {
		t.Fatalf("SaveProfileAndCredentialForUser with nil credential: %v", err)
	}
	after, found, err := st.UserAICredential(ctx, userID, "anthropic")
	if err != nil || !found {
		t.Fatalf("read credential after profile-only save: found=%v err=%v", found, err)
	}
	assertEncryptedCredentialPayload(t, after, before)
}

func TestSaveProfileAndCredentialForUserRollsBackBothOnProfileFailure(t *testing.T) {
	st := newPostgresTestStore(t)
	ctx := context.Background()
	userID := insertCredentialTestUser(t, st, "atomic-profile-failure")
	seedProfile := `{"stacks":["Go"]}`
	if _, _, err := st.SaveProfileForUser(ctx, userID, seedProfile); err != nil {
		t.Fatalf("seed profile: %v", err)
	}
	seedCredential := validEncryptedAICredential(userID, "anthropic", 0x51)
	if err := st.UpsertUserAICredential(ctx, seedCredential); err != nil {
		t.Fatalf("seed credential: %v", err)
	}
	beforeProfile, beforeHash, beforeCredential := readAtomicSaveState(t, st, userID)
	installRejectingWriteTrigger(t, st, "profiles", "reject_profile_write")

	replacement := validEncryptedAICredential(userID, "anthropic", 0x52)
	if _, _, err := st.SaveProfileAndCredentialForUser(ctx, userID, `{"stacks":["Python"]}`, &replacement); err == nil {
		t.Fatal("SaveProfileAndCredentialForUser succeeded despite profile trigger failure")
	}
	assertAtomicSaveState(t, st, userID, beforeProfile, beforeHash, beforeCredential)
}

func TestSaveProfileAndCredentialForUserRollsBackBothOnCredentialFailure(t *testing.T) {
	st := newPostgresTestStore(t)
	ctx := context.Background()
	userID := insertCredentialTestUser(t, st, "atomic-credential-failure")
	seedProfile := `{"stacks":["Go"]}`
	if _, _, err := st.SaveProfileForUser(ctx, userID, seedProfile); err != nil {
		t.Fatalf("seed profile: %v", err)
	}
	seedCredential := validEncryptedAICredential(userID, "anthropic", 0x61)
	if err := st.UpsertUserAICredential(ctx, seedCredential); err != nil {
		t.Fatalf("seed credential: %v", err)
	}
	beforeProfile, beforeHash, beforeCredential := readAtomicSaveState(t, st, userID)
	installRejectingWriteTrigger(t, st, "user_ai_credentials", "reject_credential_write")

	replacement := validEncryptedAICredential(userID, "anthropic", 0x62)
	if _, _, err := st.SaveProfileAndCredentialForUser(ctx, userID, `{"stacks":["Python"]}`, &replacement); err == nil {
		t.Fatal("SaveProfileAndCredentialForUser succeeded despite credential trigger failure")
	}
	assertAtomicSaveState(t, st, userID, beforeProfile, beforeHash, beforeCredential)
}

func installRejectingWriteTrigger(t *testing.T, st *Store, table, function string) {
	t.Helper()
	if _, err := st.db.Exec(`
CREATE FUNCTION ` + function + `() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'synthetic write failure';
END;
$$;
CREATE TRIGGER ` + function + `_trigger
BEFORE INSERT OR UPDATE ON ` + table + `
FOR EACH ROW EXECUTE FUNCTION ` + function + `()`); err != nil {
		t.Fatalf("install rejecting trigger on %s: %v", table, err)
	}
}

func readAtomicSaveState(t *testing.T, st *Store, userID int64) (string, string, EncryptedAICredential) {
	t.Helper()
	profileJSON, hash, found, err := st.ProfileForUser(context.Background(), userID)
	if err != nil || !found {
		t.Fatalf("read profile state: found=%v err=%v", found, err)
	}
	credential, found, err := st.UserAICredential(context.Background(), userID, "anthropic")
	if err != nil || !found {
		t.Fatalf("read credential state: found=%v err=%v", found, err)
	}
	return profileJSON, hash, credential
}

func assertAtomicSaveState(
	t *testing.T,
	st *Store,
	userID int64,
	wantProfile, wantHash string,
	wantCredential EncryptedAICredential,
) {
	t.Helper()
	gotProfile, gotHash, gotCredential := readAtomicSaveState(t, st, userID)
	if gotProfile != wantProfile || gotHash != wantHash {
		t.Fatalf("profile after rollback = json %q hash %q, want json %q hash %q", gotProfile, gotHash, wantProfile, wantHash)
	}
	assertEncryptedCredentialPayload(t, gotCredential, wantCredential)
}

func assertEncryptedCredentialPayload(t *testing.T, got, want EncryptedAICredential) {
	t.Helper()
	if got.UserID != want.UserID || got.Provider != "anthropic" ||
		!bytes.Equal(got.Ciphertext, want.Ciphertext) ||
		!bytes.Equal(got.Nonce, want.Nonce) ||
		got.EncryptionVersion != want.EncryptionVersion {
		t.Fatalf("credential payload mismatch: user=%d provider=%q ciphertext_equal=%v ciphertext_len=%d nonce_equal=%v nonce_len=%d version=%d",
			got.UserID, got.Provider, bytes.Equal(got.Ciphertext, want.Ciphertext), len(got.Ciphertext),
			bytes.Equal(got.Nonce, want.Nonce), len(got.Nonce), got.EncryptionVersion)
	}
}
