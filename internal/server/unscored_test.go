package server

import (
	"context"
	"testing"
	"time"

	"github.com/ohchanwu/jobcron/internal/profile"
)

// TestBuildBriefingSkipsUnscoredPosting locks in Bug 2B: a posting that exists
// but has no score row (an interrupted scrape inserted it before scoreAll ran)
// must NOT render as a blank card — no total, no 신입 chip. It is skipped until
// a later scrape / profile-save scores it, then it appears normally.
func TestBuildBriefingSkipsUnscoredPosting(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	ctx := context.Background()
	profJSON, _ := profile.Marshal(profile.Profile{})
	if _, _, err := st.SaveProfile(ctx, profJSON); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	now := time.Now().UTC()
	p := listingPosting("unscored", "점수 없는 신입 공고")
	p.FirstSeenAt, p.LastSeenAt = now, now
	id := mustUpsert(t, st, p)

	// No score row yet → must be skipped from both the main list and the
	// 관심 밖 excluded list (a blank card is the bug).
	b, err := srv.buildBriefing(ctx, now)
	if err != nil {
		t.Fatalf("buildBriefing: %v", err)
	}
	if contains(b.Today, p.Title) {
		t.Fatalf("unscored posting rendered in Today as a blank card; want it skipped")
	}
	if contains(b.Excluded, p.Title) {
		t.Fatalf("unscored posting rendered in the 관심 밖 list; want it skipped")
	}

	// Once scored, it appears in the briefing as normal.
	scoreEach(t, st, map[int64]int{id: 50})
	b, err = srv.buildBriefing(ctx, now)
	if err != nil {
		t.Fatalf("buildBriefing: %v", err)
	}
	if !contains(b.Today, p.Title) {
		t.Fatalf("scored posting missing from briefing; want it shown")
	}
}

// TestRescoreAllHealsUnscoredPosting locks in the startup heal (Fix for the
// review's Finding 1): a posting committed but never scored — e.g. a process
// crash between UpsertPosting and the end-of-run scoreAll, which the briefing
// skip and Fix 2A do NOT cover — is scored on the next boot by RescoreAll, so
// it renders normally instead of vanishing or showing a blank card.
func TestRescoreAllHealsUnscoredPosting(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	ctx := context.Background()
	profJSON, _ := profile.Marshal(profile.Profile{})
	if _, _, err := st.SaveProfile(ctx, profJSON); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	now := time.Now().UTC()
	p := listingPosting("orphan", "복구된 신입 공고")
	p.FirstSeenAt, p.LastSeenAt = now, now
	id := mustUpsert(t, st, p)

	scores, err := st.ScoresByPostingID(ctx)
	if err != nil {
		t.Fatalf("ScoresByPostingID: %v", err)
	}
	if _, ok := scores[id]; ok {
		t.Fatal("precondition failed: posting should start unscored")
	}

	// Startup heal — main calls this right after ReconfigureAI.
	if _, err := srv.RescoreAll(ctx); err != nil {
		t.Fatalf("RescoreAll: %v", err)
	}

	scores, err = st.ScoresByPostingID(ctx)
	if err != nil {
		t.Fatalf("ScoresByPostingID: %v", err)
	}
	if _, ok := scores[id]; !ok {
		t.Fatal("RescoreAll left the crash-orphaned posting unscored")
	}
	b, err := srv.buildBriefing(ctx, now)
	if err != nil {
		t.Fatalf("buildBriefing: %v", err)
	}
	if !contains(b.Today, p.Title) && !contains(b.Excluded, p.Title) {
		t.Fatal("healed posting still not rendered after RescoreAll")
	}
}
