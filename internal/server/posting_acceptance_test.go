package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ohchanwu/jobcron/internal/ai"
	"github.com/ohchanwu/jobcron/internal/profile"
	"github.com/ohchanwu/jobcron/internal/scraper"
	"github.com/ohchanwu/jobcron/internal/storage"
)

// This fixture has only an owned store and stub transport. PostgreSQL uses the
// real scoped schema helper; SQLite never reads a legacy credential file.
func seedPostingAcceptance(t *testing.T, dialect string) (*Server, *ai.StubProvider, *AIRuntime, profile.Profile, []scraper.Posting) {
	t.Helper()
	var srv *Server
	uid := int64(1)
	if dialect == "postgres" {
		srv, _ = newPostgresTestServer(t, &fakeScraper{})
		uid = insertAIRuntimeTestUser(t, srv.store, "acceptance@example.invalid")
	} else {
		srv, _ = newTestServer(t, &fakeScraper{})
	}
	zero := 0
	prof := profile.Profile{CareerYears: 0, MinScore: &zero, AIProvider: "anthropic", AIModel: ai.DefaultModel("anthropic"), JobLikes: "백엔드 서버 개발"}
	pj, _ := profile.Marshal(prof)
	if _, _, err := srv.saveProfileJSON(context.Background(), uid, pj); err != nil {
		t.Fatal(err)
	}
	stub := rerateStub()
	stub.NameVal = "anthropic"
	runtime := testAIRuntime(uid, stub, prof.AIModel)
	rows := []scraper.Posting{}
	for i := 0; i < 2; i++ {
		p := listingPosting(fmt.Sprintf("acceptance-%d", i), fmt.Sprintf("신입 서버 개발자 ROW-%d", i))
		p.Description = "서버 개발자를 찾습니다"
		p.FirstSeenAt = time.Now().UTC()
		p.LastSeenAt = p.FirstSeenAt
		p.ID = mustUpsert(t, srv.store, p)
		rows = append(rows, p)
	}
	return srv, stub, runtime, prof, rows
}

func TestPostingOutcomesAndAtomicFailureBothDialects(t *testing.T) {
	for _, dialect := range []string{"sqlite", "postgres"} {
		for _, state := range []string{storage.AIScoreRated, storage.AIScoreNoSignal, storage.AIScoreRejected, storage.AIScoreFailed, "storage_denied"} {
			t.Run(dialect+"/"+state, func(t *testing.T) {
				srv, stub, runtime, prof, rows := seedPostingAcceptance(t, dialect)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				if state == "storage_denied" {
					denyPostingFixtureWrite(t, srv.store, "ai_score_outcomes", "INSERT")
				}
				entered := make(chan struct{}, 2)
				release := make(chan struct{})
				done := make(chan struct{})
				stub.ScoreDeltaFn = func(ctx context.Context, text, _ string) ([]ai.RawDeltaItem, ai.Usage, error) {
					entered <- struct{}{}
					if strings.Contains(text, "ROW-1") {
						select {
						case <-release:
						case <-ctx.Done():
							return nil, ai.Usage{}, ctx.Err()
						}
					}
					switch state {
					case storage.AIScoreNoSignal, "storage_denied":
						return nil, ai.Usage{}, nil
					case storage.AIScoreFailed:
						return nil, ai.Usage{}, errors.New("synthetic provider failure")
					case storage.AIScoreRejected:
						return []ai.RawDeltaItem{{Signal: "unverified", Kind: ai.KindPresence, Delta: 7, Quote: "absent synthetic evidence", MatchedGoal: "goal"}}, ai.Usage{}, nil
					default:
						return []ai.RawDeltaItem{{Signal: "server", Kind: ai.KindPresence, Delta: 7, Quote: "서버 개발자를 찾습니다", MatchedGoal: "goal"}}, ai.Usage{}, nil
					}
				}
				own, _ := srv.postingUpdates.subscribe(runtime.UserID)
				defer own.close()
				other, _ := srv.postingUpdates.subscribe(runtime.UserID + 1)
				defer other.close()
				var analyzed, calls, processed int
				var runErr error
				go func() {
					defer close(done)
					analyzed, calls, processed, runErr = srv.rateStage2(ctx, rows, prof, runtime.UserID, runtime, srv.newAIBudget(ctx, runtime.UserID, runtime), &callCap{max: 2}, noopSchedulerEmit, true)
				}()
				defer func() {
					close(release)
					select {
					case <-done:
					case <-time.After(3 * time.Second):
						t.Error("owned worker failed to finish")
					}
				}()
				for i := 0; i < 2; i++ {
					select {
					case <-entered:
					case <-time.After(3 * time.Second):
						t.Fatal("worker did not enter")
					}
				}
				if state != "storage_denied" {
					select {
					case <-own.wake:
					case <-time.After(3 * time.Second):
						t.Fatal("saved outcome not visible before sibling terminal")
					}
					n := own.take()
					if n.GlobalRevision != 0 || n.UserRevision == 0 {
						t.Fatal(n)
					}
					var saved string
					if err := srv.store.SQLDB().QueryRow(`SELECT state FROM ai_score_outcomes WHERE posting_id=` + fmt.Sprint(rows[0].ID)).Scan(&saved); err != nil || saved != state {
						t.Fatal(saved, err)
					}
				}
				select {
				case <-done:
					t.Fatal("sibling prematurely terminal")
				default:
				}
				select {
				case <-other.wake:
					t.Fatal("private outcome leaked to other user")
				default:
				}
				// Cancel releases the sibling and must not turn an unsuccessful persistence
				// into an outcome, successful count, private revision, or half-committed score.
				cancel()
				<-done
				if calls != 2 || processed != 2 {
					t.Fatal("frozen processed/call accounting", calls, processed)
				}
				if state == "storage_denied" {
					var warning *rerateStorageError
					if !errors.As(runErr, &warning) || analyzed != 0 {
						t.Fatal("unsaved response counted", analyzed, runErr)
					}
					select {
					case <-own.wake:
						t.Fatal("failed persistence published saved state")
					default:
					}
					for _, table := range []string{"ai_scores", "ai_score_outcomes"} {
						var count int
						if err := srv.store.SQLDB().QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil || count != 0 {
							t.Fatal("half commit", table, count, err)
						}
					}
				}
			})
		}
	}
}

