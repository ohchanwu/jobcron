// Command capture fetches real 점핏 postings (with detail) and writes them to
// internal/scoring/testdata/qa_postings.json. It is a developer tool for
// refreshing the Step 5.5 dealbreaker QA fixture as postings expire — run it
// with `go run ./cmd/capture`. Not part of the shipped product; the release
// workflow builds only ./cmd/jobcron.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ohchanwu/jobcron/internal/scraper"
	"github.com/ohchanwu/jobcron/internal/scraper/jumpit"
)

const (
	want    = 20
	outPath = "internal/scoring/testdata/qa_postings.json"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	s := jumpit.New()
	if err := s.CheckAccess(ctx); err != nil {
		fatal("CheckAccess", err)
	}
	listing, err := s.FetchListing(ctx, want)
	if err != nil {
		fatal("FetchListing", err)
	}
	fmt.Printf("listing: %d 신입 postings\n", len(listing))

	detailed := make([]scraper.Posting, 0, len(listing))
	for i, p := range listing {
		d, err := s.FetchDetail(ctx, p)
		if err != nil {
			fmt.Printf("  [%2d] %s detail FAILED: %v\n", i, p.SourcePostingID, err)
			continue
		}
		detailed = append(detailed, d)
		fmt.Printf("  [%2d] %s  %-20s  desc %d chars\n", i, d.SourcePostingID, d.Company, len(d.Description))
	}

	out, err := json.MarshalIndent(detailed, "", "  ")
	if err != nil {
		fatal("marshal", err)
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		fatal("mkdir", err)
	}
	if err := os.WriteFile(outPath, out, 0o644); err != nil {
		fatal("write", err)
	}
	fmt.Printf("wrote %d postings to %s\n", len(detailed), outPath)
}

func fatal(what string, err error) {
	fmt.Fprintf(os.Stderr, "capture: %s: %v\n", what, err)
	os.Exit(1)
}
