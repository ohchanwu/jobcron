package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/ohchanwu/job-scraper/internal/scraper"
)

// requiredObjects are the tables / virtual tables migration 0001 must create.
var requiredObjects = []string{"postings", "postings_fts", "profile", "scores"}

// newTestStore opens a fresh, migrated database in a temp directory.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := OpenAt(filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// samplePosting is a fully-populated posting used across storage tests.
func samplePosting() scraper.Posting {
	pub := time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)
	closed := time.Date(2026, 5, 30, 23, 59, 59, 0, time.UTC)
	seen := time.Date(2026, 5, 21, 8, 0, 0, 0, time.UTC)
	edu := 4
	return scraper.Posting{
		Source:          "jumpit",
		SourcePostingID: "53688979",
		URL:             "https://jumpit.saramin.co.kr/position/53688979",
		Title:           "B2B 프로젝트 개발팀 신입",
		Company:         "에스피에이치",
		Location:        "서울 마포구",
		Newcomer:        true,
		MinCareer:       0,
		MaxCareer:       0,
		CareerLevel:     "신입",
		Education:       &edu,
		EducationName:   "대학교졸업(4년) 이상",
		StackTags:       []string{"Git", "Java", "React", "AI/인공지능"},
		Tags:            []scraper.Tag{{ID: "com_143", Name: "평균연봉 6,000 이상", Category: "salary"}},
		Description:     "서버 개발자를 찾습니다\n\n복지: 재택 가능",
		RawJSON:         `{"id":53688979}`,
		PublishedAt:     &pub,
		ClosedAt:        &closed,
		AlwaysOpen:      false,
		FirstSeenAt:     seen,
		LastSeenAt:      seen,
	}
}

// assertPosting compares two postings, using time.Equal for time fields
// (reflect.DeepEqual is fragile on time.Time across a DB round-trip).
func assertPosting(t *testing.T, got, want scraper.Posting) {
	t.Helper()
	eqPtr := func(name string, a, b *time.Time) {
		switch {
		case a == nil && b == nil:
		case a == nil || b == nil, !a.Equal(*b):
			t.Errorf("%s: got %v, want %v", name, a, b)
		}
	}
	eqPtr("PublishedAt", got.PublishedAt, want.PublishedAt)
	eqPtr("ClosedAt", got.ClosedAt, want.ClosedAt)
	if !got.FirstSeenAt.Equal(want.FirstSeenAt) {
		t.Errorf("FirstSeenAt: got %v, want %v", got.FirstSeenAt, want.FirstSeenAt)
	}
	if !got.LastSeenAt.Equal(want.LastSeenAt) {
		t.Errorf("LastSeenAt: got %v, want %v", got.LastSeenAt, want.LastSeenAt)
	}
	got.PublishedAt, want.PublishedAt = nil, nil
	got.ClosedAt, want.ClosedAt = nil, nil
	got.FirstSeenAt, want.FirstSeenAt = time.Time{}, time.Time{}
	got.LastSeenAt, want.LastSeenAt = time.Time{}, time.Time{}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("non-time fields mismatch:\n got = %+v\nwant = %+v", got, want)
	}
}

func TestOpenAtAppliesSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs.db")

	st, err := OpenAt(path)
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	for _, name := range requiredObjects {
		var got string
		err := st.db.QueryRow(
			`SELECT name FROM sqlite_master WHERE name = ?`, name).Scan(&got)
		if err != nil {
			t.Errorf("object %q not found after OpenAt: %v", name, err)
		}
	}
	if err := st.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestOpenAtIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs.db")

	st1, err := OpenAt(path)
	if err != nil {
		t.Fatalf("first OpenAt: %v", err)
	}
	if err := st1.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}

	// Re-opening an already-migrated database must succeed without
	// re-applying migration 0001.
	st2, err := OpenAt(path)
	if err != nil {
		t.Fatalf("second OpenAt: %v", err)
	}
	defer st2.Close()
}

