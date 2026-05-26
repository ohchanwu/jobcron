// Package alio is the 잡알리오 (job.alio.go.kr) implementation of the
// scraper.Scraper interface. 잡알리오 is the Korean government's public-
// sector recruitment aggregator; the public-information mandate makes it
// the friendliest legal posture in the entire Korean job-board landscape.
//
// Single-phase: the listing page carries every field we use (title,
// company, location, employment type, posted/closing dates). FetchDetail
// is a no-op — the detail page adds NCS job-family codes and a long-form
// PDF attachment, neither of which we use today.
//
// See API_NOTES.md in this directory for the reverse-engineered URL
// shape and the NCS / career-code values we filter on.
package alio
