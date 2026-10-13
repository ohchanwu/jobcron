package server

import (
	"context"
	"sort"
	"time"

	"github.com/ohchanwu/jobcron/internal/ai"
	"github.com/ohchanwu/jobcron/internal/profile"
	"github.com/ohchanwu/jobcron/internal/scoring"
	"github.com/ohchanwu/jobcron/internal/storage"
)

type postingProjection struct {
	data     storage.PostingViewData
	profile  profile.Profile
	enabled  bool
	rows     map[int64]dashboardPosting
	fresh    map[int64]bool
	revision postingNotice
}

func (s *Server) loadPostingProjection(ctx context.Context, userID int64) (postingProjection, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	endRead, err := s.postingUpdates.beginReadContext(ctx, userID)
	if err != nil {
		return postingProjection{}, err
	}
	tx, err := s.store.BeginPostingView(ctx, userID)
	if err != nil {
		endRead()
		return postingProjection{}, err
	}
	defer tx.Close()
	pj, hash, found := tx.Profile()
	var prof profile.Profile
	if found {
		prof, err = profile.Unmarshal(pj)
		if err != nil {
			endRead()
			return postingProjection{}, err
		}
	}
	enabled, err := tx.CredentialPresence(ctx, prof.AIProvider)
	revision := s.postingUpdates.revisions(userID)
	revision.Context = s.postingUpdates.contextToken(userID, hash, enabled)
	endRead()
	if err != nil {
		return postingProjection{}, err
	}
	if s.afterRenderProfileSnapshot != nil {
		s.afterRenderProfileSnapshot()
	}
	model := prof.AIModel
	if model == "" {
		model = ai.DefaultModel(prof.AIProvider)
	}
	d, err := tx.Load(ctx, profile.AIInputHash(prof), ai.ScoreVersion(prof.AIProvider, model), enabled, s.demoMode && s.store.Dialect() == storage.DialectSQLite)
	if err != nil {
		return postingProjection{}, err
	}
	if err := tx.Close(); err != nil {
		return postingProjection{}, err
	}
	if s.afterRenderScoreSnapshot != nil {
		s.afterRenderScoreSnapshot()
	}
	v := postingProjection{data: d, profile: prof, enabled: enabled, rows: map[int64]dashboardPosting{}, fresh: map[int64]bool{}, revision: revision}
	if !found {
		return v, nil
	}
	if s.demoMode {
		v.data.Bookmarks = map[int64]bool{}
		v.data.Muted = map[int64]bool{}
	}
	for _, p := range d.Postings {
		in := storedScoreInputs{Posting: p, Profile: &prof, AIEnabled: enabled, Demo: s.demoMode && s.store.Dialect() == storage.DialectSQLite, ContentHash: d.ContentHashes[p.ID], Validations: d.Validations[p.ID]}
		if ext, ok := d.Extractions[p.ID]; ok {
			in.Extraction = &ext
		}
		if delta, ok := d.Fresh[p.ID]; ok {
			in.Fresh = &delta
		}
		if delta, ok := d.Latest[p.ID]; ok {
			in.Latest = &delta
		}
		if delta, ok := d.AnyVersion[p.ID]; ok {
			in.AnyVersion = &delta
		}
		if outcome, ok := d.Outcomes[p.ID]; ok {
			in.Outcome = &outcome
		}
		composed := composeStoredScore(in)
		result := composed.Score
		v.rows[p.ID] = dashboardPosting{Posting: p, Total: result.Total, Bookmarked: v.data.Bookmarks[p.ID], NotInterested: v.data.Muted[p.ID], DuplicateSources: d.DuplicateSources[p.ID], Breakdown: result.Breakdown, Explanation: scoring.Explain(result), ExclusionReasons: exclusionReasonViews(result.ExclusionReasons), AIOutcome: composed.Outcome, AIConfigured: enabled}
		v.fresh[p.ID] = composed.Fresh
	}
	return v, nil
}

func (v postingProjection) rerateInfo(surface string, lists ...[]dashboardPosting) *rerateInfo {
	if !v.enabled {
		return nil
	}
	info := &rerateInfo{Surface: surface}
	for _, list := range lists {
		for _, dp := range list {
			pending := false
			for _, candidate := range scoring.DealbreakerCandidates(dp.Posting, v.profile) {
				if _, ok := v.data.Validations[dp.Posting.ID][v.data.ContentHashes[dp.Posting.ID]+"\x00"+candidate.ID]; !ok {
					pending = true
					break
				}
			}
			if pending {
				info.PendingContextCount++
			}
			if dp.Excluded {
				if pending {
					info.PendingCount++
				}
				continue
			}
			info.Visible++
			if v.fresh[dp.Posting.ID] {
				info.Analyzed++
			}
			stale := false
			for _, line := range dp.Breakdown {
				if line.Stale {
					stale = true
					break
				}
			}
			if stale {
				info.PendingScoreCount++
			}
			if pending || stale {
				info.PendingCount++
			}
		}
	}
	return info
}