func denyPostingFixtureWrite(t *testing.T, st *storage.Store, table, operation string) {
	t.Helper()
	var statement string
	if st.Dialect() == storage.DialectPostgres {
		statement = `CREATE FUNCTION deny_posting_fixture() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic denied'; END $$; CREATE TRIGGER deny_posting_fixture BEFORE ` + operation + ` ON ` + table + ` FOR EACH ROW EXECUTE FUNCTION deny_posting_fixture()`
	} else {
		statement = `CREATE TRIGGER deny_posting_fixture BEFORE ` + operation + ` ON ` + table + ` BEGIN SELECT RAISE(ABORT,'synthetic denied'); END`
	}
	if _, err := st.SQLDB().Exec(statement); err != nil {
		t.Fatal(err)
	}
}

func TestPostingGlobalWriteFailurePublishesNothing(t *testing.T) {
	for _, dialect := range []string{"sqlite", "postgres"} {
		for _, kind := range []string{"insert", "extraction", "sweep", "dedup"} {
			t.Run(dialect+"/"+kind, func(t *testing.T) {
				srv, stub, runtime, _, rows := seedPostingAcceptance(t, dialect)
				ctx := context.Background()
				l, _ := srv.postingUpdates.subscribe(runtime.UserID)
				defer l.close()
				before := srv.postingUpdates.revisions(runtime.UserID).GlobalRevision
				switch kind {
				case "insert":
					denyPostingFixtureWrite(t, srv.store, "postings", "INSERT")
					p := listingPosting("denied-insert", "새 서버 공고")
					p.Description = "서버 개발자를 찾습니다"
					f := &fakeScraper{listing: []scraper.Posting{p}}
					_, _, _ = srv.runScrapeSource(ctx, f, time.Now(), func() *stage1Funding {
						return &stage1Funding{userID: runtime.UserID, runtime: runtime, budget: srv.newAIBudget(ctx, runtime.UserID, runtime)}
					}, noopEmit)
					if stub.Calls != 0 {
						t.Fatal("failed insert called extraction")
					}
				case "extraction":
					denyPostingFixtureWrite(t, srv.store, "ai_extractions", "INSERT")
					funding := &stage1Funding{userID: runtime.UserID, runtime: runtime, budget: srv.newAIBudget(ctx, runtime.UserID, runtime)}
					srv.extractStage1(ctx, rows[0].ID, rows[0], time.Now(), func() *stage1Funding { return funding })
					if stub.Calls != 1 {
						t.Fatal("extraction was not actually exercised")
					}
					var count int
					if err := srv.store.SQLDB().QueryRow(`SELECT COUNT(*) FROM ai_extractions`).Scan(&count); err != nil || count != 0 {
						t.Fatal(count, err)
					}
				case "sweep":
					old := listingPosting("denied-sweep", "오래된 서버 공고")
					old.FirstSeenAt = time.Now().Add(-200 * 24 * time.Hour)
					old.LastSeenAt = old.FirstSeenAt
					mustUpsert(t, srv.store, old)
					denyPostingFixtureWrite(t, srv.store, "postings", "DELETE")
					if _, err := srv.collectPostings(ctx, noopSchedulerEmit, func(string) bool { return true }); err == nil {
						t.Fatal("failed sweep not exercised")
					}
				case "dedup":
					if err := srv.store.MarkDuplicate(ctx, rows[1].ID, rows[0].ID); err != nil {
						t.Fatal(err)
					}
					denyPostingFixtureWrite(t, srv.store, "postings", "UPDATE")
					if _, err := srv.markCrossPortalDuplicates(ctx); err == nil {
						t.Fatal("failed dedup not exercised")
					}
				}
				if srv.postingUpdates.revisions(runtime.UserID).GlobalRevision != before {
					t.Fatal("failed global mutation advanced revision")
				}
				select {
				case <-l.wake:
					t.Fatal("failed global mutation published")
				default:
				}
			})
		}
	}
}

