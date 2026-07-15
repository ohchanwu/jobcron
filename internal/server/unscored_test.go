package server

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ohchanwu/jobcron/internal/credential"
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

	// Startup heal — main calls this after resolving the sole owner.
	if _, err := srv.RescoreAll(ctx, 0, nil); err != nil {
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

func TestRescoreSoleOwnerUsesLegacySQLiteProfileWithoutAuthUser(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	ctx := context.Background()
	profJSON, _ := profile.Marshal(profile.Profile{CareerYears: 0})
	if _, _, err := st.SaveProfile(ctx, profJSON); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	p := listingPosting("sqlite-startup-heal", "SQLite 시작 복구 공고")
	p.FirstSeenAt, p.LastSeenAt = time.Now().UTC(), time.Now().UTC()
	id := mustUpsert(t, st, p)

	if _, err := srv.RescoreSoleOwner(ctx); err != nil {
		t.Fatalf("RescoreSoleOwner: %v", err)
	}
	scores, err := st.ScoresByPostingID(ctx)
	if err != nil || scores[id].PostingID != id {
		t.Fatalf("SQLite startup score = %+v err=%v, want posting %d", scores[id], err, id)
	}
}

func TestRescoreSoleOwnerHealsWithRulesWhenAIRuntimeFails(t *testing.T) {
	tests := []struct {
		name          string
		provider      string
		configure     func(*testing.T, *Server, credential.Cipher)
		wantErrorText string
	}{
		{
			name:     "credential decryption",
			provider: "anthropic",
			configure: func(t *testing.T, srv *Server, _ credential.Cipher) {
				srv.SetCredentialCipher(newAIRuntimeTestCipher(t, 0x72))
			},
			wantErrorText: "decrypt AI credential",
		},
		{
			name:          "provider construction",
			provider:      "synthetic-provider",
			configure:     func(_ *testing.T, srv *Server, cipher credential.Cipher) { srv.SetCredentialCipher(cipher) },
			wantErrorText: "construct AI provider",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, st := newPostgresTestServer(t, &fakeScraper{})
			ctx := context.Background()
			userID := insertAIRuntimeTestUser(t, st, "startup-"+strings.ReplaceAll(tt.name, " ", "-")+"@example.invalid")
			encryptingCipher := newAIRuntimeTestCipher(t, 0x71)
			saveAIRuntimeProfile(t, st, userID, profile.Profile{CareerYears: 0, AIProvider: tt.provider})
			const plaintextKey = "startup-secret-must-not-leak"
			saveAIRuntimeCredential(t, st, encryptingCipher, userID, tt.provider, plaintextKey)
			tt.configure(t, srv, encryptingCipher)

			p := listingPosting("startup-"+tt.provider, "시작 복구 규칙 점수 공고")
			p.FirstSeenAt, p.LastSeenAt = time.Now().UTC(), time.Now().UTC()
			postingID := mustUpsert(t, st, p)

			count, err := srv.RescoreSoleOwner(ctx)
			if err == nil || !strings.Contains(err.Error(), tt.wantErrorText) {
				t.Fatalf("RescoreSoleOwner error = %v, want %q", err, tt.wantErrorText)
			}
			if strings.Contains(err.Error(), plaintextKey) {
				t.Fatalf("RescoreSoleOwner leaked plaintext credential: %v", err)
			}
			if count != 1 {
				t.Fatalf("RescoreSoleOwner count = %d, want 1 rule-scored posting", count)
			}
			scores, scoreErr := st.ScoresByPostingID(ctx, userID)
			if scoreErr != nil || scores[postingID].PostingID != postingID {
				t.Fatalf("rule-only startup score = %+v err=%v, want posting %d", scores[postingID], scoreErr, postingID)
			}
		})
	}
}

func TestRescoreSoleOwnerJoinsRuntimeAndRuleScoreFailures(t *testing.T) {
	srv, st := newPostgresTestServer(t, &fakeScraper{})
	ctx := context.Background()
	userID := insertAIRuntimeTestUser(t, st, "startup-joined-errors@example.invalid")
	encryptingCipher := newAIRuntimeTestCipher(t, 0x73)
	srv.SetCredentialCipher(newAIRuntimeTestCipher(t, 0x74))
	saveAIRuntimeProfile(t, st, userID, profile.Profile{CareerYears: 0, AIProvider: "anthropic"})
	saveAIRuntimeCredential(t, st, encryptingCipher, userID, "anthropic", "joined-error-secret")
	p := listingPosting("startup-joined-errors", "복합 실패 공고")
	p.FirstSeenAt, p.LastSeenAt = time.Now().UTC(), time.Now().UTC()
	mustUpsert(t, st, p)
	if _, err := st.SQLDB().ExecContext(ctx, `DROP TABLE scores`); err != nil {
		t.Fatalf("drop scores table: %v", err)
	}

	_, err := srv.RescoreSoleOwner(ctx)
	if err == nil || !strings.Contains(err.Error(), "decrypt AI credential") || !strings.Contains(err.Error(), "save score") {
		t.Fatalf("RescoreSoleOwner error = %v, want joined runtime and rule-score failures", err)
	}
}
