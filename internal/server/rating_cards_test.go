package server

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ohchanwu/jobcron/internal/ai"
	"github.com/ohchanwu/jobcron/internal/profile"
)

func TestRatingRejectedSharedCardsAndHonestSummary(t *testing.T) {
	srv, stub, runtime := seedRerate(t)
	stub.ScoreDeltaFn = func(context.Context, string, string) ([]ai.RawDeltaItem, ai.Usage, error) {
		return []ai.RawDeltaItem{{Signal: "invalid", Kind: ai.KindPresence, Delta: 25, Quote: "지어낸 회사 업무", MatchedGoal: "목표"}}, ai.Usage{}, nil
	}
	ctx := context.Background()
	summary, err := srv.runRerate(ctx, "today", noopEmit, 1, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if msg := rerateDoneMessage(summary); !strings.Contains(msg, "처리 2/2 · 분석 완료 0/2 · 추가 반영 없음 0 · 근거 미확인 2") {
		t.Errorf("summary conflates rejection with a capped success: %s", msg)
	}
	// Collapse both rows; their definite current attempt must remain findable.
	prof := currentProfile(t, srv)
	min := 100
	prof.MinScore = &min
	pj, _ := profile.Marshal(prof)
	if _, _, err := srv.store.SaveProfile(ctx, pj); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.scoreAll(ctx, 1, runtime); err != nil {
		t.Fatal(err)
	}
	today, err := srv.buildBriefingWithRuntime(ctx, time.Now(), 1, runtime)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := srv.buildArchiveWithRuntime(ctx, time.Now(), 1, runtime)
	if err != nil {
		t.Fatal(err)
	}
	postings, _ := srv.store.AllPostings(ctx)
	for _, p := range postings {
		if err := srv.store.SetBookmark(ctx, p.ID, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	bookmarks, err := srv.buildBookmarksWithRuntime(ctx, time.Now(), 1, runtime)
	if err != nil {
		t.Fatal(err)
	}
	for name, view := range map[string]any{"index.html": today, "archive.html": archive, "bookmarks.html": bookmarks} {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			srv.render(rec, name, view)
			body := rec.Body.String()
			if strings.Count(body, "AI 분석 · 근거를 확인하지 못했어요") != 2 || strings.Contains(body, "추가로 반영할 내용 없음") {
				t.Fatal("missing/misclassified rejection on shared or collapsed surface")
			}
		})
	}
}