func TestPostingManualCohortDoesNotGrowWithLiveChanges(t *testing.T) {
	for _, surface := range []string{"today", "bookmarks", "archive"} {
		t.Run(surface, func(t *testing.T) {
			srv, stub, runtime, prof, rows := seedPostingAcceptance(t, "postgres")
			ctx := context.Background()
			for _, p := range rows {
				if err := srv.store.AddBookmark(ctx, runtime.UserID, p.ID); err != nil {
					t.Fatal(err)
				}
			}
			entered := make(chan string, 2)
			release := make(chan struct{})
			finished := make(chan struct{})
			var mu sync.Mutex
			progress := []string{}
			stub.ScoreDeltaFn = func(ctx context.Context, text, _ string) ([]ai.RawDeltaItem, ai.Usage, error) {
				entered <- text
				select {
				case <-release:
					return nil, ai.Usage{}, nil
				case <-ctx.Done():
					return nil, ai.Usage{}, ctx.Err()
				}
			}
			runtime.PerCallCap = 2
			var summary rerateSummary
			var runErr error
			go func() {
				defer close(finished)
				summary, runErr = srv.runRerate(ctx, surface, func(event, data string) {
					if event == "progress" {
						mu.Lock()
						progress = append(progress, data)
						mu.Unlock()
					}
				}, runtime.UserID, runtime)
			}()
			defer func() {
				select {
				case <-finished:
				default:
					close(release)
					<-finished
				}
			}()
			for i := 0; i < 2; i++ {
				select {
				case text := <-entered:
					if !strings.Contains(text, "ROW-") {
						t.Fatal(text)
					}
				case <-time.After(3 * time.Second):
					t.Fatal("admitted cohort did not enter")
				}
			}
			// Mutations are an intentional test-only concurrent writer, not another paid
			// operation. The two already-admitted rows and phase target must remain frozen.
			newRow := listingPosting("live-new-cohort", "신입 새 서버 개발자")
			newRow.FirstSeenAt = time.Now()
			newRow.LastSeenAt = newRow.FirstSeenAt
			if err := srv.writePostingState(0, true, func() error { _, _, err := srv.store.UpsertPosting(ctx, newRow); return err }); err != nil {
				t.Fatal(err)
			}
			high := 100
			prof.MinScore = &high
			prof.JobLikes = "changed current synthetic goal"
			pj, _ := profile.Marshal(prof)
			if _, _, err := srv.saveProfileJSON(ctx, runtime.UserID, pj); err != nil {
				t.Fatal(err)
			}
			close(release)
			<-finished
			if runErr != nil || summary.Visible != 2 || summary.Processed != 2 || summary.ProviderCalls != 2 || stub.ScoreDeltaCalls != 2 || summary.Analyzed != 2 {
				t.Fatal("live changes altered active cohort/caps", summary, runErr, stub.ScoreDeltaCalls)
			}
			mu.Lock()
			defer mu.Unlock()
			for _, text := range progress {
				if strings.Contains(text, "공고 ") && strings.Contains(text, " 분석") && !strings.Contains(text, "/2") {
					t.Fatal("phase denominator changed", text)
				}
			}
			current, err := srv.postingEnvelope(ctx, runtime.UserID, "archive", "score", "1")
			if err != nil || len(current.Rows) != 3 {
				t.Fatal("current view lost newly committed row", err, len(current.Rows))
			}
			t.Logf("frozen %s cohort=%v cap=2 stage2calls=2; current view rows=3 current goal changed; phase progress=%v", surface, []int64{rows[0].ID, rows[1].ID}, progress)
		})
	}
}

