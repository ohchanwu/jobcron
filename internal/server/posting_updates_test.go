package server

import (
	"context"
	"fmt"
	"github.com/ohchanwu/jobcron/internal/ai"
	"github.com/ohchanwu/jobcron/internal/profile"
	"github.com/ohchanwu/jobcron/internal/scraper"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPostingUpdateHubCommitReadBarrierAndScope(t *testing.T) {
	h := newPostingUpdateHub()
	a, ok := h.subscribe(1)
	if !ok {
		t.Fatal("subscribe")
	}
	defer a.close()
	b, ok := h.subscribe(2)
	if !ok {
		t.Fatal("subscribe")
	}
	defer b.close()
	finish := h.beginWrite(1, false)
	read := make(chan struct{})
	go func() { end := h.beginRead(1); close(read); end() }()
	select {
	case <-read:
		t.Fatal("read passed uncommitted mutation")
	case <-time.After(20 * time.Millisecond):
	}
	finish(true, "new-context")
	select {
	case <-read:
	case <-time.After(time.Second):
		t.Fatal("read stayed blocked")
	}
	select {
	case <-a.wake:
	case <-time.After(time.Second):
		t.Fatal("commit did not notify own user")
	}
	got := a.take()
	if got.UserRevision == 0 || got.Context != "new-context" {
		t.Fatalf("notice=%+v", got)
	}
	select {
	case <-b.wake:
		t.Fatal("private update crossed users")
	default:
	}
	failed := h.beginWrite(1, false)
	failed(false, "")
	select {
	case <-a.wake:
		t.Fatal("failed write published saved state")
	default:
	}
}

func TestPostingUpdateHubCoalescesAndBounds(t *testing.T) {
	h := newPostingUpdateHub()
	var all []*postingListener
	for i := 0; i < 8; i++ {
		l, ok := h.subscribe(1)
		if !ok {
			t.Fatal("premature bound")
		}
		all = append(all, l)
	}
	if _, ok := h.subscribe(1); ok {
		t.Fatal("user listener bound bypassed")
	}
	for u := int64(2); u <= 8; u++ {
		for i := 0; i < 8; i++ {
			l, ok := h.subscribe(u)
			if !ok {
				t.Fatal("premature total bound")
			}
			all = append(all, l)
		}
	}
	if _, ok := h.subscribe(9); ok {
		t.Fatal("total listener bound bypassed")
	}
	for _, context := range []string{"first", "latest", ""} {
		end := h.beginWrite(1, false)
		end(true, context)
	}
	<-all[0].wake
	notice := all[0].take()
	if !notice.Resync || notice.Context != "latest" {
		t.Fatalf("lost strongest resync/latest context: %+v", notice)
	}
	before := notice.UserRevision
	for _, l := range all {
		l.close()
	}
	l, ok := h.subscribe(1)
	if !ok {
		t.Fatal("cleanup leaked capacity")
	}
	defer l.close()
	if h.revisions(1).UserRevision < before {
		t.Fatal("eviction rolled revision backward")
	}
}

func TestPostingWriterBookmarkPublishesOnlySavedChange(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	ctx := context.Background()
	p := listingPosting("notice", "공지 공고")
	id := mustUpsert(t, st, p)
	l, ok := srv.postingUpdates.subscribe(0)
	if !ok {
		t.Fatal("subscribe")
	}
	defer l.close()
	r := httptest.NewRequest("PUT", "/api/bookmark/1", nil)
	r.SetPathValue("id", fmt.Sprint(id))
	w := httptest.NewRecorder()
	srv.handleBookmarkAdd(w, r)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	select {
	case <-l.wake:
	case <-time.After(time.Second):
		t.Fatal("saved bookmark had no revision")
	}
	if got := l.take(); got.UserRevision == 0 {
		t.Fatal("missing private revision")
	}
	if saved, err := st.IsBookmarked(ctx, id); err != nil || !saved {
		t.Fatal("notice preceded saved bookmark")
	}
	r = httptest.NewRequest("PUT", "/api/bookmark/999999", nil)
	r.SetPathValue("id", "999999")
	w = httptest.NewRecorder()
	srv.handleBookmarkAdd(w, r)
	if w.Code == 200 {
		t.Fatal("failed write accepted")
	}
	select {
	case <-l.wake:
		t.Fatal("failed bookmark published")
	default:
	}
}

func TestPostingProjectionEstablishesUnderActualBarrier(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	st.SQLDB().SetMaxOpenConns(2)
	pj, _ := profile.Marshal(profile.Profile{})
	if _, _, err := st.SaveProfile(context.Background(), pj); err != nil {
		t.Fatal(err)
	}
	finish := srv.postingUpdates.beginWrite(0, true)
	result := make(chan error, 1)
	go func() { _, err := srv.loadPostingProjection(context.Background(), 0); result <- err }()
	select {
	case <-result:
		t.Fatal("projection crossed uncommitted barrier")
	case <-time.After(20 * time.Millisecond):
	}
	finish(false, "")
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("reader not released")
	}
}

