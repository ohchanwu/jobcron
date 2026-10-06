package server

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ohchanwu/jobcron/internal/ai"
	"github.com/ohchanwu/jobcron/internal/profile"
	"github.com/ohchanwu/jobcron/internal/storage"
)

func TestRatingPartialProviderFailureKeepsTerminalCause(t *testing.T) {
	for _, cached := range []bool{false, true} {
		t.Run(map[bool]string{false: "new success", true: "cached success"}[cached], func(t *testing.T) {
			srv, stub, runtime := seedRerate(t)
			ctx := context.Background()
			rows, err := srv.store.AllPostings(ctx)
			if err != nil {
				t.Fatal(err)
			}
			prof := currentProfile(t, srv)
			if cached {
				if err := srv.store.UpsertAIResult(ctx, 1, rows[0].ID, profile.AIInputHash(prof), runtime.ScoreVersion, ai.Delta{}, storage.AIScoreOutcome{State: storage.AIScoreNoSignal, ComputedAt: rows[0].FirstSeenAt}); err != nil {
					t.Fatal(err)
				}
			}
			// Distinguish the real rows by text, not pool-worker call ordering.
			p := rows[0]
			p.Description += " fixture-success"
			if err := srv.store.RefreshPostingDetail(ctx, p.ID, p, p.FirstSeenAt); err != nil {
				t.Fatal(err)
			}
			stub.ScoreDeltaFn = func(_ context.Context, text, _ string) ([]ai.RawDeltaItem, ai.Usage, error) {
				if strings.Contains(text, "fixture-success") {
					return []ai.RawDeltaItem{}, ai.Usage{InputTokens: 5}, nil
				}
				return nil, ai.Usage{InputTokens: 5}, errors.New("fixture provider failure")
			}
			summary, err := srv.runRerate(ctx, "today", noopEmit, 1, runtime)
			wantCalls := 2
			if cached {
				wantCalls = 1
			}
			if err != nil || summary.Analyzed != 1 || summary.Processed != 2 || summary.ProviderCalls != wantCalls || summary.NoSignal != 1 {
				t.Fatalf("setup: %+v %v", summary, err)
			}
			msg := rerateDoneMessage(summary)
			if strings.Contains(msg, "호출 수나 토큰 예산 한도로") || !strings.Contains(msg, "AI 분석에 실패했어요") {
				t.Fatalf("partial failure mislabeled or lost: %s", msg)
			}
			run := srv.rerates.start(1, "today", "fixture-entry-12345")
			srv.rerates.complete(1, "today", run.RunID, rerateDoneOutcome(summary), msg)
			status, ok := srv.rerates.snapshot(1, "today")
			if !ok || status.Message != msg || status.Outcome != rerateOutcomePartial {
				t.Fatalf("recovery-visible cause: %+v", status)
			}
			// Context warnings and rejection explain different facts; neither may
			// hide the definite partial Stage-2 failure or invent a limit skip.
			summary.ContextPendingBefore, summary.ContextPendingAfter = 1, 1
			summary.ContextFailureMessage = "AI 문맥 확인을 완료하지 못했어요."
			summary.Rejected = 1
			mixed := rerateDoneMessage(summary)
			for _, want := range []string{"AI 분석에 실패했어요", "AI 문맥 확인을 완료하지 못했어요", "새 AI 조정을 반영하지 않았어요", "분석 완료 1/2"} {
				if !strings.Contains(mixed, want) {
					t.Errorf("mixed summary lost %q: %s", want, mixed)
				}
			}
			if strings.Contains(mixed, "호출 수나 토큰 예산 한도로") {
				t.Fatal("mixed failures fabricated a budget skip")
			}
		})
	}
}

func TestRatingTerminalCopyReportsOnlyActualLimitSkips(t *testing.T) {
	for _, cause := range []string{"cap", "budget", "rejection and cap"} {
		t.Run(cause, func(t *testing.T) {
			srv, stub, runtime := seedRerate(t)
			if cause == "budget" {
				runtime.RunTokenCap = 0
			} else {
				runtime.PerCallCap = 1
			}
			stub.ScoreDeltaFn = func(context.Context, string, string) ([]ai.RawDeltaItem, ai.Usage, error) {
				if cause == "rejection and cap" {
					return []ai.RawDeltaItem{{Signal: "invalid", MatchedGoal: "goal", Kind: ai.KindPresence, Delta: 10, Quote: "fabricated nonexistent quote"}}, ai.Usage{InputTokens: 5}, nil
				}
				return []ai.RawDeltaItem{}, ai.Usage{InputTokens: 5}, nil
			}
			summary, err := srv.runRerate(context.Background(), "today", noopEmit, 1, runtime)
			if err != nil {
				t.Fatal(err)
			}
			if summary.Processed >= summary.Visible {
				t.Fatalf("fixture did not skip: %+v", summary)
			}
			msg := rerateDoneMessage(summary)
			if !strings.Contains(msg, "호출 수나 토큰 예산 한도로") || strings.Contains(msg, "AI 분석에 실패했어요") {
				t.Fatalf("actual skip mislabeled: %s", msg)
			}
			if cause == "rejection and cap" && !strings.Contains(msg, "새 AI 조정을 반영하지 않았어요") {
				t.Fatal("rejection lost beside genuine limit skip")
			}
		})
	}
}
