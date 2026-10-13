package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/ohchanwu/jobcron/internal/ai"
	"github.com/ohchanwu/jobcron/internal/profile"
	"github.com/ohchanwu/jobcron/internal/storage"
)

// Separate phase measurements complement the real browser's wire/body/DOM
// distributions. These execute the exact shared projection, builders and escaped
// template, not a substitute benchmark page or guessed latency constant.
func TestPostingLargeReadRenderDistributions(t *testing.T) {
	for _, dialect := range []string{"sqlite", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			srv, _, runtime, prof, _ := seedPostingAcceptance(t, dialect)
			ctx := context.Background()
			prof.Dealbreakers = []string{"야근"}
			threshold := 40
			prof.MinScore = &threshold
			pj, _ := profile.Marshal(prof)
			if _, _, err := srv.saveProfileJSON(ctx, runtime.UserID, pj); err != nil {
				t.Fatal(err)
			}
			cipher := &countingCipher{inner: newAIRuntimeTestCipher(t, 0x74)}
			srv.SetCredentialCipher(cipher)
			if dialect == "postgres" {
				saveAIRuntimeCredential(t, srv.store, cipher, runtime.UserID, "anthropic", "performance-synthetic-key")
			}
			factories := 0
			srv.newAIProvider = func(string, string, string, time.Duration) (ai.Provider, error) {
				factories++
				return nil, fmt.Errorf("read constructed a provider")
			}
			now := time.Now().UTC()
			hash := profile.AIInputHash(prof)
			for i := 2; i < 2000; i++ {
				p := listingPosting(fmt.Sprintf("perf-%04d", i), "신입 서버 개발자")
				p.StackTags = []string{"Go"}
				p.Description = "서버 개발자를 찾습니다"
				p.FirstSeenAt = now.Add(-time.Duration(i/50) * 24 * time.Hour)
				p.LastSeenAt = now
				if i%10 == 0 {
					p.Title += " 야근"
				}
				p.ID = mustUpsert(t, srv.store, p)
				if i%4 == 0 {
					if err := srv.store.AddBookmark(ctx, runtime.UserID, p.ID); err != nil {
						t.Fatal(err)
					}
				}
				switch i % 4 {
				case 0:
					if err := srv.store.UpsertAIResult(ctx, runtime.UserID, p.ID, hash, runtime.ScoreVersion, ai.Delta{}, storage.AIScoreOutcome{State: storage.AIScoreNoSignal, ComputedAt: now}); err != nil {
						t.Fatal(err)
					}
				case 1:
					if err := srv.store.UpsertAIScoreFailure(ctx, runtime.UserID, p.ID, hash, runtime.ScoreVersion, storage.AIScoreOutcome{State: storage.AIScoreRejected, Proposed: 1, ComputedAt: now}); err != nil {
						t.Fatal(err)
					}
				}
			}
			type measurement struct {
				Surface, Sort                            string
				Rows, Bytes                              int
				ReadSeconds, GroupSeconds, RenderSeconds float64
			}
			samples := []measurement{}
			for _, mode := range []struct{ surface, sort string }{{"today", "score"}, {"bookmarks", "saved"}, {"archive", "date"}, {"archive", "score"}} {
				for sample := 0; sample < 3; sample++ {
					readCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
					start := time.Now()
					v, err := srv.loadPostingProjection(readCtx, runtime.UserID)
					readTime := time.Since(start)
					if err != nil {
						cancel()
						t.Fatal(err)
					}
					start = time.Now()
					var rows []dashboardPosting
					switch mode.surface {
					case "today":
						b := srv.briefingFromProjection(v, now)
						rows = append(rows, b.Today...)
						rows = append(rows, b.Excluded...)
					case "bookmarks":
						rows = srv.bookmarksFromProjection(v, now).Postings
					case "archive":
						a := srv.archiveFromProjection(v, now)
						if mode.sort == "score" {
							a.applyScoreSort()
						}
						for _, day := range a.Days {
							rows = append(rows, day.Postings...)
						}
						rows = append(rows, a.Excluded...)
					}
					groupTime := time.Since(start)
					start = time.Now()
					var body bytes.Buffer
					for _, row := range rows {
						if err := srv.tmpl.ExecuteTemplate(&body, "posting", row); err != nil {
							cancel()
							t.Fatal(err)
						}
					}
					renderTime := time.Since(start)
					cancel()
					if readTime+groupTime+renderTime >= 10*time.Second {
						t.Fatal("whole representative read/render exceeds snapshot bound")
					}
					if mode.surface == "archive" && len(rows) != 2000 {
						t.Fatal("large archive silently truncated", len(rows))
					}
					samples = append(samples, measurement{mode.surface, mode.sort, len(rows), body.Len(), readTime.Seconds(), groupTime.Seconds(), renderTime.Seconds()})
				}
			}
			if factories != 0 || cipher.opens.Load() != 0 {
				t.Fatal("measurement resolved a provider/credential", factories, cipher.opens.Load())
			}
			encoded, _ := json.Marshal(samples)
			t.Logf("READ_RENDER_DISTRIBUTIONS=%s; provider factories=0 decrypts=0; samples include complete shared builders/templates", encoded)
		})
	}
}
