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

func TestRatingPartialZeroNetKeepsEvidenceAndShowsZero(t *testing.T) {
	srv, stub, runtime := seedRerate(t)
	ctx := context.Background()
	if _, err := srv.store.SQLDB().Exec(`UPDATE postings SET description='서버 개발자를 찾습니다. 함께 협업하는 팀입니다.'`); err != nil {
		t.Fatal(err)
	}
	stub.ScoreDeltaFn = func(context.Context, string, string) ([]ai.RawDeltaItem, ai.Usage, error) {
		return []ai.RawDeltaItem{
			{Signal: "서버", MatchedGoal: "<script>alert(1)</script>", Kind: ai.KindPresence, Delta: 30, Quote: "서버 개발자를 찾습니다"},
			{Signal: "갈등", MatchedGoal: "문화", Kind: ai.KindPresence, Delta: -30, Quote: "함께 협업하는 팀입니다"},
			{Signal: "허구", MatchedGoal: "목표", Kind: ai.KindPresence, Delta: 9, Quote: "실제로 없는 구절"},
		}, ai.Usage{}, nil
	}
	summary, err := srv.runRerate(ctx, "today", noopEmit, 1, runtime)
	if err != nil || summary.Analyzed != 2 || summary.NoSignal != 0 || summary.Rejected != 0 {
		t.Fatalf("valid zero-net became empty/rejected: %+v %v", summary, err)
	}
	outcomes, err := srv.store.AIScoreOutcomesByPostingID(ctx, 1, profile.AIInputHash(currentProfile(t, srv)), runtime.ScoreVersion)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range outcomes {
		if o.State != "rated" || o.Proposed != 3 || o.Accepted != 2 {
			t.Fatalf("partial provenance: %+v", o)
		}
	}
	view, err := srv.buildBriefingWithRuntime(ctx, time.Now(), 1, runtime)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	srv.render(rec, "index.html", view)
	body := rec.Body.String()
	if strings.Count(body, `AI 분석</span><span class="v">0</span>`) != 2 {
		t.Fatal("valid zero-net must show 0 with evidence disclosure")
	}
	if !strings.Contains(body, "서버 개발자를 찾습니다") || !strings.Contains(body, "함께 협업하는 팀입니다") || strings.Contains(body, "<script>alert(1)</script>") || !strings.Contains(body, "&lt;script&gt;") {
		t.Fatal("evidence lost or untrusted goal not escaped")
	}
}
