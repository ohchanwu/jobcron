package server

import (
	"context"
	"fmt"
	"sort"

	"github.com/ohchanwu/job-scraper/internal/scoring"
)

// markCrossPortalDuplicates collapses postings that match by source
// scoring.AreDuplicates onto a single canonical row. The canonical is
// the earliest-FirstSeenAt member of each duplicate cluster; later
// members get their duplicate_of column set to the canonical's id.
//
// Runs at the end of runScrape, after sweep and before re-scoring.
// Every pass starts from a clean slate (ClearAllDuplicates) so the
// matcher can be re-tuned without leaving stale duplicate_of values
// from a previous, looser rule in the DB.
//
// Returns the number of newly-marked duplicates this pass.
//
// Cost: O(N²) pairwise comparisons. The matcher is fast (a couple of
// string normalizations + a small Jaccard over short token sets), and
// the current scale is ~300 postings per pass, so the absolute cost
// is well under the SSE event cadence. If posting counts grow past
// a few thousand we can pre-bucket by (normalizedCompany,
// normalizedLocation) to drop the O(N²) — but that's a v1.x concern.
func (s *Server) markCrossPortalDuplicates(ctx context.Context) (int, error) {
	if err := s.store.ClearAllDuplicates(ctx); err != nil {
		return 0, fmt.Errorf("clear duplicates: %w", err)
	}

	postings, err := s.store.AllPostings(ctx)
	if err != nil {
		return 0, fmt.Errorf("read postings: %w", err)
	}

	// Canonical = earliest FirstSeenAt. Stable sort by FirstSeenAt ASC
	// then by ID ASC so two rows seen in the same tick still pick the
	// same winner across runs.
	sort.SliceStable(postings, func(i, j int) bool {
		if postings[i].FirstSeenAt.Equal(postings[j].FirstSeenAt) {
			return postings[i].ID < postings[j].ID
		}
		return postings[i].FirstSeenAt.Before(postings[j].FirstSeenAt)
	})

	// canonicalOf tracks each posting index's chosen canonical row id.
	// A posting that has already been claimed as a duplicate is never
	// chosen as a canonical for some other posting — that would create
	// a chain instead of a star, which the schema discourages.
	taken := make(map[int64]bool, len(postings))
	marked := 0
	for i, canon := range postings {
		if taken[canon.ID] {
			continue // canon is already a duplicate of someone earlier
		}
		for j := i + 1; j < len(postings); j++ {
			cand := postings[j]
			if taken[cand.ID] {
				continue
			}
			if !scoring.AreDuplicates(canon, cand) {
				continue
			}
			if err := s.store.MarkDuplicate(ctx, cand.ID, canon.ID); err != nil {
				return marked, fmt.Errorf("mark dup %d -> %d: %w", cand.ID, canon.ID, err)
			}
			taken[cand.ID] = true
			marked++
		}
	}
	return marked, nil
}