func (s *Server) projectBriefing(ctx context.Context, now time.Time, userID int64) (briefing, error) {
	v, err := s.loadPostingProjection(ctx, userID)
	if err != nil {
		return briefing{}, err
	}
	return s.briefingFromProjection(v, now), nil
}

func (s *Server) briefingFromProjection(v postingProjection, now time.Time) briefing {
	b := briefing{Date: now.In(kstZone).Format("2006 / 01 / 02")}
	disabled := s.disabledSourceSet(v.profile.DisabledSources)
	for _, p := range v.data.Postings {
		dp, ok := v.rows[p.ID]
		if !ok || p.DuplicateOf != nil || disabled[p.Source] || v.data.Muted[p.ID] || !sameKSTDay(p.FirstSeenAt, now) || expired(p, now) {
			continue
		}
		dp.Deadline = deadlineBadge(p.ClosedAt, p.AlwaysOpen, now)
		dp.Excluded = dp.Total < 0 || (dp.Total < v.profile.EffectiveMinScore() && !dp.Bookmarked)
		if dp.Excluded {
			b.Excluded = append(b.Excluded, dp)
		} else {
			b.Today = append(b.Today, dp)
		}
	}
	sort.SliceStable(b.Today, func(i, j int) bool { return b.Today[i].Total > b.Today[j].Total })
	if len(b.Today) > briefingCap {
		b.Today = b.Today[:briefingCap]
	}
	b.Rerate = v.rerateInfo("today", b.Today, b.Excluded)
	b.ShowScheduledAITip = b.Rerate != nil && !v.profile.ScheduledAIEnabled
	b.AIEstimatedDailyCents = v.profile.EffectiveAIDailyUSDCapCents()
	return b
}

func (s *Server) projectArchive(ctx context.Context, now time.Time, userID int64) (archiveView, error) {
	v, err := s.loadPostingProjection(ctx, userID)
	if err != nil {
		return archiveView{}, err
	}
	return s.archiveFromProjection(v, now), nil
}
func (s *Server) archiveFromProjection(v postingProjection, now time.Time) archiveView {
	a := archiveView{Today: now.In(kstZone).Format("2006 / 01 / 02")}
	disabled := s.disabledSourceSet(v.profile.DisabledSources)
	key := ""
	for _, p := range v.data.Postings {
		dp, ok := v.rows[p.ID]
		if !ok || p.DuplicateOf != nil || disabled[p.Source] || v.data.Muted[p.ID] {
			continue
		}
		a.Total++
		dp.Deadline = deadlineBadge(p.ClosedAt, p.AlwaysOpen, now)
		dp.Excluded = dp.Total < 0 || (dp.Total < v.profile.EffectiveMinScore() && !dp.Bookmarked) || expired(p, now)
		if dp.Excluded {
			a.Excluded = append(a.Excluded, dp)
			continue
		}
		day := p.FirstSeenAt.In(kstZone).Format("2006 / 01 / 02")
		if day != key {
			a.Days = append(a.Days, archiveDay{Date: day, IsToday: sameKSTDay(p.FirstSeenAt, now)})
			key = day
		}
		last := &a.Days[len(a.Days)-1]
		last.Postings = append(last.Postings, dp)
	}
	lists := [][]dashboardPosting{a.Excluded}
	for i := range a.Days {
		day := &a.Days[i]
		sort.SliceStable(day.Postings, func(i, j int) bool { return day.Postings[i].Total > day.Postings[j].Total })
		lists = append(lists, day.Postings)
	}
	a.Rerate = v.rerateInfo("archive", lists...)
	return a
}
func (s *Server) projectBookmarks(ctx context.Context, now time.Time, userID int64) (bookmarksView, error) {
	v, err := s.loadPostingProjection(ctx, userID)
	if err != nil {
		return bookmarksView{}, err
	}
	return s.bookmarksFromProjection(v, now), nil
}
func (s *Server) bookmarksFromProjection(v postingProjection, now time.Time) bookmarksView {
	b := bookmarksView{Date: now.In(kstZone).Format("2006 / 01 / 02")}
	ids := v.data.BookmarkOrder
	if s.demoMode {
		ids = nil
		for _, p := range v.data.Postings {
			ids = append(ids, p.ID)
		}
	}
	for _, id := range ids {
		dp, ok := v.rows[id]
		if !ok {
			continue
		}
		dp.Deadline = deadlineBadge(dp.Posting.ClosedAt, dp.Posting.AlwaysOpen, now)
		dp.Excluded = dp.Total < 0
		dp.Dim = dp.Excluded
		b.Postings = append(b.Postings, dp)
	}
	b.Rerate = v.rerateInfo("bookmarks", b.Postings)
	return b
}