func TestUpsertPostingInsertsNewPosting(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	in := samplePosting()

	id, isNew, err := st.UpsertPosting(ctx, in)
	if err != nil {
		t.Fatalf("UpsertPosting: %v", err)
	}
	if !isNew {
		t.Errorf("isNew = false, want true for a brand-new posting")
	}
	if id <= 0 {
		t.Fatalf("id = %d, want a positive row id", id)
	}

	got, ok, err := st.PostingByID(ctx, id)
	if err != nil {
		t.Fatalf("PostingByID: %v", err)
	}
	if !ok {
		t.Fatalf("PostingByID: posting %d not found", id)
	}

	want := in
	want.ID = id
	assertPosting(t, got, want)
}

func TestUpsertPostingUpdatesLastSeenOnConflict(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	first := samplePosting()
	id1, isNew, err := st.UpsertPosting(ctx, first)
	if err != nil || !isNew {
		t.Fatalf("first UpsertPosting: id=%d isNew=%v err=%v", id1, isNew, err)
	}

	// The same posting (same source + source_posting_id) seen again on a
	// later scrape run, carrying a later last_seen_at.
	again := samplePosting()
	later := first.LastSeenAt.Add(48 * time.Hour)
	again.FirstSeenAt = later
	again.LastSeenAt = later

	id2, isNew, err := st.UpsertPosting(ctx, again)
	if err != nil {
		t.Fatalf("second UpsertPosting: %v", err)
	}
	if isNew {
		t.Errorf("isNew = true, want false for an already-seen posting")
	}
	if id2 != id1 {
		t.Errorf("id = %d, want %d (same posting, not a duplicate)", id2, id1)
	}

	got, ok, err := st.PostingByID(ctx, id1)
	if err != nil || !ok {
		t.Fatalf("PostingByID: ok=%v err=%v", ok, err)
	}
	if !got.LastSeenAt.Equal(later) {
		t.Errorf("LastSeenAt = %v, want %v (must advance)", got.LastSeenAt, later)
	}
	if !got.FirstSeenAt.Equal(first.FirstSeenAt) {
		t.Errorf("FirstSeenAt = %v, want %v (must not change)", got.FirstSeenAt, first.FirstSeenAt)
	}

	var count int
	if err := st.db.QueryRow(`SELECT count(*) FROM postings`).Scan(&count); err != nil {
		t.Fatalf("count postings: %v", err)
	}
	if count != 1 {
		t.Errorf("posting count = %d, want 1 (conflict updated, not duplicated)", count)
	}
}

func TestUpsertScoreInsertsScore(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	postingID, _, err := st.UpsertPosting(ctx, samplePosting())
	if err != nil {
		t.Fatalf("UpsertPosting: %v", err)
	}

	in := Score{
		PostingID:     postingID,
		ProfileHash:   "abc123def456",
		Total:         85,
		BreakdownJSON: `[{"label":"React","delta":20}]`,
		ComputedAt:    time.Date(2026, 5, 21, 8, 30, 0, 0, time.UTC),
	}
	if err := st.UpsertScore(ctx, in); err != nil {
		t.Fatalf("UpsertScore: %v", err)
	}

	got, ok, err := st.ScoreByPostingID(ctx, postingID)
	if err != nil {
		t.Fatalf("ScoreByPostingID: %v", err)
	}
	if !ok {
		t.Fatalf("ScoreByPostingID: score for posting %d not found", postingID)
	}
	if got.PostingID != in.PostingID || got.ProfileHash != in.ProfileHash ||
		got.Total != in.Total || got.BreakdownJSON != in.BreakdownJSON {
		t.Errorf("score mismatch:\n got = %+v\nwant = %+v", got, in)
	}
	if !got.ComputedAt.Equal(in.ComputedAt) {
		t.Errorf("ComputedAt = %v, want %v", got.ComputedAt, in.ComputedAt)
	}
}

