package server

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ohchanwu/jobcron/internal/ai"
	"github.com/ohchanwu/jobcron/internal/profile"
	"github.com/ohchanwu/jobcron/internal/scraper"
	"github.com/ohchanwu/jobcron/internal/storage"
)

func TestPostingSavedOutcomePublishesBeforeSiblingAndTerminal(t *testing.T) {
	for _, state := range []string{storage.AIScoreRated, storage.AIScoreNoSignal, storage.AIScoreRejected, storage.AIScoreFailed} {
		t.Run(state, func(t *testing.T) {
			srv, stub, runtime := seedRerate(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			rows, err := srv.store.AllPostings(ctx)
			if err != nil || len(rows) != 2 {
				t.Fatalf("rows=%d err=%v", len(rows), err)
			}
			rows[1].Title += " blocked-sibling"
			release := make(chan struct{})
			first := make(chan struct{}, 2)
			stub.ScoreDeltaFn = func(ctx context.Context, text, goal string) ([]ai.RawDeltaItem, ai.Usage, error) {
				first <- struct{}{}
				if strings.Contains(text, rows[1].Title) {
					select {
					case <-release:
					case <-ctx.Done():
						return nil, ai.Usage{}, ctx.Err()
					}
				}
				switch state {
				case storage.AIScoreRated:
					return []ai.RawDeltaItem{{Signal: "백엔드", MatchedGoal: "좋아하는 업무", Kind: ai.KindPresence, Delta: 10, Quote: "서버 개발자를 찾습니다"}}, ai.Usage{}, nil
				case storage.AIScoreRejected:
					return []ai.RawDeltaItem{{Signal: "unsupported", MatchedGoal: "Go 서비스 개발", Kind: ai.KindPresence, Delta: 10, Quote: "fabricated quote"}}, ai.Usage{}, nil
				case storage.AIScoreFailed:
					return nil, ai.Usage{}, errors.New("synthetic provider failure")
				default:
					return nil, ai.Usage{}, nil
				}
			}
			l, _ := srv.postingUpdates.subscribe(runtime.UserID)
			defer l.close()
			done := make(chan struct{})
			go func() {
				defer close(done)
				srv.rateStage2(ctx, rows, currentProfile(t, srv), runtime.UserID, runtime, srv.newAIBudget(ctx, runtime.UserID, runtime), &callCap{max: 200}, noopEmit, true)
			}()
			defer func() {
				close(release)
				select {
				case <-done:
				case <-time.After(3 * time.Second):
					t.Error("pool didn't finish")
				}
			}()
			for i := 0; i < 2; i++ {
				select {
				case <-first:
				case <-time.After(3 * time.Second):
					t.Fatal("workers didn't start")
				}
			}
			select {
			case <-l.wake:
			case <-time.After(3 * time.Second):
				t.Fatal("saved outcome invisible while sibling provider blocked")
			}
			n := l.take()
			if n.GlobalRevision != 0 || n.UserRevision == 0 {
				t.Fatal(n)
			}
			var got string
			if err := srv.store.SQLDB().QueryRow(`SELECT state FROM ai_score_outcomes WHERE posting_id=` + fmt.Sprint(rows[0].ID)).Scan(&got); err != nil || got != state {
				t.Fatalf("saved state=%s want=%s err=%v", got, state, err)
			}
			select {
			case <-done:
				t.Fatal("pool ended before sibling release")
			default:
			}
		})
	}
}

func TestPostingWorkerResultCarriesCommittedIdentity(t *testing.T) {
	srv, _, runtime := seedRerate(t)
	ctx := context.Background()
	rows, _ := srv.store.AllPostings(ctx)
	prof := currentProfile(t, srv)
	hash := profile.AIInputHash(prof)
	r := srv.reratePosting(ctx, rows[0], hash, profile.BuildStage2ProfileText(prof), time.Now(), runtime.UserID, runtime, srv.newAIBudget(ctx, runtime.UserID, runtime), &callCap{max: 200})
	if r.postingID != rows[0].ID || r.userID != runtime.UserID || r.inputHash != hash || r.version != runtime.ScoreVersion || r.status != postingResultCommitted || r.outcome != storage.AIScoreRated || !r.providerCalled || r.err != nil {
		t.Fatalf("worker result=%+v", r)
	}
	l, _ := srv.postingUpdates.subscribe(runtime.UserID)
	defer l.close()
	r = srv.reratePosting(ctx, rows[0], hash, profile.BuildStage2ProfileText(prof), time.Now(), runtime.UserID, runtime, srv.newAIBudget(ctx, runtime.UserID, runtime), &callCap{max: 200}, r.outcome)
	if r.status != postingResultCached || r.providerCalled || r.err != nil {
		t.Fatal(r)
	}
	select {
	case <-l.wake:
		t.Fatal("unchanged success hit flashed")
	default:
	}
}

func TestPostingBackgroundAnalysisRecordsStatusBelowNoopEmit(t *testing.T) {
	srv, stub, runtime := seedRerate(t)
	prof := currentProfile(t, srv)
	prof.ScheduledAIEnabled = true
	stub.ScoreDeltaFn = func(context.Context, string, string) ([]ai.RawDeltaItem, ai.Usage, error) {
		status, ok := srv.rerates.snapshot(runtime.UserID, "today")
		if !ok || status.State != rerateStateRunning {
			t.Error("scheduled/noop analysis has no active user status")
		}
		return nil, ai.Usage{}, nil
	}
	_, err := srv.analyzeUserCollection(context.Background(), "scheduled", noopSchedulerEmit, runtime.UserID, prof, runtime, srv.newAIBudget(context.Background(), runtime.UserID, runtime), scrapeCollection{now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	status, ok := srv.rerates.snapshot(runtime.UserID, "today")
	if !ok || status.State != rerateStateDone {
		t.Fatal("background terminal missing", status)
	}
}

func TestPostingValidationWriteFailureRetainsPendingAndNoNotice(t *testing.T) {
	srv, st := newPostgresTestServer(t, &fakeScraper{})
	ctx := context.Background()
	uid := insertAIRuntimeTestUser(t, st, "validation-storage@example.invalid")
	prof := profile.Profile{Dealbreakers: []string{"야근"}}
	saveAIRuntimeProfile(t, st, uid, prof)
	p := listingPosting("validation-failure", "야근 개발자")
	p.Description = "야근 업무를 맡습니다"
	p.ID = mustUpsert(t, st, p)
	if _, err := st.SQLDB().Exec(`CREATE FUNCTION deny_validation_write() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic persistence denial'; END $$; CREATE TRIGGER deny_validation BEFORE INSERT ON ai_dealbreaker_validations FOR EACH ROW EXECUTE FUNCTION deny_validation_write()`); err != nil {
		t.Fatal(err)
	}
	stub := &ai.StubProvider{NameVal: "stub", ValidateDealbreakersFn: func(_ context.Context, _ string, c []ai.DealbreakerCandidate) ([]ai.DealbreakerValidation, ai.Usage, error) {
		return []ai.DealbreakerValidation{{CandidateID: c[0].ID, Verdict: ai.DealbreakerNotApplicable, ReasonCode: ai.DealbreakerReasonBenefitOrEligibility}}, ai.Usage{}, nil
	}}
	runtime := testAIRuntime(uid, stub, "shared-model")
	l, _ := srv.postingUpdates.subscribe(uid)
	defer l.close()
	summary, err := srv.validateDealbreakers(ctx, uid, []scraper.Posting{p}, prof, runtime, srv.newAIBudget(ctx, uid, runtime), &callCap{max: 200}, noopEmit)
	var warning *rerateStorageError
	if !errors.As(err, &warning) || summary.PendingBefore != 1 || summary.PendingAfter != 1 || summary.AcceptedChecks != 0 || summary.UnresolvedChecks != 1 {
		t.Fatalf("unsaved validation claimed resolved: %+v err=%v", summary, err)
	}
	select {
	case <-l.wake:
		t.Fatal("failed validation published a saved change")
	default:
	}
}

func TestPostingUnknownLegacyCacheIsProcessedWithoutFreshSuccess(t *testing.T) {
	srv, stub, runtime := seedRerate(t)
	ctx := context.Background()
	prof := currentProfile(t, srv)
	rows, _ := srv.store.AllPostings(ctx)
	row := rows[0]
	if err := srv.store.UpsertAIScore(ctx, runtime.UserID, row.ID, profile.AIInputHash(prof), runtime.ScoreVersion, ai.Delta{}, time.Now()); err != nil {
		t.Fatal(err)
	}
	l, _ := srv.postingUpdates.subscribe(runtime.UserID)
	defer l.close()
	analyzed, calls, _, err := srv.rateStage2(ctx, rows[:1], prof, runtime.UserID, runtime, srv.newAIBudget(ctx, runtime.UserID, runtime), &callCap{max: 200}, noopEmit, true)
	if err != nil || analyzed != 0 || calls != 0 || stub.ScoreDeltaCalls != 0 {
		t.Fatalf("unknown legacy cache invented fresh success or spend: analyzed=%d calls=%d err=%v", analyzed, calls, err)
	}
	select {
	case <-l.wake:
		t.Fatal("unchanged legacy hit flashed")
	default:
	}
}

func TestPostingUnknownProvenanceTerminalStaysExplicit(t *testing.T) {
	message := rerateDoneMessage(rerateSummary{Visible: 1, Processed: 1, CachedUnknown: 1})
	if !strings.Contains(message, "이전 분석의 상태는 확인할 수 없어요") || strings.Contains(message, "모든 공고가 AI로 평가") || strings.Contains(message, "다시 눌러") {
		t.Fatalf("unknown provenance copy=%q", message)
	}
}
