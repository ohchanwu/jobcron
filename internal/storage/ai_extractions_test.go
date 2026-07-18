package storage

import (
	"context"
	"testing"
	"time"

	"github.com/ohchanwu/jobcron/internal/ai"
)

func ptr(n int) *int { return &n }

func TestMigration0008AppliesTo8(t *testing.T) {
	st := newTestStore(t)
	var v int
	if err := st.db.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	// Latest migration is 0012 (scrape run history); 0006 is intentionally
	// skipped. Bump this when a new migration lands.
	if v != 12 {
		t.Fatalf("user_version = %d, want 12 after 0012 applies (0006 skipped)", v)
	}
}

func TestUpsertAIExtractionRoundTrip(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	id, _, err := st.UpsertPosting(ctx, samplePosting())
	if err != nil {
		t.Fatalf("UpsertPosting: %v", err)
	}
	const ver = "abc123def456"
	now := time.Date(2026, 6, 2, 10, 0, 0, 0, time.UTC)

	t.Run("open upper bound (nil max)", func(t *testing.T) {
		ext := ai.Extraction{MinCareer: 3, MaxCareer: nil, Newcomer: false, EducationEnum: ai.EduBachelor, CareerEvidence: "경력 3년 이상", EducationEvidence: "학사 이상"}
		if err := st.UpsertAIExtraction(ctx, id, "hashA", ver, ext, now); err != nil {
			t.Fatalf("UpsertAIExtraction: %v", err)
		}
		got, ok, err := st.AIExtraction(ctx, id, "hashA", ver)
		if err != nil || !ok {
			t.Fatalf("AIExtraction: ok=%v err=%v", ok, err)
		}
		if got.MinCareer != 3 || got.MaxCareer != nil || got.Newcomer || got.EducationEnum != ai.EduBachelor || got.CareerEvidence != "경력 3년 이상" || got.EducationEvidence != "" {
			t.Fatalf("round trip = %+v", got)
		}
	})

	t.Run("bounded max round-trips", func(t *testing.T) {
		ext := ai.Extraction{MinCareer: 0, MaxCareer: ptr(0), Newcomer: true, EducationEnum: ai.EduNone, CareerEvidence: "신입"}
		if err := st.UpsertAIExtraction(ctx, id, "hashB", ver, ext, now); err != nil {
			t.Fatalf("UpsertAIExtraction: %v", err)
		}
		got, ok, err := st.AIExtraction(ctx, id, "hashB", ver)
		if err != nil || !ok {
			t.Fatalf("AIExtraction: ok=%v err=%v", ok, err)
		}
		if got.MaxCareer == nil || *got.MaxCareer != 0 || !got.Newcomer {
			t.Fatalf("bounded max round trip = %+v", got)
		}
	})

	t.Run("PK conflict updates in place", func(t *testing.T) {
		ext := ai.Extraction{MinCareer: 9, MaxCareer: ptr(9), Newcomer: false, EducationEnum: ai.EduMaster, CareerEvidence: "v2"}
		if err := st.UpsertAIExtraction(ctx, id, "hashA", ver, ext, now.Add(time.Hour)); err != nil {
			t.Fatalf("re-upsert: %v", err)
		}
		got, _, _ := st.AIExtraction(ctx, id, "hashA", ver)
		if got.MinCareer != 9 || got.EducationEnum != ai.EduMaster {
			t.Fatalf("conflict update = %+v, want updated values", got)
		}
		var n int
		st.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM ai_extractions WHERE posting_id=? AND content_hash='hashA' AND ai_version=?`, id, ver).Scan(&n)
		if n != 1 {
			t.Fatalf("conflict produced %d rows, want 1 (upsert, not duplicate)", n)
		}
	})

	t.Run("miss returns ok=false", func(t *testing.T) {
		_, ok, err := st.AIExtraction(ctx, id, "no-such-hash", ver)
		if err != nil || ok {
			t.Fatalf("miss: ok=%v err=%v", ok, err)
		}
	})
}

func TestAIExtractionsByPostingIDBatchedAndLatest(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	const ver = "ver000000001"

	p1 := samplePosting()
	id1, _, _ := st.UpsertPosting(ctx, p1)
	p2 := samplePosting()
	p2.SourcePostingID = "999"
	id2, _, _ := st.UpsertPosting(ctx, p2)

	t0 := time.Date(2026, 6, 2, 9, 0, 0, 0, time.UTC)
	// id1 has two content_hash rows under the same version; the newer wins.
	if err := st.UpsertAIExtraction(ctx, id1, "old", ver, ai.Extraction{MinCareer: 5, EducationEnum: ai.EduBachelor, CareerEvidence: "old"}, t0); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertAIExtraction(ctx, id1, "new", ver, ai.Extraction{MinCareer: 0, Newcomer: true, EducationEnum: ai.EduNone, CareerEvidence: "new"}, t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertAIExtraction(ctx, id2, "h", ver, ai.Extraction{MinCareer: 2, EducationEnum: ai.EduAssociate, CareerEvidence: "two"}, t0); err != nil {
		t.Fatal(err)
	}
	// A row under a DIFFERENT version must not leak into this version's read.
	if err := st.UpsertAIExtraction(ctx, id2, "h", "otherversion", ai.Extraction{MinCareer: 40, EducationEnum: ai.EduDoctorate, CareerEvidence: "wrong"}, t0); err != nil {
		t.Fatal(err)
	}

	m, err := st.AIExtractionsByPostingID(ctx, ver)
	if err != nil {
		t.Fatalf("AIExtractionsByPostingID: %v", err)
	}
	if len(m) != 2 {
		t.Fatalf("map has %d entries, want 2", len(m))
	}
	if m[id1].CareerEvidence != "new" || !m[id1].Newcomer {
		t.Fatalf("id1 = %+v, want the newer (computed_at DESC) row", m[id1])
	}
	if m[id2].MinCareer != 2 || m[id2].EducationEnum != ai.EduAssociate {
		t.Fatalf("id2 = %+v, want this-version row, not the otherversion one", m[id2])
	}
}

func TestAIExtractionCascadeOnPostingDelete(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	const ver = "cascadetest1"

	t.Run("direct delete cascades", func(t *testing.T) {
		id, _, _ := st.UpsertPosting(ctx, samplePosting())
		if err := st.UpsertAIExtraction(ctx, id, "h", ver, ai.Extraction{Newcomer: true, EducationEnum: ai.EduNone}, time.Now()); err != nil {
			t.Fatal(err)
		}
		if _, err := st.db.ExecContext(ctx, `DELETE FROM postings WHERE id = ?`, id); err != nil {
			t.Fatalf("delete posting: %v", err)
		}
		var n int
		st.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM ai_extractions WHERE posting_id = ?`, id).Scan(&n)
		if n != 0 {
			t.Fatalf("extraction row outlived its posting (%d rows) — ON DELETE CASCADE not engaged", n)
		}
	})

	t.Run("sweep cascades", func(t *testing.T) {
		// A posting last seen long before the per-source baseline gets swept;
		// its extraction must go with it.
		p := samplePosting()
		p.SourcePostingID = "sweepme"
		p.FirstSeenAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		p.LastSeenAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		id, _, _ := st.UpsertPosting(ctx, p)
		// A fresh posting moves the source baseline far ahead of the stale one.
		fresh := samplePosting()
		fresh.SourcePostingID = "freshrow"
		fresh.LastSeenAt = time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)
		st.UpsertPosting(ctx, fresh)
		if err := st.UpsertAIExtraction(ctx, id, "h", ver, ai.Extraction{EducationEnum: ai.EduNone}, time.Now()); err != nil {
			t.Fatal(err)
		}
		now := time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)
		if _, err := st.SweepStalePostings(ctx, now, 3*24*time.Hour, 90*24*time.Hour, nil); err != nil {
			t.Fatalf("sweep: %v", err)
		}
		var n int
		st.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM ai_extractions WHERE posting_id = ?`, id).Scan(&n)
		if n != 0 {
			t.Fatalf("extraction survived the sweep of its posting (%d rows)", n)
		}
	})
}