func TestUpsertScoreOverwritesExisting(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	postingID, _, err := st.UpsertPosting(ctx, samplePosting())
	if err != nil {
		t.Fatalf("UpsertPosting: %v", err)
	}

	base := Score{
		PostingID: postingID, ProfileHash: "hash-one-000", Total: 40,
		BreakdownJSON: `[]`, ComputedAt: time.Date(2026, 5, 21, 8, 0, 0, 0, time.UTC),
	}
	if err := st.UpsertScore(ctx, base); err != nil {
		t.Fatalf("first UpsertScore: %v", err)
	}

	// The posting is re-scored against a different profile.
	updated := Score{
		PostingID: postingID, ProfileHash: "hash-two-000", Total: 90,
		BreakdownJSON: `[{"label":"TS","delta":10}]`,
		ComputedAt:    time.Date(2026, 5, 22, 9, 0, 0, 0, time.UTC),
	}
	if err := st.UpsertScore(ctx, updated); err != nil {
		t.Fatalf("second UpsertScore: %v", err)
	}

	got, ok, err := st.ScoreByPostingID(ctx, postingID)
	if err != nil || !ok {
		t.Fatalf("ScoreByPostingID: ok=%v err=%v", ok, err)
	}
	if got.Total != 90 || got.ProfileHash != "hash-two-000" {
		t.Errorf("score not overwritten: got total=%d hash=%s, want 90/hash-two-000",
			got.Total, got.ProfileHash)
	}

	var count int
	if err := st.db.QueryRow(`SELECT count(*) FROM scores`).Scan(&count); err != nil {
		t.Fatalf("count scores: %v", err)
	}
	if count != 1 {
		t.Errorf("score count = %d, want 1 (one score per posting)", count)
	}
}

func TestSaveProfileStoresProfileWithHash(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	const profileJSON = `{"career":"신입","stacks":[{"name":"React","weight":20}]}`

	hash, changed, err := st.SaveProfile(ctx, profileJSON)
	if err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	if !changed {
		t.Errorf("changed = false, want true for the first save")
	}
	sum := sha256.Sum256([]byte(profileJSON))
	wantHash := hex.EncodeToString(sum[:])[:12]
	if hash != wantHash {
		t.Errorf("hash = %q, want %q (sha256(canonical_json)[:12])", hash, wantHash)
	}

	gotJSON, gotHash, ok, err := st.Profile(ctx)
	if err != nil {
		t.Fatalf("Profile: %v", err)
	}
	if !ok {
		t.Fatalf("Profile: ok = false, want a saved profile")
	}
	if gotJSON != profileJSON {
		t.Errorf("profile JSON = %q, want %q", gotJSON, profileJSON)
	}
	if gotHash != hash {
		t.Errorf("profile hash = %q, want %q", gotHash, hash)
	}
}

func TestSaveProfileWritesOnlyWhenContentChanges(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	const v1 = `{"career":"신입","stacks":[{"name":"React","weight":20}]}`
	const v2 = `{"career":"신입","stacks":[{"name":"Go","weight":30}]}`

	h1, _, err := st.SaveProfile(ctx, v1)
	if err != nil {
		t.Fatalf("save v1: %v", err)
	}

	// Re-saving identical content is a no-op: changed=false keeps scores valid.
	h1again, changed, err := st.SaveProfile(ctx, v1)
	if err != nil {
		t.Fatalf("re-save v1: %v", err)
	}
	if changed {
		t.Errorf("changed = true on identical re-save, want false")
	}
	if h1again != h1 {
		t.Errorf("hash = %q on identical re-save, want %q", h1again, h1)
	}

	// Saving different content persists the change and reports changed=true.
	h2, changed, err := st.SaveProfile(ctx, v2)
	if err != nil {
		t.Fatalf("save v2: %v", err)
	}
	if !changed {
		t.Errorf("changed = false after a real content change, want true")
	}
	if h2 == h1 {
		t.Errorf("hash unchanged (%q) after content change, want a different hash", h2)
	}

	gotJSON, gotHash, ok, err := st.Profile(ctx)
	if err != nil || !ok {
		t.Fatalf("Profile: ok=%v err=%v", ok, err)
	}
	if gotJSON != v2 || gotHash != h2 {
		t.Errorf("stored profile = (%q,%q), want (%q,%q)", gotJSON, gotHash, v2, h2)
	}
}