func TestPostingSnapshotGlobalAdmissionAndCanceledReadRelease(t *testing.T) {
	h := newPostingUpdateHub()
	release := []func(){}
	for uid := int64(1); uid <= 4; uid++ {
		for i := 0; i < 2; i++ {
			end, ok := h.admitSnapshot(uid)
			if !ok {
				t.Fatal("premature admission bound")
			}
			release = append(release, end)
		}
	}
	if _, ok := h.admitSnapshot(5); ok {
		t.Fatal("ninth global read admitted")
	}
	for _, end := range release {
		end()
	}
	if h.totalReads != 0 || len(h.reads) != 0 {
		t.Fatal("admission release leaked")
	}
	srv, _ := newTestServer(t, &fakeScraper{})
	finish := srv.postingUpdates.beginWrite(0, true)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	req := httptest.NewRequest("GET", "/api/posting-view?surface=archive&sort=date&generation=1", nil).WithContext(ctx)
	recorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recorder, req)
	finish(false, "")
	if recorder.Code != http.StatusServiceUnavailable || srv.postingUpdates.totalReads != 0 || srv.store.SQLDB().Stats().InUse != 0 {
		t.Fatal("cancel retained a read/transaction", recorder.Code, srv.store.SQLDB().Stats())
	}
}

func TestPostingPostgresInsertVisibleWhileExtractionBlocked(t *testing.T) {
	srv, st := newPostgresTestServer(t, &fakeScraper{})
	uid := insertAIRuntimeTestUser(t, st, "insert-blocked@example.invalid")
	ctx := context.Background()
	zero := 0
	saveAIRuntimeProfile(t, st, uid, profile.Profile{CareerYears: 0, MinScore: &zero})
	p := listingPosting("postgres-blocked-insert", "신입 서버 개발자")
	p.Description = "서버 개발자를 찾습니다"
	f := &fakeScraper{listing: []scraper.Posting{p}}
	entered, release := make(chan struct{}), make(chan struct{})
	stub := &ai.StubProvider{NameVal: "anthropic", ExtractFn: func(ctx context.Context, _ string) (ai.Extraction, ai.Usage, error) {
		close(entered)
		select {
		case <-release:
			return ai.Extraction{Newcomer: true}, ai.Usage{}, nil
		case <-ctx.Done():
			return ai.Extraction{}, ai.Usage{}, ctx.Err()
		}
	}}
	runtime := testAIRuntime(uid, stub, ai.DefaultModel("anthropic"))
	funding := &stage1Funding{userID: uid, runtime: runtime, budget: srv.newAIBudget(ctx, uid, runtime)}
	l, _ := srv.postingUpdates.subscribe(uid)
	defer l.close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _, _ = srv.runScrapeSource(ctx, f, time.Now(), func() *stage1Funding { return funding }, noopSchedulerEmit)
	}()
	defer func() { close(release); <-done }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("owned PostgreSQL extraction did not enter")
	}
	select {
	case <-l.wake:
	case <-time.After(time.Second):
		t.Fatal("committed insert hidden behind extraction")
	}
	n := l.take()
	view, err := srv.projectBriefing(ctx, time.Now(), uid)
	if err != nil || len(view.Today) != 1 || view.Today[0].Total != 25 || n.GlobalRevision == 0 || n.UserRevision != 0 || n.Context != "" {
		t.Fatal("insert rule projection/public notice", view, n, err)
	}
	select {
	case <-done:
		t.Fatal("provider prematurely terminal")
	default:
	}
}
