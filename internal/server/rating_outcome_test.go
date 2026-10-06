package server

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ohchanwu/jobcron/internal/ai"
)

func TestRatingNoSignalPersistsProvenanceBeforeSuccess(t *testing.T) {
	srv, stub, runtime := seedRerate(t)
	stub.ScoreDeltaFn = func(context.Context, string, string) ([]ai.RawDeltaItem, ai.Usage, error) {
		return []ai.RawDeltaItem{}, ai.Usage{InputTokens: 5}, nil
	}
	ctx := context.Background()
	summary, err := srv.runRerate(ctx, "today", noopEmit, 1, runtime)
	if err != nil || summary.Analyzed != 2 {
		t.Fatalf("empty success: summary=%+v err=%v", summary, err)
	}
	var n int
	if err := srv.store.SQLDB().QueryRow(`SELECT COUNT(*) FROM ai_score_outcomes WHERE user_id=1 AND state='no_signal' AND proposed=0 AND accepted=0`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("genuine empty provenance: n=%d err=%v", n, err)
	}
	summary, err = srv.runRerate(ctx, "today", noopEmit, 1, runtime)
	if err != nil || summary.Analyzed != 2 || stub.ScoreDeltaCalls != 2 {
		t.Fatalf("repeat must spend zero: summary=%+v calls=%d err=%v", summary, stub.ScoreDeltaCalls, err)
	}
}

func TestRatingNoSignalSharedSurfaceCard(t *testing.T) {
	srv, stub, runtime := seedRerate(t)
	stub.ScoreDeltaFn = func(context.Context, string, string) ([]ai.RawDeltaItem, ai.Usage, error) {
		return []ai.RawDeltaItem{}, ai.Usage{}, nil
	}
	ctx := context.Background()
	if _, err := srv.runRerate(ctx, "today", noopEmit, 1, runtime); err != nil {
		t.Fatal(err)
	}
	postings, _ := srv.store.AllPostings(ctx)
	for _, p := range postings {
		if err := srv.store.SetBookmark(ctx, p.ID, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	today, err := srv.buildBriefingWithRuntime(ctx, time.Now(), 1, runtime)
	if err != nil {
		t.Fatal(err)
	}
	bookmarks, err := srv.buildBookmarksWithRuntime(ctx, time.Now(), 1, runtime)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := srv.buildArchiveWithRuntime(ctx, time.Now(), 1, runtime)
	if err != nil {
		t.Fatal(err)
	}
	for name, view := range map[string]any{"index.html": today, "bookmarks.html": bookmarks, "archive.html": archive} {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			srv.render(rec, name, view)
			if rec.Code != 200 || strings.Count(rec.Body.String(), "AI 분석 완료 · 추가로 반영할 내용 없음") != 2 {
				t.Fatalf("surface %s lacks truthful shared empty cards: status=%d", name, rec.Code)
			}
		})
	}
}
