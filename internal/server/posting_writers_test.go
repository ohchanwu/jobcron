package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ohchanwu/jobcron/internal/profile"
)

func TestPostingProfileCommitPublishesContextBeforeScoreWrite(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	ctx := context.Background()
	old, _ := profile.Marshal(profile.Profile{JobLikes: "old synthetic goal"})
	if _, _, err := srv.saveProfileJSON(ctx, 0, old); err != nil {
		t.Fatal(err)
	}
	l, _ := srv.postingUpdates.subscribe(0)
	defer l.close()
	before, err := srv.loadPostingProjection(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	next, _ := profile.Marshal(profile.Profile{JobLikes: "new synthetic goal"})
	if _, _, err := srv.saveProfileJSON(ctx, 0, next); err != nil {
		t.Fatal(err)
	}
	select {
	case <-l.wake:
	case <-time.After(time.Second):
		t.Fatal("profile commit invisible before optional rescore")
	}
	n := l.take()
	after, err := srv.loadPostingProjection(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if n.Context == "" || n.Context == before.revision.Context || n.Context != after.revision.Context {
		t.Fatalf("context before=%+v notice=%+v after=%+v", before.revision, n, after.revision)
	}
	scores, err := st.ScoresByPostingID(ctx)
	if err != nil || len(scores) != 0 {
		t.Fatal("context publication rescored")
	}
}

func TestPostingMutePublishesOnlyDurableMembership(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	id := mustUpsert(t, st, listingPosting("mute-notice", "신입"))
	l, _ := srv.postingUpdates.subscribe(0)
	defer l.close()
	for _, method := range []string{"PUT", "DELETE"} {
		r := httptest.NewRequest(method, "/api/not-interested/1", nil)
		r.SetPathValue("id", fmt.Sprint(id))
		w := httptest.NewRecorder()
		if method == "PUT" {
			srv.handleNotInterestedAdd(w, r)
		} else {
			srv.handleNotInterestedRemove(w, r)
		}
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		select {
		case <-l.wake:
		case <-time.After(time.Second):
			t.Fatal("mute membership had no commit notice")
		}
		l.take()
	}
	r := httptest.NewRequest("PUT", "/api/not-interested/99999", nil)
	r.SetPathValue("id", "99999")
	w := httptest.NewRecorder()
	srv.handleNotInterestedAdd(w, r)
	if w.Code == 200 {
		t.Fatal("invalid mute saved")
	}
	select {
	case <-l.wake:
		t.Fatal("failed mute published")
	default:
	}
}

func TestPostingDedupPublishesCanonicalChanges(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	p := listingPosting("canonical", "Go 서버 개발자")
	p.Company = "synthetic company"
	p.Description = "Go 서버 개발자 채용 업무"
	p.FirstSeenAt = time.Now().Add(-time.Hour)
	p.LastSeenAt = time.Now()
	mustUpsert(t, st, p)
	p.Source = "rallit"
	p.SourcePostingID = "sibling"
	p.FirstSeenAt = time.Now()
	mustUpsert(t, st, p)
	l, _ := srv.postingUpdates.subscribe(0)
	defer l.close()
	n, err := srv.markCrossPortalDuplicates(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("dedup=%d err=%v", n, err)
	}
	select {
	case <-l.wake:
	case <-time.After(time.Second):
		t.Fatal("canonical mutation unannounced")
	}
	notice := l.take()
	if notice.GlobalRevision == 0 || notice.UserRevision != 0 {
		t.Fatal(notice)
	}
}

func TestPostingMaterializedScoresPublishOnlyMeaningfulSuccessfulWrites(t *testing.T) {
	for _, dialect := range []string{"sqlite", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var srv *Server
			var userID int64
			if dialect == "postgres" {
				srv, _ = newPostgresTestServer(t, &fakeScraper{})
				userID = insertAIRuntimeTestUser(t, srv.store, "materialized@example.invalid")
			} else {
				srv, _ = newTestServer(t, &fakeScraper{})
			}
			ctx := context.Background()
			pj, _ := profile.Marshal(profile.Profile{CareerYears: 0})
			if _, _, err := srv.saveProfileJSON(ctx, userID, pj); err != nil {
				t.Fatal(err)
			}
			mustUpsert(t, srv.store, listingPosting("materialized", "신입 개발자"))
			l, _ := srv.postingUpdates.subscribe(userID)
			defer l.close()
			if n, err := srv.RescoreAll(ctx, userID, nil); err != nil || n != 1 {
				t.Fatal(n, err)
			}
			select {
			case <-l.wake:
			case <-time.After(time.Second):
				t.Fatal("materialized commit has no user revision")
			}
			first := l.take()
			if first.UserRevision == 0 || first.GlobalRevision != 0 {
				t.Fatal(first)
			}
			if n, err := srv.RescoreAll(ctx, userID, nil); err != nil || n != 1 {
				t.Fatal(n, err)
			}
			select {
			case <-l.wake:
				t.Fatal("ComputedAt-only churn published")
			default:
			}
			if _, err := srv.store.SQLDB().Exec(`DROP TABLE scores`); err != nil {
				t.Fatal(err)
			}
			if _, err := srv.RescoreAll(ctx, userID, nil); err == nil {
				t.Fatal("failed score write succeeded")
			}
			select {
			case <-l.wake:
				t.Fatal("failed score write published")
			default:
			}
		})
	}
}

func TestPostingContextCommitWithCanceledMetadataRequiresRecoverableResync(t *testing.T) {
	srv, _ := newTestServer(t, &fakeScraper{})
	l, _ := srv.postingUpdates.subscribe(0)
	defer l.close()
	ctx, cancel := context.WithCancel(context.Background())
	pj, _ := profile.Marshal(profile.Profile{JobLikes: "new committed synthetic goal"})
	err := srv.writePostingContext(ctx, 0, func() error {
		_, _, err := srv.store.SaveProfile(context.Background(), pj)
		cancel() // Commit succeeded; its optional metadata lookup cannot run.
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	<-l.wake
	n := l.take()
	encoded, _ := json.Marshal(n)
	var notice map[string]any
	json.Unmarshal(encoded, &notice)
	if n.Context != "" || notice["context_changed"] != true || !n.Resync {
		t.Fatal("unavailable metadata fabricated an unmatchable context token", n)
	}
	view, err := srv.loadPostingProjection(context.Background(), 0)
	if err != nil || view.revision.Context == "" {
		t.Fatal("committed context could not recover", err)
	}
}
