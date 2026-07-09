package storage

import (
	"context"
	"testing"
	"time"
)

func TestProfileForUserIsolatesProfiles(t *testing.T) {
	st := newPostgresTestStore(t)
	ctx := context.Background()
	userA := insertTestUser(t, st, "a@example.com")
	userB := insertTestUser(t, st, "b@example.com")

	if _, _, err := st.SaveProfileForUser(ctx, userA, `{"stacks":["Go"]}`); err != nil {
		t.Fatalf("SaveProfileForUser userA: %v", err)
	}
	if _, _, err := st.SaveProfileForUser(ctx, userB, `{"stacks":["React"]}`); err != nil {
		t.Fatalf("SaveProfileForUser userB: %v", err)
	}

	gotA, _, ok, err := st.ProfileForUser(ctx, userA)
	if err != nil || !ok {
		t.Fatalf("ProfileForUser userA: ok=%v err=%v", ok, err)
	}
	gotB, _, ok, err := st.ProfileForUser(ctx, userB)
	if err != nil || !ok {
		t.Fatalf("ProfileForUser userB: ok=%v err=%v", ok, err)
	}
	if gotA != `{"stacks":["Go"]}` {
		t.Fatalf("userA profile = %s", gotA)
	}
	if gotB != `{"stacks":["React"]}` {
		t.Fatalf("userB profile = %s", gotB)
	}
}

func TestBookmarkStateIsolatesUsers(t *testing.T) {
	st := newPostgresTestStore(t)
	ctx := context.Background()
	userA := insertTestUser(t, st, "a@example.com")
	userB := insertTestUser(t, st, "b@example.com")
	postA := insertDistinctPosting(t, st, "a")
	postB := insertDistinctPosting(t, st, "b")

	if err := st.AddBookmark(ctx, userA, postA); err != nil {
		t.Fatalf("AddBookmark userA: %v", err)
	}
	if err := st.AddBookmark(ctx, userB, postB); err != nil {
		t.Fatalf("AddBookmark userB: %v", err)
	}

	gotA, err := st.BookmarkedIDsForUser(ctx, userA)
	if err != nil {
		t.Fatalf("BookmarkedIDsForUser userA: %v", err)
	}
	gotB, err := st.BookmarkedIDsForUser(ctx, userB)
	if err != nil {
		t.Fatalf("BookmarkedIDsForUser userB: %v", err)
	}
	if !gotA[postA] || gotA[postB] {
		t.Fatalf("userA bookmarks = %v, want only %d", gotA, postA)
	}
	if !gotB[postB] || gotB[postA] {
		t.Fatalf("userB bookmarks = %v, want only %d", gotB, postB)
	}
}

func TestNotInterestedStateIsolatesUsers(t *testing.T) {
	st := newPostgresTestStore(t)
	ctx := context.Background()
	userA := insertTestUser(t, st, "a@example.com")
	userB := insertTestUser(t, st, "b@example.com")
	postA := insertDistinctPosting(t, st, "a")
	postB := insertDistinctPosting(t, st, "b")

	at := time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)
	if err := st.AddNotInterested(ctx, userA, postA, at); err != nil {
		t.Fatalf("AddNotInterested userA: %v", err)
	}
	if err := st.AddNotInterested(ctx, userB, postB, at.Add(time.Hour)); err != nil {
		t.Fatalf("AddNotInterested userB: %v", err)
	}

	gotA, err := st.NotInterestedIDs(ctx, userA)
	if err != nil {
		t.Fatalf("NotInterestedIDs userA: %v", err)
	}
	gotB, err := st.NotInterestedIDs(ctx, userB)
	if err != nil {
		t.Fatalf("NotInterestedIDs userB: %v", err)
	}
	if !gotA[postA] || gotA[postB] {
		t.Fatalf("userA not_interested = %v, want only %d", gotA, postA)
	}
	if !gotB[postB] || gotB[postA] {
		t.Fatalf("userB not_interested = %v, want only %d", gotB, postB)
	}
}

func TestScoreStateIsolatesUsers(t *testing.T) {
	st := newPostgresTestStore(t)
	ctx := context.Background()
	userA := insertTestUser(t, st, "a@example.com")
	userB := insertTestUser(t, st, "b@example.com")
	postingID := insertDistinctPosting(t, st, "shared")

	if err := st.UpsertScoreForUser(ctx, userA, Score{
		PostingID: postingID, ProfileHash: "hash-a", Total: 80,
		BreakdownJSON: `[]`, ComputedAt: time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("UpsertScoreForUser userA: %v", err)
	}
	if err := st.UpsertScoreForUser(ctx, userB, Score{
		PostingID: postingID, ProfileHash: "hash-b", Total: 20,
		BreakdownJSON: `[]`, ComputedAt: time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("UpsertScoreForUser userB: %v", err)
	}

	gotA, err := st.ScoresByPostingID(ctx, userA)
	if err != nil {
		t.Fatalf("ScoresByPostingID userA: %v", err)
	}
	gotB, err := st.ScoresByPostingID(ctx, userB)
	if err != nil {
		t.Fatalf("ScoresByPostingID userB: %v", err)
	}
	if gotA[postingID].Total != 80 || gotA[postingID].ProfileHash != "hash-a" {
		t.Fatalf("userA score = %+v, want total 80/hash-a", gotA[postingID])
	}
	if gotB[postingID].Total != 20 || gotB[postingID].ProfileHash != "hash-b" {
		t.Fatalf("userB score = %+v, want total 20/hash-b", gotB[postingID])
	}
}

func insertTestUser(t *testing.T, st *Store, email string) int64 {
	t.Helper()
	var id int64
	if err := st.db.QueryRowContext(context.Background(), st.query(`
INSERT INTO users (email, password_hash, created_at, updated_at)
VALUES (?, ?, ?, ?)
RETURNING id`), email, "hash", time.Now().UTC(), time.Now().UTC()).Scan(&id); err != nil {
		t.Fatalf("insert test user %s: %v", email, err)
	}
	return id
}

func insertDistinctPosting(t *testing.T, st *Store, suffix string) int64 {
	t.Helper()
	p := samplePosting()
	p.SourcePostingID = "posting-" + suffix
	p.URL = "https://example.test/" + suffix
	id, _, err := st.UpsertPosting(context.Background(), p)
	if err != nil {
		t.Fatalf("UpsertPosting %s: %v", suffix, err)
	}
	return id
}