func TestPostingInsertNotifiesBeforeBlockedExtraction(t *testing.T) {
	p := listingPosting("insert-before-ai", "신입 서버 개발자")
	p.Description = "서버 개발 업무"
	f := &fakeScraper{listing: []scraper.Posting{p}}
	srv, st := newTestServer(t, f)
	st.SQLDB().SetMaxOpenConns(2)
	ctx := context.Background()
	zero := 0
	pj, _ := profile.Marshal(profile.Profile{CareerYears: 0, MinScore: &zero})
	if _, _, err := st.SaveProfile(ctx, pj); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	provider := &ai.StubProvider{NameVal: "stub", ExtractFn: func(ctx context.Context, _ string) (ai.Extraction, ai.Usage, error) {
		close(started)
		select {
		case <-release:
			return ai.Extraction{Newcomer: true}, ai.Usage{}, nil
		case <-ctx.Done():
			return ai.Extraction{}, ai.Usage{}, ctx.Err()
		}
	}}
	runtime := testAIRuntime(1, provider, "test-model")
	funding := &stage1Funding{userID: 1, runtime: runtime, budget: srv.newAIBudget(ctx, 1, runtime)}
	l, ok := srv.postingUpdates.subscribe(0)
	if !ok {
		t.Fatal("subscribe")
	}
	defer l.close()
	done := make(chan error, 1)
	defer func() {
		close(release)
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("scrape exit: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("fixture goroutine did not exit")
		}
	}()
	go func() {
		_, _, err := srv.runScrapeSource(ctx, f, time.Now().UTC(), testStage1Resolver(funding), noopEmit)
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("extraction did not start")
	}
	select {
	case <-l.wake:
	case <-time.After(time.Second):
		t.Fatal("committed insert invisible while provider blocked")
	}
	n := l.take()
	if n.GlobalRevision == 0 || n.UserRevision != 0 || n.Context != "" {
		t.Fatalf("global event leaked private metadata: %+v", n)
	}
	b, err := srv.buildBriefing(ctx, time.Now())
	if err != nil || len(b.Today) != 1 || b.Today[0].Total != 25 {
		t.Fatalf("insert rule projection=%+v err=%v", b, err)
	}
	select {
	case <-done:
		t.Fatal("run terminal while provider blocked")
	default:
	}
}

func TestPostingRefreshAndExtractionPublishAfterWrites(t *testing.T) {
	p := listingPosting("refresh-events", "서버 개발자")
	p.Description = "경력 조건 정보"
	p.FirstSeenAt = time.Now().UTC().Add(-48 * time.Hour)
	p.LastSeenAt = p.FirstSeenAt
	f := &fakeScraper{listing: []scraper.Posting{p}}
	srv, st := newTestServer(t, f)
	ctx := context.Background()
	mustUpsert(t, st, p)
	runtime := testAIRuntime(1, &ai.StubProvider{NameVal: "stub", ExtractFn: func(context.Context, string) (ai.Extraction, ai.Usage, error) {
		return ai.Extraction{Newcomer: true}, ai.Usage{}, nil
	}}, "test-model")
	funding := &stage1Funding{userID: 1, runtime: runtime, budget: srv.newAIBudget(ctx, 1, runtime)}
	l, ok := srv.postingUpdates.subscribe(0)
	if !ok {
		t.Fatal("subscribe")
	}
	defer l.close()
	before := srv.postingUpdates.revisions(0).GlobalRevision
	res, _, err := srv.runScrapeSource(ctx, f, time.Now().UTC(), testStage1Resolver(funding), noopEmit)
	if err != nil || res.Refreshed != 1 {
		t.Fatalf("refresh=%+v err=%v", res, err)
	}
	after := srv.postingUpdates.revisions(0).GlobalRevision
	if after-before != 3 {
		t.Fatalf("seen, detail and extraction require their own durable revisions: %d", after-before)
	}
}

func TestPostingReadBarrierHonorsCancellation(t *testing.T) {
	h := newPostingUpdateHub()
	finish := h.beginWrite(1, true)
	defer finish(false, "")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	end, err := h.beginReadContext(ctx, 1)
	if err == nil || end != nil {
		t.Fatal("blocked first-read barrier ignored cancellation")
	}
}

func TestPostingProgressDoesNotAdvanceDurableRevision(t *testing.T) {
	srv, _ := newTestServer(t, &fakeScraper{})
	l, _ := srv.postingUpdates.subscribe(1)
	defer l.close()
	before := srv.postingUpdates.revisions(1)
	run := srv.rerates.start(1, "today", "")
	srv.rerates.record(1, "today", run.RunID, "progress", "공고 1/2 분석 중...")
	select {
	case <-l.wake:
	case <-time.After(time.Second):
		t.Fatal("background status invisible")
	}
	n := l.take()
	after := srv.postingUpdates.revisions(1)
	if !n.Status || before.UserRevision != after.UserRevision || before.GlobalRevision != after.GlobalRevision {
		t.Fatal("progress certified stored state", n, before, after)
	}
}

func TestPostingRunRecoveryMapIsBounded(t *testing.T) {
	tracker := newRerateTracker()
	for user := int64(1); user <= 1000; user++ {
		run := tracker.start(user, "today", "")
		tracker.complete(user, "today", run.RunID, rerateOutcomeCached, "cached")
	}
	if len(tracker.runs) > 192 {
		t.Fatal("idle run recovery retains unbounded user state", len(tracker.runs))
	}
	if _, ok := tracker.snapshot(1000, "today"); !ok {
		t.Fatal("newest run lost")
	}
}
