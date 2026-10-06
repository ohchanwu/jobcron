package server

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ohchanwu/jobcron/internal/ai"
	"github.com/ohchanwu/jobcron/internal/profile"
	"github.com/ohchanwu/jobcron/internal/scraper"
	"github.com/ohchanwu/jobcron/internal/storage"
)

func TestRatingModelRoundtripRecordsDefiniteRetry(t *testing.T) {
	for _, dialect := range []string{"sqlite", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			for _, predecessor := range []string{storage.AIScoreRated, storage.AIScoreNoSignal} {
				for _, successor := range []string{storage.AIScoreRejected, storage.AIScoreFailed} {
					t.Run(predecessor+"/"+successor, func(t *testing.T) {
						srv, stub, runtime := seedRerate(t)
						ctx := context.Background()
						prof := currentProfile(t, srv)
						uid := int64(1)
						if dialect == "postgres" {
							pg, st := newPostgresTestServer(t, &fakeScraper{})
							uid = insertAIRuntimeTestUser(t, st, "roundtrip@example.invalid")
							saveAIRuntimeProfile(t, st, uid, prof)
							seed, err := srv.store.AllPostings(ctx)
							if err != nil {
								t.Fatal(err)
							}
							for _, p := range seed {
								mustUpsert(t, st, p)
							}
							srv = pg
							runtime = testAIRuntime(uid, stub, "test-model")
							if _, err := srv.scoreAll(ctx, uid, runtime); err != nil {
								t.Fatal(err)
							}
						}
						rows, err := srv.store.AllPostings(ctx)
						if err != nil {
							t.Fatal(err)
						}
						p := rows[0]
						hash := profile.AIInputHash(prof)
						original := runtime.ScoreVersion
						now := time.Now().UTC()
						for i, version := range []string{original, ai.ScoreVersion("stub", "model-b"), ai.ScoreVersion("stub", "model-c")} {
							d := ai.Delta{}
							o := storage.AIScoreOutcome{State: predecessor, ComputedAt: now.Add(time.Duration(i) * time.Second)}
							if predecessor == storage.AIScoreRated {
								d = ai.GateDelta([]ai.RawDeltaItem{{Signal: "server", MatchedGoal: "goal", Kind: ai.KindPresence, Delta: 7, Quote: "서버 개발자를 찾습니다"}}, p.Description, p.Description)
								o.Proposed, o.Accepted = 1, 1
							}
							if err := srv.store.UpsertAIResult(ctx, uid, p.ID, hash, version, d, o); err != nil {
								t.Fatal(err)
							}
						}
						if _, ok, err := srv.store.AIScore(ctx, uid, p.ID, hash, original); err != nil || ok {
							t.Fatalf("ordinary pruning setup: cache=%v err=%v", ok, err)
						}
						// Merely returning to A must not present a pruned success as current.
						before, err := srv.buildBriefingWithRuntime(ctx, now, uid, runtime)
						if err != nil {
							t.Fatal(err)
						}
						for _, row := range before.Today {
							if row.Posting.ID == p.ID && (row.AIOutcome == storage.AIScoreRated || row.AIOutcome == storage.AIScoreNoSignal) {
								t.Errorf("pruned success still displayed current: %s", row.AIOutcome)
							}
						}
						stub.ScoreDeltaFn = func(context.Context, string, string) ([]ai.RawDeltaItem, ai.Usage, error) {
							if successor == storage.AIScoreFailed {
								return nil, ai.Usage{InputTokens: 5}, errors.New("fixture provider failure")
							}
							return []ai.RawDeltaItem{{Signal: "invalid", MatchedGoal: "goal", Kind: ai.KindPresence, Delta: 10, Quote: "fabricated nonexistent quote"}}, ai.Usage{InputTokens: 5}, nil
						}
						cached, called, err := srv.rerateOne(ctx, p, hash, profile.BuildStage2ProfileText(prof), now, uid, runtime, srv.newAIBudget(ctx, uid, runtime), &callCap{max: 200})
						var storageErr *rerateStorageError
						if cached || !called || errors.As(err, &storageErr) || (successor == storage.AIScoreRejected && err != nil) || (successor == storage.AIScoreFailed && err == nil) {
							t.Fatalf("definite retry: cached=%v called=%v err=%v", cached, called, err)
						}
						out, err := srv.store.AIScoreOutcomesByPostingID(ctx, uid, hash, original)
						if err != nil || out[p.ID].State != successor {
							t.Fatalf("retry provenance: %v %v", out, err)
						}
						assertStage1Usage(t, srv.store, uid, 5)
						if _, err := srv.scoreAll(ctx, uid, runtime); err != nil {
							t.Fatal(err)
						}
						view, err := srv.buildBriefingWithRuntime(ctx, now, uid, runtime)
						if err != nil {
							t.Fatal(err)
						}
						if view.Rerate.Analyzed != 0 || view.Rerate.Visible != 2 {
							t.Fatalf("false successful N: %+v", view.Rerate)
						}
						rec := httptest.NewRecorder()
						srv.render(rec, "index.html", view)
						want := "AI 분석 · 근거를 확인하지 못했어요"
						if successor == storage.AIScoreFailed {
							want = "AI 분석을 완료하지 못했어요"
						}
						if !strings.Contains(rec.Body.String(), want) || strings.Contains(rec.Body.String(), "추가로 반영할 내용 없음") {
							t.Fatal("false current card after retry")
						}
						if successor == storage.AIScoreRejected {
							n, calls, _, err := srv.rateStage2(ctx, []scraper.Posting{p}, prof, uid, runtime, srv.newAIBudget(ctx, uid, runtime), &callCap{max: 200}, noopEmit, false)
							if err != nil || n != 0 || calls != 0 {
								t.Fatalf("automatic rejection re-spend: n=%d calls=%d err=%v", n, calls, err)
							}
						}
					})
				}
			}
		})
	}
}
